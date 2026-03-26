package papers

import (
	"context"
	"log/slog"
	"strings"
	"sync"
)

// fetcher orchestrates multiple SourceFetchers, deduplicates results, and returns
// a combined list of papers for a given topic.
type fetcher struct {
	sources map[string][]SourceFetcher
}

// NewFetcher returns a Fetcher wired with all production sources from CLAUDE.md.
func NewFetcher() Fetcher {
	return &fetcher{
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

// sourceResult carries the output of one concurrent source fetch.
type sourceResult struct {
	papers []Paper
	err    error
}

// FetchTopic fetches papers for the given topic from all configured sources
// concurrently. Topic matching is case-insensitive. Source errors are logged
// with slog.Warn and skipped so that a single failing source never aborts the
// entire fetch.
func (f *fetcher) FetchTopic(ctx context.Context, topic string) ([]Paper, error) {
	key := strings.ToUpper(strings.TrimSpace(topic))
	sources, ok := f.sources[key]
	if !ok {
		return nil, &UnknownTopicError{Topic: topic, Known: f.Topics()}
	}

	results := make(chan sourceResult, len(sources))

	var wg sync.WaitGroup
	for _, src := range sources {
		wg.Add(1)
		go func(s SourceFetcher) {
			defer wg.Done()
			papers, err := s.Fetch(ctx, key)
			results <- sourceResult{papers: papers, err: err}
		}(src)
	}

	// Close the channel once all goroutines finish so we can range over it.
	go func() {
		wg.Wait()
		close(results)
	}()

	seen := make(map[string]bool)
	var all []Paper

	for res := range results {
		if res.err != nil {
			slog.Warn("fetcher: source failed", "topic", key, "err", res.err)
			continue
		}
		for _, p := range res.papers {
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
func (f *fetcher) Topics() []string {
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
