# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## 项目概述

JetBrains AI 转 OpenAI 兼容 API 的代理服务器。核心功能：API 格式转换、账户池管理、工具调用优化、性能监控。

**关键技术栈**: Go 1.23+, Gin, ByteDance Sonic (JSON), Redis (可选缓存)

## 开发命令

### 构建和运行
```bash
# 依赖管理
go mod download && go mod tidy

# 构建
go build -o jetbrainsai2api *.go

# 开发运行（推荐）
GIN_MODE=debug go run *.go

# 生产运行
GIN_MODE=release ./jetbrainsai2api

# 代码检查
go fmt ./... && go vet ./...
```

### 测试
```bash
# 运行所有测试
go test -v ./...

# 运行特定测试
go test -v -run TestRequestProcessor_ProcessMessages

# 带 Redis 的测试（需要本地 Redis）
REDIS_URL="redis://localhost:6379" go test -v -run TestInitStorage

# 测试覆盖率
go test -cover ./...
```

### 环境配置
必须配置 `.env` 文件（参考 `.env.example`）：
- `CLIENT_API_KEYS`: 客户端认证密钥（逗号分隔）
- `JETBRAINS_LICENSE_IDS` + `JETBRAINS_AUTHORIZATIONS`: 许可证模式（推荐）
- `JETBRAINS_JWTS`: 静态 JWT 模式（会过期）
- `GIN_MODE`: debug/release/test
- `REDIS_URL`: 可选，用于分布式缓存

## 核心架构

### 三层架构设计

```
HTTP 请求 → Server → RequestProcessor → JetBrains API
              ↓            ↓
         AccountManager  Cache
```

**Server** (`server.go`):
- HTTP 服务器和路由管理
- 中间件：CORS、认证、超时控制
- 优雅停机和健康检查
- 依赖注入：AccountManager, RequestProcessor

**RequestProcessor** (`request_processor.go`):
- 请求预处理：消息转换、工具验证
- 负责构建 JetBrains API payload
- 上游请求发送和响应处理
- 核心方法：
  - `ProcessMessages()`: OpenAI 消息 → JetBrains 格式
  - `ProcessTools()`: 工具参数验证和规范化
  - `BuildJetbrainsPayload()`: 构建完整请求体
  - `SendUpstreamRequest()`: 发送上游请求

**AccountManager** (`account_manager.go`):
- 账户池管理：负载均衡、故障转移
- JWT 自动刷新（过期前12小时）
- 配额监控和账户健康检查
- 接口设计：`AcquireAccount()`, `ReleaseAccount()`, `RefreshJWT()`, `CheckQuota()`

### 关键组件

**格式转换** (`converter.go`, `anthropic_*.go`, `jetbrains_*.go`):
- OpenAI ↔ JetBrains API 双向转换
- 支持多种消息类型（text, image, tool_call, tool_result）
- 流式和非流式响应处理
- 特殊处理：图像验证 (`image_validator.go`)

**工具调用优化** (`tools_validator.go`):
- 参数名称规范化（最大64字符，仅 `[a-zA-Z0-9_.-]`）
- JSON Schema 简化（anyOf/oneOf/allOf → 扁平结构）
- 强制工具使用机制（`tool_choice` 处理）
- 缓存验证结果（30分钟 TTL）

**缓存系统** (`cache.go`):
- LRU 缓存实现，支持 TTL
- 三种缓存：消息转换(10min)、工具验证(30min)、配额查询(1h)
- 可选 Redis 后端（分布式场景）
- 统一 JSON 序列化：`marshalJSON()` 封装 Sonic 库

**性能监控** (`stats.go`, `performance.go`, `storage.go`):
- 实时指标：QPS、响应时间、成功率、错误率
- 统计数据异步持久化（防抖机制）
- Web 监控面板：`/` (HTML), `/api/stats` (JSON)

**统一日志** (`logger.go`):
- 接口：Debug/Info/Warn/Error/Fatal
- 自动调试模式检测（仅 debug 模式输出 Debug 日志）

### 数据流

1. **请求流**:
   ```
   Client → Server.corsMiddleware → Server.authenticate
        → handlers.handleChatCompletion
        → RequestProcessor.ProcessMessages/ProcessTools
        → RequestProcessor.BuildJetbrainsPayload
        → AccountManager.AcquireAccount
        → RequestProcessor.SendUpstreamRequest
        → response_handler (流式/非流式)
   ```

2. **账户管理流**:
   ```
   启动 → loadJetbrainsAccountsFromEnv
        → AccountManager.RefreshJWT (后台定时)
        → AccountManager.CheckQuota (请求时)
        → 账户池负载均衡
   ```

3. **缓存流**:
   ```
   请求 → 检查缓存 → 命中返回 / 未命中计算
        → 写入缓存（带 TTL）
        → 可选写入 Redis
   ```

## 开发规范

**架构约束**:
- 依赖方向：`handlers` → `RequestProcessor` → `AccountManager`，禁止反向依赖
- Server 通过依赖注入持有组件，禁止全局变量
- 所有外部 API 调用必须通过 `httpClient`（连接池复用）
- 缓存键必须包含版本号，避免格式变更导致的脏数据

**编码规范**:
- 日志：使用 `logger.Debug/Info/Warn/Error/Fatal`，禁止 `log.Printf`
- JSON：使用 `marshalJSON()`，禁止直接调用 `sonic.Marshal` 或 `json.Marshal`
- 类型：使用 `any` 替代 `interface{}`
- 错误处理：返回 error，记录上下文（账户ID、请求ID）

**测试要求**:
- 新功能必须添加单元测试（参考 `request_processor_test.go`）
- 使用 table-driven tests 模式
- Mock 外部依赖（JetBrains API、Redis）

## MCP工具使用规范

**⚠️ 强制要求: 优先使用Serena MCP工具**

**代码探索**:
```
mcp__serena__get_symbols_overview → mcp__serena__find_symbol(include_body=true)
```

**代码编辑**(Symbol级别):
```
mcp__serena__replace_symbol_body / insert_after_symbol / insert_before_symbol
```

**代码搜索**:
```
mcp__serena__search_for_pattern  # 避免全文件读取
```

**依赖分析**:
```
mcp__serena__find_referencing_symbols  # 查找符号引用
```

**Token效率原则**:
- ❌ 禁止: 不加思考使用`Read`读取整个文件
- ✅ 推荐: Overview → Find Symbol → 精确编辑
- ⚠️ 标准工具: 仅用于非代码文件(`.md`/`.json`/`.yaml`)

## 配置文件

**models.json**: 模型映射配置
- 格式：`"openai_model_id": "jetbrains_internal_id"`
- 支持：Anthropic Claude, Google Gemini, OpenAI GPT 系列
- 热更新：修改后无需重启（下次请求生效）

**.env**: 环境变量（见 `.env.example`）
- 三种账户模式：静态 JWT / 许可证 / 混合
- 推荐许可证模式（支持自动刷新）

## 调试和监控

**本地调试**:
```bash
GIN_MODE=debug go run *.go  # 启用详细日志
```

**监控端点**:
- `/`: Web 监控面板（实时统计）
- `/api/stats`: JSON 格式统计数据
- `/api/health`: 健康检查

**关键日志位置**:
- 账户管理：`account_manager.go` - JWT 刷新、配额检查
- 工具验证：`tools_validator.go` - 参数规范化、Schema 简化
- 请求处理：`request_processor.go` - 消息转换、payload 构建
