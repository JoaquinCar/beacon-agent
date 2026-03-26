package papers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
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
func NewHuggingFaceFetcher() *HuggingFaceFetcher {
	return &HuggingFaceFetcher{
		httpClient: &http.Client{Timeout: 15 * time.Second},
		apiBase:    hfAPIBase,
	}
}

// Fetch retrieves today's papers from HuggingFace Papers.
func (f *HuggingFaceFetcher) Fetch(ctx context.Context, topic string) ([]Paper, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, f.apiBase, nil)
	if err != nil {
		return nil, fmt.Errorf("huggingface: build request: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := f.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("huggingface: request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
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

	return papers, nil
}
