package config

import "time"

// Environment 应用环境类型
type Environment string

const (
	EnvDevelopment Environment = "dev"
	EnvStaging     Environment = "staging"
	EnvProduction  Environment = "prod"
)

// ServerConfig 服务器配置
type ServerConfig struct {
	Port         int           `mapstructure:"port" env:"SERVER_PORT"`
	ReadTimeout  time.Duration `mapstructure:"read_timeout" env:"SERVER_READ_TIMEOUT"`
	WriteTimeout time.Duration `mapstructure:"write_timeout" env:"SERVER_WRITE_TIMEOUT"`
	LogLevel     string        `mapstructure:"log_level" env:"LOG_LEVEL"`
}

// DatabaseConfig 数据库配置
type DatabaseConfig struct {
	Host     string `mapstructure:"host" env:"DB_HOST"`
	Port     int    `mapstructure:"port" env:"DB_PORT"`
	User     string `mapstructure:"user" env:"DB_USER"`
	Password string `mapstructure:"-" env:"DB_PASSWORD"` // 敏感字段，不从配置文件读取
	Name     string `mapstructure:"name" env:"DB_NAME"`
	SSLMode  string `mapstructure:"ssl_mode" env:"DB_SSL_MODE"`
}

// PrometheusConfig Prometheus配置
type PrometheusConfig struct {
	Address      string        `mapstructure:"address" env:"PROMETHEUS_ADDRESS"`
	QueryTimeout time.Duration `mapstructure:"query_timeout" env:"PROMETHEUS_QUERY_TIMEOUT"`
}

// KubernetesConfig Kubernetes配置
type KubernetesConfig struct {
	APIServer string `mapstructure:"api_server" env:"K8S_API_SERVER"`
	RBAC      struct {
		Enabled bool `mapstructure:"enabled" env:"K8S_RBAC_ENABLED"`
	} `mapstructure:"rbac"`
}

// BusinessConfig 业务配置
type BusinessConfig struct {
	CostCalculation struct {
		CPUPricePerCoreHour float64 `mapstructure:"cpu_price_per_core_hour" env:"COST_CPU_PRICE"`
		MemPricePerGBHour   float64 `mapstructure:"mem_price_per_gb_hour" env:"COST_MEM_PRICE"`
	} `mapstructure:"cost_calculation"`
	SLOThresholds struct {
		Availability float64 `mapstructure:"availability" env:"SLO_AVAILABILITY_THRESHOLD"`
		Latency      int     `mapstructure:"latency" env:"SLO_LATENCY_THRESHOLD_MS"`
	} `mapstructure:"slo_thresholds"`
}

// Config 应用总配置
type Config struct {
	Env        Environment      `mapstructure:"env" env:"ENV"`
	Server     ServerConfig     `mapstructure:"server"`
	Database   DatabaseConfig   `mapstructure:"database"`
	Prometheus PrometheusConfig `mapstructure:"prometheus"`
	Kubernetes KubernetesConfig `mapstructure:"kubernetes"`
	Business   BusinessConfig   `mapstructure:"business"`
}
