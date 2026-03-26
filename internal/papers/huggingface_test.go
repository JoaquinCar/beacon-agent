package papers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

const hfJSON = `[
  {
    "paper": {
      "id": "2401.00001",
      "title": "HF Paper One",
      "summary": "Summary of HF paper.",
      "publishedAt": "2024-01-15T00:00:00.000Z",
      "authors": [{"name": "Alice"}, {"name": "Bob"}]
    }
  },
  {
    "paper": {
      "id": "2401.00002",
      "title": "HF Paper Two",
      "summary": "Another summary.",
      "publishedAt": "2024-01-14T00:00:00.000Z",
      "authors": [{"name": "Carol"}]
    }
  }
]`

func TestHuggingFaceFetcher_ParsesResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(hfJSON))
	}))
	defer srv.Close()

	f := NewHuggingFaceFetcher()
	f.apiBase = srv.URL

	papers, err := f.Fetch(context.Background(), "AI")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(papers) != 2 {
		t.Fatalf("expected 2 papers, got %d", len(papers))
	}

	p := papers[0]
	if p.Title != "HF Paper One" {
		t.Errorf("title: got %q", p.Title)
	}
	if len(p.Authors) != 2 || p.Authors[0] != "Alice" {
		t.Errorf("authors: got %v", p.Authors)
	}
	if p.Source != "huggingface" {
		t.Errorf("source: got %q, want huggingface", p.Source)
	}
	if p.URL != "https://arxiv.org/abs/2401.00001" {
		t.Errorf("url: got %q", p.URL)
	}
	if p.Date.Year() != 2024 || p.Date.Month() != 1 || p.Date.Day() != 15 {
		t.Errorf("date: got %v", p.Date)
	}
}

func TestHuggingFaceFetcher_NonOKStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	f := NewHuggingFaceFetcher()
	f.apiBase = srv.URL

	_, err := f.Fetch(context.Background(), "AI")
	if err == nil {
		t.Fatal("expected error for non-200 status")
	}
}

func TestHuggingFaceFetcher_RateLimited(t *testing.T) {
	for _, code := range []int{http.StatusTooManyRequests, http.StatusServiceUnavailable} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(code)
		}))

		f := NewHuggingFaceFetcher()
		f.apiBase = srv.URL

		_, err := f.Fetch(context.Background(), "AI")
		srv.Close()
		if err == nil {
			t.Errorf("status %d: expected rate-limit error", code)
		}
	}
}

func TestHuggingFaceFetcher_MalformedJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("not json"))
	}))
	defer srv.Close()

	f := NewHuggingFaceFetcher()
	f.apiBase = srv.URL

	_, err := f.Fetch(context.Background(), "AI")
	if err == nil {
		t.Fatal("expected error for malformed JSON")
	}
}

func TestHuggingFaceFetcher_EmptyList(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`[]`))
	}))
	defer srv.Close()

	f := NewHuggingFaceFetcher()
	f.apiBase = srv.URL

	papers, err := f.Fetch(context.Background(), "AI")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(papers) != 0 {
		t.Errorf("expected 0 papers, got %d", len(papers))
	}
}
