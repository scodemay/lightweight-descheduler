# 快速开始指南

本指南将帮助您在 5 分钟内快速部署和使用轻量级重调度器。

## 🎯 前提条件

在开始之前，请确保您有：

- 一个运行中的 Kubernetes 集群（版本 1.20+）
- `kubectl` 命令行工具已配置
- 对集群的管理员权限（用于创建 RBAC 资源）

## 🚀 快速部署

### 方法一：使用预构建的配置文件

1. **克隆项目**
   ```bash
   git clone https://github.com/your-org/lightweight-descheduler.git
   cd lightweight-descheduler
   ```

2. **部署到集群**
   ```bash
   # 部署 RBAC 权限
   kubectl apply -f deploy/rbac.yaml
   
   # 部署配置文件
   kubectl apply -f deploy/configmap.yaml
   
   # 部署重调度器 (选择其中一种方式)
   # 方式A: 持续运行模式
   kubectl apply -f deploy/deployment.yaml
   
   # 方式B: 定时任务模式
   # kubectl apply -f deploy/cronjob.yaml
   ```

3. **验证部署**
   ```bash
   # 检查 Pod 状态
   kubectl get pods -n kube-system -l app=lightweight-descheduler
   
   # 查看日志
   kubectl logs -n kube-system -l app=lightweight-descheduler
   ```

### 方法二：使用 Makefile

如果您有构建环境，可以使用我们提供的 Makefile：

```bash
# 构建和部署
make deploy

# 查看状态
make status

# 查看日志
make logs
```

## 📋 验证安装

部署成功后，您应该看到类似的输出：

```bash
$ kubectl get pods -n kube-system -l app=lightweight-descheduler
NAME                                     READY   STATUS    RESTARTS   AGE
lightweight-descheduler-xxxxxxxxx-xxxxx   1/1     Running   0          2m
```

查看日志确认重调度器正常运行：

```bash
$ kubectl logs -n kube-system -l app=lightweight-descheduler --tail=20
I0101 12:00:00.000000       1 main.go:XX] Starting lightweight-descheduler v1.0.0
I0101 12:00:00.000000       1 main.go:XX] Configuration loaded successfully
I0101 12:00:00.000000       1 scheduler.go:XX] Created scheduler with 2 enabled strategies
I0101 12:00:00.000000       1 scheduler.go:XX]   - RemoveFailedPods
I0101 12:00:00.000000       1 scheduler.go:XX]   - LowNodeUtilization
I0101 12:00:00.000000       1 scheduler.go:XX] === Starting descheduling cycle ===
```

## 🎛️ 基本配置

默认配置文件位于 `deploy/configmap.yaml`，包含以下主要设置：

```yaml
# 基本配置
interval: "5m"          # 每5分钟运行一次
dryRun: false           # 实际驱逐Pod（设为true进行模拟）
logLevel: "info"        # 日志级别

# 驱逐限制
limits:
  maxPodsToEvictPerNode: 5        # 每节点最多驱逐5个Pod
  maxPodsToEvictPerNamespace: 3   # 每命名空间最多驱逐3个Pod
  maxPodsToEvictTotal: 20         # 每次最多驱逐20个Pod

# 启用的策略
strategies:
  removeFailedPods:
    enabled: true                 # 清理失败的Pod
  lowNodeUtilization:
    enabled: true                 # 平衡节点资源利用率
  removeDuplicates:
    enabled: false                # 清理重复Pod（默认关闭）
```

## 🧪 测试功能

### 1. DryRun 模式测试

首先在 DryRun 模式下测试，确保不会意外驱逐 Pod：

```bash
# 修改配置启用 DryRun
kubectl patch configmap lightweight-descheduler-config -n kube-system --type merge -p '{"data":{"config.yaml":"interval: \"5m\"\ndryRun: true\n..."}}'

# 重启 Pod 应用新配置
kubectl rollout restart deployment/lightweight-descheduler -n kube-system

# 观察日志
kubectl logs -n kube-system -l app=lightweight-descheduler -f
```

