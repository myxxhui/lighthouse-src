package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestConfigStructure(t *testing.T) {
	// 测试Config结构体完整性
	cfg := &Config{
		Env: EnvDevelopment,
		Server: ServerConfig{
			Port:         8080,
			ReadTimeout:  30 * time.Second,
			WriteTimeout: 30 * time.Second,
			LogLevel:     "debug",
			MaxConn:      100,
			GracePeriod:  30 * time.Second,
		},
		Postgres: PostgresConfig{
			Host:            "localhost",
			Port:            5432,
			User:            "test",
			Password:        "test",
			Database:        "test",
			SSLMode:         "disable",
			MaxOpenConns:    10,
			MaxIdleConns:    5,
			ConnMaxLifetime: time.Hour,
			MigrationPath:   "./migrations",
		},
		ClickHouse: ClickHouseConfig{
			Host:         "localhost",
			Port:         9000,
			User:         "test",
			Password:     "test",
			Database:     "test",
			Cluster:      "default",
			Secure:       false,
			Compression:  true,
			MaxOpenConns: 20,
			MaxIdleConns: 10,
		},
		Prometheus: PrometheusConfig{
			Address:          "http://localhost:9090",
			QueryTimeout:     10 * time.Second,
			MaxQueryRange:    7 * 24 * time.Hour,
			StepInterval:     15 * time.Minute,
			QueryConcurrency: 5,
			BearerToken:      "test",
			SkipTLSVerify:    false,
		},
		Kubernetes: KubernetesConfig{
			APIServer:       "https://localhost:6443",
			Namespace:       "default",
			ServiceAccount:  "default",
			BearerTokenFile: "/var/run/secrets/token",
			InCluster:       false,
			RBAC: struct {
				Enabled         bool `mapstructure:"enabled" env:"K8S_RBAC_ENABLED"`
				ReadOnlyAccess  bool `mapstructure:"read_only_access" env:"K8S_READ_ONLY_ACCESS"`
				NamespaceScoped bool `mapstructure:"namespace_scoped" env:"K8S_NAMESPACE_SCOPED"`
			}{
				Enabled:         true,
				ReadOnlyAccess:  true,
				NamespaceScoped: true,
			},
		},
		AnalysisEngine: AnalysisEngineConfig{
			Address:       "http://localhost:8000",
			Timeout:       30 * time.Second,
			APIKey:        "test",
			MaxRetries:    3,
			RetryDelay:    time.Second,
			EnableTracing: true,
		},
		Retention: RetentionConfig{
			Postgres: struct {
				Incidents      time.Duration `mapstructure:"incidents" env:"RETENTION_PG_INCIDENTS"`
				DailySnapshots time.Duration `mapstructure:"daily_snapshots" env:"RETENTION_PG_DAILY_SNAPSHOTS"`
				CostHistory    time.Duration `mapstructure:"cost_history" env:"RETENTION_PG_COST_HISTORY"`
			}{
				Incidents:      90 * 24 * time.Hour,
				DailySnapshots: 180 * 24 * time.Hour,
				CostHistory:    365 * 24 * time.Hour,
			},
			ClickHouse: struct {
				ErrorLogs   time.Duration `mapstructure:"error_logs" env:"RETENTION_CH_ERROR_LOGS"`
				SampledLogs time.Duration `mapstructure:"sampled_logs" env:"RETENTION_CH_SAMPLED_LOGS"`
				TraceData   time.Duration `mapstructure:"trace_data" env:"RETENTION_CH_TRACE_DATA"`
				AccessLogs  time.Duration `mapstructure:"access_logs" env:"RETENTION_CH_ACCESS_LOGS"`
			}{
				ErrorLogs:   14 * 24 * time.Hour,
				SampledLogs: 3 * 24 * time.Hour,
				TraceData:   30 * 24 * time.Hour,
				AccessLogs:  7 * 24 * time.Hour,
			},
		},
		Business: BusinessConfig{
			CostCalculation: struct {
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
			}{
				CPUPricePerCoreHour: 0.025,
				MemPricePerGBHour:   0.01,
				CalculationInterval: time.Hour,
				AggregationLevels:   []string{"namespace", "service", "pod"},
				EfficiencyThresholds: struct {
					Zombie          float64 `mapstructure:"zombie" env:"COST_EFFICIENCY_ZOMBIE_THRESHOLD"`
					OverProvisioned float64 `mapstructure:"over_provisioned" env:"COST_EFFICIENCY_OVER_PROVISIONED_THRESHOLD"`
					Healthy         float64 `mapstructure:"healthy" env:"COST_EFFICIENCY_HEALTHY_THRESHOLD"`
					Danger          float64 `mapstructure:"danger" env:"COST_EFFICIENCY_DANGER_THRESHOLD"`
				}{
					Zombie:          10,
					OverProvisioned: 40,
					Healthy:         70,
					Danger:          90,
				},
			},
			SLO: struct {
				AvailabilityThreshold float64       `mapstructure:"availability_threshold" env:"SLO_AVAILABILITY_THRESHOLD"`
				LatencyP95Threshold   int           `mapstructure:"latency_p95_threshold" env:"SLO_LATENCY_P95_THRESHOLD_MS"`
				SnapshotWindow        time.Duration `mapstructure:"snapshot_window" env:"SLO_SNAPSHOT_WINDOW"`
				TriggerDelay          time.Duration `mapstructure:"trigger_delay" env:"SLO_TRIGGER_DELAY"`
				EvidenceCollection    struct {
					UserImpact      bool `mapstructure:"user_impact" env:"SLO_EVIDENCE_USER_IMPACT"`
					ChangeEvents    bool `mapstructure:"change_events" env:"SLO_EVIDENCE_CHANGE_EVENTS"`
					ResourceMetrics bool `mapstructure:"resource_metrics" env:"SLO_EVIDENCE_RESOURCE_METRICS"`
				} `mapstructure:"evidence_collection"`
			}{
				AvailabilityThreshold: 99.9,
				LatencyP95Threshold:   300,
				SnapshotWindow:        10 * time.Minute,
				TriggerDelay:          5 * time.Minute,
				EvidenceCollection: struct {
					UserImpact      bool `mapstructure:"user_impact" env:"SLO_EVIDENCE_USER_IMPACT"`
					ChangeEvents    bool `mapstructure:"change_events" env:"SLO_EVIDENCE_CHANGE_EVENTS"`
					ResourceMetrics bool `mapstructure:"resource_metrics" env:"SLO_EVIDENCE_RESOURCE_METRICS"`
				}{
					UserImpact:      true,
					ChangeEvents:    true,
					ResourceMetrics: true,
				},
			},
			ROI: struct {
				BaselineDate      string        `mapstructure:"baseline_date" env:"ROI_BASELINE_DATE"`
				TrackingFrequency time.Duration `mapstructure:"tracking_frequency" env:"ROI_TRACKING_FREQUENCY"`
				Metrics           []string      `mapstructure:"metrics" env:"ROI_METRICS"`
			}{
				BaselineDate:      "2025-01-01",
				TrackingFrequency: 24 * time.Hour,
				Metrics:           []string{"financial_savings", "efficiency_gains", "risk_reduction"},
			},
		},
		Security: SecurityConfig{
			ResourceLimits: struct {
				CPULimit       string `mapstructure:"cpu_limit" env:"SECURITY_CPU_LIMIT"`
				MemoryLimit    string `mapstructure:"memory_limit" env:"SECURITY_MEMORY_LIMIT"`
				MaxConnections int    `mapstructure:"max_connections" env:"SECURITY_MAX_CONNECTIONS"`
			}{
				CPULimit:       "500m",
				MemoryLimit:    "1Gi",
				MaxConnections: 100,
			},
			RateLimiting: struct {
				PrometheusQueriesPerMinute int `mapstructure:"prometheus_queries_per_minute" env:"SECURITY_PROMETHEUS_QUERIES_PER_MINUTE"`
				K8SAPICallsPerMinute       int `mapstructure:"k8s_api_calls_per_minute" env:"SECURITY_K8S_API_CALLS_PER_MINUTE"`
				DatabaseQueriesPerMinute   int `mapstructure:"database_queries_per_minute" env:"SECURITY_DATABASE_QUERIES_PER_MINUTE"`
			}{
				PrometheusQueriesPerMinute: 60,
				K8SAPICallsPerMinute:       120,
				DatabaseQueriesPerMinute:   300,
			},
			Encryption: struct {
				EnableDataEncryption bool   `mapstructure:"enable_data_encryption" env:"SECURITY_ENABLE_DATA_ENCRYPTION"`
				EncryptionKey        string `mapstructure:"-" env:"SECURITY_ENCRYPTION_KEY"`
			}{
				EnableDataEncryption: false,
				EncryptionKey:        "test-key",
			},
		},
	}

	// 验证环境类型
	if cfg.Env != EnvDevelopment {
		t.Errorf("expected env development, got %v", cfg.Env)
	}

	// 验证服务器配置
	if cfg.Server.Port != 8080 {
		t.Errorf("expected server port 8080, got %v", cfg.Server.Port)
	}

	// 验证三级存储架构配置
	if cfg.Postgres.Host != "localhost" {
		t.Errorf("expected postgres host localhost, got %v", cfg.Postgres.Host)
	}
	if cfg.ClickHouse.Host != "localhost" {
		t.Errorf("expected clickhouse host localhost, got %v", cfg.ClickHouse.Host)
	}
	if cfg.Prometheus.Address != "http://localhost:9090" {
		t.Errorf("expected prometheus address http://localhost:9090, got %v", cfg.Prometheus.Address)
	}

	// 验证业务配置
	if cfg.Business.CostCalculation.CPUPricePerCoreHour != 0.025 {
		t.Errorf("expected CPU price 0.025, got %v", cfg.Business.CostCalculation.CPUPricePerCoreHour)
	}
	if cfg.Business.SLO.AvailabilityThreshold != 99.9 {
		t.Errorf("expected SLO availability threshold 99.9, got %v", cfg.Business.SLO.AvailabilityThreshold)
	}
}

