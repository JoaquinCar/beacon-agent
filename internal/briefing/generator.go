package briefing

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net/http"
	"strings"
	"time"

	"github.com/joako/beacon/internal/papers"
)

const (
	anthropicAPIBase = "https://api.anthropic.com/v1/messages"
	anthropicVersion = "2023-06-01"
	claudeModel      = "claude-sonnet-4-20250514"

	maxTokensSummary = 600  // HIGH_BPM: 5–8 lines
	maxTokensFull    = 1500 // NORMAL: structured analysis

	maxRetries   = 3
	retryBase    = 200 * time.Millisecond
	defaultDelay = 200 * time.Millisecond // between papers — never concurrent
)

// Generator calls the Claude API to produce briefing sections for each paper.
type Generator struct {
	apiKey     string
	apiBase    string
	httpClient *http.Client
	paperDelay time.Duration
}

// NewGenerator returns a Generator that uses the given Anthropic API key.
func NewGenerator(apiKey string) *Generator {
	return &Generator{
		apiKey:     apiKey,
		apiBase:    anthropicAPIBase,
		httpClient: &http.Client{Timeout: 60 * time.Second},
		paperDelay: defaultDelay,
	}
}

// Generate processes each paper sequentially and returns a Briefing.
// Individual paper failures are logged and skipped; the call only returns an
// error if no sections could be generated at all.
func (g *Generator) Generate(ctx context.Context, ps []papers.Paper, mode Mode) (Briefing, error) {
	b := Briefing{
		Mode:        mode,
		GeneratedAt: time.Now(),
	}

	for i, p := range ps {
		sec, err := g.generateSection(ctx, p, mode)
		if err != nil {
			slog.Warn("briefing: skipping paper after failed generation",
				"title", p.Title, "err", err)
			continue
		}
		b.Sections = append(b.Sections, sec)

		// 200ms between papers — never send concurrent Claude requests.
		if i < len(ps)-1 {
			select {
			case <-time.After(g.paperDelay):
			case <-ctx.Done():
				return b, ctx.Err()
			}
		}
	}

	if len(b.Sections) == 0 && len(ps) > 0 {
		return b, fmt.Errorf("briefing: all %d paper(s) failed generation", len(ps))
	}

	return b, nil
}

// generateSection calls Claude for a single paper, with retry on 429/5xx.
func (g *Generator) generateSection(ctx context.Context, p papers.Paper, mode Mode) (Section, error) {
	prompt := buildPrompt(p, mode)
	maxTok := maxTokensFull
	if mode == ModeSummary {
		maxTok = maxTokensSummary
	}

	var text string
	err := withRetry(ctx, maxRetries, retryBase, func() error {
		var callErr error
		text, callErr = g.callClaude(ctx, prompt, maxTok)
		return callErr
	})
	if err != nil {
		return Section{}, fmt.Errorf("briefing: generate section for %q: %w", p.Title, err)
	}

	sec := Section{Paper: p}
	if mode == ModeFull {
		sec.TLDr, sec.Body = parseTLDr(text)
	} else {
		sec.Body = strings.TrimSpace(text)
	}
	return sec, nil
}

