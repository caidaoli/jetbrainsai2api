# JetBrains AI to OpenAI API Bridge

一个用 Go 语言编写的高性能 JetBrains AI 转 OpenAI 兼容 API 的代理服务器。它使用 Gin 框架，支持将 JetBrains AI 接口无缝转换为标准 OpenAI 格式，方便与现有 OpenAI 客户端和工具集成，并提供完整的监控、统计和管理功能。

## 核心特性

### 🔗 API 兼容性
- **完整的 OpenAI API 兼容**: 支持 `/v1/models` 和 `/v1/chat/completions` 端点
- **多种认证方式**: 支持 Bearer token 和 `x-api-key` 头部认证
- **流式和非流式响应**: 完整支持实时流式输出和标准批量响应

### 🛠️ 工具调用 (Function Calling)
- **智能工具验证**: 自动验证工具参数名称和结构，确保 JetBrains API 兼容性
- **复杂参数转换**: 智能处理 `anyOf`、`oneOf`、`allOf` 等复杂 JSON Schema 结构
- **参数名称规范化**: 自动修正不符合 JetBrains API 要求的参数名（最大64字符，仅支持字母数字和 `_.-`）
- **嵌套对象优化**: 对于过于复杂的嵌套参数，自动转换为兼容格式
- **强制工具使用**: 当提供工具时自动优化提示以确保工具被正确调用

### ⚡ 性能优化 (最新重构)
- **账户池管理**: 多账户负载均衡，支持自动故障转移
- **统一 JSON 序列化**: 全面使用 ByteDance Sonic 高性能 JSON 库，显著提升序列化性能
- **智能缓存系统**:
  - 消息转换缓存 (10分钟 TTL)
  - 工具验证缓存 (30分钟 TTL)
  - 配额查询缓存 (1小时 TTL)
- **连接池优化**:
  - 最大连接数: 500
  - 每主机连接数: 100
  - HTTP/2 支持
  - 10分钟连接保持
- **代码优化**: 消除重复代码，统一错误处理，提高代码可维护性
- **异步统计持久化**: 防抖机制避免频繁I/O操作

### 🎯 账户管理
- **自动 JWT 刷新**: 智能检测 JWT 过期并自动刷新（过期前12小时）
- **配额实时监控**: 自动检查账户配额状态，支持配额耗尽自动切换
- **账户健康检查**: 实时监控账户状态和可用性
- **许可证支持**: 支持许可证ID和授权token模式

### 📊 监控和统计
- **实时Web界面**: 访问根路径查看详细统计信息
- **性能指标**: QPS监控、响应时间统计、成功率分析
- **账户状态监控**: 配额使用情况、过期时间预警
- **历史数据**: 24小时/7天/30天的详细统计报告
- **健康检查端点**: `/health` 提供服务状态信息

### 🔧 模型映射
- **灵活配置**: 通过 `models.json` 文件配置模型映射关系
- **热更新支持**: 修改配置文件后无需重启服务
- **多厂商支持**: 同时支持 Anthropic、Google、OpenAI 等多个AI厂商的模型

## 支持的模型

根据 `models.json` 配置，当前支持以下模型：

### 🤖 Anthropic Claude 系列
- **claude-4-opus**: 最新 Claude 4 Opus 模型
- **claude-4-1-opus**: Claude 4.1 Opus 版本
- **claude-4-sonnet**: Claude 4 Sonnet 模型
- **claude-3-7-sonnet**: Claude 3.7 Sonnet 版本
- **claude-3-5-sonnet**: Claude 3.5 Sonnet 模型
- **claude-3-5-haiku**: Claude 3.5 Haiku 快速版本

### 🧠 Google Gemini 系列
- **gemini-2.5-pro**: Gemini 2.5 Pro 专业版
- **gemini-2.5-flash**: Gemini 2.5 Flash 快速版

### 🔮 OpenAI 系列
- **o4-mini**: OpenAI o4-mini 模型
- **o3-mini**: OpenAI o3-mini 轻量版
- **o3**: OpenAI o3 标准版
- **o1**: OpenAI o1 模型
- **gpt-4o**: GPT-4 Omni 模型
- **gpt-4.1 系列**: gpt-4.1, gpt-4.1-mini, gpt-4.1-nano
- **gpt-5 系列**: gpt-5, gpt-5-mini, gpt-5-nano (最新版本)

> **注意**: 模型可用性取决于您的 JetBrains AI 账户权限和配额限制

## 🚀 快速开始

### 1. 环境配置

首先复制环境配置文件并配置必要参数：

```bash
cp .env.example .env
```

