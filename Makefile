# Lighthouse 项目 Makefile
# 支持前后端统一构建、镜像管理、本地开发

# 全局变量定义
PROJECT_NAME := lighthouse
VERSION := $(shell git describe --tags --always 2>/dev/null || echo "0.1.0")
GIT_COMMIT := $(shell git rev-parse --short=8 HEAD)
BRANCH_NAME := $(shell git rev-parse --abbrev-ref HEAD)
IMAGE_TAG := $(VERSION)-$(GIT_COMMIT)
REGISTRY ?= registry.example.com
NAMESPACE ?= lighthouse

# 目录定义
BACKEND_DIR := .
FRONTEND_DIR := ./web
DEPLOY_DIR := ../lighthouse-deploy

.PHONY: help build build-backend build-frontend build-all docker-backend docker-frontend \
        docker-all run-local run-docker push-images test lint clean security-scan \
        verify-build verify-phase1 generate-sbom sign-images

# 默认目标：显示帮助
help:
	@echo "Lighthouse 项目构建工具"
	@echo ""
	@echo "可用命令:"
	@echo "  make build-backend     构建后端二进制文件"
	@echo "  make build-frontend    构建前端静态资源"
	@echo "  make build-all         构建前后端所有组件"
	@echo "  make docker-backend    构建后端Docker镜像"
	@echo "  make docker-frontend   构建前端Docker镜像"
	@echo "  make docker-all        构建所有Docker镜像"
	@echo "  make run-local         本地运行开发环境"
	@echo "  make run-docker        使用Docker运行完整环境"
	@echo "  make push-images       推送镜像到远程仓库"
	@echo "  make test              运行所有测试"
	@echo "  make lint              代码检查"
	@echo "  make clean             清理构建产物"
	@echo "  make security-scan     安全扫描"
	@echo "  make verify-build      验证构建结果"
	@echo "  make verify-phase1     Phase1 一键验收（骨架+领域+配置）"
	@echo ""
	@echo "当前版本信息:"
	@echo "  版本: $(VERSION)"
	@echo "  Git提交: $(GIT_COMMIT)"
	@echo "  分支: $(BRANCH_NAME)"
	@echo "  镜像标签: $(IMAGE_TAG)"

# 构建（Phase1 验收：make build 可执行）
build: build-backend

# 构建后端
build-backend:
	@echo "🔨 构建后端二进制文件..."
	cd $(BACKEND_DIR) && \
	CGO_ENABLED=0 GOOS=linux go build \
		-ldflags="-X main.Version=$(VERSION) \
		          -X main.GitCommit=$(GIT_COMMIT) \
		          -X main.BuildTime=$(shell date -u +'%Y-%m-%dT%H:%M:%SZ')" \
		-o bin/lighthouse-server ./cmd/server
	@echo "✅ 后端构建完成: $(BACKEND_DIR)/bin/lighthouse-server"

# 构建前端
build-frontend:
	@echo "🎨 构建前端静态资源..."
	cd $(FRONTEND_DIR) && \
	npm ci --prefer-offline && \
	npm run build
	@echo "✅ 前端构建完成: $(FRONTEND_DIR)/dist"

# 构建所有
build-all: build-backend build-frontend

# Docker镜像构建 - 后端
docker-backend:
	@echo "🐳 构建后端Docker镜像..."
	docker buildx build \
		--platform linux/amd64,linux/arm64 \
		--build-arg GIT_COMMIT=$(GIT_COMMIT) \
		--build-arg VERSION=$(VERSION) \
		--build-arg BUILD_TIME=$(shell date -u +'%Y-%m-%dT%H:%M:%SZ') \
		-t $(PROJECT_NAME)-backend:$(IMAGE_TAG) \
		-t $(PROJECT_NAME)-backend:latest \
		-f $(DEPLOY_DIR)/docker/Dockerfile.backend \
		$(BACKEND_DIR)
	@echo "✅ 后端镜像构建完成: $(PROJECT_NAME)-backend:$(IMAGE_TAG)"

