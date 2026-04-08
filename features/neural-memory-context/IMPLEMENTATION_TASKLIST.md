# Neural Memory Context Integration - Task Tracker

Status legend:

- `[ ]` pending
- `[-]` in progress
- `[x]` done

## Milestone A - Foundation

- [x] A1. Add `context-memory` config schema + defaults + validation
- [x] A2. Add model-scope matcher (`enabled`, `default`, `include`, `exclude`)
- [x] A3. Wire runtime toggle check (`IsContextMemoryEnabledForModel`)

## Milestone B - Identity + Capture

- [x] B1. Implement identity normalizer module (`session_key`, `thread_key`, `client_family`)
- [x] B2. Add TTL cache for normalized session state
- [x] B3. Implement full prompt capture writer via `nmem remember`
- [x] B4. Add redaction + payload size cap + fail-open capture behavior

## Milestone C - Budget + Reduction Engine

- [x] C1. Implement hybrid context-window detector (registry -> API hints -> fallback)
- [x] C2. Implement cost-first token target selector (`32k/48k/60k`, floor `16k`, hard max `100k`)
- [x] C3. Implement progressive thresholds (`60/75/85/95`) and adaptive ratio shifts
- [x] C4. Implement final compression pass to enforce selected target

## Milestone D - Memory Retrieval + Assembly

- [x] D1. Implement retrieval buckets (full/latest short-term + full/latest long-term + derived summaries)
- [x] D2. Implement bucket scoring and token-budget allocator
- [x] D3. Implement prompt assembler + injector (preserve system + latest turns)
- [x] D4. Implement optional multi-segment ordering for very large context windows

## Milestone E - Pipeline + Observability

- [x] E1. Integrate module into pre-executor pipeline
- [x] E2. Add structured logs (`session_key`, `model_scope_hit`, `tokens_before/after`)
- [x] E3. Add counters/metrics (capture/reducer success, fallback, errors)

## Milestone F - Testing

- [ ] F1. Unit tests: matcher, identity precedence, budget math, ratio math
- [ ] F2. Unit tests: redaction, truncation, fail-open behavior
- [ ] F3. Integration tests: capture + reduction for openai/anthropic routes
- [ ] F4. Regression tests: feature OFF behavior unchanged

## Milestone G - Canary + Tuning

- [ ] G1. Run canary targets (`20k/32k/40k/48k/60k/80k`)
- [ ] G2. Compare latency/error/quality proxy metrics per target
- [ ] G3. Select smallest stable default targets by complexity bucket
- [ ] G4. Update config defaults and docs from measured results

## Milestone H - GLM-5 Harden Benchmark

- [ ] H1. Build fixed benchmark prompt set (stratified by task complexity)
- [ ] H2. Implement variant runner for context targets (`20k/32k/40k/48k/60k/80k/100k`)
- [ ] H3. Run **1000 requests per variant** through CLIProxy
- [ ] H4. Implement `glm-5` judge prompt + structured scoring parser
- [ ] H5. Produce per-variant analytics (quality/latency/errors/cost proxy)
- [ ] H6. Select best target by smallest-size-pass rule and document decision

## Current Next Steps

- [x] Start Milestone A (A1 -> A2 -> A3)
- [ ] Commit in small slices per milestone
