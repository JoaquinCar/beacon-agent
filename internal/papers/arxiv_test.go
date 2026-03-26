package papers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

const arxivXML = `<?xml version="1.0" encoding="UTF-8"?>
<feed xmlns="http://www.w3.org/2005/Atom">
  <entry>
    <id>https://arxiv.org/abs/2401.00001v1</id>
    <title>Test Paper One</title>
    <summary>Abstract of paper one.</summary>
    <published>2024-01-15T00:00:00Z</published>
    <author><name>Alice Smith</name></author>
    <author><name>Bob Jones</name></author>
  </entry>
  <entry>
    <id>https://arxiv.org/abs/2401.00002v1</id>
    <title>  Test Paper Two  </title>
    <summary>  Abstract of paper two.  </summary>
    <published>2024-01-14T00:00:00Z</published>
    <author><name>Carol White</name></author>
  </entry>
</feed>`

func TestArXivFetcher_ParsesFeed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/atom+xml")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(arxivXML))
	}))
	defer srv.Close()

	f := NewArXivFetcher("cs.AI")
	f.apiBase = srv.URL

	papers, err := f.Fetch(context.Background(), "AI")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(papers) != 2 {
		t.Fatalf("expected 2 papers, got %d", len(papers))
	}

	p := papers[0]
	if p.Title != "Test Paper One" {
		t.Errorf("title: got %q, want %q", p.Title, "Test Paper One")
	}
	if len(p.Authors) != 2 || p.Authors[0] != "Alice Smith" {
		t.Errorf("authors: got %v", p.Authors)
	}
	if p.Source != "arxiv" {
		t.Errorf("source: got %q, want arxiv", p.Source)
	}
	if p.Topic != "AI" {
		t.Errorf("topic: got %q, want AI", p.Topic)
	}
	if p.URL != "https://arxiv.org/abs/2401.00001v1" {
		t.Errorf("url: got %q", p.URL)
	}
	if p.Date.Year() != 2024 || p.Date.Month() != 1 || p.Date.Day() != 15 {
		t.Errorf("date: got %v", p.Date)
	}

	// Whitespace trimming on second entry.
	if papers[1].Title != "Test Paper Two" {
		t.Errorf("title[1]: got %q", papers[1].Title)
	}
	if papers[1].Abstract != "Abstract of paper two." {
		t.Errorf("abstract[1]: got %q", papers[1].Abstract)
	}
}

func TestArXivFetcher_NonOKStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	f := NewArXivFetcher("cs.AI")
	f.apiBase = srv.URL

	_, err := f.Fetch(context.Background(), "AI")
	if err == nil {
		t.Fatal("expected error for non-200 status")
	}
}

func TestArXivFetcher_RateLimited(t *testing.T) {
	for _, code := range []int{http.StatusTooManyRequests, http.StatusServiceUnavailable} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(code)
		}))

		f := NewArXivFetcher("cs.AI")
		f.apiBase = srv.URL

		_, err := f.Fetch(context.Background(), "AI")
		srv.Close()
		if err == nil {
			t.Errorf("status %d: expected rate-limit error", code)
		}
	}
}

func TestArXivFetcher_MalformedXML(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("not xml"))
	}))
	defer srv.Close()

	f := NewArXivFetcher("cs.AI")
	f.apiBase = srv.URL

	_, err := f.Fetch(context.Background(), "AI")
	if err == nil {
		t.Fatal("expected error for malformed XML")
	}
}

func TestArXivFetcher_EmptyFeed(t *testing.T) {
	empty := `<?xml version="1.0" encoding="UTF-8"?>
<feed xmlns="http://www.w3.org/2005/Atom"></feed>`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(empty))
	}))
	defer srv.Close()

	f := NewArXivFetcher("cs.AI")
	f.apiBase = srv.URL

	papers, err := f.Fetch(context.Background(), "AI")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(papers) != 0 {
		t.Errorf("expected 0 papers, got %d", len(papers))
	}
}
