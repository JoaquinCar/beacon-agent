package delivery

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/joako/beacon/internal/briefing"
	"github.com/joako/beacon/internal/config"
	"github.com/joako/beacon/internal/papers"
)

func makeCfg(dryRun bool) *config.Config {
	return &config.Config{
		ResendAPIKey:    "test-key",
		DeliveryEmailTo: "user@example.com",
		DryRun:          dryRun,
	}
}

func makeBriefing(mode briefing.Mode) briefing.Briefing {
	return briefing.Briefing{
		Mode:        mode,
		GeneratedAt: time.Date(2024, 1, 15, 9, 0, 0, 0, time.UTC),
		Sections: []briefing.Section{
			{
				Paper: papers.Paper{
					Title:   "Test Paper",
					Authors: []string{"Alice"},
					Source:  "arxiv",
					Date:    time.Date(2024, 1, 10, 0, 0, 0, 0, time.UTC),
					URL:     "https://arxiv.org/abs/1",
				},
				Body: "Summary body text.",
			},
		},
	}
}

func TestEmailSender_DryRun_NoHTTPCall(t *testing.T) {
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	s := NewEmailSender(makeCfg(true))
	s.apiBase = srv.URL

	if err := s.Send(context.Background(), makeBriefing(briefing.ModeSummary)); err != nil {
		t.Fatalf("Send dry-run: %v", err)
	}
	if called {
		t.Error("dry-run should not make HTTP call")
	}
}

func TestEmailSender_Send_Success(t *testing.T) {
	var gotReq *http.Request
	var gotBody []byte

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotReq = r
		var err error
		gotBody, err = io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read body: %v", err)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"abc"}`))
	}))
	defer srv.Close()

	s := NewEmailSender(makeCfg(false))
	s.apiBase = srv.URL

	if err := s.Send(context.Background(), makeBriefing(briefing.ModeSummary)); err != nil {
		t.Fatalf("Send: %v", err)
	}

	// Verify Authorization header.
	if !strings.HasPrefix(gotReq.Header.Get("Authorization"), "Bearer ") {
		t.Errorf("missing Bearer auth header: %q", gotReq.Header.Get("Authorization"))
	}

	// Verify JSON payload.
	var payload map[string]any
	if err := json.Unmarshal(gotBody, &payload); err != nil {
		t.Fatalf("unmarshal body: %v", err)
	}
	if payload["to"] == nil {
		t.Error("missing 'to' field in payload")
	}
	if payload["subject"] == nil {
		t.Error("missing 'subject' field in payload")
	}
	if payload["text"] == nil {
		t.Error("missing 'text' field in payload")
	}
}

func TestEmailSender_Send_APIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"message":"invalid api key"}`))
	}))
	defer srv.Close()

	s := NewEmailSender(makeCfg(false))
	s.apiBase = srv.URL

	err := s.Send(context.Background(), makeBriefing(briefing.ModeSummary))
	if err == nil {
		t.Fatal("expected error for 401 response")
	}
	if !strings.Contains(err.Error(), "401") {
		t.Errorf("error should mention status 401: %v", err)
	}
}

func TestEmailSender_SubjectIncludesDate(t *testing.T) {
	var gotSubject string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]any
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &payload)
		gotSubject, _ = payload["subject"].(string)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	s := NewEmailSender(makeCfg(false))
	s.apiBase = srv.URL

	_ = s.Send(context.Background(), makeBriefing(briefing.ModeFull))

	if !strings.Contains(gotSubject, "2024-01-15") {
		t.Errorf("subject should contain date, got: %q", gotSubject)
	}
}
