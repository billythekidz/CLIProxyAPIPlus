# Gemini CLI OpenAI Integration Blueprint

This blueprint defines how to integrate `gemini-cli-openai` into `cli-proxy-api` using a submodule and sidecar architecture, with first-class multi-account support.

## 1) Integration Goal

- Add a new provider path backed by Gemini CLI OAuth credentials.
- Keep `cli-proxy-api` as the main router and policy engine.
- Run `gemini-cli-openai` as one or more sidecar instances (one account per instance).
- Reuse existing routing/cooldown behavior (`fill-first`, quota cooldown, retry).

## 2) Source of Truth (Submodule)

- Submodule path: `third_party/gemini-cli-openai`
- Upstream: `https://github.com/GewoonJaap/gemini-cli-openai`
- Policy: pin a specific commit hash in parent repo; update intentionally.

## 3) Runtime Architecture

- `cli-proxy-api` starts and monitors sidecar processes.
- Each sidecar exposes OpenAI-compatible endpoints on a unique local port.
- `cli-proxy-api` registers each sidecar as a routable auth/provider target.

Recommended default mode:

- `mode: sidecar` for controlled local orchestration.
- Optional `mode: external` to attach pre-existing sidecars.

## 4) Multi-Account Sidecar Model

Use one sidecar instance per OAuth account.

Benefits:

- account isolation (errors/limits do not contaminate others)
- independent health and cooldown states
- deterministic rotation and easier debugging

### Example config shape

```yaml
gemini-cli-openai:
  enabled: true
  mode: sidecar
  start-timeout: 45s
  healthcheck-interval: 20s
  restart-policy: on-failure
  max-restarts: 5
  cooldown-on-fail: 2m

  instances:
    - id: gcli-a1
      listen: 127.0.0.1:18787
      base-url: http://127.0.0.1:18787/v1
      creds-file: /home/theaux/.gemini/account-a1.json
      project-id: ""
      worker-api-key: ""
      disabled: false
      weight: 1

    - id: gcli-a2
      listen: 127.0.0.1:18788
      base-url: http://127.0.0.1:18788/v1
      creds-file: /home/theaux/.gemini/account-a2.json
      project-id: ""
      worker-api-key: ""
      disabled: false
      weight: 1

routing:
  strategy: fill-first

quota-exceeded:
  switch-project: true
```

## 5) Rotation Behavior

With `routing.strategy: fill-first`:

- prefer first healthy sidecar instance
- continue until cooldown/unavailable/near-quota policy triggers
- switch to next healthy instance

With pre-rotation enabled (optional extension):

- if remaining quota <= threshold (for example 10%), mark current instance cooldown and rotate

## 6) Sidecar Lifecycle Management

For each instance:

- start process with isolated env (`GCP_SERVICE_ACCOUNT`, optional project override, optional API key)
- wait for health (`GET /v1/models`)
- mark instance active only after health success
- on failure: restart per policy and enforce cooldown before rejoin

## 7) Security Requirements

- credentials files must be mode `600`
- never log access/refresh/id tokens
- bind sidecars to loopback (`127.0.0.1`) by default
- if non-loopback is used, require `worker-api-key`

## 8) Observability & Debugging

Add provider logs:

- `instance_id`, `account_id`, `model`, `status`, `latency_ms`, `retry_after`, `cooldown_reason`
- periodic summary every 5m per instance/model

Suggested debug endpoint:

- `/v1/debug/providers/gemini-cli-openai`
  - instance state, uptime, last health, restart count, last error

## 9) Failure Policy

- sidecar down -> mark unavailable and rotate
- repeated auth failures (401/403) -> suspend instance and raise warning
- rate limits (429/503) -> cooldown and rotate

## 10) Phased Delivery Plan

1. Config schema + instance parser.
2. Sidecar manager (start/stop/health/restart).
3. Provider registration per instance.
4. Rotation/cooldown wiring + logs.
5. Integration tests and runbook.
