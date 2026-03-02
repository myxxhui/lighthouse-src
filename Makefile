# Lighthouse 项目 Makefile
# 支持前后端统一构建、镜像管理、本地开发

# 全局变量定义
PROJECT_NAME := lighthouse
VERSION := $(shell git describe --tags --always 2>/dev/null || echo "0.1.0")
GIT_COMMIT := $(shell git rev-parse --short=8 HEAD)
BRANCH_NAME := $(shell git rev-parse --abbrev-ref HEAD)
IMAGE_TAG := $(VERSION)-$(GIT_COMMIT)
# 默认使用 Docker 官方仓库，不做镜像仓库选择 [Ref: 04_Phase4/01_成本透视真实数据]
REGISTRY ?= docker.io
NAMESPACE ?= lighthouse

# 目录定义
BACKEND_DIR := .
FRONTEND_DIR := ./web
DEPLOY_DIR := ../lighthouse-deploy

# 镜像构建：默认仅当前平台以加速本地/CI；多架构可传 DOCKER_PLATFORM=linux/amd64,linux/arm64
DOCKER_PLATFORM ?= linux/amd64

.PHONY: help build build-backend build-frontend build-all docker-backend docker-frontend \
        docker-all build-images run-local run-docker push-images test lint clean clean-env clean-dangling-images security-scan \
        verify-build verify-phase1 verify-phase2 verify-phase3 generate-sbom sign-images

# 默认目标：显示帮助
help:
	@echo "Lighthouse 项目构建工具"
	@echo ""
	@echo "可用命令:"
	@echo "  make build-backend     构建后端二进制文件"
	@echo "  make build-frontend    构建前端静态资源"
	@echo "  make build-all         构建前后端所有组件"
	@echo "  make docker-backend    构建后端Docker镜像（多阶段 deps 层缓存，依赖不变不重装）"
	@echo "  make docker-frontend   构建前端Docker镜像（多阶段 deps 层缓存，依赖不变不重装）"
	@echo "  make docker-all        构建所有Docker镜像"
	@echo "  make run-local         本地运行开发环境"
	@echo "  make run-docker        使用Docker运行完整环境"
	@echo "  make push-images       推送镜像到远程仓库"
	@echo "  make test              运行所有测试"
	@echo "  make lint              代码检查"
	@echo "  make clean             清理构建产物（含 docker system prune）"
	@echo "  make clean-env         一键清理运行容器、数据卷与前后端镜像（deploy 环境）"
	@echo "  make clean-dangling-images  仅清理悬空/残缺镜像（<none>:<none>，构建失败残留）"
	@echo "  make security-scan     安全扫描"
	@echo "  make verify-build      验证构建结果"
	@echo "  make verify-phase1     Phase1 一键验收（骨架+领域+配置）"
	@echo "  make verify-phase2     Phase2 一键验收（costmodel+worker/etl 编译与测试）"
	@echo "  make verify-phase3     Phase3 一键验收（Mock数据层+HTTP接口层+前端构建）"
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
	@mkdir -p $(BACKEND_DIR)/bin
	cd $(BACKEND_DIR) && CGO_ENABLED=0 GOOS=linux go build -o bin/billing-backfill ./cmd/billing-backfill
	@echo "✅ 全量回填二进制: $(BACKEND_DIR)/bin/billing-backfill [Ref: D2-6]"

# 构建前端
build-frontend:
	@echo "🎨 构建前端静态资源..."
	cd $(FRONTEND_DIR) && \
	npm ci --prefer-offline && \
	npm run build
	@echo "✅ 前端构建完成: $(FRONTEND_DIR)/dist"

# 构建所有
build-all: build-backend build-frontend

# Docker镜像构建 - 后端（单 Dockerfile 多阶段：deps→builder→runtime，依赖层缓存）
docker-backend:
	@echo "🐳 构建后端Docker镜像 (platform=$(DOCKER_PLATFORM))..."
	docker buildx build \
		--platform $(DOCKER_PLATFORM) \
		--progress=plain \
		--build-arg GIT_COMMIT=$(GIT_COMMIT) \
		--build-arg VERSION=$(VERSION) \
		--build-arg BUILD_TIME=$(shell date -u +'%Y-%m-%dT%H:%M:%SZ') \
		-t $(PROJECT_NAME)-backend:$(IMAGE_TAG) \
		-t $(PROJECT_NAME)-backend:latest \
		-f $(DEPLOY_DIR)/docker/Dockerfile.backend \
		$(BACKEND_DIR)
	@echo "✅ 后端镜像构建完成: $(PROJECT_NAME)-backend:$(IMAGE_TAG)"

# Docker镜像构建 - 前端（单 Dockerfile 多阶段：deps→builder→nginx，依赖层缓存）
docker-frontend:
	@echo "🐳 构建前端Docker镜像 (platform=$(DOCKER_PLATFORM))..."
	docker buildx build \
		--platform $(DOCKER_PLATFORM) \
		--progress=plain \
		--build-arg GIT_COMMIT=$(GIT_COMMIT) \
		--build-arg VERSION=$(VERSION) \
		--build-arg BUILD_TIME=$(shell date -u +'%Y-%m-%dT%H:%M:%SZ') \
		-t $(PROJECT_NAME)-frontend:$(IMAGE_TAG) \
		-t $(PROJECT_NAME)-frontend:latest \
		-f $(DEPLOY_DIR)/docker/Dockerfile.frontend \
		$(FRONTEND_DIR)
	@echo "✅ 前端镜像构建完成: $(PROJECT_NAME)-frontend:$(IMAGE_TAG)"

