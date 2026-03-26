package mood

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/joako/beacon/internal/config"
)

// clientForTest returns a Client whose API calls go to baseURL.
// A valid token is pre-seeded so tests don't need a real token endpoint.
func clientForTest(t *testing.T, baseURL string) *Client {
	t.Helper()
	cfg := &config.Config{
		SpotifyClientID:     "test-id",
		SpotifyClientSecret: "test-secret",
		SpotifyRefreshToken: "test-refresh",
	}
	c := NewClient(cfg)
	c.httpClient = &http.Client{}
	c.mu.Lock()
	c.accessToken = "test-access-token"
	c.tokenExpiry = time.Now().Add(1 * time.Hour)
	c.apiBase = baseURL
	c.mu.Unlock()
	return c
}

// tokenServerHandler returns an HTTP handler that serves a valid token response.
func tokenServerHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"access_token": "refreshed-access-token",
			"expires_in":   3600,
			"token_type":   "Bearer",
		})
	}
}

func TestNewClient_WithLastFMKey(t *testing.T) {
	cfg := &config.Config{
		SpotifyClientID:     "id",
		SpotifyClientSecret: "secret",
		SpotifyRefreshToken: "refresh",
		LastFMAPIKey:        "lfm-key",
	}
	c := NewClient(cfg)
	if c.lastfm == nil {
		t.Error("NewClient with LastFMAPIKey: lastfm client should not be nil")
	}
}

func TestClient_TokenExpiresIn(t *testing.T) {
	cfg := &config.Config{
		SpotifyClientID:     "id",
		SpotifyClientSecret: "secret",
		SpotifyRefreshToken: "refresh",
	}
	c := NewClient(cfg)

	// No token yet — TTL should be negative
	if got := c.TokenExpiresIn(); got > 0 {
		t.Errorf("expected non-positive TTL with no token, got %d", got)
	}

	// Set a token expiring in ~1 hour
	c.mu.Lock()
	c.accessToken = "tok"
	c.tokenExpiry = time.Now().Add(3600 * time.Second)
	c.mu.Unlock()

	if ttl := c.TokenExpiresIn(); ttl < 3500 || ttl > 3600 {
		t.Errorf("TokenExpiresIn() = %d, want ~3600", ttl)
	}
}

func TestClient_NowPlaying_NothingPlaying(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	c := clientForTest(t, srv.URL)
	track, err := c.NowPlaying(context.Background())
	if err != nil {
		t.Fatalf("NowPlaying() unexpected error: %v", err)
	}
	if track != nil {
		t.Errorf("NowPlaying() = %v, want nil (204 Nothing Playing)", track)
	}
}

func TestClient_NowPlaying_Paused(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"is_playing": false,
			"item": map[string]any{
				"id":   "track123",
				"name": "Some Song",
				"artists": []map[string]any{
					{"id": "artist1", "name": "Artist"},
				},
			},
		})
	}))
	defer srv.Close()

	c := clientForTest(t, srv.URL)
	track, err := c.NowPlaying(context.Background())
	if err != nil {
		t.Fatalf("NowPlaying() unexpected error: %v", err)
	}
	if track != nil {
		t.Errorf("NowPlaying() = %v, want nil (paused)", track)
	}
}

func TestClient_NowPlaying_WithTrack(t *testing.T) {
	mux := http.NewServeMux()

	mux.HandleFunc("/me/player/currently-playing", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"is_playing": true,
			"item": map[string]any{
				"id":   "track123",
				"name": "HUMBLE.",
				"artists": []map[string]any{
					{"id": "artist1", "name": "Kendrick Lamar"},
				},
			},
		})
	})

	mux.HandleFunc("/artists/artist1", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"genres": []string{"rap", "hip-hop", "west coast rap"},
		})
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := clientForTest(t, srv.URL)
	track, err := c.NowPlaying(context.Background())
	if err != nil {
		t.Fatalf("NowPlaying() error: %v", err)
	}
	if track == nil {
		t.Fatal("NowPlaying() = nil, want track")
	}
	if track.Title != "HUMBLE." {
		t.Errorf("Title = %q, want \"HUMBLE.\"", track.Title)
	}
	if track.Artist != "Kendrick Lamar" {
		t.Errorf("Artist = %q, want \"Kendrick Lamar\"", track.Artist)
	}
	if len(track.Genres) != 3 {
		t.Errorf("Genres len = %d, want 3", len(track.Genres))
	}
}

func TestClient_NowPlaying_UnexpectedStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := clientForTest(t, srv.URL)
	track, err := c.NowPlaying(context.Background())
	if err == nil {
		t.Error("NowPlaying() expected error on 500, got nil")
	}
	if track != nil {
		t.Errorf("NowPlaying() = %v, want nil on error", track)
	}
}

