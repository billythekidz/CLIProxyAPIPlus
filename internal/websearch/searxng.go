package websearch

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	log "github.com/sirupsen/logrus"
)

const (
	searxngMaxRetries     = 2
	searxngBaseBackoffSec = 1
	searxngMaxBackoffSec  = 10
)

// Ensure constants are int.
var _ = searxngMaxRetries + searxngBaseBackoffSec + searxngMaxBackoffSec

// SearXNGClient executes search queries against a SearXNG instance.
type SearXNGClient struct {
	baseURL    string
	httpClient *http.Client
	maxResults int
}

// NewSearXNGClient creates a new SearXNG search client.
func NewSearXNGClient(baseURL string, httpClient *http.Client, maxResults int) *SearXNGClient {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 15 * time.Second}
	}
	if maxResults <= 0 {
		maxResults = 10
	}
	return &SearXNGClient{
		baseURL:    strings.TrimRight(baseURL, "/"),
		httpClient: httpClient,
		maxResults: maxResults,
	}
}

// Search executes a search query against SearXNG and returns normalized results.
func (c *SearXNGClient) Search(ctx context.Context, query SearchQuery) (*SearchResponse, error) {
	searchURL := c.buildSearchURL(query)

	var body []byte
	var statusCode int
	var err error

	for attempt := 0; attempt <= searxngMaxRetries; attempt++ {
		if attempt > 0 {
			backoffSec := searxngBaseBackoffSec * (1 << attempt)
			if backoffSec > searxngMaxBackoffSec {
				backoffSec = searxngMaxBackoffSec
			}
			backoff := time.Duration(backoffSec) * time.Second
			log.Debugf("searxng: retry %d after %v", attempt, backoff)
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(backoff):
			}
		}

		body, statusCode, err = c.doRequest(ctx, searchURL)
		if err != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			log.Debugf("searxng: request failed: %v", err)
			continue
		}

		if statusCode >= 500 {
			log.Debugf("searxng: server error %d, retrying", statusCode)
			continue
		}

		// Non-5xx response: don't retry
		break
	}

	if err != nil {
		return nil, fmt.Errorf("searxng: request failed after retries: %w", err)
	}

	if statusCode != http.StatusOK {
		return nil, fmt.Errorf("searxng: unexpected status %d", statusCode)
	}

	return c.parseResponse(body, query.Query)
}

func (c *SearXNGClient) buildSearchURL(query SearchQuery) string {
	q := query.Query

	// Apply domain filtering via site: operators
	for _, domain := range query.AllowedDomains {
		q += fmt.Sprintf(" site:%s", domain)
	}
	for _, domain := range query.BlockedDomains {
		q += fmt.Sprintf(" -site:%s", domain)
	}

	params := url.Values{}
	params.Set("q", q)
	params.Set("format", "json")
	params.Set("pageno", "1")

	return c.baseURL + "/search?" + params.Encode()
}

func (c *SearXNGClient) doRequest(ctx context.Context, searchURL string) ([]byte, int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, searchURL, nil)
	if err != nil {
		return nil, 0, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, fmt.Errorf("read body: %w", err)
	}

	return body, resp.StatusCode, nil
}

func (c *SearXNGClient) parseResponse(body []byte, originalQuery string) (*SearchResponse, error) {
	var raw searxngResponse
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("searxng: parse response: %w", err)
	}

	results := make([]SearchResult, 0, len(raw.Results))
	limit := c.maxResults
	if limit > len(raw.Results) {
		limit = len(raw.Results)
	}

	for i := 0; i < limit; i++ {
		r := raw.Results[i]
		sr := SearchResult{
			Title:  r.Title,
			URL:    r.URL,
			Snippet: r.Content,
			Domain: extractDomain(r.URL),
			Source: "searxng",
		}
		if r.PublishedDate != nil {
			sr.PublishedDate = r.PublishedDate
		}
		results = append(results, sr)
	}

	totalResults := len(raw.Results)
	if raw.NumberOfResults > 0 {
		totalResults = raw.NumberOfResults
	}

	return &SearchResponse{
		Results:      results,
		TotalResults: totalResults,
		Query:        originalQuery,
	}, nil
}

func extractDomain(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	return u.Hostname()
}

// searxngResponse maps the SearXNG JSON search API response.
type searxngResponse struct {
	Query           string `json:"query"`
	NumberOfResults int    `json:"number_of_results"`
	Results         []struct {
		Title         string `json:"title"`
		URL           string `json:"url"`
		Content       string `json:"content"`
		Engine        string `json:"engine"`
		PublishedDate *int64 `json:"publishedDate"`
		Score         string `json:"score"`
		Category      string `json:"category"`
	} `json:"results"`
	UnresponsiveEngines []interface{} `json:"unresponsive_engines"`
}
