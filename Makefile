BINARY    := jetbrainsai2api
CMD       := ./cmd/server
IMAGE     := jetbrainsai2api
TAG       := latest
GOFLAGS   := -trimpath
LDFLAGS   := -w -s

.PHONY: build run test cover lint fmt vet clean docker help

build: ## 构建二进制
	go build $(GOFLAGS) -ldflags '$(LDFLAGS)' -o $(BINARY) $(CMD)

run: ## 开发模式运行
	GIN_MODE=debug go run $(CMD)

test: ## 运行全部测试
	go test -v ./...

cover: ## 测试覆盖率
	go test -cover ./...

cover-html: ## 生成 HTML 覆盖率报告
	go test -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html
	@echo "coverage.html generated"

lint: fmt vet ## 格式化 + 静态分析 + lint
	@command -v golangci-lint >/dev/null 2>&1 && golangci-lint run || echo "golangci-lint not installed, skipped"

fmt: ## 格式化代码
	go fmt ./...

vet: ## 静态分析
	go vet ./...

clean: ## 清理构建产物
	rm -f $(BINARY) coverage.out coverage.html

docker: ## 构建 Docker 镜像
	docker build -t $(IMAGE):$(TAG) .

help: ## 显示帮助
	@grep -E '^[a-zA-Z_-]+:.*?## ' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-12s\033[0m %s\n", $$1, $$2}'

.DEFAULT_GOAL := help