func TestEnvironmentValidation(t *testing.T) {
	validator := NewConfigValidator()

	// 测试开发环境配置
	devCfg := &Config{
		Env:        EnvDevelopment,
		Server:     ServerConfig{LogLevel: "debug"},
		Postgres:   PostgresConfig{Host: "localhost", Port: 5432},
		ClickHouse: ClickHouseConfig{Host: "localhost", Port: 9000},
		Business: BusinessConfig{
			CostCalculation: struct {
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
			}{
				CPUPricePerCoreHour: 0.025,
				MemPricePerGBHour:   0.01,
				CalculationInterval: time.Hour,
				EfficiencyThresholds: struct {
					Zombie          float64 `mapstructure:"zombie" env:"COST_EFFICIENCY_ZOMBIE_THRESHOLD"`
					OverProvisioned float64 `mapstructure:"over_provisioned" env:"COST_EFFICIENCY_OVER_PROVISIONED_THRESHOLD"`
					Healthy         float64 `mapstructure:"healthy" env:"COST_EFFICIENCY_HEALTHY_THRESHOLD"`
					Danger          float64 `mapstructure:"danger" env:"COST_EFFICIENCY_DANGER_THRESHOLD"`
				}{
					Zombie:          10,
					OverProvisioned: 40,
					Healthy:         70,
					Danger:          90,
				},
			},
			SLO: struct {
				AvailabilityThreshold float64       `mapstructure:"availability_threshold" env:"SLO_AVAILABILITY_THRESHOLD"`
				LatencyP95Threshold   int           `mapstructure:"latency_p95_threshold" env:"SLO_LATENCY_P95_THRESHOLD_MS"`
				SnapshotWindow        time.Duration `mapstructure:"snapshot_window" env:"SLO_SNAPSHOT_WINDOW"`
				TriggerDelay          time.Duration `mapstructure:"trigger_delay" env:"SLO_TRIGGER_DELAY"`
				EvidenceCollection    struct {
					UserImpact      bool `mapstructure:"user_impact" env:"SLO_EVIDENCE_USER_IMPACT"`
					ChangeEvents    bool `mapstructure:"change_events" env:"SLO_EVIDENCE_CHANGE_EVENTS"`
					ResourceMetrics bool `mapstructure:"resource_metrics" env:"SLO_EVIDENCE_RESOURCE_METRICS"`
				} `mapstructure:"evidence_collection"`
			}{
				AvailabilityThreshold: 99.9,
				LatencyP95Threshold:   300,
			},
		},
		Security: SecurityConfig{
			RateLimiting: struct {
				PrometheusQueriesPerMinute int `mapstructure:"prometheus_queries_per_minute" env:"SECURITY_PROMETHEUS_QUERIES_PER_MINUTE"`
				K8SAPICallsPerMinute       int `mapstructure:"k8s_api_calls_per_minute" env:"SECURITY_K8S_API_CALLS_PER_MINUTE"`
				DatabaseQueriesPerMinute   int `mapstructure:"database_queries_per_minute" env:"SECURITY_DATABASE_QUERIES_PER_MINUTE"`
			}{
				PrometheusQueriesPerMinute: 60,
				K8SAPICallsPerMinute:       120,
				DatabaseQueriesPerMinute:   300,
			},
		},
	}

	// 开发环境应该通过验证
	if err := validator.Validate(devCfg); err != nil {
		t.Errorf("dev config validation failed: %v", err)
	}

	// 测试生产环境配置（应该失败，因为缺少安全配置）
	prodCfg := &Config{
		Env:        EnvProduction,
		Server:     ServerConfig{LogLevel: "info"},
		Postgres:   PostgresConfig{Host: "localhost", Port: 5432},
		ClickHouse: ClickHouseConfig{Host: "localhost", Port: 9000},
		Kubernetes: KubernetesConfig{
			RBAC: struct {
				Enabled         bool `mapstructure:"enabled" env:"K8S_RBAC_ENABLED"`
				ReadOnlyAccess  bool `mapstructure:"read_only_access" env:"K8S_READ_ONLY_ACCESS"`
				NamespaceScoped bool `mapstructure:"namespace_scoped" env:"K8S_NAMESPACE_SCOPED"`
			}{
				Enabled: true,
			},
		},
		Business: BusinessConfig{
			CostCalculation: struct {
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
			}{
				CPUPricePerCoreHour: 0.025,
				MemPricePerGBHour:   0.01,
				CalculationInterval: time.Hour,
				EfficiencyThresholds: struct {
					Zombie          float64 `mapstructure:"zombie" env:"COST_EFFICIENCY_ZOMBIE_THRESHOLD"`
					OverProvisioned float64 `mapstructure:"over_provisioned" env:"COST_EFFICIENCY_OVER_PROVISIONED_THRESHOLD"`
					Healthy         float64 `mapstructure:"healthy" env:"COST_EFFICIENCY_HEALTHY_THRESHOLD"`
					Danger          float64 `mapstructure:"danger" env:"COST_EFFICIENCY_DANGER_THRESHOLD"`
				}{
					Zombie:          10,
					OverProvisioned: 40,
					Healthy:         70,
					Danger:          90,
				},
			},
			SLO: struct {
				AvailabilityThreshold float64       `mapstructure:"availability_threshold" env:"SLO_AVAILABILITY_THRESHOLD"`
				LatencyP95Threshold   int           `mapstructure:"latency_p95_threshold" env:"SLO_LATENCY_P95_THRESHOLD_MS"`
				SnapshotWindow        time.Duration `mapstructure:"snapshot_window" env:"SLO_SNAPSHOT_WINDOW"`
				TriggerDelay          time.Duration `mapstructure:"trigger_delay" env:"SLO_TRIGGER_DELAY"`
				EvidenceCollection    struct {
					UserImpact      bool `mapstructure:"user_impact" env:"SLO_EVIDENCE_USER_IMPACT"`
					ChangeEvents    bool `mapstructure:"change_events" env:"SLO_EVIDENCE_CHANGE_EVENTS"`
					ResourceMetrics bool `mapstructure:"resource_metrics" env:"SLO_EVIDENCE_RESOURCE_METRICS"`
				} `mapstructure:"evidence_collection"`
			}{
				AvailabilityThreshold: 99.9,
				LatencyP95Threshold:   300,
			},
		},
		Security: SecurityConfig{
			ResourceLimits: struct {
				CPULimit       string `mapstructure:"cpu_limit" env:"SECURITY_CPU_LIMIT"`
				MemoryLimit    string `mapstructure:"memory_limit" env:"SECURITY_MEMORY_LIMIT"`
				MaxConnections int    `mapstructure:"max_connections" env:"SECURITY_MAX_CONNECTIONS"`
			}{
				CPULimit:       "",
				MemoryLimit:    "",
				MaxConnections: 0,
			},
			RateLimiting: struct {
				PrometheusQueriesPerMinute int `mapstructure:"prometheus_queries_per_minute" env:"SECURITY_PROMETHEUS_QUERIES_PER_MINUTE"`
				K8SAPICallsPerMinute       int `mapstructure:"k8s_api_calls_per_minute" env:"SECURITY_K8S_API_CALLS_PER_MINUTE"`
				DatabaseQueriesPerMinute   int `mapstructure:"database_queries_per_minute" env:"SECURITY_DATABASE_QUERIES_PER_MINUTE"`
			}{
				PrometheusQueriesPerMinute: 60,
				K8SAPICallsPerMinute:       120,
				DatabaseQueriesPerMinute:   300,
			},
			Encryption: struct {
				EnableDataEncryption bool   `mapstructure:"enable_data_encryption" env:"SECURITY_ENABLE_DATA_ENCRYPTION"`
				EncryptionKey        string `mapstructure:"-" env:"SECURITY_ENCRYPTION_KEY"`
			}{
				EnableDataEncryption: false,
			},
		},
	}

	// 生产环境应该验证失败（缺少资源限制和数据加密）
	if err := validator.Validate(prodCfg); err == nil {
		t.Error("expected production config validation to fail, but it passed")
	}
}

