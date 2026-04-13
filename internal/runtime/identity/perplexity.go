package identity

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

// PerplexityIdentity holds derived Perplexity-specific routing identifiers.
type PerplexityIdentity struct {
	SessionID           string
	ThreadID            string
	FrontendContextUUID string
	Source              string
}

// PerplexityInput carries the inputs needed to derive a Perplexity thread identity.
type PerplexityInput struct {
	// CallerKey is a stable identifier for the caller (e.g. auth ID hash).
	CallerKey string
	// SessionID is the execution session ID from request context.
	SessionID string
}

// DerivePerplexityIdentity produces deterministic Perplexity routing identifiers
// from callerKey + sessionID only. This ensures all requests from the same
// caller session share one Perplexity thread regardless of model alias or
// message content.
func DerivePerplexityIdentity(in PerplexityInput) PerplexityIdentity {
	callerKey := strings.TrimSpace(in.CallerKey)
	if callerKey == "" {
		callerKey = "-"
	}
	sessionID := strings.TrimSpace(in.SessionID)

	if sessionID == "" {
		return PerplexityIdentity{
			SessionID:           "",
			ThreadID:            "",
			FrontendContextUUID: "",
			Source:              "passthrough:no-session",
		}
	}

	// Thread seed is callerKey + sessionID — no message content.
	// Same caller + same session = same thread, even if model differs.
	threadSeed := callerKey + "|" + sessionID

	threadHash := sha256.Sum256([]byte("perplexity:thread:" + threadSeed))
	threadID := hex.EncodeToString(threadHash[:8])

	contextSeed := callerKey + "|" + sessionID + "|ctx"
	contextHash := sha256.Sum256([]byte("perplexity:context:" + contextSeed))
	frontendContextUUID := hex.EncodeToString(contextHash[:4]) + "-" +
		hex.EncodeToString(contextHash[4:6]) + "-" +
		hex.EncodeToString(contextHash[6:8]) + "-" +
		hex.EncodeToString(contextHash[8:10]) + "-" +
		hex.EncodeToString(contextHash[10:16])

	return PerplexityIdentity{
		SessionID:           sessionID,
		ThreadID:            threadID,
		FrontendContextUUID: frontendContextUUID,
		Source:              "derived",
	}
}
