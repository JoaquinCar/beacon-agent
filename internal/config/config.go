package config

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strconv"

	"github.com/joho/godotenv"
)

// Config holds all runtime configuration loaded from environment variables.
type Config struct {
	// Spotify OAuth credentials
	SpotifyClientID     string
	SpotifyClientSecret string
	SpotifyRefreshToken string

	// Anthropic API
	AnthropicAPIKey string

	// Email delivery — required. Use Resend or SendGrid.
	ResendAPIKey    string // required if SendGridAPIKey is empty
	SendGridAPIKey  string // required if ResendAPIKey is empty
	DeliveryEmailTo string // required

	// Last.fm — optional, used as genre fallback when Spotify returns empty genres
	LastFMAPIKey string

	// Telegram delivery — optional
	TelegramBotToken string
	TelegramChatID   string

	// App settings
	Timezone string // default: "America/Merida"
	LogLevel string // default: "info"
	DryRun   bool   // default: false
}

// Load reads configuration from environment variables.
// It attempts to load a .env file if present, but does not fail if absent
// (env vars may be set by the OS in production).
func Load() (*Config, error) {
	if err := godotenv.Load(); err != nil && !os.IsNotExist(err) {
		// File exists but could not be parsed
		return nil, fmt.Errorf("config: failed to parse .env: %w", err)
	}

	cfg := &Config{
		SpotifyClientID:     os.Getenv("SPOTIFY_CLIENT_ID"),
		SpotifyClientSecret: os.Getenv("SPOTIFY_CLIENT_SECRET"),
		SpotifyRefreshToken: os.Getenv("SPOTIFY_REFRESH_TOKEN"),
		AnthropicAPIKey:     os.Getenv("ANTHROPIC_API_KEY"),
		LastFMAPIKey:        os.Getenv("LASTFM_API_KEY"),
		TelegramBotToken:    os.Getenv("TELEGRAM_BOT_TOKEN"),
		TelegramChatID:      os.Getenv("TELEGRAM_CHAT_ID"),
		ResendAPIKey:        os.Getenv("RESEND_API_KEY"),
		SendGridAPIKey:      os.Getenv("SENDGRID_API_KEY"),
		DeliveryEmailTo:     os.Getenv("DELIVERY_EMAIL_TO"),
		Timezone:            getEnvOrDefault("TIMEZONE", "America/Merida"),
		LogLevel:            getEnvOrDefault("LOG_LEVEL", "info"),
	}

	dryRun, err := strconv.ParseBool(os.Getenv("DRY_RUN"))
	if err == nil {
		cfg.DryRun = dryRun
	}

	if err := cfg.validate(); err != nil {
		return nil, err
	}

	return cfg, nil
}

// validate checks that all required fields are set.
func (c *Config) validate() error {
	var errs []error

	// Core required fields
	for _, r := range []struct{ name, value string }{
		{"SPOTIFY_CLIENT_ID", c.SpotifyClientID},
		{"SPOTIFY_CLIENT_SECRET", c.SpotifyClientSecret},
		{"SPOTIFY_REFRESH_TOKEN", c.SpotifyRefreshToken},
		{"ANTHROPIC_API_KEY", c.AnthropicAPIKey},
		{"DELIVERY_EMAIL_TO", c.DeliveryEmailTo},
	} {
		if r.value == "" {
			errs = append(errs, fmt.Errorf("config: %s is required", r.name))
		}
	}

	// At least one email provider key is required
	if c.ResendAPIKey == "" && c.SendGridAPIKey == "" {
		errs = append(errs, fmt.Errorf("config: RESEND_API_KEY or SENDGRID_API_KEY is required"))
	}

	return errors.Join(errs...)
}

// SetupLogger initialises the default slog logger based on the configured level.
func SetupLogger(level string) {
	var l slog.Level
	switch level {
	case "debug":
		l = slog.LevelDebug
	case "warn":
		l = slog.LevelWarn
	case "error":
		l = slog.LevelError
	default:
		l = slog.LevelInfo
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: l})))
}

// LogSafe logs which config keys were loaded without revealing their values.
func (c *Config) LogSafe() {
	slog.Debug("config loaded",
		"spotify_client_id_set", c.SpotifyClientID != "",
		"spotify_client_secret_set", c.SpotifyClientSecret != "",
		"spotify_refresh_token_set", c.SpotifyRefreshToken != "",
		"anthropic_api_key_set", c.AnthropicAPIKey != "",
		"lastfm_api_key_set", c.LastFMAPIKey != "",
		"resend_api_key_set", c.ResendAPIKey != "",
		"sendgrid_api_key_set", c.SendGridAPIKey != "",
		"delivery_email_to_set", c.DeliveryEmailTo != "",
		"telegram_bot_token_set", c.TelegramBotToken != "",
		"telegram_chat_id_set", c.TelegramChatID != "",
		"timezone", c.Timezone,
		"log_level", c.LogLevel,
		"dry_run", c.DryRun,
	)
}

func getEnvOrDefault(key, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultVal
}
