package briefing

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/joako/beacon/internal/papers"
)

// claudeResp builds a minimal Anthropic API response with the given text.
func claudeResp(text string) []byte {
	r := anthropicResponse{
		Content: []anthropicContent{{Type: "text", Text: text}},
	}
	b, _ := json.Marshal(r)
	return b
}

func makePaperForGen(title string) papers.Paper {
	return papers.Paper{
		Title:    title,
		Authors:  []string{"Alice Smith", "Bob Jones"},
		Source:   "arxiv",
		Abstract: "Abstract text.",
		URL:      "https://arxiv.org/abs/1",
		Date:     time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC),
	}
}

func TestGenerator_SummaryMode_ReturnsBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(claudeResp("Line 1.\nLine 2.\nLine 3.\nLine 4.\nLine 5."))
	}))
	defer srv.Close()

	g := NewGenerator("test-key")
	g.apiBase = srv.URL
	g.paperDelay = 0

	b, err := g.Generate(context.Background(), []papers.Paper{makePaperForGen("P1")}, ModeSummary)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if len(b.Sections) != 1 {
		t.Fatalf("expected 1 section, got %d", len(b.Sections))
	}
	if b.Sections[0].TLDr != "" {
		t.Error("summary mode should not have TLDr")
	}
	if !strings.Contains(b.Sections[0].Body, "Line 1.") {
		t.Errorf("body missing: %s", b.Sections[0].Body)
	}
}

func TestGenerator_FullMode_ParsesTLDr(t *testing.T) {
	response := "TL;DR: This paper introduces X.\n\nMotivation: Y\nMethod: Z"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(claudeResp(response))
	}))
	defer srv.Close()

	g := NewGenerator("test-key")
	g.apiBase = srv.URL
	g.paperDelay = 0

	b, err := g.Generate(context.Background(), []papers.Paper{makePaperForGen("P1")}, ModeFull)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	sec := b.Sections[0]
	if sec.TLDr != "This paper introduces X." {
		t.Errorf("TLDr: got %q", sec.TLDr)
	}
	if !strings.Contains(sec.Body, "Motivation: Y") {
		t.Errorf("body: got %q", sec.Body)
	}
}

func TestGenerator_RetryOn429(t *testing.T) {
	attempts := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attempts++
		if attempts < 3 {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(claudeResp("summary text"))
	}))
	defer srv.Close()

	g := NewGenerator("test-key")
	g.apiBase = srv.URL
	g.paperDelay = 0

	b, err := g.Generate(context.Background(), []papers.Paper{makePaperForGen("P1")}, ModeSummary)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if attempts != 3 {
		t.Errorf("expected 3 attempts (2 retries), got %d", attempts)
	}
	if len(b.Sections) != 1 {
		t.Errorf("expected 1 section after retry, got %d", len(b.Sections))
	}
}

func TestGenerator_MaxRetriesExceeded_SkipsPaper(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	g := NewGenerator("test-key")
	g.apiBase = srv.URL
	g.paperDelay = 0

	// With 2 papers, one always fails — should still get the second (if it passes)
	// Here both fail, so we expect error "all papers failed"
	b, err := g.Generate(context.Background(), []papers.Paper{makePaperForGen("P1")}, ModeSummary)
	if err == nil {
		t.Fatal("expected error when all papers fail")
	}
	if len(b.Sections) != 0 {
		t.Errorf("expected 0 sections, got %d", len(b.Sections))
	}
}

func TestGenerator_OneFailOneSucceeds_SkipsFailed(t *testing.T) {
	call := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		call++
		// First paper: always 503 (fails after 3 retries)
		// Second paper: success
		// Each paper attempt is tracked; first paper gets 3 tries before second starts
		if call <= maxRetries {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(claudeResp("good summary"))
	}))
	defer srv.Close()

	g := NewGenerator("test-key")
	g.apiBase = srv.URL
	g.paperDelay = 0

	ps := []papers.Paper{makePaperForGen("Fail"), makePaperForGen("Pass")}
	b, err := g.Generate(context.Background(), ps, ModeSummary)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if len(b.Sections) != 1 {
		t.Errorf("expected 1 section (skip failed, keep good), got %d", len(b.Sections))
	}
	if b.Sections[0].Paper.Title != "Pass" {
		t.Errorf("expected second paper to succeed, got %q", b.Sections[0].Paper.Title)
	}
}

func TestGenerator_NonRetryableError_NoRetry(t *testing.T) {
	attempts := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attempts++
		w.WriteHeader(http.StatusUnauthorized) // 401 — not retryable
	}))
	defer srv.Close()

	g := NewGenerator("test-key")
	g.apiBase = srv.URL
	g.paperDelay = 0

	_, err := g.Generate(context.Background(), []papers.Paper{makePaperForGen("P1")}, ModeSummary)
	if err == nil {
		t.Fatal("expected error for 401")
	}
	if attempts != 1 {
		t.Errorf("expected 1 attempt for non-retryable error, got %d", attempts)
	}
}

func TestGenerator_ContextCancelledDuringDelay(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(claudeResp("ok"))
	}))
	defer srv.Close()

	g := NewGenerator("test-key")
	g.apiBase = srv.URL
	g.paperDelay = 100 * time.Millisecond

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	ps := []papers.Paper{makePaperForGen("P1"), makePaperForGen("P2")}
	b, err := g.Generate(ctx, ps, ModeSummary)
	// First paper should succeed; context cancelled during the 100ms delay before second
	if err == nil {
		t.Fatal("expected context cancellation error")
	}
	if len(b.Sections) != 1 {
		t.Errorf("expected 1 section before cancel, got %d", len(b.Sections))
	}
}

func TestParseTLDr_WithPrefix(t *testing.T) {
	tldr, body := parseTLDr("TL;DR: Sentence here.\n\nMotivation: some stuff")
	if tldr != "Sentence here." {
		t.Errorf("tldr: got %q", tldr)
	}
	if body != "Motivation: some stuff" {
		t.Errorf("body: got %q", body)
	}
}

func TestParseTLDr_WithoutPrefix(t *testing.T) {
	tldr, body := parseTLDr("Some analysis without TL;DR prefix")
	if tldr != "" {
		t.Errorf("expected empty tldr, got %q", tldr)
	}
	if body != "Some analysis without TL;DR prefix" {
		t.Errorf("body: got %q", body)
	}
}

func TestGenerator_EmptyPapers(t *testing.T) {
	g := NewGenerator("test-key")
	b, err := g.Generate(context.Background(), nil, ModeSummary)
	if err != nil {
		t.Fatalf("unexpected error for empty input: %v", err)
	}
	if len(b.Sections) != 0 {
		t.Errorf("expected 0 sections for empty input, got %d", len(b.Sections))
	}
}

func TestGenerator_SetsMode(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(claudeResp("text"))
	}))
	defer srv.Close()

	g := NewGenerator("test-key")
	g.apiBase = srv.URL
	g.paperDelay = 0

	b, _ := g.Generate(context.Background(), []papers.Paper{makePaperForGen("P")}, ModeFull)
	if b.Mode != ModeFull {
		t.Errorf("mode: got %v, want ModeFull", b.Mode)
	}
}

func TestGenerator_SetsGeneratedAt(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(claudeResp("text"))
	}))
	defer srv.Close()

	g := NewGenerator("test-key")
	g.apiBase = srv.URL
	g.paperDelay = 0

	before := time.Now()
	b, _ := g.Generate(context.Background(), []papers.Paper{makePaperForGen("P")}, ModeSummary)
	if b.GeneratedAt.Before(before) {
		t.Error("GeneratedAt should be set to current time")
	}
}
