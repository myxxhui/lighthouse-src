package config

import (
	"fmt"
	"regexp"
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
		if cfg.Server.LogLevel == "debug" {
			return fmt.Errorf("debug log level not allowed in production")
		}
		if cfg.Kubernetes.RBAC.Enabled == false {
			return fmt.Errorf("RBAC must be enabled in production")
		}
	}

	// 数据库配置验证
	if cfg.Database.Host == "" {
		return fmt.Errorf("database host is required")
	}

	// Prometheus地址验证
	if cfg.Prometheus.Address != "" {
		if !isValidURL(cfg.Prometheus.Address) {
			return fmt.Errorf("invalid Prometheus address format")
		}
	}

	// 业务配置验证
	if cfg.Business.CostCalculation.CPUPricePerCoreHour <= 0 {
		return fmt.Errorf("CPU price must be positive")
	}

	return nil
}

func isValidURL(url string) bool {
	r, _ := regexp.Compile(`^(http|https)://[a-zA-Z0-9.-]+(:[0-9]+)?(/.*)?$`)
	return r.MatchString(url)
}
