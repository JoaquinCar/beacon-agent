package mood

import "context"

// MoodLevel represents the user's current energy level derived from Spotify playback.
type MoodLevel int

const (
	// NORMAL is the default mood — calm, focused, or no playback detected.
	NORMAL MoodLevel = iota
	// HIGH_BPM indicates high-energy music is playing (energetic genres detected).
	HIGH_BPM
)

// String returns a human-readable representation of the MoodLevel.
func (m MoodLevel) String() string {
	switch m {
	case HIGH_BPM:
		return "HIGH_BPM"
	default:
		return "NORMAL"
	}
}

// Track holds data about the currently playing Spotify track.
type Track struct {
	ID     string
	Title  string
	Artist string
	Genres []string
	// TokenExpiresIn is the number of seconds until the OAuth access token expires.
	TokenExpiresIn int
	// IsRecentlyPlayed is true when nothing is currently active and the track
	// was fetched from /v1/me/player/recently-played as a fallback.
	IsRecentlyPlayed bool
}

// SpotifyClient is the interface the mood package uses to query Spotify.
// It is defined here (consumer package) so implementations can be mocked easily.
type SpotifyClient interface {
	// NowPlaying returns the currently playing track, or nil if nothing is playing.
	// Returns nil, nil when Spotify reports 204 No Content or the track is paused.
	NowPlaying(ctx context.Context) (*Track, error)
	// TokenExpiresIn returns the number of seconds until the current access token expires.
	TokenExpiresIn() int
}

// DetectResult is returned by Detector.Detect and carries the classification
// result along with the raw track data needed for display.
type DetectResult struct {
	Level  MoodLevel
	Track  *Track // nil if nothing is playing
	Reason string // human-readable explanation, e.g. "genre 'rap' is high-energy"
}

// MoodDetector classifies the user's current mood.
// Defined here for use by the scheduler package.
type MoodDetector interface {
	Detect(ctx context.Context) (DetectResult, error)
}
