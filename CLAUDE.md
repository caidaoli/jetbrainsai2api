# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## 项目概述

JetBrains AI 转 OpenAI/Anthropic 兼容 API 的代理服务器。支持双 API 格式（OpenAI `/v1/chat/completions` + Anthropic `/v1/messages`），账户池管理，工具调用转换。

**技术栈**: Go 1.23+ (toolchain 1.24), Gin, ByteDance Sonic (JSON), Redis (可选)

## 开发命令

```bash
go build -o jetbrainsai2api .              # 构建
GIN_MODE=debug go run .                     # 开发运行
go test -v ./...                            # 全部测试
go test -v -run TestRequestProcessor_ProcessMessages  # 单个测试
go test -cover ./...                        # 覆盖率
go fmt ./... && go vet ./...                # 格式化 + 静态分析
golangci-lint run                           # lint（推荐）
```

需要 `.env` 文件（参考 `.env.example`），核心变量：`CLIENT_API_KEYS`, `JETBRAINS_LICENSE_IDS` + `JETBRAINS_AUTHORIZATIONS`（许可证模式），或 `JETBRAINS_JWTS`（静态 JWT 模式）。

## 架构

单 package `main`，所有 `.go` 文件在根目录，通过接口解耦。

### 接口层 (`interfaces.go`)

所有核心抽象集中定义：`Logger`, `Cache`, `StorageInterface`, `AccountManager`, `MetricsCollector`。
提供 `NopLogger` / `NopMetrics` 空实现用于测试。这是依赖反转的关键——所有组件依赖接口而非具体实现。

### 请求处理链

```
Client → Server.corsMiddleware → Server.authenticateClient
      → handlers.go (OpenAI) 或 anthropic_handlers.go (Anthropic)
      → RequestProcessor.ProcessMessages / ProcessTools
      → RequestProcessor.BuildJetbrainsPayload
      → AccountManager.AcquireAccount
      → RequestProcessor.SendUpstreamRequest
      → response_handler.go (OpenAI) 或 anthropic_response_handler.go (Anthropic)
```

### 核心组件

| 文件 | 职责 |
|------|------|
| `server.go` | `Server` 结构体：路由、依赖注入、优雅停机。通过 `NewServer()` 组装所有依赖 |
| `config.go` | `ServerConfig` / `HTTPClientSettings`，模型映射加载 (`loadModels`) |
| `main.go` | 入口：`loadServerConfigFromEnv()` → `loadJetbrainsAccountsFromEnv()` → `NewServer()` |
| `middleware.go` | CORS (`corsMiddleware`) + 客户端认证 (`authenticateClient`) |
| `handlers.go` | OpenAI 格式端点：`listModels`, `chatCompletions` |
| `anthropic_handlers.go` | Anthropic 格式端点：`anthropicMessages`, `callJetbrainsAPIDirect` |
| `handler_helpers.go` | SSE 写入工具、错误响应（OpenAI/Anthropic 两种格式）、panic recovery |
| `request_processor.go` | `RequestProcessor`：消息转换、工具处理、payload 构建、上游请求发送 |
| `account_manager.go` | `PooledAccountManager`：账户池、负载均衡、`AcquireAccount`/`ReleaseAccount` |
| `jetbrains_api.go` | JWT 刷新、配额查询、请求头设置、JWT 过期重试 |

### 格式转换（双向）

**OpenAI 路径**: `converter.go`（`MessageConverter`，OpenAI → JetBrains） → `response_handler.go`（JetBrains → OpenAI，流式/非流式）

**Anthropic 路径**: `anthropic_direct_converter.go`（Anthropic → JetBrains） → `jetbrains_direct_converter.go`（JetBrains → Anthropic，流式/非流式）

### 支撑组件

| 文件 | 职责 |
|------|------|
| `models.go` | 所有数据结构：请求/响应类型（OpenAI, Anthropic, JetBrains）、`JetbrainsAccount` |
| `constants.go` | 所有常量：API URL、超时、缓存 TTL、协议字段名、`APIFormatOpenAI` / `APIFormatAnthropic` |
| `tools_validator.go` | 工具参数名规范化（64字符限制，`[a-zA-Z0-9_.-]`）、JSON Schema 简化（anyOf/oneOf/allOf 扁平化） |
| `image_validator.go` | 图片格式/大小校验 |
| `cache.go` | LRU + TTL 缓存，可选 Redis 后端 |
| `metrics.go` | `MetricsService`：QPS、响应时间、成功率 |
| `storage.go` | 统计数据持久化（`stats.json`），防抖写入 |
| `logger.go` | 统一日志接口，debug 模式自动检测 |
| `utils.go` | ID 生成、环境变量解析、JetBrains 请求构造 |

## 编码规范

- **日志**: `logger.Debug/Info/Warn/Error/Fatal`，禁止 `log.Printf`
- **JSON**: `marshalJSON()` 封装 Sonic，禁止直接 `sonic.Marshal` 或 `json.Marshal`
- **类型**: `any` 替代 `interface{}`
- **依赖方向**: `handlers` → `RequestProcessor` → `AccountManager`，禁止反向。Server 通过依赖注入持有所有组件
- **HTTP**: 所有外部 API 调用通过 `httpClient`（连接池复用）
- **缓存键**: 必须包含 `CacheKeyVersion`，避免格式变更后脏数据
- **测试**: table-driven tests，Mock 外部依赖（参考 `*_test.go`）

## 配置

- **`models.json`**: `"openai_model_id": "jetbrains_internal_id"` 映射，热更新
- **`.env`**: 三种账户模式——静态 JWT / 许可证（推荐，支持自动刷新）/ 混合
- **Docker**: distroless 镜像，端口 7860

## 监控端点

- `/` — Web 监控面板
- `/api/stats` — JSON 统计
- `/api/health` — 健康检查

## MCP 工具使用规范

**优先使用 Serena MCP 工具进行代码探索和编辑**：
- 探索: `get_symbols_overview` → `find_symbol(include_body=true)` 按需读取
- 编辑: `replace_symbol_body` / `insert_after_symbol` / `insert_before_symbol`
- 搜索: `search_for_pattern`（避免全文件读取）
- 引用: `find_referencing_symbols`
- 标准 Read/Write 工具仅用于非代码文件（`.md`/`.json`/`.yaml`）