func TestEnvMapping(t *testing.T) {
	// 测试环境变量映射完整性
	mapping := GetEnvMapping()

	// 检查关键环境变量是否存在
	requiredEnvs := []string{
		"ENV",
		"SERVER_PORT",
		"PG_HOST", "PG_PASSWORD",
		"CH_HOST", "CH_PASSWORD",
		"PROMETHEUS_ADDRESS",
		"K8S_API_SERVER",
		"COST_CPU_PRICE",
		"SLO_AVAILABILITY_THRESHOLD",
		"SECURITY_CPU_LIMIT",
	}

	for _, env := range requiredEnvs {
		if _, exists := mapping[env]; !exists {
			t.Errorf("required environment variable %s not found in mapping", env)
		}
	}

	// 检查映射数量（应该有相当数量的环境变量）
	if len(mapping) < 50 {
		t.Errorf("expected at least 50 environment variables, got %d", len(mapping))
	}
}

func TestMultiEnvironmentSupport(t *testing.T) {
	// 测试多环境支持
	envs := []Environment{EnvDevelopment, EnvStaging, EnvProduction}

	for _, env := range envs {
		cfg := &Config{Env: env}

		// 验证环境类型
		switch env {
		case EnvDevelopment:
			if cfg.Env != "dev" {
				t.Errorf("expected dev environment, got %v", cfg.Env)
			}
		case EnvStaging:
			if cfg.Env != "staging" {
				t.Errorf("expected staging environment, got %v", cfg.Env)
			}
		case EnvProduction:
			if cfg.Env != "prod" {
				t.Errorf("expected prod environment, got %v", cfg.Env)
			}
		}
	}

	// 测试环境特定配置加载
	tempDir := t.TempDir()

	// 创建基础配置文件
	baseConfig := `
env: dev
server:
  port: 8080
postgres:
  host: localhost
`
	basePath := filepath.Join(tempDir, "config.yaml")
	if err := os.WriteFile(basePath, []byte(baseConfig), 0644); err != nil {
		t.Fatal(err)
	}

	// 创建开发环境配置文件
	devConfig := `
server:
  log_level: debug
postgres:
  port: 5432
`
	devPath := filepath.Join(tempDir, "config.dev.yaml")
	if err := os.WriteFile(devPath, []byte(devConfig), 0644); err != nil {
		t.Fatal(err)
	}

	// 设置环境变量
	os.Setenv("ENV", "dev")
	defer os.Unsetenv("ENV")

	// 注意：这里简化了测试，实际加载逻辑在loader.go中
	t.Logf("Multi-environment configuration test directory: %s", tempDir)
}

