#!/bin/bash

# 轻量级重调度器测试脚本
# 此脚本帮助您快速测试重调度器的各种功能

set -e

# 颜色定义
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# 打印带颜色的消息
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

# 检查依赖
check_dependencies() {
    print_info "检查依赖..."
    
    if ! command -v kubectl &> /dev/null; then
        print_error "kubectl 未找到，请先安装 kubectl"
        exit 1
    fi
    
    if ! kubectl cluster-info &> /dev/null; then
        print_error "无法连接到 Kubernetes 集群"
        exit 1
    fi
    
    print_success "依赖检查通过"
}

# 检查重调度器状态
check_descheduler() {
    print_info "检查重调度器状态..."
    
    if kubectl get deployment lightweight-descheduler -n kube-system &> /dev/null; then
        local status=$(kubectl get deployment lightweight-descheduler -n kube-system -o jsonpath='{.status.readyReplicas}')
        if [[ "$status" == "1" ]]; then
            print_success "重调度器运行正常"
        else
            print_warning "重调度器可能未正常运行，请检查日志"
        fi
    else
        print_error "重调度器未部署，请先部署重调度器"
        exit 1
    fi
}

# 启用DryRun模式
enable_dryrun() {
    print_info "启用 DryRun 模式进行安全测试..."
    
    kubectl patch configmap lightweight-descheduler-config -n kube-system --type merge -p '{
        "data": {
            "config.yaml": "interval: \"2m\"\ndryRun: true\nlogLevel: \"debug\"\nlimits:\n  maxPodsToEvictPerNode: 5\n  maxPodsToEvictPerNamespace: 3\n  maxPodsToEvictTotal: 20\nstrategies:\n  removeFailedPods:\n    enabled: true\n    minPodLifetimeSeconds: 60\n    excludedNamespaces:\n      - \"kube-system\"\n  lowNodeUtilization:\n    enabled: true\n    numberOfNodes: 1\n    thresholds:\n      cpu: 20\n      memory: 20\n      pods: 20\n    targetThresholds:\n      cpu: 80\n      memory: 80\n      pods: 80"
        }
    }' > /dev/null
    
    # 重启重调度器应用新配置
    kubectl rollout restart deployment/lightweight-descheduler -n kube-system > /dev/null
    kubectl rollout status deployment/lightweight-descheduler -n kube-system > /dev/null
    
    print_success "DryRun 模式已启用"
}

# 创建测试场景
create_test_scenarios() {
    print_info "创建测试场景..."
    
    # 创建失败的Pod
    print_info "创建失败的Pod..."
    kubectl apply -f - > /dev/null <<EOF
apiVersion: v1
kind: Pod
metadata:
  name: failing-pod-test
  namespace: default
  labels:
    test-scenario: "failed-pod"
spec:
  restartPolicy: Never
  containers:
  - name: failing-container
    image: busybox
    command: ["/bin/sh"]
    args: ["-c", "echo 'This pod will fail'; exit 1"]
EOF

    # 等待Pod失败
    sleep 10
    
    # 创建资源密集型应用
    print_info "创建资源密集型应用..."
    kubectl apply -f - > /dev/null <<EOF
apiVersion: apps/v1
kind: Deployment
metadata:
  name: resource-test-app
  namespace: default
  labels:
    test-scenario: "resource-utilization"
spec:
  replicas: 3
  selector:
    matchLabels:
      app: resource-test
  template:
    metadata:
      labels:
        app: resource-test
        test-scenario: "resource-utilization"
    spec:
      containers:
      - name: app
        image: nginx:alpine
        resources:
          requests:
            cpu: 200m
            memory: 256Mi
          limits:
            cpu: 500m
            memory: 512Mi
EOF

    print_success "测试场景创建完成"
}

# 监控重调度器日志
monitor_logs() {
    print_info "监控重调度器日志 (按 Ctrl+C 停止)..."
    echo "寻找包含 [DryRun] 的日志行..."
    echo "----------------------------------------"
    
    kubectl logs -n kube-system -l app=lightweight-descheduler -f --tail=20 | grep --line-buffered -E "(DryRun|Statistics|Executing|completed)"
}

# 显示集群状态
show_cluster_status() {
    print_info "显示集群状态..."
    
    echo "节点状态:"
    kubectl get nodes -o wide
    echo ""
    
    echo "测试 Pod 状态:"
    kubectl get pods -l test-scenario --all-namespaces -o wide
    echo ""
    
    echo "重调度器状态:"
    kubectl get pods -n kube-system -l app=lightweight-descheduler -o wide
}

