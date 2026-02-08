package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestChatCompletions_InvalidRequestBody(t *testing.T) {
	server := newTestServer(t)

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewBufferString(`{invalid`))
	req.Header.Set("Authorization", "Bearer test-key")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	server.router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("invalid JSON should return 400, got %d", w.Code)
	}
}

func TestChatCompletions_ModelNotFound(t *testing.T) {
	server := newTestServer(t)

	body := `{"model":"nonexistent-model","messages":[{"role":"user","content":"hello"}]}`
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewBufferString(body))
	req.Header.Set("Authorization", "Bearer test-key")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	server.router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("unknown model should return 404, got %d", w.Code)
	}
}

func TestChatCompletions_RequiresAuth(t *testing.T) {
	server := newTestServer(t)

	body := `{"model":"gpt-4o","messages":[{"role":"user","content":"hello"}]}`
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	server.router.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("missing auth should return 401, got %d", w.Code)
	}
}

func TestListModels_RequiresAuth(t *testing.T) {
	server := newTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	w := httptest.NewRecorder()
	server.router.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("missing auth should return 401, got %d", w.Code)
	}
}

func TestListModels_ValidAuth(t *testing.T) {
	server := newTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	req.Header.Set("Authorization", "Bearer test-key")
	w := httptest.NewRecorder()
	server.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("valid auth should return 200, got %d", w.Code)
	}
}
