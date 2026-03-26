//go:build ignore

// spotify-auth is a one-time script that runs the Spotify Authorization Code
// flow and prints the refresh token to stdout.
//
// Usage:
//
//	go run scripts/spotify-auth.go
//
// Before running, add http://127.0.0.1:8888/callback as a Redirect URI in
// your Spotify app dashboard: https://developer.spotify.com/dashboard
package main

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

const (
	redirectURI = "http://127.0.0.1:8888/callback"
	authURL     = "https://accounts.spotify.com/authorize"
	tokenURL    = "https://accounts.spotify.com/api/token"
	scopes      = "user-read-playback-state user-read-currently-playing user-read-recently-played"
)

func main() {
	if err := godotenv.Load(); err != nil && !os.IsNotExist(err) {
		fatalf("load .env: %v", err)
	}

	clientID := os.Getenv("SPOTIFY_CLIENT_ID")
	clientSecret := os.Getenv("SPOTIFY_CLIENT_SECRET")

	if clientID == "" {
		fatalf("SPOTIFY_CLIENT_ID is not set in .env")
	}
	if clientSecret == "" {
		fatalf("SPOTIFY_CLIENT_SECRET is not set in .env")
	}

	state, err := randomState()
	if err != nil {
		fatalf("generate state: %v", err)
	}

	params := url.Values{
		"client_id":     {clientID},
		"response_type": {"code"},
		"redirect_uri":  {redirectURI},
		"scope":         {scopes},
		"state":         {state},
	}
	authorizationURL := authURL + "?" + params.Encode()

	codeCh := make(chan string, 1)
	errCh := make(chan error, 1)

	mux := http.NewServeMux()
	srv := &http.Server{Addr: "127.0.0.1:8888", Handler: mux}

	mux.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()

		if q.Get("state") != state {
			errCh <- fmt.Errorf("state mismatch — possible CSRF attack")
			http.Error(w, "state mismatch", http.StatusBadRequest)
			return
		}

		if e := q.Get("error"); e != "" {
			errCh <- fmt.Errorf("spotify denied authorization: %s", e)
			fmt.Fprintf(w, "<html><body><h2>Authorization denied: %s</h2><p>You can close this tab.</p></body></html>", e)
			return
		}

		code := q.Get("code")
		if code == "" {
			errCh <- fmt.Errorf("no code parameter in callback URL")
			http.Error(w, "missing code", http.StatusBadRequest)
			return
		}

		fmt.Fprint(w, "<html><body><h2>✅ Authorized!</h2><p>Return to your terminal — you can close this tab.</p></body></html>")
		codeCh <- code
	})

	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- fmt.Errorf("local server: %w", err)
		}
	}()

	fmt.Println("Opening Spotify authorization page in your browser...")
	fmt.Printf("\nIf it doesn't open automatically, paste this URL:\n%s\n\n", authorizationURL)
	openBrowser(authorizationURL)
	fmt.Println("Waiting for authorization (5 min timeout)...")

	var code string
	select {
	case code = <-codeCh:
	case authErr := <-errCh:
		fatalf("%v", authErr)
	case <-time.After(5 * time.Minute):
		fatalf("timed out waiting for Spotify callback")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = srv.Shutdown(shutdownCtx)

	refreshToken, err := exchangeCode(clientID, clientSecret, code)
	if err != nil {
		fatalf("exchange code: %v", err)
	}

	fmt.Printf("\n✅ Success! Add this to your .env:\n\nSPOTIFY_REFRESH_TOKEN=%s\n", refreshToken)
}

func exchangeCode(clientID, clientSecret, code string) (string, error) {
	form := url.Values{
		"grant_type":   {"authorization_code"},
		"code":         {code},
		"redirect_uri": {redirectURI},
	}

	req, err := http.NewRequest(http.MethodPost, tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString(
		[]byte(clientID+":"+clientSecret),
	))

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("token request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("unexpected status %d: %s", resp.StatusCode, body)
	}

	var result struct {
		RefreshToken string `json:"refresh_token"`
		Scope        string `json:"scope"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("parse response: %w", err)
	}
	if result.RefreshToken == "" {
		return "", fmt.Errorf("no refresh_token in response (check scopes)")
	}

	return result.RefreshToken, nil
}

func openBrowser(u string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", u)
	case "darwin":
		cmd = exec.Command("open", u)
	default:
		cmd = exec.Command("xdg-open", u)
	}
	_ = cmd.Start()
}

func randomState() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(b), nil
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "error: "+format+"\n", args...)
	os.Exit(1)
}
