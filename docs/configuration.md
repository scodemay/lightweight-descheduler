# 配置指南

本指南详细介绍轻量级重调度器的所有配置选项，帮助您根据实际需求定制重调度行为。

## 📁 配置文件结构

配置文件使用 YAML 格式，主要包含以下几个部分：

```yaml
# 基本配置
interval: "5m"
dryRun: false  
logLevel: "info"

# 节点选择器（可选）
nodeSelector:
  key: value

# 驱逐限制
limits:
  maxPodsToEvictPerNode: 5
  maxPodsToEvictPerNamespace: 3
  maxPodsToEvictTotal: 20

# 策略配置
strategies:
  removeFailedPods: {...}
  lowNodeUtilization: {...}
  removeDuplicates: {...}
```

## ⚙️ 基本配置

### interval (运行间隔)

**类型**: `duration`  
**默认值**: `5m`  
**描述**: 重调度器的运行间隔时间

**支持的格式**:
- `30s` - 30秒
- `5m` - 5分钟  
- `1h` - 1小时
- `0` - 只运行一次后退出

**示例**:
```yaml
# 每2分钟运行一次
interval: "2m"

# 每半小时运行一次  
interval: "30m"

# 只运行一次
interval: "0"
```

### dryRun (模拟运行)

**类型**: `boolean`  
**默认值**: `false`  
**描述**: 是否只模拟运行而不实际驱逐Pod

**使用场景**:
- 🧪 **测试配置** - 验证策略行为
- 🔍 **调试问题** - 分析重调度逻辑
- 📊 **影响评估** - 评估重调度影响范围

**示例**:
```yaml
# 启用模拟模式
dryRun: true

# 禁用模拟模式（实际驱逐）
dryRun: false
```

**DryRun 模式日志示例**:
```
[DryRun] Would evict pod default/my-app-xxx on node worker-1, reason: Failed pod cleanup
```

### logLevel (日志级别)

**类型**: `string`  
**默认值**: `"info"`  
**可选值**: `"debug"`, `"info"`, `"warn"`, `"error"`

**级别说明**:
- `debug` - 详细调试信息，包含所有操作细节
- `info` - 一般信息，包含重要操作和统计
- `warn` - 警告信息，包含潜在问题
- `error` - 错误信息，只记录失败操作

**示例**:
```yaml
# 详细调试日志
logLevel: "debug"

# 生产环境推荐
logLevel: "info"  

# 静默模式
logLevel: "error"
```

## 🎯 节点选择器

### nodeSelector (节点选择器)

**类型**: `map[string]string`  
**默认值**: `nil` (处理所有节点)  
**描述**: 只处理匹配指定标签的节点

**使用场景**:
- 🏭 **环境隔离** - 只在特定环境运行
- 🔧 **节点分组** - 针对特定类型节点
- 🚫 **排除节点** - 避免处理特殊节点

**示例**:
```yaml
# 只处理生产环境节点
nodeSelector:
  environment: "production"

# 只处理工作节点
nodeSelector:
  node-role.kubernetes.io/worker: ""

# 多标签匹配
nodeSelector:
  environment: "production"
  node-type: "compute"
  zone: "us-west-1a"
```

## 🚦 驱逐限制

驱逐限制是重调度器的安全机制，防止过度驱逐影响集群稳定性。

### maxPodsToEvictPerNode (每节点限制)

**类型**: `int`  
**默认值**: `10`  
**描述**: 每个节点在单次运行中最多驱逐的Pod数量

**建议值**:
- **小集群** (< 10节点): `3-5`
- **中等集群** (10-50节点): `5-10` 
- **大集群** (> 50节点): `10-20`

**示例**:
```yaml
limits:
  maxPodsToEvictPerNode: 5  # 保守设置
```

### maxPodsToEvictPerNamespace (每命名空间限制)

**类型**: `int`  
**默认值**: `5`  
**描述**: 每个命名空间在单次运行中最多驱逐的Pod数量

