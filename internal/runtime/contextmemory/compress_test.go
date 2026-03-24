package contextmemory

import "testing"

func TestFinalCompress_DropsLowScoreFirst(t *testing.T) {
	blocks := []PromptBlock{
		{Kind: "system", Text: "keep this system instruction", Score: 100, Protected: true},
		{Kind: "mem", Text: "low score block with many words many words many words many words", Score: 0.1},
		{Kind: "mem", Text: "high score block with many words many words many words many words", Score: 0.9},
	}
	res := FinalCompress(blocks, 20)
	if res.TokensAfter > 20 {
		t.Fatalf("tokens after > target: %d", res.TokensAfter)
	}
	if res.Dropped == 0 {
		t.Fatalf("expected at least one dropped block")
	}
	for i := range res.Blocks {
		if res.Blocks[i].Kind == "system" && res.Blocks[i].Text == "" {
			t.Fatalf("protected block should not be removed")
		}
	}
}

func TestFinalCompress_TrimsWhenNoDroppable(t *testing.T) {
	blocks := []PromptBlock{
		{Kind: "system", Text: "very long protected system content repeated repeated repeated repeated repeated repeated repeated repeated", Score: 10, Protected: true},
		{Kind: "user", Text: "very long user content repeated repeated repeated repeated repeated repeated repeated repeated", Score: 10, Protected: true},
	}
	res := FinalCompress(blocks, 10)
	if res.TokensAfter <= 0 {
		t.Fatalf("expected non-zero tokens after compression")
	}
	if len(res.Blocks) != 2 {
		t.Fatalf("protected blocks should remain present")
	}
}
