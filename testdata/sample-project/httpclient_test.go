package sample

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestFetchStatus_HappyPath(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := &http.Client{}
	status, err := FetchStatus(client, server.URL)

	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if status != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, status)
	}
}

func TestFetchStatus_NetworkError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	server.Close() // Close immediately to simulate network error

	client := &http.Client{}
	status, err := FetchStatus(client, server.URL)

	if status != 0 {
		t.Errorf("Expected status 0, got %d", status)
	}
	if err == nil {
		t.Errorf("Expected error, got nil")
	}
	if !strings.Contains(err.Error(), "get") {
		t.Errorf("Expected error message to contain 'get', got: %v", err)
	}
}

func TestFetchStatus_InvalidURL(t *testing.T) {
	client := &http.Client{}
	status, err := FetchStatus(client, "http://invalid-url-that-does-not-exist-12345.com")

	if status != 0 {
		t.Errorf("Expected status 0, got %d", status)
	}
	if err == nil {
		t.Errorf("Expected error, got nil")
	}
}

func TestFetchJSON_HappyPath(t *testing.T) {
	response := APIResponse{
		Status:  "success",
		Message: "ok",
	}
	jsonData, _ := json.Marshal(response)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Header().Set("Content-Type", "application/json")
		w.Write(jsonData)
	}))
	defer server.Close()

	client := &http.Client{}
	result, err := FetchJSON(client, server.URL)

	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if result == nil {
		t.Errorf("Expected non-nil result")
	}
	if result.Status != response.Status {
		t.Errorf("Expected status %s, got %s", response.Status, result.Status)
	}
	if result.Message != response.Message {
		t.Errorf("Expected message %s, got %s", response.Message, result.Message)
	}
}

func TestFetchJSON_UnexpectedStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	client := &http.Client{}
	result, err := FetchJSON(client, server.URL)

	if result != nil {
		t.Errorf("Expected nil result, got %v", result)
	}
	if err == nil {
		t.Errorf("Expected error, got nil")
	}
	if !strings.Contains(err.Error(), "unexpected status") {
		t.Errorf("Expected error message to contain 'unexpected status', got: %v", err)
	}
}

func TestFetchJSON_InvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status": "success", "message":}`))
	}))
	defer server.Close()

	client := &http.Client{}
	result, err := FetchJSON(client, server.URL)

	if result != nil {
		t.Errorf("Expected nil result, got %v", result)
	}
	if err == nil {
		t.Errorf("Expected error, got nil")
	}
	if !strings.Contains(err.Error(), "unmarshal") {
		t.Errorf("Expected error message to contain 'unmarshal', got: %v", err)
	}
}

func TestFetchJSON_BodyReadError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Header().Set("Content-Type", "application/json")
		// Return a body that cannot be read
		w.(http.Flusher).Flush()
		// This is a bit artificial but ensures we have a connection that will fail
	}))
	defer server.Close()

	client := &http.Client{}
	result, err := FetchJSON(client, server.URL)

	if result != nil {
		t.Errorf("Expected nil result, got %v", result)
	}
	if err == nil {
		t.Errorf("Expected error, got nil")
	}
}

func TestFetchJSON_NetworkError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	server.Close() // Close immediately to simulate network error

	client := &http.Client{}
	result, err := FetchJSON(client, server.URL)

	if result != nil {
		t.Errorf("Expected nil result, got %v", result)
	}
	if err == nil {
		t.Errorf("Expected error, got nil")
	}
	if !strings.Contains(err.Error(), "get") {
		t.Errorf("Expected error message to contain 'get', got: %v", err)
	}
}
