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
	MaxConn      int           `mapstructure:"max_conn" env:"SERVER_MAX_CONN"`
	GracePeriod  time.Duration `mapstructure:"grace_period" env:"SERVER_GRACE_PERIOD"`
}

// PostgreSQL控制平面配置 (Control Plane)
type PostgresConfig struct {
	Host            string        `mapstructure:"host" env:"PG_HOST"`
	Port            int           `mapstructure:"port" env:"PG_PORT"`
	User            string        `mapstructure:"user" env:"PG_USER"`
	Password        string        `mapstructure:"-" env:"PG_PASSWORD"` // 敏感字段，不从配置文件读取
	Database        string        `mapstructure:"database" env:"PG_DATABASE"`
	SSLMode         string        `mapstructure:"ssl_mode" env:"PG_SSL_MODE"`
	MaxOpenConns    int           `mapstructure:"max_open_conns" env:"PG_MAX_OPEN_CONNS"`
	MaxIdleConns    int           `mapstructure:"max_idle_conns" env:"PG_MAX_IDLE_CONNS"`
	ConnMaxLifetime time.Duration `mapstructure:"conn_max_lifetime" env:"PG_CONN_MAX_LIFETIME"`
	MigrationPath   string        `mapstructure:"migration_path" env:"PG_MIGRATION_PATH"`
}

// ClickHouse证据平面配置 (Evidence Plane)
type ClickHouseConfig struct {
	Host         string `mapstructure:"host" env:"CH_HOST"`
	Port         int    `mapstructure:"port" env:"CH_PORT"`
	User         string `mapstructure:"user" env:"CH_USER"`
	Password     string `mapstructure:"-" env:"CH_PASSWORD"` // 敏感字段，不从配置文件读取
	Database     string `mapstructure:"database" env:"CH_DATABASE"`
	Cluster      string `mapstructure:"cluster" env:"CH_CLUSTER"`
	Secure       bool   `mapstructure:"secure" env:"CH_SECURE"`
	Compression  bool   `mapstructure:"compression" env:"CH_COMPRESSION"`
	MaxOpenConns int    `mapstructure:"max_open_conns" env:"CH_MAX_OPEN_CONNS"`
	MaxIdleConns int    `mapstructure:"max_idle_conns" env:"CH_MAX_IDLE_CONNS"`
}

// Prometheus信号平面配置 (Signal Plane)
type PrometheusConfig struct {
	Address          string        `mapstructure:"address" env:"PROMETHEUS_ADDRESS"`
	QueryTimeout     time.Duration `mapstructure:"query_timeout" env:"PROMETHEUS_QUERY_TIMEOUT"`
	MaxQueryRange    time.Duration `mapstructure:"max_query_range" env:"PROMETHEUS_MAX_QUERY_RANGE"`
	StepInterval     time.Duration `mapstructure:"step_interval" env:"PROMETHEUS_STEP_INTERVAL"`
	QueryConcurrency int           `mapstructure:"query_concurrency" env:"PROMETHEUS_QUERY_CONCURRENCY"`
	BearerToken      string        `mapstructure:"-" env:"PROMETHEUS_BEARER_TOKEN"` // 敏感字段
	SkipTLSVerify    bool          `mapstructure:"skip_tls_verify" env:"PROMETHEUS_SKIP_TLS_VERIFY"`
}

// Kubernetes配置
type KubernetesConfig struct {
	APIServer       string `mapstructure:"api_server" env:"K8S_API_SERVER"`
	Namespace       string `mapstructure:"namespace" env:"K8S_NAMESPACE"`
	ServiceAccount  string `mapstructure:"service_account" env:"K8S_SERVICE_ACCOUNT"`
	BearerTokenFile string `mapstructure:"bearer_token_file" env:"K8S_BEARER_TOKEN_FILE"`
	InCluster       bool   `mapstructure:"in_cluster" env:"K8S_IN_CLUSTER"`
	RBAC            struct {
		Enabled         bool `mapstructure:"enabled" env:"K8S_RBAC_ENABLED"`
		ReadOnlyAccess  bool `mapstructure:"read_only_access" env:"K8S_READ_ONLY_ACCESS"`
		NamespaceScoped bool `mapstructure:"namespace_scoped" env:"K8S_NAMESPACE_SCOPED"`
	} `mapstructure:"rbac"`
}

// Analysis Engine配置
type AnalysisEngineConfig struct {
	Address       string        `mapstructure:"address" env:"ANALYSIS_ENGINE_ADDRESS"`
	Timeout       time.Duration `mapstructure:"timeout" env:"ANALYSIS_ENGINE_TIMEOUT"`
	APIKey        string        `mapstructure:"-" env:"ANALYSIS_ENGINE_API_KEY"` // 敏感字段
	MaxRetries    int           `mapstructure:"max_retries" env:"ANALYSIS_ENGINE_MAX_RETRIES"`
	RetryDelay    time.Duration `mapstructure:"retry_delay" env:"ANALYSIS_ENGINE_RETRY_DELAY"`
	EnableTracing bool          `mapstructure:"enable_tracing" env:"ANALYSIS_ENGINE_ENABLE_TRACING"`
}

