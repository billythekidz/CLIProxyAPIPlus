# Neural-Memory Integration Plan for CLIProxyAPI

## Goal

Design a production-ready integration that:

1. captures **all sessions with identity** into neural-memory when feature is enabled,
2. reconstructs a compact, high-value prompt context from neural-memory,
3. reduces prompt cost while preserving task continuity.
4. supports model-scoped activation with global toggle + include/exclude filters.

Primary optimization objective for this feature is **token minimization**.

- Hard ceiling per forwarded prompt context: **100,000 input tokens**
- Preferred operating band: **40,000-60,000 input tokens**
- Allow smaller contexts whenever quality is preserved (for example 20,000-40,000)

This plan builds on:

- `features/neural-memory-context/ARCHITECTURE.md`
- `features/neural-memory-context/IDENTITY_NORMALIZER_SPEC.md`

## Research Notes from Neural-Memory Repo

Based on usage docs and source code in submodule `features/neural-memory-context/neural-memory`:

### CLI capabilities (usable from CLIProxy)

- `nmem remember` supports:
  - `--type` (`fact`, `decision`, `context`, `error`, ...),
  - `--tag`, `--priority`, `--expires`, `--json`
  - source: `src/neural_memory/cli/commands/memory.py:128`
- `nmem recall` supports:
  - depth `0..3`, `--max-tokens`, `--min-confidence`, `--json`
  - source: `src/neural_memory/cli/commands/memory.py:336`
- `nmem context` supports:
  - `--limit`, `--fresh-only`, `--json`
  - returns recent context + per-fiber metadata
  - source: `src/neural_memory/cli/commands/memory.py:439`

### MCP/tool behavior (future option)

- `nmem_remember` supports tags, priority, expiry, encryption path
  - source: `src/neural_memory/mcp/tool_handlers.py:84`
- `nmem_recall` and `nmem_context` support token-bounded retrieval and confidence
  - source: `src/neural_memory/mcp/tool_handlers.py:444`, `:670`
- built-in session tool exists (`nmem_session`) with `session_state` and `session_summary`
  - source: `src/neural_memory/mcp/session_handler.py:45`

### Context optimization internals we can reuse conceptually

- neural-memory already ranks context with composite scoring and token budgeting
  - source: `src/neural_memory/engine/context_optimizer.py`

## Integration Mode Choice

### Phase 1 (recommended now): CLI adapter

CLIProxy calls `nmem` directly:

- write path: `nmem remember`
- read path: `nmem recall` + `nmem context`

Pros: simplest deployment, no JSON-RPC server plumbing.

### Phase 2 (optional): MCP adapter

Use `nmem-mcp` for richer typed session semantics (`nmem_session`, structured recalls).

## Data Capture Design (All Sessions + Identity)

When `context-memory.enabled=true` and `capture.enabled=true`, for every eligible request:

1. identity-normalizer resolves:
   - `session_key`, `thread_key`, `client_family`, `api_key_hash`
2. build capture payload from full incoming prompt/messages,
3. persist to neural-memory as `context` memory type.

Eligibility is model-scoped (see config section): request is eligible only when model passes include/exclude policy.

## Model-Scoped Toggle Policy

Feature activation has two levels:

1. **Global toggle** (`context-memory.enabled`)
2. **Per-model scope** (`model-scope.include` / `model-scope.exclude`)

Policy rules:

- If `enabled=false` -> feature OFF for all models.
- If `enabled=true` -> evaluate model scope.
- `exclude` has highest priority.
- If not excluded:
  - with `default=exclude`, model must match `include` to be ON,
  - with `default=include`, model is ON unless matched by `exclude`.

Default rollout policy:

- OFF for all models except Opus/Sonnet families.
- Example include patterns:
  - `claude-opus-*`
  - `claude-sonnet-*`

### Storage shape in neural-memory (Phase 1)

Use `nmem remember` with:

- `--type context`
- tags:
  - `session:{session_key}`
  - `thread:{thread_key}`
  - `client:{client_family}`
  - `model:{requested_model}`
  - `scope:full_prompt`
  - `scope:shortterm` (current session)
- priority:
  - short-term request captures: `6`
  - long-term distilled decisions (optional): `8`

Content body format: compact JSON string

```json
{
  "v": 1,
  "ts": "2026-03-02T10:00:00Z",
  "session_key": "nm_sess:...",
  "thread_key": "nm_thread:...",
  "client_family": "ampcode",
  "protocol": "anthropic",
  "path": "/v1/messages",
  "model_requested": "claude-opus-4-6",
  "model_routed": "claude-opus-4.5",
  "prompt_full": {"messages": [...]}
}
```

## Memory Buckets for Context Assembly

To optimize reducer quality, assemble context from five buckets:

1. **Full short-term memory (current session identity)**
   - all captured prompts in current `session_key` within recent TTL window.
2. **Latest short-term memory (current session identity)**
   - most recent high-signal turns for the same session/thread.
3. **Full long-term memory (all sessions identity)**
   - older cross-session facts/decisions/workflows for project/user scope.
