package websearch

import (
	"context"
	"fmt"

	"github.com/router-for-me/CLIProxyAPI/v6/internal/config"
)

// InterceptResult holds the outcome of a web search interception.
type InterceptResult struct {
	// ModifiedBody is the request payload with search results injected
	// and (when override-builtin is enabled) native web_search tools stripped.
	ModifiedBody []byte

	// SearchResponse contains the raw search results from SearXNG.
	SearchResponse *SearchResponse

	// Format is the detected request format (claude, openai, codex, etc.).
	Format string

	// Query is the extracted search query.
	Query string

	// CallID is the tool call/use ID generated for injection.
	CallID string
}

// ShouldIntercept checks whether a request should be intercepted for proxy-managed web search.
// Returns (format, query, true) when interception is needed.
func ShouldIntercept(cfg *config.Config, model string, body []byte, format string) (string, string, bool) {
	if cfg == nil || !cfg.WebSearch.Enabled {
		return "", "", false
	}
	if !cfg.IsWebSearchEnabledForModel(model) {
		return "", "", false
	}

	detectedFormat, query, hasTool := DetectFormatAndQuery(body)
	if !hasTool || query == "" {
		return "", "", false
	}

	// When override-builtin is off, only intercept if the tool would not be
	// handled natively by the provider. Since we detect web_search tool presence,
	// override-builtin controls whether we strip the native tool first.
	return detectedFormat, query, true
}

// InterceptRequest performs the full interception pipeline:
// 1. Search via SearXNG (orchestrator)
// 2. Strip native web_search tools if override-builtin is enabled
// 3. Inject search results into the payload in the correct format
func InterceptRequest(ctx context.Context, cfg *config.Config, body []byte, format string, model string) (*InterceptResult, error) {
	detectedFormat, query, shouldIntercept := ShouldIntercept(cfg, model, body, format)
	if !shouldIntercept {
		return nil, nil
	}

	orchestrator := NewSearchOrchestrator(&cfg.WebSearch)
	if orchestrator == nil {
		return nil, fmt.Errorf("websearch: orchestrator not available")
	}

	searchQuery := SearchQuery{Query: query}
	resp, err := orchestrator.Execute(ctx, searchQuery)
	if err != nil {
		return nil, fmt.Errorf("websearch: search failed: %w", err)
	}

	if resp == nil || len(resp.Results) == 0 {
		return nil, nil // No results, let request pass through
	}

	// Strip native web_search tools when override is enabled
	if cfg.WebSearch.OverrideBuiltin {
		body = StripWebSearchTool(body, detectedFormat)
	}

	// Generate a call ID based on format
	callID := GenerateToolUseID()

	// Inject search results based on format
	var injectErr error
	switch detectedFormat {
	case FormatClaude:
		body, injectErr = InjectToolResultsClaude(body, callID, query, resp)
	case FormatOpenAI:
		body, injectErr = InjectToolResultsOpenAI(body, callID, query, resp)
	case FormatCodexResponses:
		body, injectErr = InjectToolResultsCodexResponses(body, callID, query, resp)
	default:
		body, injectErr = InjectToolResultsClaude(body, callID, query, resp)
	}

	if injectErr != nil {
		return nil, fmt.Errorf("websearch: injection failed: %w", injectErr)
	}

	return &InterceptResult{
		ModifiedBody:   body,
		SearchResponse: resp,
		Format:         detectedFormat,
		Query:          query,
		CallID:         callID,
	}, nil
}
