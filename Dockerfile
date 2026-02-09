# HuggingFace Spaces 和通用部署用 Dockerfile
# 使用官方 Go 镜像作为构建基础，更新到最新版本
FROM golang:1.24-alpine AS builder

# 设置标签
LABEL maintainer="caidaoli <caidaoli@gmail.com>"
LABEL description="JetBrains AI 转 OpenAI 兼容 API 代理服务器"
LABEL version="latest"

# 安装构建依赖和工具
RUN apk add --no-cache \
    git \
    ca-certificates \
    tzdata \
    curl \
    && update-ca-certificates

# 设置工作目录
WORKDIR /app

# 优化 Go 模块缓存，先复制 go.mod 和 go.sum
COPY go.mod go.sum ./

# 下载依赖并验证
RUN go mod download && go mod verify

# 复制源代码
COPY . .

# 设置目标平台变量
ARG TARGETOS
ARG TARGETARCH


# 构建应用，添加优化参数
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -a -installsuffix cgo \
    -ldflags='-w -s -extldflags "-static"' \
    -o jetbrainsai2api ./cmd/server

# 使用更安全的 distroless 镜像作为运行时基础
FROM gcr.io/distroless/static:nonroot

# 设置标签
LABEL maintainer="caidaoli <caidaoli@gmail.com>"
LABEL description="JetBrains AI 转 OpenAI 兼容 API 代理服务器"

# 设置工作目录
WORKDIR /app

# 从构建镜像复制时区数据
COPY --from=builder /usr/share/zoneinfo /usr/share/zoneinfo

# 从构建镜像复制二进制文件
COPY --from=builder /app/jetbrainsai2api .

# 复制配置文件
COPY --from=builder /app/models.json .

# 创建数据目录（distroless 镜像已经使用 nonroot 用户）
USER 65532:65532

# 暴露端口
EXPOSE 7860

# 设置环境变量
ENV ADDR=0.0.0.0
ENV PORT=7860
ENV GIN_MODE=release
ENV TZ=Asia/Shanghai

# 启动应用
ENTRYPOINT ["./jetbrainsai2api"]