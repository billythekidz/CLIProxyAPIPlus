package trafficlog

import (
	"database/sql"
	"log"
	"sync"
)

// configSnapshot is an internal command send through the record channel so that
// config snapshot INSERT runs on the single-writer goroutine — avoiding concurrent writes.
type configSnapshot struct {
	hash    string
	content string
}

// writerCmd is a discriminated union passed through recordChan.
type writerCmd struct {
	record   *TrafficRecord
	snapshot *configSnapshot
}

type trafficLoggerImpl struct {
	db         *sql.DB
	cmdChan    chan writerCmd
	configHash string
	configMu   sync.RWMutex
	wg         sync.WaitGroup
	drainOnce  sync.Once
	done       chan struct{}
}

// NewTrafficLogger creates a new SQLite traffic logger and starts the
// background single-writer goroutine safely.
func NewTrafficLogger(db *sql.DB) Logger {
	l := &trafficLoggerImpl{
		db:      db,
		cmdChan: make(chan writerCmd, 256),
		done:    make(chan struct{}),
	}

	l.wg.Add(1)
	go l.writerTask()

	return l
}

// Record enqueues a traffic record for async write. Non-blocking; drops if channel full.
func (l *trafficLoggerImpl) Record(record TrafficRecord) {
	cmd := writerCmd{record: &record}
	select {
	case <-l.done:
		// Logger is shutting down; silently drop.
	case l.cmdChan <- cmd:
		// Enqueued.
	default:
		log.Println("[TrafficLogger] Warning: record channel is full, dropping traffic log")
	}
}

// CurrentConfigHash returns the current config hash (thread-safe).
func (l *trafficLoggerImpl) CurrentConfigHash() string {
	l.configMu.RLock()
	defer l.configMu.RUnlock()
	return l.configHash
}

// UpdateConfig updates the config hash and, if changed, enqueues a snapshot INSERT
// onto the single-writer goroutine so there are zero concurrent DB writes.
func (l *trafficLoggerImpl) UpdateConfig(hash string, yamlContent string) {
	l.configMu.Lock()
	if l.configHash == hash {
		l.configMu.Unlock()
		return
	}
	l.configHash = hash
	l.configMu.Unlock()

	// Route snapshot insert through the writer goroutine to keep single-writer semantics.
	cmd := writerCmd{snapshot: &configSnapshot{hash: hash, content: yamlContent}}
	select {
	case <-l.done:
	case l.cmdChan <- cmd:
	default:
		log.Printf("[TrafficLogger] Warning: channel full, config snapshot for %s dropped", hash)
	}
}

// GetDB returns the underlying database connection.
func (l *trafficLoggerImpl) GetDB() *sql.DB {
	return l.db
}

func (l *trafficLoggerImpl) writerTask() {
	defer l.wg.Done()

	insertQuery := `
		INSERT INTO traffic_logs (
			request_id, provider, endpoint, model, method, status_code,
			duration_ms, is_streaming, config_hash, raw_request, proc_request,
			response, sse_payload, error_info
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	stmt, err := l.db.Prepare(insertQuery)
	if err != nil {
		log.Printf("[TrafficLogger] Fatal: failed to prepare statement: %v", err)
		return
	}
	defer stmt.Close()

	for {
		select {
		case cmd := <-l.cmdChan:
			l.processCmd(stmt, cmd)
		case <-l.done:
			// Drain remaining messages before exiting.
			for {
				select {
				case cmd := <-l.cmdChan:
					l.processCmd(stmt, cmd)
				default:
					return
				}
			}
		}
	}
}

func (l *trafficLoggerImpl) processCmd(stmt *sql.Stmt, cmd writerCmd) {
	if cmd.snapshot != nil {
		l.insertSnapshot(cmd.snapshot)
		return
	}
	if cmd.record != nil {
		l.insertRecord(stmt, cmd.record)
	}
}

func (l *trafficLoggerImpl) insertSnapshot(snap *configSnapshot) {
	_, err := l.db.Exec(
		`INSERT OR IGNORE INTO config_snapshots (hash, content) VALUES (?, ?)`,
		snap.hash, snap.content,
	)
	if err != nil {
		log.Printf("[TrafficLogger] Error saving config snapshot: %v", err)
	}
}

func (l *trafficLoggerImpl) insertRecord(stmt *sql.Stmt, r *TrafficRecord) {
	isStreaming := 0
	if r.IsStreaming {
		isStreaming = 1
	}
	_, err := stmt.Exec(
		r.RequestID, r.Provider, r.Endpoint, r.Model, r.Method, r.StatusCode,
		r.DurationMs, isStreaming, r.ConfigHash, r.RawRequest, r.ProcRequest,
		r.Response, r.SSEPayload, r.ErrorInfo,
	)
	if err != nil {
		log.Printf("[TrafficLogger] Error inserting record: %v", err)
	}
}

// Drain signals the writer goroutine to stop and waits for it to finish.
// Safe to call multiple times (idempotent via sync.Once).
func (l *trafficLoggerImpl) Drain() {
	l.drainOnce.Do(func() {
		close(l.done)
	})
	l.wg.Wait()
}

// Close closes the underlying database connection. Call after Drain().
func (l *trafficLoggerImpl) Close() error {
	return l.db.Close()
}
