package contextmemory

import (
	"context"
	"strings"
)

type Message struct {
	Role      string
	Content   string
	Score     float64
	Protected bool
}

type AssembleRequest struct {
	ContextWindow int
	TargetTokens  int
	Query         string
	SessionKey    string
	ThreadKey     string
	ClientFamily  string
	Model         string
	System        []string
	Active        []Message
	Ratios        RatioSet
}

type AssembleResult struct {
	Blocks       []PromptBlock
	TokensBefore int
	TokensAfter  int
	Dropped      int
	Trimmed      int
}

func AssembleReducedContext(ctx context.Context, retriever BucketRetriever, req AssembleRequest) (AssembleResult, error) {
	if req.TargetTokens <= 0 {
		req.TargetTokens = 48000
	}
	r := req.Ratios.Normalize()
	if r.Active == 0 && r.Recalled == 0 && r.Summaries == 0 && r.Buffer == 0 {
		r = RatioSet{Active: 0.5, Recalled: 0.3, Summaries: 0.15, Buffer: 0.05}
	}

	activeBudget := int(float64(req.TargetTokens) * r.Active)
	recalledBudget := int(float64(req.TargetTokens) * r.Recalled)
	summaryBudget := int(float64(req.TargetTokens) * r.Summaries)

	blocks := make([]PromptBlock, 0, len(req.System)+len(req.Active)+8)
	for i := range req.System {
		if strings.TrimSpace(req.System[i]) == "" {
			continue
		}
		blocks = append(blocks, PromptBlock{Kind: "system", Text: req.System[i], Score: 100, Protected: true})
	}

	longBudget := int(float64(recalledBudget) * 0.55)
	shortBudget := recalledBudget - longBudget
	latestWeight := 0.65

	bucketReqs := []BucketRequest{
		{Kind: BucketDerivedSummary, Query: req.Query, SessionKey: req.SessionKey, ThreadKey: req.ThreadKey, ClientFamily: req.ClientFamily, Model: req.Model, MaxTokens: summaryBudget, Limit: 8},
		{Kind: BucketFullLongTerm, Query: req.Query, SessionKey: req.SessionKey, ThreadKey: req.ThreadKey, ClientFamily: req.ClientFamily, Model: req.Model, MaxTokens: int(float64(longBudget) * (1 - latestWeight))},
		{Kind: BucketLatestLongTerm, Query: req.Query, SessionKey: req.SessionKey, ThreadKey: req.ThreadKey, ClientFamily: req.ClientFamily, Model: req.Model, MaxTokens: int(float64(longBudget) * latestWeight)},
		{Kind: BucketFullShortTerm, Query: req.Query, SessionKey: req.SessionKey, ThreadKey: req.ThreadKey, ClientFamily: req.ClientFamily, Model: req.Model, MaxTokens: int(float64(shortBudget) * (1 - latestWeight))},
		{Kind: BucketLatestShortTerm, Query: req.Query, SessionKey: req.SessionKey, ThreadKey: req.ThreadKey, ClientFamily: req.ClientFamily, Model: req.Model, MaxTokens: int(float64(shortBudget) * latestWeight)},
	}
	bucketBlocks := fetchBucketBlocks(ctx, retriever, bucketReqs)
	bucketBlocks = orderBucketBlocks(req.ContextWindow, bucketBlocks)
	blocks = append(blocks, bucketBlocks...)

	activeBlocks := make([]PromptBlock, 0, len(req.Active))
	for i := range req.Active {
		m := req.Active[i]
		if strings.TrimSpace(m.Content) == "" {
			continue
		}
		score := m.Score
		if score <= 0 {
			score = 1
		}
		activeBlocks = append(activeBlocks, PromptBlock{Kind: "active", Text: m.Content, Score: score, Protected: m.Protected})
	}
	activeBlocks = enforceBudget(activeBlocks, activeBudget)
	blocks = append(blocks, activeBlocks...)

	compressed := FinalCompress(blocks, req.TargetTokens)
	return AssembleResult{
		Blocks:       compressed.Blocks,
		TokensBefore: compressed.TokensBefore,
		TokensAfter:  compressed.TokensAfter,
		Dropped:      compressed.Dropped,
		Trimmed:      compressed.Trimmed,
	}, nil
}

func fetchBucketBlocks(ctx context.Context, retriever BucketRetriever, reqs []BucketRequest) []PromptBlock {
	out := make([]PromptBlock, 0, len(reqs))
	for i := range reqs {
		res, err := retriever.Retrieve(ctx, reqs[i])
		if err != nil || strings.TrimSpace(res.Content) == "" {
			continue
		}
		kind := "memory"
		if reqs[i].Kind == BucketDerivedSummary {
			kind = "summary"
		}
		score := res.Score
		if score <= 0 {
			score = 0.5
		}
		out = append(out, PromptBlock{Kind: kind, Text: res.Content, Score: score, Protected: false})
	}
	return out
}

func orderBucketBlocks(contextWindow int, blocks []PromptBlock) []PromptBlock {
	if len(blocks) == 0 {
		return blocks
	}
	if contextWindow < 1000000 {
		return blocks
	}
	// Multi-segment friendly ordering for very large windows:
	// summaries first, other memory blocks later (active is appended at end by caller).
	summaries := make([]PromptBlock, 0, len(blocks))
	other := make([]PromptBlock, 0, len(blocks))
	for i := range blocks {
		if blocks[i].Kind == "summary" {
			summaries = append(summaries, blocks[i])
		} else {
			other = append(other, blocks[i])
		}
	}
	ordered := make([]PromptBlock, 0, len(blocks))
	ordered = append(ordered, summaries...)
	ordered = append(ordered, other...)
	return ordered
}

func enforceBudget(blocks []PromptBlock, budget int) []PromptBlock {
	if budget <= 0 || len(blocks) == 0 {
		return []PromptBlock{}
	}
	used := 0
	out := make([]PromptBlock, 0, len(blocks))
	for i := range blocks {
		toks := estimateTokens(blocks[i].Text)
		if used+toks <= budget {
			out = append(out, blocks[i])
			used += toks
			continue
		}
		// partial trim if room remains
		room := budget - used
		if room < 16 {
			break
		}
		trimmed, changed := trimToTokens(blocks[i].Text, room)
		if !changed {
			break
		}
		blocks[i].Text = trimmed
		out = append(out, blocks[i])
		used += estimateTokens(trimmed)
		break
	}
	return out
}

func trimToTokens(text string, target int) (string, bool) {
	words := strings.Fields(strings.TrimSpace(text))
	if len(words) == 0 || target <= 0 {
		return text, false
	}
	maxWords := int(float64(target) / 1.3)
	if maxWords < 8 || maxWords >= len(words) {
		return text, false
	}
	return strings.Join(words[:maxWords], " ") + " ...", true
}
