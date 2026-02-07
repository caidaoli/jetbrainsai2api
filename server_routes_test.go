package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"
)

func newTestServer(t *testing.T, statsAuthEnabled bool) *Server {
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

	storage := NewFileStorage(statsFile.Name())
	config := ServerConfig{
		Port:              "0",
		GinMode:           "test",
		ClientAPIKeys:     []string{"test-key"},
		JetbrainsAccounts: []JetbrainsAccount{{JWT: "dummy-jwt", LastUpdated: float64(time.Now().Unix()), HasQuota: true}},
		ModelsConfigPath:  modelsFile.Name(),
		HTTPClientSettings: HTTPClientSettings{
			MaxIdleConns:        1,
			MaxIdleConnsPerHost: 1,
			MaxConnsPerHost:     1,
			IdleConnTimeout:     time.Second,
			TLSHandshakeTimeout: time.Second,
			RequestTimeout:      time.Second,
		},
		StatsAuthEnabled: statsAuthEnabled,
		Storage:          storage,
		Logger:           &NopLogger{},
	}

	server, err := NewServer(config)
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
		_ = storage.Close()
		_ = os.Remove(modelsFile.Name())
		_ = os.Remove(statsFile.Name())
	})

	return server
}

func TestServerRoutes_StatsAuthToggle(t *testing.T) {
	secured := newTestServer(t, true)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	secured.router.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("启用统计鉴权时未携带 key 应返回 401，实际 %d", w.Code)
	}

	open := newTestServer(t, false)
	req = httptest.NewRequest(http.MethodGet, "/", nil)
	w = httptest.NewRecorder()
	open.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("关闭统计鉴权时首页应公开访问，实际 %d", w.Code)
	}
}

func TestServerRoutes_OpenAIAndAnthropicModelErrors(t *testing.T) {
	server := newTestServer(t, true)

	// /v1/models 认证成功且包含 gpt-4o
	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	req.Header.Set(HeaderAuthorization, AuthBearerPrefix+"test-key")
	w := httptest.NewRecorder()
	server.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("/v1/models 应返回 200，实际 %d", w.Code)
	}
	if !bytes.Contains(w.Body.Bytes(), []byte(`"gpt-4o"`)) {
		t.Fatalf("/v1/models 应包含 gpt-4o，实际: %s", w.Body.String())
	}

	// OpenAI 协议：模型不存在
	openAIBody := []byte(`{"model":"not-exist","messages":[{"role":"user","content":"hi"}],"stream":false}`)
	req = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(openAIBody))
	req.Header.Set(HeaderAuthorization, AuthBearerPrefix+"test-key")
	req.Header.Set(HeaderContentType, ContentTypeJSON)
	w = httptest.NewRecorder()
	server.router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("/v1/chat/completions 模型不存在应返回 404，实际 %d", w.Code)
	}
	if !bytes.Contains(w.Body.Bytes(), []byte(`"error":"Model not-exist not found"`)) {
		t.Fatalf("OpenAI 错误格式不符合预期: %s", w.Body.String())
	}

	// Anthropic 协议：模型不存在
	anthropicBody := []byte(`{"model":"not-exist","max_tokens":1,"messages":[{"role":"user","content":"hi"}]}`)
	req = httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(anthropicBody))
	req.Header.Set(HeaderAuthorization, AuthBearerPrefix+"test-key")
	req.Header.Set(HeaderContentType, ContentTypeJSON)
	w = httptest.NewRecorder()
	server.router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("/v1/messages 模型不存在应返回 404，实际 %d", w.Code)
	}
	if !bytes.Contains(w.Body.Bytes(), []byte(`"type":"error"`)) ||
		!bytes.Contains(w.Body.Bytes(), []byte(`"model_not_found_error"`)) {
		t.Fatalf("Anthropic 错误格式不符合预期: %s", w.Body.String())
	}
}
