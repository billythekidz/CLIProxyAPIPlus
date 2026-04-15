package trafficlog

import "database/sql"

// TrafficRecord represents a single proxy request lifecycle log entry.
type TrafficRecord struct {
	RequestID   string
	Provider    string
	Endpoint    string
	Model       string
	Method      string
	StatusCode  int
	DurationMs  int64
	IsStreaming bool
	ConfigHash  string
	RawRequest  string
	ProcRequest string
	Response    string
	SSEPayload  string
	ErrorInfo   string
}

// Logger is the interface for recording traffic logs.
type Logger interface {
	Record(record TrafficRecord)
	CurrentConfigHash() string
	UpdateConfig(hash string, yamlContent string)
	GetDB() *sql.DB
	Drain()
	Close() error
}
