package sample

import (
	"context"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"
)

func TestLoadConfig_HappyPath(t *testing.T) {
	t.Setenv("APP_HOST", "example.com")
	t.Setenv("APP_PORT", "8080")
	t.Setenv("APP_DEBUG", "true")
	t.Setenv("APP_TIMEOUT", "5s")

	config, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}

	expected := &AppConfig{
		Host:    "example.com",
		Port:    8080,
		Debug:   true,
		Timeout: 5 * time.Second,
	}

	if config.Host != expected.Host {
		t.Errorf("LoadConfig().Host = %v, want %v", config.Host, expected.Host)
	}
	if config.Port != expected.Port {
		t.Errorf("LoadConfig().Port = %v, want %v", config.Port, expected.Port)
	}
	if config.Debug != expected.Debug {
		t.Errorf("LoadConfig().Debug = %v, want %v", config.Debug, expected.Debug)
	}
	if config.Timeout != expected.Timeout {
		t.Errorf("LoadConfig().Timeout = %v, want %v", config.Timeout, expected.Timeout)
	}
}

func TestLoadConfig_Defaults(t *testing.T) {
	// Clear all env vars
	os.Unsetenv("APP_HOST")
	os.Unsetenv("APP_PORT")
	os.Unsetenv("APP_DEBUG")
	os.Unsetenv("APP_TIMEOUT")

	config, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}

	expected := &AppConfig{
		Host:    "localhost",
		Port:    8080,
		Debug:   false,
		Timeout: 30 * time.Second,
	}

	if config.Host != expected.Host {
		t.Errorf("LoadConfig().Host = %v, want %v", config.Host, expected.Host)
	}
	if config.Port != expected.Port {
		t.Errorf("LoadConfig().Port = %v, want %v", config.Port, expected.Port)
	}
	if config.Debug != expected.Debug {
		t.Errorf("LoadConfig().Debug = %v, want %v", config.Debug, expected.Debug)
	}
	if config.Timeout != expected.Timeout {
		t.Errorf("LoadConfig().Timeout = %v, want %v", config.Timeout, expected.Timeout)
	}
}

func TestLoadConfig_InvalidPort(t *testing.T) {
	t.Setenv("APP_PORT", "70000")

	_, err := LoadConfig()
	if err == nil {
		t.Error("LoadConfig() should return error for invalid port")
	}
}

func TestLoadConfig_NegativePort(t *testing.T) {
	t.Setenv("APP_PORT", "-1")

	_, err := LoadConfig()
	if err == nil {
		t.Error("LoadConfig() should return error for negative port")
	}
}

func TestLoadConfig_InvalidPortFormat(t *testing.T) {
	t.Setenv("APP_PORT", "not-a-number")

	_, err := LoadConfig()
	if err == nil {
		t.Error("LoadConfig() should return error for invalid port format")
	}
}

func TestLoadConfig_InvalidTimeout(t *testing.T) {
	t.Setenv("APP_TIMEOUT", "invalid-duration")

	_, err := LoadConfig()
	if err == nil {
		t.Error("LoadConfig() should return error for invalid timeout")
	}
}

func TestFetchWithTimeout_HappyPath(t *testing.T) {
	ctx := context.Background()
	key := "test-key"

	value, err := FetchWithTimeout(ctx, key)
	if err != nil {
		t.Fatalf("FetchWithTimeout() error = %v", err)
	}

	expected := "value_for_test-key"
	if value != expected {
		t.Errorf("FetchWithTimeout() value = %v, want %v", value, expected)
	}
}

func TestFetchWithTimeout_EmptyKey(t *testing.T) {
	ctx := context.Background()

	_, err := FetchWithTimeout(ctx, "")
	if err == nil {
		t.Error("FetchWithTimeout() should return error for empty key")
	}
}

func TestFetchWithTimeout_CancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := FetchWithTimeout(ctx, "test-key")
	if err == nil {
		t.Error("FetchWithTimeout() should return error for cancelled context")
	}
}

func TestFetchWithTimeout_ExpiredTimeout(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Millisecond)
	defer cancel()
	time.Sleep(5 * time.Millisecond)

	_, err := FetchWithTimeout(ctx, "test-key")
	if err == nil {
		t.Error("FetchWithTimeout() should return error for expired timeout")
	}
}

func TestFetchWithTimeout_Concurrent(t *testing.T) {
	const numGoroutines = 100
	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func(i int) {
			defer wg.Done()
			ctx := context.Background()
			key := fmt.Sprintf("key-%d", i)
			value, err := FetchWithTimeout(ctx, key)
			if err != nil {
				t.Errorf("FetchWithTimeout() error = %v", err)
				return
			}
			expected := fmt.Sprintf("value_for_%s", key)
			if value != expected {
				t.Errorf("FetchWithTimeout() value = %v, want %v", value, expected)
			}
		}(i)
	}

	wg.Wait()
}

func TestTimeSince_JustNow(t *testing.T) {
	now := time.Now()
	result := TimeSince(now)
	expected := "just now"
	if result != expected {
		t.Errorf("TimeSince() = %v, want %v", result, expected)
	}
}

