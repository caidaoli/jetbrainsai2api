package server

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func newTestServerForMiddleware(clientKeys []string) *Server {
	gin.SetMode(gin.TestMode)
	keyMap := make(map[string]bool)
	for _, k := range clientKeys {
		keyMap[k] = true
	}
	return &Server{
		validClientKeys: keyMap,
	}
}

func TestAuthenticateClient_ValidBearerToken(t *testing.T) {
	s := newTestServerForMiddleware([]string{"test-key-1", "test-key-2"})
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	c.Request.Header.Set("Authorization", "Bearer test-key-1")
	s.authenticateClient(c)
	if w.Code != http.StatusOK { t.Errorf("valid bearer token should pass, got status %d", w.Code) }
	if c.IsAborted() { t.Error("valid bearer token should not abort") }
}

func TestAuthenticateClient_ValidXAPIKey(t *testing.T) {
	s := newTestServerForMiddleware([]string{"test-key-1"})
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	c.Request.Header.Set("x-api-key", "test-key-1")
	s.authenticateClient(c)
	if w.Code != http.StatusOK { t.Errorf("valid x-api-key should pass, got status %d", w.Code) }
	if c.IsAborted() { t.Error("valid x-api-key should not abort") }
}

func TestAuthenticateClient_InvalidKey(t *testing.T) {
	s := newTestServerForMiddleware([]string{"valid-key"})
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	c.Request.Header.Set("Authorization", "Bearer wrong-key")
	s.authenticateClient(c)
	if w.Code != http.StatusForbidden { t.Errorf("invalid key should return 403, got %d", w.Code) }
	if !c.IsAborted() { t.Error("invalid key should abort") }
}

func TestAuthenticateClient_MissingKey(t *testing.T) {
	s := newTestServerForMiddleware([]string{"valid-key"})
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	s.authenticateClient(c)
	if w.Code != http.StatusUnauthorized { t.Errorf("missing key should return 401, got %d", w.Code) }
	if !c.IsAborted() { t.Error("missing key should abort") }
}

func TestAuthenticateClient_NoKeysConfigured(t *testing.T) {
	s := newTestServerForMiddleware(nil)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	s.authenticateClient(c)
	if w.Code != http.StatusServiceUnavailable { t.Errorf("no keys configured should return 503, got %d", w.Code) }
	if !c.IsAborted() { t.Error("no keys configured should abort") }
}

func TestCorsMiddleware_SetsHeaders(t *testing.T) {
	s := newTestServerForMiddleware(nil)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	handler := s.corsMiddleware()
	handler(c)
	if origin := w.Header().Get("Access-Control-Allow-Origin"); origin != "*" {
		t.Errorf("expected Access-Control-Allow-Origin '*', got '%s'", origin)
	}
}

func TestCorsMiddleware_OptionsRequest(t *testing.T) {
	s := newTestServerForMiddleware(nil)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodOptions, "/v1/chat/completions", nil)
	handler := s.corsMiddleware()
	handler(c)
	if w.Code != http.StatusNoContent { t.Errorf("OPTIONS should return 204, got %d", w.Code) }
	if !c.IsAborted() { t.Error("OPTIONS should abort (skip handler)") }
}

func TestAuthenticateClient_XAPIKeyTakesPrecedence(t *testing.T) {
	s := newTestServerForMiddleware([]string{"valid-key"})
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	c.Request.Header.Set("x-api-key", "invalid-key")
	c.Request.Header.Set("Authorization", "Bearer valid-key")
	s.authenticateClient(c)
	if w.Code != http.StatusForbidden {
		t.Errorf("invalid x-api-key should return 403 even with valid Bearer, got %d", w.Code)
	}
}
