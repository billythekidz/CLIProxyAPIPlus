# Neural Memory Context Reduction Integration

## Objective

Add a new optional module in CLIProxyAPI that reduces oversized prompt context before upstream execution by using `neural-memory` as an external memory brain.

This integration must:

- be toggleable on/off at runtime via config,
- preserve compatibility with existing request protocols (`openai`, `claude`, `gemini`, `codex`),
- run directly from the local submodule source (no remote API dependency),
- degrade safely (pass-through) when memory retrieval fails.

## Feature Folder Layout

All feature assets are colocated in one folder:

`features/neural-memory-context/`

- `ARCHITECTURE.md` (this file)
- `neural-memory/` (git submodule)
- future integration code/docs/tests for this feature

Submodule:

- URL: `https://github.com/nhadaututtheky/neural-memory`
- Path: `features/neural-memory-context/neural-memory`

## Problem Statement

Current traffic can send full thread history to upstream models (notably `claude-opus-4.6` via claudible route), causing:

- higher latency,
- more quota/billing pressure,
- increased limit/rate errors,
- unstable large-context requests.

## High-Level Solution

Insert a pre-execution context optimizer in request pipeline:

1. Parse incoming conversation messages.
2. Estimate input token footprint.
3. If over budget, trim oldest conversational turns.
4. Query `neural-memory` for associative recall based on retained context.
5. Inject compact memory summary as a system/context block.
6. Forward reduced payload to existing executors unchanged.

If module is disabled or unavailable, behavior remains current pass-through.

## Integration Strategy (CLIProxyAPI)

### 1) New Runtime Module

Planned package:

- `internal/runtime/contextmemory/`

Primary responsibilities:

- request normalization (protocol-specific message extraction),
- budget evaluation,
- memory recall adapter,
- context rewrite/injection,
- telemetry counters.

### 2) Neural-Memory Adapter

Planned package:

- `internal/runtime/contextmemory/adapter_neuralmemory.go`

Adapter mode (phase 1):

- shell out to local `nmem` CLI (`nmem context --json`, `nmem recall ...`) with strict timeout.

Adapter mode (phase 2):

- optional long-lived sidecar process for lower overhead.

### 3) Pipeline Hook Point

Apply before executor dispatch (after model mapping, before provider execution):

- preserve existing auth/routing/fill-first behavior,
- only mutate payload body/messages.

### 4) Safety Defaults

- Fail-open (if any memory op fails, forward original request).
- Hard timeout for memory calls (default 250ms-500ms).
- Token guardrails (never exceed configured max injected memory tokens).
- PII-safe logging (no raw prompt dump).

## Config Design (Toggle On/Off)

Add top-level config block:

```yaml
context-memory:
  enabled: true
  model-scope:
    default: "exclude"            # exclude | include
    include:
      - "claude-opus-*"
      - "claude-sonnet-*"
    exclude: []
  provider: "neural-memory"
  mode: "reduce"               # reduce | observe
  token-budget:
    default-max-input: 48000
    per-model:
      - model: "claude-opus-4.6"
        max-input: 36000
  trim-policy:
    keep-system: true
    keep-latest-turns: 8
    drop-oldest-first: true
  neural-memory:
    command: "nmem"
    args-context: ["context", "--json", "--limit", "12"]
    timeout-ms: 400
    max-injected-tokens: 1200
    fallback-on-error: true
  telemetry:
    log-decisions: true
    emit-metrics: true
```

Behavior:

- `enabled=false` => hard OFF for all models (ignore include/exclude lists).
- `enabled=true` + `model-scope.default=exclude` => OFF by default; only models in `include` are active.
- `exclude` list always wins over `include` list.
- `mode=observe` => compute and log decisions but do not rewrite payload.
- `mode=reduce` => rewrite request when above budget.

Default policy target:

- OFF for all models except Opus/Sonnet families.

## Request Rewrite Rules

When above budget:

- Always preserve system instructions (unless invalid/empty).
- Preserve newest conversational turns by `keep-latest-turns`.
- Remove oldest middle history first.
- Inject a synthetic system block:
  - concise memory summary,
  - key decisions/facts/actions from neural-memory recall,
  - bounded by `max-injected-tokens`.

When below budget:

- no mutation.

## Observability

Add structured logs/metrics:

- `context_memory_enabled`
- `context_memory_triggered`
- `tokens_before`, `tokens_after`
- `messages_dropped_count`
- `memory_injected_tokens`
- `memory_lookup_latency_ms`
- `memory_lookup_error_count`

## Rollout Plan

### Phase A (this step)

- Add submodule and architecture/design doc.

### Phase B

- Implement observe-only mode (no payload mutation).
- Validate token estimates and memory call stability.

### Phase C

- Enable reduce mode for selected models (`claude-opus-4.6`, optional others).
- Canary with low traffic and compare error/latency.

### Phase D

- Add sidecar optimization and richer per-protocol heuristics.

## Testing Plan

- Unit:
  - budget decision logic,
  - trim policy correctness,
  - injected context bounds,
  - fail-open behavior.
- Integration:
  - claude/openai/codex request rewrite snapshots,
  - timeout/error fallback to original payload,
  - toggle behavior (`enabled`, `observe`, `reduce`).
- Runtime:
  - A/B compare latency, upstream token usage, and 4xx/5xx rates.

## Risks and Mitigations

- **Risk:** semantic loss after trimming.
  - **Mitigation:** keep latest turns + memory summary + observe mode burn-in.
- **Risk:** CLI dependency instability.
  - **Mitigation:** hard timeout + fail-open + health check.
- **Risk:** prompt injection via recalled memory.
  - **Mitigation:** sanitize/quote memory snippets and cap length.

## Open Decisions

- Direct Python embedding vs CLI adapter (initial recommendation: CLI adapter).
- Exact token estimator implementation reuse from existing executors.
- Whether to store conversation snapshots automatically into neural-memory on each request (phase 2).

## Acceptance Criteria (MVP)

- Feature disabled by default with zero behavior change.
- When enabled in `reduce` mode and budget exceeded:
  - request payload is reduced,
  - memory summary injected,
  - upstream request succeeds with lower input size.
- On memory failures, request still succeeds via original pass-through.
