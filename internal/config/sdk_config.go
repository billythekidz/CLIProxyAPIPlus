// Package config provides configuration management for the CLI Proxy API server.
// It handles loading and parsing YAML configuration files, and provides structured
// access to application settings including server port, authentication directory,
// debug settings, proxy configuration, and API keys.
package config

// SDKConfig represents the application's configuration, loaded from a YAML file.
type SDKConfig struct {
	// ProxyURL is the URL of an optional proxy server to use for outbound requests.
	ProxyURL string `yaml:"proxy-url" json:"proxy-url"`

	// EnableGeminiCLIEndpoint controls whether Gemini CLI internal endpoints (/v1internal:*) are enabled.
	// Default is false for safety; when false, /v1internal:* requests are rejected.
	EnableGeminiCLIEndpoint bool `yaml:"enable-gemini-cli-endpoint" json:"enable-gemini-cli-endpoint"`

	// ForceModelPrefix requires explicit model prefixes (e.g., "teamA/gemini-3-pro-preview")
	// to target prefixed credentials. When false, unprefixed model requests may use prefixed
	// credentials as well.
	ForceModelPrefix bool `yaml:"force-model-prefix" json:"force-model-prefix"`

	// RequestLog enables or disables detailed request logging functionality.
	RequestLog bool `yaml:"request-log" json:"request-log"`

	// APIKeys is a list of keys for authenticating clients to this proxy server.
	APIKeys []string `yaml:"api-keys" json:"api-keys"`

	// PassthroughHeaders controls whether upstream response headers are forwarded to downstream clients.
	// Default is false (disabled).
	PassthroughHeaders bool `yaml:"passthrough-headers" json:"passthrough-headers"`

	// Streaming configures server-side streaming behavior (keep-alives and safe bootstrap retries).
	Streaming StreamingConfig `yaml:"streaming" json:"streaming"`

	// NonStreamKeepAliveInterval controls how often blank lines are emitted for non-streaming responses.
	// <= 0 disables keep-alives. Value is in seconds.
	NonStreamKeepAliveInterval int `yaml:"nonstream-keepalive-interval,omitempty" json:"nonstream-keepalive-interval,omitempty"`

	// MattermostThreadMode describes how Mattermost requests are mapped to Perplexity threads.
	// "all user 1 thread" or "per user per thread"
	MattermostThreadMode string `yaml:"mattermost-thread-mode" json:"mattermost-thread-mode"`

	// LlmGateToolQuality enables LLM Gate Tool Quality for internal logic.
	LlmGateToolQuality bool `yaml:"llm-gate-tool-quality" json:"llm-gate-tool-quality"`

	// Enhance service configuration
	Enhance EnhanceConfig `yaml:"enhance" json:"enhance"`
}

// StreamingConfig holds server streaming behavior configuration.
type StreamingConfig struct {
	// KeepAliveSeconds controls how often the server emits SSE heartbeats (": keep-alive\n\n").
	// <= 0 disables keep-alives. Default is 0.
	KeepAliveSeconds int `yaml:"keepalive-seconds,omitempty" json:"keepalive-seconds,omitempty"`

	// BootstrapRetries controls how many times the server may retry a streaming request before any bytes are sent,
	// to allow auth rotation / transient recovery.
	// <= 0 disables bootstrap retries. Default is 0.
	BootstrapRetries int `yaml:"bootstrap-retries,omitempty" json:"bootstrap-retries,omitempty"`
}

// EnhanceConfig configures dynamic model postfix routing to the external enhance service.
type EnhanceConfig struct {
	// Enabled toggles the dynamic postfix routing feature.
	Enabled bool `yaml:"enabled" json:"enabled"`
	
	// RouteAll when true, applies enhance routing unconditionally to all models and endpoints.
	RouteAll bool `yaml:"route-all" json:"route-all"`
	
	// Postfix is the suffix applied to requested model names (e.g., "-enhance"). Default: "-enhance"
	Postfix string `yaml:"postfix" json:"postfix"`
	
	// Endpoint is the full URL of the enhance service endpoint (e.g., "http://vps_proxy_model_enhance:8318/v1/chat/completions").
	Endpoint string `yaml:"endpoint" json:"endpoint"`
}
