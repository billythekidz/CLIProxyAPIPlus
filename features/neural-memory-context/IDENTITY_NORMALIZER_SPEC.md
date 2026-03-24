# Identity Normalizer Spec (Neural Memory Context Feature)

## Purpose

Define a unified identity/session layer for heterogeneous clients (`Claude Code`, `AmpCode`, `VSCode`, `OpenClaw`, `OpenCode`, generic OpenAI/Anthropic SDKs) without requiring a shared header standard.

When `context-memory.enabled=true`, this layer also controls how full prompt context is persisted into `neural-memory` for later recall/compression.

## Scope

This spec covers:

- session identity normalization,
- session state persistence,
- full-context prompt capture into neural-memory,
- config and privacy controls,
- runtime pipeline behavior.

It does not define UI dashboards or billing policy.

## Folder and Feature Boundaries

All related assets stay under:

- `features/neural-memory-context/`

Current files:

- `ARCHITECTURE.md`
- `IDENTITY_NORMALIZER_SPEC.md` (this file)
- `neural-memory/` (submodule)

Planned runtime code:

- `internal/runtime/contextmemory/`
- `internal/runtime/identity/`

## Design Goals

1. **No client lock-in**: work even if client sends no custom identity headers.
2. **Stable-enough session key**: same chat flow maps to one internal session most of the time.
3. **Fail-open**: identity resolution failures must not block request forwarding.
4. **Full capture (feature-on)**: persist full incoming prompt context to neural-memory.
5. **Toggleable**: both identity tracking and memory capture configurable at runtime.

## Non-Goals

- Perfect user identity across NAT/shared API keys.
- Replacing upstream provider thread/session semantics.

## Terminology

- **Client family**: inferred source type (`claude_code`, `ampcode`, `vscode`, `openclaw`, `opencode`, `generic`).
- **Session key**: internal normalized key used for tracking (`nm_sess_*`).
- **Thread key**: optional finer-grain key per conversation.
- **Capture record**: normalized payload snapshot stored into neural-memory.

## Runtime Architecture

Pipeline order (for chat-style requests):

1. Authenticate request (existing logic).
2. Infer client family.
3. Normalize identity -> produce `session_key`, `thread_key`.
4. If `context-memory.enabled && capture.enabled`:
   - serialize full prompt context and metadata,
   - write capture record to neural-memory.
5. If context reduction enabled, perform budget/recall rewrite.
6. Execute request via existing routing/executor.
7. Emit structured logs and counters.

## Identity Normalization Algorithm

### Priority Sources

Identity extraction uses first non-empty source in this order:

1. **Explicit headers (preferred)**
   - `X-Session-Id`, `X-Thread-Id`, `X-Conversation-Id`, `X-Client-Id`, `X-User-Id`
2. **Known client headers (family-specific)**
   - e.g. Claude/Amp custom headers if present
3. **Body identifiers**
   - `conversation_id`, `thread_id`, `session_id`, protocol equivalents
4. **Derived fallback fingerprint**
   - `api_key_hash + user_agent + source_ip_masked + path_group + time_bucket`

### Session Key Construction

Canonical format:

`nm_sess:{client_family}:{sha256(compound_identity)[:24]}`

Where `compound_identity` includes:

- strongest available explicit id,
- optional thread id,
- api key hash,
- normalized user-agent signature.

### Thread Key Construction

If a thread/conversation id is present:

`nm_thread:{client_family}:{sha256(thread_identity)[:24]}`

Else inherit session key.

### TTL State Cache

Maintain in-memory map:

- key: `session_key`
- value: last seen metadata (`thread_key`, family, ua sig, first/last seen)
- TTL default: 120 minutes

## Full Prompt Capture Design

When enabled, each eligible request stores **full input context** in neural-memory.

### Capture Trigger

`context-memory.enabled=true` and `context-memory.capture.enabled=true`

### Capture Payload

Store normalized JSON document as memory content:

```json
{
  "capture_version": "v1",
  "timestamp": "2026-03-02T09:00:00Z",
  "session_key": "nm_sess:ampcode:...",
  "thread_key": "nm_thread:ampcode:...",
  "client_family": "ampcode",
  "protocol": "anthropic",
  "path": "/v1/messages",
  "model_requested": "claude-opus-4-6",
  "model_routed": "claude-opus-4.5",
  "prompt_full": {
    "system": "...",
    "messages": ["...full content..."]
  },
  "meta": {
    "api_key_hash": "...",
    "source_ip_masked": "...",
    "user_agent": "..."
  }
}
```

### Write Mode

Phase 1 (MVP):

- call `nmem remember` with structured JSON text (type `context`),
- add tags in content for retrieval filtering (`session_key`, `thread_key`, `model`).

Phase 2:

- use dedicated MCP/server tool path for typed metadata and indexing.

### Record Granularity

- default: one capture per request,
- optional batching window (`capture.batch-window-ms`) to reduce write overhead.

## Config Spec (Proposed)

```yaml
context-memory:
  enabled: false

  identity-normalizer:
    enabled: true
    ttl-minutes: 120
    time-bucket-minutes: 15
    trust-forwarded-for: false
    header-priority:
      - "x-session-id"
      - "x-thread-id"
      - "x-conversation-id"
      - "x-client-id"
      - "x-user-id"
    body-priority:
      - "conversation_id"
      - "thread_id"
      - "session_id"

  capture:
    enabled: true
    mode: "full"                 # full | metadata-only
    max-bytes-per-record: 262144  # 256KB hard cap
    redact:
      enabled: true
      patterns:
        - "api_key"
        - "authorization"
        - "password"
    on-error: "skip"             # skip | block (default skip)

  neural-memory:
    command: "nmem"
    timeout-ms: 500
    write-enabled: true
    read-enabled: true
```

## Privacy and Safety

Because full prompts are stored, enforce strict controls:

- Redact sensitive fields before persistence.
- Byte cap and truncation marker for very large payloads.
- Never store raw `Authorization` headers.
- Store hashed API key fingerprint only.
- Optional opt-out per route/model.

## Failure Handling

- If identity normalization fails -> generate ephemeral session key and continue.
- If neural-memory write fails/timeouts -> skip write and continue request.
- If capture payload too large -> truncate with marker + metadata.

No request should fail solely because memory capture fails (unless explicitly set to `on-error=block`).

## Observability

Structured log fields:

- `session_key`
- `thread_key`
- `client_family`
- `capture_enabled`
- `capture_written`
- `capture_bytes`
- `capture_latency_ms`
- `capture_error`

Metrics:

- `identity_normalizer_requests_total`
- `identity_normalizer_fallback_total`
- `context_capture_success_total`
- `context_capture_error_total`
- `context_capture_bytes_total`

## Compatibility Matrix

- OpenAI-compatible endpoints: supported
- Anthropic-compatible endpoints: supported
- Amp provider routes: supported
- Gemini/Codex protocol paths: supported

If protocol-specific fields differ, extractor maps them into normalized capture schema.

## Implementation Plan

1. Add identity extractor + normalizer package (`internal/runtime/identity`).
2. Add request capture writer (`internal/runtime/contextmemory/capture.go`).
3. Add config structs and validation.
4. Hook into request pipeline before executor dispatch.
5. Add unit tests for:
   - identity fallback precedence,
   - key stability across repeated requests,
   - redaction/truncation,
   - fail-open behavior.
6. Add integration tests for openai/anthropic payload capture.

## Acceptance Criteria

- With feature off: no behavior change.
- With feature on:
  - each eligible request has a stable `session_key`,
  - full prompt context persisted to neural-memory (subject to caps/redaction),
  - request forwarding remains successful when capture fails.
