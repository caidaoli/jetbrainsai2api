package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAnthropicMessages_InvalidRequestBody(t *testing.T) {
	server := newTestServer(t)

	req := httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewBufferString(`{invalid`))
	req.Header.Set("Authorization", "Bearer test-key")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	server.router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("invalid JSON should return 400, got %d", w.Code)
	}
}

func TestAnthropicMessages_MissingModel(t *testing.T) {
	server := newTestServer(t)

	body := `{"max_tokens":100,"messages":[{"role":"user","content":"hello"}]}`
	req := httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewBufferString(body))
	req.Header.Set("Authorization", "Bearer test-key")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	server.router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("missing model should return 400, got %d", w.Code)
	}
}

func TestAnthropicMessages_InvalidMaxTokens(t *testing.T) {
	server := newTestServer(t)

	body := `{"model":"gpt-4o","max_tokens":0,"messages":[{"role":"user","content":"hello"}]}`
	req := httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewBufferString(body))
	req.Header.Set("Authorization", "Bearer test-key")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	server.router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("max_tokens=0 should return 400, got %d", w.Code)
	}
}

func TestAnthropicMessages_EmptyMessages(t *testing.T) {
	server := newTestServer(t)

	body := `{"model":"gpt-4o","max_tokens":100,"messages":[]}`
	req := httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewBufferString(body))
	req.Header.Set("Authorization", "Bearer test-key")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	server.router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("empty messages should return 400, got %d", w.Code)
	}
}

func TestAnthropicMessages_UnknownModel(t *testing.T) {
	server := newTestServer(t)

	body := `{"model":"nonexistent-model","max_tokens":100,"messages":[{"role":"user","content":"hello"}]}`
	req := httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewBufferString(body))
	req.Header.Set("Authorization", "Bearer test-key")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	server.router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("unknown model should return 404, got %d", w.Code)
	}
}

func TestAnthropicMessages_RequiresAuth(t *testing.T) {
	server := newTestServer(t)

	body := `{"model":"gpt-4o","max_tokens":100,"messages":[{"role":"user","content":"hello"}]}`
	req := httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	server.router.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("missing auth should return 401, got %d", w.Code)
	}
}
