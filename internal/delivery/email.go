package delivery

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/joako/beacon/internal/briefing"
	"github.com/joako/beacon/internal/config"
)

const resendAPIBase = "https://api.resend.com/emails"

// EmailSender delivers briefings via the Resend email API.
type EmailSender struct {
	cfg        *config.Config
	httpClient *http.Client
	apiBase    string // overridable in tests
}

// NewEmailSender returns an EmailSender using the provided config.
func NewEmailSender(cfg *config.Config) *EmailSender {
	return &EmailSender{
		cfg:        cfg,
		httpClient: &http.Client{Timeout: 15 * time.Second},
		apiBase:    resendAPIBase,
	}
}

// Send formats the briefing and delivers it via email.
// If cfg.DryRun is true the briefing is printed to stdout and no network call is made.
func (e *EmailSender) Send(ctx context.Context, b briefing.Briefing) error {
	body := briefing.Format(b)

	if e.cfg.DryRun {
		slog.Info("delivery: dry-run — email not sent",
			"to", e.cfg.DeliveryEmailTo,
			"mode", b.Mode,
			"sections", len(b.Sections),
		)
		fmt.Println("=== DRY-RUN email ===")
		fmt.Println(body)
		fmt.Println("=== end ===")
		return nil
	}

	type resendPayload struct {
		From    string   `json:"from"`
		To      []string `json:"to"`
		Subject string   `json:"subject"`
		Text    string   `json:"text"`
	}

	payload := resendPayload{
		From:    "Beacon <beacon@resend.dev>",
		To:      []string{e.cfg.DeliveryEmailTo},
		Subject: fmt.Sprintf("Beacon briefing — %s [%s]", b.GeneratedAt.Format("2006-01-02"), b.Mode),
		Text:    body,
	}

	reqBody, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("delivery: marshal email payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, e.apiBase, bytes.NewReader(reqBody))
	if err != nil {
		return fmt.Errorf("delivery: build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+e.cfg.ResendAPIKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := e.httpClient.Do(req)
	if resp != nil {
		defer resp.Body.Close()
	}
	if err != nil {
		return fmt.Errorf("delivery: send request: %w", err)
	}

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return fmt.Errorf("delivery: read response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("delivery: resend API status %d: %s", resp.StatusCode, respBody)
	}

	slog.Info("delivery: email sent",
		"to", e.cfg.DeliveryEmailTo,
		"mode", b.Mode,
		"sections", len(b.Sections),
		"status", resp.StatusCode,
	)
	return nil
}
