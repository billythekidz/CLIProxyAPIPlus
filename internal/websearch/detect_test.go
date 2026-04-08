package websearch

import (
	"testing"
)

func TestIsWebSearchToolName(t *testing.T) {
	tests := []struct {
		name     string
		toolType string
		want     bool
	}{
		{"web_search", "", true},
		{"WEB_SEARCH", "", true},
		{"web_search_20250305", "", true},
		{"web_search_preview", "", true},
		{"", "web_search_20250305", true},
		{"", "web_search", true},
		{"tool_search", "", false},
		{"ToolSearch", "", false},
		{"web_fetch", "", false},
		{"WebFetch", "", false},
		{"computer_tool", "", false},
		{"", "computer_20250124", false},
		{"", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name+"/"+tt.toolType, func(t *testing.T) {
			got := IsWebSearchToolName(tt.name, tt.toolType)
			if got != tt.want {
				t.Errorf("IsWebSearchToolName(%q, %q) = %v, want %v", tt.name, tt.toolType, got, tt.want)
			}
		})
	}
}

func TestDetectFormatAndQuery_Claude(t *testing.T) {
	body := []byte(`{
		"model": "claude-sonnet-4-20250514",
		"tools": [{"type":"web_search_20250305","name":"web_search","max_uses":5}],
		"messages": [{"role":"user","content":"What is Go?"}]
	}`)
	format, query, hasTool := DetectFormatAndQuery(body)
	if format != FormatClaude {
		t.Errorf("format = %q, want %q", format, FormatClaude)
	}
	if !hasTool {
		t.Error("hasTool = false, expected true since tool is present")
	}
	if query != "What is Go?" {
		t.Errorf("query = %q, want %q", query, "What is Go?")
	}
}

func TestDetectFormatAndQuery_ClaudeWithAssistantQuery(t *testing.T) {
	// When assistant has made a tool_use, the query is extracted from last user message
	body := []byte(`{
		"model": "claude-sonnet-4-20250514",
		"tools": [{"type":"web_search_20250305","name":"web_search","max_uses":5}],
		"messages": [
			{"role":"user","content":"search for golang"},
			{"role":"assistant","content":[{"type":"tool_use","id":"tu_123","name":"web_search","input":{"query":"golang programming"}}]},
			{"role":"user","content":[{"type":"tool_result","tool_use_id":"tu_123","content":"results here"}]},
			{"role":"assistant","content":"Can you search more specifically?"},
			{"role":"user","content":"golang generics tutorial"}
		]
	}`)
	format, query, hasTool := DetectFormatAndQuery(body)
	if format != FormatClaude {
		t.Errorf("format = %q, want %q", format, FormatClaude)
	}
	if !hasTool {
		t.Error("hasTool = false, expected true")
	}
	// Should extract from last user message
	if query != "golang generics tutorial" {
		t.Errorf("query = %q, want %q", query, "golang generics tutorial")
	}
}

func TestDetectFormatAndQuery_OpenAI(t *testing.T) {
	body := []byte(`{
		"model": "gpt-4",
		"tools": [{"type":"function","function":{"name":"web_search","parameters":{}}}],
		"messages": [
			{"role":"user","content":"rust vs go comparison"}
		]
	}`)
	format, query, hasTool := DetectFormatAndQuery(body)
	if format != FormatOpenAI {
		t.Errorf("format = %q, want %q", format, FormatOpenAI)
	}
	if !hasTool {
		t.Error("hasTool = false, expected true")
	}
	if query != "rust vs go comparison" {
		t.Errorf("query = %q, want %q", query, "rust vs go comparison")
	}
}

func TestDetectFormatAndQuery_NoWebSearch(t *testing.T) {
	body := []byte(`{
		"model": "claude-sonnet-4-20250514",
		"tools": [{"type":"computer_20250124","name":"computer"}],
		"messages": [{"role":"user","content":"test"}]
	}`)
	_, _, hasTool := DetectFormatAndQuery(body)
	if hasTool {
		t.Error("hasTool = true, expected false for non-web_search tools")
	}
}

func TestDetectFormatAndQuery_ExcludesToolSearch(t *testing.T) {
	body := []byte(`{
		"model": "claude-sonnet-4-20250514",
		"tools": [{"name":"ToolSearch","type":"tool_search"}],
		"messages": [{"role":"user","content":"test"}]
	}`)
	_, _, hasTool := DetectFormatAndQuery(body)
	if hasTool {
		t.Error("hasTool = true, expected false for ToolSearch")
	}
}

func TestDetectFormatAndQuery_ExcludesWebFetch(t *testing.T) {
	body := []byte(`{
		"model": "claude-sonnet-4-20250514",
		"tools": [{"name":"WebFetch","type":"web_fetch"}],
		"messages": [{"role":"user","content":"test"}]
	}`)
	_, _, hasTool := DetectFormatAndQuery(body)
	if hasTool {
		t.Error("hasTool = true, expected false for WebFetch")
	}
}

func TestDetectFormatAndQuery_EmptyBody(t *testing.T) {
	_, _, hasTool := DetectFormatAndQuery([]byte(`{}`))
	if hasTool {
		t.Error("hasTool = true for empty body")
	}
}