func TestClient_RefreshAccessToken_HappyPath(t *testing.T) {
	tokenSrv := httptest.NewServer(tokenServerHandler())
	defer tokenSrv.Close()

	cfg := &config.Config{
		SpotifyClientID:     "id",
		SpotifyClientSecret: "secret",
		SpotifyRefreshToken: "old-refresh",
	}
	c := NewClient(cfg)
	c.httpClient = &http.Client{}
	c.tokenURL = tokenSrv.URL

	if err := c.refreshAccessToken(context.Background()); err != nil {
		t.Fatalf("refreshAccessToken() error: %v", err)
	}

	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.accessToken != "refreshed-access-token" {
		t.Errorf("accessToken = %q, want \"refreshed-access-token\"", c.accessToken)
	}
	if c.tokenExpiry.Before(time.Now()) {
		t.Error("tokenExpiry should be in the future")
	}
}

func TestClient_RefreshAccessToken_ServerError(t *testing.T) {
	tokenSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer tokenSrv.Close()

	cfg := &config.Config{
		SpotifyClientID:     "id",
		SpotifyClientSecret: "secret",
		SpotifyRefreshToken: "bad-refresh",
	}
	c := NewClient(cfg)
	c.httpClient = &http.Client{}
	c.tokenURL = tokenSrv.URL

	if err := c.refreshAccessToken(context.Background()); err == nil {
		t.Error("refreshAccessToken() expected error on 401, got nil")
	}
}

func TestClient_EnsureToken_RefreshesWhenExpired(t *testing.T) {
	tokenSrv := httptest.NewServer(tokenServerHandler())
	defer tokenSrv.Close()

	cfg := &config.Config{
		SpotifyClientID:     "id",
		SpotifyClientSecret: "secret",
		SpotifyRefreshToken: "refresh",
	}
	c := NewClient(cfg)
	c.httpClient = &http.Client{}
	c.tokenURL = tokenSrv.URL
	// Set an expired token
	c.mu.Lock()
	c.accessToken = "old-token"
	c.tokenExpiry = time.Now().Add(-1 * time.Second)
	c.mu.Unlock()

	if err := c.ensureToken(context.Background()); err != nil {
		t.Fatalf("ensureToken() error: %v", err)
	}

	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.accessToken != "refreshed-access-token" {
		t.Errorf("after ensureToken, accessToken = %q, want refreshed token", c.accessToken)
	}
}

func TestClient_GetArtistGenres_EmptyID(t *testing.T) {
	// Empty artist ID should return nil, nil without making any HTTP call
	cfg := &config.Config{
		SpotifyClientID:     "id",
		SpotifyClientSecret: "secret",
		SpotifyRefreshToken: "refresh",
	}
	c := NewClient(cfg)
	genres, err := c.getArtistGenres(context.Background(), "")
	if err != nil {
		t.Fatalf("getArtistGenres(\"\") error: %v", err)
	}
	if genres != nil {
		t.Errorf("getArtistGenres(\"\") = %v, want nil", genres)
	}
}

func TestClient_NowPlaying_EnsureTokenError(t *testing.T) {
	// ensureToken fails when the token is expired and the token endpoint is unreachable
	tokenSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError) // token refresh fails
	}))
	defer tokenSrv.Close()

	cfg := &config.Config{
		SpotifyClientID:     "id",
		SpotifyClientSecret: "secret",
		SpotifyRefreshToken: "refresh",
	}
	c := NewClient(cfg)
	c.httpClient = &http.Client{}
	c.tokenURL = tokenSrv.URL
	// Force token to be expired so ensureToken calls refresh
	c.mu.Lock()
	c.accessToken = "expired"
	c.tokenExpiry = time.Now().Add(-1 * time.Second)
	c.mu.Unlock()

	track, err := c.NowPlaying(context.Background())
	if err == nil {
		t.Error("NowPlaying() expected error when ensureToken fails, got nil")
	}
	if track != nil {
		t.Errorf("NowPlaying() = %v, want nil on error", track)
	}
}

func TestClient_GetArtistGenres_UnexpectedStatus(t *testing.T) {
	mux := http.NewServeMux()

	mux.HandleFunc("/me/player/currently-playing", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"is_playing": true,
			"item": map[string]any{
				"id":   "track-artist-err",
				"name": "Test",
				"artists": []map[string]any{
					{"id": "bad-artist", "name": "Bad Artist"},
				},
			},
		})
	})

	// Artist endpoint returns 500 — should be treated as non-fatal warning
	mux.HandleFunc("/artists/bad-artist", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := clientForTest(t, srv.URL)
	track, err := c.NowPlaying(context.Background())
	if err != nil {
		t.Fatalf("NowPlaying() unexpected error: %v", err)
	}
	if track == nil {
		t.Fatal("NowPlaying() = nil, want track")
	}
	if len(track.Genres) != 0 {
		t.Errorf("Genres = %v, want empty on artist error", track.Genres)
	}
}