// 数据保留策略配置
type RetentionConfig struct {
	// PostgreSQL控制平面保留策略
	Postgres struct {
		Incidents      time.Duration `mapstructure:"incidents" env:"RETENTION_PG_INCIDENTS"`             // 故障快照元数据
		DailySnapshots time.Duration `mapstructure:"daily_snapshots" env:"RETENTION_PG_DAILY_SNAPSHOTS"` // 日报
		CostHistory    time.Duration `mapstructure:"cost_history" env:"RETENTION_PG_COST_HISTORY"`       // 成本历史
	} `mapstructure:"postgres"`

	// ClickHouse证据平面保留策略
	ClickHouse struct {
		ErrorLogs   time.Duration `mapstructure:"error_logs" env:"RETENTION_CH_ERROR_LOGS"`     // 错误日志
		SampledLogs time.Duration `mapstructure:"sampled_logs" env:"RETENTION_CH_SAMPLED_LOGS"` // 采样日志
		TraceData   time.Duration `mapstructure:"trace_data" env:"RETENTION_CH_TRACE_DATA"`     // Trace数据
		AccessLogs  time.Duration `mapstructure:"access_logs" env:"RETENTION_CH_ACCESS_LOGS"`   // 访问日志
	} `mapstructure:"clickhouse"`
}

// 业务配置
type BusinessConfig struct {
	CostCalculation struct {
		CPUPricePerCoreHour  float64       `mapstructure:"cpu_price_per_core_hour" env:"COST_CPU_PRICE"`
		MemPricePerGBHour    float64       `mapstructure:"mem_price_per_gb_hour" env:"COST_MEM_PRICE"`
		CalculationInterval  time.Duration `mapstructure:"calculation_interval" env:"COST_CALCULATION_INTERVAL"`
		AggregationLevels    []string      `mapstructure:"aggregation_levels" env:"COST_AGGREGATION_LEVELS"`
		EfficiencyThresholds struct {
			Zombie          float64 `mapstructure:"zombie" env:"COST_EFFICIENCY_ZOMBIE_THRESHOLD"`
			OverProvisioned float64 `mapstructure:"over_provisioned" env:"COST_EFFICIENCY_OVER_PROVISIONED_THRESHOLD"`
			Healthy         float64 `mapstructure:"healthy" env:"COST_EFFICIENCY_HEALTHY_THRESHOLD"`
			Danger          float64 `mapstructure:"danger" env:"COST_EFFICIENCY_DANGER_THRESHOLD"`
		} `mapstructure:"efficiency_thresholds"`
	} `mapstructure:"cost_calculation"`

	SLO struct {
		AvailabilityThreshold float64       `mapstructure:"availability_threshold" env:"SLO_AVAILABILITY_THRESHOLD"`
		LatencyP95Threshold   int           `mapstructure:"latency_p95_threshold" env:"SLO_LATENCY_P95_THRESHOLD_MS"`
		SnapshotWindow        time.Duration `mapstructure:"snapshot_window" env:"SLO_SNAPSHOT_WINDOW"`
		TriggerDelay          time.Duration `mapstructure:"trigger_delay" env:"SLO_TRIGGER_DELAY"`
		EvidenceCollection    struct {
			UserImpact      bool `mapstructure:"user_impact" env:"SLO_EVIDENCE_USER_IMPACT"`
			ChangeEvents    bool `mapstructure:"change_events" env:"SLO_EVIDENCE_CHANGE_EVENTS"`
			ResourceMetrics bool `mapstructure:"resource_metrics" env:"SLO_EVIDENCE_RESOURCE_METRICS"`
		} `mapstructure:"evidence_collection"`
	} `mapstructure:"slo"`

	ROI struct {
		BaselineDate      string        `mapstructure:"baseline_date" env:"ROI_BASELINE_DATE"`
		TrackingFrequency time.Duration `mapstructure:"tracking_frequency" env:"ROI_TRACKING_FREQUENCY"`
		Metrics           []string      `mapstructure:"metrics" env:"ROI_METRICS"`
	} `mapstructure:"roi"`
}

// 安全配置
type SecurityConfig struct {
	ResourceLimits struct {
		CPULimit       string `mapstructure:"cpu_limit" env:"SECURITY_CPU_LIMIT"`
		MemoryLimit    string `mapstructure:"memory_limit" env:"SECURITY_MEMORY_LIMIT"`
		MaxConnections int    `mapstructure:"max_connections" env:"SECURITY_MAX_CONNECTIONS"`
	} `mapstructure:"resource_limits"`

	RateLimiting struct {
		PrometheusQueriesPerMinute int `mapstructure:"prometheus_queries_per_minute" env:"SECURITY_PROMETHEUS_QUERIES_PER_MINUTE"`
		K8SAPICallsPerMinute       int `mapstructure:"k8s_api_calls_per_minute" env:"SECURITY_K8S_API_CALLS_PER_MINUTE"`
		DatabaseQueriesPerMinute   int `mapstructure:"database_queries_per_minute" env:"SECURITY_DATABASE_QUERIES_PER_MINUTE"`
	} `mapstructure:"rate_limiting"`

	Encryption struct {
		EnableDataEncryption bool   `mapstructure:"enable_data_encryption" env:"SECURITY_ENABLE_DATA_ENCRYPTION"`
		EncryptionKey        string `mapstructure:"-" env:"SECURITY_ENCRYPTION_KEY"` // 敏感字段
	} `mapstructure:"encryption"`
}

// Config 应用总配置
type Config struct {
	Env            Environment          `mapstructure:"env" env:"ENV"`
	Server         ServerConfig         `mapstructure:"server"`
	Postgres       PostgresConfig       `mapstructure:"postgres"`
	ClickHouse     ClickHouseConfig     `mapstructure:"clickhouse"`
	Prometheus     PrometheusConfig     `mapstructure:"prometheus"`
	Kubernetes     KubernetesConfig     `mapstructure:"kubernetes"`
	AnalysisEngine AnalysisEngineConfig `mapstructure:"analysis_engine"`
	Retention      RetentionConfig      `mapstructure:"retention"`
	Business       BusinessConfig       `mapstructure:"business"`
	Security       SecurityConfig       `mapstructure:"security"`
}
