// Package llm implements a client for interacting with LLMs via the OpenAI-compatible API.
// Supports: OpenAI, OpenRouter, local models (Ollama, LM Studio, etc.)
package llm

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/gizatulin/testgen-agent/internal/prompt"
)

// Config holds LLM client settings.
type Config struct {
	APIKey     string   // API key (empty for local models)
	BaseURL    string   // base API URL (e.g., https://api.openai.com/v1)
	Model      string   // model name (e.g., gpt-4o, deepseek-coder, etc.)
	Timeout    int      // timeout in seconds (default 120)
	MaxTokens  int      // max response tokens (0 = unlimited)
	MaxRetries int      // max HTTP retries on transient errors (default 3)
	Temperature *float64 // sampling temperature (nil = backend default)
	Seed        *int     // sampling seed for reproducibility (nil = not set)
	// Provider is the OpenRouter "provider.only" allowlist used to pin a
	// request to a specific upstream backend (e.g. ["Phala"]). When empty,
	// the gateway picks providers automatically. When non-empty, the field
	// also controls whether OpenRouter is allowed to fall back outside the
	// allowlist (see AllowFallbacks). Ignored by non-OpenRouter backends.
	Provider []string
	// AllowFallbacks toggles OpenRouter "provider.allow_fallbacks". Only
	// meaningful when Provider is non-empty. Defaults to false (strict pin)
	// to keep experimental runs reproducible across requests.
	AllowFallbacks bool
}

// DefaultConfig returns the default configuration (OpenAI).
//
// MaxTokens is intentionally set high (16k) because some OpenRouter
// providers — most notably Anthropic native — strictly enforce this
// limit and silently truncate long responses, breaking AST-merge for
// table-driven tests on large functions. Other providers (Bedrock,
// Google AI Studio) treat it as a soft hint, so a generous default
// is safe for them. See benchmark notes from 2026-05-16 for the
// empirical evidence behind this choice.
func DefaultConfig() Config {
	return Config{
		BaseURL:    "https://api.openai.com/v1",
		Model:      "gpt-4o-mini",
		Timeout:    300,
		MaxTokens:  16384,
		MaxRetries: 3,
	}
}

// Client is the LLM client.
type Client struct {
	config Config
	http   *http.Client
}

// NewClient creates a new LLM client.
func NewClient(cfg Config) *Client {
	if cfg.Timeout == 0 {
		cfg.Timeout = 300
	}
	if cfg.BaseURL == "" {
		cfg.BaseURL = "https://api.openai.com/v1"
	}

	return &Client{
		config: cfg,
		http: &http.Client{
			Timeout: time.Duration(cfg.Timeout) * time.Second,
		},
	}
}

// chatRequest is the LLM API request body.
type chatRequest struct {
	Model       string           `json:"model"`
	Messages    []prompt.Message `json:"messages"`
	MaxTokens   int              `json:"max_tokens,omitempty"`
	Temperature *float64         `json:"temperature,omitempty"`
	Seed        *int             `json:"seed,omitempty"`
	// Provider is the OpenRouter routing override. Serialized only when the
	// caller actually populated Config.Provider; otherwise omitted so that
	// other OpenAI-compatible backends (which would reject the field) keep
	// working unchanged.
	Provider *providerSpec `json:"provider,omitempty"`
}

// providerSpec mirrors OpenRouter's "provider" routing object.
// See https://openrouter.ai/docs/provider-routing for the full schema.
type providerSpec struct {
	Only           []string `json:"only,omitempty"`
	AllowFallbacks bool     `json:"allow_fallbacks"`
}

// chatResponse is the LLM API response.
type chatResponse struct {
	// Provider is OpenRouter-specific: the upstream backend that actually
	// served the request (e.g., "Phala"). Empty for non-OpenRouter gateways.
	Provider string `json:"provider,omitempty"`
	Choices  []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
	} `json:"error,omitempty"`
}

// GenerateResponse holds the generation result.
type GenerateResponse struct {
	Content          string // response text
	PromptTokens     int    // number of prompt tokens
	CompletionTokens int    // number of completion tokens
	TotalTokens      int    // total token count
	Model            string // model that generated the response
	// Provider is the upstream backend reported by OpenRouter (e.g. "Phala").
	// Empty for non-OpenRouter gateways or when the field is absent.
	Provider string
}

