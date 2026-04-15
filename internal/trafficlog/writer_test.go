package trafficlog

import (
	"path/filepath"
	"runtime"
	"sync"
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

	// Update config — should be routed through writer goroutine
	logger.UpdateConfig("hash123", "yaml: content")
	if logger.CurrentConfigHash() != "hash123" {
		t.Fatalf("Expected config hash 'hash123', got '%s'", logger.CurrentConfigHash())
	}

	// Second UpdateConfig with same hash should be a no-op
	logger.UpdateConfig("hash123", "yaml: content")

	// Drain everything
	logger.Drain()

	// Verify log insertion
	db := logger.(*trafficLoggerImpl).GetDB()
	var logCount int
	if err = db.QueryRow("SELECT COUNT(*) FROM traffic_logs").Scan(&logCount); err != nil {
		t.Fatalf("Failed to query db: %v", err)
	}
	if logCount != 1 {
		t.Fatalf("Expected 1 log record, got %d", logCount)
	}

	// Verify config snapshot insertion (only 1, not 2 — dedup works)
	var snapCount int
	if err = db.QueryRow("SELECT COUNT(*) FROM config_snapshots").Scan(&snapCount); err != nil {
		t.Fatalf("Failed to query config_snapshots: %v", err)
	}
	if snapCount != 1 {
		t.Fatalf("Expected 1 config snapshot, got %d", snapCount)
	}
}

func TestConcurrentRecords(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "concurrent_test.db")

	if err := InitGlobalLogger(dbPath); err != nil {
		t.Fatalf("InitGlobalLogger failed: %v", err)
	}
	defer Close()

	StartWriting()
	logger := GetGlobalLogger()

	const n = 200
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			logger.Record(TrafficRecord{
				Provider:   "concurrent",
				Endpoint:   "/v1/test",
				StatusCode: 200,
			})
			runtime.Gosched()
		}(i)
	}
	wg.Wait()
	logger.Drain()

	db := logger.(*trafficLoggerImpl).GetDB()
	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM traffic_logs").Scan(&count); err != nil {
		t.Fatalf("count query failed: %v", err)
	}
	// Some may be dropped if chan fills, but there should be no panics and no DB errors
	if count < 1 {
		t.Fatalf("Expected at least 1 record, got %d", count)
	}
}

func TestDrainIdempotent(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "drain_test.db")

	if err := InitGlobalLogger(dbPath); err != nil {
		t.Fatalf("InitGlobalLogger failed: %v", err)
	}

	logger := GetGlobalLogger()

	// Calling Drain multiple times must not panic
	logger.Drain()
	logger.Drain()
	logger.Drain()

	// Clean up
	logger.Close() //nolint:errcheck
}
