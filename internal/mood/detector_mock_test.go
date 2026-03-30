package mood

import (
	"context"
	"testing"
)

// TestDetector_MockMood_HighBPM verifies MOCK_MOOD=HIGH_BPM bypasses Spotify.
func TestDetector_MockMood_HighBPM(t *testing.T) {
	t.Setenv("MOCK_MOOD", "HIGH_BPM")

	// Even with a nil-track mock, MOCK_MOOD should short-circuit to HIGH_BPM.
	det := newDetector(nil, nil)
	got, err := det.Detect(context.Background())
	if err != nil {
		t.Fatalf("Detect() unexpected error: %v", err)
	}
	if got.Level != HIGH_BPM {
		t.Errorf("MOCK_MOOD=HIGH_BPM: got %v, want HIGH_BPM", got.Level)
	}
}

// TestDetector_MockMood_Normal verifies MOCK_MOOD=NORMAL bypasses Spotify.
func TestDetector_MockMood_Normal(t *testing.T) {
	t.Setenv("MOCK_MOOD", "NORMAL")

	// Even with a high-energy genre track, MOCK_MOOD should force NORMAL.
	det := newDetector(&Track{Genres: []string{"rap"}}, nil)
	got, err := det.Detect(context.Background())
	if err != nil {
		t.Fatalf("Detect() unexpected error: %v", err)
	}
	if got.Level != NORMAL {
		t.Errorf("MOCK_MOOD=NORMAL: got %v, want NORMAL", got.Level)
	}
}

// TestDetector_MockMood_Unknown verifies an unknown MOCK_MOOD value falls through to Spotify.
func TestDetector_MockMood_Unknown(t *testing.T) {
	t.Setenv("MOCK_MOOD", "INVALID_VALUE")

	// Should fall through and classify the track normally.
	det := newDetector(&Track{Genres: []string{"rap"}}, nil)
	got, err := det.Detect(context.Background())
	if err != nil {
		t.Fatalf("Detect() unexpected error: %v", err)
	}
	if got.Level != HIGH_BPM {
		t.Errorf("unknown MOCK_MOOD: got %v, want HIGH_BPM (fell through to Spotify)", got.Level)
	}
}
