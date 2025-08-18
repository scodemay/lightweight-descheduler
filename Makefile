# 构建配置
APP_NAME := lightweight-descheduler
VERSION := v1.0.0
REGISTRY := chenyuma725
IMAGE := $(REGISTRY)/$(APP_NAME):$(VERSION)
LATEST_IMAGE := $(REGISTRY)/$(APP_NAME):latest

# Go配置
GOOS := linux
GOARCH := amd64
CGO_ENABLED := 0

# 默认目标
.PHONY: all
all: build

# 构建二进制文件
.PHONY: build
build:
	@echo "Building $(APP_NAME)..."
	CGO_ENABLED=$(CGO_ENABLED) GOOS=$(GOOS) GOARCH=$(GOARCH) go build \
		-ldflags='-w -s -extldflags "-static"' \
		-o bin/$(APP_NAME) \
		./cmd/main.go
	@echo "Build complete: bin/$(APP_NAME)"

# 清理构建文件
.PHONY: clean
clean:
	@echo "Cleaning up..."
	rm -rf bin/
	@echo "Clean complete"

# 运行测试
.PHONY: test
test:
	@echo "Running tests..."
	go test -v ./...

# 格式化代码
.PHONY: fmt
fmt:
	@echo "Formatting code..."
	go fmt ./...

# 代码检查
.PHONY: vet
vet:
	@echo "Running go vet..."
	go vet ./...

# 构建Docker镜像
.PHONY: docker-build
docker-build:
	@echo "Building Docker image $(IMAGE)..."
	docker build -t $(IMAGE) -t $(LATEST_IMAGE) .
	@echo "Docker build complete"

# 推送Docker镜像
.PHONY: docker-push
docker-push: docker-build
	@echo "Pushing Docker image $(IMAGE)..."
	docker push $(IMAGE)
	docker push $(LATEST_IMAGE)
	@echo "Docker push complete"

# 本地运行（需要配置文件）
.PHONY: run
run: build
	@echo "Running $(APP_NAME) locally..."
	./bin/$(APP_NAME) -config configs/config.yaml -log-level 3

# 生成部署文件
.PHONY: generate-manifests
generate-manifests:
	@echo "Generating Kubernetes manifests..."
	@mkdir -p generated/
	@# 替换镜像名称
	@sed 's|lightweight-descheduler:v1.0.0|$(IMAGE)|g' deploy/deployment.yaml > generated/deployment.yaml
	@sed 's|lightweight-descheduler:v1.0.0|$(IMAGE)|g' deploy/cronjob.yaml > generated/cronjob.yaml
	@cp deploy/rbac.yaml generated/
	@cp deploy/configmap.yaml generated/
	@echo "Manifests generated in generated/"

# 部署到Kubernetes (Deployment模式)
.PHONY: deploy
deploy: generate-manifests
	@echo "Deploying to Kubernetes (Deployment mode)..."
	kubectl apply -f generated/rbac.yaml
	kubectl apply -f generated/configmap.yaml
	kubectl apply -f generated/deployment.yaml
	@echo "Deployment complete"

# 部署到Kubernetes (CronJob模式)
.PHONY: deploy-cronjob
deploy-cronjob: generate-manifests
	@echo "Deploying to Kubernetes (CronJob mode)..."
	kubectl apply -f generated/rbac.yaml
	kubectl apply -f generated/configmap.yaml
	kubectl apply -f generated/cronjob.yaml
	@echo "CronJob deployment complete"

# 从Kubernetes卸载
.PHONY: undeploy
undeploy:
	@echo "Removing from Kubernetes..."
	kubectl delete -f deploy/ --ignore-not-found=true
	kubectl delete -f generated/ --ignore-not-found=true
	@echo "Undeploy complete"

# 查看Pod日志
.PHONY: logs
logs:
	@echo "Showing logs..."
	kubectl logs -n kube-system -l app=$(APP_NAME) --tail=100 -f

# 查看Pod状态
.PHONY: status
status:
	@echo "Checking status..."
	kubectl get pods -n kube-system -l app=$(APP_NAME)
	kubectl get configmap -n kube-system lightweight-descheduler-config

# 开发环境设置
.PHONY: dev-setup
dev-setup:
	@echo "Setting up development environment..."
	go mod download
	go mod tidy
	@echo "Development setup complete"

# 完整的CI/CD流水线
.PHONY: ci
ci: fmt vet test build docker-build

# 帮助信息
.PHONY: help
help:
	@echo "轻量级重调度器 Makefile"
	@echo ""
	@echo "可用目标:"
	@echo "  build               构建二进制文件"
	@echo "  clean               清理构建文件"
	@echo "  test                运行测试"
	@echo "  fmt                 格式化代码"
	@echo "  vet                 代码检查"
	@echo "  docker-build        构建Docker镜像"
	@echo "  docker-push         推送Docker镜像"
	@echo "  run                 本地运行"
	@echo "  generate-manifests  生成Kubernetes部署文件"
	@echo "  deploy              部署到Kubernetes (Deployment)"
	@echo "  deploy-cronjob      部署到Kubernetes (CronJob)"
	@echo "  undeploy            从Kubernetes卸载"
	@echo "  logs                查看Pod日志"
	@echo "  status              查看Pod状态"
	@echo "  dev-setup           开发环境设置"
	@echo "  ci                  CI流水线 (fmt + vet + test + build + docker-build)"
	@echo "  help                显示此帮助信息"
	@echo ""
	@echo "变量:"
	@echo "  VERSION=$(VERSION)"
	@echo "  REGISTRY=$(REGISTRY)"
	@echo "  IMAGE=$(IMAGE)"
