package identity

import (
	"strings"
	"testing"
)

func TestDerivePerplexityIdentity_SameCallerSession_SameThread(t *testing.T) {
	a := DerivePerplexityIdentity(PerplexityInput{
		CallerKey: "auth-123",
		SessionID: "session-abc",
	})
	b := DerivePerplexityIdentity(PerplexityInput{
		CallerKey: "auth-123",
		SessionID: "session-abc",
	})
	if a.ThreadID != b.ThreadID {
		t.Fatalf("same caller+session should produce same thread_id: %q != %q", a.ThreadID, b.ThreadID)
	}
	if a.FrontendContextUUID != b.FrontendContextUUID {
		t.Fatalf("same caller+session should produce same frontend_context_uuid: %q != %q", a.FrontendContextUUID, b.FrontendContextUUID)
	}
}

func TestDerivePerplexityIdentity_DifferentCaller_DifferentThread(t *testing.T) {
	a := DerivePerplexityIdentity(PerplexityInput{
		CallerKey: "auth-123",
		SessionID: "session-abc",
	})
	b := DerivePerplexityIdentity(PerplexityInput{
		CallerKey: "auth-456",
		SessionID: "session-abc",
	})
	if a.ThreadID == b.ThreadID {
		t.Fatal("different caller should produce different thread_id")
	}
}

func TestDerivePerplexityIdentity_DifferentSession_DifferentThread(t *testing.T) {
	a := DerivePerplexityIdentity(PerplexityInput{
		CallerKey: "auth-123",
		SessionID: "session-abc",
	})
	b := DerivePerplexityIdentity(PerplexityInput{
		CallerKey: "auth-123",
		SessionID: "session-xyz",
	})
	if a.ThreadID == b.ThreadID {
		t.Fatal("different session should produce different thread_id")
	}
}

func TestDerplexityIdentity_NoSession(t *testing.T) {
	id := DerivePerplexityIdentity(PerplexityInput{
		CallerKey: "auth-123",
		SessionID: "",
	})
	if id.SessionID != "" {
		t.Fatalf("expected empty session_id, got %q", id.SessionID)
	}
	if id.Source != "derived:no-session" {
		t.Fatalf("expected derived:no-session source, got %q", id.Source)
	}
	if id.ThreadID == "" {
		t.Fatal("thread_id should still be produced")
	}
}

func TestDerivePerplexityIdentity_UUIDFormat(t *testing.T) {
	id := DerivePerplexityIdentity(PerplexityInput{
		CallerKey: "auth-123",
		SessionID: "session-abc",
	})
	parts := strings.Split(id.FrontendContextUUID, "-")
	if len(parts) != 5 {
		t.Fatalf("frontend_context_uuid should be UUID-like with 5 parts, got %d: %q", len(parts), id.FrontendContextUUID)
	}
}