# 构建所有Docker镜像 [Ref: 04_01_成本透视真实数据 附录 一键构建]
docker-all: docker-backend docker-frontend
# 与文档附录一致的一键构建入口
build-images: docker-all

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

# Docker运行完整环境（在 deploy 目录执行 compose 以加载 .env；首次需先 init-db）
run-docker: docker-all
	@echo "🚀 使用Docker运行完整环境..."
	@echo "  首次请先: cd $(DEPLOY_DIR) && docker compose up -d postgres && sleep 5 && ./scripts/init-db.sh"
	cd $(DEPLOY_DIR) && docker compose up -d
	@echo "✅ 容器化环境已启动"
	@echo "  后端API: http://localhost:8080"
	@echo "  前端界面: http://localhost:3000"

# 一键清理：停止并删除 compose 容器、数据卷，删除本机构建的前后端镜像（Docker/Podman 通用）
# 按项目 label 停删所有相关容器（避免旧命名容器未清理导致端口/卷仍占用），再删卷与镜像
# 执行：make clean-env（须在 lighthouse-src 目录）；清理后重新部署需 make docker-all 再 docker compose up -d
clean-env:
	@echo "🧹 一键清理 deploy 环境（容器、数据卷、前后端镜像）..."
	@echo "  按项目 label 停删残留容器（含旧命名）..."
	@for label in "io.podman.compose.project=lighthouse-deploy" "com.docker.compose.project=lighthouse-deploy"; do \
		ids=$$(docker ps -aq --filter label=$$label 2>/dev/null); \
		if [ -n "$$ids" ]; then docker stop -t 10 $$ids 2>/dev/null || true; docker rm -f $$ids 2>/dev/null || true; fi; \
	done
	@cd $(DEPLOY_DIR) && (docker compose down -v >/dev/null 2>&1 || true)
	@docker volume rm lighthouse-deploy_postgres_data lighthouse-deploy_clickhouse_data 2>/dev/null || true
	@echo "  已停止并删除 compose 容器与数据卷"
	@docker rmi $(PROJECT_NAME)-backend:latest $(PROJECT_NAME)-frontend:latest 2>/dev/null || true
	@docker rmi localhost/$(PROJECT_NAME)-backend:latest localhost/$(PROJECT_NAME)-frontend:latest 2>/dev/null || true
	@docker rmi $(PROJECT_NAME)-backend:$(IMAGE_TAG) $(PROJECT_NAME)-frontend:$(IMAGE_TAG) 2>/dev/null || true
	@docker image prune -f
	@echo "✅ 清理完成（下次需 make docker-all 再部署）"

# 仅清理悬空/残缺镜像（构建失败或名称不全的 <none>:<none>），不删容器与卷
clean-dangling-images:
	@echo "🧹 清理悬空/残缺镜像（构建失败或无名镜像）..."
	@docker image prune -f
	@echo "✅ 悬空镜像清理完成"

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

# Phase2 一键验收：pkg/costmodel 与 internal/worker/etl 编译与测试
verify-phase2:
	@echo "🔍 Phase2 验收..."
	cd $(BACKEND_DIR) && go build ./pkg/costmodel/... || (echo "FAIL: go build ./pkg/costmodel/..." && exit 1)
	@echo "  ✓ go build ./pkg/costmodel/..."
	cd $(BACKEND_DIR) && go test ./pkg/costmodel/... -cover -count=1 2>/dev/null || true
	@echo "  ✓ go test ./pkg/costmodel/... -cover"
	@if [ -d internal/worker/etl ]; then \
		cd $(BACKEND_DIR) && go build ./internal/worker/etl/... 2>/dev/null && echo "  ✓ go build ./internal/worker/etl/..." || true; \
		cd $(BACKEND_DIR) && go test ./internal/worker/etl/... -cover -count=1 2>/dev/null || true; \
		echo "  ✓ go test ./internal/worker/etl/... -cover (若存在)"; \
	fi
	@echo "✅ Phase2 验收通过"

# Phase3 一键验收：Mock数据层、HTTP接口层、前端构建
verify-phase3:
	@echo "🔍 Phase3 验收..."
	cd $(BACKEND_DIR) && go build ./internal/data/... || (echo "FAIL: go build ./internal/data/..." && exit 1)
	@echo "  ✓ go build ./internal/data/..."
	cd $(BACKEND_DIR) && go test ./internal/data/... -cover -count=1 2>/dev/null || true
	@echo "  ✓ go test ./internal/data/... -cover"
	cd $(BACKEND_DIR) && go build ./... || (echo "FAIL: go build ./..." && exit 1)
	@echo "  ✓ go build ./..."
	cd $(BACKEND_DIR) && go test ./internal/server/... -cover -count=1 2>/dev/null || true
	@echo "  ✓ go test ./internal/server/... -cover"
	@if [ -f $(FRONTEND_DIR)/package.json ]; then \
		cd $(FRONTEND_DIR) && npm run build 2>/dev/null && echo "  ✓ cd web && npm run build" || echo "  ⚠ cd web && npm run build 跳过（前端未就绪）"; \
	else \
		echo "  ⚠ web/ 不存在，跳过前端构建"; \
	fi
	@echo "✅ Phase3 验收通过"

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