func TestTimeSince_MinutesAgo(t *testing.T) {
	minutesAgo := time.Now().Add(-30 * time.Minute)
	result := TimeSince(minutesAgo)
	expected := "30 minutes ago"
	if result != expected {
		t.Errorf("TimeSince() = %v, want %v", result, expected)
	}
}

func TestTimeSince_HoursAgo(t *testing.T) {
	hoursAgo := time.Now().Add(-5 * time.Hour)
	result := TimeSince(hoursAgo)
	expected := "5 hours ago"
	if result != expected {
		t.Errorf("TimeSince() = %v, want %v", result, expected)
	}
}

func TestTimeSince_DaysAgo(t *testing.T) {
	daysAgo := time.Now().Add(-3 * 24 * time.Hour)
	result := TimeSince(daysAgo)
	expected := "3 days ago"
	if result != expected {
		t.Errorf("TimeSince() = %v, want %v", result, expected)
	}
}

func TestLoadConfig_ValidPort(t *testing.T) {
	// Set up environment
	oldPort := os.Getenv("APP_PORT")
	os.Setenv("APP_PORT", "8080")
	defer func() {
		os.Setenv("APP_PORT", oldPort)
	}()

	config, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	if config.Port != 8080 {
		t.Errorf("LoadConfig() Port = %v, want %v", config.Port, 8080)
	}
}

func TestLoadConfig_MaxPort(t *testing.T) {
	// Set up environment
	oldPort := os.Getenv("APP_PORT")
	os.Setenv("APP_PORT", "65535")
	defer func() {
		os.Setenv("APP_PORT", oldPort)
	}()

	config, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	if config.Port != 65535 {
		t.Errorf("LoadConfig() Port = %v, want %v", config.Port, 65535)
	}
}

func TestLoadConfig_ZeroPort(t *testing.T) {
	// Set up environment
	oldPort := os.Getenv("APP_PORT")
	os.Setenv("APP_PORT", "0")
	defer func() {
		os.Setenv("APP_PORT", oldPort)
	}()

	_, err := LoadConfig()
	if err == nil {
		t.Errorf("LoadConfig() error = nil, want error")
	}
}

func TestFetchWithTimeout_ExpiredContext(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Millisecond)
	defer cancel()

	_, err := FetchWithTimeout(ctx, "test")
	if err == nil {
		t.Errorf("FetchWithTimeout() error = nil, want error")
	}
}

func TestTimeSince_ExactMinute(t *testing.T) {
	past := time.Now().Add(-1 * time.Minute)
	result := TimeSince(past)
	if result != "1 minutes ago" {
		t.Errorf("TimeSince() = %v, want \"1 minutes ago\"", result)
	}
}

func TestTimeSince_ExactHour(t *testing.T) {
	past := time.Now().Add(-1 * time.Hour)
	result := TimeSince(past)
	if result != "1 hours ago" {
		t.Errorf("TimeSince() = %v, want \"1 hours ago\"", result)
	}
}

func TestTimeSince_ExactDay(t *testing.T) {
	past := time.Now().Add(-24 * time.Hour)
	result := TimeSince(past)
	if result != "1 days ago" {
		t.Errorf("TimeSince() = %v, want \"1 days ago\"", result)
	}
}

func TestLoadConfig_EnvVarOverrides(t *testing.T) {
	t.Setenv("APP_HOST", "example.com")
	t.Setenv("APP_PORT", "9090")
	t.Setenv("APP_DEBUG", "true")
	t.Setenv("APP_TIMEOUT", "60s")

	config, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}

	expected := &AppConfig{
		Host:    "example.com",
		Port:    9090,
		Debug:   true,
		Timeout: 60 * time.Second,
	}

	if config.Host != expected.Host {
		t.Errorf("Host = %v, want %v", config.Host, expected.Host)
	}
	if config.Port != expected.Port {
		t.Errorf("Port = %v, want %v", config.Port, expected.Port)
	}
	if config.Debug != expected.Debug {
		t.Errorf("Debug = %v, want %v", config.Debug, expected.Debug)
	}
	if config.Timeout != expected.Timeout {
		t.Errorf("Timeout = %v, want %v", config.Timeout, expected.Timeout)
	}
}

func TestLoadConfig_InvalidTimeoutFormat(t *testing.T) {
	t.Setenv("APP_TIMEOUT", "not-a-duration")

	_, err := LoadConfig()
	if err == nil {
		t.Error("LoadConfig() expected error for invalid APP_TIMEOUT, got nil")
	}
}

func TestFetchWithTimeout_InvalidKey(t *testing.T) {
	_, err := FetchWithTimeout(context.Background(), "")
	if err == nil {
		t.Error("FetchWithTimeout() expected error for empty key, got nil")
	}
}

func TestFetchWithTimeout_InvalidKey_Concurrent(t *testing.T) {
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := FetchWithTimeout(context.Background(), "")
			if err == nil {
				t.Error("FetchWithTimeout() expected error for empty key, got nil")
			}
		}()
	}
	wg.Wait()
}

func TestTimeSince_MaxDuration(t *testing.T) {
	now := time.Now()
	result := TimeSince(now.Add(-time.Hour * 24 * 365))
	if result != "365 days ago" {
		t.Errorf("TimeSince() with one year ago = %v, want %v", result, "365 days ago")
	}
}
