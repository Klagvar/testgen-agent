package sample

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCalcHandler_MethodNotAllowed(t *testing.T) {
	req := httptest.NewRequest("POST", "/calc?a=1&b=1&op=add", nil)
	rec := httptest.NewRecorder()
	CalcHandler(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected status %d, got %d", http.StatusMethodNotAllowed, rec.Code)
	}
}

func TestCalcHandler_InvalidParameterA(t *testing.T) {
	req := httptest.NewRequest("GET", "/calc?a=abc&b=1&op=add", nil)
	rec := httptest.NewRecorder()
	CalcHandler(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, rec.Code)
	}
}

func TestCalcHandler_InvalidParameterB(t *testing.T) {
	req := httptest.NewRequest("GET", "/calc?a=1&b=abc&op=add", nil)
	rec := httptest.NewRecorder()
	CalcHandler(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, rec.Code)
	}
}

func TestCalcHandler_DivisionByZero(t *testing.T) {
	req := httptest.NewRequest("GET", "/calc?a=10&b=0&op=div", nil)
	rec := httptest.NewRecorder()
	CalcHandler(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, rec.Code)
	}
}

func TestCalcHandler_UnknownOperation(t *testing.T) {
	req := httptest.NewRequest("GET", "/calc?a=1&b=1&op=mod", nil)
	rec := httptest.NewRecorder()
	CalcHandler(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, rec.Code)
	}
}

func TestCalcHandler_Add(t *testing.T) {
	req := httptest.NewRequest("GET", "/calc?a=5&b=3&op=add", nil)
	rec := httptest.NewRecorder()
	CalcHandler(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, rec.Code)
	}
}

func TestCalcHandler_Sub(t *testing.T) {
	req := httptest.NewRequest("GET", "/calc?a=5&b=3&op=sub", nil)
	rec := httptest.NewRecorder()
	CalcHandler(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, rec.Code)
	}
}

func TestCalcHandler_Mul(t *testing.T) {
	req := httptest.NewRequest("GET", "/calc?a=5&b=3&op=mul", nil)
	rec := httptest.NewRecorder()
	CalcHandler(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, rec.Code)
	}
}

func TestCalcHandler_Div(t *testing.T) {
	req := httptest.NewRequest("GET", "/calc?a=10&b=2&op=div", nil)
	rec := httptest.NewRecorder()
	CalcHandler(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, rec.Code)
	}
}

func TestHealthHandler_MethodNotAllowed(t *testing.T) {
	req := httptest.NewRequest("POST", "/health", nil)
	rec := httptest.NewRecorder()
	HealthHandler(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected status %d, got %d", http.StatusMethodNotAllowed, rec.Code)
	}
}

func TestHealthHandler_Get(t *testing.T) {
	req := httptest.NewRequest("GET", "/health", nil)
	rec := httptest.NewRecorder()
	HealthHandler(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, rec.Code)
	}
}

func TestWriteError(t *testing.T) {
	rec := httptest.NewRecorder()
	writeError(rec, http.StatusInternalServerError, "test error")

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("expected status %d, got %d", http.StatusInternalServerError, rec.Code)
	}
}