# 清理测试资源
cleanup_test_resources() {
    print_info "清理测试资源..."
    
    kubectl delete pod failing-pod-test --ignore-not-found=true > /dev/null
    kubectl delete deployment resource-test-app --ignore-not-found=true > /dev/null
    kubectl delete pods -l test-scenario --all-namespaces --ignore-not-found=true > /dev/null
    
    print_success "测试资源清理完成"
}

# 恢复正常配置
restore_config() {
    print_info "恢复正常配置..."
    
    kubectl patch configmap lightweight-descheduler-config -n kube-system --type merge -p '{
        "data": {
            "config.yaml": "interval: \"5m\"\ndryRun: false\nlogLevel: \"info\"\nlimits:\n  maxPodsToEvictPerNode: 5\n  maxPodsToEvictPerNamespace: 3\n  maxPodsToEvictTotal: 20\nstrategies:\n  removeFailedPods:\n    enabled: true\n    minPodLifetimeSeconds: 300\n    excludedNamespaces:\n      - \"kube-system\"\n  lowNodeUtilization:\n    enabled: true\n    numberOfNodes: 1\n    thresholds:\n      cpu: 20\n      memory: 20\n      pods: 20\n    targetThresholds:\n      cpu: 80\n      memory: 80\n      pods: 80"
        }
    }' > /dev/null
    
    kubectl rollout restart deployment/lightweight-descheduler -n kube-system > /dev/null
    
    print_success "配置已恢复"
}

# 显示帮助信息
show_help() {
    echo "轻量级重调度器测试脚本"
    echo ""
    echo "用法: $0 [选项]"
    echo ""
    echo "选项:"
    echo "  test       - 运行完整测试 (推荐)"
    echo "  check      - 检查重调度器状态"
    echo "  dryrun     - 启用 DryRun 模式"
    echo "  create     - 创建测试场景"
    echo "  monitor    - 监控日志"
    echo "  status     - 显示集群状态"
    echo "  cleanup    - 清理测试资源"
    echo "  restore    - 恢复正常配置"
    echo "  help       - 显示此帮助信息"
    echo ""
    echo "示例:"
    echo "  $0 test      # 运行完整测试流程"
    echo "  $0 monitor   # 只监控日志"
}

# 运行完整测试
run_full_test() {
    print_info "开始完整测试流程..."
    echo ""
    
    # 1. 检查依赖和状态
    check_dependencies
    check_descheduler
    echo ""
    
    # 2. 启用DryRun模式
    enable_dryrun
    echo ""
    
    # 3. 创建测试场景
    create_test_scenarios
    echo ""
    
    # 4. 显示初始状态
    show_cluster_status
    echo ""
    
    # 5. 等待重调度器运行
    print_info "等待重调度器运行 (2分钟)..."
    sleep 120
    
    # 6. 显示日志
    print_info "显示最近的日志..."
    kubectl logs -n kube-system -l app=lightweight-descheduler --tail=30 | grep -E "(DryRun|Statistics|Executing|completed)" || true
    echo ""
    
    # 7. 询问是否继续监控
    read -p "是否要继续监控实时日志？(y/n): " -n 1 -r
    echo ""
    if [[ $REPLY =~ ^[Yy]$ ]]; then
        monitor_logs
    fi
    
    # 8. 清理和恢复
    print_info "测试完成，正在清理..."
    cleanup_test_resources
    restore_config
    
    print_success "完整测试流程完成！"
}

# 主逻辑
case "${1:-}" in
    "test")
        run_full_test
        ;;
    "check")
        check_dependencies
        check_descheduler
        ;;
    "dryrun")
        enable_dryrun
        ;;
    "create")
        create_test_scenarios
        ;;
    "monitor")
        monitor_logs
        ;;
    "status")
        show_cluster_status
        ;;
    "cleanup")
        cleanup_test_resources
        ;;
    "restore")
        restore_config
        ;;
    "help"|"-h"|"--help")
        show_help
        ;;
    "")
        print_warning "未指定操作，显示帮助信息:"
        echo ""
        show_help
        ;;
    *)
        print_error "未知选项: $1"
        echo ""
        show_help
        exit 1
        ;;
esac
