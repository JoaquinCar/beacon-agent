package papers

import (
	"context"
	"time"
)

// SourceFetcher is implemented by each paper source.
type SourceFetcher interface {
	Fetch(ctx context.Context, topic string) ([]Paper, error)
}

// Fetcher is the public interface for the multi-source orchestrator.
// Consumers depend on this interface, not on the concrete struct.
type Fetcher interface {
	FetchTopic(ctx context.Context, topic string) ([]Paper, error)
	Topics() []string
}

// Paper represents a single academic paper fetched from any source.
type Paper struct {
	Title    string
	Authors  []string
	Source   string // "arxiv" | "huggingface" | "semantic_scholar" | "biorxiv"
	Topic    string
	Date     time.Time
	URL      string
	DOI      string
	Abstract string
}
