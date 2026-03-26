package papers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

const biorxivAPIBase = "https://api.biorxiv.org/details"

type biorxivResponse struct {
	Collection []biorxivPaper `json:"collection"`
}

type biorxivPaper struct {
	DOI      string `json:"doi"`
	Title    string `json:"title"`
	Authors  string `json:"authors"` // comma-separated string, not an array
	Date     string `json:"date"`    // "YYYY-MM-DD"
	Abstract string `json:"abstract"`
}

// BioRxivFetcher fetches recent preprints from bioRxiv or medRxiv.
type BioRxivFetcher struct {
	httpClient *http.Client
	apiBase    string
	server     string // "biorxiv" or "medrxiv"
	daysBack   int
	limit      int
}

// NewBioRxivFetcher creates a fetcher for the given server ("biorxiv" or "medrxiv").
// Panics if the production API base URL is not HTTPS (caught at program init).
func NewBioRxivFetcher(server string) *BioRxivFetcher {
	if err := validateHTTPS(biorxivAPIBase); err != nil {
		panic("biorxiv: " + err.Error())
	}
	return &BioRxivFetcher{
		httpClient: &http.Client{Timeout: 20 * time.Second},
		apiBase:    biorxivAPIBase,
		server:     server,
		daysBack:   7,
		limit:      10,
	}
}

// Fetch retrieves preprints submitted in the last daysBack days.
func (f *BioRxivFetcher) Fetch(ctx context.Context, topic string) ([]Paper, error) {
	slog.DebugContext(ctx, "biorxiv: fetching papers", "server", f.server)

	now := time.Now().UTC()
	start := now.AddDate(0, 0, -f.daysBack)
	interval := start.Format("2006-01-02") + "/" + now.Format("2006-01-02")

	reqURL := fmt.Sprintf("%s/%s/%s/0/json", f.apiBase, f.server, interval)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("biorxiv: build request: %w", err)
	}

	resp, err := f.httpClient.Do(req)
	if resp != nil {
		defer resp.Body.Close()
	}
	if err != nil {
		return nil, fmt.Errorf("biorxiv: request: %w", err)
	}

	switch resp.StatusCode {
	case http.StatusOK:
		// proceed
	case http.StatusTooManyRequests, http.StatusServiceUnavailable:
		return nil, fmt.Errorf("biorxiv: rate limited or unavailable (status %d)", resp.StatusCode)
	default:
		return nil, fmt.Errorf("biorxiv: unexpected status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 5<<20))
	if err != nil {
		return nil, fmt.Errorf("biorxiv: read body: %w", err)
	}

	var result biorxivResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("biorxiv: parse JSON: %w", err)
	}

	// Cap results at limit to avoid overwhelming the output.
	entries := result.Collection
	if len(entries) > f.limit {
		entries = entries[:f.limit]
	}

	papers := make([]Paper, 0, len(entries))
	for _, p := range entries {
		authors := splitAuthors(p.Authors)

		pub, err := time.Parse("2006-01-02", p.Date)
		if err != nil {
			pub = time.Time{}
		}

		papers = append(papers, Paper{
			Title:    strings.TrimSpace(p.Title),
			Authors:  authors,
			Source:   f.server,
			Topic:    topic,
			Date:     pub,
			URL:      "https://doi.org/" + p.DOI,
			DOI:      p.DOI,
			Abstract: strings.TrimSpace(p.Abstract),
		})
	}

	slog.DebugContext(ctx, "biorxiv: fetched papers", "count", len(papers))
	return papers, nil
}

// splitAuthors parses a comma-separated author string into a slice.
func splitAuthors(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if trimmed := strings.TrimSpace(p); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}
