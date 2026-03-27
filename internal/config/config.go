package config

import (
	"os"
	"strings"
	"time"
)

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

// CloudBillingETLData 云账单 ETL 拉取与保留月数。[Ref: 01_实践 月源数据近5年；16_ §5.4]
// 由配置文件或环境变量控制，避免硬编码。
type CloudBillingETLData struct {
	DailyPullMonths    int `mapstructure:"daily_pull_months" env:"BILLING_DAILY_PULL_MONTHS"`       // 日表拉取月数（全量回填时拉取最近 N 个月）
	DailyRetentionMonths int `mapstructure:"daily_retention_months" env:"BILLING_DAILY_RETENTION_MONTHS"` // 日表保留月数，超期删除
	MonthlyPullMonths  int `mapstructure:"monthly_pull_months" env:"BILLING_MONTHLY_PULL_MONTHS"`   // 月表拉取月数，每次更新全量对比拉取
	MonthlyRetentionMonths int `mapstructure:"monthly_retention_months" env:"BILLING_MONTHLY_RETENTION_MONTHS"` // 月表保留月数，超期删除
}

// CloudBillingConfig 云账单配置（15_ 规范）。凭证仅由环境变量或 K8s Secret 注入，不落配置文件。
// 环境变量：CLOUD_BILLING_PROVIDER、CLOUD_BILLING_ENDPOINT、CLOUD_BILLING_PERIOD、CLOUD_BILLING_CYCLE；
// 阿里云 AK/SK：ALIBABA_CLOUD_ACCESS_KEY_ID、ALIBABA_CLOUD_ACCESS_KEY_SECRET（或 K8s Secret 注入）。
// [Ref: 成本透视实践优化 D2-5] ETL/聚合执行时间可配置，非硬编码。
type CloudBillingConfig struct {
	Provider        string               `mapstructure:"provider" env:"PROVIDER"`                   // "aliyun" | "aws" | ""
	Endpoint        string               `mapstructure:"endpoint" env:"ENDPOINT"`                   // 可选
	PeriodType      string               `mapstructure:"period_type" env:"PERIOD"`                   // "day" | "month"
	BillingCycle    string               `mapstructure:"billing_cycle" env:"CYCLE"`                 // 账期，如 2025-01
	ETLScheduleCron string               `mapstructure:"etl_schedule_cron" env:"ETL_SCHEDULE_CRON"` // ETL 执行时间：cron 表达式，默认 "0 1 * * *"（每日 01:00）
	ETLData         CloudBillingETLData  `mapstructure:"etl_data"`                                 // 日/月拉取与保留月数
}

// EffectiveETLScheduleCron 返回 ETL 执行 cron；空时返回默认每日 01:00。供 CronJob 或调度器使用。
func (c CloudBillingConfig) EffectiveETLScheduleCron() string {
	if c.ETLScheduleCron != "" {
		return c.ETLScheduleCron
	}
	return "0 1 * * *"
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
	// FinOpsCGSource 五维 C/G 默认数据源：oss | api；可被 finops_cg_source_by_env 或 FINOPS_CG_SOURCE_<ENV> 覆盖。[Ref: 03_Phase6/01_FinOps FINOPS_CG_SOURCE]
	FinOpsCGSource string `mapstructure:"finops_cg_source" env:"FINOPS_CG_SOURCE"`
	// FinOpsCGSourceByEnv 按环境名覆盖 C/G 源（与 cost_env_account_config.environment 一致）；键大小写不敏感。[Ref: 03_Phase6/01_FinOps]
	FinOpsCGSourceByEnv map[string]string `mapstructure:"finops_cg_source_by_env"`
	// FinOpsSyncAuxTimeout 主动同步 Job 中 sync_auxiliary 阶段超时；0 或未设置表示使用 EffectiveFinOpsSyncAuxTimeout 默认（30m）。环境变量 FINOPS_SYNC_AUX_TIMEOUT，Go duration 如 30m、45m。[Ref: 03_Phase6/01_FinOps 主动同步]
	FinOpsSyncAuxTimeout time.Duration `mapstructure:"finops_sync_aux_timeout" env:"FINOPS_SYNC_AUX_TIMEOUT"`
	// FinOpsSyncJobAPIKey 非空时 POST /api/v1/finops/sync-jobs 须带请求头 X-FinOps-Sync-Key 或与之一致的 Bearer；密钥不入仓库。[Ref: 03_Phase6/01_FinOps 主动同步]
	FinOpsSyncJobAPIKey string `mapstructure:"-" env:"FINOPS_SYNC_JOB_API_KEY"`
	Server         ServerConfig         `mapstructure:"server"`
	Postgres       PostgresConfig       `mapstructure:"postgres"`
	ClickHouse     ClickHouseConfig     `mapstructure:"clickhouse"`
	Prometheus     PrometheusConfig     `mapstructure:"prometheus"`
	Kubernetes     KubernetesConfig     `mapstructure:"kubernetes"`
	AnalysisEngine AnalysisEngineConfig `mapstructure:"analysis_engine"`
	Retention      RetentionConfig      `mapstructure:"retention"`
	Business       BusinessConfig       `mapstructure:"business"`
	Security       SecurityConfig       `mapstructure:"security"`
	CloudBilling   CloudBillingConfig   `mapstructure:"cloud_billing"`
}

