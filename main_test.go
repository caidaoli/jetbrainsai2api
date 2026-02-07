package main

import (
	"os"
	"testing"
)

// TestLoadServerConfigFromEnv_StatsAuthEnabled 验证统计认证开关可由环境变量驱动
func TestLoadServerConfigFromEnv_StatsAuthEnabled(t *testing.T) {
	originalClientKeys := os.Getenv("CLIENT_API_KEYS")
	originalStatsAuth := os.Getenv("STATS_AUTH_ENABLED")
	t.Cleanup(func() {
		_ = os.Setenv("CLIENT_API_KEYS", originalClientKeys)
		if originalStatsAuth == "" {
			_ = os.Unsetenv("STATS_AUTH_ENABLED")
		} else {
			_ = os.Setenv("STATS_AUTH_ENABLED", originalStatsAuth)
		}
	})

	if err := os.Setenv("CLIENT_API_KEYS", "test-key"); err != nil {
		t.Fatalf("设置 CLIENT_API_KEYS 失败: %v", err)
	}

	// 默认值应为 true
	if err := os.Unsetenv("STATS_AUTH_ENABLED"); err != nil {
		t.Fatalf("清理 STATS_AUTH_ENABLED 失败: %v", err)
	}
	config, err := loadServerConfigFromEnv()
	if err != nil {
		t.Fatalf("加载配置失败: %v", err)
	}
	if !config.StatsAuthEnabled {
		t.Fatalf("未设置 STATS_AUTH_ENABLED 时应默认启用统计认证")
	}

	// 显式关闭
	if err := os.Setenv("STATS_AUTH_ENABLED", "false"); err != nil {
		t.Fatalf("设置 STATS_AUTH_ENABLED 失败: %v", err)
	}
	config, err = loadServerConfigFromEnv()
	if err != nil {
		t.Fatalf("加载配置失败: %v", err)
	}
	if config.StatsAuthEnabled {
		t.Fatalf("STATS_AUTH_ENABLED=false 时应关闭统计认证")
	}

	// 标准布尔大小写应被识别
	if err := os.Setenv("STATS_AUTH_ENABLED", "TRUE"); err != nil {
		t.Fatalf("设置 STATS_AUTH_ENABLED 失败: %v", err)
	}
	config, err = loadServerConfigFromEnv()
	if err != nil {
		t.Fatalf("加载配置失败: %v", err)
	}
	if !config.StatsAuthEnabled {
		t.Fatalf("STATS_AUTH_ENABLED=TRUE 时应启用统计认证")
	}

	// 非法值应回退到默认值 true
	if err := os.Setenv("STATS_AUTH_ENABLED", "not-a-bool"); err != nil {
		t.Fatalf("设置 STATS_AUTH_ENABLED 失败: %v", err)
	}
	config, err = loadServerConfigFromEnv()
	if err != nil {
		t.Fatalf("加载配置失败: %v", err)
	}
	if !config.StatsAuthEnabled {
		t.Fatalf("STATS_AUTH_ENABLED 非法值时应回退到默认 true")
	}
}
