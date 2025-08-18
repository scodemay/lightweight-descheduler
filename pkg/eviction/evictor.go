package eviction

import (
	"context"
	"fmt"
	"sync"

	v1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"

	"lightweight-descheduler/pkg/config"
)

// PodEvictor Pod驱逐器接口
type PodEvictor interface {
	// EvictPod 驱逐指定的Pod
	EvictPod(ctx context.Context, pod *v1.Pod, reason string) error

	// CanEvictPod 检查是否可以驱逐指定的Pod
	CanEvictPod(pod *v1.Pod) (bool, string)

	// GetEvictionStats 获取驱逐统计信息
	GetEvictionStats() EvictionStats

	// ResetStats 重置统计信息
	ResetStats()
}

// EvictionStats 驱逐统计信息
type EvictionStats struct {
	// TotalEvicted 总驱逐数量
	TotalEvicted int

	// EvictedByNode 按节点统计的驱逐数量
	EvictedByNode map[string]int

	// EvictedByNamespace 按命名空间统计的驱逐数量
	EvictedByNamespace map[string]int

	// EvictedByReason 按原因统计的驱逐数量
	EvictedByReason map[string]int

	// FailedEvictions 驱逐失败数量
	FailedEvictions int
}

// DefaultPodEvictor 默认Pod驱逐器实现
type DefaultPodEvictor struct {
	client      kubernetes.Interface
	config      *config.Config
	stats       EvictionStats
	mu          sync.RWMutex
	gracePeriod *int64
}

// NewDefaultPodEvictor 创建默认Pod驱逐器
func NewDefaultPodEvictor(client kubernetes.Interface, cfg *config.Config) *DefaultPodEvictor {
	gracePeriod := int64(30) // 30秒优雅删除时间
	return &DefaultPodEvictor{
		client:      client,
		config:      cfg,
		gracePeriod: &gracePeriod,
		stats: EvictionStats{
			EvictedByNode:      make(map[string]int),
			EvictedByNamespace: make(map[string]int),
			EvictedByReason:    make(map[string]int),
		},
	}
}

// EvictPod 实现Pod驱逐
func (e *DefaultPodEvictor) EvictPod(ctx context.Context, pod *v1.Pod, reason string) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	// 检查驱逐限制
	if err := e.checkEvictionLimits(pod); err != nil {
		return err
	}

	// 如果是DryRun模式，只记录日志不实际驱逐
	if e.config.DryRun {
		klog.Infof("[DryRun] Would evict pod %s/%s on node %s, reason: %s",
			pod.Namespace, pod.Name, pod.Spec.NodeName, reason)
		e.updateStats(pod, reason, true)
		return nil
	}

	// 创建驱逐对象
	eviction := &policyv1.Eviction{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "policy/v1",
			Kind:       "Eviction",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      pod.Name,
			Namespace: pod.Namespace,
		},
		DeleteOptions: &metav1.DeleteOptions{
			GracePeriodSeconds: e.gracePeriod,
		},
	}

	// 执行驱逐
	err := e.client.PolicyV1().Evictions(pod.Namespace).Evict(ctx, eviction)
	if err != nil {
		e.stats.FailedEvictions++
		klog.Errorf("Failed to evict pod %s/%s: %v", pod.Namespace, pod.Name, err)
		return fmt.Errorf("failed to evict pod %s/%s: %v", pod.Namespace, pod.Name, err)
	}

	klog.Infof("Successfully evicted pod %s/%s on node %s, reason: %s",
		pod.Namespace, pod.Name, pod.Spec.NodeName, reason)

	e.updateStats(pod, reason, true)
	return nil
}

// CanEvictPod 检查是否可以驱逐Pod
func (e *DefaultPodEvictor) CanEvictPod(pod *v1.Pod) (bool, string) {
	// 系统关键Pod不能驱逐
	if isSystemCriticalPod(pod) {
		return false, "system critical pod"
	}

	// DaemonSet的Pod不能驱逐
	if isDaemonSetPod(pod) {
		return false, "daemonset pod"
	}

	// 静态Pod不能驱逐
	if isStaticPod(pod) {
		return false, "static pod"
	}

	// 没有控制器的Pod不能驱逐（除非是失败状态）
	if isStandalonePod(pod) && pod.Status.Phase != v1.PodFailed {
		return false, "standalone pod (not failed)"
	}

	// 正在删除的Pod不能驱逐
	if pod.DeletionTimestamp != nil {
		return false, "pod is being deleted"
	}

	// 有本地存储的Pod默认不驱逐
	if hasLocalStorage(pod) {
		return false, "pod has local storage"
	}

	return true, ""
}

