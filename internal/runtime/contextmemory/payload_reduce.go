package contextmemory

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

type PayloadReduceRequest struct {
	Payload       []byte
	Model         string
	ContextWindow int
	TargetTokens  int
	Ratios        RatioSet
	SessionKey    string
	ThreadKey     string
	ClientFamily  string
	Retriever     BucketRetriever
}

type PayloadReduceResult struct {
	Payload      []byte
	TokensBefore int
	TokensAfter  int
	Dropped      int
	Trimmed      int
	Applied      bool
}

func ReduceOpenAIPayload(ctx context.Context, req PayloadReduceRequest) (PayloadReduceResult, error) {
	if len(req.Payload) == 0 {
		return PayloadReduceResult{}, nil
	}
	if req.TargetTokens <= 0 {
		req.TargetTokens = 48000
	}
	if req.Ratios == (RatioSet{}) {
		req.Ratios = RatioSet{Active: 0.5, Recalled: 0.3, Summaries: 0.15, Buffer: 0.05}
	}

	var root map[string]any
	if err := json.Unmarshal(req.Payload, &root); err != nil {
		return PayloadReduceResult{}, err
	}
	messages, ok := root["messages"].([]any)
	if !ok || len(messages) == 0 {
		return PayloadReduceResult{Payload: req.Payload}, nil
	}

	type activeEntry struct {
		Role      string
		Content   string
		Score     float64
		Protected bool
	}
	active := make([]activeEntry, 0, len(messages))
	systemTexts := make([]string, 0, 4)
	lastUser := ""
	for i := range messages {
		msgMap, ok := messages[i].(map[string]any)
		if !ok {
			continue
		}
		role := strings.TrimSpace(strings.ToLower(toString(msgMap["role"])))
		if role == "" {
			role = "user"
		}
		content := strings.TrimSpace(extractMessageContentText(msgMap["content"]))
		if content == "" {
			continue
		}
		if role == "system" {
			systemTexts = append(systemTexts, content)
			continue
		}
		score := 1 + float64(i)/3
		protected := i >= len(messages)-2 || (role == "user" && i == len(messages)-1)
		active = append(active, activeEntry{Role: role, Content: content, Score: score, Protected: protected})
		if role == "user" {
			lastUser = content
		}
	}
	if len(active) == 0 {
		return PayloadReduceResult{Payload: req.Payload}, nil
	}

	if v := strings.TrimSpace(toString(root["system"])); v != "" {
		systemTexts = append(systemTexts, v)
	}
	if v := strings.TrimSpace(toString(root["instructions"])); v != "" {
		systemTexts = append(systemTexts, v)
	}
	query := lastUser
	if query == "" {
		query = active[len(active)-1].Content
	}

	recalledBudget := int(float64(req.TargetTokens) * req.Ratios.Normalize().Recalled)
	summaryBudget := int(float64(req.TargetTokens) * req.Ratios.Normalize().Summaries)
	longBudget := int(float64(recalledBudget) * 0.55)
	shortBudget := recalledBudget - longBudget
	latestWeight := 0.65
	bucketReqs := []BucketRequest{
		{Kind: BucketDerivedSummary, Query: query, SessionKey: req.SessionKey, ThreadKey: req.ThreadKey, ClientFamily: req.ClientFamily, Model: req.Model, MaxTokens: summaryBudget, Limit: 8},
		{Kind: BucketFullLongTerm, Query: query, SessionKey: req.SessionKey, ThreadKey: req.ThreadKey, ClientFamily: req.ClientFamily, Model: req.Model, MaxTokens: int(float64(longBudget) * (1 - latestWeight))},
		{Kind: BucketLatestLongTerm, Query: query, SessionKey: req.SessionKey, ThreadKey: req.ThreadKey, ClientFamily: req.ClientFamily, Model: req.Model, MaxTokens: int(float64(longBudget) * latestWeight)},
		{Kind: BucketFullShortTerm, Query: query, SessionKey: req.SessionKey, ThreadKey: req.ThreadKey, ClientFamily: req.ClientFamily, Model: req.Model, MaxTokens: int(float64(shortBudget) * (1 - latestWeight))},
		{Kind: BucketLatestShortTerm, Query: query, SessionKey: req.SessionKey, ThreadKey: req.ThreadKey, ClientFamily: req.ClientFamily, Model: req.Model, MaxTokens: int(float64(shortBudget) * latestWeight)},
	}
	bucketBlocks := fetchBucketBlocks(ctx, req.Retriever, bucketReqs)
	bucketBlocks = orderBucketBlocks(req.ContextWindow, bucketBlocks)

	blocks := make([]PromptBlock, 0, len(systemTexts)+len(bucketBlocks)+len(active))
	for i := range systemTexts {
		text := strings.TrimSpace(systemTexts[i])
		if text == "" {
			continue
		}
		blocks = append(blocks, PromptBlock{Kind: "system", Text: text, Score: 100, Protected: true})
	}
	blocks = append(blocks, bucketBlocks...)
	for i := range active {
		kind := fmt.Sprintf("active:%s", sanitizeRole(active[i].Role))
		blocks = append(blocks, PromptBlock{Kind: kind, Text: active[i].Content, Score: active[i].Score, Protected: active[i].Protected})
	}

	compressed := FinalCompress(blocks, req.TargetTokens)
	if compressed.TokensAfter >= compressed.TokensBefore {
		return PayloadReduceResult{Payload: req.Payload, TokensBefore: compressed.TokensBefore, TokensAfter: compressed.TokensAfter}, nil
	}

	newMessages := make([]map[string]any, 0, len(compressed.Blocks))
	for i := range compressed.Blocks {
		b := compressed.Blocks[i]
		text := strings.TrimSpace(b.Text)
		if text == "" {
			continue
		}
		role := "system"
		if strings.HasPrefix(b.Kind, "active:") {
			role = strings.TrimPrefix(b.Kind, "active:")
			if role == "" {
				role = "user"
			}
		}
		newMessages = append(newMessages, map[string]any{"role": role, "content": text})
	}
	if len(newMessages) == 0 {
		return PayloadReduceResult{Payload: req.Payload, TokensBefore: compressed.TokensBefore, TokensAfter: compressed.TokensAfter}, nil
	}

	root["messages"] = newMessages
	delete(root, "system")
	delete(root, "instructions")
	reducedPayload, err := json.Marshal(root)
	if err != nil {
		return PayloadReduceResult{}, err
	}

	return PayloadReduceResult{
		Payload:      reducedPayload,
		TokensBefore: compressed.TokensBefore,
		TokensAfter:  compressed.TokensAfter,
		Dropped:      compressed.Dropped,
		Trimmed:      compressed.Trimmed,
		Applied:      true,
	}, nil
}

func sanitizeRole(role string) string {
	role = strings.TrimSpace(strings.ToLower(role))
	switch role {
	case "assistant", "tool", "developer", "system":
		return role
	default:
		return "user"
	}
}

func toString(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case []byte:
		return string(t)
	default:
		return ""
	}
}

func extractMessageContentText(v any) string {
	switch content := v.(type) {
	case string:
		return content
	case []any:
		parts := make([]string, 0, len(content))
		for i := range content {
			item, ok := content[i].(map[string]any)
			if !ok {
				continue
			}
			if text := strings.TrimSpace(toString(item["text"])); text != "" {
				parts = append(parts, text)
				continue
			}
			if text := strings.TrimSpace(toString(item["content"])); text != "" {
				parts = append(parts, text)
				continue
			}
			if text := strings.TrimSpace(toString(item["input_text"])); text != "" {
				parts = append(parts, text)
				continue
			}
			if text := strings.TrimSpace(toString(item["output_text"])); text != "" {
				parts = append(parts, text)
			}
		}
		return strings.Join(parts, "\n")
	default:
		return ""
	}
}
