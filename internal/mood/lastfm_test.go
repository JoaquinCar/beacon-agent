package mood

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func lastfmClientForTest(t *testing.T, baseURL string) *lastFMClient {
	t.Helper()
	c := newLastFMClient("test-api-key")
	c.httpClient = &http.Client{}
	c.apiBase = baseURL + "/"
	return c
}

func TestLastFM_GetTags_TrackError(t *testing.T) {
	// getTrackTags errors → GetTags propagates the error without calling artist fallback
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := lastfmClientForTest(t, srv.URL)
	_, err := c.GetTags(context.Background(), "Artist", "Track")
	if err == nil {
		t.Error("GetTags() expected error when track lookup fails, got nil")
	}
}

func TestLastFM_GetTrackTags_HappyPath(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"toptags": map[string]any{
				"tag": []map[string]any{
					{"name": "reggaeton", "count": 100},
					{"name": "trap latino", "count": 80},
					{"name": "latin pop", "count": 60},
				},
			},
		})
	}))
	defer srv.Close()

	c := lastfmClientForTest(t, srv.URL)
	tags, err := c.getTrackTags(context.Background(), "Myke Towers", "HORA CERO")
	if err != nil {
		t.Fatalf("getTrackTags() error: %v", err)
	}
	if len(tags) != 3 {
		t.Errorf("len(tags) = %d, want 3", len(tags))
	}
	if tags[0] != "reggaeton" {
		t.Errorf("tags[0] = %q, want \"reggaeton\"", tags[0])
	}
}

func TestLastFM_GetTrackTags_FiltersZeroCount(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"toptags": map[string]any{
				"tag": []map[string]any{
					{"name": "rap", "count": 50},
					{"name": "noise", "count": 0},
				},
			},
		})
	}))
	defer srv.Close()

	c := lastfmClientForTest(t, srv.URL)
	tags, err := c.getTrackTags(context.Background(), "Artist", "Track")
	if err != nil {
		t.Fatalf("getTrackTags() error: %v", err)
	}
	if len(tags) != 1 || tags[0] != "rap" {
		t.Errorf("tags = %v, want [\"rap\"] (zero-count filtered)", tags)
	}
}

func TestLastFM_GetTrackTags_TrackNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"error":   6,
			"message": "Track not found",
		})
	}))
	defer srv.Close()

	c := lastfmClientForTest(t, srv.URL)
	tags, err := c.getTrackTags(context.Background(), "Unknown", "Unknown")
	if err != nil {
		t.Fatalf("getTrackTags() should not return error on track-not-found, got: %v", err)
	}
	if tags != nil {
		t.Errorf("tags = %v, want nil for not-found", tags)
	}
}

func TestLastFM_GetTrackTags_UnexpectedStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := lastfmClientForTest(t, srv.URL)
	_, err := c.getTrackTags(context.Background(), "Artist", "Track")
	if err == nil {
		t.Error("getTrackTags() expected error on 500, got nil")
	}
}

func TestLastFM_GetTrackTags_InvalidJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("{bad json"))
	}))
	defer srv.Close()

	c := lastfmClientForTest(t, srv.URL)
	_, err := c.getTrackTags(context.Background(), "Artist", "Track")
	if err == nil {
		t.Error("getTrackTags() expected error on invalid JSON, got nil")
	}
}

func TestLastFM_GetTags_FallsBackToArtist(t *testing.T) {
	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		method := r.URL.Query().Get("method")
		w.Header().Set("Content-Type", "application/json")
		if method == "track.getTopTags" {
			// Track has no tags
			json.NewEncoder(w).Encode(map[string]any{
				"toptags": map[string]any{"tag": []any{}},
			})
			return
		}
		// Artist tags
		json.NewEncoder(w).Encode(map[string]any{
			"toptags": map[string]any{
				"tag": []map[string]any{
					{"name": "reggaeton", "count": 100},
				},
			},
		})
	}))
	defer srv.Close()

	c := lastfmClientForTest(t, srv.URL)
	tags, err := c.GetTags(context.Background(), "Myke Towers", "HORA CERO")
	if err != nil {
		t.Fatalf("GetTags() error: %v", err)
	}
	if len(tags) != 1 || tags[0] != "reggaeton" {
		t.Errorf("GetTags() = %v, want [reggaeton] from artist fallback", tags)
	}
	if callCount != 2 {
		t.Errorf("expected 2 calls (track + artist), got %d", callCount)
	}
}

func TestLastFM_GetTags_UsesTrackWhenAvailable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"toptags": map[string]any{
				"tag": []map[string]any{{"name": "rap", "count": 50}},
			},
		})
	}))
	defer srv.Close()

	c := lastfmClientForTest(t, srv.URL)
	tags, err := c.GetTags(context.Background(), "Kendrick", "HUMBLE.")
	if err != nil {
		t.Fatalf("GetTags() error: %v", err)
	}
	if len(tags) == 0 || tags[0] != "rap" {
		t.Errorf("GetTags() = %v, want [rap]", tags)
	}
}

func TestLastFM_GetArtistTags_UnexpectedStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := lastfmClientForTest(t, srv.URL)
	_, err := c.getArtistTags(context.Background(), "Artist")
	if err == nil {
		t.Error("getArtistTags() expected error on 500, got nil")
	}
}

func TestLastFM_GetArtistTags_InvalidJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("{bad json"))
	}))
	defer srv.Close()

	c := lastfmClientForTest(t, srv.URL)
	_, err := c.getArtistTags(context.Background(), "Artist")
	if err == nil {
		t.Error("getArtistTags() expected error on invalid JSON, got nil")
	}
}

func TestLastFM_GetArtistTags_APIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"error": 6, "message": "Artist not found"})
	}))
	defer srv.Close()

	c := lastfmClientForTest(t, srv.URL)
	tags, err := c.getArtistTags(context.Background(), "Unknown")
	if err != nil {
		t.Fatalf("getArtistTags() error on API error: %v", err)
	}
	if tags != nil {
		t.Errorf("getArtistTags() = %v, want nil on API error", tags)
	}
}

func TestLastFM_GetArtistTags_EmptyArtist(t *testing.T) {
	c := lastfmClientForTest(t, "http://unused")
	tags, err := c.getArtistTags(context.Background(), "")
	if err != nil {
		t.Fatalf("getArtistTags(\"\") error: %v", err)
	}
	if tags != nil {
		t.Errorf("getArtistTags(\"\") = %v, want nil", tags)
	}
}

func TestLastFM_GetTags_ArtistFallbackError(t *testing.T) {
	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Query().Get("method") == "track.getTopTags" {
			json.NewEncoder(w).Encode(map[string]any{
				"toptags": map[string]any{"tag": []any{}},
			})
			return
		}
		// artist fallback returns error
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := lastfmClientForTest(t, srv.URL)
	_, err := c.GetTags(context.Background(), "Artist", "Track")
	if err == nil {
		t.Error("GetTags() expected error when artist fallback fails, got nil")
	}
}

func TestLastFM_GetTrackTags_EmptyTags(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"toptags": map[string]any{
				"tag": []map[string]any{},
			},
		})
	}))
	defer srv.Close()

	c := lastfmClientForTest(t, srv.URL)
	tags, err := c.getTrackTags(context.Background(), "Artist", "Track")
	if err != nil {
		t.Fatalf("getTrackTags() error: %v", err)
	}
	if len(tags) != 0 {
		t.Errorf("tags = %v, want empty", tags)
	}
}
