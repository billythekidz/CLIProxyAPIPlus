package trafficlog

import (
	"database/sql"
	"log"
	"os"
	"time"
)

// Pruner runs a background ticker and cleans up database if > 1GB
type Pruner struct {
	db     *sql.DB
	dbPath string
	ticker *time.Ticker
	done   chan struct{}
}

func NewPruner(db *sql.DB, dbPath string) *Pruner {
	return &Pruner{
		db:     db,
		dbPath: dbPath,
		ticker: time.NewTicker(30 * time.Minute),
		done:   make(chan struct{}),
	}
}

func (p *Pruner) Start() {
	go func() {
		// Run initial check
		p.pruneIfNeeded()

		for {
			select {
			case <-p.ticker.C:
				p.pruneIfNeeded()
			case <-p.done:
				p.ticker.Stop()
				return
			}
		}
	}()
}

func (p *Pruner) Stop() {
	close(p.done)
}

func (p *Pruner) pruneIfNeeded() {
	fi, err := os.Stat(p.dbPath)
	if err != nil {
		if !os.IsNotExist(err) {
			log.Printf("[TrafficLogger Pruner] Error stat db: %v", err)
		}
		return
	}

	const maxSizeBytes = 1 * 1024 * 1024 * 1024 // 1GB
	const targetBytes = 900 * 1024 * 1024       // 900MB

	if fi.Size() <= maxSizeBytes {
		return
	}

	log.Printf("[TrafficLogger Pruner] DB size %d bytes > 1GB. Starting pruning...", fi.Size())

	for {
		// Delete 5000 oldest records at a time
		delQuery := `
			DELETE FROM traffic_logs
			WHERE id IN (
				SELECT id FROM traffic_logs
				ORDER BY created_at ASC
				LIMIT 5000
			)
		`
		if _, err := p.db.Exec(delQuery); err != nil {
			log.Printf("[TrafficLogger Pruner] Error deleting records: %v", err)
			break
		}

		// Reclaim space using incremental vacuum
		if _, err := p.db.Exec("PRAGMA incremental_vacuum(1000)"); err != nil {
			log.Printf("[TrafficLogger Pruner] Error during incremental vacuum: %v", err)
		}

		// Re-check size
		fi, err = os.Stat(p.dbPath)
		if err != nil || fi.Size() <= targetBytes {
			break
		}
	}

	// Also prune orphaned config snapshots (no traffic record references them)
	pruneConfigQuery := `
		DELETE FROM config_snapshots 
		WHERE hash NOT IN (SELECT DISTINCT config_hash FROM traffic_logs)
	`
	if _, err := p.db.Exec(pruneConfigQuery); err != nil {
		log.Printf("[TrafficLogger Pruner] Error pruning config snapshots: %v", err)
	}

	log.Printf("[TrafficLogger Pruner] Pruning completed.")
}
