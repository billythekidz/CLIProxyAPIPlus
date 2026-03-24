package contextmemory

import (
	"net/http"
	"testing"
)

func TestDetectContextWindow_Priority(t *testing.T) {
	h := http.Header{}
	h.Set("x-ratelimit-limit-tokens", "100000")
	res := DetectContextWindow(WindowDetectInput{RegistryWindow: 200000, Headers: h, FallbackWindow: 128000})
	if res.Source != ContextWindowSourceRegistry || res.ContextWindow != 200000 {
		t.Fatalf("registry should win, got %+v", res)
	}

	res = DetectContextWindow(WindowDetectInput{Headers: h, FallbackWindow: 128000})
	if res.Source != ContextWindowSourceAPI || res.ContextWindow != 80000 {
		t.Fatalf("api hint expected 80000, got %+v", res)
	}

	res = DetectContextWindow(WindowDetectInput{FallbackWindow: 64000})
	if res.Source != ContextWindowSourceDefault || res.ContextWindow != 64000 {
		t.Fatalf("default fallback expected, got %+v", res)
	}
}

func TestSelectTokenTarget_Clamps(t *testing.T) {
	cfg := TokenTargetConfig{HardMax: 50000, DefaultTarget: 48000, LowTarget: 12000, HighTarget: 70000, Floor: 16000}
	if got := SelectTokenTarget(cfg, ComplexityLow); got != 16000 {
		t.Fatalf("low target clamp got %d want 16000", got)
	}
	if got := SelectTokenTarget(cfg, ComplexityHigh); got != 50000 {
		t.Fatalf("high target clamp got %d want 50000", got)
	}
	if got := SelectTokenTarget(cfg, ComplexityNormal); got != 48000 {
		t.Fatalf("normal target got %d want 48000", got)
	}
}

func TestApplyUtilizationAdjustment(t *testing.T) {
	base := RatioSet{Active: 0.60, Recalled: 0.20, Summaries: 0.10, Buffer: 0.10}
	r := ApplyUtilizationAdjustment(base, 0.8)
	if r.Active >= base.Active {
		t.Fatalf("active should shrink at high utilization")
	}
	if r.Recalled <= base.Recalled {
		t.Fatalf("recalled should increase at high utilization")
	}
	sum := r.Active + r.Recalled + r.Summaries + r.Buffer
	if sum < 0.999 || sum > 1.001 {
		t.Fatalf("ratios should normalize to 1, got %f", sum)
	}
}
