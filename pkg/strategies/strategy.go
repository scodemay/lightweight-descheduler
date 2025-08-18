package strategies

import (
	"context"

	v1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"

	"lightweight-descheduler/pkg/config"
	"lightweight-descheduler/pkg/eviction"
)

// Strategy 重调度策略接口
type Strategy interface {
	// Name 策略名称
	Name() string

	// Execute 执行策略
	Execute(ctx context.Context, nodes []*v1.Node) error

	// IsEnabled 检查策略是否启用
	IsEnabled() bool
}

// StrategyContext 策略执行上下文
type StrategyContext struct {
	// Client Kubernetes客户端
	Client kubernetes.Interface

	// Config 配置信息
	Config *config.Config

	// Evictor Pod驱逐器
	Evictor eviction.PodEvictor
}

// StrategyFactory 策略工厂
type StrategyFactory struct {
	context *StrategyContext
}

// NewStrategyFactory 创建策略工厂
func NewStrategyFactory(client kubernetes.Interface, cfg *config.Config, evictor eviction.PodEvictor) *StrategyFactory {
	return &StrategyFactory{
		context: &StrategyContext{
			Client:  client,
			Config:  cfg,
			Evictor: evictor,
		},
	}
}

// CreateStrategies 创建所有启用的策略
func (f *StrategyFactory) CreateStrategies() []Strategy {
	var strategies []Strategy

	// 失败Pod清理策略
	if f.context.Config.Strategies.RemoveFailedPods != nil &&
		f.context.Config.Strategies.RemoveFailedPods.Enabled {
		strategies = append(strategies, NewRemoveFailedPodsStrategy(f.context))
	}

	// 低节点利用率策略
	if f.context.Config.Strategies.LowNodeUtilization != nil &&
		f.context.Config.Strategies.LowNodeUtilization.Enabled {
		strategies = append(strategies, NewLowNodeUtilizationStrategy(f.context))
	}

	// 重复Pod清理策略
	if f.context.Config.Strategies.RemoveDuplicates != nil &&
		f.context.Config.Strategies.RemoveDuplicates.Enabled {
		strategies = append(strategies, NewRemoveDuplicatesStrategy(f.context))
	}

	return strategies
}
