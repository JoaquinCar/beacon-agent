package papers

import (
	"context"
	"time"
)

// SourceFetcher is implemented by each paper source.
type SourceFetcher interface {
	Fetch(ctx context.Context, topic string) ([]Paper, error)
}

// Paper represents a single academic paper fetched from any source.
type Paper struct {
	Title    string
	Authors  []string
	Source   string // "arxiv" | "huggingface" | "semantic_scholar" | "biorxiv"
	Topic    string
	Date     time.Time
	URL      string
	Abstract string
}
