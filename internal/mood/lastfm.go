package mood

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
)

const (
	lastfmAPIBase  = "https://ws.audioscrobbler.com/2.0/"
	lastfmMinCount = 1 // ignore tags with zero votes
	lastfmMaxTags  = 5 // only use the top N tags (sorted by count desc)
)

// lastFMClient fetches track tags from the Last.fm API.
// Used as a genre fallback when Spotify returns empty genres.
type lastFMClient struct {
	httpClient *http.Client
	apiKey     string
	apiBase    string // overrideable for tests
}

// newLastFMClient creates a new Last.fm client.
func newLastFMClient(apiKey string) *lastFMClient {
	return &lastFMClient{
		httpClient: &http.Client{Timeout: httpTimeout},
		apiKey:     apiKey,
		apiBase:    lastfmAPIBase,
	}
}

// GetTags returns genre tags for a track, falling back to artist-level tags if
// the track has no community tags. Returns nil when neither has data.
func (c *lastFMClient) GetTags(ctx context.Context, artist, track string) ([]string, error) {
	tags, err := c.getTrackTags(ctx, artist, track)
	if err != nil {
		return nil, err
	}
	if len(tags) > 0 {
		return tags, nil
	}
	return c.getArtistTags(ctx, artist)
}

// getTrackTags returns the top tags for a track from Last.fm.
// Returns nil, nil when the track is not found (API error 6) — not an error.
func (c *lastFMClient) getTrackTags(ctx context.Context, artist, track string) ([]string, error) {
	params := url.Values{
		"method":      {"track.getTopTags"},
		"artist":      {artist},
		"track":       {track},
		"api_key":     {c.apiKey},
		"format":      {"json"},
		"autocorrect": {"1"},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.apiBase+"?"+params.Encode(), nil)
	if err != nil {
		return nil, fmt.Errorf("lastfm: build request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("lastfm: request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("lastfm: unexpected status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		return nil, fmt.Errorf("lastfm: read body: %w", err)
	}

	var payload struct {
		TopTags struct {
			Tag []struct {
				Name  string `json:"name"`
				Count int    `json:"count"`
			} `json:"tag"`
		} `json:"toptags"`
		Error   int    `json:"error"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("lastfm: parse body: %w", err)
	}

	// API-level error (e.g. error 6 = track not found) — non-fatal
	if payload.Error != 0 {
		slog.Debug("lastfm: api error", "code", payload.Error, "message", payload.Message)
		return nil, nil
	}

	var tags []string
	for _, t := range payload.TopTags.Tag {
		if len(tags) >= lastfmMaxTags {
			break
		}
		if t.Count >= lastfmMinCount {
			tags = append(tags, t.Name)
		}
	}
	return tags, nil
}

// getArtistTags returns the top tags for an artist from Last.fm.
// Used as a fallback when a track has no community tags.
func (c *lastFMClient) getArtistTags(ctx context.Context, artist string) ([]string, error) {
	if artist == "" {
		return nil, nil
	}

	params := url.Values{
		"method":      {"artist.getTopTags"},
		"artist":      {artist},
		"api_key":     {c.apiKey},
		"format":      {"json"},
		"autocorrect": {"1"},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.apiBase+"?"+params.Encode(), nil)
	if err != nil {
		return nil, fmt.Errorf("lastfm: artist tags: build request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("lastfm: artist tags: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("lastfm: artist tags: unexpected status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		return nil, fmt.Errorf("lastfm: artist tags: read body: %w", err)
	}

	var payload struct {
		TopTags struct {
			Tag []struct {
				Name  string `json:"name"`
				Count int    `json:"count"`
			} `json:"tag"`
		} `json:"toptags"`
		Error   int    `json:"error"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("lastfm: artist tags: parse body: %w", err)
	}

	if payload.Error != 0 {
		slog.Debug("lastfm: artist tags api error", "code", payload.Error, "message", payload.Message)
		return nil, nil
	}

	var tags []string
	for _, t := range payload.TopTags.Tag {
		if len(tags) >= lastfmMaxTags {
			break
		}
		if t.Count >= lastfmMinCount {
			tags = append(tags, t.Name)
		}
	}
	return tags, nil
}
