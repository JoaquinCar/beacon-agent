package papers

import (
	"context"
	"errors"
	"slices"
	"sync/atomic"
	"testing"
	"time"
)

// mockFetcher is a test double for SourceFetcher.
type mockFetcher struct {
	papers  []Paper
	err     error
	callCnt *atomic.Int32
}

func (m *mockFetcher) Fetch(_ context.Context, topic string) ([]Paper, error) {
	if m.callCnt != nil {
		m.callCnt.Add(1)
	}
	if m.err != nil {
		return nil, m.err
	}
	out := make([]Paper, len(m.papers))
	copy(out, m.papers)
	for i := range out {
		out[i].Topic = topic
	}
	return out, nil
}

func makePaper(title, source string) Paper {
	return Paper{Title: title, Source: source, Date: time.Now()}
}

func TestFetcher_CombinesMultipleSources(t *testing.T) {
	f := &fetcher{
		sources: map[string][]SourceFetcher{
			"TEST": {
				&mockFetcher{papers: []Paper{makePaper("Paper A", "arxiv")}},
				&mockFetcher{papers: []Paper{makePaper("Paper B", "huggingface")}},
			},
		},
	}

	papers, err := f.FetchTopic(context.Background(), "test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(papers) != 2 {
		t.Fatalf("expected 2 papers, got %d", len(papers))
	}
}

func TestFetcher_DeduplicatesByTitle(t *testing.T) {
	duplicate := makePaper("Same Title", "arxiv")
	f := &fetcher{
		sources: map[string][]SourceFetcher{
			"TEST": {
				&mockFetcher{papers: []Paper{duplicate}},
				&mockFetcher{papers: []Paper{duplicate}}, // same title, different source
			},
		},
	}

	papers, err := f.FetchTopic(context.Background(), "test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(papers) != 1 {
		t.Errorf("expected deduplication to 1 paper, got %d", len(papers))
	}
}

func TestFetcher_SourceErrorSkipped(t *testing.T) {
	f := &fetcher{
		sources: map[string][]SourceFetcher{
			"TEST": {
				&mockFetcher{err: errors.New("source down")},
				&mockFetcher{papers: []Paper{makePaper("Good Paper", "arxiv")}},
			},
		},
	}

	papers, err := f.FetchTopic(context.Background(), "test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(papers) != 1 {
		t.Errorf("expected 1 paper from healthy source, got %d", len(papers))
	}
}

func TestFetcher_UnknownTopicError(t *testing.T) {
	f := NewFetcher()
	_, err := f.FetchTopic(context.Background(), "NONEXISTENT")
	if err == nil {
		t.Fatal("expected error for unknown topic")
	}
	var ute *UnknownTopicError
	if !errors.As(err, &ute) {
		t.Errorf("expected UnknownTopicError, got %T: %v", err, err)
	}
	if ute.Topic != "NONEXISTENT" {
		t.Errorf("topic: got %q, want NONEXISTENT", ute.Topic)
	}
	if msg := ute.Error(); msg == "" {
		t.Error("Error() returned empty string")
	}
}

func TestFetcher_TopicCaseInsensitive(t *testing.T) {
	f := &fetcher{
		sources: map[string][]SourceFetcher{
			"AI": {
				&mockFetcher{papers: []Paper{makePaper("AI Paper", "arxiv")}},
			},
		},
	}

	for _, input := range []string{"ai", "Ai", "aI", "AI"} {
		papers, err := f.FetchTopic(context.Background(), input)
		if err != nil {
			t.Errorf("FetchTopic(%q): unexpected error: %v", input, err)
		}
		if len(papers) != 1 {
			t.Errorf("FetchTopic(%q): expected 1 paper, got %d", input, len(papers))
		}
	}
}

func TestFetcher_DeduplicationKeyNormalisesSpaces(t *testing.T) {
	p1 := Paper{Title: "Hello World"}
	p2 := Paper{Title: "helloworld"}
	p3 := Paper{Title: "HELLO WORLD"}

	k1 := deduplicationKey(p1)
	k2 := deduplicationKey(p2)
	k3 := deduplicationKey(p3)

	if k1 != k2 {
		t.Errorf("expected same key for %q and %q", p1.Title, p2.Title)
	}
	if k1 != k3 {
		t.Errorf("expected same key for %q and %q", p1.Title, p3.Title)
	}
}

func TestFetcher_AllSourcesFail_ReturnsEmpty(t *testing.T) {
	f := &fetcher{
		sources: map[string][]SourceFetcher{
			"TEST": {
				&mockFetcher{err: errors.New("err1")},
				&mockFetcher{err: errors.New("err2")},
			},
		},
	}

	papers, err := f.FetchTopic(context.Background(), "test")
	if err != nil {
		t.Fatalf("unexpected top-level error: %v", err)
	}
	if papers != nil {
		t.Errorf("expected nil papers when all sources fail, got %v", papers)
	}
}

func TestFetcher_Topics(t *testing.T) {
	f := NewFetcher()
	topics := f.Topics()
	if len(topics) == 0 {
		t.Error("expected at least one topic")
	}
	if !slices.Contains(topics, "AI") {
		t.Error("expected AI in topics list")
	}
}

// TestFetcher_ConcurrentFetch_AllSourcesRun verifies that every source is called
// when FetchTopic runs sources concurrently.
func TestFetcher_ConcurrentFetch_AllSourcesRun(t *testing.T) {
	var cnt1, cnt2, cnt3 atomic.Int32

	f := &fetcher{
		sources: map[string][]SourceFetcher{
			"TEST": {
				&mockFetcher{papers: []Paper{makePaper("Paper 1", "src1")}, callCnt: &cnt1},
				&mockFetcher{papers: []Paper{makePaper("Paper 2", "src2")}, callCnt: &cnt2},
				&mockFetcher{papers: []Paper{makePaper("Paper 3", "src3")}, callCnt: &cnt3},
			},
		},
	}

	papers, err := f.FetchTopic(context.Background(), "test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cnt1.Load() != 1 {
		t.Errorf("source 1 expected 1 call, got %d", cnt1.Load())
	}
	if cnt2.Load() != 1 {
		t.Errorf("source 2 expected 1 call, got %d", cnt2.Load())
	}
	if cnt3.Load() != 1 {
		t.Errorf("source 3 expected 1 call, got %d", cnt3.Load())
	}
	if len(papers) != 3 {
		t.Errorf("expected 3 papers from 3 sources, got %d", len(papers))
	}
}