func TestClient_RefreshAccessToken_InvalidJSON(t *testing.T) {
	tokenSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("not-json"))
	}))
	defer tokenSrv.Close()

	cfg := &config.Config{
		SpotifyClientID:     "id",
		SpotifyClientSecret: "secret",
		SpotifyRefreshToken: "refresh",
	}
	c := NewClient(cfg)
	c.httpClient = &http.Client{}
	c.tokenURL = tokenSrv.URL

	if err := c.refreshAccessToken(context.Background()); err == nil {
		t.Error("refreshAccessToken() expected error on invalid JSON, got nil")
	}
}

func TestClient_GetNowPlaying_InvalidJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("{invalid json"))
	}))
	defer srv.Close()

	c := clientForTest(t, srv.URL)
	track, err := c.getNowPlaying(context.Background())
	if err == nil {
		t.Error("getNowPlaying() expected error on invalid JSON, got nil")
	}
	if track != nil {
		t.Errorf("getNowPlaying() = %v, want nil on error", track)
	}
}

func TestClient_NowPlaying_401ReauthFailed(t *testing.T) {
	// First call returns 401; token refresh then fails
	tokenSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized) // token refresh fails
	}))
	defer tokenSrv.Close()

	apiSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer apiSrv.Close()

	cfg := &config.Config{
		SpotifyClientID:     "id",
		SpotifyClientSecret: "secret",
		SpotifyRefreshToken: "refresh",
	}
	c := NewClient(cfg)
	c.httpClient = &http.Client{}
	c.apiBase = apiSrv.URL
	c.tokenURL = tokenSrv.URL
	c.mu.Lock()
	c.accessToken = "expired-token"
	c.tokenExpiry = time.Now().Add(1 * time.Hour)
	c.mu.Unlock()

	track, err := c.NowPlaying(context.Background())
	if err == nil {
		t.Error("NowPlaying() expected error when reauth fails after 401, got nil")
	}
	if track != nil {
		t.Errorf("NowPlaying() = %v, want nil", track)
	}
}

func TestClient_GetArtistGenres_InvalidJSON(t *testing.T) {
	mux := http.NewServeMux()

	mux.HandleFunc("/me/player/currently-playing", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"is_playing": true,
			"item": map[string]any{
				"id":   "trackbad2",
				"name": "Bad JSON Artist",
				"artists": []map[string]any{
					{"id": "badartist", "name": "Bad"},
				},
			},
		})
	})

	mux.HandleFunc("/artists/badartist", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("{bad json"))
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := clientForTest(t, srv.URL)
	// Artist genres parse error is non-fatal — track should still be returned
	track, err := c.NowPlaying(context.Background())
	if err != nil {
		t.Fatalf("NowPlaying() unexpected error on artist bad JSON: %v", err)
	}
	if track == nil {
		t.Fatal("NowPlaying() = nil, want track")
	}
	if len(track.Genres) != 0 {
		t.Errorf("Genres = %v, want empty on parse error", track.Genres)
	}
}

func TestClient_NowPlaying_401Retry(t *testing.T) {
	npCallCount := 0
	tokenSrv := httptest.NewServer(tokenServerHandler())
	defer tokenSrv.Close()

	apiMux := http.NewServeMux()
	apiMux.HandleFunc("/me/player/currently-playing", func(w http.ResponseWriter, _ *http.Request) {
		npCallCount++
		if npCallCount == 1 {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	// recently-played returns 403 (scope not granted) — NowPlaying returns nil, nil
	apiMux.HandleFunc("/me/player/recently-played", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	})
	apiSrv := httptest.NewServer(apiMux)
	defer apiSrv.Close()

	cfg := &config.Config{
		SpotifyClientID:     "id",
		SpotifyClientSecret: "secret",
		SpotifyRefreshToken: "refresh",
	}
	c := NewClient(cfg)
	c.httpClient = &http.Client{}
	c.apiBase = apiSrv.URL
	c.tokenURL = tokenSrv.URL
	// Start with a valid-looking token that will get a 401
	c.mu.Lock()
	c.accessToken = "expired-token"
	c.tokenExpiry = time.Now().Add(1 * time.Hour)
	c.mu.Unlock()

	track, err := c.NowPlaying(context.Background())
	if err != nil {
		t.Fatalf("NowPlaying() 401 retry: unexpected error: %v", err)
	}
	if track != nil {
		t.Errorf("NowPlaying() = %v, want nil (204 after retry)", track)
	}
	if npCallCount != 2 {
		t.Errorf("expected 2 currently-playing calls (initial 401 + retry), got %d", npCallCount)
	}
}

func TestClient_NowPlaying_RecentlyPlayedFallback(t *testing.T) {
	mux := http.NewServeMux()

	// Nothing currently playing
	mux.HandleFunc("/me/player/currently-playing", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	// Recently played returns a track
	mux.HandleFunc("/me/player/recently-played", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"items": []map[string]any{
				{
					"track": map[string]any{
						"id":   "recent-track",
						"name": "HORA CERO",
						"artists": []map[string]any{
							{"id": "artist-recent", "name": "Myke Towers"},
						},
					},
				},
			},
		})
	})

	mux.HandleFunc("/artists/artist-recent", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"genres": []string{"reggaeton", "trap latino"}})
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := clientForTest(t, srv.URL)
	track, err := c.NowPlaying(context.Background())
	if err != nil {
		t.Fatalf("NowPlaying() error: %v", err)
	}
	if track == nil {
		t.Fatal("NowPlaying() = nil, want recently-played track")
	}
	if !track.IsRecentlyPlayed {
		t.Error("IsRecentlyPlayed = false, want true")
	}
	if track.Title != "HORA CERO" {
		t.Errorf("Title = %q, want \"HORA CERO\"", track.Title)
	}
	if len(track.Genres) == 0 {
		t.Error("Genres empty, want reggaeton/trap latino")
	}
}

