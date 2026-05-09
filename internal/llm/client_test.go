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
			name:  "clean code",
			input: "package calc\n\nfunc TestAdd(t *testing.T) {}",
			want:  "package calc\n\nfunc TestAdd(t *testing.T) {}",
		},
		{
			name:  "wrapped in ```go",
			input: "```go\npackage calc\n\nfunc TestAdd(t *testing.T) {}\n```",
			want:  "package calc\n\nfunc TestAdd(t *testing.T) {}",
		},
		{
			name:  "wrapped in ```",
			input: "```\npackage calc\n\nfunc TestAdd(t *testing.T) {}\n```",
			want:  "package calc\n\nfunc TestAdd(t *testing.T) {}",
		},
		{
			name:  "with spaces around",
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
		t.Error("BaseURL is empty")
	}
	if cfg.Model == "" {
		t.Error("Model is empty")
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
		t.Fatal("NewClient returned nil")
	}
	if client.config.Timeout != 300 {
		t.Errorf("Timeout = %d, expected 300", client.config.Timeout)
	}
}

func TestGenerate_MockServer(t *testing.T) {
	// Start a fake server simulating the OpenAI API
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check headers
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("Content-Type = %q", r.Header.Get("Content-Type"))
		}
		if r.Header.Get("Authorization") != "Bearer test-key-123" {
			t.Errorf("Authorization = %q", r.Header.Get("Authorization"))
		}

		// Check request body
		var reqBody chatRequest
		if err := json.NewDecoder(r.Body).Decode(&reqBody); err != nil {
			t.Errorf("error decoding request: %v", err)
		}
		if reqBody.Model != "test-model" {
			t.Errorf("Model = %q, expected test-model", reqBody.Model)
		}
		if len(reqBody.Messages) != 2 {
			t.Errorf("Messages = %d, expected 2", len(reqBody.Messages))
		}

		// Return response
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
		t.Fatalf("Generate returned error: %v", err)
	}

	// Code should be cleaned of markdown wrappers
	if result.Content == "" {
		t.Error("Content is empty")
	}
	if result.Content[0:7] != "package" {
		t.Errorf("Content should start with 'package', starts with %q", result.Content[0:7])
	}

	t.Logf("Generated code:\n%s", result.Content)
	t.Logf("Tokens: prompt=%d, completion=%d, total=%d", result.PromptTokens, result.CompletionTokens, result.TotalTokens)

	if result.TotalTokens != 150 {
		t.Errorf("TotalTokens = %d, expected 150", result.TotalTokens)
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
		t.Fatal("expected error on 401")
	}
	t.Logf("Expected error: %v", err)
}

// TestGenerate_Provider_Omitted ensures that when Config.Provider is empty
// the request body does not include a "provider" field at all (so that
// non-OpenRouter gateways which would otherwise reject the field keep
// working unchanged).
func TestGenerate_Provider_Omitted(t *testing.T) {
	var observed map[string]interface{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&observed)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`))
	}))
	defer server.Close()

	client := NewClient(Config{BaseURL: server.URL, Model: "test-model", Timeout: 10})
	if _, err := client.Generate([]prompt.Message{{Role: "user", Content: "hi"}}); err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if _, has := observed["provider"]; has {
		t.Errorf("expected no \"provider\" field in body when Config.Provider is empty, got: %v", observed["provider"])
	}
}

// TestGenerate_Provider_Pinned verifies the OpenRouter routing object is
// serialized correctly and that the upstream provider name flows back into
// GenerateResponse.Provider for downstream logging.
func TestGenerate_Provider_Pinned(t *testing.T) {
	var got chatRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"provider":"Phala","choices":[{"message":{"content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`))
	}))
	defer server.Close()

	client := NewClient(Config{
		BaseURL:        server.URL,
		Model:          "qwen/qwen-2.5-7b-instruct",
		Timeout:        10,
		Provider:       []string{"Phala"},
		AllowFallbacks: false,
	})
	resp, err := client.Generate([]prompt.Message{{Role: "user", Content: "hi"}})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	if got.Provider == nil {
		t.Fatal("expected provider object in request body")
	}
	if len(got.Provider.Only) != 1 || got.Provider.Only[0] != "Phala" {
		t.Errorf("provider.only = %v, want [Phala]", got.Provider.Only)
	}
	if got.Provider.AllowFallbacks {
		t.Errorf("provider.allow_fallbacks = true, want false (strict pin)")
	}

	if resp.Provider != "Phala" {
		t.Errorf("response.Provider = %q, want %q", resp.Provider, "Phala")
	}
}

// TestGenerate_Provider_AllowFallbacks ensures the fallback flag is forwarded
// when the caller opts in (e.g. multi-provider pool with fallback).
func TestGenerate_Provider_AllowFallbacks(t *testing.T) {
	var got chatRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&got)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`))
	}))
	defer server.Close()

	client := NewClient(Config{
		BaseURL:        server.URL,
		Model:          "test-model",
		Timeout:        10,
		Provider:       []string{"Phala", "Together"},
		AllowFallbacks: true,
	})
	if _, err := client.Generate([]prompt.Message{{Role: "user", Content: "hi"}}); err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if got.Provider == nil || !got.Provider.AllowFallbacks {
		t.Errorf("provider.allow_fallbacks = %v, want true", got.Provider)
	}
	if len(got.Provider.Only) != 2 {
		t.Errorf("provider.only len = %d, want 2", len(got.Provider.Only))
	}
}
