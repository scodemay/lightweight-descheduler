#!/bin/bash

# 轻量级重调度器构建脚本

set -e

# 配置
APP_NAME="lightweight-descheduler"
VERSION="v1.0.0"
REGISTRY="scodemay"
IMAGE="${REGISTRY}/${APP_NAME}:${VERSION}"
LATEST_IMAGE="${REGISTRY}/${APP_NAME}:latest"

# 颜色定义
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

print_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

print_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

print_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

print_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# 检查 Docker
check_docker() {
    print_info "检查 Docker..."
    if ! command -v docker &> /dev/null; then
        print_error "Docker 未安装"
        exit 1
    fi
    
    if ! docker info &> /dev/null; then
        print_error "Docker daemon 未运行，请先启动 Docker"
        print_info "macOS: 打开 Docker Desktop"
        print_info "Linux: sudo systemctl start docker"
        exit 1
    fi
    
    print_success "Docker 检查通过"
}

# 构建二进制文件
build_binary() {
    print_info "构建 Go 二进制文件..."
    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
        -ldflags='-w -s -extldflags "-static"' \
        -o bin/${APP_NAME} \
        ./cmd/main.go
    print_success "二进制文件构建完成"
}

# 构建 Docker 镜像
build_image() {
    print_info "构建 Docker 镜像: ${IMAGE}"
    docker build -t ${IMAGE} -t ${LATEST_IMAGE} .
    print_success "Docker 镜像构建完成"
}

# 推送镜像
push_image() {
    print_info "推送镜像到 Docker Hub..."
    
    # 检查是否已登录
    if ! docker info | grep -q "Username"; then
        print_warning "请先登录 Docker Hub"
        docker login
    fi
    
    docker push ${IMAGE}
    docker push ${LATEST_IMAGE}
    print_success "镜像推送完成"
}

# 更新部署文件
update_manifests() {
    print_info "更新部署文件..."
    
    # 创建 generated 目录
    mkdir -p generated/
    
    # 替换镜像名称
    sed "s|lightweight-descheduler:v1.0.0|${IMAGE}|g" deploy/deployment.yaml > generated/deployment.yaml
    sed "s|lightweight-descheduler:v1.0.0|${IMAGE}|g" deploy/cronjob.yaml > generated/cronjob.yaml
    cp deploy/rbac.yaml generated/
    cp deploy/configmap.yaml generated/
    
    print_success "部署文件已更新到 generated/ 目录"
}

# 主函数
main() {
    case "${1:-all}" in
        "check")
            check_docker
            ;;
        "build")
            check_docker
            build_binary
            build_image
            ;;
        "push")
            check_docker
            push_image
            ;;
        "manifests")
            update_manifests
            ;;
        "all")
            check_docker
            build_binary
            build_image
            push_image
            update_manifests
            ;;
        "help")
            echo "用法: $0 [选项]"
            echo "选项:"
            echo "  check     - 检查 Docker 环境"
            echo "  build     - 构建镜像"
            echo "  push      - 推送镜像"
            echo "  manifests - 更新部署文件"
            echo "  all       - 执行所有步骤 (默认)"
            echo "  help      - 显示帮助"
            ;;
        *)
            print_error "未知选项: $1"
            echo "使用 '$0 help' 查看帮助"
            exit 1
            ;;
    esac
}

main "$@"