**使用场景**:
- 🛡️ **应用保护** - 防止某个应用所有Pod被驱逐
- ⚖️ **资源公平** - 确保资源公平分配
- 🔒 **租户隔离** - 多租户环境保护

**示例**:
```yaml
limits:
  maxPodsToEvictPerNamespace: 2  # 每个应用最多2个Pod
```

### maxPodsToEvictTotal (总量限制)

**类型**: `int`  
**默认值**: `50`  
**描述**: 单次运行中最多驱逐的Pod总数

**建议配置**:
```yaml
# 保守配置 - 适合生产环境
limits:
  maxPodsToEvictTotal: 20

# 激进配置 - 适合大规模清理
limits:
  maxPodsToEvictTotal: 100
```

## 📋 策略配置

### removeFailedPods (失败Pod清理)

清理长时间处于失败状态的Pod。

```yaml
removeFailedPods:
  enabled: true
  minPodLifetimeSeconds: 300
  excludeOwnerKinds:
    - "Job"
  includedNamespaces:
    - "default"  
    - "my-app"
  excludedNamespaces:
    - "kube-system"
```

**参数说明**:

| 参数 | 类型 | 默认值 | 描述 |
|------|------|--------|------|
| `enabled` | boolean | `false` | 是否启用此策略 |
| `minPodLifetimeSeconds` | int | `0` | Pod最小存活时间（秒） |
| `excludeOwnerKinds` | []string | `[]` | 排除的Owner类型 |
| `includedNamespaces` | []string | `[]` | 包含的命名空间 |
| `excludedNamespaces` | []string | `[]` | 排除的命名空间 |

**常用Owner类型**:
- `Job` - 批处理任务
- `CronJob` - 定时任务
- `ReplicaSet` - 副本集
- `DaemonSet` - 守护进程集

### lowNodeUtilization (低节点利用率)

平衡节点间的资源利用率。

```yaml
lowNodeUtilization:
  enabled: true
  numberOfNodes: 1
  thresholds:
    cpu: 20
    memory: 20  
    pods: 20
  targetThresholds:
    cpu: 80
    memory: 80
    pods: 80
```

**参数说明**:

| 参数 | 类型 | 默认值 | 描述 |
|------|------|--------|------|
| `enabled` | boolean | `false` | 是否启用此策略 |
| `numberOfNodes` | int | `0` | 低利用率节点数量阈值 |
| `thresholds` | object | - | 低利用率阈值（百分比） |
| `targetThresholds` | object | - | 高利用率阈值（百分比） |

**阈值配置建议**:

| 场景 | CPU阈值 | 内存阈值 | Pod阈值 |
|------|---------|----------|---------|
| 保守 | 30/70 | 30/70 | 30/70 |
| 标准 | 20/80 | 20/80 | 20/80 |
| 激进 | 10/90 | 10/90 | 10/90 |

### removeDuplicates (重复Pod清理)

清理同一节点上的重复Pod实例。

```yaml
removeDuplicates:
  enabled: false  # 默认关闭
  excludeOwnerKinds:
    - "DaemonSet"
  includedNamespaces: []
  excludedNamespaces:
    - "kube-system"
```

**参数说明**:

| 参数 | 类型 | 默认值 | 描述 |
|------|------|--------|------|
| `enabled` | boolean | `false` | 是否启用此策略 |
| `excludeOwnerKinds` | []string | `[]` | 排除的Owner类型 |
| `includedNamespaces` | []string | `[]` | 包含的命名空间 |
| `excludedNamespaces` | []string | `[]` | 排除的命名空间 |

**注意事项**:
⚠️ 此策略较为激进，建议在充分测试后再启用

## 🔧 高级配置

### 环境变量配置

除了配置文件，还可以通过环境变量配置某些参数：

```bash
# 日志级别
export LOG_LEVEL=debug

# 配置文件路径
export CONFIG_PATH=/etc/descheduler/config.yaml
```

### 动态配置更新

修改ConfigMap后，重启Pod应用新配置：

