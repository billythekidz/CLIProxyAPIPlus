package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfig_ContextMemoryDefaults(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte("{}\n"), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}

	if !cfg.ContextMemory.Enabled {
		t.Fatalf("ContextMemory.Enabled = false, want true")
	}
	if cfg.ContextMemory.ModelScope.Default != "exclude" {
		t.Fatalf("ModelScope.Default = %q, want %q", cfg.ContextMemory.ModelScope.Default, "exclude")
	}
	if len(cfg.ContextMemory.ModelScope.Include) != 2 {
		t.Fatalf("include patterns = %d, want 2", len(cfg.ContextMemory.ModelScope.Include))
	}
	if !cfg.IsContextMemoryEnabledForModel("claude-opus-4.6") {
		t.Fatalf("expected Opus family enabled by default")
	}
	if !cfg.IsContextMemoryEnabledForModel("claude-sonnet-4.6") {
		t.Fatalf("expected Sonnet family enabled by default")
	}
	if cfg.IsContextMemoryEnabledForModel("gpt-5.3-codex") {
		t.Fatalf("expected non-Opus/Sonnet model disabled by default")
	}
}

func TestContextMemoryModelScope_PrecedenceAndMatching(t *testing.T) {
	t.Parallel()

	cfg := &Config{}
	cfg.ContextMemory.Enabled = true
	cfg.ContextMemory.ModelScope.Default = "exclude"
	cfg.ContextMemory.ModelScope.Include = []string{"claude-*", " CLAUDE-* ", ""}
	cfg.ContextMemory.ModelScope.Exclude = []string{"claude-opus-*", "claude-opus-*", "  "}
	cfg.SanitizeContextMemory()

	if got := len(cfg.ContextMemory.ModelScope.Include); got != 1 {
		t.Fatalf("sanitized include len = %d, want 1", got)
	}
	if got := len(cfg.ContextMemory.ModelScope.Exclude); got != 1 {
		t.Fatalf("sanitized exclude len = %d, want 1", got)
	}

	// Exclude should win even if include matches.
	if cfg.IsContextMemoryEnabledForModel("claude-opus-4.6") {
		t.Fatalf("exclude should override include")
	}
	// Include should enable when not excluded.
	if !cfg.IsContextMemoryEnabledForModel("claude-sonnet-4.6") {
		t.Fatalf("include should enable claude-sonnet-4.6")
	}
	// Default exclude should keep unrelated models disabled.
	if cfg.IsContextMemoryEnabledForModel("gpt-5.3-codex") {
		t.Fatalf("default exclude should disable unrelated models")
	}
}

func TestContextMemoryModelScope_GlobalDisable(t *testing.T) {
	t.Parallel()

	cfg := &Config{}
	cfg.ContextMemory.Enabled = false
	cfg.ContextMemory.ModelScope.Default = "include"
	cfg.ContextMemory.ModelScope.Include = []string{"*"}
	cfg.SanitizeContextMemory()

	if cfg.IsContextMemoryEnabledForModel("claude-opus-4.6") {
		t.Fatalf("global disable should force false")
	}
}
