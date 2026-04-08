package contextmemory

import "strings"

type PromptBlock struct {
	Kind      string
	Text      string
	Score     float64
	Protected bool
}

type CompressResult struct {
	Blocks       []PromptBlock
	TokensBefore int
	TokensAfter  int
	Dropped      int
	Trimmed      int
}

func FinalCompress(blocks []PromptBlock, targetTokens int) CompressResult {
	if targetTokens <= 0 {
		targetTokens = 1
	}
	work := make([]PromptBlock, 0, len(blocks))
	for i := range blocks {
		b := blocks[i]
		if strings.TrimSpace(b.Text) == "" {
			continue
		}
		work = append(work, b)
	}
	before := countTokens(work)
	if before <= targetTokens {
		return CompressResult{Blocks: work, TokensBefore: before, TokensAfter: before}
	}

	dropped := 0
	trimmed := 0
	// Pass 1: drop lowest-score non-protected blocks first.
	for countTokens(work) > targetTokens {
		idx := indexLowestDroppable(work)
		if idx < 0 {
			break
		}
		work = append(work[:idx], work[idx+1:]...)
		dropped++
	}

	// Pass 2: trim longest non-protected blocks until within budget.
	for countTokens(work) > targetTokens {
		idx := indexLongestTrimmable(work)
		if idx < 0 {
			break
		}
		trimmedText, changed := trimHalf(work[idx].Text)
		if !changed {
			break
		}
		work[idx].Text = trimmedText
		trimmed++
	}
	after := countTokens(work)
	return CompressResult{Blocks: work, TokensBefore: before, TokensAfter: after, Dropped: dropped, Trimmed: trimmed}
}

func indexLowestDroppable(blocks []PromptBlock) int {
	idx := -1
	minScore := 1e18
	for i := range blocks {
		if blocks[i].Protected {
			continue
		}
		if blocks[i].Score < minScore {
			minScore = blocks[i].Score
			idx = i
		}
	}
	return idx
}

func indexLongestTrimmable(blocks []PromptBlock) int {
	idx := -1
	maxTokens := 0
	for i := range blocks {
		if blocks[i].Protected {
			continue
		}
		toks := estimateTokens(blocks[i].Text)
		if toks > maxTokens {
			maxTokens = toks
			idx = i
		}
	}
	if maxTokens < 16 {
		return -1
	}
	return idx
}

func trimHalf(text string) (string, bool) {
	words := strings.Fields(strings.TrimSpace(text))
	if len(words) < 10 {
		return text, false
	}
	n := len(words) / 2
	if n < 6 {
		n = 6
	}
	out := strings.Join(words[:n], " ") + " ..."
	return out, true
}

func estimateTokens(text string) int {
	words := strings.Fields(strings.TrimSpace(text))
	if len(words) == 0 {
		return 0
	}
	return int(float64(len(words))*1.3 + 0.5)
}

func countTokens(blocks []PromptBlock) int {
	total := 0
	for i := range blocks {
		total += estimateTokens(blocks[i].Text)
	}
	return total
}