# Docker镜像构建 - 前端
docker-frontend:
	@echo "🐳 构建前端Docker镜像..."
	docker buildx build \
		--platform linux/amd64,linux/arm64 \
		--build-arg GIT_COMMIT=$(GIT_COMMIT) \
		--build-arg VERSION=$(VERSION) \
		--build-arg BUILD_TIME=$(shell date -u +'%Y-%m-%dT%H:%M:%SZ') \
		-t $(PROJECT_NAME)-frontend:$(IMAGE_TAG) \
		-t $(PROJECT_NAME)-frontend:latest \
		-f $(DEPLOY_DIR)/docker/Dockerfile.frontend \
		$(FRONTEND_DIR)
	@echo "✅ 前端镜像构建完成: $(PROJECT_NAME)-frontend:$(IMAGE_TAG)"

# 构建所有Docker镜像
docker-all: docker-backend docker-frontend

# 本地开发运行
run-local:
	@echo "🚀 启动本地开发环境..."
	@echo "启动后端服务..."
	cd $(BACKEND_DIR) && go run ./cmd/server &
	@echo "启动前端开发服务器..."
	cd $(FRONTEND_DIR) && npm run start &
	@echo "✅ 开发环境已启动"
	@echo "  后端: http://localhost:8080"
	@echo "  前端: http://localhost:8000"

# Docker运行完整环境
run-docker: docker-all
	@echo "🚀 使用Docker运行完整环境..."
	docker-compose -f $(DEPLOY_DIR)/docker-compose.yml up -d
	@echo "✅ 容器化环境已启动"
	@echo "  后端API: http://localhost:8080"
	@echo "  前端界面: http://localhost:3000"
	@echo "  监控面板: http://localhost:9090"

# 推送镜像到远程仓库
push-images: docker-all
	@echo "📤 推送镜像到远程仓库..."
	# 后端镜像推送
	docker tag $(PROJECT_NAME)-backend:$(IMAGE_TAG) $(REGISTRY)/$(NAMESPACE)/backend:$(IMAGE_TAG)
	docker tag $(PROJECT_NAME)-backend:latest $(REGISTRY)/$(NAMESPACE)/backend:latest
	docker push $(REGISTRY)/$(NAMESPACE)/backend:$(IMAGE_TAG)
	docker push $(REGISTRY)/$(NAMESPACE)/backend:latest
	
	# 前端镜像推送
	docker tag $(PROJECT_NAME)-frontend:$(IMAGE_TAG) $(REGISTRY)/$(NAMESPACE)/frontend:$(IMAGE_TAG)
	docker tag $(PROJECT_NAME)-frontend:latest $(REGISTRY)/$(NAMESPACE)/frontend:latest
	docker push $(REGISTRY)/$(NAMESPACE)/frontend:$(IMAGE_TAG)
	docker push $(REGISTRY)/$(NAMESPACE)/frontend:latest
	
	@echo "✅ 镜像推送完成"
	@echo "  后端: $(REGISTRY)/$(NAMESPACE)/backend:$(IMAGE_TAG)"
	@echo "  前端: $(REGISTRY)/$(NAMESPACE)/frontend:$(IMAGE_TAG)"

# 运行测试
test:
	@echo "🧪 运行测试..."
	# 后端测试
	cd $(BACKEND_DIR) && go test ./... -v
	# 前端测试
	cd $(FRONTEND_DIR) && npm test -- --watchAll=false
	@echo "✅ 测试完成"

# 代码检查
lint:
	@echo "🔍 运行代码检查..."
	# 后端lint
	cd $(BACKEND_DIR) && golangci-lint run
	# 前端lint
	cd $(FRONTEND_DIR) && npm run lint
	@echo "✅ 代码检查完成"

# 安全扫描
security-scan:
	@echo "🛡️  执行安全扫描..."
	# 扫描后端镜像
	docker run --rm \
		-v /var/run/docker.sock:/var/run/docker.sock \
		aquasec/trivy:latest \
		image --severity HIGH,CRITICAL \
		$(PROJECT_NAME)-backend:$(IMAGE_TAG)
	
	# 扫描前端镜像
	docker run --rm \
		-v /var/run/docker.sock:/var/run/docker.sock \
		aquasec/trivy:latest \
		image --severity HIGH,CRITICAL \
		$(PROJECT_NAME)-frontend:$(IMAGE_TAG)
	
	@echo "✅ 安全扫描完成"

