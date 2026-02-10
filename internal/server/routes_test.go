package server

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"jetbrainsai2api/internal/config"
	"jetbrainsai2api/internal/core"
	"jetbrainsai2api/internal/storage"
)

func newTestServer(t *testing.T) *Server {
	t.Helper()

	modelsFile, err := os.CreateTemp("", "models_test_*.json")
	if err != nil {
		t.Fatalf("创建 models 临时文件失败: %v", err)
	}
	if _, err := modelsFile.WriteString(`{"models":{"gpt-4o":"openai-gpt-4o"}}`); err != nil {
		t.Fatalf("写入 models 临时文件失败: %v", err)
	}
	_ = modelsFile.Close()

	statsFile, err := os.CreateTemp("", "stats_test_*.json")
	if err != nil {
		t.Fatalf("创建 stats 临时文件失败: %v", err)
	}
	_ = statsFile.Close()

	st := storage.NewFileStorage(statsFile.Name())
	cfg := config.ServerConfig{
		Port:              "0",
		GinMode:           "test",
		ClientAPIKeys:     []string{"test-key"},
		JetbrainsAccounts: []core.JetbrainsAccount{{JWT: "dummy-jwt", LastUpdated: float64(time.Now().Unix()), HasQuota: true}},
		ModelsConfigPath:  modelsFile.Name(),
		HTTPClientSettings: config.HTTPClientSettings{
			MaxIdleConns:        1,
			MaxIdleConnsPerHost: 1,
			MaxConnsPerHost:     1,
			IdleConnTimeout:     time.Second,
			TLSHandshakeTimeout: time.Second,
			RequestTimeout:      time.Second,
		},
		Storage: st,
		Logger:  &core.NopLogger{},
	}

	server, err := NewServer(cfg)
	if err != nil {
		t.Fatalf("创建测试 Server 失败: %v", err)
	}

	t.Cleanup(func() {
		if server.metricsService != nil {
			_ = server.metricsService.Close()
		}
		if server.cache != nil {
			_ = server.cache.Close()
		}
		_ = server.accountManager.Close()
		_ = st.Close()
		_ = os.Remove(modelsFile.Name())
		_ = os.Remove(statsFile.Name())
	})

	return server
}

func TestServerRoutes_StatsPublicAccess(t *testing.T) {
	server := newTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	server.router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("监控页面应公开访问，实际 %d", w.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/stats", nil)
	w = httptest.NewRecorder()
	server.router.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("/api/stats 应需要认证，实际 %d", w.Code)
	}

	// /api/stats with valid key should return 200
	req = httptest.NewRequest(http.MethodGet, "/api/stats", nil)
	req.Header.Set(core.HeaderAuthorization, core.AuthBearerPrefix+"test-key")
	w = httptest.NewRecorder()
	server.router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("/api/stats 带认证应返回 200，实际 %d", w.Code)
	}
}

func TestServerRoutes_OpenAIAndAnthropicModelErrors(t *testing.T) {
	server := newTestServer(t)

	// /v1/models
	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	req.Header.Set(core.HeaderAuthorization, core.AuthBearerPrefix+"test-key")
	w := httptest.NewRecorder()
	server.router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("/v1/models 应返回 200，实际 %d", w.Code)
	}
	if !bytes.Contains(w.Body.Bytes(), []byte(`"gpt-4o"`)) {
		t.Fatalf("/v1/models 应包含 gpt-4o")
	}

	// OpenAI 协议：模型不存在
	openAIBody := []byte(`{"model":"not-exist","messages":[{"role":"user","content":"hi"}],"stream":false}`)
	req = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(openAIBody))
	req.Header.Set(core.HeaderAuthorization, core.AuthBearerPrefix+"test-key")
	req.Header.Set(core.HeaderContentType, core.ContentTypeJSON)
	w = httptest.NewRecorder()
	server.router.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("/v1/chat/completions 模型不存在应返回 404，实际 %d", w.Code)
	}

	// Anthropic 协议：模型不存在
	anthropicBody := []byte(`{"model":"not-exist","max_tokens":1,"messages":[{"role":"user","content":"hi"}]}`)
	req = httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(anthropicBody))
	req.Header.Set(core.HeaderAuthorization, core.AuthBearerPrefix+"test-key")
	req.Header.Set(core.HeaderContentType, core.ContentTypeJSON)
	w = httptest.NewRecorder()
	server.router.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("/v1/messages 模型不存在应返回 404，实际 %d", w.Code)
	}
}
