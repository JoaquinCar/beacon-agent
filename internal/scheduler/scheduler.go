package scheduler

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/joako/beacon/internal/briefing"
	"github.com/joako/beacon/internal/config"
	"github.com/joako/beacon/internal/mood"
	"github.com/joako/beacon/internal/papers"
)

// Briefer generates a Briefing from a list of papers in the given mode.
// Defined in the consumer package (scheduler) per the consumer-side interface pattern.
type Briefer interface {
	Generate(ctx context.Context, ps []papers.Paper, mode briefing.Mode) (briefing.Briefing, error)
}

// Sender delivers a Briefing to an external channel.
type Sender interface {
	Send(ctx context.Context, b briefing.Briefing) error
}

// Scheduler drives the 9am/9pm briefing pipeline.
type Scheduler struct {
	cfg      *config.Config
	detector mood.MoodDetector
	fetcher  papers.Fetcher
	briefer  Briefer
	sender   Sender
	loc      *time.Location
}

// New creates a Scheduler with all dependencies injected.
// Returns an error if the timezone in cfg cannot be loaded.
func New(
	cfg *config.Config,
	detector mood.MoodDetector,
	fetcher papers.Fetcher,
	briefer Briefer,
	sender Sender,
) (*Scheduler, error) {
	loc, err := time.LoadLocation(cfg.Timezone)
	if err != nil {
		return nil, fmt.Errorf("scheduler: load timezone %q: %w", cfg.Timezone, err)
	}
	return &Scheduler{
		cfg:      cfg,
		detector: detector,
		fetcher:  fetcher,
		briefer:  briefer,
		sender:   sender,
		loc:      loc,
	}, nil
}

// Start runs the cron loop, blocking until ctx is cancelled.
// It fires at 09:00 and 21:00 in the timezone specified by cfg.Timezone.
func (s *Scheduler) Start(ctx context.Context) error {
	for {
		next := s.nextFire(time.Now().In(s.loc))
		slog.Info("scheduler: next run", "at", next.Format(time.RFC3339))
		timer := time.NewTimer(time.Until(next))
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case t := <-timer.C:
			hour := t.In(s.loc).Hour()
			if err := s.RunOnce(ctx, hour); err != nil {
				slog.Error("scheduler: run failed", "hour", hour, "err", err)
				// Non-fatal: log and wait for the next trigger.
			}
		}
	}
}

// RunOnce executes one pipeline cycle for the given hour (9 = morning, 21 = evening).
// Both hours fetch papers and send — mode depends on current mood:
//
//	HIGH_BPM → ModeSummary (short 5–8 line summaries, good for gym/breakfast)
//	NORMAL   → ModeFull    (structured analysis with TL;DR)
//
// Exported for use by dry-run (cmd/beacon/main.go) and tests.
func (s *Scheduler) RunOnce(ctx context.Context, hour int) error {
	slog.Info("scheduler: running pipeline", "hour", hour)

	result, _ := s.detector.Detect(ctx)
	// Detect() never returns a non-nil error; the blank assignment is intentional.

	mode := briefing.ModeFull
	if result.Level == mood.HIGH_BPM {
		mode = briefing.ModeSummary
	}

	ps, err := s.fetchAllTopics(ctx)
	if err != nil {
		return err
	}
	if len(ps) == 0 {
		slog.Info("scheduler: no papers fetched, skipping briefing", "hour", hour)
		return nil
	}

	b, err := s.briefer.Generate(ctx, ps, mode)
	if err != nil {
		return fmt.Errorf("scheduler: generate briefing: %w", err)
	}
	return s.sender.Send(ctx, b)
}

// fetchAllTopics fetches papers from every topic the fetcher supports.
// Individual topic failures are logged and skipped so one broken source
// does not abort the entire pipeline.
func (s *Scheduler) fetchAllTopics(ctx context.Context) ([]papers.Paper, error) {
	var all []papers.Paper
	for _, topic := range s.fetcher.Topics() {
		ps, err := s.fetcher.FetchTopic(ctx, topic)
		if err != nil {
			slog.Warn("scheduler: fetch failed, skipping topic", "topic", topic, "err", err)
			continue
		}
		all = append(all, ps...)
	}
	return all, nil
}

// nextFire returns the next 09:00 or 21:00 trigger time in the scheduler's location.
func (s *Scheduler) nextFire(now time.Time) time.Time {
	morning := time.Date(now.Year(), now.Month(), now.Day(), 9, 0, 0, 0, now.Location())
	evening := time.Date(now.Year(), now.Month(), now.Day(), 21, 0, 0, 0, now.Location())

	if now.Before(morning) {
		return morning
	}
	if now.Before(evening) {
		return evening
	}
	// Both fires have passed today — schedule for 9am tomorrow.
	return morning.Add(24 * time.Hour)
}