编辑 `.env` 文件，配置以下关键参数：

```bash
# 客户端API密钥（用于访问此服务）
CLIENT_API_KEYS=your-api-key-1,your-api-key-2

# 方式1：使用许可证模式（推荐）
JETBRAINS_LICENSE_IDS=your-license-id-1,your-license-id-2
JETBRAINS_AUTHORIZATIONS=your-auth-token-1,your-auth-token-2

# 方式2：使用静态JWT模式（不推荐，JWT会过期）
# JETBRAINS_JWTS=your-jwt-token-1,your-jwt-token-2

# 可选配置
PORT=7860                    # 服务端口
GIN_MODE=release            # 运行模式 (debug/release)
STATS_AUTH_ENABLED=true     # 统计页面/API是否需要鉴权
REDIS_URL=redis://localhost:6379  # Redis缓存（可选）
```

### 2. 运行服务

#### 方式一：直接运行 Go 程序
```bash
# 安装依赖
go mod tidy

# 构建可执行文件
go build -o jetbrainsai2api *.go

# 运行服务
./jetbrainsai2api

# 开发模式（显示详细日志）
GIN_MODE=debug ./jetbrainsai2api
```

#### 方式二：使用 Docker
```bash
# 构建镜像
docker build -t jetbrainsai2api .

# 运行容器
docker run -p 7860:7860 \
  -e TZ=Asia/Shanghai \
  -e CLIENT_API_KEYS=your-api-key \
  -e JETBRAINS_LICENSE_IDS=your-license-id \
  -e JETBRAINS_AUTHORIZATIONS=your-auth-token \
  jetbrainsai2api
```

#### 方式三：使用 Docker Compose
```bash
# 启动服务
docker-compose up -d

# 查看日志
docker-compose logs -f
```

### 3. 验证服务
```bash
# 检查服务状态
curl http://localhost:7860/health

# 获取模型列表
curl -H "Authorization: Bearer your-api-key" \
  http://localhost:7860/v1/models

# 访问监控面板
open http://localhost:7860/
```

## 📚 API 使用指南

### 基本聊天补全
```bash
# 简单对话
curl -H "Authorization: Bearer your-api-key" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "claude-4-sonnet",
    "messages": [{"role": "user", "content": "你好！"}],
    "stream": false
  }' \
  http://localhost:7860/v1/chat/completions

# 流式响应
curl -H "Authorization: Bearer your-api-key" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gemini-2.5-pro",
    "messages": [{"role": "user", "content": "写一首诗"}],
    "stream": true
  }' \
  http://localhost:7860/v1/chat/completions
```

### 获取可用模型
```bash
curl -H "Authorization: Bearer your-api-key" \
  http://localhost:7860/v1/models
```

### 🔧 工具调用 (Function Calling)

系统提供强大的工具调用功能，自动处理复杂参数验证和转换：

```bash
# 简单工具调用
curl -H "Authorization: Bearer your-api-key" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "claude-4-sonnet",
    "messages": [{"role": "user", "content": "北京的天气怎么样？"}],
    "tools": [{
      "type": "function",
      "function": {
        "name": "get_weather",
        "description": "获取指定城市的天气信息",
        "parameters": {
          "type": "object",
          "properties": {
            "location": {"type": "string", "description": "城市名称"}
          },
          "required": ["location"]
        }
      }
    }],
    "tool_choice": "auto"
  }' \
  http://localhost:7860/v1/chat/completions

# 复杂嵌套参数工具调用
curl -H "Authorization: Bearer your-api-key" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "claude-4-sonnet",
    "messages": [{"role": "user", "content": "创建一个新用户"}],
    "tools": [{
      "type": "function",
      "function": {
        "name": "create_user",
        "description": "创建新用户",
        "parameters": {
          "type": "object",
          "properties": {
            "user_info": {
              "type": "object",
              "properties": {
                "name": {"type": "string"},
                "email": {"type": "string"},
                "address": {
                  "type": "object",
                  "properties": {
                    "street": {"type": "string"},
                    "city": {"type": "string"}
                  }
                }
              },
              "required": ["name", "email"]
            }
          },
          "required": ["user_info"]
        }
      }
    }]
  }' \
  http://localhost:7860/v1/chat/completions
```

#### 工具调用特性
- **智能参数验证**: 自动检查参数名称长度（≤64字符）和字符规范
- **复杂结构简化**: 自动处理 `anyOf`/`oneOf`/`allOf` JSON Schema
- **嵌套对象优化**: 超过15个属性的复杂工具自动简化
- **强制工具使用**: 提供工具时自动增强提示确保工具被调用
- **参数名称转换**: 自动修正不符合规范的参数名

