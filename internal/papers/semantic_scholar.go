package papers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const ssAPIBase = "https://api.semanticscholar.org/graph/v1/paper/search"

type ssResponse struct {
	Data []ssPaper `json:"data"`
}

type ssPaper struct {
	PaperID     string      `json:"paperId"`
	Title       string      `json:"title"`
	Abstract    string      `json:"abstract"`
	Year        int         `json:"year"`
	Authors     []ssAuthor  `json:"authors"`
	ExternalIDs ssExternals `json:"externalIds"`
}

type ssAuthor struct {
	Name string `json:"name"`
}

type ssExternals struct {
	ArXiv string `json:"ArXiv"`
}

// SemanticScholarFetcher fetches papers from the Semantic Scholar search API.
type SemanticScholarFetcher struct {
	httpClient *http.Client
	apiBase    string
	query      string
	limit      int
}

// NewSemanticScholarFetcher creates a fetcher with the given search query.
func NewSemanticScholarFetcher(query string) *SemanticScholarFetcher {
	return &SemanticScholarFetcher{
		httpClient: &http.Client{Timeout: 20 * time.Second},
		apiBase:    ssAPIBase,
		query:      query,
		limit:      10,
	}
}

// Fetch queries Semantic Scholar for papers matching the configured query.
func (f *SemanticScholarFetcher) Fetch(ctx context.Context, topic string) ([]Paper, error) {
	params := url.Values{}
	params.Set("query", f.query)
	params.Set("fields", "title,authors,year,externalIds,abstract")
	params.Set("limit", strconv.Itoa(f.limit))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, f.apiBase+"?"+params.Encode(), nil)
	if err != nil {
		return nil, fmt.Errorf("semantic_scholar: build request: %w", err)
	}

	resp, err := f.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("semantic_scholar: request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("semantic_scholar: unexpected status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 5<<20))
	if err != nil {
		return nil, fmt.Errorf("semantic_scholar: read body: %w", err)
	}

	var result ssResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("semantic_scholar: parse JSON: %w", err)
	}

	papers := make([]Paper, 0, len(result.Data))
	for _, p := range result.Data {
		authors := make([]string, len(p.Authors))
		for i, a := range p.Authors {
			authors[i] = a.Name
		}

		// Approximate date: January 1 of the publication year.
		var pub time.Time
		if p.Year > 0 {
			pub = time.Date(p.Year, time.January, 1, 0, 0, 0, 0, time.UTC)
		}

		// Prefer ArXiv URL if available.
		paperURL := "https://www.semanticscholar.org/paper/" + p.PaperID
		if p.ExternalIDs.ArXiv != "" {
			paperURL = "https://arxiv.org/abs/" + p.ExternalIDs.ArXiv
		}

		papers = append(papers, Paper{
			Title:    strings.TrimSpace(p.Title),
			Authors:  authors,
			Source:   "semantic_scholar",
			Topic:    topic,
			Date:     pub,
			URL:      paperURL,
			Abstract: strings.TrimSpace(p.Abstract),
		})
	}

	return papers, nil
}
