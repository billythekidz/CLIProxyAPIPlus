package geminicli

import (
	"testing"
	"time"
)

func TestInstanceStatusTransitions(t *testing.T) {
	inst := &Instance{ID: "test-1", Status: StatusPending}

	if inst.IsHealthy() {
		t.Fatal("new instance should not be healthy")
	}

	inst.SetStatus(StatusHealthy)
	if !inst.IsHealthy() {
		t.Fatal("expected healthy after SetStatus")
	}
	if inst.GetStatus() != StatusHealthy {
		t.Fatal("GetStatus mismatch")
	}

	inst.SetStatus(StatusCooldown)
	inst.mu.Lock()
	inst.CooldownUntil = time.Now().Add(1 * time.Hour)
	inst.mu.Unlock()
	if !inst.InCooldown() {
		t.Fatal("expected in cooldown")
	}

	inst.mu.Lock()
	inst.CooldownUntil = time.Now().Add(-1 * time.Minute)
	inst.mu.Unlock()
	if inst.InCooldown() {
		t.Fatal("expected cooldown expired")
	}
}

func TestDebugSnapshot(t *testing.T) {
	inst := &Instance{
		ID:           "test-1",
		Status:       StatusHealthy,
		PID:          12345,
		StartedAt:    time.Now().Add(-5 * time.Minute),
		RestartCount: 2,
		LastHealth:   time.Now(),
	}

	snap := inst.DebugSnapshot()
	if snap.ID != "test-1" {
		t.Fatalf("expected id test-1, got %s", snap.ID)
	}
	if snap.Status != StatusHealthy {
		t.Fatalf("expected healthy, got %s", snap.Status)
	}
	if snap.PID != 12345 {
		t.Fatalf("expected PID 12345, got %d", snap.PID)
	}
	if snap.RestartCount != 2 {
		t.Fatalf("expected restart count 2, got %d", snap.RestartCount)
	}
	if snap.Uptime == "" {
		t.Fatal("expected non-empty uptime")
	}
	if snap.LastHealth == "" {
		t.Fatal("expected non-empty last health")
	}
}

func TestParseDuration(t *testing.T) {
	cases := []struct {
		input    string
		fallback time.Duration
		want     time.Duration
	}{
		{"30s", time.Minute, 30 * time.Second},
		{"2m", time.Second, 2 * time.Minute},
		{"", 45 * time.Second, 45 * time.Second},
		{"invalid", 10 * time.Second, 10 * time.Second},
	}
	for _, tc := range cases {
		got := parseDuration(tc.input, tc.fallback)
		if got != tc.want {
			t.Errorf("parseDuration(%q, %v) = %v, want %v", tc.input, tc.fallback, got, tc.want)
		}
	}
}