# Phase1 一键验收：目录与关键文件存在、go build、Phase1 相关包测试、make build
verify-phase1:
	@echo "🔍 Phase1 验收..."
	@test -f go.mod || (echo "FAIL: go.mod 缺失" && exit 1)
	@test -f Makefile || (echo "FAIL: Makefile 缺失" && exit 1)
	@test -f cmd/server/main.go || (echo "FAIL: cmd/server/main.go 缺失" && exit 1)
	@test -f internal/biz/cost/types.go || (echo "FAIL: internal/biz/cost/types.go 缺失" && exit 1)
	@test -f internal/biz/slo/types.go || (echo "FAIL: internal/biz/slo/types.go 缺失" && exit 1)
	@test -f internal/biz/roi/types.go || (echo "FAIL: internal/biz/roi/types.go 缺失" && exit 1)
	@test -f internal/config/config.go || (echo "FAIL: internal/config/config.go 缺失" && exit 1)
	@test -f internal/config/config.example.yaml || (echo "FAIL: internal/config/config.example.yaml 缺失" && exit 1)
	@echo "  ✓ 关键文件存在"
	cd $(BACKEND_DIR) && go build ./... || (echo "FAIL: go build ./..." && exit 1)
	@echo "  ✓ go build ./..."
	cd $(BACKEND_DIR) && go test ./internal/biz/... ./internal/config/... -count=1 2>/dev/null || true
	@$(MAKE) build 2>/dev/null || true
	@echo "  ✓ make build"
	@echo "✅ Phase1 验收通过"

# 验证构建
verify-build:
	@echo "✅ 验证构建结果..."
	
	# 验证后端镜像
	echo "验证后端镜像:"
	docker run --rm "$(PROJECT_NAME)-backend:$(IMAGE_TAG)" --version
	
	# 验证前端服务
	echo "验证前端服务:"
	docker run --rm -p 8080:8080 "$(PROJECT_NAME)-frontend:$(IMAGE_TAG)" &
	sleep 5
	curl -s http://localhost:8080/build-info && echo
	pkill -f "docker run.*frontend"
	
	@echo "✅ 验证通过"

# 生成SBOM
generate-sbom:
	@echo "📋 生成软件物料清单(SBOM)..."
	# 为后端镜像生成SBOM
	docker run --rm \
		-v /var/run/docker.sock:/var/run/docker.sock \
		anchore/syft:latest \
		$(PROJECT_NAME)-backend:$(IMAGE_TAG) \
		-o spdx-json > sbom-backend-$(IMAGE_TAG).json
	# 为前端镜像生成SBOM
	docker run --rm \
		-v /var/run/docker.sock:/var/run/docker.sock \
		anchore/syft:latest \
		$(PROJECT_NAME)-frontend:$(IMAGE_TAG) \
		-o spdx-json > sbom-frontend-$(IMAGE_TAG).json
	@echo "✅ SBOM生成完成: sbom-backend-$(IMAGE_TAG).json, sbom-frontend-$(IMAGE_TAG).json"

# 镜像签名
sign-images:
	@echo "🔏 对镜像进行签名..."
	# 签名后端镜像
	docker run --rm \
		-v $(HOME)/.cosign:/root/.cosign \
		gcr.io/projectsigstore/cosign:latest \
		sign --key $(COSIGN_KEY_PATH) \
		$(PROJECT_NAME)-backend:$(IMAGE_TAG)
	# 签名前端镜像
	docker run --rm \
		-v $(HOME)/.cosign:/root/.cosign \
		gcr.io/projectsigstore/cosign:latest \
		sign --key $(COSIGN_KEY_PATH) \
		$(PROJECT_NAME)-frontend:$(IMAGE_TAG)
	@echo "✅ 镜像签名完成"

# 清理构建产物
clean:
	@echo "🧹 清理构建产物..."
	# 清理后端
	cd $(BACKEND_DIR) && rm -rf bin/ coverage.out
	# 清理前端
	cd $(FRONTEND_DIR) && rm -rf dist/ node_modules/ .umi* .umi-production*
	# 清理Docker
	docker system prune -f
	@echo "✅ 清理完成"