// EffectiveFinOpsCGSource 返回小写 oss|api；空或非法时回退为 oss（与部署默认一致）。[Ref: 03_Phase6/01_FinOps]
func EffectiveFinOpsCGSource(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "api":
		return "api"
	case "oss", "":
		return "oss"
	default:
		return "oss"
	}
}

// BuildFinOpsCGSourceByEnvMap 合并配置中的按环境覆盖并规范为大写环境名 → oss|api。[Ref: 03_Phase6/01_FinOps]
func BuildFinOpsCGSourceByEnvMap(m map[string]string) map[string]string {
	if len(m) == 0 {
		return nil
	}
	out := make(map[string]string)
	for k, v := range m {
		kk := strings.ToUpper(strings.TrimSpace(k))
		if kk == "" {
			continue
		}
		out[kk] = EffectiveFinOpsCGSource(v)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// parseFinOpsCGSourceByEnvFromEnviron 解析 FINOPS_CG_SOURCE_<ENV>（ENV 任意非空后缀，键大写）。[Ref: 03_Phase6/01_FinOps]
func parseFinOpsCGSourceByEnvFromEnviron(environ []string) map[string]string {
	prefix := "FINOPS_CG_SOURCE_"
	out := make(map[string]string)
	for _, kv := range environ {
		idx := strings.IndexByte(kv, '=')
		if idx <= 0 {
			continue
		}
		key := kv[:idx]
		if !strings.HasPrefix(key, prefix) {
			continue
		}
		suffix := strings.TrimSpace(key[len(prefix):])
		if suffix == "" {
			continue
		}
		val := kv[idx+1:]
		out[strings.ToUpper(suffix)] = EffectiveFinOpsCGSource(val)
	}
	return out
}

// mergeFinOpsCGFromEnvironSlice 将 parseFinOpsCGSourceByEnvFromEnviron 结果合并入 cfg（覆盖同名键）。[Ref: 03_Phase6/01_FinOps]
func mergeFinOpsCGFromEnvironSlice(cfg *Config, environ []string) {
	if cfg == nil {
		return
	}
	m := parseFinOpsCGSourceByEnvFromEnviron(environ)
	if len(m) == 0 {
		return
	}
	if cfg.FinOpsCGSourceByEnv == nil {
		cfg.FinOpsCGSourceByEnv = make(map[string]string)
	}
	for k, v := range m {
		cfg.FinOpsCGSourceByEnv[k] = v
	}
}

// MergeFinOpsCGFromEnviron 将 FINOPS_CG_SOURCE_<ENV> 合并入 cfg（覆盖同名键）。须在 YAML 加载之后调用。[Ref: 03_Phase6/01_FinOps]
func MergeFinOpsCGFromEnviron(cfg *Config) {
	mergeFinOpsCGFromEnvironSlice(cfg, os.Environ())
}

// EffectiveFinOpsSyncAuxTimeout 返回 sync_auxiliary 阶段超时；d<=0 时默认 30 分钟。[Ref: 03_Phase6/01_FinOps 主动同步]
func EffectiveFinOpsSyncAuxTimeout(d time.Duration) time.Duration {
	if d <= 0 {
		return 30 * time.Minute
	}
	return d
}
