package contextmemory

import (
	"net/http"
	"strconv"
	"strings"
)

type ContextWindowSource string

const (
	ContextWindowSourceRegistry ContextWindowSource = "registry"
	ContextWindowSourceAPI      ContextWindowSource = "api"
	ContextWindowSourceDefault  ContextWindowSource = "default"
)

type WindowDetectInput struct {
	Model          string
	RegistryWindow int
	Headers        http.Header
	FallbackWindow int
}

type WindowDetectResult struct {
	ContextWindow int
	Source        ContextWindowSource
}

func DetectContextWindow(in WindowDetectInput) WindowDetectResult {
	fallback := in.FallbackWindow
	if fallback <= 0 {
		fallback = 128000
	}
	if in.RegistryWindow > 0 {
		return WindowDetectResult{ContextWindow: in.RegistryWindow, Source: ContextWindowSourceRegistry}
	}
	if hinted := detectWindowFromHeaders(in.Headers); hinted > 0 {
		return WindowDetectResult{ContextWindow: hinted, Source: ContextWindowSourceAPI}
	}
	return WindowDetectResult{ContextWindow: fallback, Source: ContextWindowSourceDefault}
}

func detectWindowFromHeaders(headers http.Header) int {
	if headers == nil {
		return 0
	}
	tryParse := func(key string) int {
		raw := strings.TrimSpace(headers.Get(key))
		if raw == "" {
			return 0
		}
		if idx := strings.Index(raw, ","); idx > 0 {
			raw = strings.TrimSpace(raw[:idx])
		}
		v, err := strconv.Atoi(raw)
		if err != nil || v <= 0 {
			return 0
		}
		return v
	}
	if v := tryParse("x-context-window"); v > 0 {
		return v
	}
	if v := tryParse("x-ratelimit-tokens-limit"); v > 0 {
		return v
	}
	if v := tryParse("x-ratelimit-limit-tokens"); v > 0 {
		// OpenAI style limits may include output budget; keep conservative input slice.
		return int(float64(v) * 0.8)
	}
	return 0
}

type Complexity string

const (
	ComplexityLow    Complexity = "low"
	ComplexityNormal Complexity = "normal"
	ComplexityHigh   Complexity = "high"
)

type TokenTargetConfig struct {
	HardMax       int
	DefaultTarget int
	LowTarget     int
	HighTarget    int
	Floor         int
}

func DefaultTokenTargetConfig() TokenTargetConfig {
	return TokenTargetConfig{
		HardMax:       100000,
		DefaultTarget: 48000,
		LowTarget:     32000,
		HighTarget:    60000,
		Floor:         16000,
	}
}

func SelectTokenTarget(cfg TokenTargetConfig, complexity Complexity) int {
	if cfg.HardMax <= 0 {
		cfg.HardMax = 100000
	}
	if cfg.Floor <= 0 {
		cfg.Floor = 16000
	}
	if cfg.DefaultTarget <= 0 {
		cfg.DefaultTarget = 48000
	}
	if cfg.LowTarget <= 0 {
		cfg.LowTarget = 32000
	}
	if cfg.HighTarget <= 0 {
		cfg.HighTarget = 60000
	}

	target := cfg.DefaultTarget
	switch complexity {
	case ComplexityLow:
		target = cfg.LowTarget
	case ComplexityHigh:
		target = cfg.HighTarget
	}
	if target < cfg.Floor {
		target = cfg.Floor
	}
	if target > cfg.HardMax {
		target = cfg.HardMax
	}
	return target
}

type RatioSet struct {
	Active    float64
	Recalled  float64
	Summaries float64
	Buffer    float64
}

func (r RatioSet) Normalize() RatioSet {
	total := r.Active + r.Recalled + r.Summaries + r.Buffer
	if total <= 0 {
		return RatioSet{Active: 0.5, Recalled: 0.3, Summaries: 0.15, Buffer: 0.05}
	}
	return RatioSet{
		Active:    r.Active / total,
		Recalled:  r.Recalled / total,
		Summaries: r.Summaries / total,
		Buffer:    r.Buffer / total,
	}
}

func BaseRatiosByWindow(contextWindow int) RatioSet {
	switch {
	case contextWindow >= 1000000:
		return RatioSet{Active: 0.30, Recalled: 0.40, Summaries: 0.20, Buffer: 0.10}
	case contextWindow >= 400000:
		return RatioSet{Active: 0.50, Recalled: 0.30, Summaries: 0.12, Buffer: 0.08}
	case contextWindow >= 128000:
		return RatioSet{Active: 0.60, Recalled: 0.20, Summaries: 0.10, Buffer: 0.10}
	default:
		return RatioSet{Active: 0.70, Recalled: 0.15, Summaries: 0.10, Buffer: 0.05}
	}
}

func Utilization(currentTokens, targetTokens int) float64 {
	if targetTokens <= 0 {
		return 1
	}
	if currentTokens <= 0 {
		return 0
	}
	return float64(currentTokens) / float64(targetTokens)
}

func ApplyUtilizationAdjustment(base RatioSet, utilization float64) RatioSet {
	r := base.Normalize()
	switch {
	case utilization < 0.60:
		return r
	case utilization < 0.75:
		r.Active -= 0.10
		r.Recalled += 0.05
		r.Summaries += 0.05
	case utilization < 0.90:
		r.Active -= 0.20
		r.Recalled += 0.10
		r.Summaries += 0.10
	default:
		r.Active = 0.30
		r.Recalled = 0.40
		r.Summaries = 0.25
		r.Buffer = 0.05
	}
	if r.Active < 0.20 {
		r.Active = 0.20
	}
	if r.Buffer < 0.05 {
		r.Buffer = 0.05
	}
	return r.Normalize()
}
