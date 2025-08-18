package strategies

import (
	"context"
	"fmt"
	"time"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"

	"lightweight-descheduler/pkg/config"
	"lightweight-descheduler/pkg/utils"
)

// RemoveFailedPodsStrategy 失败Pod清理策略
type RemoveFailedPodsStrategy struct {
	client  kubernetes.Interface
	config  *config.RemoveFailedPodsConfig
	context *StrategyContext
}

// NewRemoveFailedPodsStrategy 创建失败Pod清理策略
func NewRemoveFailedPodsStrategy(ctx *StrategyContext) *RemoveFailedPodsStrategy {
	return &RemoveFailedPodsStrategy{
		client:  ctx.Client,
		config:  ctx.Config.Strategies.RemoveFailedPods,
		context: ctx,
	}
}

// Name 返回策略名称
func (s *RemoveFailedPodsStrategy) Name() string {
	return "RemoveFailedPods"
}

// IsEnabled 检查策略是否启用
func (s *RemoveFailedPodsStrategy) IsEnabled() bool {
	return s.config != nil && s.config.Enabled
}

// Execute 执行失败Pod清理策略
func (s *RemoveFailedPodsStrategy) Execute(ctx context.Context, nodes []*v1.Node) error {
	klog.Infof("Executing %s strategy", s.Name())

	evictedCount := 0
	skippedCount := 0

	for _, node := range nodes {
		klog.V(2).Infof("Processing node: %s", node.Name)

		// 获取节点上的所有失败Pod
		failedPods, err := s.getFailedPods(ctx, node.Name)
		if err != nil {
			klog.Errorf("Failed to get failed pods on node %s: %v", node.Name, err)
			continue
		}

		klog.V(2).Infof("Found %d failed pods on node %s", len(failedPods), node.Name)

		// 处理每个失败的Pod
		for _, pod := range failedPods {
			// 检查是否可以驱逐此Pod
			if canEvict, reason := s.canEvictPod(pod); !canEvict {
				klog.V(3).Infof("Skipping pod %s/%s: %s", pod.Namespace, pod.Name, reason)
				skippedCount++
				continue
			}

			// 检查Pod是否满足驱逐条件
			if !s.shouldEvictPod(pod) {
				klog.V(3).Infof("Pod %s/%s does not meet eviction criteria", pod.Namespace, pod.Name)
				skippedCount++
				continue
			}

			// 驱逐Pod
			reason := fmt.Sprintf("Failed pod cleanup - Phase: %s", pod.Status.Phase)
			if pod.Status.Reason != "" {
				reason += fmt.Sprintf(", Reason: %s", pod.Status.Reason)
			}

			err := s.context.Evictor.EvictPod(ctx, pod, reason)
			if err != nil {
				klog.Errorf("Failed to evict pod %s/%s: %v", pod.Namespace, pod.Name, err)
				continue
			}

			evictedCount++
			klog.V(2).Infof("Successfully evicted failed pod %s/%s on node %s",
				pod.Namespace, pod.Name, node.Name)
		}
	}

	klog.Infof("RemoveFailedPods strategy completed. Evicted: %d, Skipped: %d",
		evictedCount, skippedCount)
	return nil
}

// getFailedPods 获取指定节点上的失败Pod
func (s *RemoveFailedPodsStrategy) getFailedPods(ctx context.Context, nodeName string) ([]*v1.Pod, error) {
	// 获取所有Pod
	pods, err := s.client.CoreV1().Pods("").List(ctx, metav1.ListOptions{
		FieldSelector: fmt.Sprintf("spec.nodeName=%s", nodeName),
	})
	if err != nil {
		return nil, err
	}

	var failedPods []*v1.Pod
	for i := range pods.Items {
		pod := &pods.Items[i]

		// 只处理失败状态的Pod
		if pod.Status.Phase == v1.PodFailed {
			failedPods = append(failedPods, pod)
		}
	}

	return failedPods, nil
}

// canEvictPod 检查是否可以驱逐Pod
func (s *RemoveFailedPodsStrategy) canEvictPod(pod *v1.Pod) (bool, string) {
	// 使用通用的驱逐检查
	return s.context.Evictor.CanEvictPod(pod)
}

// shouldEvictPod 检查Pod是否满足驱逐条件
func (s *RemoveFailedPodsStrategy) shouldEvictPod(pod *v1.Pod) bool {
	// 检查命名空间过滤
	if !s.shouldProcessNamespace(pod.Namespace) {
		return false
	}

	// 检查Pod最小存活时间
	if s.config.MinPodLifetimeSeconds > 0 {
		podAge := time.Since(pod.CreationTimestamp.Time).Seconds()
		if int(podAge) < s.config.MinPodLifetimeSeconds {
			klog.V(3).Infof("Pod %s/%s is too young (age: %ds, min: %ds)",
				pod.Namespace, pod.Name, int(podAge), s.config.MinPodLifetimeSeconds)
			return false
		}
	}

	// 检查排除的Owner类型
	if len(s.config.ExcludeOwnerKinds) > 0 {
		for _, ownerRef := range pod.OwnerReferences {
			if utils.Contains(s.config.ExcludeOwnerKinds, ownerRef.Kind) {
				klog.V(3).Infof("Pod %s/%s owner kind %s is excluded",
					pod.Namespace, pod.Name, ownerRef.Kind)
				return false
			}
		}
	}

	return true
}

// shouldProcessNamespace 检查是否应该处理此命名空间
func (s *RemoveFailedPodsStrategy) shouldProcessNamespace(namespace string) bool {
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
