package contextmemory

import (
	"context"
	"encoding/json"
	"testing"
)

type fixedRetriever struct {
	content string
}

func (r fixedRetriever) Retrieve(_ context.Context, req BucketRequest) (BucketResult, error) {
	return BucketResult{Kind: req.Kind, Content: r.content, Score: 0.9}, nil
}

func TestReduceOpenAIPayload_ReducesTokensAndPreservesMessages(t *testing.T) {
	payload := []byte(`{"model":"claude-opus-4.6","messages":[{"role":"system","content":"you are helpful"},{"role":"user","content":"alpha beta gamma delta epsilon zeta eta theta iota kappa lambda mu nu xi omicron pi rho sigma tau"},{"role":"assistant","content":"old answer with many words repeated repeated repeated repeated repeated"},{"role":"user","content":"new task details with enough words to allow trimming and compression in the reducer"}]}`)

	result, err := ReduceOpenAIPayload(context.Background(), PayloadReduceRequest{
		Payload:       payload,
		Model:         "claude-opus-4.6",
		ContextWindow: 128000,
		TargetTokens:  40,
		Ratios:        RatioSet{Active: 0.5, Recalled: 0.3, Summaries: 0.15, Buffer: 0.05},
		SessionKey:    "s1",
		ThreadKey:     "t1",
		ClientFamily:  "generic",
		Retriever:     fixedRetriever{content: "memory context block with prior facts"},
	})
	if err != nil {
		t.Fatalf("ReduceOpenAIPayload error: %v", err)
	}
	if !result.Applied {
		t.Fatalf("expected reduction to be applied")
	}
	if result.TokensAfter >= result.TokensBefore {
		t.Fatalf("expected token reduction, before=%d after=%d", result.TokensBefore, result.TokensAfter)
	}
	var root map[string]any
	if err := json.Unmarshal(result.Payload, &root); err != nil {
		t.Fatalf("reduced payload invalid json: %v", err)
	}
	msgs, ok := root["messages"].([]any)
	if !ok || len(msgs) == 0 {
		t.Fatalf("expected non-empty messages")
	}
}
