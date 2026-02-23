package config

import (
	"testing"

	"gopkg.in/yaml.v3"
)

func TestGeminiCLIOpenAIConfigParsing(t *testing.T) {
	raw := `
gemini-cli-openai:
  enabled: true
  mode: sidecar
  start-timeout: 30s
  healthcheck-interval: 15s
  restart-policy: on-failure
  max-restarts: 3
  cooldown-on-fail: 1m
  instances:
    - id: gcli-a1
      listen: 127.0.0.1:18787
      base-url: http://127.0.0.1:18787/v1
      creds-file: /tmp/creds1.json
      project-id: proj1
      worker-api-key: sk-test1
      weight: 2
    - id: gcli-a2
      listen: 127.0.0.1:18788
      base-url: http://127.0.0.1:18788/v1
      creds-file: /tmp/creds2.json
      disabled: true
      weight: 1
`
	var cfg Config
	if err := yaml.Unmarshal([]byte(raw), &cfg); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if !cfg.GeminiCLIOpenAI.Enabled {
		t.Fatal("expected enabled=true")
	}
	if cfg.GeminiCLIOpenAI.Mode != "sidecar" {
		t.Fatalf("expected mode=sidecar, got %s", cfg.GeminiCLIOpenAI.Mode)
	}
	if cfg.GeminiCLIOpenAI.MaxRestarts != 3 {
		t.Fatalf("expected max-restarts=3, got %d", cfg.GeminiCLIOpenAI.MaxRestarts)
	}
	if len(cfg.GeminiCLIOpenAI.Instances) != 2 {
		t.Fatalf("expected 2 instances, got %d", len(cfg.GeminiCLIOpenAI.Instances))
	}

	inst0 := cfg.GeminiCLIOpenAI.Instances[0]
	if inst0.ID != "gcli-a1" || inst0.BaseURL != "http://127.0.0.1:18787/v1" {
		t.Fatalf("inst0 mismatch: %+v", inst0)
	}
	if inst0.Weight != 2 {
		t.Fatalf("expected weight=2, got %d", inst0.Weight)
	}

	inst1 := cfg.GeminiCLIOpenAI.Instances[1]
	if !inst1.Disabled {
		t.Fatal("expected inst1 disabled=true")
	}
}

func TestSanitizeGeminiCLIOpenAI(t *testing.T) {
	cfg := &Config{}
	cfg.GeminiCLIOpenAI.Instances = []GeminiCLIOpenAIInstance{
		{ID: "  valid  ", BaseURL: " http://localhost:8787/v1 ", Weight: 0},
		{ID: "", BaseURL: "http://localhost:8788/v1"}, // should be dropped (no ID)
		{ID: "nourl", BaseURL: ""},                    // should be dropped (no URL)
	}
	cfg.SanitizeGeminiCLIOpenAI()

	if cfg.GeminiCLIOpenAI.Mode != "sidecar" {
		t.Fatalf("expected default mode=sidecar, got %s", cfg.GeminiCLIOpenAI.Mode)
	}
	if cfg.GeminiCLIOpenAI.RestartPolicy != "on-failure" {
		t.Fatalf("expected default restart-policy=on-failure, got %s", cfg.GeminiCLIOpenAI.RestartPolicy)
	}
	if cfg.GeminiCLIOpenAI.MaxRestarts != 5 {
		t.Fatalf("expected default max-restarts=5, got %d", cfg.GeminiCLIOpenAI.MaxRestarts)
	}
	if len(cfg.GeminiCLIOpenAI.Instances) != 1 {
		t.Fatalf("expected 1 valid instance, got %d", len(cfg.GeminiCLIOpenAI.Instances))
	}
	if cfg.GeminiCLIOpenAI.Instances[0].ID != "valid" {
		t.Fatalf("expected trimmed ID 'valid', got '%s'", cfg.GeminiCLIOpenAI.Instances[0].ID)
	}
	if cfg.GeminiCLIOpenAI.Instances[0].Weight != 1 {
		t.Fatalf("expected default weight=1, got %d", cfg.GeminiCLIOpenAI.Instances[0].Weight)
	}
}
