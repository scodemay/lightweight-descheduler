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

// RemoveDuplicatesStrategy 重复Pod清理策略
type RemoveDuplicatesStrategy struct {
	client  kubernetes.Interface
	config  *config.RemoveDuplicatesConfig
	context *StrategyContext
}

// NewRemoveDuplicatesStrategy 创建重复Pod清理策略
func NewRemoveDuplicatesStrategy(ctx *StrategyContext) *RemoveDuplicatesStrategy {
	return &RemoveDuplicatesStrategy{
		client:  ctx.Client,
		config:  ctx.Config.Strategies.RemoveDuplicates,
		context: ctx,
	}
}

// Name 返回策略名称
func (s *RemoveDuplicatesStrategy) Name() string {
	return "RemoveDuplicates"
}

// IsEnabled 检查策略是否启用
func (s *RemoveDuplicatesStrategy) IsEnabled() bool {
	return s.config != nil && s.config.Enabled
}

// Execute 执行重复Pod清理策略
func (s *RemoveDuplicatesStrategy) Execute(ctx context.Context, nodes []*v1.Node) error {
	klog.Infof("Executing %s strategy", s.Name())

	evictedCount := 0
	skippedCount := 0

	// 收集所有节点上的Pod信息，按签名分组
	podGroups, err := s.groupPodsBySignature(ctx, nodes)
	if err != nil {
		return fmt.Errorf("failed to group pods by signature: %v", err)
	}

	klog.V(2).Infof("Found %d unique pod signatures", len(podGroups))

	// 处理每个Pod组，查找和驱逐重复的Pod
	for signature, nodePodsMap := range podGroups {
		klog.V(3).Infof("Processing pod signature: %s", signature)

		// 检查是否存在重复（同一个签名的Pod在多个节点上）
		duplicateNodes := s.findDuplicateNodes(nodePodsMap)
		if len(duplicateNodes) == 0 {
			continue
		}

		klog.V(2).Infof("Found duplicates for signature %s on nodes: %v", signature, duplicateNodes)

		// 驱逐重复的Pod，保留每个节点上最旧的Pod
		for _, nodeName := range duplicateNodes {
			pods := nodePodsMap[nodeName]
			if len(pods) <= 1 {
				continue
			}

			// 按创建时间排序，保留最旧的Pod
			sortedPods := s.sortPodsByAge(pods)

			// 驱逐除了最旧的Pod之外的所有Pod
			for i := 1; i < len(sortedPods); i++ {
				pod := sortedPods[i]

				// 检查是否可以驱逐此Pod
				if canEvict, reason := s.canEvictPod(pod); !canEvict {
					klog.V(3).Infof("Skipping duplicate pod %s/%s: %s", pod.Namespace, pod.Name, reason)
					skippedCount++
					continue
				}

				// 驱逐Pod
				evictionReason := fmt.Sprintf("Duplicate pod removal - keeping oldest pod on node %s", nodeName)
				err := s.context.Evictor.EvictPod(ctx, pod, evictionReason)
				if err != nil {
					klog.Errorf("Failed to evict duplicate pod %s/%s: %v", pod.Namespace, pod.Name, err)
					continue
				}

				evictedCount++
				klog.V(2).Infof("Successfully evicted duplicate pod %s/%s from node %s",
					pod.Namespace, pod.Name, nodeName)
			}
		}
	}

	klog.Infof("RemoveDuplicates strategy completed. Evicted: %d, Skipped: %d",
		evictedCount, skippedCount)
	return nil
}

// groupPodsBySignature 按Pod签名分组
func (s *RemoveDuplicatesStrategy) groupPodsBySignature(ctx context.Context, nodes []*v1.Node) (map[string]map[string][]*v1.Pod, error) {
	// podGroups[signature][nodeName] = []*v1.Pod
	podGroups := make(map[string]map[string][]*v1.Pod)

	for _, node := range nodes {
		klog.V(2).Infof("Processing node: %s", node.Name)

		// 获取节点上的Pod
		pods, err := s.getProcessablePods(ctx, node.Name)
		if err != nil {
			return nil, fmt.Errorf("failed to get pods on node %s: %v", node.Name, err)
		}

		// 为每个Pod生成签名并分组
		for _, pod := range pods {
			signature := utils.GeneratePodSignature(pod)

			if podGroups[signature] == nil {
				podGroups[signature] = make(map[string][]*v1.Pod)
			}

			podGroups[signature][node.Name] = append(podGroups[signature][node.Name], pod)
		}
	}

	return podGroups, nil
}

