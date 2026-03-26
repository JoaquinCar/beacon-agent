package papers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

const ssJSON = `{
  "data": [
    {
      "paperId": "abc123",
      "title": "SS Paper One",
      "abstract": "Abstract one.",
      "year": 2024,
      "authors": [{"name": "Alice"}, {"name": "Bob"}],
      "externalIds": {"ArXiv": "2401.00001"}
    },
    {
      "paperId": "def456",
      "title": "SS Paper Two",
      "abstract": "Abstract two.",
      "year": 2023,
      "authors": [{"name": "Carol"}],
      "externalIds": {}
    }
  ]
}`

func TestSemanticScholarFetcher_ParsesResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(ssJSON))
	}))
	defer srv.Close()

	f := NewSemanticScholarFetcher("AI")
	f.apiBase = srv.URL

	papers, err := f.Fetch(context.Background(), "AI")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(papers) != 2 {
		t.Fatalf("expected 2 papers, got %d", len(papers))
	}

	p := papers[0]
	if p.Title != "SS Paper One" {
		t.Errorf("title: got %q", p.Title)
	}
	// Paper with ArXiv ID should use arxiv URL.
	if p.URL != "https://arxiv.org/abs/2401.00001" {
		t.Errorf("url[0]: got %q", p.URL)
	}
	if p.Date.Year() != 2024 {
		t.Errorf("date year: got %d", p.Date.Year())
	}
	if p.Source != "semantic_scholar" {
		t.Errorf("source: got %q", p.Source)
	}

	// Paper without ArXiv ID should use semanticscholar URL.
	if papers[1].URL != "https://www.semanticscholar.org/paper/def456" {
		t.Errorf("url[1]: got %q", papers[1].URL)
	}
	if papers[1].Date.Year() != 2023 {
		t.Errorf("date year[1]: got %d", papers[1].Date.Year())
	}
}

func TestSemanticScholarFetcher_NonOKStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()

	f := NewSemanticScholarFetcher("AI")
	f.apiBase = srv.URL

	_, err := f.Fetch(context.Background(), "AI")
	if err == nil {
		t.Fatal("expected error for non-200 status")
	}
}

func TestSemanticScholarFetcher_EmptyData(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"data":[]}`))
	}))
	defer srv.Close()

	f := NewSemanticScholarFetcher("AI")
	f.apiBase = srv.URL

	papers, err := f.Fetch(context.Background(), "AI")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(papers) != 0 {
		t.Errorf("expected 0 papers, got %d", len(papers))
	}
}

func TestSemanticScholarFetcher_ZeroYear(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"data":[{"paperId":"x","title":"T","year":0,"authors":[],"externalIds":{}}]}`))
	}))
	defer srv.Close()

	f := NewSemanticScholarFetcher("AI")
	f.apiBase = srv.URL

	papers, err := f.Fetch(context.Background(), "AI")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !papers[0].Date.IsZero() {
		t.Errorf("expected zero date for year=0, got %v", papers[0].Date)
	}
}
