package identity

import (
	"net/http"
	"testing"
	"time"
)

func TestNormalize_UsesHeaderPriority(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Now = func() time.Time { return time.Unix(1700000000, 0) }
	headers := http.Header{}
	headers.Set("X-Session-Id", "sess-123")
	headers.Set("User-Agent", "AmpCode/1.0")

	res := Normalize(cfg, Input{Headers: headers, Path: "/v1/messages", APIKeyHash: "k1"})
	if res.Source != "header:x-session-id" {
		t.Fatalf("source = %q, want header:x-session-id", res.Source)
	}
	if res.ClientFamily != "ampcode" {
		t.Fatalf("family = %q, want ampcode", res.ClientFamily)
	}
	if res.SessionIDRaw != "sess-123" {
		t.Fatalf("session raw = %q, want sess-123", res.SessionIDRaw)
	}
	if res.SessionKey == "" || res.ThreadKey == "" {
		t.Fatalf("session/thread keys must be non-empty")
	}
}

func TestNormalize_UsesBodyWhenNoHeaders(t *testing.T) {
	cfg := DefaultConfig()
	res := Normalize(cfg, Input{Body: []byte(`{"conversation_id":"conv-abc"}`), UserAgent: "Claude Code"})
	if res.Source != "body:conversation_id" {
		t.Fatalf("source = %q, want body:conversation_id", res.Source)
	}
	if res.ClientFamily != "claude_code" {
		t.Fatalf("family = %q, want claude_code", res.ClientFamily)
	}
	if res.SessionIDRaw != "conv-abc" {
		t.Fatalf("session raw = %q, want conv-abc", res.SessionIDRaw)
	}
}

func TestNormalize_FallbackDeterministicWithinBucket(t *testing.T) {
	now := time.Unix(1700000000, 0)
	cfg := DefaultConfig()
	cfg.Now = func() time.Time { return now }
	cfg.TimeBucketMinutes = 15
	in := Input{
		Path:       "/v1/messages",
		RemoteAddr: "203.0.113.45:1234",
		APIKeyHash: "apikeyhash",
		UserAgent:  "VSCode",
	}
	a := Normalize(cfg, in)
	b := Normalize(cfg, in)
	if a.Source != "fallback" || b.Source != "fallback" {
		t.Fatalf("expected fallback source")
	}
	if a.SessionKey != b.SessionKey {
		t.Fatalf("session key should be deterministic in bucket")
	}
	if a.ClientFamily != "vscode" {
		t.Fatalf("family = %q, want vscode", a.ClientFamily)
	}
}

func TestStatefulNormalizer_FallbackStableAcrossBucketWithinTTL(t *testing.T) {
	now := time.Unix(1700000000, 0)
	cfg := DefaultConfig()
	cfg.Now = func() time.Time { return now }
	cfg.TimeBucketMinutes = 1
	cfg.SessionTTLMinutes = 10
	n := NewStatefulNormalizer(cfg)
	in := Input{
		Path:       "/v1/messages",
		RemoteAddr: "203.0.113.45:1234",
		APIKeyHash: "apikeyhash",
		UserAgent:  "VSCode",
	}
	a := n.Resolve(in)
	now = now.Add(2 * time.Minute) // different fallback bucket
	b := n.Resolve(in)
	if a.SessionKey != b.SessionKey {
		t.Fatalf("expected same session key within TTL, got %q != %q", a.SessionKey, b.SessionKey)
	}
	now = now.Add(11 * time.Minute) // beyond TTL
	c := n.Resolve(in)
	if b.SessionKey == c.SessionKey {
		t.Fatalf("expected new session key after TTL expiry")
	}
}
