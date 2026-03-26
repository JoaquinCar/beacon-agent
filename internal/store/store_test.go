package store

import (
	"context"
	"sync"
	"testing"

	"github.com/joako/beacon/internal/papers"
)

func makePaper(title string) papers.Paper {
	return papers.Paper{Title: title, Source: "arxiv"}
}

func TestMemoryStore_SaveAndDrain_ReturnsAll(t *testing.T) {
	s := NewMemoryStore()
	ctx := context.Background()

	ps := []papers.Paper{makePaper("A"), makePaper("B"), makePaper("C")}
	if err := s.Save(ctx, ps); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, err := s.Drain(ctx)
	if err != nil {
		t.Fatalf("Drain: %v", err)
	}
	if len(got) != 3 {
		t.Errorf("expected 3 papers, got %d", len(got))
	}
}

func TestMemoryStore_DrainEmpty_ReturnsEmpty(t *testing.T) {
	s := NewMemoryStore()
	got, err := s.Drain(context.Background())
	if err != nil {
		t.Fatalf("Drain: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected empty drain, got %d", len(got))
	}
}

func TestMemoryStore_DrainClears(t *testing.T) {
	s := NewMemoryStore()
	ctx := context.Background()

	_ = s.Save(ctx, []papers.Paper{makePaper("X")})
	_, _ = s.Drain(ctx) // first drain — clears

	got, _ := s.Drain(ctx) // second drain — must be empty
	if len(got) != 0 {
		t.Errorf("expected empty after second Drain, got %d", len(got))
	}
}

func TestMemoryStore_SaveMultipleBatches(t *testing.T) {
	s := NewMemoryStore()
	ctx := context.Background()

	_ = s.Save(ctx, []papers.Paper{makePaper("A"), makePaper("B")})
	_ = s.Save(ctx, []papers.Paper{makePaper("C")})

	got, _ := s.Drain(ctx)
	if len(got) != 3 {
		t.Errorf("expected 3 papers after two saves, got %d", len(got))
	}
}

func TestMemoryStore_ConcurrentSave(t *testing.T) {
	s := NewMemoryStore()
	ctx := context.Background()

	var wg sync.WaitGroup
	for range 10 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = s.Save(ctx, []papers.Paper{makePaper("P")})
		}()
	}
	wg.Wait()

	got, _ := s.Drain(ctx)
	if len(got) != 10 {
		t.Errorf("expected 10 papers from concurrent saves, got %d", len(got))
	}
}
