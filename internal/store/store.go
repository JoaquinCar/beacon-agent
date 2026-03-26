package store

import (
	"context"
	"sync"

	"github.com/joako/beacon/internal/papers"
)

// Store accumulates papers between the 9am and 9pm runs.
// Only the scheduler package is allowed to call Drain.
type Store interface {
	Save(ctx context.Context, ps []papers.Paper) error
	Drain(ctx context.Context) ([]papers.Paper, error)
}

// memoryStore is a thread-safe in-memory implementation of Store.
type memoryStore struct {
	mu     sync.Mutex
	papers []papers.Paper
}

// NewMemoryStore returns a new in-memory Store.
func NewMemoryStore() Store {
	return &memoryStore{}
}

func (s *memoryStore) Save(_ context.Context, ps []papers.Paper) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.papers = append(s.papers, ps...)
	return nil
}

// Drain returns all stored papers and clears the store atomically.
func (s *memoryStore) Drain(_ context.Context) ([]papers.Paper, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := s.papers
	s.papers = nil
	return out, nil
}
