package geminicli

import (
	"sync"
	"time"
)

// InstanceStatus represents the current state of a sidecar instance.
type InstanceStatus string

const (
	StatusPending   InstanceStatus = "pending"
	StatusStarting InstanceStatus = "starting"
	StatusHealthy  InstanceStatus = "healthy"
	StatusUnhealthy InstanceStatus = "unhealthy"
	StatusCooldown InstanceStatus = "cooldown"
	StatusStopped  InstanceStatus = "stopped"
	StatusFailed   InstanceStatus = "failed"
)

// Instance tracks the runtime state of a single sidecar process.
type Instance struct {
	mu           sync.RWMutex
	ID           string
	Listen       string
	BaseURL      string
	CredsFile    string
	ProjectID    string
	WorkerAPIKey string
	Weight       int

	Status        InstanceStatus
	PID           int
	StartedAt     time.Time
	RestartCount  int
	LastError     string
	LastHealth    time.Time
	CooldownUntil time.Time
}

func (i *Instance) SetStatus(s InstanceStatus) {
	i.mu.Lock()
	i.Status = s
	i.mu.Unlock()
}

func (i *Instance) GetStatus() InstanceStatus {
	i.mu.RLock()
	defer i.mu.RUnlock()
	return i.Status
}

func (i *Instance) IsHealthy() bool {
	i.mu.RLock()
	defer i.mu.RUnlock()
	return i.Status == StatusHealthy
}

func (i *Instance) InCooldown() bool {
	i.mu.RLock()
	defer i.mu.RUnlock()
	return i.Status == StatusCooldown && time.Now().Before(i.CooldownUntil)
}

// DebugInfo returns a snapshot of instance state for the debug endpoint.
type DebugInfo struct {
	ID            string         `json:"id"`
	Status        InstanceStatus `json:"status"`
	PID           int            `json:"pid"`
	Uptime        string         `json:"uptime,omitempty"`
	RestartCount  int            `json:"restart_count"`
	LastError     string         `json:"last_error,omitempty"`
	LastHealth    string         `json:"last_health,omitempty"`
	CooldownUntil string         `json:"cooldown_until,omitempty"`
}

func (i *Instance) DebugSnapshot() DebugInfo {
	i.mu.RLock()
	defer i.mu.RUnlock()
	info := DebugInfo{
		ID:           i.ID,
		Status:       i.Status,
		PID:          i.PID,
		RestartCount: i.RestartCount,
		LastError:    i.LastError,
	}
	if !i.StartedAt.IsZero() {
		info.Uptime = time.Since(i.StartedAt).Truncate(time.Second).String()
	}
	if !i.LastHealth.IsZero() {
		info.LastHealth = i.LastHealth.Format(time.RFC3339)
	}
	if i.Status == StatusCooldown && !i.CooldownUntil.IsZero() {
		info.CooldownUntil = i.CooldownUntil.Format(time.RFC3339)
	}
	return info
}
