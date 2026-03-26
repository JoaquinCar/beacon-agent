package papers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const biorxivJSON = `{
  "messages": [{"status": "ok", "count": 2, "total": 2}],
  "collection": [
    {
      "doi": "10.1101/2024.01.15.000001",
      "title": "BioRxiv Paper One",
      "authors": "Alice Smith, Bob Jones",
      "date": "2024-01-15",
      "abstract": "Bio abstract one."
    },
    {
      "doi": "10.1101/2024.01.14.000002",
      "title": "BioRxiv Paper Two",
      "authors": "Carol White",
      "date": "2024-01-14",
      "abstract": "Bio abstract two."
    }
  ]
}`

func TestBioRxivFetcher_ParsesResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(biorxivJSON))
	}))
	defer srv.Close()

	f := NewBioRxivFetcher("biorxiv")
	f.apiBase = srv.URL

	papers, err := f.Fetch(context.Background(), "BIO")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(papers) != 2 {
		t.Fatalf("expected 2 papers, got %d", len(papers))
	}

	p := papers[0]
	if p.Title != "BioRxiv Paper One" {
		t.Errorf("title: got %q", p.Title)
	}
	if len(p.Authors) != 2 || p.Authors[0] != "Alice Smith" {
		t.Errorf("authors: got %v", p.Authors)
	}
	if p.Source != "biorxiv" {
		t.Errorf("source: got %q", p.Source)
	}
	if p.URL != "https://doi.org/10.1101/2024.01.15.000001" {
		t.Errorf("url: got %q", p.URL)
	}
	if p.Date.Year() != 2024 || p.Date.Month() != 1 || p.Date.Day() != 15 {
		t.Errorf("date: got %v", p.Date)
	}
}

func TestBioRxivFetcher_URLContainsDateRange(t *testing.T) {
	var capturedURL string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedURL = r.URL.Path
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"collection":[]}`))
	}))
	defer srv.Close()

	f := NewBioRxivFetcher("biorxiv")
	f.apiBase = srv.URL

	_, err := f.Fetch(context.Background(), "BIO")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Path should be /biorxiv/YYYY-MM-DD/YYYY-MM-DD/0/json
	if !strings.HasPrefix(capturedURL, "/biorxiv/") {
		t.Errorf("path should start with /biorxiv/, got %q", capturedURL)
	}
	if !strings.HasSuffix(capturedURL, "/0/json") {
		t.Errorf("path should end with /0/json, got %q", capturedURL)
	}
}

func TestBioRxivFetcher_LimitCaps(t *testing.T) {
	// Build a response with 20 papers.
	var items []string
	for i := range 20 {
		items = append(items, `{"doi":"10.1101/x.`+string(rune('a'+i))+`","title":"T","authors":"A","date":"2024-01-01","abstract":"B"}`)
	}
	payload := `{"collection":[` + strings.Join(items, ",") + `]}`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(payload))
	}))
	defer srv.Close()

	f := NewBioRxivFetcher("biorxiv")
	f.apiBase = srv.URL
	f.limit = 5

	papers, err := f.Fetch(context.Background(), "BIO")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(papers) != 5 {
		t.Errorf("expected limit of 5, got %d", len(papers))
	}
}

func TestBioRxivFetcher_NonOKStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer srv.Close()

	f := NewBioRxivFetcher("biorxiv")
	f.apiBase = srv.URL

	_, err := f.Fetch(context.Background(), "BIO")
	if err == nil {
		t.Fatal("expected error for non-200 status")
	}
}

func TestBioRxivFetcher_RateLimited(t *testing.T) {
	for _, code := range []int{http.StatusTooManyRequests, http.StatusServiceUnavailable} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(code)
		}))

		f := NewBioRxivFetcher("biorxiv")
		f.apiBase = srv.URL

		_, err := f.Fetch(context.Background(), "BIO")
		srv.Close()
		if err == nil {
			t.Errorf("status %d: expected rate-limit error", code)
		}
	}
}

func TestSplitAuthors(t *testing.T) {
	cases := []struct {
		input string
		want  []string
	}{
		{"Alice Smith, Bob Jones", []string{"Alice Smith", "Bob Jones"}},
		{"Solo Author", []string{"Solo Author"}},
		{"", []string{}},
		{"  Alice ,  Bob  ", []string{"Alice", "Bob"}},
	}
	for _, tc := range cases {
		got := splitAuthors(tc.input)
		if len(got) != len(tc.want) {
			t.Errorf("splitAuthors(%q): got %v, want %v", tc.input, got, tc.want)
			continue
		}
		for i := range got {
			if got[i] != tc.want[i] {
				t.Errorf("splitAuthors(%q)[%d]: got %q, want %q", tc.input, i, got[i], tc.want[i])
			}
		}
	}
}