// getProcessablePods 获取节点上可处理的Pod
func (s *RemoveDuplicatesStrategy) getProcessablePods(ctx context.Context, nodeName string) ([]*v1.Pod, error) {
	podList, err := s.client.CoreV1().Pods("").List(ctx, metav1.ListOptions{
		FieldSelector: fmt.Sprintf("spec.nodeName=%s", nodeName),
	})
	if err != nil {
		return nil, err
	}

	var processablePods []*v1.Pod
	for i := range podList.Items {
		pod := &podList.Items[i]

		// 只处理正在运行的Pod
		if pod.Status.Phase != v1.PodRunning {
			continue
		}

		// 检查命名空间过滤
		if !s.shouldProcessNamespace(pod.Namespace) {
			continue
		}

		// 检查是否有Owner且不在排除列表中
		if !s.shouldProcessPod(pod) {
			continue
		}

		processablePods = append(processablePods, pod)
	}

	return processablePods, nil
}

// shouldProcessNamespace 检查是否应该处理此命名空间
func (s *RemoveDuplicatesStrategy) shouldProcessNamespace(namespace string) bool {
	// 如果指定了包含的命名空间，只处理这些命名空间
	if len(s.config.IncludedNamespaces) > 0 {
		return utils.Contains(s.config.IncludedNamespaces, namespace)
	}

	// 如果指定了排除的命名空间，不处理这些命名空间
	if len(s.config.ExcludedNamespaces) > 0 {
		return !utils.Contains(s.config.ExcludedNamespaces, namespace)
	}

	// 默认处理所有命名空间
	return true
}

// shouldProcessPod 检查是否应该处理此Pod
func (s *RemoveDuplicatesStrategy) shouldProcessPod(pod *v1.Pod) bool {
	// Pod必须有Owner
	if len(pod.OwnerReferences) == 0 {
		return false
	}

	// 检查排除的Owner类型
	if len(s.config.ExcludeOwnerKinds) > 0 {
		for _, ownerRef := range pod.OwnerReferences {
			if utils.Contains(s.config.ExcludeOwnerKinds, ownerRef.Kind) {
				return false
			}
		}
	}

	return true
}

// findDuplicateNodes 查找有重复Pod的节点
func (s *RemoveDuplicatesStrategy) findDuplicateNodes(nodePodsMap map[string][]*v1.Pod) []string {
	var duplicateNodes []string

	// 只有当同一个签名的Pod出现在多个节点上时才认为是重复
	if len(nodePodsMap) < 2 {
		return duplicateNodes
	}

	for nodeName, pods := range nodePodsMap {
		// 如果一个节点上同一个签名有多个Pod，或者多个节点都有这个签名的Pod
		if len(pods) > 1 {
			duplicateNodes = append(duplicateNodes, nodeName)
		}
	}

	// 如果没有节点有重复Pod，但多个节点都有相同签名的Pod，
	// 选择Pod数量最多的节点作为有重复的节点
	if len(duplicateNodes) == 0 && len(nodePodsMap) > 1 {
		maxPods := 0
		var maxNode string
		for nodeName, pods := range nodePodsMap {
			if len(pods) > maxPods {
				maxPods = len(pods)
				maxNode = nodeName
			}
		}
		if maxNode != "" {
			duplicateNodes = append(duplicateNodes, maxNode)
		}
	}

	return duplicateNodes
}

// sortPodsByAge 按Pod年龄排序（最旧的在前）
func (s *RemoveDuplicatesStrategy) sortPodsByAge(pods []*v1.Pod) []*v1.Pod {
	// 创建Pod的副本以避免修改原始切片
	sortedPods := make([]*v1.Pod, len(pods))
	copy(sortedPods, pods)

	// 简单的冒泡排序，按创建时间排序
	for i := 0; i < len(sortedPods)-1; i++ {
		for j := 0; j < len(sortedPods)-i-1; j++ {
			if sortedPods[j].CreationTimestamp.After(sortedPods[j+1].CreationTimestamp.Time) {
				sortedPods[j], sortedPods[j+1] = sortedPods[j+1], sortedPods[j]
			}
		}
	}

	return sortedPods
}

// canEvictPod 检查是否可以驱逐Pod
func (s *RemoveDuplicatesStrategy) canEvictPod(pod *v1.Pod) (bool, string) {
	// 使用通用的驱逐检查
	return s.context.Evictor.CanEvictPod(pod)
}
