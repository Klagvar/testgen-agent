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
	APIKey     string // API key (empty for local models)
	BaseURL    string // base API URL (e.g., https://api.openai.com/v1)
	Model      string // model name (e.g., gpt-4o, deepseek-coder, etc.)
	Timeout    int    // timeout in seconds (default 120)
	MaxTokens  int    // max response tokens (0 = unlimited)
	MaxRetries int    // max HTTP retries on transient errors (default 3)
}

// DefaultConfig returns the default configuration (OpenAI).
func DefaultConfig() Config {
	return Config{
		BaseURL:    "https://api.openai.com/v1",
		Model:      "gpt-4o-mini",
		Timeout:    300,
		MaxTokens:  4096,
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
	Model     string           `json:"model"`
	Messages  []prompt.Message `json:"messages"`
	MaxTokens int              `json:"max_tokens,omitempty"`
}

// chatResponse is the LLM API response.
type chatResponse struct {
	Choices []struct {
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
		Model:    c.config.Model,
		Messages: messages,
	}
	if c.config.MaxTokens > 0 {
		reqBody.MaxTokens = c.config.MaxTokens
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
