package executor

import (
	"net/http"
	"strings"
	"testing"
)

func TestCodexTerminalEventError_ContextLengthExceeded(t *testing.T) {
	raw := []byte(`{"type":"response.failed","response":{"error":{"code":"context_length_exceeded","message":"Your input exceeds the context window of this model."}}}`)

	status, body, ok := codexTerminalEventError(raw)
	if !ok {
		t.Fatalf("expected ok=true")
	}
	if status != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", status, http.StatusBadRequest)
	}
	if !strings.Contains(body, `"type":"invalid_request_error"`) {
		t.Fatalf("body missing invalid_request_error: %s", body)
	}
	if !strings.Contains(body, `"code":"context_length_exceeded"`) {
		t.Fatalf("body missing context_length_exceeded code: %s", body)
	}
}

func TestCodexTerminalEventError_RateLimit(t *testing.T) {
	raw := []byte(`{"type":"response.failed","response":{"error":{"code":"rate_limit_exceeded","message":"Too many requests"}}}`)

	status, body, ok := codexTerminalEventError(raw)
	if !ok {
		t.Fatalf("expected ok=true")
	}
	if status != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want %d", status, http.StatusTooManyRequests)
	}
	if !strings.Contains(body, `"type":"rate_limit_error"`) {
		t.Fatalf("body missing rate_limit_error: %s", body)
	}
}

func TestCodexTerminalEventError_NonTerminal(t *testing.T) {
	raw := []byte(`{"type":"response.created"}`)

	_, _, ok := codexTerminalEventError(raw)
	if ok {
		t.Fatalf("expected ok=false")
	}
}

