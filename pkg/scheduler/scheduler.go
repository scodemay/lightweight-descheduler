package scheduler

import (
	"context"
	"fmt"
	"time"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"

	"lightweight-descheduler/pkg/config"
	"lightweight-descheduler/pkg/eviction"
	"lightweight-descheduler/pkg/strategies"
	"lightweight-descheduler/pkg/utils"
)

// Scheduler 轻量级重调度器
type Scheduler struct {
	client     kubernetes.Interface
	config     *config.Config
	evictor    eviction.PodEvictor
	strategies []strategies.Strategy
}

// NewScheduler 创建新的重调度器
func NewScheduler(client kubernetes.Interface, cfg *config.Config) (*Scheduler, error) {
	// 创建Pod驱逐器
	evictor := eviction.NewDefaultPodEvictor(client, cfg)

	// 创建策略工厂
	strategyFactory := strategies.NewStrategyFactory(client, cfg, evictor)

	// 创建所有启用的策略
	enabledStrategies := strategyFactory.CreateStrategies()

	scheduler := &Scheduler{
		client:     client,
		config:     cfg,
		evictor:    evictor,
		strategies: enabledStrategies,
	}

	klog.Infof("Created scheduler with %d enabled strategies", len(enabledStrategies))
	for _, strategy := range enabledStrategies {
		klog.Infof("  - %s", strategy.Name())
	}

	return scheduler, nil
}

// Run 运行重调度器
func (s *Scheduler) Run(ctx context.Context) error {
	klog.Infof("Starting lightweight descheduler")
	klog.Infof("Configuration: DryRun=%v, Interval=%v", s.config.DryRun, s.config.Interval)

	if s.config.DryRun {
		klog.Infof("Running in DRY RUN mode - no pods will actually be evicted")
	}

	// 如果间隔为0，只运行一次
	if s.config.Interval == 0 {
		return s.runOnce(ctx)
	}

	// 定期运行
	ticker := time.NewTicker(s.config.Interval)
	defer ticker.Stop()

	// 立即运行一次
	if err := s.runOnce(ctx); err != nil {
		klog.Errorf("Initial run failed: %v", err)
	}

	for {
		select {
		case <-ctx.Done():
			klog.Infof("Scheduler stopped by context cancellation")
			return ctx.Err()
		case <-ticker.C:
			if err := s.runOnce(ctx); err != nil {
				klog.Errorf("Scheduler run failed: %v", err)
			}
		}
	}
}

// runOnce 执行一次重调度循环
func (s *Scheduler) runOnce(ctx context.Context) error {
	startTime := time.Now()
	klog.Infof("=== Starting descheduling cycle ===")

	// 重置驱逐统计
	s.evictor.ResetStats()

	// 获取可用节点
	nodes, err := s.getAvailableNodes(ctx)
	if err != nil {
		return fmt.Errorf("failed to get available nodes: %v", err)
	}

	klog.Infof("Found %d available nodes", len(nodes))
	if len(nodes) < 2 {
		klog.Infof("Need at least 2 nodes for descheduling, found %d. Skipping cycle.", len(nodes))
		return nil
	}

	// 应用节点选择器过滤
	filteredNodes := s.filterNodesBySelector(nodes)
	klog.Infof("After node selector filtering: %d nodes", len(filteredNodes))

	if len(filteredNodes) == 0 {
		klog.Infof("No nodes match the node selector. Skipping cycle.")
		return nil
	}

	// 执行所有启用的策略
	for _, strategy := range s.strategies {
		if !strategy.IsEnabled() {
			continue
		}

		klog.Infof("--- Executing strategy: %s ---", strategy.Name())
		strategyStartTime := time.Now()

		err := strategy.Execute(ctx, filteredNodes)
		if err != nil {
			klog.Errorf("Strategy %s failed: %v", strategy.Name(), err)
			continue
		}

		strategyDuration := time.Since(strategyStartTime)
		klog.Infof("Strategy %s completed in %v", strategy.Name(), strategyDuration)
	}

	// 输出统计信息
	s.printCycleStats(startTime)

	klog.Infof("=== Descheduling cycle completed ===")
	return nil
}

// getAvailableNodes 获取可用的节点
func (s *Scheduler) getAvailableNodes(ctx context.Context) ([]*v1.Node, error) {
	nodeList, err := s.client.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}

	var availableNodes []*v1.Node
	for i := range nodeList.Items {
		node := &nodeList.Items[i]

		// 只考虑就绪且可调度的节点
		if utils.IsReadyNode(node) && utils.IsSchedulableNode(node) {
			availableNodes = append(availableNodes, node)
			klog.V(2).Infof("Node %s is available for descheduling", node.Name)
		} else {
			klog.V(2).Infof("Node %s is not available (ready=%v, schedulable=%v)",
				node.Name, utils.IsReadyNode(node), utils.IsSchedulableNode(node))
		}
	}

	return availableNodes, nil
}

// filterNodesBySelector 根据节点选择器过滤节点
func (s *Scheduler) filterNodesBySelector(nodes []*v1.Node) []*v1.Node {
	if len(s.config.NodeSelector) == 0 {
		return nodes
	}

	var filteredNodes []*v1.Node
	for _, node := range nodes {
		if s.nodeMatchesSelector(node) {
			filteredNodes = append(filteredNodes, node)
			klog.V(2).Infof("Node %s matches node selector", node.Name)
		} else {
			klog.V(2).Infof("Node %s does not match node selector", node.Name)
		}
	}

	return filteredNodes
}

// nodeMatchesSelector 检查节点是否匹配选择器
func (s *Scheduler) nodeMatchesSelector(node *v1.Node) bool {
	for key, value := range s.config.NodeSelector {
		nodeValue, exists := node.Labels[key]
		if !exists || nodeValue != value {
			return false
		}
	}
	return true
}

// printCycleStats 输出循环统计信息
func (s *Scheduler) printCycleStats(startTime time.Time) {
	duration := time.Since(startTime)
	stats := s.evictor.GetEvictionStats()

	klog.Infof("=== Cycle Statistics ===")
	klog.Infof("Duration: %v", duration)
	klog.Infof("Total evicted: %d", stats.TotalEvicted)
	klog.Infof("Failed evictions: %d", stats.FailedEvictions)

	if len(stats.EvictedByNode) > 0 {
		klog.Infof("Evictions by node:")
		for nodeName, count := range stats.EvictedByNode {
			klog.Infof("  %s: %d", nodeName, count)
		}
	}

	if len(stats.EvictedByNamespace) > 0 {
		klog.Infof("Evictions by namespace:")
		for namespace, count := range stats.EvictedByNamespace {
			klog.Infof("  %s: %d", namespace, count)
		}
	}

	if len(stats.EvictedByReason) > 0 {
		klog.Infof("Evictions by reason:")
		for reason, count := range stats.EvictedByReason {
			klog.Infof("  %s: %d", reason, count)
		}
	}
}

// GetStats 获取调度器统计信息
func (s *Scheduler) GetStats() eviction.EvictionStats {
	return s.evictor.GetEvictionStats()
}

// Stop 停止调度器
func (s *Scheduler) Stop() {
	klog.Infof("Scheduler stopping...")
	// 这里可以添加清理逻辑
}
