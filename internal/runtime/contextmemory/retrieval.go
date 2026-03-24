package contextmemory

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

type BucketKind string

const (
	BucketFullShortTerm   BucketKind = "full_shortterm"
	BucketLatestShortTerm BucketKind = "latest_shortterm"
	BucketFullLongTerm    BucketKind = "full_longterm"
	BucketLatestLongTerm  BucketKind = "latest_longterm"
	BucketDerivedSummary  BucketKind = "derived_summary"
)

type BucketRequest struct {
	Kind         BucketKind
	SessionKey   string
	ThreadKey    string
	ClientFamily string
	Model        string
	Query        string
	MaxTokens    int
	Limit        int
}

type BucketResult struct {
	Kind    BucketKind
	Content string
	Score   float64
}

type BucketRetriever interface {
	Retrieve(context.Context, BucketRequest) (BucketResult, error)
}

type NMemRetrieverConfig struct {
	Command   string
	TimeoutMS int
}

type NMemRetriever struct {
	cfg NMemRetrieverConfig
}

func NewNMemRetriever(cfg NMemRetrieverConfig) *NMemRetriever {
	if strings.TrimSpace(cfg.Command) == "" {
		cfg.Command = "nmem"
	}
	if cfg.TimeoutMS <= 0 {
		cfg.TimeoutMS = 600
	}
	return &NMemRetriever{cfg: cfg}
}

func (r *NMemRetriever) Retrieve(ctx context.Context, req BucketRequest) (BucketResult, error) {
	if r == nil {
		return BucketResult{}, errors.New("nil retriever")
	}
	q := buildQuery(req)
	if strings.TrimSpace(q) == "" {
		q = "recent context"
	}
	timeout := time.Duration(r.cfg.TimeoutMS) * time.Millisecond
	callCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	maxTokens := req.MaxTokens
	if maxTokens <= 0 {
		maxTokens = 800
	}
	args := []string{"recall", q, "--max-tokens", fmt.Sprintf("%d", maxTokens), "--json"}
	cmd := exec.CommandContext(callCtx, r.cfg.Command, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return BucketResult{}, err
	}
	content, score := parseRecallJSON(out)
	if strings.TrimSpace(content) == "" {
		// fallback to context endpoint for summary bucket
		if req.Kind == BucketDerivedSummary {
			limit := req.Limit
			if limit <= 0 {
				limit = 8
			}
			cargs := []string{"context", "--limit", fmt.Sprintf("%d", limit), "--json"}
			cmd2 := exec.CommandContext(callCtx, r.cfg.Command, cargs...)
			out2, err2 := cmd2.CombinedOutput()
			if err2 == nil {
				content = parseContextJSON(out2)
				score = 0.6
			}
		}
	}
	if strings.TrimSpace(content) == "" {
		return BucketResult{}, errors.New("empty bucket content")
	}
	return BucketResult{Kind: req.Kind, Content: content, Score: score}, nil
}

func buildQuery(req BucketRequest) string {
	parts := []string{}
	if strings.TrimSpace(req.Query) != "" {
		parts = append(parts, req.Query)
	}
	tagHints := []string{}
	if req.SessionKey != "" {
		tagHints = append(tagHints, "session:"+req.SessionKey)
	}
	if req.ThreadKey != "" {
		tagHints = append(tagHints, "thread:"+req.ThreadKey)
	}
	if req.ClientFamily != "" {
		tagHints = append(tagHints, "client:"+req.ClientFamily)
	}
	if req.Model != "" {
		tagHints = append(tagHints, "model:"+req.Model)
	}
	switch req.Kind {
	case BucketFullShortTerm:
		parts = append(parts, "full short term conversation context")
	case BucketLatestShortTerm:
		parts = append(parts, "latest short term conversation context")
	case BucketFullLongTerm:
		parts = append(parts, "full long term decisions facts workflows")
	case BucketLatestLongTerm:
		parts = append(parts, "latest long term updates and decisions")
	case BucketDerivedSummary:
		parts = append(parts, "compact summary key decisions errors resolved")
	}
	if len(tagHints) > 0 {
		parts = append(parts, "tags:"+strings.Join(tagHints, " "))
	}
	return strings.TrimSpace(strings.Join(parts, " | "))
}

func parseRecallJSON(raw []byte) (string, float64) {
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return "", 0
	}
	answer, _ := payload["answer"].(string)
	if strings.TrimSpace(answer) == "" {
		if msg, ok := payload["message"].(string); ok {
			answer = msg
		}
	}
	score := 0.5
	if c, ok := payload["confidence"].(float64); ok {
		score = c
	}
	return strings.TrimSpace(answer), score
}

func parseContextJSON(raw []byte) string {
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return ""
	}
	if s, ok := payload["context"].(string); ok {
		return strings.TrimSpace(s)
	}
	return ""
}
