package websearch

import (
	"strings"

	"github.com/tidwall/gjson"
)

// Format constants for payload detection.
const (
	FormatClaude        = "claude"
	FormatOpenAI        = "openai"
	FormatCodex         = "codex"
	FormatCodexResponses = "codex-responses"
	FormatUnknown       = "unknown"
)

// webSearchAliases lists all known web_search tool name/type aliases
// across Claude, OpenAI, and Codex formats.
var webSearchAliases = map[string]bool{
	"web_search":                    true,
	"web_search_20250305":           true,
	"web_search_preview":            true,
	"web_search_preview_2025_03_11": true,
}

// IsWebSearchToolName checks if a tool name or type matches any known
// web_search variant. Returns false for ToolSearch or WebFetch.
func IsWebSearchToolName(name, toolType string) bool {
	name = strings.ToLower(name)
	toolType = strings.ToLower(toolType)

	// Explicitly exclude non-search tools
	if name == "toolsearch" || name == "webfetch" || name == "fetch" {
		return false
	}

	if webSearchAliases[name] {
		return true
	}
	if webSearchAliases[toolType] {
		return true
	}
	// Also match any type starting with "web_search"
	if strings.HasPrefix(toolType, "web_search") {
		return true
	}
	return false
}

// HasWebSearchTool checks if a Claude-format payload contains a web_search tool.
// Unlike the Kiro version, this returns true if ANY tool in the array is web_search,
// not just when it's the only tool.
func HasWebSearchTool(body []byte) bool {
	tools := gjson.GetBytes(body, "tools")
	if !tools.IsArray() {
		return false
	}
	for _, tool := range tools.Array() {
		name := strings.ToLower(tool.Get("name").String())
		toolType := strings.ToLower(tool.Get("type").String())
		if IsWebSearchToolName(name, toolType) {
			return true
		}
	}
	return false
}

// DetectFormatAndQuery detects the payload format and extracts the search query.
// Returns (format, query, hasTool).
func DetectFormatAndQuery(body []byte) (string, string, bool) {
	// Try Claude format first (most common in this proxy)
	if HasWebSearchTool(body) {
		query := ExtractSearchQueryClaude(body)
		return FormatClaude, query, true
	}

	// Try OpenAI chat-completions format
	if hasWebSearchToolOpenAI(body) {
		query := extractSearchQueryOpenAI(body)
		return FormatOpenAI, query, true
	}

	// Try Codex Responses format
	if hasWebSearchToolCodex(body) {
		query := extractSearchQueryGeneric(body) // Codex Responses uses "input" field
		return FormatCodexResponses, query, true
	}

	return FormatUnknown, "", false
}

// ExtractSearchQueryClaude extracts the search query from a Claude-format payload.
// It reads the last user message content and strips known prefixes.
func ExtractSearchQueryClaude(body []byte) string {
	messages := gjson.GetBytes(body, "messages")
	if !messages.IsArray() {
		return ""
	}

	// Walk backwards to find the last user message
	msgs := messages.Array()
	for i := len(msgs) - 1; i >= 0; i-- {
		msg := msgs[i]
		if msg.Get("role").String() != "user" {
			continue
		}
		return extractTextFromContent(msg.Get("content"))
	}
	return ""
}

// hasWebSearchToolOpenAI checks for web_search in OpenAI chat-completions format.
// OpenAI uses tools[].type == "function" with function.name == "web_search".
func hasWebSearchToolOpenAI(body []byte) bool {
	tools := gjson.GetBytes(body, "tools")
	if !tools.IsArray() {
		return false
	}
	for _, tool := range tools.Array() {
		// OpenAI function tool
		if tool.Get("type").String() == "function" {
			name := strings.ToLower(tool.Get("function.name").String())
			if IsWebSearchToolName(name, "") {
				return true
			}
		}
	}
	return false
}

// hasWebSearchToolCodex checks for web_search in Codex Responses format.
// Codex uses tools[].type == "web_search" or aliases like "web_search_preview".
func hasWebSearchToolCodex(body []byte) bool {
	tools := gjson.GetBytes(body, "tools")
	if !tools.IsArray() {
		return false
	}
	for _, tool := range tools.Array() {
		toolType := strings.ToLower(tool.Get("type").String())
		if IsWebSearchToolName("", toolType) {
			return true
		}
	}
	return false
}

// extractSearchQueryOpenAI extracts query from OpenAI chat-completions messages.
func extractSearchQueryOpenAI(body []byte) string {
	messages := gjson.GetBytes(body, "messages")
	if !messages.IsArray() {
		return ""
	}
	msgs := messages.Array()
	for i := len(msgs) - 1; i >= 0; i-- {
		msg := msgs[i]
		if msg.Get("role").String() != "user" {
			continue
		}
		return msg.Get("content").String()
	}
	return ""
}

// extractSearchQueryGeneric extracts query from any format's "input" or "messages" field.
func extractSearchQueryGeneric(body []byte) string {
	// Try "input" array first (Responses format)
	input := gjson.GetBytes(body, "input")
	if input.IsArray() {
		for i := len(input.Array()) - 1; i >= 0; i-- {
			item := input.Array()[i]
			role := item.Get("role").String()
			if role == "user" {
				return extractTextFromContent(item.Get("content"))
			}
		}
	}

	// Fallback to "messages"
	return ExtractSearchQueryClaude(body)
}

// extractTextFromContent extracts plain text from a content field that may be
// a string or an array of content blocks.
func extractTextFromContent(content gjson.Result) string {
	if content.IsArray() {
		for _, block := range content.Array() {
			if block.Get("type").String() == "text" {
				return strings.TrimSpace(block.Get("text").String())
			}
		}
		return ""
	}
	return strings.TrimSpace(content.String())
}
