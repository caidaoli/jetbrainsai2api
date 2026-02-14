# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## 项目概述

JetBrains AI 转 OpenAI/Anthropic 兼容 API 的代理服务器。支持双 API 格式（OpenAI `/v1/chat/completions` + Anthropic `/v1/messages`），账户池管理，工具调用转换。

**技术栈**: Go 1.23+ (toolchain 1.24), Gin, ByteDance Sonic (JSON), Redis (可选)

## 开发命令

```bash
go build -o jetbrainsai2api ./cmd/server     # 构建
GIN_MODE=debug go run ./cmd/server           # 开发运行
go test -v ./...                             # 全部测试
go test -v ./internal/process/ -run TestRequestProcessor_ProcessMessages  # 单个测试
go test -cover ./...                         # 覆盖率
go fmt ./... && go vet ./...                 # 格式化 + 静态分析
golangci-lint run                            # lint（推荐）
```

需要 `.env` 文件（参考 `.env.example`），核心变量：`CLIENT_API_KEYS`, `JETBRAINS_LICENSE_IDS` + `JETBRAINS_AUTHORIZATIONS`（许可证模式），或 `JETBRAINS_JWTS`（静态 JWT 模式）。

## 架构

多包 `cmd/` + `internal/` 布局，通过接口解耦，依赖单向向下。

### 目录结构

```
cmd/server/main.go              # 入口点
internal/
├── core/                       # 基础类型、接口、常量（无外部依赖，按领域拆分）
│   ├── interfaces.go           # Logger/Cache/Storage/AccountManager/MetricsCollector + Nop 实现
│   ├── types_account.go        # 账户、统计相关类型
│   ├── types_model.go          # 模型映射类型
│   ├── types_openai.go         # OpenAI 请求/响应类型
│   ├── types_jetbrains.go      # JetBrains 请求/响应类型
│   ├── types_anthropic.go      # Anthropic 请求/响应类型
│   ├── constants_infra.go      # 基础设施常量（HTTP、缓存、超时）
│   ├── constants_protocol.go   # 协议常量（SSE、Content-Type）
│   ├── constants_jetbrains.go  # JetBrains API 常量
│   ├── constants_openai.go     # OpenAI 格式常量
│   └── constants_anthropic.go  # Anthropic 格式常量
├── log/logger.go               # 日志实现
├── util/util.go                # 通用工具函数
├── cache/cache.go              # LRU + TTL 缓存，可选 Redis
├── storage/storage.go          # 统计数据持久化
├── metrics/                    # 监控指标 + 嵌入式 Web 面板
│   ├── metrics.go
│   └── static/index.html
├── account/                    # 账户池管理
│   ├── manager.go              # PooledAccountManager
│   └── api.go                  # JWT 刷新、配额查询
├── validate/                   # 输入验证
│   ├── tools.go                # 工具参数名规范化、Schema 简化
│   └── image.go                # 图片格式/大小校验
├── convert/                    # 格式转换（双向）
│   ├── openai.go               # OpenAI → JetBrains
│   ├── anthropic.go            # Anthropic → JetBrains
│   └── jetbrains.go            # JetBrains → Anthropic
├── process/processor.go        # 请求处理器
├── config/config.go            # 配置加载
└── server/                     # HTTP 服务器层
    ├── server.go               # Server 结构体、NewServer、Run、优雅停机
    ├── routes.go               # 路由注册
    ├── middleware.go            # CORS + 客户端认证
    ├── helpers.go              # SSE 写入工具、错误响应
    ├── handler_openai.go       # OpenAI 格式端点
    ├── handler_anthropic.go    # Anthropic 格式端点
    ├── response_openai.go      # JetBrains → OpenAI 流式/非流式
    └── response_anthropic.go   # JetBrains → Anthropic 流式/非流式
```

### 依赖关系（无循环）

```
cmd/server/main
    ↓
internal/server ──→ process, convert, account, metrics, cache, config, validate
    ↓
internal/process ──→ convert, validate, account(via iface), cache(via iface), util
    ↓
internal/convert ──→ validate, util
    ↓
internal/account ──→ cache(via iface), util
    ↓
internal/metrics ──→ storage
    ↓
internal/cache, internal/storage, internal/validate, internal/config
    ↓
internal/log, internal/util
    ↓
internal/core  ← 所有包都依赖此包
```

### 接口层 (`internal/core/interfaces.go`)

所有核心抽象集中定义：`Logger`, `Cache`, `StorageInterface`, `AccountManager`, `MetricsCollector`。
提供 `NopLogger` / `NopMetrics` 空实现用于测试。这是依赖反转的关键——所有组件依赖接口而非具体实现。

### 请求处理链

```
Client → server.corsMiddleware → server.authenticateClient
      → handler_openai.go (OpenAI) 或 handler_anthropic.go (Anthropic)
      → process.RequestProcessor.ProcessMessages / ProcessTools
      → process.RequestProcessor.BuildJetbrainsPayload
      → account.AccountManager.AcquireAccount
      → process.RequestProcessor.SendUpstreamRequest
      → response_openai.go (OpenAI) 或 response_anthropic.go (Anthropic)
```

### 格式转换（双向）

**OpenAI 路径**: `convert/openai.go`（`MessageConverter`，OpenAI → JetBrains） → `server/response_openai.go`（JetBrains → OpenAI，流式/非流式）

**Anthropic 路径**: `convert/anthropic.go`（Anthropic → JetBrains） → `convert/jetbrains.go` + `server/response_anthropic.go`（JetBrains → Anthropic，流式/非流式）

## 编码规范

- **日志**: `logger.Debug/Info/Warn/Error/Fatal`，禁止 `log.Printf`
- **JSON**: `util.MarshalJSON()` 封装 Sonic，禁止直接 `sonic.Marshal` 或 `json.Marshal`
- **类型**: `any` 替代 `interface{}`
- **依赖方向**: `server` → `process` → `account`，禁止反向。Server 通过依赖注入持有所有组件
- **HTTP**: 所有外部 API 调用通过 `httpClient`（连接池复用）
- **缓存键**: 必须包含 `CacheKeyVersion`，避免格式变更后脏数据
- **测试**: table-driven tests，包内测试（`package xxx` 非 `xxx_test`），Mock 外部依赖

## 配置

- **`models.json`**: `"openai_model_id": "jetbrains_internal_id"` 映射，热更新
- **`.env`**: 三种账户模式——静态 JWT / 许可证（推荐，支持自动刷新）/ 混合
- **Docker**: distroless 镜像，端口 7860

## 监控端点

- `/` — Web 监控面板
- `/api/stats` — JSON 统计
- `/health` — 健康检查

## MCP 工具使用规范

**优先使用 Serena MCP 工具进行代码探索和编辑**：
- 探索: `get_symbols_overview` → `find_symbol(include_body=true)` 按需读取
- 编辑: `replace_symbol_body` / `insert_after_symbol` / `insert_before_symbol`
- 搜索: `search_for_pattern`（避免全文件读取）
- 引用: `find_referencing_symbols`
- 标准 Read/Write 工具仅用于非代码文件（`.md`/`.json`/`.yaml`）
