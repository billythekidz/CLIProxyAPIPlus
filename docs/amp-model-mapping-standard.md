# Amp Model Mapping Standard (ProxyPal Baseline)

This document defines the recommended `ampcode.model-mappings` baseline for CLIProxyAPIPlus.

The baseline follows ProxyPal's current Amp slot model IDs and keeps mappings explicit by role.

Validation source in this update: ProxyPal `main` (fetched from `origin/main` on 2026-02-22, HEAD `512d11b`).

## ProxyPal mapping model (how it works)

ProxyPal does not treat Amp mapping as a flat list only. It uses a role-slot system plus custom overrides:

- Role slots are predefined in `AMP_MODEL_SLOTS` (Smart, Rush/Titling, Deep, Oracle, Librarian, Search, Review, Handoff, Topics, Painter).
- Each slot has a fixed `fromModel` (the exact model ID Amp emits for that role).
- Each slot can be enabled/disabled independently.
- Each slot can map to any available target model (`to`/`alias`).
- Optional `fork` per slot sends to both original and mapped model.
- Custom mappings can be added for non-slot models.
- Mapping priority is controlled by `force-model-mappings`.
- Slot migration logic updates old slot IDs when Amp changes naming.

Reference implementation in ProxyPal:

- `src/lib/tauri/models.ts` (`AMP_MODEL_SLOTS`, `AMP_MODEL_ALIASES`, `SLOT_MODEL_MIGRATIONS`)
- `src/components/settings/AmpSettings.tsx` (slot UI, per-slot enable/fork/reasoning, custom mappings)
- `src-tauri/src/types/amp.rs` (`enabled`, `fork` fields)
- `src-tauri/src/commands/proxy.rs` (only `enabled=true` mappings are rendered into `ampcode.model-mappings`)

## Mapping semantics in CLIProxyAPIPlus

CLIProxyAPIPlus uses `ampcode.model-mappings` entries in `config.yaml`:

- `from` = exact incoming Amp model ID
- `to` = local target model
- `fork: true` is supported per mapping when needed
- there is no per-entry `enabled` flag in YAML; enable by keeping entry, disable by removing entry
- `force-model-mappings: true` is required for strict local routing (no upstream fallback)

ProxyPal-specific behavior worth mirroring:

- ProxyPal keeps `enabled` and `fork` in app state, but writes only enabled mappings into generated YAML.
- ProxyPal includes slot migration logic (`SLOT_MODEL_MIGRATIONS`) so older `from` model IDs can auto-upgrade.
- ProxyPal also maintains an alias-normalization table (`AMP_MODEL_ALIASES`) for variant model IDs.

## Why this baseline

- Amp model IDs must match exactly (`from` is exact-match).
- Amp slots can change over time; this baseline tracks known active slot IDs.
- `force-model-mappings: true` is recommended when you do not want upstream Amp fallback.

## Recommended config block

```yaml
ampcode:
  force-model-mappings: true
  model-mappings:
    # Baseline Amp slots (ProxyPal standard)
    - from: "claude-opus-4-6"                        # Smart
      to: "gpt-5.3-codex"
    - from: "claude-haiku-4-5-20251001"             # Rush/Titling
      to: "claude-haiku-4.5"
    - from: "gpt-5.2-codex"                         # Deep
      to: "gpt-5.3-codex"
    - from: "gpt-5.2"                               # Oracle
      to: "gpt-5.2"
    - from: "claude-sonnet-4-5-20241022"            # Librarian
      to: "claude-sonnet-4.5"

    # Gemini-role slots -> gemini-3-flash-preview
    - from: "gemini-3-flash-preview"                # Search
      to: "gemini-3-flash-preview"
    - from: "gemini-3-pro-preview"                  # Review
      to: "gemini-3-flash-preview"
    - from: "gemini-2.5-flash"                      # Handoff
      to: "gemini-3-flash-preview"
    - from: "gemini-2.5-flash-lite-preview-09-2025" # Topics
      to: "gemini-3-flash-preview"
    - from: "gemini-3-pro-image-preview"            # Painter
      to: "gemini-3-flash-preview"
```

## Role table (ProxyPal slot baseline)

| Role | Amp `from` model ID | Suggested `to` model |
|---|---|---|
| Smart | `claude-opus-4-6` | `gpt-5.3-codex` |
| Rush/Titling | `claude-haiku-4-5-20251001` | `claude-haiku-4.5` |
| Deep | `gpt-5.2-codex` | `gpt-5.3-codex` |
| Oracle | `gpt-5.2` | `gpt-5.2` |
| Librarian | `claude-sonnet-4-5-20241022` | `claude-sonnet-4.5` |
| Search | `gemini-3-flash-preview` | `gemini-3-flash-preview` |
| Review | `gemini-3-pro-preview` | `gemini-3-flash-preview` |
| Handoff | `gemini-2.5-flash` | `gemini-3-flash-preview` |
| Topics | `gemini-2.5-flash-lite-preview-09-2025` | `gemini-3-flash-preview` |
| Painter | `gemini-3-pro-image-preview` | `gemini-3-flash-preview` |

## Compatibility aliases (recommended)

Amp can emit variant IDs across versions. Add compatibility entries if your logs show them.

```yaml
ampcode:
  model-mappings:
    # Opus variants
    - from: "claude-opus-4-6"
      to: "gpt-5.3-codex"
    - from: "claude-opus-4-6-20260205"
      to: "gpt-5.3-codex"
    - from: "claude-opus-4.6"
      to: "gpt-5.3-codex"
    - from: "claude-opus-4-5-20251101"
      to: "gpt-5.3-codex"
    - from: "claude-opus-4.5"
      to: "gpt-5.3-codex"

    # Sonnet variants
    - from: "claude-sonnet-4-5-20241022"
      to: "claude-sonnet-4.5"
    - from: "claude-sonnet-4-5-20250929"
      to: "claude-sonnet-4.5"
    - from: "claude-sonnet-4.5"
      to: "claude-sonnet-4.5"

    # Haiku variants
    - from: "claude-haiku-4-5-20251001"
      to: "claude-haiku-4.5"
    - from: "claude-haiku-4.5"
      to: "claude-haiku-4.5"

    # GPT variants
    - from: "gpt-5.2"
      to: "gpt-5.2"
    - from: "gpt-5-2"
      to: "gpt-5.2"
    - from: "gpt-5.2-codex"
      to: "gpt-5.3-codex"
    - from: "gpt-5-2-codex"
      to: "gpt-5.3-codex"
```

## Operational notes

- Ensure every `to` model exists in your `GET /v1/models` output.
- If a slot ID changes in Amp, add/adjust `from` immediately.
- Keep `force-model-mappings: true` when running local-only routing.
- After editing `config.yaml`, restart CLIProxyAPIPlus.
- If you need per-role on/off behavior like ProxyPal `enabled`, manage it by adding/removing that role's mapping entry.
- If you need dual-run for debugging/canary, add `fork: true` on selected role entries only.

## Reference source

ProxyPal baseline taken from:

- `src/lib/tauri/models.ts` (`AMP_MODEL_SLOTS`, `AMP_MODEL_ALIASES`, `SLOT_MODEL_MIGRATIONS`)
- `src/components/settings/AmpSettings.tsx` (slot-level mapping UI and custom mappings)
- `src-tauri/src/types/amp.rs` (`AmpModelMapping`: `enabled`, `fork`)
- `src-tauri/src/commands/proxy.rs` (`build_amp_model_mappings_section`)
