package config

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

// Config 重调度器的主要配置
type Config struct {
	// Interval 重调度运行间隔
	Interval time.Duration `yaml:"interval"`

	// DryRun 是否只是模拟运行，不实际驱逐Pod
	DryRun bool `yaml:"dryRun"`

	// NodeSelector 用于选择要处理的节点
	NodeSelector map[string]string `yaml:"nodeSelector,omitempty"`

	// Limits 驱逐限制配置
	Limits EvictionLimits `yaml:"limits"`

	// Strategies 启用的策略配置
	Strategies StrategiesConfig `yaml:"strategies"`

	// LogLevel 日志级别 (info, debug, warn, error)
	LogLevel string `yaml:"logLevel"`
}

// EvictionLimits 驱逐限制配置
type EvictionLimits struct {
	// MaxPodsToEvictPerNode 每个节点最大驱逐Pod数量
	MaxPodsToEvictPerNode int `yaml:"maxPodsToEvictPerNode"`

	// MaxPodsToEvictPerNamespace 每个命名空间最大驱逐Pod数量
	MaxPodsToEvictPerNamespace int `yaml:"maxPodsToEvictPerNamespace"`

	// MaxPodsToEvictTotal 每次运行最大驱逐Pod总数
	MaxPodsToEvictTotal int `yaml:"maxPodsToEvictTotal"`
}

// StrategiesConfig 策略配置
type StrategiesConfig struct {
	// RemoveFailedPods 失败Pod清理策略
	RemoveFailedPods *RemoveFailedPodsConfig `yaml:"removeFailedPods,omitempty"`

	// LowNodeUtilization 低节点利用率策略
	LowNodeUtilization *LowNodeUtilizationConfig `yaml:"lowNodeUtilization,omitempty"`

	// RemoveDuplicates 重复Pod清理策略
	RemoveDuplicates *RemoveDuplicatesConfig `yaml:"removeDuplicates,omitempty"`
}

// RemoveFailedPodsConfig 失败Pod清理策略配置
type RemoveFailedPodsConfig struct {
	Enabled bool `yaml:"enabled"`

	// MinPodLifetimeSeconds Pod最小存活时间（秒），小于此时间的Pod不会被驱逐
	MinPodLifetimeSeconds int `yaml:"minPodLifetimeSeconds"`

	// ExcludeOwnerKinds 排除的Owner类型，如Job, CronJob等
	ExcludeOwnerKinds []string `yaml:"excludeOwnerKinds,omitempty"`

	// IncludedNamespaces 只处理这些命名空间的Pod
	IncludedNamespaces []string `yaml:"includedNamespaces,omitempty"`

	// ExcludedNamespaces 排除这些命名空间的Pod
	ExcludedNamespaces []string `yaml:"excludedNamespaces,omitempty"`
}

// LowNodeUtilizationConfig 低节点利用率策略配置
type LowNodeUtilizationConfig struct {
	Enabled bool `yaml:"enabled"`

	// Thresholds 节点利用率阈值，低于此值的节点被认为是低利用率节点
	Thresholds ResourceThresholds `yaml:"thresholds"`

	// TargetThresholds 目标利用率阈值，高于此值的节点Pod可能被驱逐
	TargetThresholds ResourceThresholds `yaml:"targetThresholds"`

	// NumberOfNodes 只有当低利用率节点数量大于此值时才运行此策略
	NumberOfNodes int `yaml:"numberOfNodes"`
}

// RemoveDuplicatesConfig 重复Pod清理策略配置
type RemoveDuplicatesConfig struct {
	Enabled bool `yaml:"enabled"`

	// ExcludeOwnerKinds 排除的Owner类型
	ExcludeOwnerKinds []string `yaml:"excludeOwnerKinds,omitempty"`

	// IncludedNamespaces 只处理这些命名空间的Pod
	IncludedNamespaces []string `yaml:"includedNamespaces,omitempty"`

	// ExcludedNamespaces 排除这些命名空间的Pod
	ExcludedNamespaces []string `yaml:"excludedNamespaces,omitempty"`
}

// ResourceThresholds 资源阈值配置
type ResourceThresholds struct {
	// CPU CPU利用率阈值 (百分比, 0-100)
	CPU int `yaml:"cpu"`

	// Memory 内存利用率阈值 (百分比, 0-100)
	Memory int `yaml:"memory"`

	// Pods Pod数量利用率阈值 (百分比, 0-100)
	Pods int `yaml:"pods"`
}

// LoadConfig 从文件加载配置
func LoadConfig(filepath string) (*Config, error) {
	data, err := os.ReadFile(filepath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %v", err)
	}

	config := &Config{}
	if err := yaml.Unmarshal(data, config); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %v", err)
	}

	// 设置默认值
	if err := setDefaults(config); err != nil {
		return nil, fmt.Errorf("failed to set default values: %v", err)
	}

	// 验证配置
	if err := validateConfig(config); err != nil {
		return nil, fmt.Errorf("invalid config: %v", err)
	}

	return config, nil
}

// setDefaults 设置默认配置值
func setDefaults(config *Config) error {
	if config.Interval == 0 {
		config.Interval = 5 * time.Minute
	}

	if config.LogLevel == "" {
		config.LogLevel = "info"
	}

	if config.Limits.MaxPodsToEvictPerNode == 0 {
		config.Limits.MaxPodsToEvictPerNode = 10
	}

	if config.Limits.MaxPodsToEvictPerNamespace == 0 {
		config.Limits.MaxPodsToEvictPerNamespace = 5
	}

	if config.Limits.MaxPodsToEvictTotal == 0 {
		config.Limits.MaxPodsToEvictTotal = 50
	}

	return nil
}

// validateConfig 验证配置有效性
func validateConfig(config *Config) error {
	if config.Interval < time.Minute {
		return fmt.Errorf("interval must be at least 1 minute")
	}

	if config.Limits.MaxPodsToEvictPerNode < 0 {
		return fmt.Errorf("maxPodsToEvictPerNode must be >= 0")
	}

	if config.Limits.MaxPodsToEvictPerNamespace < 0 {
		return fmt.Errorf("maxPodsToEvictPerNamespace must be >= 0")
	}

	if config.Limits.MaxPodsToEvictTotal < 0 {
		return fmt.Errorf("maxPodsToEvictTotal must be >= 0")
	}

	// 验证策略配置
	if config.Strategies.LowNodeUtilization != nil && config.Strategies.LowNodeUtilization.Enabled {
		if err := validateResourceThresholds(&config.Strategies.LowNodeUtilization.Thresholds); err != nil {
			return fmt.Errorf("invalid thresholds: %v", err)
		}
		if err := validateResourceThresholds(&config.Strategies.LowNodeUtilization.TargetThresholds); err != nil {
			return fmt.Errorf("invalid targetThresholds: %v", err)
		}
	}

	return nil
}

// validateResourceThresholds 验证资源阈值配置
func validateResourceThresholds(thresholds *ResourceThresholds) error {
	if thresholds.CPU < 0 || thresholds.CPU > 100 {
		return fmt.Errorf("CPU threshold must be between 0 and 100")
	}
	if thresholds.Memory < 0 || thresholds.Memory > 100 {
		return fmt.Errorf("Memory threshold must be between 0 and 100")
	}
	if thresholds.Pods < 0 || thresholds.Pods > 100 {
		return fmt.Errorf("Pods threshold must be between 0 and 100")
	}
	return nil
}
