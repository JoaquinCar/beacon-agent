package papers

import (
	"context"
	"log/slog"
	"strings"
)

// Fetcher orchestrates multiple SourceFetchers, deduplicates results, and returns
// a combined list of papers for a given topic.
type Fetcher struct {
	sources map[string][]SourceFetcher
}

// NewFetcher returns a Fetcher wired with all production sources from CLAUDE.md.
func NewFetcher() *Fetcher {
	return &Fetcher{
		sources: map[string][]SourceFetcher{
			"AI": {
				NewArXivFetcher("cs.AI"),
				NewArXivFetcher("cs.LG"),
				NewArXivFetcher("cs.CL"),
				NewHuggingFaceFetcher(),
			},
			"HEALTHCARE": {
				NewArXivFetcher("cs.AI"),
				NewArXivFetcher("q-bio"),
				NewSemanticScholarFetcher("healthcare artificial intelligence"),
			},
			"BCI": {
				NewArXivFetcher("eess.SP"),
				NewBioRxivFetcher("biorxiv"),
			},
			"CV": {
				NewArXivFetcher("cs.CV"),
			},
			"BIO": {
				NewBioRxivFetcher("biorxiv"),
				NewBioRxivFetcher("medrxiv"),
			},
			"ANTHROPIC": {
				NewArXivFetcher("cs.AI"),
				NewHuggingFaceFetcher(),
			},
		},
	}
}

// FetchTopic fetches papers for the given topic from all configured sources.
// Topic matching is case-insensitive. Source errors are logged and skipped so
// that a single failing source never aborts the entire fetch.
func (f *Fetcher) FetchTopic(ctx context.Context, topic string) ([]Paper, error) {
	key := strings.ToUpper(strings.TrimSpace(topic))
	sources, ok := f.sources[key]
	if !ok {
		return nil, &UnknownTopicError{Topic: topic, Known: f.Topics()}
	}

	seen := make(map[string]bool)
	var all []Paper

	for _, src := range sources {
		papers, err := src.Fetch(ctx, key)
		if err != nil {
			slog.Warn("fetcher: source failed", "topic", key, "err", err)
			continue
		}
		for _, p := range papers {
			dk := deduplicationKey(p)
			if !seen[dk] {
				seen[dk] = true
				all = append(all, p)
			}
		}
	}

	return all, nil
}

// Topics returns the list of known topic keys.
func (f *Fetcher) Topics() []string {
	keys := make([]string, 0, len(f.sources))
	for k := range f.sources {
		keys = append(keys, k)
	}
	return keys
}

// deduplicationKey produces a stable key for a paper used to detect duplicates.
// It normalises the title to lowercase with no spaces.
func deduplicationKey(p Paper) string {
	return strings.ToLower(strings.ReplaceAll(p.Title, " ", ""))
}

// UnknownTopicError is returned when FetchTopic receives a topic key that has
// no registered sources.
type UnknownTopicError struct {
	Topic string
	Known []string
}

func (e *UnknownTopicError) Error() string {
	return "unknown topic '" + e.Topic + "'; known topics: " + strings.Join(e.Known, ", ")
}
