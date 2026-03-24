package contextmemory

import (
	"context"
	"encoding/json"
	"errors"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

type CaptureConfig struct {
	Enabled           bool
	WriteEnabled      bool
	Command           string
	TimeoutMS         int
	MaxBytesPerRecord int
	RedactEnabled     bool
	RedactPatterns    []string
	OnError           string
}

type CaptureRequest struct {
	SessionKey     string
	ThreadKey      string
	ClientFamily   string
	Protocol       string
	Path           string
	ModelRequested string
	ModelRouted    string
	APIKeyHash     string
	SourceIP       string
	UserAgent      string
	PromptFull     []byte
}

type commandRunner func(ctx context.Context, name string, args ...string) ([]byte, error)

type CaptureWriter struct {
	cfg CaptureConfig
	run commandRunner
}

func NewCaptureWriter(cfg CaptureConfig) *CaptureWriter {
	if strings.TrimSpace(cfg.Command) == "" {
		cfg.Command = "nmem"
	}
	if cfg.TimeoutMS <= 0 {
		cfg.TimeoutMS = 500
	}
	if cfg.MaxBytesPerRecord <= 0 {
		cfg.MaxBytesPerRecord = 256 * 1024
	}
	if strings.TrimSpace(cfg.OnError) == "" {
		cfg.OnError = "skip"
	}
	return &CaptureWriter{cfg: cfg, run: defaultRunner}
}

func (w *CaptureWriter) SetRunnerForTest(r commandRunner) {
	if r != nil {
		w.run = r
	}
}

func (w *CaptureWriter) Capture(ctx context.Context, req CaptureRequest) error {
	if w == nil || !w.cfg.Enabled || !w.cfg.WriteEnabled {
		return nil
	}
	content, err := buildCaptureContent(w.cfg, req)
	if err != nil {
		if strings.EqualFold(w.cfg.OnError, "block") {
			return err
		}
		return nil
	}
	args := []string{
		"remember", content,
		"--type", "context",
		"--priority", "6",
		"--expires", "7",
		"--json",
		"--tag", "scope:full_prompt",
		"--tag", "scope:shortterm",
	}
	tags := buildTags(req)
	for i := range tags {
		args = append(args, "--tag", tags[i])
	}
	timeout := time.Duration(w.cfg.TimeoutMS) * time.Millisecond
	callCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	_, err = w.run(callCtx, w.cfg.Command, args...)
	if err != nil && strings.EqualFold(w.cfg.OnError, "block") {
		return err
	}
	return nil
}

func buildTags(req CaptureRequest) []string {
	tags := []string{}
	add := func(v string) {
		v = strings.TrimSpace(strings.ToLower(v))
		if v != "" {
			tags = append(tags, v)
		}
	}
	add("session:" + req.SessionKey)
	add("thread:" + req.ThreadKey)
	add("client:" + req.ClientFamily)
	add("model:" + req.ModelRequested)
	return dedupe(tags)
}

func buildCaptureContent(cfg CaptureConfig, req CaptureRequest) (string, error) {
	prompt := strings.TrimSpace(string(req.PromptFull))
	if prompt == "" {
		return "", errors.New("empty prompt payload")
	}
	if cfg.RedactEnabled {
		prompt = redactText(prompt, cfg.RedactPatterns)
	}
	payload := map[string]any{
		"v":               1,
		"ts":              time.Now().UTC().Format(time.RFC3339),
		"session_key":     req.SessionKey,
		"thread_key":      req.ThreadKey,
		"client_family":   req.ClientFamily,
		"protocol":        req.Protocol,
		"path":            req.Path,
		"model_requested": req.ModelRequested,
		"model_routed":    req.ModelRouted,
		"meta": map[string]any{
			"api_key_hash": req.APIKeyHash,
			"source_ip":    req.SourceIP,
			"user_agent":   req.UserAgent,
		},
		"prompt_full": prompt,
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	if len(encoded) <= cfg.MaxBytesPerRecord {
		return string(encoded), nil
	}
	marker := "\n[TRUNCATED_BY_CLIPROXY_CONTEXT_MEMORY]"
	maxPromptBytes := cfg.MaxBytesPerRecord / 2
	if maxPromptBytes < 1024 {
		maxPromptBytes = 1024
	}
	if len(prompt) > maxPromptBytes {
		prompt = prompt[:maxPromptBytes] + marker
	}
	payload["prompt_full"] = prompt
	payload["truncated"] = true
	encoded, err = json.Marshal(payload)
	if err != nil {
		return "", err
	}
	if len(encoded) > cfg.MaxBytesPerRecord {
		encoded = encoded[:cfg.MaxBytesPerRecord]
	}
	return string(encoded), nil
}

func redactText(s string, custom []string) string {
	patterns := []string{
		`(?i)("authorization"\s*:\s*")[^"]*(")`,
		`(?i)("api[_-]?key"\s*:\s*")[^"]*(")`,
		`(?i)("password"\s*:\s*")[^"]*(")`,
		`(?i)("token"\s*:\s*")[^"]*(")`,
	}
	patterns = append(patterns, custom...)
	out := s
	for i := range patterns {
		re, err := regexp.Compile(patterns[i])
		if err != nil {
			continue
		}
		out = re.ReplaceAllString(out, `${1}[REDACTED]${2}`)
	}
	return out
}

func dedupe(items []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(items))
	for i := range items {
		if _, ok := seen[items[i]]; ok {
			continue
		}
		seen[items[i]] = struct{}{}
		out = append(out, items[i])
	}
	return out
}

func defaultRunner(ctx context.Context, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	return cmd.CombinedOutput()
}
