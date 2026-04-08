package websearch

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v6/internal/config"
	log "github.com/sirupsen/logrus"
)

// SearchOrchestrator coordinates SearXNG search and optional Perplexity enrichment.
type SearchOrchestrator struct {
	searxng    *SearXNGClient
	perplexity *PerplexityClient
	maxResults int
}

// NewSearchOrchestrator creates a new orchestrator from config.
func NewSearchOrchestrator(cfg *config.WebSearchConfig) *SearchOrchestrator {
	if cfg == nil {
		return nil
	}

	var perplexity *PerplexityClient
	if cfg.PerplexityEnabled && cfg.PerplexityAPIKey != "" {
		perplexity = NewPerplexityClient(cfg.PerplexityAPIKey, cfg.PerplexityBaseURL, nil)
	}

	return &SearchOrchestrator{
		searxng:    NewSearXNGClient(cfg.SearXNGURL, nil, cfg.MaxResults),
		perplexity: perplexity,
		maxResults: cfg.MaxResults,
	}
}

// Execute performs a search via SearXNG and optionally enriches with Perplexity.
// Always returns SearXNG results. Perplexity failure only affects SummaryHeader.
func (o *SearchOrchestrator) Execute(ctx context.Context, query SearchQuery) (*SearchResponse, error) {
	resp, err := o.searxng.Search(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("searxng search: %w", err)
	}

	// Optional Perplexity enrichment — errors are swallowed deliberately
	if o.perplexity != nil && len(resp.Results) > 0 {
		summary := o.perplexity.Summarize(ctx, query.Query, resp.Results)
		if summary != "" {
			resp.SummaryHeader = summary
			log.Debugf("websearch: perplexity summary enriched (%d chars)", len(summary))
		}
	}

	return resp, nil
}

// SearchAndFormat executes a search and returns both the structured response
// and a formatted text block suitable for injecting as a tool_result.
func (o *SearchOrchestrator) SearchAndFormat(ctx context.Context, query SearchQuery) (*SearchResponse, string, error) {
	resp, err := o.Execute(ctx, query)
	if err != nil {
		return nil, "", err
	}
	return resp, FormatToolResultText(resp), nil
}

// FormatToolResultText formats a SearchResponse as a text block for tool_result injection.
func FormatToolResultText(resp *SearchResponse) string {
	var sb strings.Builder

	if resp.SummaryHeader != "" {
		sb.WriteString("## Summary\n")
		sb.WriteString(resp.SummaryHeader)
		sb.WriteString("\n\n---\n\n")
	}

	sb.WriteString(fmt.Sprintf("Found %d search result(s) for query: \"%s\"\n\n", len(resp.Results), resp.Query))

	for i, r := range resp.Results {
		sb.WriteString(fmt.Sprintf("%d. **%s**\n", i+1, r.Title))
		sb.WriteString(fmt.Sprintf("   URL: %s\n", r.URL))
		if r.Snippet != "" {
			sb.WriteString(fmt.Sprintf("   %s\n", r.Snippet))
		}
		if r.Domain != "" {
			sb.WriteString(fmt.Sprintf("   Domain: %s\n", r.Domain))
		}
		sb.WriteString("\n")
	}

	return sb.String()
}

// FormatToolResultJSON formats a SearchResponse as compact JSON for tool_result injection.
func FormatToolResultJSON(resp *SearchResponse) string {
	data, err := json.Marshal(resp)
	if err != nil {
		return fmt.Sprintf(`{"error":"marshal failed: %s"}`, err.Error())
	}
	return string(data)
}

// Ensure orchestrator compiles.
var _ = time.Second
