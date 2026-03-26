package mood

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/joako/beacon/internal/config"
)

const (
	spotifyTokenURL      = "https://accounts.spotify.com/api/token"
	spotifyAPIBase       = "https://api.spotify.com/v1"
	httpTimeout          = 10 * time.Second
	maxResponseBytes     = 5 << 20 // 5 MB
	tokenRefreshLeadTime = 60 * time.Second
)

// Client is the Spotify API client. It implements SpotifyClient.
type Client struct {
	httpClient   *http.Client
	clientID     string
	clientSecret string
	refreshToken string
	apiBase      string // overrideable for tests; defaults to spotifyAPIBase
	tokenURL     string // overrideable for tests; defaults to spotifyTokenURL
	lastfm       *lastFMClient // optional genre fallback

	mu          sync.RWMutex
	accessToken string
	tokenExpiry time.Time
}

// NewClient creates a new Spotify client from config.
// If cfg.LastFMAPIKey is set, a Last.fm client is wired in as a genre fallback.
func NewClient(cfg *config.Config) *Client {
	c := &Client{
		httpClient:   &http.Client{Timeout: httpTimeout},
		clientID:     cfg.SpotifyClientID,
		clientSecret: cfg.SpotifyClientSecret,
		refreshToken: cfg.SpotifyRefreshToken,
		apiBase:      spotifyAPIBase,
		tokenURL:     spotifyTokenURL,
	}
	if cfg.LastFMAPIKey != "" {
		c.lastfm = newLastFMClient(cfg.LastFMAPIKey)
	}
	return c
}

// TokenExpiresIn returns the number of seconds until the current access token expires.
func (c *Client) TokenExpiresIn() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return int(time.Until(c.tokenExpiry).Seconds())
}

// NowPlaying returns the currently playing track, or nil if nothing is playing.
// Returns nil, nil for 204 No Content or paused playback — these are not errors.
func (c *Client) NowPlaying(ctx context.Context) (*Track, error) {
	if err := c.ensureToken(ctx); err != nil {
		return nil, fmt.Errorf("spotify: %w", err)
	}

	raw, err := c.getNowPlaying(ctx)
	if err != nil {
		return nil, err
	}

	isRecentlyPlayed := false
	if raw == nil {
		// Nothing playing or paused — fall back to recently played.
		recent, err := c.getRecentlyPlayed(ctx)
		if err != nil {
			slog.Warn("spotify: recently-played fallback failed", "err", err)
			return nil, nil
		}
		if recent == nil {
			return nil, nil
		}
		raw = recent
		isRecentlyPlayed = true
		slog.Debug("spotify: no active playback, classifying from recently played", "track", raw.Title)
	}

	// Try each artist until we find one with genres populated.
	var genres []string
	for _, artistID := range raw.artistIDs {
		g, err := c.getArtistGenres(ctx, artistID)
		if err != nil {
			slog.Warn("spotify: could not fetch artist genres", "artist_id", artistID, "err", err)
			continue
		}
		if len(g) > 0 {
			genres = g
			slog.Debug("spotify: genres found", "artist_id", artistID, "genres", genres)
			break
		}
		slog.Debug("spotify: artist has no genres, trying next", "artist_id", artistID)
	}
	// Fallback: if Spotify has no genres, try Last.fm track tags.
	if len(genres) == 0 && c.lastfm != nil {
		tags, err := c.lastfm.GetTags(ctx, raw.Artist, raw.Title)
		if err != nil {
			slog.Warn("lastfm: genre fallback failed", "track", raw.Title, "err", err)
		} else if len(tags) > 0 {
			genres = tags
			slog.Debug("lastfm: genres from fallback", "track", raw.Title, "genres", genres)
		} else {
			slog.Warn("lastfm: no tags found for track", "track", raw.Title)
		}
	} else if len(genres) == 0 {
		slog.Warn("spotify: no genres found and no Last.fm fallback configured", "artists", raw.artistIDs)
	}

	return &Track{
		ID:               raw.ID,
		Title:            raw.Title,
		Artist:           raw.Artist,
		Genres:           genres,
		TokenExpiresIn:   c.TokenExpiresIn(),
		IsRecentlyPlayed: isRecentlyPlayed,
	}, nil
}

// ensureToken checks whether the access token is still valid and refreshes if needed.
func (c *Client) ensureToken(ctx context.Context) error {
	c.mu.RLock()
	needsRefresh := c.accessToken == "" || time.Now().After(c.tokenExpiry.Add(-tokenRefreshLeadTime))
	c.mu.RUnlock()

	if !needsRefresh {
		return nil
	}
	return c.refreshAccessToken(ctx)
}

// refreshAccessToken exchanges the refresh token for a new access token.
func (c *Client) refreshAccessToken(ctx context.Context) error {
	form := url.Values{}
	form.Set("grant_type", "refresh_token")
	form.Set("refresh_token", c.refreshToken)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return fmt.Errorf("token refresh: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString(
		[]byte(c.clientID+":"+c.clientSecret),
	))

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("token refresh: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		return fmt.Errorf("token refresh: read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("token refresh: unexpected status %d", resp.StatusCode)
	}

	var result struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return fmt.Errorf("token refresh: parse response: %w", err)
	}

	c.mu.Lock()
	c.accessToken = result.AccessToken
	c.tokenExpiry = time.Now().Add(time.Duration(result.ExpiresIn) * time.Second)
	c.mu.Unlock()

	slog.Debug("spotify: token refreshed", "expires_in_seconds", result.ExpiresIn)
	return nil
}

