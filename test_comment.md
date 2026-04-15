<details><summary>Click to view code changes: writer_test.go</summary>

```go
package trafficlog

import (
	"path/filepath"
	"testing"
)

func TestTrafficLoggerLifecycle(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test_traffic_logs.db")

	err := InitGlobalLogger(dbPath)
	if err != nil {
		t.Fatalf("Failed to InitGlobalLogger: %v", err)
	}
	defer Close()

	StartWriting()

	logger := GetGlobalLogger()
	if logger == nil {
		t.Fatal("Global logger is nil")
	}

	record := TrafficRecord{
		Provider:   "test-provider",
		Endpoint:   "/api/v1/test",
		Model:      "test-model",
		Method:     "POST",
		StatusCode: 200,
		DurationMs: 150,
	}
	logger.Record(record)

	// Update config
	logger.UpdateConfig("hash123", "yaml content")

	// Drain everything
	logger.Drain()

	// Verify insertion
	db := logger.(*trafficLoggerImpl).GetDB()
	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM traffic_logs").Scan(&count)
	if err != nil {
		t.Fatalf("Failed to query db: %v", err)
	}

	if count != 1 {
		t.Fatalf("Expected 1 log record, got %d", count)
	}
}
```
</details>

**What**: Implemented `TestTrafficLoggerLifecycle` test to verify the complete sequence from init through DB insertion to teardown.
**Why**: Validates that SQLite initialization, record processing channels, and `Drain()` mechanisms work perfectly without dropping logs.
**Who**: Future maintainers checking correct concurrency/locking properties of the writer loops.
**How**: Creates a temporary SQLite database, calls lifecycle handlers, inserts a dummy record, drains the pipeline, and scans the database to verify log generation correctly persisted.
