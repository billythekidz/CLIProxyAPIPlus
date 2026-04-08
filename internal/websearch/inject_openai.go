package websearch

import (
	"fmt"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// InjectToolResultsOpenAI modifies an OpenAI chat-completions request payload
// to inject search results as a function tool call + tool role message.
func InjectToolResultsOpenAI(body []byte, callID, query string, resp *SearchResponse) ([]byte, error) {
	if resp == nil || len(resp.Results) == 0 {
		return body, nil
	}

	resultText := FormatToolResultText(resp)

	// Append assistant message with tool_calls
	assistantMsg := fmt.Sprintf(
		`{"role":"assistant","content":null,"tool_calls":[{"id":"%s","type":"function","function":{"name":"web_search","arguments":%s}}]}`,
		callID, mustMarshal(fmt.Sprintf(`{"query":%s}`, mustMarshal(query))),
	)
	body, _ = sjson.SetRawBytes(body, "messages.-1", []byte(assistantMsg))

	// Append tool role message with result
	toolMsg := fmt.Sprintf(
		`{"role":"tool","tool_call_id":"%s","content":%s}`,
		callID, mustMarshal(resultText),
	)
	body, _ = sjson.SetRawBytes(body, "messages.-1", []byte(toolMsg))

	return body, nil
}

// InjectToolResultsCodexResponses modifies a Codex Responses request payload
// to inject search results. Codex Responses uses "input" instead of "messages"
// and expects web_search output items.
func InjectToolResultsCodexResponses(body []byte, callID, query string, resp *SearchResponse) ([]byte, error) {
	if resp == nil || len(resp.Results) == 0 {
		return body, nil
	}

	resultText := FormatToolResultText(resp)

	// For Codex Responses, we append the search result as a function_call_output input item
	outputItem := fmt.Sprintf(
		`{"type":"function_call_output","call_id":"%s","output":%s}`,
		callID, mustMarshal(resultText),
	)

	input := gjson.GetBytes(body, "input")
	if input.IsArray() {
		body, _ = sjson.SetRawBytes(body, "input.-1", []byte(outputItem))
	} else {
		body, _ = sjson.SetRawBytes(body, "input", []byte("["+outputItem+"]"))
	}

	return body, nil
}

// GenerateSearchIndicatorEventsOpenAI generates SSE events for OpenAI streaming.
// Produces function_call + function_call_output events.
func GenerateSearchIndicatorEventsOpenAI(callID, query string, resp *SearchResponse, startIndex int) [][]byte {
	if resp == nil {
		return nil
	}

	var events [][]byte
	resultText := FormatToolResultText(resp)

	// function_call_start
	events = append(events, []byte(fmt.Sprintf(
		"event: response.function_call_arguments.delta\ndata: {\"type\":\"function_call\",\"id\":\"%s\",\"call_id\":\"%s\",\"name\":\"web_search\",\"arguments\":%s}\n\n",
		callID, callID, mustMarshal(fmt.Sprintf(`{"query":%s}`, mustMarshal(query))),
	)))

	// function_call_output
	events = append(events, []byte(fmt.Sprintf(
		"event: response.function_call_output.done\ndata: {\"type\":\"function_call_output\",\"call_id\":\"%s\",\"output\":%s}\n\n",
		callID, mustMarshal(resultText),
	)))

	return events
}
