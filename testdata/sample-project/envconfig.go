package sample

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"time"
)

// AppConfig holds application configuration loaded from environment.
type AppConfig struct {
	Host    string
	Port    int
	Debug   bool
	Timeout time.Duration
}

// LoadConfig reads configuration from environment variables.
func LoadConfig() (*AppConfig, error) {
	host := os.Getenv("APP_HOST")
	if host == "" {
		host = "localhost"
	}

	portStr := os.Getenv("APP_PORT")
	port := 8080
	if portStr != "" {
		p, err := strconv.Atoi(portStr)
		if err != nil {
			return nil, fmt.Errorf("invalid APP_PORT: %w", err)
		}
		if p < 1 || p > 65535 {
			return nil, errors.New("APP_PORT must be 1-65535")
		}
		port = p
	}

	debug := os.Getenv("APP_DEBUG") == "true"

	timeoutStr := os.Getenv("APP_TIMEOUT")
	timeout := 30 * time.Second
	if timeoutStr != "" {
		d, err := time.ParseDuration(timeoutStr)
		if err != nil {
			return nil, fmt.Errorf("invalid APP_TIMEOUT: %w", err)
		}
		timeout = d
	}

	return &AppConfig{
		Host:    host,
		Port:    port,
		Debug:   debug,
		Timeout: timeout,
	}, nil
}

// FetchWithTimeout performs an operation with a context timeout.
// Returns an error if the context is cancelled or times out.
func FetchWithTimeout(ctx context.Context, key string) (string, error) {
	if key == "" {
		return "", errors.New("key must not be empty")
	}

	select {
	case <-ctx.Done():
		return "", fmt.Errorf("operation cancelled: %w", ctx.Err())
	case <-time.After(10 * time.Millisecond):
		return fmt.Sprintf("value_for_%s", key), nil
	}
}

// TimeSince returns a human-readable string for elapsed time.
func TimeSince(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%d minutes ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%d hours ago", int(d.Hours()))
	default:
		return fmt.Sprintf("%d days ago", int(d.Hours()/24))
	}
}
