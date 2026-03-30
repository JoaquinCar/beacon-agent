package papers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"
)

const hfAPIBase = "https://huggingface.co/api/daily_papers"

type hfResponse []hfItem

type hfItem struct {
	Paper hfPaper `json:"paper"`
}

type hfPaper struct {
	ID          string     `json:"id"`
	Title       string     `json:"title"`
	Summary     string     `json:"summary"`
	PublishedAt string     `json:"publishedAt"`
	Authors     []hfAuthor `json:"authors"`
}

type hfAuthor struct {
	Name string `json:"name"`
}

// HuggingFaceFetcher fetches today's papers from the HuggingFace daily papers feed.
type HuggingFaceFetcher struct {
	httpClient *http.Client
	apiBase    string
}

// NewHuggingFaceFetcher creates a HuggingFace daily papers fetcher.
// Panics if the production API base URL is not HTTPS (caught at program init).
func NewHuggingFaceFetcher() *HuggingFaceFetcher {
	if err := validateHTTPS(hfAPIBase); err != nil {
		panic("huggingface: " + err.Error())
	}
	return &HuggingFaceFetcher{
		httpClient: &http.Client{Timeout: 15 * time.Second},
		apiBase:    hfAPIBase,
	}
}

// Fetch retrieves today's papers from HuggingFace Papers.
func (f *HuggingFaceFetcher) Fetch(ctx context.Context, topic string) ([]Paper, error) {
	slog.DebugContext(ctx, "huggingface: fetching papers")

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, f.apiBase, nil)
	if err != nil {
		return nil, fmt.Errorf("huggingface: build request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "Beacon/1.0 (research aggregator)")

	resp, err := f.httpClient.Do(req)
	if resp != nil {
		defer resp.Body.Close()
	}
	if err != nil {
		return nil, fmt.Errorf("huggingface: request: %w", err)
	}

	switch resp.StatusCode {
	case http.StatusOK:
		// proceed
	case http.StatusTooManyRequests, http.StatusServiceUnavailable:
		return nil, fmt.Errorf("huggingface: rate limited or unavailable (status %d)", resp.StatusCode)
	default:
		return nil, fmt.Errorf("huggingface: unexpected status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 5<<20))
	if err != nil {
		return nil, fmt.Errorf("huggingface: read body: %w", err)
	}

	var items hfResponse
	if err := json.Unmarshal(body, &items); err != nil {
		return nil, fmt.Errorf("huggingface: parse JSON: %w", err)
	}

	papers := make([]Paper, 0, len(items))
	for _, item := range items {
		p := item.Paper

		authors := make([]string, len(p.Authors))
		for i, a := range p.Authors {
			authors[i] = a.Name
		}

		pub, err := time.Parse(time.RFC3339, p.PublishedAt)
		if err != nil {
			pub = time.Time{}
		}

		paperURL := "https://arxiv.org/abs/" + p.ID

		papers = append(papers, Paper{
			Title:    p.Title,
			Authors:  authors,
			Source:   "huggingface",
			Topic:    topic,
			Date:     pub,
			URL:      paperURL,
			Abstract: p.Summary,
		})
	}

	slog.DebugContext(ctx, "huggingface: fetched papers", "count", len(papers))
	return papers, nil
}