### 使用 x-api-key 认证
```bash
# 使用 x-api-key 头部认证
curl -H "x-api-key: your-api-key" \
  -H "Content-Type: application/json" \
  -d '{"model": "claude-4-sonnet", "messages": [...]}' \
  http://localhost:7860/v1/chat/completions
```

## 📊 监控和统计

### Web 监控面板
- **访问地址**: `http://localhost:7860/`
- **功能**: 实时QPS、成功率、响应时间、账户状态监控
- **历史数据**: 24小时/7天/30天统计报告

### API 端点
```bash
# 获取统计数据
curl http://localhost:7860/api/stats

# 健康检查
curl http://localhost:7860/health

# 实时日志流（SSE）
curl http://localhost:7860/log
```

### 监控指标
- **请求统计**: 总请求数、成功率、失败数
- **性能指标**: 平均响应时间、QPS（每秒查询数）
- **账户监控**: 配额使用情况、JWT过期时间
- **缓存效率**: 命中率统计（消息转换、工具验证、配额查询）

## ⚙️ 配置文件

### models.json 配置
定义可用模型及其到 JetBrains AI 内部模型的映射关系：

```json
{
  "models": {
    "claude-4-opus": "anthropic-claude-4-opus",
    "claude-4-sonnet": "anthropic-claude-4-sonnet",
    "claude-3-5-sonnet": "anthropic-claude-3.5-sonnet",
    "gemini-2.5-pro": "google-chat-gemini-pro-2.5",
    "gemini-2.5-flash": "google-chat-gemini-flash-2.5",
    "o4-mini": "openai-o4-mini",
    "gpt-4o": "openai-gpt-4o",
    "gpt-5": "openai-gpt-5"
  }
}
```

**配置说明**:
- **键名**: 对外暴露的模型名称（OpenAI API 兼容）
- **键值**: JetBrains AI 内部模型标识符
- **热更新**: 修改配置文件后无需重启服务即可生效

### 环境变量配置

#### 必需配置
```bash
# 客户端API密钥（逗号分隔多个密钥）
CLIENT_API_KEYS=key1,key2,key3

# 方式1：许可证模式（推荐）
JETBRAINS_LICENSE_IDS=license-id-1,license-id-2
JETBRAINS_AUTHORIZATIONS=auth-token-1,auth-token-2

# 方式2：静态JWT模式（不推荐，会过期）
JETBRAINS_JWTS=jwt-token-1,jwt-token-2
```

#### 可选配置
```bash
PORT=7860                                    # 服务监听端口
GIN_MODE=release                            # 运行模式: debug/release/test
REDIS_URL=redis://localhost:6379           # Redis缓存连接（可选）
TZ=Asia/Shanghai                           # 时区设置
```

#### 高级性能配置
```bash
# HTTP客户端配置（代码中硬编码的默认值）
MAX_IDLE_CONNS=500                         # 最大空闲连接数
MAX_IDLE_CONNS_PER_HOST=100               # 每主机最大空闲连接数
MAX_CONNS_PER_HOST=200                    # 每主机最大连接数
IDLE_CONN_TIMEOUT=600s                    # 空闲连接超时
TLS_HANDSHAKE_TIMEOUT=30s                 # TLS握手超时
```

## 🔧 开发指南

### 本地开发
```bash
# 克隆项目
git clone <repository-url>
cd jetbrainsai2api

# 安装依赖
go mod tidy

# 启动开发模式（显示详细日志）
GIN_MODE=debug go run *.go

# 构建生产版本
go build -o jetbrainsai2api *.go
```

### 🎯 重构亮点 (v2024.8)
- **性能提升**: 统一使用 Sonic JSON 库，JSON 序列化性能提升 2-5x
- **代码质量**: 消除重复代码，减少维护成本
- **导入优化**: 移除未使用的导入，减少二进制文件大小
- **错误处理**: 统一错误处理模式，提高系统稳定性
- **类型安全**: 强化类型检查，减少运行时错误

### 代码质量工具
```bash
# 代码格式化
go fmt ./...

# 静态分析
go vet ./...

# 构建检查
go build -o jetbrainsai2api *.go

# 依赖清理
go mod tidy
```

## 🚀 部署指南

