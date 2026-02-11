package config

import (
	"fmt"
	"regexp"
	"strings"
)

// Validator 配置验证接口
type Validator interface {
	Validate(cfg *Config) error
}

// ConfigValidator 配置验证实现
type ConfigValidator struct{}

func NewConfigValidator() Validator {
	return &ConfigValidator{}
}

func (v *ConfigValidator) Validate(cfg *Config) error {
	// 基础验证
	if cfg.Env == "" {
		return fmt.Errorf("environment must be specified")
	}

	// 环境特定规则
	switch cfg.Env {
	case EnvProduction:
		// 生产环境不允许debug日志级别
		if strings.ToLower(cfg.Server.LogLevel) == "debug" {
			return fmt.Errorf("debug log level not allowed in production")
		}
		// 生产环境必须启用RBAC
		if !cfg.Kubernetes.RBAC.Enabled {
			return fmt.Errorf("RBAC must be enabled in production")
		}
		// 生产环境必须启用数据加密
		if !cfg.Security.Encryption.EnableDataEncryption {
			return fmt.Errorf("data encryption must be enabled in production")
		}
		// 生产环境必须有资源限制
		if cfg.Security.ResourceLimits.CPULimit == "" || cfg.Security.ResourceLimits.MemoryLimit == "" {
			return fmt.Errorf("resource limits must be specified in production")
		}
	}

	// PostgreSQL控制平面配置验证
	if cfg.Postgres.Host == "" {
		return fmt.Errorf("postgres host is required for control plane")
	}
	if cfg.Postgres.Port <= 0 || cfg.Postgres.Port > 65535 {
		return fmt.Errorf("postgres port must be valid (1-65535)")
	}

	// ClickHouse证据平面配置验证
	if cfg.ClickHouse.Host == "" {
		return fmt.Errorf("clickhouse host is required for evidence plane")
	}
	if cfg.ClickHouse.Port <= 0 || cfg.ClickHouse.Port > 65535 {
		return fmt.Errorf("clickhouse port must be valid (1-65535)")
	}

	// Prometheus信号平面配置验证
	if cfg.Prometheus.Address != "" {
		if !isValidURL(cfg.Prometheus.Address) {
			return fmt.Errorf("invalid Prometheus address format")
		}
	} else {
		// Prometheus地址是可选的，但建议配置
		fmt.Println("[WARNING] Prometheus address not configured, SLO monitoring will be limited")
	}

	// Analysis Engine配置验证（可选）
	if cfg.AnalysisEngine.Address != "" && !isValidURL(cfg.AnalysisEngine.Address) {
		return fmt.Errorf("invalid Analysis Engine address format")
	}

	// 业务配置验证
	if cfg.Business.CostCalculation.CPUPricePerCoreHour <= 0 {
		return fmt.Errorf("CPU price must be positive")
	}
	if cfg.Business.CostCalculation.MemPricePerGBHour <= 0 {
		return fmt.Errorf("memory price must be positive")
	}
	if cfg.Business.CostCalculation.CalculationInterval <= 0 {
		return fmt.Errorf("cost calculation interval must be positive")
	}

	// SLO配置验证
	if cfg.Business.SLO.AvailabilityThreshold <= 0 || cfg.Business.SLO.AvailabilityThreshold > 100 {
		return fmt.Errorf("SLO availability threshold must be between 0 and 100")
	}
	if cfg.Business.SLO.LatencyP95Threshold <= 0 {
		return fmt.Errorf("SLO latency threshold must be positive")
	}

	// 效率阈值验证
	thresholds := cfg.Business.CostCalculation.EfficiencyThresholds
	if thresholds.Zombie <= 0 || thresholds.Zombie >= 100 {
		return fmt.Errorf("zombie efficiency threshold must be between 0 and 100")
	}
	if thresholds.OverProvisioned <= thresholds.Zombie || thresholds.OverProvisioned >= 100 {
		return fmt.Errorf("over-provisioned efficiency threshold must be between zombie and 100")
	}
	if thresholds.Healthy <= thresholds.OverProvisioned || thresholds.Healthy >= 100 {
		return fmt.Errorf("healthy efficiency threshold must be between over-provisioned and 100")
	}
	if thresholds.Danger <= thresholds.Healthy || thresholds.Danger >= 100 {
		return fmt.Errorf("danger efficiency threshold must be between healthy and 100")
	}

	// 安全配置验证
	if cfg.Security.RateLimiting.PrometheusQueriesPerMinute <= 0 {
		return fmt.Errorf("Prometheus query rate limit must be positive")
	}
	if cfg.Security.RateLimiting.K8SAPICallsPerMinute <= 0 {
		return fmt.Errorf("K8S API call rate limit must be positive")
	}

	return nil
}

func isValidURL(url string) bool {
	r, _ := regexp.Compile(`^(http|https)://[a-zA-Z0-9.-]+(:[0-9]+)?(/.*)?$`)
	return r.MatchString(url)
}