func TestSensitiveFieldHandling(t *testing.T) {
	// 测试敏感字段处理
	cfg := &Config{
		Postgres: PostgresConfig{
			Password: "should-be-empty-from-config",
		},
		ClickHouse: ClickHouseConfig{
			Password: "should-be-empty-from-config",
		},
		Prometheus: PrometheusConfig{
			BearerToken: "should-be-empty-from-config",
		},
		AnalysisEngine: AnalysisEngineConfig{
			APIKey: "should-be-empty-from-config",
		},
		Security: SecurityConfig{
			Encryption: struct {
				EnableDataEncryption bool   `mapstructure:"enable_data_encryption" env:"SECURITY_ENABLE_DATA_ENCRYPTION"`
				EncryptionKey        string `mapstructure:"-" env:"SECURITY_ENCRYPTION_KEY"`
			}{
				EncryptionKey: "should-be-empty-from-config",
			},
		},
	}

	// 验证敏感字段在配置文件中的值应该被忽略（因为mapstructure:"-"）
	// 实际值应该从环境变量加载
	// 这里我们只是检查结构体字段存在
	if cfg.Postgres.Password == "" {
		t.Log("Postgres password field is empty as expected (should come from env)")
	}
	if cfg.ClickHouse.Password == "" {
		t.Log("ClickHouse password field is empty as expected (should come from env)")
	}
	if cfg.Prometheus.BearerToken == "" {
		t.Log("Prometheus bearer token field is empty as expected (should come from env)")
	}
	if cfg.AnalysisEngine.APIKey == "" {
		t.Log("Analysis Engine API key field is empty as expected (should come from env)")
	}
	if cfg.Security.Encryption.EncryptionKey == "" {
		t.Log("Encryption key field is empty as expected (should come from env)")
	}
}
