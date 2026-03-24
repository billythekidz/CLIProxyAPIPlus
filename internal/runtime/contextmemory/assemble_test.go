package contextmemory

import (
	"context"
	"testing"
)

type fakeRetriever struct{}

func (f fakeRetriever) Retrieve(ctx context.Context, req BucketRequest) (BucketResult, error) {
	content := ""
	score := 0.6
	switch req.Kind {
	case BucketDerivedSummary:
		content = "summary: key decisions and resolved issues"
		score = 0.9
	case BucketLatestShortTerm:
		content = "latest shortterm: recent command outputs and user constraints"
		score = 0.8
	case BucketFullShortTerm:
		content = "full shortterm: larger recent context"
		score = 0.7
	case BucketLatestLongTerm:
		content = "latest longterm: recent cross-session updates"
		score = 0.7
	case BucketFullLongTerm:
		content = "full longterm: architecture decisions and workflows"
		score = 0.6
	}
	return BucketResult{Kind: req.Kind, Content: content, Score: score}, nil
}

func TestAssembleReducedContext_WithinTargetAndPreservesSystem(t *testing.T) {
	req := AssembleRequest{
		ContextWindow: 200000,
		TargetTokens:  120,
		Query:         "debug auth issue",
		SessionKey:    "nm_sess:abc",
		ThreadKey:     "nm_thread:def",
		ClientFamily:  "ampcode",
		Model:         "claude-opus-4-6",
		System:        []string{"You are a safe coding assistant."},
		Active: []Message{
			{Role: "user", Content: "please inspect auth flow and keep backward compatibility", Score: 1, Protected: true},
			{Role: "assistant", Content: "I will inspect and propose minimal patches", Score: 0.9},
		},
		Ratios: RatioSet{Active: 0.5, Recalled: 0.3, Summaries: 0.15, Buffer: 0.05},
	}
	res, err := AssembleReducedContext(context.Background(), fakeRetriever{}, req)
	if err != nil {
		t.Fatalf("AssembleReducedContext() error = %v", err)
	}
	if res.TokensAfter > req.TargetTokens {
		t.Fatalf("tokens after = %d, want <= %d", res.TokensAfter, req.TargetTokens)
	}
	foundSystem := false
	for i := range res.Blocks {
		if res.Blocks[i].Kind == "system" {
			foundSystem = true
			break
		}
	}
	if !foundSystem {
		t.Fatalf("expected system block preserved")
	}
}