4. **Latest long-term memory (all sessions identity)**
   - recent cross-session developments (e.g. last 1-7 days).
5. **Derived memory bucket (auto-defined)**
   - distilled summaries/habits/errors/resolutions from neural-memory recalls.

## Token Policy (Cost-First)

This integration is cost-first, not context-maximizing.

### Absolute limits

- `hard_max_input_tokens`: `100000` (never exceed)
- `target_input_tokens_default`: `48000`
- `target_input_tokens_high_complexity`: `60000`
- `target_input_tokens_low_complexity`: `32000`
- `target_input_tokens_floor`: `16000`

### Complexity-aware target selection

Choose target band before assembly:

- **Low complexity** (short query, no tool traces, no code diff ask): target `~32k`
- **Normal complexity**: target `~48k`
- **High complexity** (multi-file refactor/debug planning): target `~60k`

If context can be reduced further without dropping required constraints/system instructions, keep shrinking toward floor.

### Budget slicing inside selected target

Instead of fixed large-context allocations, apply a tight budget split tuned for reduction:

- active/current prompt: `50%`
- recalled memory: `30%`
- summaries/distilled memory: `15%`
- safety buffer: `5%`

Within recalled-memory 30%:

- long-term: `55%`
- short-term: `45%`

Within each long/short bucket:

- latest: `65%`
- full history: `35%`

Rationale: prioritize compact high-signal memory blocks over raw long message windows.

Use your requested research baseline:

- top-level ratio: **60 : 20 : 10 : 10**
  - `active/current prompt`: 60%
  - `recalled memory`: 20%
  - `summaries`: 10%
  - `buffer`: 10%

Within recalled-memory 20%:

- **long-term 60%**
- **short-term 40%**

Within each long/short recalled bucket, split:

- `full` vs `latest` default: **40 : 60**
  - latest gets priority due to recency bias.

### Adaptive ratios by context window size (secondary)

Context-window scaling is secondary to cost cap. We still apply adaptive shaping, but **always inside the selected token target** above.

- **Small** (`<128k`): conservative
  - `active: 70%`, `recalled: 15%`, `summaries: 10%`, `buffer: 5%`
- **Medium** (`128k-400k`): balanced (default)
  - `active: 60%`, `recalled: 20%`, `summaries: 10%`, `buffer: 10%`
- **Large** (`400k-1M`): expansive
  - `active: 50%`, `recalled: 30%`, `summaries: 12%`, `buffer: 8%`
- **Ultra-large** (`>=1M`): multi-segment strategy
  - `active: 30%`, `recalled: 40%`, `summaries: 20%`, `buffer: 10%`

For `>=1M`, place segments to reduce lost-in-middle impact:

- start (0-10%): summaries + key decisions,
- early (10-30%): long-term recalled,
- late (70-90%): short-term recalled,
- end (90-100%): active current window.

### Effective breakdown (of message budget)

- Active current prompt: 60%
- Recalled long-term full: 4.8%
- Recalled long-term latest: 7.2%
- Recalled short-term full: 3.2%
- Recalled short-term latest: 4.8%
- Summaries/derived: 10%
- Buffer: 10%

## Dynamic Adjustment Rules

Conversation length aware overrides:

- `<100` session messages: short-term favored (`long:short = 40:60`)
- `100-500`: balanced (`60:40`)
- `>500`: long-term favored (`70:30`)

Model-aware budget:

- use `%` of model context window (no hard fixed tokens),
- default message budget starts from selected target, not full window percentage.

### Context window detection (hybrid)

Use a hybrid chain instead of hard-coded-only limits:

1. **Model registry** (primary, maintained by CLIProxy model metadata)
2. **API response hints/headers** when available (provider-specific)
3. **Conservative fallback** (`128k`) if unknown

Return source metadata (`registry|api|default`) for observability.

### Progressive resizing (do not wait until max)

Apply threshold-based resizing during conversation:

- `0-60% of target`: normal mode
- `60-75% of target`: background summarization prep
- `75-85% of target`: active compression (tier down oldest prompt blocks)
- `85-95% of target`: emergency archive/compress
- `95%+ of target`: hard protection (aggressive trim before forward)

Ratios should adapt by utilization (not only model size):

- at higher utilization, reduce active slice and increase recalled/summaries slices.

## Retrieval and Assembly Flow

1. Detect model context window using hybrid detector.
2. Select cost target (`32k/48k/60k`) by request complexity.
3. Clamp target by hard ceiling (`<=100k`) and floor.
4. Compute utilization against selected target and apply progressive actions.
5. Keep active prompt window under active budget slice.
6. Query neural-memory for recalled slices:
   - short-term by `session:{session_key}` tags + recency,
   - long-term by project/client tags + semantic query.
7. Build summary bucket from:
   - `nmem context --fresh-only` + scored condensation,
   - optional periodic distilled memories (`type=insight/decision`).
8. Merge buckets in anti-lost-in-middle order (or multi-segment for very large windows, still within target).
9. If still over target, run final compression pass (drop oldest low-score items, tighten summaries).
10. Inject compact memory block and forward.

