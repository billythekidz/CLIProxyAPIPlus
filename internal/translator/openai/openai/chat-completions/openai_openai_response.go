// Package chat_completions provides passthrough response translation for OpenAI Chat Completions.
// It normalizes OpenAI-compatible SSE lines by stripping the "data:" prefix, dropping "[DONE]",
// and removing known special tokens from response content.
package chat_completions

import (
	"bytes"
	"context"
	"regexp"
)

// specialTokenRe matches common LLM special tokens that may leak into output.
// Covers: ChatML tokens (Qwen), EOS/endoftext markers, and legacy control tokens.
var specialTokenRe = regexp.MustCompile(`(?:<\|im_end\|>|<\|im_start\|>(?:system|user|assistant)?|<\|endoftext\|>|<\|endofprompt\|>|</s>)`)

// stripSpecialTokens removes known special tokens from raw bytes.
func stripSpecialTokens(data []byte) []byte {
	return specialTokenRe.ReplaceAll(data, nil)
}

// ConvertOpenAIResponseToOpenAI normalizes a single chunk of an OpenAI-compatible streaming response.
// Strips SSE "data:" prefix, drops "[DONE]", and removes special tokens from content.
func ConvertOpenAIResponseToOpenAI(_ context.Context, _ string, originalRequestRawJSON, requestRawJSON, rawJSON []byte, param *any) [][]byte {
	if bytes.HasPrefix(rawJSON, []byte("data:")) {
		rawJSON = bytes.TrimSpace(rawJSON[5:])
	}
	if bytes.Equal(rawJSON, []byte("[DONE]")) {
		return [][]byte{}
	}
	return [][]byte{stripSpecialTokens(rawJSON)}
}

// ConvertOpenAIResponseToOpenAINonStream passes through a non-streaming OpenAI response
// with special tokens stripped from the content.
func ConvertOpenAIResponseToOpenAINonStream(ctx context.Context, modelName string, originalRequestRawJSON, requestRawJSON, rawJSON []byte, param *any) []byte {
	return stripSpecialTokens(rawJSON)
}
