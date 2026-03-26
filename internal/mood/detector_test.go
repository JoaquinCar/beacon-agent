package mood

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// inlineSpotifyMock is a local mock so detector_test.go has no external test
// dependencies. A shared mock also lives in tests/mocks/ for other packages.
type inlineSpotifyMock struct {
	track *Track
	err   error
}

func (m *inlineSpotifyMock) NowPlaying(_ context.Context) (*Track, error) {
	return m.track, m.err
}

func (m *inlineSpotifyMock) TokenExpiresIn() int { return 3600 }

func newDetector(track *Track, err error) *Detector {
	return NewDetector(&inlineSpotifyMock{track: track, err: err})
}

// TestDetector_GenreOverride_AllGenres verifies every high-energy genre triggers HIGH_BPM.
func TestDetector_GenreOverride_AllGenres(t *testing.T) {
	for _, genre := range highEnergyGenres {
		genre := genre // capture
		t.Run("genre="+genre, func(t *testing.T) {
			det := newDetector(&Track{Genres: []string{genre}}, nil)
			got, err := det.Detect(context.Background())
			if err != nil {
				t.Fatalf("Detect() unexpected error: %v", err)
			}
			if got.Level != HIGH_BPM {
				t.Errorf("genre %q: got %v, want HIGH_BPM", genre, got.Level)
			}
		})
	}
}

// TestDetector_NilTrack_DefaultNormal verifies that nil track → NORMAL with no error.
func TestDetector_NilTrack_DefaultNormal(t *testing.T) {
	det := newDetector(nil, nil)
	got, err := det.Detect(context.Background())
	if err != nil {
		t.Fatalf("Detect() unexpected error: %v", err)
	}
	if got.Level != NORMAL {
		t.Errorf("nil track: got %v, want NORMAL", got.Level)
	}
	if got.Track != nil {
		t.Errorf("nil track: DetectResult.Track should be nil")
	}
}

// TestDetector_SpotifyError_DefaultNormal verifies that a Spotify error is absorbed
// and NORMAL is returned without propagating the error.
func TestDetector_SpotifyError_DefaultNormal(t *testing.T) {
	det := newDetector(nil, errors.New("connection refused"))
	got, err := det.Detect(context.Background())
	if err != nil {
		t.Errorf("Detect() should not return error on Spotify failure, got: %v", err)
	}
	if got.Level != NORMAL {
		t.Errorf("spotify error: got %v, want NORMAL", got.Level)
	}
}

// TestDetector_GeneralCases covers genre-based classification scenarios.
func TestDetector_GeneralCases(t *testing.T) {
	tests := []struct {
		name    string
		track   *Track
		err     error
		want    MoodLevel
		wantErr bool
	}{
		{
			name:  "high energy genre",
			track: &Track{Genres: []string{"rap"}},
			want:  HIGH_BPM,
		},
		{
			name:  "corridos tumbados is high energy",
			track: &Track{Genres: []string{"corridos tumbados"}},
			want:  HIGH_BPM,
		},
		{
			name:  "normal mood lo-fi",
			track: &Track{Genres: []string{"lo-fi"}},
			want:  NORMAL,
		},
		{
			name:  "lo-fi hip-hop is not high energy",
			track: &Track{Genres: []string{"lo-fi hip-hop"}},
			want:  NORMAL,
		},
		{
			name:  "jazz rap is not high energy",
			track: &Track{Genres: []string{"jazz rap"}},
			want:  NORMAL,
		},
		{
			name:  "trap soul is not high energy",
			track: &Track{Genres: []string{"trap soul"}},
			want:  NORMAL,
		},
		{
			name:  "cloud rap is not high energy",
			track: &Track{Genres: []string{"cloud rap"}},
			want:  NORMAL,
		},
		{
			name:  "spotify not playing",
			track: nil,
			want:  NORMAL,
		},
		{
			name:    "spotify error defaults to NORMAL no error returned",
			track:   nil,
			err:     errors.New("timeout"),
			want:    NORMAL,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			det := newDetector(tt.track, tt.err)
			got, err := det.Detect(context.Background())
			if (err != nil) != tt.wantErr {
				t.Errorf("Detect() error = %v, wantErr %v", err, tt.wantErr)
			}
			if got.Level != tt.want {
				t.Errorf("Detect() Level = %v, want %v", got.Level, tt.want)
			}
		})
	}
}

// TestDetector_ReasonString verifies the reason output format.
func TestDetector_ReasonString(t *testing.T) {
	tests := []struct {
		name         string
		track        *Track
		wantContains string
	}{
		{
			name:         "genre reason",
			track:        &Track{Genres: []string{"rap"}},
			wantContains: "high-energy",
		},
		{
			name:         "normal reason",
			track:        &Track{Genres: []string{"classical"}},
			wantContains: "no high-energy genres",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			det := newDetector(tt.track, nil)
			got, _ := det.Detect(context.Background())
			if !contains(got.Reason, tt.wantContains) {
				t.Errorf("Reason = %q, want it to contain %q", got.Reason, tt.wantContains)
			}
		})
	}
}

// TestMoodLevel_String verifies String() output.
func TestMoodLevel_String(t *testing.T) {
	if NORMAL.String() != "NORMAL" {
		t.Errorf("NORMAL.String() = %q, want \"NORMAL\"", NORMAL.String())
	}
	if HIGH_BPM.String() != "HIGH_BPM" {
		t.Errorf("HIGH_BPM.String() = %q, want \"HIGH_BPM\"", HIGH_BPM.String())
	}
}

func contains(s, substr string) bool {
	return strings.Contains(s, substr)
}