// nowPlayingResponse is an internal struct to hold the raw track fields we need.
type nowPlayingResponse struct {
	ID        string
	Title     string
	Artist    string
	artistIDs []string // all artists on the track, for genre lookup
}

// getNowPlaying calls /v1/me/player/currently-playing and returns minimal track info.
// Returns nil, nil when nothing is playing (204) or the track is paused.
func (c *Client) getNowPlaying(ctx context.Context) (*nowPlayingResponse, error) {
	req, err := c.newAPIRequest(ctx, http.MethodGet, "/me/player/currently-playing", nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("spotify: now-playing request: %w", err)
	}
	defer resp.Body.Close()

	// 204 = nothing playing — not an error
	if resp.StatusCode == http.StatusNoContent {
		return nil, nil
	}

	if resp.StatusCode == http.StatusUnauthorized {
		// Token expired mid-flight — try one refresh and retry
		if err := c.refreshAccessToken(ctx); err != nil {
			return nil, fmt.Errorf("spotify: re-auth failed: %w", err)
		}
		return c.getNowPlaying(ctx)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("spotify: now-playing: unexpected status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		return nil, fmt.Errorf("spotify: now-playing: read body: %w", err)
	}

	var payload struct {
		IsPlaying bool `json:"is_playing"`
		Item      struct {
			ID      string `json:"id"`
			Name    string `json:"name"`
			Artists []struct {
				ID   string `json:"id"`
				Name string `json:"name"`
			} `json:"artists"`
		} `json:"item"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("spotify: now-playing: parse body: %w", err)
	}

	// Paused = treat as not playing
	if !payload.IsPlaying || payload.Item.ID == "" {
		return nil, nil
	}

	track := &nowPlayingResponse{
		ID:    payload.Item.ID,
		Title: payload.Item.Name,
	}
	for i, a := range payload.Item.Artists {
		if i == 0 {
			track.Artist = a.Name
		}
		if a.ID != "" {
			track.artistIDs = append(track.artistIDs, a.ID)
		}
	}
	return track, nil
}

// getArtistGenres fetches genre tags for an artist via /v1/artists/{id}.
func (c *Client) getArtistGenres(ctx context.Context, artistID string) ([]string, error) {
	if artistID == "" {
		return nil, nil
	}

	req, err := c.newAPIRequest(ctx, http.MethodGet, "/artists/"+artistID, nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("spotify: artist-genres: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("spotify: artist-genres: unexpected status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		return nil, fmt.Errorf("spotify: artist-genres: read body: %w", err)
	}

	var artist struct {
		Genres []string `json:"genres"`
	}
	if err := json.Unmarshal(body, &artist); err != nil {
		return nil, fmt.Errorf("spotify: artist-genres: parse body: %w", err)
	}

	return artist.Genres, nil
}

// getRecentlyPlayed returns the most recently played track via /v1/me/player/recently-played.
// Returns nil, nil when the history is empty or the scope is missing (403).
// Requires the user-read-recently-played OAuth scope.
func (c *Client) getRecentlyPlayed(ctx context.Context) (*nowPlayingResponse, error) {
	req, err := c.newAPIRequest(ctx, http.MethodGet, "/me/player/recently-played?limit=1", nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("spotify: recently-played: %w", err)
	}
	defer resp.Body.Close()

	// 403 = missing scope — user needs to re-run spotify-auth with the new scope
	if resp.StatusCode == http.StatusForbidden {
		slog.Warn("spotify: recently-played requires user-read-recently-played scope — re-run scripts/spotify-auth.go")
		return nil, nil
	}
	if resp.StatusCode == http.StatusNoContent {
		return nil, nil
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("spotify: recently-played: unexpected status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		return nil, fmt.Errorf("spotify: recently-played: read body: %w", err)
	}

	var payload struct {
		Items []struct {
			Track struct {
				ID      string `json:"id"`
				Name    string `json:"name"`
				Artists []struct {
					ID   string `json:"id"`
					Name string `json:"name"`
				} `json:"artists"`
			} `json:"track"`
		} `json:"items"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("spotify: recently-played: parse body: %w", err)
	}

	if len(payload.Items) == 0 || payload.Items[0].Track.ID == "" {
		return nil, nil
	}

	item := payload.Items[0].Track
	track := &nowPlayingResponse{
		ID:    item.ID,
		Title: item.Name,
	}
	for i, a := range item.Artists {
		if i == 0 {
			track.Artist = a.Name
		}
		if a.ID != "" {
			track.artistIDs = append(track.artistIDs, a.ID)
		}
	}
	return track, nil
}

// newAPIRequest builds an authenticated request to the Spotify API.
func (c *Client) newAPIRequest(ctx context.Context, method, path string, body io.Reader) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, method, c.apiBase+path, body)
	if err != nil {
		return nil, fmt.Errorf("spotify: build request: %w", err)
	}

	c.mu.RLock()
	token := c.accessToken
	c.mu.RUnlock()

	req.Header.Set("Authorization", "Bearer "+token)
	return req, nil
}
