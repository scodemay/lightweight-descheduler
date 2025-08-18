package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
	"k8s.io/klog/v2"

	"lightweight-descheduler/pkg/config"
	"lightweight-descheduler/pkg/scheduler"
)

var (
	configPath  = flag.String("config", "", "Path to configuration file")
	kubeconfig  = flag.String("kubeconfig", "", "Path to kubeconfig file (optional, defaults to in-cluster config)")
	logLevel    = flag.String("log-level", "2", "Log level (0-5)")
	showVersion = flag.Bool("version", false, "Show version and exit")
	showHelp    = flag.Bool("help", false, "Show help and exit")
)

const (
	version = "v1.0.0"
	appName = "lightweight-descheduler"
)

func main() {
	// 初始化klog
	klog.InitFlags(nil)
	flag.Parse()

	if *showVersion {
		fmt.Printf("%s version %s\n", appName, version)
		os.Exit(0)
	}

	if *showHelp {
		printHelp()
		os.Exit(0)
	}

	// 设置日志级别
	if err := flag.Set("v", *logLevel); err != nil {
		klog.Fatalf("Failed to set log level: %v", err)
	}

	klog.Infof("Starting %s %s", appName, version)

	// 加载配置
	cfg, err := loadConfig()
	if err != nil {
		klog.Fatalf("Failed to load configuration: %v", err)
	}

	klog.Infof("Configuration loaded successfully")
	klog.Infof("DryRun: %v, Interval: %v, LogLevel: %s",
		cfg.DryRun, cfg.Interval, cfg.LogLevel)

	// 创建Kubernetes客户端
	client, err := createKubernetesClient()
	if err != nil {
		klog.Fatalf("Failed to create kubernetes client: %v", err)
	}

	klog.Infof("Kubernetes client created successfully")

	// 创建调度器
	sched, err := scheduler.NewScheduler(client, cfg)
	if err != nil {
		klog.Fatalf("Failed to create scheduler: %v", err)
	}

	// 设置信号处理
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		sig := <-sigChan
		klog.Infof("Received signal %v, shutting down...", sig)
		cancel()
	}()

	// 运行调度器
	klog.Infof("Starting scheduler...")
	if err := sched.Run(ctx); err != nil && err != context.Canceled {
		klog.Fatalf("Scheduler failed: %v", err)
	}

	klog.Infof("Scheduler stopped gracefully")
}

// loadConfig 加载配置文件
func loadConfig() (*config.Config, error) {
	configFile := *configPath

	// 如果没有指定配置文件，尝试默认位置
	if configFile == "" {
		defaultPaths := []string{
			"./config.yaml",
			"/etc/descheduler/config.yaml",
			"./configs/config.yaml",
		}

		for _, path := range defaultPaths {
			if _, err := os.Stat(path); err == nil {
				configFile = path
				break
			}
		}

		if configFile == "" {
			return nil, fmt.Errorf("no configuration file found. Please specify with -config flag or place config.yaml in current directory")
		}
	}

	klog.Infof("Loading configuration from: %s", configFile)
	return config.LoadConfig(configFile)
}

// createKubernetesClient 创建Kubernetes客户端
func createKubernetesClient() (kubernetes.Interface, error) {
	var cfg *rest.Config
	var err error

	if *kubeconfig != "" {
		// 使用指定的kubeconfig文件
		klog.Infof("Using kubeconfig: %s", *kubeconfig)
		cfg, err = clientcmd.BuildConfigFromFlags("", *kubeconfig)
	} else {
		// 尝试in-cluster配置
		cfg, err = rest.InClusterConfig()
		if err != nil {
			// 如果in-cluster配置失败，尝试默认的kubeconfig位置
			if home := homedir.HomeDir(); home != "" {
				defaultKubeconfig := filepath.Join(home, ".kube", "config")
				if _, err := os.Stat(defaultKubeconfig); err == nil {
					klog.Infof("Using default kubeconfig: %s", defaultKubeconfig)
					cfg, err = clientcmd.BuildConfigFromFlags("", defaultKubeconfig)
				}
			}
		} else {
			klog.Infof("Using in-cluster configuration")
		}
	}

	if err != nil {
		return nil, fmt.Errorf("failed to create kubernetes config: %v", err)
	}

	// 设置客户端配置
	cfg.QPS = 50
	cfg.Burst = 100

	client, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create kubernetes client: %v", err)
	}

	// 测试连接
	ctx, cancel := context.WithTimeout(context.Background(), 10)
	defer cancel()

	_, err = client.CoreV1().Nodes().List(ctx, metav1.ListOptions{Limit: 1})
	if err != nil {
		return nil, fmt.Errorf("failed to connect to kubernetes cluster: %v", err)
	}

	return client, nil
}

// printHelp 输出帮助信息
func printHelp() {
	fmt.Printf(`%s %s - 轻量级 Kubernetes 重调度器

用法:
  %s [选项]

选项:
  -config string
      配置文件路径 (默认查找 ./config.yaml 或 /etc/descheduler/config.yaml)
  -kubeconfig string
      kubeconfig 文件路径 (默认使用 in-cluster 配置或 ~/.kube/config)
  -log-level string
      日志级别 0-5 (默认: "2")
  -version
      显示版本信息
  -help
      显示此帮助信息

示例:
  # 使用默认配置运行
  %s

  # 指定配置文件
  %s -config /path/to/config.yaml

  # 指定kubeconfig和日志级别
  %s -kubeconfig ~/.kube/config -log-level 3

配置文件示例请参考 configs/config.yaml

更多信息请访问: https://github.com/your-org/lightweight-descheduler
`, appName, version, appName, appName, appName, appName)
}
