package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/joako/beacon/internal/config"
	"github.com/joako/beacon/internal/mood"
	"github.com/joako/beacon/internal/papers"
)

func main() {
	cmd := flag.String("cmd", "run", "Command to execute: mood | fetch | run")
	topic := flag.String("topic", "AI", "Topic for --cmd=fetch (AI, HEALTHCARE, BCI, CV, BIO, ANTHROPIC)")
	flag.Parse()

	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	config.SetupLogger(cfg.LogLevel)
	cfg.LogSafe()

	ctx := context.Background()

	switch *cmd {
	case "mood":
		if err := runMoodCheck(ctx, cfg); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(2)
		}
	case "fetch":
		if err := runPaperFetch(ctx, *topic); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(2)
		}
	default:
		fmt.Println("scheduler: not implemented yet (Week 4)")
	}
}

// runMoodCheck runs the /test-mood command: detects mood and prints a structured report.
// All output goes to stdout using fmt.Printf (not slog).
func runMoodCheck(ctx context.Context, cfg *config.Config) error {
	client := mood.NewClient(cfg)
	detector := mood.NewDetector(client)

	result, err := detector.Detect(ctx)
	if err != nil {
		// Detect never returns errors (Spotify failures are absorbed), but be defensive.
		return fmt.Errorf("mood check: %w", err)
	}

	fmt.Println("=== Beacon / mood check ===")
	fmt.Println()

	if result.Track == nil {
		fmt.Println("Track:   (nothing playing)")
		fmt.Printf("Mood:    NORMAL (default — no active playback)\n")
	} else {
		trackLabel := result.Track.Title + " — " + result.Track.Artist
		if result.Track.IsRecentlyPlayed {
			trackLabel += "  (recently played)"
		}
		fmt.Printf("Track:   %s\n", trackLabel)
		fmt.Printf("Genres:  %v\n", result.Track.Genres)
		fmt.Println()
		fmt.Printf("Mood:    %s\n", result.Level)
		fmt.Printf("Reason:  %s\n", result.Reason)
	}

	fmt.Println()
	fmt.Printf("Spotify token: valid (expires in %ds)\n", client.TokenExpiresIn())

	return nil
}

// runPaperFetch fetches papers for the given topic and prints them to stdout.
func runPaperFetch(ctx context.Context, topic string) error {
	fetcher := papers.NewFetcher()
	results, err := fetcher.FetchTopic(ctx, topic)
	if err != nil {
		return fmt.Errorf("paper fetch: %w", err)
	}

	fmt.Printf("=== Beacon / paper fetch — topic: %s ===\n\n", topic)

	if len(results) == 0 {
		fmt.Println("(no papers found)")
		return nil
	}

	for i, p := range results {
		fmt.Printf("[%d] %s\n", i+1, p.Title)
		if len(p.Authors) > 0 {
			authors := p.Authors
			if len(authors) > 3 {
				authors = append(authors[:3], "et al.")
			}
			fmt.Printf("    Authors: %s\n", strings.Join(authors, ", "))
		}
		fmt.Printf("    Source:  %s | Date: %s\n", p.Source, formatDate(p))
		fmt.Printf("    URL:     %s\n", p.URL)
		if p.Abstract != "" {
			abs := p.Abstract
			if len(abs) > 200 {
				abs = abs[:200] + "…"
			}
			fmt.Printf("    Abstract: %s\n", abs)
		}
		fmt.Println()
	}

	fmt.Printf("Total: %d papers\n", len(results))
	return nil
}

func formatDate(p papers.Paper) string {
	if p.Date.IsZero() {
		return "unknown"
	}
	return p.Date.Format("2006-01-02")
}
