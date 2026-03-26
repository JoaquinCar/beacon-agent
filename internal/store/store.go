package store

import (
	"context"

	"github.com/joako/beacon/internal/papers"
)

// Store accumulates papers between the 9am and 9pm runs.
// Only the scheduler package is allowed to call Drain.
type Store interface {
	Save(ctx context.Context, ps []papers.Paper) error
	Drain(ctx context.Context) ([]papers.Paper, error)
}

// MemoryStore is an in-memory implementation of Store.
// It will be fully implemented in Week 3.
type MemoryStore struct {
	papers []papers.Paper
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{}
}

func (s *MemoryStore) Save(_ context.Context, ps []papers.Paper) error {
	s.papers = append(s.papers, ps...)
	return nil
}

func (s *MemoryStore) Drain(_ context.Context) ([]papers.Paper, error) {
	out := s.papers
	s.papers = nil
	return out, nil
}
