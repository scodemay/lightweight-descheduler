package utils

import (
	"fmt"
	"sort"
	"strings"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Contains 检查切片是否包含指定元素
func Contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

// RemoveFromSlice 从切片中移除指定元素
func RemoveFromSlice(slice []string, item string) []string {
	var result []string
	for _, s := range slice {
		if s != item {
			result = append(result, s)
		}
	}
	return result
}

// NodeResourceUtilization 节点资源利用率信息
type NodeResourceUtilization struct {
	NodeName      string
	CPUUsage      int64 // CPU使用量（毫核心）
	MemoryUsage   int64 // 内存使用量（字节）
	PodsCount     int   // Pod数量
	CPUPercent    int   // CPU使用率百分比
	MemoryPercent int   // 内存使用率百分比
	PodsPercent   int   // Pod数量使用率百分比
}

// CalculateNodeUtilization 计算节点资源利用率
func CalculateNodeUtilization(node *v1.Node, pods []*v1.Pod) *NodeResourceUtilization {
	utilization := &NodeResourceUtilization{
		NodeName: node.Name,
	}

	// 获取节点可分配资源
	allocatable := node.Status.Allocatable
	cpuAllocatable := allocatable[v1.ResourceCPU]
	memoryAllocatable := allocatable[v1.ResourceMemory]
	podsAllocatable := allocatable[v1.ResourcePods]

	// 计算Pod资源请求总和
	var cpuRequests, memoryRequests resource.Quantity
	podCount := 0

	for _, pod := range pods {
		// 跳过失败和成功的Pod
		if pod.Status.Phase == v1.PodFailed || pod.Status.Phase == v1.PodSucceeded {
			continue
		}

		podCount++
		for _, container := range pod.Spec.Containers {
			if cpu := container.Resources.Requests[v1.ResourceCPU]; !cpu.IsZero() {
				cpuRequests.Add(cpu)
			}
			if memory := container.Resources.Requests[v1.ResourceMemory]; !memory.IsZero() {
				memoryRequests.Add(memory)
			}
		}
	}

	utilization.CPUUsage = cpuRequests.MilliValue()
	utilization.MemoryUsage = memoryRequests.Value()
	utilization.PodsCount = podCount

	// 计算使用率百分比
	if !cpuAllocatable.IsZero() {
		utilization.CPUPercent = int((cpuRequests.MilliValue() * 100) / cpuAllocatable.MilliValue())
	}
	if !memoryAllocatable.IsZero() {
		utilization.MemoryPercent = int((memoryRequests.Value() * 100) / memoryAllocatable.Value())
	}
	if !podsAllocatable.IsZero() {
		utilization.PodsPercent = int((int64(podCount) * 100) / podsAllocatable.Value())
	}

	return utilization
}

// IsNodeUnderUtilized 检查节点是否利用率不足
func IsNodeUnderUtilized(utilization *NodeResourceUtilization, thresholds map[string]int) bool {
	cpuThreshold := thresholds["cpu"]
	memoryThreshold := thresholds["memory"]
	podsThreshold := thresholds["pods"]

	return utilization.CPUPercent < cpuThreshold &&
		utilization.MemoryPercent < memoryThreshold &&
		utilization.PodsPercent < podsThreshold
}

// IsNodeOverUtilized 检查节点是否利用率过高
func IsNodeOverUtilized(utilization *NodeResourceUtilization, thresholds map[string]int) bool {
	cpuThreshold := thresholds["cpu"]
	memoryThreshold := thresholds["memory"]
	podsThreshold := thresholds["pods"]

	return utilization.CPUPercent > cpuThreshold ||
		utilization.MemoryPercent > memoryThreshold ||
		utilization.PodsPercent > podsThreshold
}

// PodKey 生成Pod的唯一标识
func PodKey(pod *v1.Pod) string {
	return fmt.Sprintf("%s/%s", pod.Namespace, pod.Name)
}

// GetPodOwners 获取Pod的Owner信息
func GetPodOwners(pod *v1.Pod) []metav1.OwnerReference {
	return pod.OwnerReferences
}

// GetPodImages 获取Pod中所有容器的镜像列表
func GetPodImages(pod *v1.Pod) []string {
	var images []string
	for _, container := range pod.Spec.Containers {
		images = append(images, container.Image)
	}
	sort.Strings(images)
	return images
}

// GeneratePodSignature 生成Pod的签名用于重复检测
func GeneratePodSignature(pod *v1.Pod) string {
	var parts []string

	// 添加命名空间
	parts = append(parts, pod.Namespace)

	// 添加Owner信息
	for _, owner := range pod.OwnerReferences {
		parts = append(parts, fmt.Sprintf("%s:%s", owner.Kind, owner.Name))
	}

	// 添加镜像信息
	images := GetPodImages(pod)
	parts = append(parts, strings.Join(images, ","))

	return strings.Join(parts, "|")
}

// IsReadyNode 检查节点是否就绪
func IsReadyNode(node *v1.Node) bool {
	for _, condition := range node.Status.Conditions {
		if condition.Type == v1.NodeReady {
			return condition.Status == v1.ConditionTrue
		}
	}
	return false
}

// IsSchedulableNode 检查节点是否可调度
func IsSchedulableNode(node *v1.Node) bool {
	return !node.Spec.Unschedulable
}

// FilterReadySchedulableNodes 过滤出就绪且可调度的节点
func FilterReadySchedulableNodes(nodes []*v1.Node) []*v1.Node {
	var readyNodes []*v1.Node
	for _, node := range nodes {
		if IsReadyNode(node) && IsSchedulableNode(node) {
			readyNodes = append(readyNodes, node)
		}
	}
	return readyNodes
}

// FormatBytes 格式化字节数为可读字符串
func FormatBytes(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

// FormatCPU 格式化CPU为可读字符串
func FormatCPU(milliCores int64) string {
	if milliCores < 1000 {
		return fmt.Sprintf("%dm", milliCores)
	}
	return fmt.Sprintf("%.1f", float64(milliCores)/1000.0)
}
