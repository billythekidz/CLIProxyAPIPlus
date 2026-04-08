package websearch

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	log "github.com/sirupsen/logrus"
)

// PerplexityClient calls the Perplexity API for optional summary enrichment.
// Errors are never propagated — Summarize returns empty string on any failure.
type PerplexityClient struct {
	apiKey     string
	baseURL    string
	httpClient *http.Client
}

// NewPerplexityClient creates a new Perplexity client.
func NewPerplexityClient(apiKey, baseURL string, httpClient *http.Client) *PerplexityClient {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 10 * time.Second}
	}
	return &PerplexityClient{
		apiKey:     apiKey,
		baseURL:    baseURL,
		httpClient: httpClient,
	}
}

// perplexityRequest is the payload sent to the Perplexity API.
type perplexityRequest struct {
	Model    string               `json:"model"`
	Messages []perplexityMessage  `json:"messages"`
}

// perplexityMessage is a single message in the Perplexity request.
type perplexityMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// perplexityResponse is the expected response from the Perplexity API.
type perplexityResponse struct {
	Choices []perplexityChoice `json:"choices"`
}

// perplexityChoice is a single choice in the Perplexity response.
type perplexityChoice struct {
	Message perplexityMessage `json:"message"`
}

// Summarize generates a concise summary of search results using Perplexity Sonar Pro.
// Returns empty string on any failure — errors are logged but never propagated.
func (c *PerplexityClient) Summarize(ctx context.Context, query string, results []SearchResult) string {
	if c.apiKey == "" {
		return ""
	}

	// Build context from search results
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Search query: %s\n\nResults:\n", query))
	for i, r := range results {
		sb.WriteString(fmt.Sprintf("%d. %s\n   %s\n   %s\n\n", i+1, r.Title, r.URL, r.Snippet))
	}

	prompt := fmt.Sprintf(
		"Provide a concise summary (2-3 paragraphs) synthesizing the following search results for the query \"%s\". "+
			"Focus on the most relevant and actionable information. Do not include citations or references.",
		query,
	)

	reqBody := perplexityRequest{
		Model: "sonar-pro",
		Messages: []perplexityMessage{
			{Role: "system", Content: "You are a helpful search result summarizer. Provide concise, accurate summaries."},
			{Role: "user", Content: fmt.Sprintf("%s\n\n%s", prompt, sb.String())},
		},
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		log.Debugf("websearch: perplexity marshal error: %v", err)
		return ""
	}

	url := strings.TrimRight(c.baseURL, "/") + "/chat/completions"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(bodyBytes))
	if err != nil {
		log.Debugf("websearch: perplexity request error: %v", err)
		return ""
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		log.Debugf("websearch: perplexity http error: %v", err)
		return ""
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		log.Debugf("websearch: perplexity api status %d: %s", resp.StatusCode, string(body))
		return ""
	}

	var pResp perplexityResponse
	if err := json.NewDecoder(resp.Body).Decode(&pResp); err != nil {
		log.Debugf("websearch: perplexity decode error: %v", err)
		return ""
	}

	if len(pResp.Choices) == 0 {
		return ""
	}

	summary := strings.TrimSpace(pResp.Choices[0].Message.Content)
	if summary == "" {
		return ""
	}

	return summary
}
