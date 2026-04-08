package websearch

import (
	"strings"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// StripWebSearchTool removes web_search tools from a request payload based on format.
// Only strips web_search variants — ToolSearch and WebFetch are preserved.
func StripWebSearchTool(body []byte, format string) []byte {
	switch format {
	case FormatClaude:
		return stripWebSearchToolClaude(body)
	case FormatOpenAI:
		return stripWebSearchToolOpenAI(body)
	case FormatCodexResponses:
		return stripWebSearchToolCodex(body)
	default:
		return stripWebSearchToolClaude(body)
	}
}

func stripWebSearchToolClaude(body []byte) []byte {
	tools := gjson.GetBytes(body, "tools")
	if !tools.IsArray() {
		return body
	}

	var remaining []string
	tools.ForEach(func(_, tool gjson.Result) bool {
		name := strings.ToLower(tool.Get("name").String())
		toolType := strings.ToLower(tool.Get("type").String())
		if !IsWebSearchToolName(name, toolType) {
			remaining = append(remaining, tool.Raw)
		}
		return true
	})

	if len(remaining) == 0 {
		body, _ = sjson.DeleteBytes(body, "tools")
		return body
	}

	newTools := "[" + strings.Join(remaining, ",") + "]"
	body, _ = sjson.SetRawBytes(body, "tools", []byte(newTools))
	return body
}

func stripWebSearchToolOpenAI(body []byte) []byte {
	tools := gjson.GetBytes(body, "tools")
	if !tools.IsArray() {
		return body
	}

	var remaining []string
	tools.ForEach(func(_, tool gjson.Result) bool {
		toolType := tool.Get("type").String()
		name := strings.ToLower(tool.Get("function.name").String())
		if toolType == "function" && IsWebSearchToolName(name, "") {
			return true // skip
		}
		rawType := strings.ToLower(tool.Get("type").String())
		if IsWebSearchToolName("", rawType) {
			return true // skip
		}
		remaining = append(remaining, tool.Raw)
		return true
	})

	if len(remaining) == 0 {
		body, _ = sjson.DeleteBytes(body, "tools")
		return body
	}

	newTools := "[" + strings.Join(remaining, ",") + "]"
	body, _ = sjson.SetRawBytes(body, "tools", []byte(newTools))
	return body
}

func stripWebSearchToolCodex(body []byte) []byte {
	return stripWebSearchToolClaude(body)
}