在 DryRun 模式下，您会看到类似的日志：
```
[DryRun] Would evict pod default/my-app-xxx on node worker-1, reason: Failed pod cleanup
```

### 2. 创建测试场景

创建一些失败的 Pod 来测试清理功能：

```bash
# 创建一个会失败的 Pod
kubectl run failing-pod --image=busybox --restart=Never -- /bin/sh -c "exit 1"

# 等待 Pod 失败
kubectl wait --for=condition=Ready pod/failing-pod --timeout=30s || true

# 检查 Pod 状态
kubectl get pod failing-pod
```

### 3. 观察重调度行为

```bash
# 实时观察重调度器日志
kubectl logs -n kube-system -l app=lightweight-descheduler -f

# 监控 Pod 变化
kubectl get pods --all-namespaces --watch
```

## 📊 监控和指标

重调度器提供详细的统计信息：

```bash
# 查看详细日志了解执行统计
kubectl logs -n kube-system -l app=lightweight-descheduler --tail=50 | grep "Statistics"
```

您会看到类似的统计信息：
```
=== Cycle Statistics ===
Duration: 2.345s
Total evicted: 3
Failed evictions: 0
Evictions by node:
  worker-1: 2
  worker-2: 1
Evictions by reason:
  Failed pod cleanup: 3
```

## 🔧 常见配置调整

### 调整运行频率

```bash
# 修改为每10分钟运行一次
kubectl patch configmap lightweight-descheduler-config -n kube-system --type merge -p '{"data":{"config.yaml":"interval: \"10m\"\n..."}}'
```

### 调整驱逐限制

```bash
# 减少驱逐限制以更保守
kubectl patch configmap lightweight-descheduler-config -n kube-system --type json -p='[{"op": "replace", "path": "/data/config.yaml", "value": "limits:\n  maxPodsToEvictPerNode: 2\n  maxPodsToEvictTotal: 10\n..."}]'
```

### 启用/禁用策略

```bash
# 启用重复Pod清理策略
kubectl patch configmap lightweight-descheduler-config -n kube-system --type json -p='[{"op": "replace", "path": "/data/config.yaml", "value": "strategies:\n  removeDuplicates:\n    enabled: true\n..."}]'
```

## 🚨 安全注意事项

1. **首次部署建议使用 DryRun 模式**，观察重调度器的行为
2. **从保守的限制开始**，逐步调整到合适的值
3. **监控应用服务**，确保重调度不影响业务
4. **备份重要数据**，虽然重调度器只驱逐 Pod，但建议做好准备

## 🔍 故障排除

### Pod 不启动

```bash
# 检查 RBAC 权限
kubectl auth can-i --list --as=system:serviceaccount:kube-system:lightweight-descheduler

# 检查镜像拉取
kubectl describe pod -n kube-system -l app=lightweight-descheduler
```

### 配置不生效

```bash
# 重启 Pod 应用新配置
kubectl rollout restart deployment/lightweight-descheduler -n kube-system

# 检查配置文件格式
kubectl get configmap lightweight-descheduler-config -n kube-system -o yaml
```

### 权限错误

```bash
# 检查 ServiceAccount 和权限绑定
kubectl get serviceaccount lightweight-descheduler -n kube-system
kubectl get clusterrolebinding lightweight-descheduler
```

## 📚 下一步

现在您已经成功部署了轻量级重调度器！接下来可以：

1. 阅读 [配置指南](./configuration.md) 了解详细配置选项
2. 查看 [策略详解](./strategies.md) 理解各种策略的工作原理
3. 参考 [部署指南](./deployment.md) 了解生产环境部署最佳实践

## 🆘 获取帮助

如果遇到问题，请：

1. 查看 [故障排除指南](./troubleshooting.md)
2. 在 GitHub 上 [提交 Issue](https://github.com/your-org/lightweight-descheduler/issues)
3. 查看项目 [Wiki](https://github.com/your-org/lightweight-descheduler/wiki)

---

**恭喜！** 您已经成功部署了轻量级重调度器。现在可以享受自动化的 Pod 重调度带来的便利了！
