package mocks

import (
	"context"

	"github.com/joako/beacon/internal/mood"
)

// MockSpotifyClient is a test double for mood.SpotifyClient.
type MockSpotifyClient struct {
	Track     *mood.Track
	Err       error
	ExpiresIn int
}

func (m *MockSpotifyClient) NowPlaying(_ context.Context) (*mood.Track, error) {
	return m.Track, m.Err
}

func (m *MockSpotifyClient) TokenExpiresIn() int {
	return m.ExpiresIn
}
