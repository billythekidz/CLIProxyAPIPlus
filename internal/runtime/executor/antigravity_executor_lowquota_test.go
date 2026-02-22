package executor

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v6/internal/config"
	cliproxyauth "github.com/router-for-me/CLIProxyAPI/v6/sdk/cliproxy/auth"
)

func TestAntigravityLowQuotaRetryAfter_FillFirstTriggersRotation(t *testing.T) {
	server := newAntigravityModelsServer(t, 0.08, time.Now().Add(30*time.Minute))
	defer server.Close()

	exec := &AntigravityExecutor{cfg: &config.Config{}}
	exec.cfg.Routing.Strategy = "fill-first"
	exec.cfg.QuotaExceeded.SwitchProject = true

	auth := &cliproxyauth.Auth{Metadata: map[string]any{"base_url": server.URL}}
	retryAfter := exec.lowQuotaRetryAfter(context.Background(), auth, "dummy-token", "gemini-3-flash")
	if retryAfter == nil {
		t.Fatalf("expected retryAfter when remainingFraction is below threshold")
	}
	if *retryAfter < antigravityLowQuotaMinCooldown {
		t.Fatalf("expected retryAfter >= %s, got %s", antigravityLowQuotaMinCooldown, *retryAfter)
	}
}

func TestAntigravityLowQuotaRetryAfter_NotFillFirstSkipsRotation(t *testing.T) {
	exec := &AntigravityExecutor{cfg: &config.Config{}}
	exec.cfg.Routing.Strategy = "round-robin"
	exec.cfg.QuotaExceeded.SwitchProject = true

	auth := &cliproxyauth.Auth{Metadata: map[string]any{"base_url": "http://127.0.0.1:1"}}
	retryAfter := exec.lowQuotaRetryAfter(context.Background(), auth, "dummy-token", "gemini-3-flash")
	if retryAfter != nil {
		t.Fatalf("expected nil retryAfter when strategy is not fill-first")
	}
}

func TestAntigravityLowQuotaRetryAfter_HealthyQuotaNoRotation(t *testing.T) {
	server := newAntigravityModelsServer(t, 0.42, time.Now().Add(30*time.Minute))
	defer server.Close()

	exec := &AntigravityExecutor{cfg: &config.Config{}}
	exec.cfg.Routing.Strategy = "fill-first"
	exec.cfg.QuotaExceeded.SwitchProject = true

	auth := &cliproxyauth.Auth{Metadata: map[string]any{"base_url": server.URL}}
	retryAfter := exec.lowQuotaRetryAfter(context.Background(), auth, "dummy-token", "gemini-3-flash")
	if retryAfter != nil {
		t.Fatalf("expected nil retryAfter for healthy quota")
	}
}

func newAntigravityModelsServer(t *testing.T, remaining float64, resetAt time.Time) *httptest.Server {
	t.Helper()
	resetValue := resetAt.UTC().Format(time.RFC3339)
	body := fmt.Sprintf(`{"models":{"gemini-3-flash":{"quotaInfo":{"remainingFraction":%.4f,"resetTime":"%s"}}}}`, remaining, resetValue)
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != antigravityModelsPath {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
}
