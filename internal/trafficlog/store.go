package trafficlog

import (
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite"
)

// NewStore initializes a SQLite database with WAL and pruning pragmas,
// and ensures the schema is up to date.
func NewStore(dbPath string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open sqlite DB at %s: %w", dbPath, err)
	}

	// PRAGMA Configuration for WAL and performance
	pragmas := []string{
		"PRAGMA journal_mode=WAL",
		"PRAGMA busy_timeout=5000",
		"PRAGMA synchronous=NORMAL",
		"PRAGMA auto_vacuum=INCREMENTAL",
	}

	for _, p := range pragmas {
		if _, err := db.Exec(p); err != nil {
			return nil, fmt.Errorf("failed to execute pragma %s: %w", p, err)
		}
	}

	if err := migrateSchema(db); err != nil {
		return nil, fmt.Errorf("failed to migrate schema: %w", err)
	}

	return db, nil
}

func migrateSchema(db *sql.DB) error {
	schema := `
CREATE TABLE IF NOT EXISTS traffic_logs (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    request_id      TEXT    NOT NULL DEFAULT '',
    provider        TEXT    NOT NULL DEFAULT '',
    endpoint        TEXT    NOT NULL DEFAULT '',
    model           TEXT    NOT NULL DEFAULT '',
    method          TEXT    NOT NULL DEFAULT '',
    status_code     INTEGER NOT NULL DEFAULT 0,
    duration_ms     INTEGER NOT NULL DEFAULT 0,
    is_streaming    INTEGER NOT NULL DEFAULT 0,
    config_hash     TEXT    NOT NULL DEFAULT '',
    raw_request     TEXT,
    proc_request    TEXT,
    response        TEXT,
    sse_payload     TEXT,
    error_info      TEXT,
    created_at      DATETIME NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);

CREATE INDEX IF NOT EXISTS idx_tl_provider   ON traffic_logs(provider);
CREATE INDEX IF NOT EXISTS idx_tl_created    ON traffic_logs(created_at);
CREATE INDEX IF NOT EXISTS idx_tl_config     ON traffic_logs(config_hash);
CREATE INDEX IF NOT EXISTS idx_tl_model      ON traffic_logs(model);
CREATE INDEX IF NOT EXISTS idx_tl_request_id ON traffic_logs(request_id);

CREATE TABLE IF NOT EXISTS config_snapshots (
    hash       TEXT PRIMARY KEY,
    content    TEXT NOT NULL,
    created_at DATETIME NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);
`
	_, err := db.Exec(schema)
	return err
}
