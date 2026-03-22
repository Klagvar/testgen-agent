package sample

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
)

// APIResponse is a standard JSON response.
type APIResponse struct {
	Status  string      `json:"status"`
	Message string      `json:"message,omitempty"`
	Data    interface{} `json:"data,omitempty"`
}

// HealthHandler returns a simple health check response.
func HealthHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(APIResponse{
		Status:  "ok",
		Message: "service is healthy",
	})
}

// CalcHandler handles /calc?a=X&b=Y&op=add|sub|mul|div
func CalcHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	aStr := r.URL.Query().Get("a")
	bStr := r.URL.Query().Get("b")
	op := r.URL.Query().Get("op")

	a, err := strconv.Atoi(aStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid parameter 'a'")
		return
	}
	b, err := strconv.Atoi(bStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid parameter 'b'")
		return
	}

	var result int
	switch op {
	case "add":
		result = a + b
	case "sub":
		result = a - b
	case "mul":
		result = a * b
	case "div":
		if b == 0 {
			writeError(w, http.StatusBadRequest, "division by zero")
			return
		}
		result = a / b
	default:
		writeError(w, http.StatusBadRequest, fmt.Sprintf("unknown operation: %s", op))
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(APIResponse{
		Status: "ok",
		Data:   result,
	})
}

func writeError(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(APIResponse{
		Status:  "error",
		Message: msg,
	})
}
