package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/joako/beacon/internal/config"
	"github.com/joako/beacon/internal/mood"
)

func main() {
	cmd := flag.String("cmd", "run", "Command to execute: mood | fetch | run")
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
		fmt.Println("paper fetch: not implemented yet (Week 2)")
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