// GetEvictionStats 获取驱逐统计信息
func (e *DefaultPodEvictor) GetEvictionStats() EvictionStats {
	e.mu.RLock()
	defer e.mu.RUnlock()

	// 深拷贝统计信息
	stats := EvictionStats{
		TotalEvicted:       e.stats.TotalEvicted,
		FailedEvictions:    e.stats.FailedEvictions,
		EvictedByNode:      make(map[string]int),
		EvictedByNamespace: make(map[string]int),
		EvictedByReason:    make(map[string]int),
	}

	for k, v := range e.stats.EvictedByNode {
		stats.EvictedByNode[k] = v
	}
	for k, v := range e.stats.EvictedByNamespace {
		stats.EvictedByNamespace[k] = v
	}
	for k, v := range e.stats.EvictedByReason {
		stats.EvictedByReason[k] = v
	}

	return stats
}

// ResetStats 重置统计信息
func (e *DefaultPodEvictor) ResetStats() {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.stats = EvictionStats{
		EvictedByNode:      make(map[string]int),
		EvictedByNamespace: make(map[string]int),
		EvictedByReason:    make(map[string]int),
	}
}

// checkEvictionLimits 检查驱逐限制
func (e *DefaultPodEvictor) checkEvictionLimits(pod *v1.Pod) error {
	limits := e.config.Limits

	// 检查总驱逐限制
	if limits.MaxPodsToEvictTotal > 0 && e.stats.TotalEvicted >= limits.MaxPodsToEvictTotal {
		return fmt.Errorf("reached total eviction limit: %d", limits.MaxPodsToEvictTotal)
	}

	// 检查节点驱逐限制
	if limits.MaxPodsToEvictPerNode > 0 && pod.Spec.NodeName != "" {
		if e.stats.EvictedByNode[pod.Spec.NodeName] >= limits.MaxPodsToEvictPerNode {
			return fmt.Errorf("reached node %s eviction limit: %d",
				pod.Spec.NodeName, limits.MaxPodsToEvictPerNode)
		}
	}

	// 检查命名空间驱逐限制
	if limits.MaxPodsToEvictPerNamespace > 0 {
		if e.stats.EvictedByNamespace[pod.Namespace] >= limits.MaxPodsToEvictPerNamespace {
			return fmt.Errorf("reached namespace %s eviction limit: %d",
				pod.Namespace, limits.MaxPodsToEvictPerNamespace)
		}
	}

	return nil
}

// updateStats 更新驱逐统计信息
func (e *DefaultPodEvictor) updateStats(pod *v1.Pod, reason string, success bool) {
	if success {
		e.stats.TotalEvicted++
		if pod.Spec.NodeName != "" {
			e.stats.EvictedByNode[pod.Spec.NodeName]++
		}
		e.stats.EvictedByNamespace[pod.Namespace]++
		e.stats.EvictedByReason[reason]++
	}
}

// isSystemCriticalPod 检查是否是系统关键Pod
func isSystemCriticalPod(pod *v1.Pod) bool {
	// 检查优先级类
	if pod.Spec.PriorityClassName == "system-cluster-critical" ||
		pod.Spec.PriorityClassName == "system-node-critical" {
		return true
	}

	// 检查系统命名空间
	systemNamespaces := []string{"kube-system", "kube-public", "kube-node-lease"}
	for _, ns := range systemNamespaces {
		if pod.Namespace == ns {
			return true
		}
	}

	return false
}

// isDaemonSetPod 检查是否是DaemonSet的Pod
func isDaemonSetPod(pod *v1.Pod) bool {
	for _, owner := range pod.OwnerReferences {
		if owner.Kind == "DaemonSet" {
			return true
		}
	}
	return false
}

// isStaticPod 检查是否是静态Pod
func isStaticPod(pod *v1.Pod) bool {
	source, ok := pod.Annotations["kubernetes.io/config.source"]
	return ok && source == "file"
}

// isStandalonePod 检查是否是独立Pod（没有控制器）
func isStandalonePod(pod *v1.Pod) bool {
	return len(pod.OwnerReferences) == 0
}

// hasLocalStorage 检查Pod是否使用了本地存储
func hasLocalStorage(pod *v1.Pod) bool {
	for _, volume := range pod.Spec.Volumes {
		if volume.HostPath != nil || volume.EmptyDir != nil {
			return true
		}
	}
	return false
}