// isRetryableStatus returns true for HTTP status codes worth retrying.
func isRetryableStatus(code int) bool {
	return code == http.StatusTooManyRequests ||
		code == http.StatusInternalServerError ||
		code == http.StatusBadGateway ||
		code == http.StatusServiceUnavailable ||
		code == http.StatusGatewayTimeout
}

// Generate sends messages to the LLM and returns the response.
// Retries transient HTTP errors with exponential backoff + jitter.
func (c *Client) Generate(messages []prompt.Message) (*GenerateResponse, error) {
	reqBody := chatRequest{
		Model:       c.config.Model,
		Messages:    messages,
		Temperature: c.config.Temperature,
		Seed:        c.config.Seed,
	}
	if c.config.MaxTokens > 0 {
		reqBody.MaxTokens = c.config.MaxTokens
	}
	if len(c.config.Provider) > 0 {
		reqBody.Provider = &providerSpec{
			Only:           append([]string(nil), c.config.Provider...),
			AllowFallbacks: c.config.AllowFallbacks,
		}
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("error serializing request: %w", err)
	}

	url := strings.TrimRight(c.config.BaseURL, "/") + "/chat/completions"

	maxAttempts := c.config.MaxRetries
	if maxAttempts < 1 {
		maxAttempts = 1
	}

	var lastErr error
	for attempt := 0; attempt < maxAttempts; attempt++ {
		if attempt > 0 {
			backoff := time.Duration(1<<uint(attempt-1)) * time.Second
			jitter := time.Duration(rand.Intn(500)) * time.Millisecond
			time.Sleep(backoff + jitter)
		}

		req, err := http.NewRequest("POST", url, bytes.NewReader(jsonBody))
		if err != nil {
			return nil, fmt.Errorf("error creating HTTP request: %w", err)
		}

		req.Header.Set("Content-Type", "application/json")
		if c.config.APIKey != "" {
			req.Header.Set("Authorization", "Bearer "+c.config.APIKey)
		}

		resp, err := c.http.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("HTTP request to LLM failed: %w", err)
			continue
		}

		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			lastErr = fmt.Errorf("error reading LLM response: %w", err)
			continue
		}

		if isRetryableStatus(resp.StatusCode) {
			lastErr = fmt.Errorf("LLM API returned status %d: %s", resp.StatusCode, string(body))
			continue
		}

		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("LLM API returned status %d: %s", resp.StatusCode, string(body))
		}

		var chatResp chatResponse
		if err := json.Unmarshal(body, &chatResp); err != nil {
			return nil, fmt.Errorf("error parsing LLM response: %w", err)
		}

		if chatResp.Error != nil {
			return nil, fmt.Errorf("LLM API error: [%s] %s", chatResp.Error.Type, chatResp.Error.Message)
		}

		if len(chatResp.Choices) == 0 {
			return nil, fmt.Errorf("LLM API returned empty response")
		}

		content := chatResp.Choices[0].Message.Content
		content = cleanCodeResponse(content)

		return &GenerateResponse{
			Content:          content,
			PromptTokens:     chatResp.Usage.PromptTokens,
			CompletionTokens: chatResp.Usage.CompletionTokens,
			TotalTokens:      chatResp.Usage.TotalTokens,
			Model:            c.config.Model,
			Provider:         chatResp.Provider,
		}, nil
	}

	return nil, fmt.Errorf("LLM request failed after %d attempts: %w", maxAttempts, lastErr)
}

// codeBlockRe matches opening fenced code blocks with optional language tags.
var codeBlockRe = regexp.MustCompile("^```[a-zA-Z]*\\s*\n?")

// cleanCodeResponse removes markdown wrappers from the LLM response.
// Handles ```go, ```golang, ```Go, bare ```, and trailing ```.
func cleanCodeResponse(s string) string {
	s = strings.TrimSpace(s)

	if loc := codeBlockRe.FindStringIndex(s); loc != nil && loc[0] == 0 {
		s = s[loc[1]:]
		s = strings.TrimSpace(s)
	}

	if strings.HasSuffix(s, "```") {
		s = strings.TrimSuffix(s, "```")
		s = strings.TrimSpace(s)
	}

	return s
}
