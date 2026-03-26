package papers

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const arxivAPIBase = "https://export.arxiv.org/api/query"

// arXivFeed is the top-level Atom feed returned by the ArXiv API.
type arXivFeed struct {
	XMLName xml.Name     `xml:"http://www.w3.org/2005/Atom feed"`
	Entries []arXivEntry `xml:"http://www.w3.org/2005/Atom entry"`
}

type arXivEntry struct {
	ID        string        `xml:"http://www.w3.org/2005/Atom id"`
	Title     string        `xml:"http://www.w3.org/2005/Atom title"`
	Summary   string        `xml:"http://www.w3.org/2005/Atom summary"`
	Published string        `xml:"http://www.w3.org/2005/Atom published"`
	Authors   []arXivAuthor `xml:"http://www.w3.org/2005/Atom author"`
}

type arXivAuthor struct {
	Name string `xml:"http://www.w3.org/2005/Atom name"`
}

// ArXivFetcher fetches papers from a single ArXiv category.
type ArXivFetcher struct {
	httpClient *http.Client
	apiBase    string
	category   string
	maxResults int
}

// NewArXivFetcher creates a fetcher for the given ArXiv category (e.g. "cs.AI").
// Panics if the production API base URL is not HTTPS (caught at program init).
func NewArXivFetcher(category string) *ArXivFetcher {
	if err := validateHTTPS(arxivAPIBase); err != nil {
		panic("arxiv: " + err.Error())
	}
	return &ArXivFetcher{
		httpClient: &http.Client{Timeout: 15 * time.Second},
		apiBase:    arxivAPIBase,
		category:   category,
		maxResults: 10,
	}
}

// Fetch retrieves the most recent papers in the configured category.
// The topic parameter is recorded on each returned Paper for tagging.
func (f *ArXivFetcher) Fetch(ctx context.Context, topic string) ([]Paper, error) {
	slog.DebugContext(ctx, "arxiv: fetching papers", "category", f.category)

	params := url.Values{}
	params.Set("search_query", "cat:"+f.category)
	params.Set("start", "0")
	params.Set("max_results", fmt.Sprintf("%d", f.maxResults))
	params.Set("sortBy", "submittedDate")
	params.Set("sortOrder", "descending")

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, f.apiBase+"?"+params.Encode(), nil)
	if err != nil {
		return nil, fmt.Errorf("arxiv: build request: %w", err)
	}

	resp, err := f.httpClient.Do(req)
	if resp != nil {
		defer resp.Body.Close()
	}
	if err != nil {
		return nil, fmt.Errorf("arxiv: request: %w", err)
	}

	switch resp.StatusCode {
	case http.StatusOK:
		// proceed
	case http.StatusTooManyRequests, http.StatusServiceUnavailable:
		return nil, fmt.Errorf("arxiv: rate limited or unavailable (status %d)", resp.StatusCode)
	default:
		return nil, fmt.Errorf("arxiv: unexpected status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 5<<20))
	if err != nil {
		return nil, fmt.Errorf("arxiv: read body: %w", err)
	}

	var feed arXivFeed
	if err := xml.Unmarshal(body, &feed); err != nil {
		return nil, fmt.Errorf("arxiv: parse XML: %w", err)
	}

	papers := make([]Paper, 0, len(feed.Entries))
	for _, e := range feed.Entries {
		authors := make([]string, len(e.Authors))
		for i, a := range e.Authors {
			authors[i] = a.Name
		}

		pub, err := time.Parse(time.RFC3339, e.Published)
		if err != nil {
			pub = time.Time{}
		}

		papers = append(papers, Paper{
			Title:    strings.TrimSpace(e.Title),
			Authors:  authors,
			Source:   "arxiv",
			Topic:    topic,
			Date:     pub,
			URL:      strings.TrimSpace(e.ID),
			Abstract: strings.TrimSpace(e.Summary),
		})
	}

	slog.DebugContext(ctx, "arxiv: fetched papers", "count", len(papers))
	return papers, nil
}
