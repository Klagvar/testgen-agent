package llm

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gizatulin/testgen-agent/internal/prompt"
)

func TestCleanCodeResponse(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "чистый код",
			input: "package calc\n\nfunc TestAdd(t *testing.T) {}",
			want:  "package calc\n\nfunc TestAdd(t *testing.T) {}",
		},
		{
			name:  "обёртка ```go",
			input: "```go\npackage calc\n\nfunc TestAdd(t *testing.T) {}\n```",
			want:  "package calc\n\nfunc TestAdd(t *testing.T) {}",
		},
		{
			name:  "обёртка ```",
			input: "```\npackage calc\n\nfunc TestAdd(t *testing.T) {}\n```",
			want:  "package calc\n\nfunc TestAdd(t *testing.T) {}",
		},
		{
			name:  "с пробелами вокруг",
			input: "  \n```go\npackage calc\n```\n  ",
			want:  "package calc",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := cleanCodeResponse(tt.input)
			if got != tt.want {
				t.Errorf("cleanCodeResponse() =\n%q\nwant:\n%q", got, tt.want)
			}
		})
	}
}

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.BaseURL == "" {
		t.Error("BaseURL пуст")
	}
	if cfg.Model == "" {
		t.Error("Model пуст")
	}
	if cfg.Timeout == 0 {
		t.Error("Timeout = 0")
	}
}

func TestNewClient(t *testing.T) {
	cfg := Config{
		APIKey:  "test-key",
		BaseURL: "http://localhost:11434/v1",
		Model:   "llama3",
	}

	client := NewClient(cfg)
	if client == nil {
		t.Fatal("NewClient вернул nil")
	}
	if client.config.Timeout != 120 {
		t.Errorf("Timeout = %d, ожидалось 120", client.config.Timeout)
	}
}

func TestGenerate_MockServer(t *testing.T) {
	// Поднимаем фейковый сервер, имитирующий OpenAI API
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Проверяем заголовки
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("Content-Type = %q", r.Header.Get("Content-Type"))
		}
		if r.Header.Get("Authorization") != "Bearer test-key-123" {
			t.Errorf("Authorization = %q", r.Header.Get("Authorization"))
		}

		// Проверяем тело запроса
		var reqBody chatRequest
		if err := json.NewDecoder(r.Body).Decode(&reqBody); err != nil {
			t.Errorf("ошибка декодирования запроса: %v", err)
		}
		if reqBody.Model != "test-model" {
			t.Errorf("Model = %q, ожидалось test-model", reqBody.Model)
		}
		if len(reqBody.Messages) != 2 {
			t.Errorf("Messages = %d, ожидалось 2", len(reqBody.Messages))
		}

		// Возвращаем ответ
		resp := chatResponse{
			Choices: []struct {
				Message struct {
					Content string `json:"content"`
				} `json:"message"`
				FinishReason string `json:"finish_reason"`
			}{
				{
					Message: struct {
						Content string `json:"content"`
					}{
						Content: "```go\npackage calc\n\nimport \"testing\"\n\nfunc TestAdd(t *testing.T) {\n\tresult := Add(1, 2)\n\tif result != 3 {\n\t\tt.Errorf(\"Add(1,2) = %d, want 3\", result)\n\t}\n}\n```",
					},
					FinishReason: "stop",
				},
			},
			Usage: struct {
				PromptTokens     int `json:"prompt_tokens"`
				CompletionTokens int `json:"completion_tokens"`
				TotalTokens      int `json:"total_tokens"`
			}{
				PromptTokens:     100,
				CompletionTokens: 50,
				TotalTokens:      150,
			},
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewClient(Config{
		APIKey:  "test-key-123",
		BaseURL: server.URL,
		Model:   "test-model",
		Timeout: 10,
	})

	messages := []prompt.Message{
		{Role: "system", Content: "You are a test generator."},
		{Role: "user", Content: "Generate tests for Add function."},
	}

	result, err := client.Generate(messages)
	if err != nil {
		t.Fatalf("Generate вернул ошибку: %v", err)
	}

	// Код должен быть очищен от markdown-обёрток
	if result.Content == "" {
		t.Error("Content пуст")
	}
	if result.Content[0:7] != "package" {
		t.Errorf("Content должен начинаться с 'package', начинается с %q", result.Content[0:7])
	}

	t.Logf("Сгенерированный код:\n%s", result.Content)
	t.Logf("Токены: prompt=%d, completion=%d, total=%d", result.PromptTokens, result.CompletionTokens, result.TotalTokens)

	if result.TotalTokens != 150 {
		t.Errorf("TotalTokens = %d, ожидалось 150", result.TotalTokens)
	}
}

func TestGenerate_APIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error":{"message":"Invalid API key","type":"auth_error"}}`))
	}))
	defer server.Close()

	client := NewClient(Config{
		BaseURL: server.URL,
		Model:   "test-model",
		Timeout: 10,
	})

	_, err := client.Generate([]prompt.Message{{Role: "user", Content: "hello"}})
	if err == nil {
		t.Fatal("ожидалась ошибка при 401")
	}
	t.Logf("Ожидаемая ошибка: %v", err)
}
