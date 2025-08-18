package strategies

import (
	"context"
	"fmt"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"

	"lightweight-descheduler/pkg/config"
	"lightweight-descheduler/pkg/utils"
)

// LowNodeUtilizationStrategy 低节点利用率策略
type LowNodeUtilizationStrategy struct {
	client  kubernetes.Interface
	config  *config.LowNodeUtilizationConfig
	context *StrategyContext
}

// NewLowNodeUtilizationStrategy 创建低节点利用率策略
func NewLowNodeUtilizationStrategy(ctx *StrategyContext) *LowNodeUtilizationStrategy {
	return &LowNodeUtilizationStrategy{
		client:  ctx.Client,
		config:  ctx.Config.Strategies.LowNodeUtilization,
		context: ctx,
	}
}

// Name 返回策略名称
func (s *LowNodeUtilizationStrategy) Name() string {
	return "LowNodeUtilization"
}

// IsEnabled 检查策略是否启用
func (s *LowNodeUtilizationStrategy) IsEnabled() bool {
	return s.config != nil && s.config.Enabled
}

// Execute 执行低节点利用率策略
func (s *LowNodeUtilizationStrategy) Execute(ctx context.Context, nodes []*v1.Node) error {
	klog.Infof("Executing %s strategy", s.Name())

	// 过滤出就绪且可调度的节点
	readyNodes := utils.FilterReadySchedulableNodes(nodes)
	if len(readyNodes) < 2 {
		klog.Infof("Need at least 2 ready nodes, found %d. Skipping strategy.", len(readyNodes))
		return nil
	}

	// 计算每个节点的资源利用率
	nodeUtilizations, err := s.calculateNodeUtilizations(ctx, readyNodes)
	if err != nil {
		return fmt.Errorf("failed to calculate node utilizations: %v", err)
	}

	// 分类节点：低利用率、高利用率、正常利用率
	lowUtilizationNodes, overUtilizationNodes := s.categorizeNodes(nodeUtilizations)

	klog.Infof("Found %d low utilization nodes and %d over utilization nodes",
		len(lowUtilizationNodes), len(overUtilizationNodes))

	// 检查是否满足执行条件
	if len(lowUtilizationNodes) < s.config.NumberOfNodes {
		klog.Infof("Low utilization nodes (%d) below threshold (%d). Skipping strategy.",
			len(lowUtilizationNodes), s.config.NumberOfNodes)
		return nil
	}

	if len(overUtilizationNodes) == 0 {
		klog.Infof("No over utilization nodes found. Skipping strategy.")
		return nil
	}

	// 从高利用率节点驱逐Pod到低利用率节点
	return s.evictPodsFromOverUtilizedNodes(ctx, overUtilizationNodes, lowUtilizationNodes)
}

// calculateNodeUtilizations 计算节点资源利用率
func (s *LowNodeUtilizationStrategy) calculateNodeUtilizations(ctx context.Context, nodes []*v1.Node) (map[string]*utils.NodeResourceUtilization, error) {
	utilizations := make(map[string]*utils.NodeResourceUtilization)

	for _, node := range nodes {
		// 获取节点上的Pod
		pods, err := s.getPodsOnNode(ctx, node.Name)
		if err != nil {
			return nil, fmt.Errorf("failed to get pods on node %s: %v", node.Name, err)
		}

		// 计算利用率
		utilization := utils.CalculateNodeUtilization(node, pods)
		utilizations[node.Name] = utilization

		klog.V(2).Infof("Node %s utilization: CPU=%d%%, Memory=%d%%, Pods=%d%%",
			node.Name, utilization.CPUPercent, utilization.MemoryPercent, utilization.PodsPercent)
	}

	return utilizations, nil
}

// getPodsOnNode 获取指定节点上的Pod
func (s *LowNodeUtilizationStrategy) getPodsOnNode(ctx context.Context, nodeName string) ([]*v1.Pod, error) {
	podList, err := s.client.CoreV1().Pods("").List(ctx, metav1.ListOptions{
		FieldSelector: fmt.Sprintf("spec.nodeName=%s", nodeName),
	})
	if err != nil {
		return nil, err
	}

	var pods []*v1.Pod
	for i := range podList.Items {
		pods = append(pods, &podList.Items[i])
	}

	return pods, nil
}

// categorizeNodes 分类节点
func (s *LowNodeUtilizationStrategy) categorizeNodes(utilizations map[string]*utils.NodeResourceUtilization) (
	lowUtilization []*utils.NodeResourceUtilization,
	overUtilization []*utils.NodeResourceUtilization) {

	// 转换配置为map格式
	thresholds := map[string]int{
		"cpu":    s.config.Thresholds.CPU,
		"memory": s.config.Thresholds.Memory,
		"pods":   s.config.Thresholds.Pods,
	}
	targetThresholds := map[string]int{
		"cpu":    s.config.TargetThresholds.CPU,
		"memory": s.config.TargetThresholds.Memory,
		"pods":   s.config.TargetThresholds.Pods,
	}

	for _, utilization := range utilizations {
		if utils.IsNodeUnderUtilized(utilization, thresholds) {
			lowUtilization = append(lowUtilization, utilization)
			klog.V(2).Infof("Node %s is under-utilized", utilization.NodeName)
		} else if utils.IsNodeOverUtilized(utilization, targetThresholds) {
			overUtilization = append(overUtilization, utilization)
			klog.V(2).Infof("Node %s is over-utilized", utilization.NodeName)
		}
	}

	return lowUtilization, overUtilization
}