func TestClient_NowPlaying_RecentlyPlayedForbidden(t *testing.T) {
	mux := http.NewServeMux()

	mux.HandleFunc("/me/player/currently-playing", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	// 403 = missing scope
	mux.HandleFunc("/me/player/recently-played", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := clientForTest(t, srv.URL)
	track, err := c.NowPlaying(context.Background())
	if err != nil {
		t.Fatalf("NowPlaying() unexpected error on 403 recently-played: %v", err)
	}
	if track != nil {
		t.Errorf("NowPlaying() = %v, want nil when scope missing", track)
	}
}

func TestClient_NowPlaying_RecentlyPlayedInvalidJSON(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/me/player/currently-playing", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("/me/player/recently-played", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("{bad json"))
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := clientForTest(t, srv.URL)
	// Parse error is fatal for recently-played → NowPlaying returns nil, error
	track, err := c.NowPlaying(context.Background())
	if err != nil {
		// The error from getRecentlyPlayed is logged as a warning; NowPlaying returns nil, nil
		t.Fatalf("NowPlaying() unexpected error: %v", err)
	}
	if track != nil {
		t.Errorf("NowPlaying() = %v, want nil on recently-played parse error", track)
	}
}

func TestClient_NowPlaying_RecentlyPlayedEmpty(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/me/player/currently-playing", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("/me/player/recently-played", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"items": []any{}})
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := clientForTest(t, srv.URL)
	track, err := c.NowPlaying(context.Background())
	if err != nil {
		t.Fatalf("NowPlaying() error: %v", err)
	}
	if track != nil {
		t.Errorf("NowPlaying() = %v, want nil for empty recently-played", track)
	}
}

func TestClient_NowPlaying_LastFMFallback(t *testing.T) {
	// Spotify returns a track but artist has no genres — Last.fm fallback should kick in.
	spotifySrv := httptest.NewServer(http.NewServeMux())
	defer spotifySrv.Close()

	spotifyMux := http.NewServeMux()
	spotifyMux.HandleFunc("/me/player/currently-playing", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"is_playing": true,
			"item": map[string]any{
				"id":   "track-lfm",
				"name": "HORA CERO",
				"artists": []map[string]any{
					{"id": "artist-lfm", "name": "Myke Towers"},
				},
			},
		})
	})
	spotifyMux.HandleFunc("/artists/artist-lfm", func(w http.ResponseWriter, _ *http.Request) {
		// Spotify returns empty genres
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"genres": []string{}})
	})

	spotifySrv2 := httptest.NewServer(spotifyMux)
	defer spotifySrv2.Close()

	// Last.fm stub
	lastfmSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"toptags": map[string]any{
				"tag": []map[string]any{
					{"name": "reggaeton", "count": 100},
					{"name": "trap latino", "count": 80},
				},
			},
		})
	}))
	defer lastfmSrv.Close()

	c := clientForTest(t, spotifySrv2.URL)
	c.lastfm = newLastFMClient("test-key")
	c.lastfm.httpClient = &http.Client{}
	c.lastfm.apiBase = lastfmSrv.URL + "/"

	track, err := c.NowPlaying(context.Background())
	if err != nil {
		t.Fatalf("NowPlaying() error: %v", err)
	}
	if track == nil {
		t.Fatal("NowPlaying() = nil, want track")
	}
	if len(track.Genres) != 2 {
		t.Errorf("Genres len = %d, want 2 (from Last.fm)", len(track.Genres))
	}
	if track.Genres[0] != "reggaeton" {
		t.Errorf("Genres[0] = %q, want \"reggaeton\"", track.Genres[0])
	}
}