```bash
# 修改配置
kubectl edit configmap lightweight-descheduler-config -n kube-system

# 重启应用新配置
kubectl rollout restart deployment/lightweight-descheduler -n kube-system
```

## 📊 配置模板

### 开发环境配置

```yaml
interval: "2m"
dryRun: true  # 安全模式
logLevel: "debug"

limits:
  maxPodsToEvictPerNode: 2
  maxPodsToEvictPerNamespace: 1
  maxPodsToEvictTotal: 5

strategies:
  removeFailedPods:
    enabled: true
    minPodLifetimeSeconds: 60
    excludedNamespaces:
      - "kube-system"
  
  lowNodeUtilization:
    enabled: false  # 开发环境通常不需要
```

### 生产环境配置

```yaml
interval: "10m"
dryRun: false
logLevel: "info"

# 只处理工作节点
nodeSelector:
  node-role.kubernetes.io/worker: ""

limits:
  maxPodsToEvictPerNode: 5
  maxPodsToEvictPerNamespace: 3
  maxPodsToEvictTotal: 20

strategies:
  removeFailedPods:
    enabled: true
    minPodLifetimeSeconds: 600  # 10分钟
    excludeOwnerKinds:
      - "Job"  # 保护批处理任务
    excludedNamespaces:
      - "kube-system"
      - "monitoring"
      - "ingress"
  
  lowNodeUtilization:
    enabled: true
    numberOfNodes: 2
    thresholds:
      cpu: 25
      memory: 25
      pods: 25
    targetThresholds:
      cpu: 75
      memory: 75
      pods: 75
```

### 大规模集群配置

```yaml
interval: "15m"
dryRun: false
logLevel: "info"

limits:
  maxPodsToEvictPerNode: 10
  maxPodsToEvictPerNamespace: 5
  maxPodsToEvictTotal: 50

strategies:
  removeFailedPods:
    enabled: true
    minPodLifetimeSeconds: 300
    
  lowNodeUtilization:
    enabled: true
    numberOfNodes: 5  # 大集群需要更多低利用率节点才触发
    thresholds:
      cpu: 15
      memory: 15
      pods: 15
    targetThresholds:
      cpu: 85
      memory: 85
      pods: 85
      
  removeDuplicates:
    enabled: true  # 大集群更容易出现重复Pod
    excludeOwnerKinds:
      - "DaemonSet"
```

## ✅ 配置验证

### 语法检查

```bash
# 使用 yq 验证 YAML 语法
yq eval '.' config.yaml > /dev/null && echo "语法正确" || echo "语法错误"

# 使用 kubectl 验证配置
kubectl create configmap test-config --from-file=config.yaml --dry-run=client -o yaml
```

### 逻辑检查

确保配置逻辑合理：

1. **阈值关系**: `thresholds` 应小于 `targetThresholds`
2. **限制合理**: 驱逐限制不应过大或过小
3. **命名空间**: 避免意外包含/排除重要命名空间

### 配置测试

```bash
# 启用 DryRun 模式测试
kubectl patch configmap lightweight-descheduler-config -n kube-system \
  --type merge -p '{"data":{"config.yaml":"dryRun: true\n..."}}'

# 观察日志验证行为
kubectl logs -n kube-system -l app=lightweight-descheduler -f
```

## 🚨 最佳实践

1. **🧪 始终先测试** - 新配置先在DryRun模式验证
2. **📊 监控影响** - 关注驱逐对应用的影响
3. **🔄 渐进调整** - 从保守配置开始，逐步优化
4. **📝 文档记录** - 记录配置变更和原因
5. **🚨 告警监控** - 设置异常驱逐的告警

## 🔍 故障排除

### 配置不生效

1. 检查YAML语法
2. 验证ConfigMap更新
3. 重启Pod应用配置
4. 查看错误日志

### 驱逐过多/过少

1. 调整限制参数
2. 修改策略阈值
3. 检查节点选择器
4. 验证命名空间过滤

---

**下一步**: 阅读 [策略详解](./strategies.md) 深入了解各种重调度策略的工作原理。
