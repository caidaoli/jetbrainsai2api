package server

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"jetbrainsai2api/internal/config"
	"jetbrainsai2api/internal/core"
	"jetbrainsai2api/internal/storage"
)

func writeTempTestFile(t *testing.T, fileName string, content []byte) string {
	t.Helper()
	filePath := filepath.Join(t.TempDir(), fileName)
	if err := os.WriteFile(filePath, content, core.FilePermissionReadWrite); err != nil {
		t.Fatalf("写入临时文件失败: %v", err)
	}
	return filePath
}

func newTestServer(t *testing.T) *Server {
	t.Helper()

	modelsPath := writeTempTestFile(t, "models.json", []byte(`{"models":{"gpt-4o":"openai-gpt-4o"}}`))
	statsPath := writeTempTestFile(t, "stats.json", []byte(`{}`))

	st := storage.NewFileStorage(statsPath)
	cfg := config.ServerConfig{
		Port:              "0",
		GinMode:           "test",
		ClientAPIKeys:     []string{"test-key"},
		JetbrainsAccounts: []core.JetbrainsAccount{{JWT: "dummy-jwt", LastUpdated: float64(time.Now().Unix()), HasQuota: true}},
		ModelsConfigPath:  modelsPath,
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
		_ = server.Close()
		_ = st.Close()
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
	if w.Code != http.StatusOK {
		t.Fatalf("/api/stats 应公开访问，实际 %d", w.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/log", nil)
	w = httptest.NewRecorder()
	server.router.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("/log 应需要认证，实际 %d", w.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/log", nil)
	req.Header.Set(core.HeaderAuthorization, core.AuthBearerPrefix+"test-key")
	w = httptest.NewRecorder()
	server.router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("/log 带认证应返回 200，实际 %d", w.Code)
	}
}

type spyStorage struct {
	mu       sync.Mutex
	saveCall int
	lastStat core.RequestStats
}

func (s *spyStorage) SaveStats(stats *core.RequestStats) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.saveCall++
	if stats != nil {
		s.lastStat = *stats
		s.lastStat.RequestHistory = append([]core.RequestRecord(nil), stats.RequestHistory...)
	}
	return nil
}

func (s *spyStorage) LoadStats() (*core.RequestStats, error) {
	return &core.RequestStats{}, nil
}

func (s *spyStorage) Close() error {
	return nil
}

func (s *spyStorage) snapshot() (int, core.RequestStats) {
	s.mu.Lock()
	defer s.mu.Unlock()

	statsCopy := s.lastStat
	statsCopy.RequestHistory = append([]core.RequestRecord(nil), s.lastStat.RequestHistory...)
	return s.saveCall, statsCopy
}

func TestServerClose_PersistsBufferedMetrics(t *testing.T) {
	modelsPath := writeTempTestFile(t, "models_close.json", []byte(`{"models":{"gpt-4o":"openai-gpt-4o"}}`))

	st := &spyStorage{}
	cfg := config.ServerConfig{
		Port:              "0",
		GinMode:           "test",
		ClientAPIKeys:     []string{"test-key"},
		JetbrainsAccounts: []core.JetbrainsAccount{{JWT: "dummy-jwt", LastUpdated: float64(time.Now().Unix()), HasQuota: true}},
		ModelsConfigPath:  modelsPath,
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

	server.metricsService.RecordRequest(true, 10, "gpt-4o", "acct-1")
	server.metricsService.RecordRequest(false, 20, "gpt-4o", "acct-1")

	beforeSaves, beforeStats := st.snapshot()
	if beforeStats.TotalRequests != 1 {
		t.Fatalf("关闭前应只持久化首条记录，实际 total=%d", beforeStats.TotalRequests)
	}

	if err := server.Close(); err != nil {
		t.Fatalf("关闭 Server 失败: %v", err)
	}

	afterSaves, afterStats := st.snapshot()
	if afterSaves <= beforeSaves {
		t.Fatalf("关闭后应触发最终持久化，save 次数 %d -> %d", beforeSaves, afterSaves)
	}
	if afterStats.TotalRequests != 2 {
		t.Fatalf("关闭后应持久化全部请求，实际 total=%d", afterStats.TotalRequests)
	}
	if len(afterStats.RequestHistory) != 2 {
		t.Fatalf("关闭后应持久化完整历史，实际 history=%d", len(afterStats.RequestHistory))
	}
}

func TestServerClose_Idempotent(t *testing.T) {
	server := newTestServer(t)

	if err := server.Close(); err != nil {
		t.Fatalf("第一次关闭失败: %v", err)
	}
	if err := server.Close(); err != nil {
		t.Fatalf("第二次关闭失败: %v", err)
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
