package scheduler

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/joako/beacon/internal/briefing"
	"github.com/joako/beacon/internal/config"
	"github.com/joako/beacon/internal/mood"
	"github.com/joako/beacon/internal/papers"
)

// --- mocks ----------------------------------------------------------------

type mockDetector struct{ level mood.MoodLevel }

func (m *mockDetector) Detect(_ context.Context) (mood.DetectResult, error) {
	return mood.DetectResult{Level: m.level}, nil
}

type mockFetcher struct{ papers []papers.Paper }

func (m *mockFetcher) FetchTopic(_ context.Context, _ string) ([]papers.Paper, error) {
	return m.papers, nil
}

func (m *mockFetcher) Topics() []string { return []string{"AI"} }

type failFetcher struct{}

func (f *failFetcher) FetchTopic(_ context.Context, _ string) ([]papers.Paper, error) {
	return nil, errors.New("network error")
}

func (f *failFetcher) Topics() []string { return []string{"AI"} }

type mockBriefer struct {
	called int
	mode   briefing.Mode
}

func (m *mockBriefer) Generate(_ context.Context, _ []papers.Paper, mode briefing.Mode) (briefing.Briefing, error) {
	m.called++
	m.mode = mode
	return briefing.Briefing{Mode: mode}, nil
}

type mockSender struct {
	called   int
	briefing briefing.Briefing
}

func (m *mockSender) Send(_ context.Context, b briefing.Briefing) error {
	m.called++
	m.briefing = b
	return nil
}

// --- helpers ---------------------------------------------------------------

func makePaper(title string) papers.Paper {
	return papers.Paper{Title: title, Source: "arxiv"}
}

func makeScheduler(t *testing.T, level mood.MoodLevel, ps []papers.Paper, briefer Briefer, sender Sender) *Scheduler {
	t.Helper()
	cfg := &config.Config{Timezone: "America/Merida"}
	sched, err := New(cfg, &mockDetector{level}, &mockFetcher{ps}, briefer, sender)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return sched
}

// --- tests -----------------------------------------------------------------

// TestRunOnce_NormalMood_SendsFull: NORMAL → ModeFull at both 9am and 9pm.
func TestRunOnce_NormalMood_SendsFull(t *testing.T) {
	for _, hour := range []int{9, 21} {
		br := &mockBriefer{}
		sn := &mockSender{}
		sched := makeScheduler(t, mood.NORMAL, []papers.Paper{makePaper("P1")}, br, sn)

		if err := sched.RunOnce(context.Background(), hour); err != nil {
			t.Fatalf("hour=%d RunOnce: %v", hour, err)
		}
		if br.mode != briefing.ModeFull {
			t.Errorf("hour=%d NORMAL: mode = %v, want ModeFull", hour, br.mode)
		}
		if sn.called != 1 {
			t.Errorf("hour=%d NORMAL: sender called %d times, want 1", hour, sn.called)
		}
	}
}

// TestRunOnce_HighBPM_SendsSummary: HIGH_BPM → ModeSummary at both 9am and 9pm.
func TestRunOnce_HighBPM_SendsSummary(t *testing.T) {
	for _, hour := range []int{9, 21} {
		br := &mockBriefer{}
		sn := &mockSender{}
		sched := makeScheduler(t, mood.HIGH_BPM, []papers.Paper{makePaper("P1")}, br, sn)

		if err := sched.RunOnce(context.Background(), hour); err != nil {
			t.Fatalf("hour=%d RunOnce: %v", hour, err)
		}
		if br.mode != briefing.ModeSummary {
			t.Errorf("hour=%d HIGH_BPM: mode = %v, want ModeSummary", hour, br.mode)
		}
		if sn.called != 1 {
			t.Errorf("hour=%d HIGH_BPM: sender called %d times, want 1", hour, sn.called)
		}
	}
}

// TestRunOnce_NoPapers_NoBriefing: no papers → neither briefer nor sender called.
func TestRunOnce_NoPapers_NoBriefing(t *testing.T) {
	br := &mockBriefer{}
	sn := &mockSender{}
	sched := makeScheduler(t, mood.NORMAL, nil, br, sn)

	if err := sched.RunOnce(context.Background(), 9); err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	if br.called != 0 {
		t.Errorf("briefer should not be called with 0 papers, called %d", br.called)
	}
	if sn.called != 0 {
		t.Errorf("sender should not be called with 0 papers, called %d", sn.called)
	}
}

// TestRunOnce_FetchFails_NoPanic: fetch error is logged and skipped, not fatal.
func TestRunOnce_FetchFails_NoPanic(t *testing.T) {
	br := &mockBriefer{}
	sn := &mockSender{}
	cfg := &config.Config{Timezone: "America/Merida"}

	sched, err := New(cfg, &mockDetector{mood.NORMAL}, &failFetcher{}, br, sn)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	if err := sched.RunOnce(context.Background(), 9); err != nil {
		t.Fatalf("RunOnce should not error on fetch failure: %v", err)
	}
	if br.called != 0 {
		t.Errorf("briefer should not be called with zero papers, called %d", br.called)
	}
}

// TestNew_InvalidTimezone: New returns an error for unknown timezone strings.
func TestNew_InvalidTimezone(t *testing.T) {
	cfg := &config.Config{Timezone: "Not/ATimezone"}
	_, err := New(cfg, &mockDetector{}, &mockFetcher{}, &mockBriefer{}, &mockSender{})
	if err == nil {
		t.Fatal("expected error for invalid timezone")
	}
}

// TestStart_CancelledContext: Start returns when ctx is cancelled before the first fire.
func TestStart_CancelledContext(t *testing.T) {
	sched := makeScheduler(t, mood.NORMAL, nil, &mockBriefer{}, &mockSender{})

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancelled immediately

	err := sched.Start(ctx)
	if err == nil {
		t.Fatal("expected error from cancelled context")
	}
}

// TestNextFire: verifies the scheduler picks the correct next trigger time.
func TestNextFire(t *testing.T) {
	cfg := &config.Config{Timezone: "UTC"}
	sched, err := New(cfg, &mockDetector{}, &mockFetcher{}, &mockBriefer{}, &mockSender{})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	loc := time.UTC
	tests := []struct {
		name string
		now  time.Time
		want int // hour of expected next fire
	}{
		{"before 9am", time.Date(2024, 1, 1, 7, 0, 0, 0, loc), 9},
		{"between 9am and 9pm", time.Date(2024, 1, 1, 14, 0, 0, 0, loc), 21},
		{"after 9pm", time.Date(2024, 1, 1, 22, 0, 0, 0, loc), 9}, // next day 9am
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sched.nextFire(tt.now)
			if got.Hour() != tt.want {
				t.Errorf("nextFire(%v) = %v (hour %d), want hour %d", tt.now, got, got.Hour(), tt.want)
			}
		})
	}
}