## Prompt Replacement Strategy

Reducer should not blindly replace all messages.

### Default safe strategy

- Keep system prompt(s).
- Keep most recent N turns from current request.
- Replace dropped historical chunk with:
  - `memory summary block` (from bucket mix),
  - `key decisions/errors resolved` list,
  - optional `session recap` lines.

### Failure behavior

- If retrieval fails or times out: pass-through original payload.

## Config Additions (proposed)

```yaml
context-memory:
  enabled: true
  model-scope:
    default: "exclude"        # exclude | include
    include:
      - "claude-opus-*"
      - "claude-sonnet-*"
    exclude: []
  identity-normalizer:
    enabled: true
  capture:
    enabled: true
  reducer:
    enabled: true
    adaptive-mode: true
    token-target:
      hard-max-input: 100000
      default: 48000
      low-complexity: 32000
      high-complexity: 60000
      floor: 16000
    thresholds:
      comfortable: 0.60
      caution: 0.75
      warning: 0.85
      critical: 0.95
    ratios:
      active: 0.50
      recalled: 0.30
      summaries: 0.15
      buffer: 0.05
    ratios-by-window:
      small: { active: 0.70, recalled: 0.15, summaries: 0.10, buffer: 0.05 }
      medium: { active: 0.60, recalled: 0.20, summaries: 0.10, buffer: 0.10 }
      large: { active: 0.50, recalled: 0.30, summaries: 0.12, buffer: 0.08 }
      xlarge: { active: 0.30, recalled: 0.40, summaries: 0.20, buffer: 0.10 }
    recalled-split:
      longterm: 0.55
      shortterm: 0.45
    recency-split:
      full: 0.35
      latest: 0.65
    model-budget:
      message-context-ratio: 0.60
      context-window-fallback: 128000
      detect-source: "hybrid" # registry | api | hybrid
```

## Implementation Plan

1. **Capture path**
   - add capture writer (`capture.go`) that calls `nmem remember` with identity tags.
2. **Retriever path**
   - implement memory fetchers for each bucket (short/full/latest, long/full/latest).
3. **Assembler path**
   - token-budgeted merge using ratio matrix.
4. **Injector path**
   - replace old context chunk with compact memory block.
5. **Metrics**
   - track `tokens_before/after`, hit rates per bucket, failure fallback count.

6. **Online tuning test harness**
   - run canary at multiple targets (`32k`, `48k`, `60k`),
   - compare latency, error rate, and proxy-side quality heuristics,
   - auto-select lowest target that passes quality gates.

## Optimization Test Plan (What to Measure)

To determine the truly optimal context size per prompt in this stack:

1. Bucket traffic by request complexity (low/normal/high).
2. For each bucket, run controlled target sets:
   - `20k`, `32k`, `40k`, `48k`, `60k`, `80k` (never above `100k`).
3. Collect per-target metrics:
   - p50/p95 latency,
   - upstream 4xx/5xx,
   - retry/cooldown events,
   - answer-length and tool-call completion proxies,
   - manual spot-check quality sample.
4. Pick smallest target meeting quality SLOs.

Initial recommendation until tuning completes:

- default target `48k`
- high complexity `60k`
- low complexity `32k`
- emergency clamp `<=100k`

## Quality Scoring Harness (GLM-5)

To harden context-size optimization, add an automated judge stage using `glm-5` as evaluator.

### Objective

For each context-size target variant, run large-sample quality evaluation and choose the smallest size that preserves answer quality.

### Test matrix

Context-size variants:

- `20k`, `32k`, `40k`, `48k`, `60k`, `80k`, `100k`

Per-variant sample size:

- **1000 requests per variant** (same request distribution across variants)

### Method

1. Prepare fixed benchmark set of prompts (balanced by complexity/type).
2. For each prompt and each context-size variant:
   - run CLIProxy with that reducer target,
   - collect model output and metadata.
3. Send `(question, answer, optional reference)` to `glm-5` judge prompt.
4. Judge returns structured score JSON:
   - correctness (0-10)
   - completeness (0-10)
   - relevance (0-10)
   - hallucination risk (0-10 inverse)
   - overall quality (weighted)
5. Aggregate across 1000 requests per variant.

### Output analytics

For each context-size variant, compute:

- mean/median/p95 quality score,
- failure rate (quality below threshold),
- latency p50/p95,
- token-in/token-out and cost proxy,
- 4xx/5xx rate.

Selection rule:

- choose the **smallest context size** whose quality stays above gate (for example: overall >= 8.0 and failure rate <= 3%) and latency/error targets remain healthy.

### Guardrails

- Keep judge prompt fixed for all runs.
- Randomize request order to reduce temporal bias.
- Run at least 2 repetitions of the full matrix to confirm stability.

## Acceptance Criteria

- With feature ON, all eligible sessions are captured with stable `session_key` tags.
- Reducer applies ratio policy and lowers prompt tokens while preserving response quality.
- If neural-memory unavailable, request still succeeds unchanged.
- With default model-scope policy, feature is active only for Opus/Sonnet model families.
