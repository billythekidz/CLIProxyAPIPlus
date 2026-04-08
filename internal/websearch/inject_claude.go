package websearch

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// InjectToolResultsClaude modifies a Claude-format request payload to append
// assistant tool_use + user tool_result messages containing search results.
// This follows the pattern from kiro_websearch.go InjectToolResultsClaude.
func InjectToolResultsClaude(body []byte, toolUseID, query string, resp *SearchResponse) ([]byte, error) {
	if resp == nil || len(resp.Results) == 0 {
		return body, nil
	}

	resultText := FormatToolResultText(resp)

	// Append assistant message with tool_use
	assistantMsg := fmt.Sprintf(
		`{"role":"assistant","content":[{"type":"tool_use","id":"%s","name":"web_search","input":{"query":%s}}]}`,
		toolUseID, mustMarshal(query),
	)
	body, _ = sjson.SetRawBytes(body, "messages.-1", []byte(assistantMsg))

	// Append user message with tool_result + search guidance
	searchGuidance := buildSearchGuidance()
	toolResultMsg := fmt.Sprintf(
		`{"role":"user","content":[{"type":"tool_result","tool_use_id":"%s","content":%s},{"type":"text","text":%s}]}`,
		toolUseID, mustMarshal(resultText), mustMarshal(searchGuidance),
	)
	body, _ = sjson.SetRawBytes(body, "messages.-1", []byte(toolResultMsg))

	return body, nil
}

// InjectSearchIndicatorsClaude prepends server_tool_use + web_search_tool_result
// content blocks into a non-streaming Claude response.
func InjectSearchIndicatorsClaude(responsePayload []byte, toolUseID, query string, resp *SearchResponse) ([]byte, error) {
	if resp == nil {
		return responsePayload, nil
	}

	existingContent := gjson.GetBytes(responsePayload, "content").Raw
	if existingContent == "" {
		existingContent = "[]"
	}

	// Build web_search_result items for web_search_tool_result
	searchResults := buildClaudeSearchResultItems(resp)

	// server_tool_use block
	serverToolUse := fmt.Sprintf(
		`{"type":"server_tool_use","id":"%s","name":"web_search","input":{"query":%s}}`,
		toolUseID, mustMarshal(query),
	)

	// web_search_tool_result block
	toolResult := fmt.Sprintf(
		`{"type":"web_search_tool_result","tool_use_id":"%s","content":%s}`,
		toolUseID, searchResults,
	)

	// Prepend before existing content
	newContent := fmt.Sprintf(`[%s,%s,%s]`, serverToolUse, toolResult, strings.TrimPrefix(existingContent, "["))
	// Fix trailing bracket
	if !strings.HasSuffix(newContent, "]") {
		newContent += "]"
	}

	result, err := sjson.SetRawBytes(responsePayload, "content", []byte(newContent))
	if err != nil {
		return responsePayload, fmt.Errorf("inject search indicators: %w", err)
	}
	return result, nil
}

// GenerateSearchIndicatorEventsClaude generates SSE events for streaming.
// Produces server_tool_use + web_search_tool_result events matching the Claude stream format.
func GenerateSearchIndicatorEventsClaude(query, toolUseID string, resp *SearchResponse, startIndex int) [][]byte {
	if resp == nil {
		return nil
	}

	var events [][]byte

	// 1. content_block_start for server_tool_use
	events = append(events, []byte(fmt.Sprintf(
		"event: content_block_start\ndata: {\"type\":\"content_block_start\",\"index\":%d,\"content_block\":{\"type\":\"server_tool_use\",\"id\":\"%s\",\"name\":\"web_search\",\"input\":{\"query\":%s}}}\n\n",
		startIndex, toolUseID, mustMarshal(query),
	)))

	// 2. content_block_stop for server_tool_use
	events = append(events, []byte(fmt.Sprintf(
		"event: content_block_stop\ndata: {\"type\":\"content_block_stop\",\"index\":%d}\n\n",
		startIndex,
	)))

	// 3. content_block_start for web_search_tool_result
	searchResults := buildClaudeSearchResultItems(resp)
	events = append(events, []byte(fmt.Sprintf(
		"event: content_block_start\ndata: {\"type\":\"content_block_start\",\"index\":%d,\"content_block\":{\"type\":\"web_search_tool_result\",\"tool_use_id\":\"%s\",\"content\":%s}}\n\n",
		startIndex+1, toolUseID, searchResults,
	)))

	// 4. content_block_stop for web_search_tool_result
	events = append(events, []byte(fmt.Sprintf(
		"event: content_block_stop\ndata: {\"type\":\"content_block_stop\",\"index\":%d}\n\n",
		startIndex+1,
	)))

	return events
}

// buildClaudeSearchResultItems builds the web_search_result content array
// matching the Claude API wire format for web_search_tool_result.
func buildClaudeSearchResultItems(resp *SearchResponse) string {
	items := make([]string, 0, len(resp.Results))
	for _, r := range resp.Results {
		item := fmt.Sprintf(
			`{"type":"web_search_result","title":%s,"url":%s,"encrypted_content":%s,"page_age":null}`,
			mustMarshal(r.Title), mustMarshal(r.URL), mustMarshal(r.Snippet),
		)
		items = append(items, item)
	}
	return "[" + strings.Join(items, ",") + "]"
}

// buildSearchGuidance returns the guidance text appended after tool_result.
func buildSearchGuidance() string {
	return fmt.Sprintf(
		"Current date: %s. If these results are insufficient, you may request another search with a refined query.",
		time.Now().Format("January 2, 2006"),
	)
}

// GenerateToolUseID creates a Claude-style tool use ID.
func GenerateToolUseID() string {
	return "srvtoolu_" + strings.ReplaceAll(uuid.New().String(), "-", "")[:32]
}

func mustMarshal(v string) string {
	b, err := json.Marshal(v)
	if err != nil {
		return `""`
	}
	return string(b)
}
