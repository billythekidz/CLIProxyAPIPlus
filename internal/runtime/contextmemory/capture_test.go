package contextmemory

import (
	"context"
	"strings"
	"testing"
)

func TestCaptureWriter_BuildsRememberCommand(t *testing.T) {
	cfg := CaptureConfig{Enabled: true, WriteEnabled: true, Command: "nmem", RedactEnabled: true}
	w := NewCaptureWriter(cfg)
	var gotName string
	var gotArgs []string
	w.SetRunnerForTest(func(ctx context.Context, name string, args ...string) ([]byte, error) {
		gotName = name
		gotArgs = append([]string{}, args...)
		return []byte(`{"ok":true}`), nil
	})

	err := w.Capture(context.Background(), CaptureRequest{
		SessionKey:     "nm_sess:test",
		ThreadKey:      "nm_thread:test",
		ClientFamily:   "ampcode",
		Protocol:       "anthropic",
		Path:           "/v1/messages",
		ModelRequested: "claude-opus-4-6",
		ModelRouted:    "claude-opus-4.5",
		PromptFull:     []byte(`{"messages":[{"role":"user","content":"hi"}],"api_key":"secret"}`),
	})
	if err != nil {
		t.Fatalf("Capture() error = %v", err)
	}
	if gotName != "nmem" {
		t.Fatalf("runner name = %q, want nmem", gotName)
	}
	joined := strings.Join(gotArgs, " ")
	if !strings.Contains(joined, "remember") || !strings.Contains(joined, "--type context") {
		t.Fatalf("args missing remember/type: %s", joined)
	}
	if !strings.Contains(joined, "[REDACTED]") {
		t.Fatalf("expected redaction marker in payload args")
	}
}

func TestBuildCaptureContent_TruncatesLargePayload(t *testing.T) {
	cfg := CaptureConfig{MaxBytesPerRecord: 2048, RedactEnabled: false}
	big := strings.Repeat("x", 8000)
	out, err := buildCaptureContent(cfg, CaptureRequest{PromptFull: []byte(big)})
	if err != nil {
		t.Fatalf("buildCaptureContent() error = %v", err)
	}
	if len(out) > 2048 {
		t.Fatalf("output too large: %d", len(out))
	}
	if !strings.Contains(out, "TRUNCATED_BY_CLIPROXY_CONTEXT_MEMORY") {
		t.Fatalf("expected truncation marker in output")
	}
}

func TestCaptureWriter_OnErrorSkip(t *testing.T) {
	cfg := CaptureConfig{Enabled: true, WriteEnabled: true, OnError: "skip"}
	w := NewCaptureWriter(cfg)
	w.SetRunnerForTest(func(ctx context.Context, name string, args ...string) ([]byte, error) {
		return nil, context.DeadlineExceeded
	})
	err := w.Capture(context.Background(), CaptureRequest{PromptFull: []byte(`{"messages":[1]}`)})
	if err != nil {
		t.Fatalf("expected nil error on skip mode, got %v", err)
	}
}
