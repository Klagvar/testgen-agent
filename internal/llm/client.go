// Package llm реализует клиент для взаимодействия с LLM через OpenAI-совместимый API.
// Поддерживает: OpenAI, OpenRouter, локальные модели (Ollama, LM Studio и др.)
package llm

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gizatulin/testgen-agent/internal/prompt"
)

// Config — настройки LLM-клиента.
type Config struct {
	APIKey  string  // API-ключ (пустой для локальных моделей)
	BaseURL string  // базовый URL API (например, https://api.openai.com/v1)
	Model   string  // имя модели (например, gpt-4o, deepseek-coder, и т.д.)
	Timeout int     // таймаут в секундах (по умолчанию 120)
	MaxTokens int   // максимальное количество токенов ответа (0 = без ограничений)
}

// DefaultConfig возвращает конфигурацию по умолчанию (OpenAI).
func DefaultConfig() Config {
	return Config{
		BaseURL:   "https://api.openai.com/v1",
		Model:     "gpt-4o-mini",
		Timeout:   120,
		MaxTokens: 4096,
	}
}

// Client — LLM-клиент.
type Client struct {
	config Config
	http   *http.Client
}

// NewClient создаёт новый LLM-клиент.
func NewClient(cfg Config) *Client {
	if cfg.Timeout == 0 {
		cfg.Timeout = 120
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

// chatRequest — тело запроса к LLM API.
type chatRequest struct {
	Model     string           `json:"model"`
	Messages  []prompt.Message `json:"messages"`
	MaxTokens int              `json:"max_tokens,omitempty"`
}

// chatResponse — ответ LLM API.
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

// GenerateResponse — результат генерации.
type GenerateResponse struct {
	Content          string // текст ответа
	PromptTokens     int    // количество токенов в промпте
	CompletionTokens int    // количество токенов в ответе
	TotalTokens      int    // общее количество токенов
	Model            string // модель, которая сгенерировала ответ
}

// Generate отправляет сообщения в LLM и возвращает ответ.
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
		return nil, fmt.Errorf("ошибка сериализации запроса: %w", err)
	}

	url := strings.TrimRight(c.config.BaseURL, "/") + "/chat/completions"

	req, err := http.NewRequest("POST", url, bytes.NewReader(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("ошибка создания HTTP-запроса: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	if c.config.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.config.APIKey)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ошибка HTTP-запроса к LLM: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("ошибка чтения ответа LLM: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("LLM API вернул статус %d: %s", resp.StatusCode, string(body))
	}

	var chatResp chatResponse
	if err := json.Unmarshal(body, &chatResp); err != nil {
		return nil, fmt.Errorf("ошибка парсинга ответа LLM: %w", err)
	}

	if chatResp.Error != nil {
		return nil, fmt.Errorf("LLM API ошибка: [%s] %s", chatResp.Error.Type, chatResp.Error.Message)
	}

	if len(chatResp.Choices) == 0 {
		return nil, fmt.Errorf("LLM API вернул пустой ответ")
	}

	content := chatResp.Choices[0].Message.Content

	// Очищаем от markdown-обёрток, если LLM вернула с ними
	content = cleanCodeResponse(content)

	return &GenerateResponse{
		Content:          content,
		PromptTokens:     chatResp.Usage.PromptTokens,
		CompletionTokens: chatResp.Usage.CompletionTokens,
		TotalTokens:      chatResp.Usage.TotalTokens,
		Model:            c.config.Model,
	}, nil
}

// cleanCodeResponse убирает markdown-обёртки из ответа LLM.
func cleanCodeResponse(s string) string {
	s = strings.TrimSpace(s)

	// Убираем ```go ... ```
	if strings.HasPrefix(s, "```go") {
		s = strings.TrimPrefix(s, "```go")
		s = strings.TrimSpace(s)
	} else if strings.HasPrefix(s, "```") {
		s = strings.TrimPrefix(s, "```")
		s = strings.TrimSpace(s)
	}

	if strings.HasSuffix(s, "```") {
		s = strings.TrimSuffix(s, "```")
		s = strings.TrimSpace(s)
	}

	return s
}