// evictPodsFromOverUtilizedNodes 从高利用率节点驱逐Pod
func (s *LowNodeUtilizationStrategy) evictPodsFromOverUtilizedNodes(
	ctx context.Context,
	overUtilizedNodes []*utils.NodeResourceUtilization,
	_ []*utils.NodeResourceUtilization) error {

	evictedCount := 0
	skippedCount := 0

	for _, nodeUtil := range overUtilizedNodes {
		klog.V(2).Infof("Processing over-utilized node: %s (CPU=%d%%, Memory=%d%%, Pods=%d%%)",
			nodeUtil.NodeName, nodeUtil.CPUPercent, nodeUtil.MemoryPercent, nodeUtil.PodsPercent)

		// 获取可驱逐的Pod
		evictablePods, err := s.getEvictablePodsOnNode(ctx, nodeUtil.NodeName)
		if err != nil {
			klog.Errorf("Failed to get evictable pods on node %s: %v", nodeUtil.NodeName, err)
			continue
		}

		// 按优先级排序Pod，优先驱逐低优先级的Pod
		sortedPods := s.sortPodsByPriority(evictablePods)

		// 驱逐Pod，但限制数量避免过度驱逐
		maxEvictions := s.calculateMaxEvictions(nodeUtil)
		evicted := 0

		for _, pod := range sortedPods {
			if evicted >= maxEvictions {
				break
			}

			// 检查是否可以驱逐此Pod
			if canEvict, reason := s.context.Evictor.CanEvictPod(pod); !canEvict {
				klog.V(3).Infof("Skipping pod %s/%s: %s", pod.Namespace, pod.Name, reason)
				skippedCount++
				continue
			}

			// 驱逐Pod
			evictionReason := fmt.Sprintf("Node over-utilization balancing - CPU=%d%%, Memory=%d%%, Pods=%d%%",
				nodeUtil.CPUPercent, nodeUtil.MemoryPercent, nodeUtil.PodsPercent)

			err := s.context.Evictor.EvictPod(ctx, pod, evictionReason)
			if err != nil {
				klog.Errorf("Failed to evict pod %s/%s: %v", pod.Namespace, pod.Name, err)
				continue
			}

			evicted++
			evictedCount++
			klog.V(2).Infof("Successfully evicted pod %s/%s from over-utilized node %s",
				pod.Namespace, pod.Name, nodeUtil.NodeName)
		}

		klog.V(2).Infof("Evicted %d pods from node %s", evicted, nodeUtil.NodeName)
	}

	klog.Infof("LowNodeUtilization strategy completed. Evicted: %d, Skipped: %d",
		evictedCount, skippedCount)
	return nil
}

// getEvictablePodsOnNode 获取节点上可驱逐的Pod
func (s *LowNodeUtilizationStrategy) getEvictablePodsOnNode(ctx context.Context, nodeName string) ([]*v1.Pod, error) {
	pods, err := s.getPodsOnNode(ctx, nodeName)
	if err != nil {
		return nil, err
	}

	var evictablePods []*v1.Pod
	for _, pod := range pods {
		// 跳过系统Pod和特殊状态的Pod
		if pod.Status.Phase == v1.PodSucceeded || pod.Status.Phase == v1.PodFailed {
			continue
		}

		if canEvict, _ := s.context.Evictor.CanEvictPod(pod); canEvict {
			evictablePods = append(evictablePods, pod)
		}
	}

	return evictablePods, nil
}

// sortPodsByPriority 按优先级排序Pod
func (s *LowNodeUtilizationStrategy) sortPodsByPriority(pods []*v1.Pod) []*v1.Pod {
	// 简单的排序策略：优先驱逐没有优先级类的Pod
	var lowPriorityPods, normalPods []*v1.Pod

	for _, pod := range pods {
		if pod.Spec.PriorityClassName == "" || pod.Spec.Priority == nil || *pod.Spec.Priority <= 0 {
			lowPriorityPods = append(lowPriorityPods, pod)
		} else {
			normalPods = append(normalPods, pod)
		}
	}

	// 先返回低优先级的Pod
	result := append(lowPriorityPods, normalPods...)
	return result
}

// calculateMaxEvictions 计算节点的最大驱逐数量
func (s *LowNodeUtilizationStrategy) calculateMaxEvictions(nodeUtil *utils.NodeResourceUtilization) int {
	// 简单策略：根据超出阈值的程度计算驱逐数量
	cpuExcess := max(0, nodeUtil.CPUPercent-s.config.TargetThresholds.CPU)
	memoryExcess := max(0, nodeUtil.MemoryPercent-s.config.TargetThresholds.Memory)
	podsExcess := max(0, nodeUtil.PodsPercent-s.config.TargetThresholds.Pods)

	// 取最大的超出比例，转换为驱逐Pod数量
	maxExcess := max(cpuExcess, max(memoryExcess, podsExcess))

	// 基于超出比例计算驱逐数量，最少1个，最多5个
	evictions := max(1, min(5, maxExcess/10))

	return evictions
}

// max 返回两个整数的最大值
func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// min 返回两个整数的最小值
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