### Docker 部署
```yaml
# docker-compose.yml
version: '3.8'
services:
  jetbrainsai2api:
    build: .
    ports:
      - "7860:7860"
    environment:
      - CLIENT_API_KEYS=${CLIENT_API_KEYS}
      - JETBRAINS_LICENSE_IDS=${JETBRAINS_LICENSE_IDS}
      - JETBRAINS_AUTHORIZATIONS=${JETBRAINS_AUTHORIZATIONS}
      - GIN_MODE=release
      - TZ=Asia/Shanghai
    volumes:
      - ./stats.json:/app/stats.json
      - ./models.json:/app/models.json
    restart: unless-stopped
```

### 生产环境建议
- **负载均衡**: 使用Nginx/HAProxy进行负载均衡
- **反向代理**: 配置SSL终端和缓存
- **监控**: 集成Prometheus + Grafana监控
- **日志**: 使用ELK Stack收集和分析日志
- **备份**: 定期备份`stats.json`统计数据

### HuggingFace Spaces
```bash
# 1. Fork项目到GitHub
# 2. 创建HuggingFace Space (Docker SDK)
# 3. 配置Repository secrets:
CLIENT_API_KEYS=your-keys
JETBRAINS_LICENSE_IDS=your-license-ids
JETBRAINS_AUTHORIZATIONS=your-auth-tokens
```

## 🔍 故障排除

### 常见问题

#### JWT相关问题
```bash
# 问题: JWT过期或无效
# 解决: 检查许可证配置，服务会自动刷新
tail -f logs/app.log | grep "JWT"

# 问题: JWT刷新失败
# 解决: 验证JETBRAINS_AUTHORIZATIONS配置
curl -X POST https://api.jetbrains.ai/auth/jetbrains-jwt/provide-access/license/v2
```

#### 配额问题
```bash
# 问题: 账户配额不足 (HTTP 477)
# 解决: 添加更多账户或等待配额重置
curl http://localhost:7860/api/stats | jq '.tokensInfo'
```

#### 性能问题
```bash
# 问题: 响应时间过长
# 解决: 检查连接池和缓存配置，监控Web界面查看QPS和响应时间
curl http://localhost:7860/api/stats | jq '.performance'

# 问题: 内存使用过高
# 解决: 启用race检测模式运行，检查并发问题
go build -race -o jetbrainsai2api *.go
GIN_MODE=debug ./jetbrainsai2api
```

#### 工具调用问题
```bash
# 问题: 工具参数验证失败
# 解决: 启用调试模式查看详细转换过程
GIN_MODE=debug ./jetbrainsai2api

# 问题: 复杂嵌套参数无法处理
# 解决: 检查参数名称长度和字符规范（≤64字符，仅a-zA-Z0-9_.-）
```

### 调试技巧
- **开启调试日志**: `GIN_MODE=debug`
- **实时监控**: Web界面 `http://localhost:7860/`
- **健康检查**: `curl http://localhost:7860/health`
- **统计API**: `curl http://localhost:7860/api/stats`
- **性能监控**: 通过统计面板查看QPS、响应时间和缓存命中率

### 日志说明
```bash
# 工具验证日志
2024/01/01 12:00:00 === TOOL VALIDATION DEBUG START ===
2024/01/01 12:00:00 Original tools count: 2
2024/01/01 12:00:00 Successfully validated tool: get_weather

# 账户池日志
2024/01/01 12:00:00 Account pool initialized with 3 accounts
2024/01/01 12:00:00 Successfully refreshed JWT for licenseId xxx

# 缓存日志
2024/01/01 12:00:00 Cache hit for message conversion
2024/01/01 12:00:00 Cache miss for tool validation
```

## 📄 许可证

本项目基于 MIT 许可证开源，详细条款如下：

```
MIT License

Copyright (c) 2024 JetBrains AI to OpenAI API Bridge

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
```

## 🤝 贡献指南

欢迎贡献代码和反馈！在提交Pull Request之前，请确保：

1. **代码质量**: 遵循Go语言编码规范
2. **文档更新**: 更新相关文档和README
3. **性能考虑**: 确保不会显著影响现有性能

### 提交流程
```bash
# 1. Fork项目并创建功能分支
git checkout -b feature/new-feature

# 2. 开发和代码检查
go fmt ./...

# 3. 提交更改
git commit -m "feat: add new feature"

# 4. 推送并创建Pull Request
git push origin feature/new-feature
```

## 🔗 相关链接

- **JetBrains AI**: https://ai.jetbrains.com/
- **OpenAI API**: https://platform.openai.com/docs/api-reference
- **Go语言**: https://golang.org/
- **Gin框架**: https://github.com/gin-gonic/gin

---

**免责声明**: 本项目为非官方实现，与JetBrains公司无正式关联。使用前请确保遵守相关服务条款。
