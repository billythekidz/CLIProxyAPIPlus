package websearch

import (
	"strings"
	"testing"

	"github.com/tidwall/gjson"
)

func TestInjectToolResultsClaude(t *testing.T) {
	body := []byte(`{"model":"claude-sonnet-4","messages":[{"role":"user","content":"test"}]}`)

	resp := &SearchResponse{
		Query: "golang",
		Results: []SearchResult{
			{Title: "Go Wiki", URL: "https://go.dev/wiki", Snippet: "The Go programming language"},
		},
	}

	result, err := InjectToolResultsClaude(body, "srvtoolu_test123", "golang", resp)
	if err != nil {
		t.Fatalf("InjectToolResultsClaude error: %v", err)
	}

	// Should have 3 messages now: user, assistant(tool_use), user(tool_result)
	msgCount := gjson.GetBytes(result, "messages.#").Int()
	if msgCount != 3 {
		t.Errorf("messages count = %d, want 3", msgCount)
	}

	// Assistant message should have tool_use
	assistantContent := gjson.GetBytes(result, "messages.1.content.0.type").String()
	if assistantContent != "tool_use" {
		t.Errorf("assistant content type = %q, want tool_use", assistantContent)
	}

	// User message should have tool_result
	userContent := gjson.GetBytes(result, "messages.2.content.0.type").String()
	if userContent != "tool_result" {
		t.Errorf("user content type = %q, want tool_result", userContent)
	}
}

func TestInjectToolResultsClaude_NilResponse(t *testing.T) {
	body := []byte(`{"model":"claude-sonnet-4","messages":[{"role":"user","content":"test"}]}`)
	result, err := InjectToolResultsClaude(body, "id", "q", nil)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if string(result) != string(body) {
		t.Error("body should be unchanged for nil response")
	}
}

func TestInjectToolResultsClaude_EmptyResults(t *testing.T) {
	body := []byte(`{"model":"claude-sonnet-4","messages":[{"role":"user","content":"test"}]}`)
	resp := &SearchResponse{Query: "test", Results: []SearchResult{}}
	result, err := InjectToolResultsClaude(body, "id", "q", resp)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if string(result) != string(body) {
		t.Error("body should be unchanged for empty results")
	}
}

func TestInjectSearchIndicatorsClaude(t *testing.T) {
	resp := []byte(`{"content":[{"type":"text","text":"Hello"}]}`)

	searchResp := &SearchResponse{
		Query: "test",
		Results: []SearchResult{
			{Title: "Test", URL: "https://example.com", Snippet: "Test snippet"},
		},
	}

	result, err := InjectSearchIndicatorsClaude(resp, "srvtoolu_test", "test query", searchResp)
	if err != nil {
		t.Fatalf("InjectSearchIndicatorsClaude error: %v", err)
	}

	contentTypes := []string{}
	gjson.GetBytes(result, "content").ForEach(func(_, v gjson.Result) bool {
		contentTypes = append(contentTypes, v.Get("type").String())
		return true
	})

	if len(contentTypes) < 3 {
		t.Fatalf("expected at least 3 content blocks, got %d: %v", len(contentTypes), contentTypes)
	}

	if contentTypes[0] != "server_tool_use" {
		t.Errorf("first content type = %q, want server_tool_use", contentTypes[0])
	}
	if contentTypes[1] != "web_search_tool_result" {
		t.Errorf("second content type = %q, want web_search_tool_result", contentTypes[1])
	}
}

func TestGenerateSearchIndicatorEventsClaude(t *testing.T) {
	resp := &SearchResponse{
		Query: "test",
		Results: []SearchResult{
			{Title: "Test", URL: "https://example.com", Snippet: "Snippet"},
		},
	}

	events := GenerateSearchIndicatorEventsClaude("test query", "srvtoolu_123", resp, 0)
	if len(events) != 4 {
		t.Fatalf("expected 4 events, got %d", len(events))
	}

	// Check event types
	for i, expected := range []string{"content_block_start", "content_block_stop", "content_block_start", "content_block_stop"} {
		if !strings.HasPrefix(string(events[i]), "event: "+expected) {
			t.Errorf("event[%d] = %q, want prefix event: %s", i, string(events[i])[:40], expected)
		}
	}
}

func TestGenerateToolUseID(t *testing.T) {
	id := GenerateToolUseID()
	if !strings.HasPrefix(id, "srvtoolu_") {
		t.Errorf("id = %q, want prefix srvtoolu_", id)
	}
	// "srvtoolu_" (9) + 32 hex chars = 41
	if len(id) != 41 {
		t.Errorf("id length = %d, want 41", len(id))
	}
}
