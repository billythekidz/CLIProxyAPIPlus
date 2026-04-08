package config

import (
	"os"
	"strings"
)

// WebSearchConfig controls the universal proxy-managed web search feature.
// When enabled, the proxy intercepts web_search tool calls and executes them
// via SearXNG instead of relying on provider-native built-in search.
type WebSearchConfig struct {
	// Enabled globally toggles the universal web search feature.
	Enabled bool `yaml:"enabled" json:"enabled"`

	// SearXNGURL is the base URL of the SearXNG instance (e.g. "http://searxng:8080").
	SearXNGURL string `yaml:"searxng-url" json:"searxng-url"`

	// SearXNGTimeout is the HTTP timeout for SearXNG requests (default "15s").
	SearXNGTimeout string `yaml:"searxng-timeout" json:"searxng-timeout"`

	// MaxResults caps the number of search results returned (default 10).
	MaxResults int `yaml:"max-results" json:"max-results"`

	// OverrideBuiltin when true strips provider-native web_search tools from
	// requests and routes them through the universal proxy-managed path instead.
	OverrideBuiltin bool `yaml:"override-builtin" json:"override-builtin"`

	// PerplexityEnabled toggles optional Perplexity Sonar Pro summary enrichment.
	PerplexityEnabled bool `yaml:"perplexity-enabled" json:"perplexity-enabled"`

	// PerplexityAPIKey is the API key for Perplexity. Required when perplexity-enabled is true.
	PerplexityAPIKey string `yaml:"perplexity-api-key" json:"perplexity-api-key,omitempty"`

	// PerplexityBaseURL is the Perplexity API endpoint (default "https://api.perplexity.ai").
	PerplexityBaseURL string `yaml:"perplexity-base-url" json:"perplexity-base-url"`

	// ModelScope controls per-model activation.
	ModelScope WebSearchModelScope `yaml:"model-scope" json:"model-scope"`
}

// WebSearchModelScope controls model include/exclude matching for web search.
type WebSearchModelScope struct {
	// Default determines baseline behavior for models not matched by include/exclude.
	// Supported values: "include" (default for web search), "exclude".
	Default string `yaml:"default" json:"default"`

	// Include model wildcard patterns to activate when default=exclude.
	Include []string `yaml:"include" json:"include"`

	// Exclude model wildcard patterns to disable regardless of include/default.
	Exclude []string `yaml:"exclude" json:"exclude"`
}

// SanitizeWebSearch normalizes web search feature settings and applies
// environment variable overrides.
func (cfg *Config) SanitizeWebSearch() {
	if cfg == nil {
		return
	}
	ws := &cfg.WebSearch

	// Apply environment variable overrides
	if v := os.Getenv("WEBSEARCH_OVERRIDE_BUILTIN"); v != "" {
		ws.OverrideBuiltin = strings.EqualFold(v, "true") || v == "1"
	}
	if v := os.Getenv("WEBSEARCH_SEARXNG_URL"); v != "" {
		ws.SearXNGURL = strings.TrimSpace(v)
	}

	// Normalize URLs
	ws.SearXNGURL = strings.TrimSpace(ws.SearXNGURL)
	ws.PerplexityAPIKey = strings.TrimSpace(ws.PerplexityAPIKey)
	ws.PerplexityBaseURL = strings.TrimSpace(ws.PerplexityBaseURL)

	// Apply defaults
	if ws.SearXNGTimeout == "" {
		ws.SearXNGTimeout = "15s"
	}
	if ws.MaxResults <= 0 {
		ws.MaxResults = 10
	}
	if ws.PerplexityBaseURL == "" {
		ws.PerplexityBaseURL = "https://api.perplexity.ai"
	}

	// Normalize model scope
	scope := &ws.ModelScope
	def := strings.ToLower(strings.TrimSpace(scope.Default))
	if def != "include" && def != "exclude" {
		def = "include" // default for web search: include all models
	}
	scope.Default = def
	scope.Include = sanitizeWebSearchPatterns(scope.Include)
	scope.Exclude = sanitizeWebSearchPatterns(scope.Exclude)

	// Auto-disable Perplexity when no API key
	if ws.PerplexityEnabled && ws.PerplexityAPIKey == "" {
		ws.PerplexityEnabled = false
	}
}

// IsWebSearchEnabledForModel reports whether universal web search should be active
// for a model according to global toggle and model-scope include/exclude rules.
func (cfg *Config) IsWebSearchEnabledForModel(model string) bool {
	if cfg == nil {
		return false
	}
	if !cfg.WebSearch.Enabled {
		return false
	}
	model = strings.TrimSpace(model)
	if model == "" {
		return false
	}
	for i := range cfg.WebSearch.ModelScope.Exclude {
		if matchSimpleWildcard(cfg.WebSearch.ModelScope.Exclude[i], model) {
			return false
		}
	}
	for i := range cfg.WebSearch.ModelScope.Include {
		if matchSimpleWildcard(cfg.WebSearch.ModelScope.Include[i], model) {
			return true
		}
	}
	return cfg.WebSearch.ModelScope.Default == "include"
}

func sanitizeWebSearchPatterns(patterns []string) []string {
	if len(patterns) == 0 {
		return []string{}
	}
	out := make([]string, 0, len(patterns))
	seen := make(map[string]struct{}, len(patterns))
	for i := range patterns {
		pattern := strings.ToLower(strings.TrimSpace(patterns[i]))
		if pattern == "" {
			continue
		}
		if _, ok := seen[pattern]; ok {
			continue
		}
		seen[pattern] = struct{}{}
		out = append(out, pattern)
	}
	return out
}
