package trafficlog

import "sync"

var (
	globalMu     sync.RWMutex
	globalLogger Logger
	globalPruner *Pruner
)

// SetGlobalLogger sets the global traffic logger instance (thread-safe).
func SetGlobalLogger(l Logger) {
	globalMu.Lock()
	defer globalMu.Unlock()
	globalLogger = l
}

// GetGlobalLogger returns the global traffic logger instance (thread-safe).
// Returns nil if not initialized.
func GetGlobalLogger() Logger {
	globalMu.RLock()
	defer globalMu.RUnlock()
	return globalLogger
}

// InitGlobalLogger initializes the DB, Logger, and Pruner and stores them.
// Safe to call multiple times; subsequent calls are no-ops if already initialized.
func InitGlobalLogger(dbPath string) error {
	globalMu.Lock()
	defer globalMu.Unlock()

	if globalLogger != nil {
		return nil // already initialized
	}

	db, err := NewStore(dbPath)
	if err != nil {
		return err
	}

	logger := NewTrafficLogger(db)
	globalLogger = logger
	globalPruner = NewPruner(db, dbPath)

	return nil
}

// StartWriting starts the background pruner. The writer goroutine is started by
// NewTrafficLogger; this only needs to start the pruner.
func StartWriting() {
	globalMu.RLock()
	p := globalPruner
	globalMu.RUnlock()

	if p != nil {
		p.Start()
	}
}

// Close fully stops and cleans up global traffic logging routines.
func Close() {
	globalMu.Lock()
	p := globalPruner
	l := globalLogger
	globalPruner = nil
	globalLogger = nil
	globalMu.Unlock()

	if p != nil {
		p.Stop()
	}
	if l != nil {
		l.Drain()
		l.Close() //nolint:errcheck
	}
}
