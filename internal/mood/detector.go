package mood

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
)

// highEnergyGenres is the canonical list of genres that trigger HIGH_BPM classification.
// Matching is substring-based (case-insensitive), but genres containing a
// lowEnergyModifier are skipped first to avoid false positives like "lo-fi hip-hop"
// or "jazz rap" being classified as high energy.
var highEnergyGenres = []string{
	"gym", "workout",
	"rap", "hip-hop",
	"trap", "drill", "grime", "phonk",
	"corridos", "corridos tumbados", "sierreño",
	"banda",
	"reggaeton", "dembow", "latin trap",
	"hard rock", "heavy metal", "thrash metal", "death metal", "metalcore",
	"drum and bass", "dnb",
	"hardstyle", "gabber",
}

// lowEnergyModifiers are terms that, when present in a genre tag, indicate a
// chill or hybrid variant of an otherwise high-energy genre.
// E.g. "lo-fi hip-hop", "jazz rap", "trap soul", "cloud rap" → NORMAL.
var lowEnergyModifiers = []string{
	"lo-fi", "lofi", "chill", "ambient",
	"jazz", "soul", "cloud", "sleep", "study",
	"house", // deep house, tech house, afro house, progressive house → focus/NORMAL
}

// Detector classifies the user's current mood based on Spotify playback.
type Detector struct {
	client SpotifyClient
}

// NewDetector creates a new Detector backed by the given SpotifyClient.
func NewDetector(client SpotifyClient) *Detector {
	return &Detector{client: client}
}

// Detect queries Spotify and returns a DetectResult.
// This method never returns a non-nil error — Spotify failures default to NORMAL.
func (d *Detector) Detect(ctx context.Context) (DetectResult, error) {
	track, err := d.client.NowPlaying(ctx)
	if err != nil {
		slog.Warn("mood: spotify unavailable, defaulting to NORMAL", "err", err)
		return DetectResult{
			Level:  NORMAL,
			Reason: "spotify unavailable, defaulting to NORMAL",
		}, nil
	}

	if track == nil {
		slog.Info("mood: no active playback, defaulting to NORMAL")
		return DetectResult{
			Level:  NORMAL,
			Reason: "no active playback",
		}, nil
	}

	level, reason := classifyTrack(track)
	return DetectResult{
		Level:  level,
		Track:  track,
		Reason: reason,
	}, nil
}

// classifyTrack applies genre rules to determine MoodLevel.
// Returns the level and a human-readable reason string for display.
func classifyTrack(track *Track) (MoodLevel, string) {
	for _, genre := range track.Genres {
		lowerGenre := strings.ToLower(genre)

		// Skip genres that are chill/hybrid variants of high-energy genres.
		chilled := false
		for _, mod := range lowEnergyModifiers {
			if strings.Contains(lowerGenre, mod) {
				chilled = true
				break
			}
		}
		if chilled {
			continue
		}

		for _, highGenre := range highEnergyGenres {
			if strings.Contains(lowerGenre, highGenre) {
				return HIGH_BPM, fmt.Sprintf("genre '%s' is high-energy", genre)
			}
		}
	}
	return NORMAL, "no high-energy genres detected"
}