// callClaude sends one request to the Anthropic Messages API.
func (g *Generator) callClaude(ctx context.Context, prompt string, maxTokens int) (string, error) {
	reqBody, err := json.Marshal(anthropicRequest{
		Model:     claudeModel,
		MaxTokens: maxTokens,
		Messages:  []anthropicMessage{{Role: "user", Content: prompt}},
	})
	if err != nil {
		return "", fmt.Errorf("briefing: marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, g.apiBase, bytes.NewReader(reqBody))
	if err != nil {
		return "", fmt.Errorf("briefing: build request: %w", err)
	}
	req.Header.Set("x-api-key", g.apiKey)
	req.Header.Set("anthropic-version", anthropicVersion)
	req.Header.Set("content-type", "application/json")

	resp, err := g.httpClient.Do(req)
	if resp != nil {
		defer resp.Body.Close()
	}
	if err != nil {
		return "", fmt.Errorf("briefing: request: %w", err)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 5<<20))
	if err != nil {
		return "", fmt.Errorf("briefing: read response: %w", err)
	}

	switch resp.StatusCode {
	case http.StatusOK:
		// proceed
	case http.StatusTooManyRequests, http.StatusServiceUnavailable:
		return "", &retryableError{status: resp.StatusCode, body: string(body)}
	default:
		if resp.StatusCode >= 500 {
			return "", &retryableError{status: resp.StatusCode, body: string(body)}
		}
		return "", fmt.Errorf("briefing: claude API status %d: %s", resp.StatusCode, body)
	}

	var ar anthropicResponse
	if err := json.Unmarshal(body, &ar); err != nil {
		return "", fmt.Errorf("briefing: parse response: %w", err)
	}
	if len(ar.Content) == 0 {
		return "", fmt.Errorf("briefing: empty response from claude")
	}
	return ar.Content[0].Text, nil
}

// buildPrompt returns the system prompt for the given mode.
func buildPrompt(p papers.Paper, mode Mode) string {
	authors := strings.Join(p.Authors, ", ")
	if len(p.Authors) > 3 {
		authors = strings.Join(p.Authors[:3], ", ") + " et al."
	}

	if mode == ModeSummary {
		return fmt.Sprintf(`Analyze this academic paper.

Write a concise summary of 5 to 8 lines covering:
1. What the paper proposes or does (1–2 lines)
2. The key finding or main contribution (2–3 lines)
3. Why it matters in practice (1–2 lines)

Be direct. Do not start sentences with "This paper". No filler phrases.

Title: %s
Authors: %s
Abstract: %s`, p.Title, authors, p.Abstract)
	}

	return fmt.Sprintf(`Analyze this academic paper in two parts.

First line must be exactly in this format (fill in the brackets):
TL;DR: [one sentence: what this paper does and why it matters]

Then leave a blank line and write a structured analysis with these labeled sections:
Motivation: [what problem does this solve?]
Method: [how do they approach it?]
Results: [what did they find or achieve?]
Implications: [what does this mean for the field?]
Caveats: [what are the limitations or open questions?]

Title: %s
Authors: %s
Abstract: %s`, p.Title, authors, p.Abstract)
}

// parseTLDr splits a NORMAL-mode Claude response into TL;DR and body.
func parseTLDr(text string) (tldr, body string) {
	text = strings.TrimSpace(text)
	if !strings.HasPrefix(text, "TL;DR:") {
		return "", text
	}
	parts := strings.SplitN(text, "\n\n", 2)
	tldr = strings.TrimSpace(strings.TrimPrefix(parts[0], "TL;DR:"))
	if len(parts) == 2 {
		body = strings.TrimSpace(parts[1])
	}
	return tldr, body
}

// withRetry runs fn up to maxAttempts times, backing off on retryableError.
func withRetry(ctx context.Context, maxAttempts int, base time.Duration, fn func() error) error {
	var err error
	for i := range maxAttempts {
		err = fn()
		if err == nil {
			return nil
		}
		var re *retryableError
		if !isRetryable(err, &re) {
			return err
		}
		if i < maxAttempts-1 {
			wait := time.Duration(math.Pow(2, float64(i))) * base
			slog.Warn("briefing: retrying after error", "attempt", i+1, "wait", wait, "err", err)
			select {
			case <-time.After(wait):
			case <-ctx.Done():
				return ctx.Err()
			}
		}
	}
	return fmt.Errorf("briefing: max retries reached: %w", err)
}

func isRetryable(err error, re **retryableError) bool {
	var r retryableError
	if strings.Contains(err.Error(), "retryable:") {
		return true
	}
	_ = re
	_ = r
	var ok bool
	*re, ok = err.(*retryableError)
	return ok
}

// retryableError wraps HTTP errors that should trigger a retry (429, 5xx).
type retryableError struct {
	status int
	body   string
}

func (e *retryableError) Error() string {
	return fmt.Sprintf("retryable: claude API status %d", e.status)
}

// Anthropic API wire types.
type anthropicRequest struct {
	Model     string             `json:"model"`
	MaxTokens int                `json:"max_tokens"`
	Messages  []anthropicMessage `json:"messages"`
}

type anthropicMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type anthropicResponse struct {
	Content []anthropicContent `json:"content"`
}

type anthropicContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}
