package config

// GetEnvMapping 获取环境变量映射关系
func GetEnvMapping() map[string]string {
	return map[string]string{
		// 环境标识
		"ENV": "环境类型 (dev/staging/prod)",

		// 服务器配置
		"SERVER_PORT":          "服务端口",
		"SERVER_READ_TIMEOUT":  "服务读取超时",
		"SERVER_WRITE_TIMEOUT": "服务写入超时",
		"LOG_LEVEL":            "日志级别",
		"SERVER_MAX_CONN":      "最大连接数",
		"SERVER_GRACE_PERIOD":  "优雅关闭等待时间",

		// PostgreSQL控制平面配置
		"PG_HOST":              "PostgreSQL主机地址",
		"PG_PORT":              "PostgreSQL端口",
		"PG_USER":              "PostgreSQL用户名",
		"PG_PASSWORD":          "PostgreSQL密码 (敏感信息)",
		"PG_DATABASE":          "PostgreSQL数据库名",
		"PG_SSL_MODE":          "PostgreSQL SSL模式",
		"PG_MAX_OPEN_CONNS":    "PostgreSQL最大打开连接数",
		"PG_MAX_IDLE_CONNS":    "PostgreSQL最大空闲连接数",
		"PG_CONN_MAX_LIFETIME": "PostgreSQL连接最大生命周期",
		"PG_MIGRATION_PATH":    "PostgreSQL迁移文件路径",

		// ClickHouse证据平面配置
		"CH_HOST":           "ClickHouse主机地址",
		"CH_PORT":           "ClickHouse端口",
		"CH_USER":           "ClickHouse用户名",
		"CH_PASSWORD":       "ClickHouse密码 (敏感信息)",
		"CH_DATABASE":       "ClickHouse数据库名",
		"CH_CLUSTER":        "ClickHouse集群名",
		"CH_SECURE":         "ClickHouse安全连接",
		"CH_COMPRESSION":    "ClickHouse压缩",
		"CH_MAX_OPEN_CONNS": "ClickHouse最大打开连接数",
		"CH_MAX_IDLE_CONNS": "ClickHouse最大空闲连接数",

		// Prometheus信号平面配置
		"PROMETHEUS_ADDRESS":           "Prometheus地址",
		"PROMETHEUS_QUERY_TIMEOUT":     "Prometheus查询超时",
		"PROMETHEUS_MAX_QUERY_RANGE":   "Prometheus最大查询范围",
		"PROMETHEUS_STEP_INTERVAL":     "Prometheus步进间隔",
		"PROMETHEUS_QUERY_CONCURRENCY": "Prometheus查询并发数",
		"PROMETHEUS_BEARER_TOKEN":      "Prometheus Bearer Token (敏感信息)",
		"PROMETHEUS_SKIP_TLS_VERIFY":   "Prometheus跳过TLS验证",

		// Kubernetes配置
		"K8S_API_SERVER":        "Kubernetes API服务器地址",
		"K8S_NAMESPACE":         "Kubernetes命名空间",
		"K8S_SERVICE_ACCOUNT":   "Kubernetes服务账户",
		"K8S_BEARER_TOKEN_FILE": "Kubernetes Bearer Token文件路径",
		"K8S_IN_CLUSTER":        "是否在集群内运行",
		"K8S_RBAC_ENABLED":      "是否启用RBAC",
		"K8S_READ_ONLY_ACCESS":  "是否只读访问",
		"K8S_NAMESPACE_SCOPED":  "是否命名空间作用域",

		// Analysis Engine配置
		"ANALYSIS_ENGINE_ADDRESS":        "Analysis Engine地址",
		"ANALYSIS_ENGINE_TIMEOUT":        "Analysis Engine超时",
		"ANALYSIS_ENGINE_API_KEY":        "Analysis Engine API Key (敏感信息)",
		"ANALYSIS_ENGINE_MAX_RETRIES":    "Analysis Engine最大重试次数",
		"ANALYSIS_ENGINE_RETRY_DELAY":    "Analysis Engine重试延迟",
		"ANALYSIS_ENGINE_ENABLE_TRACING": "Analysis Engine启用追踪",

		// 数据保留策略配置
		"RETENTION_PG_INCIDENTS":       "PostgreSQL故障快照保留时间",
		"RETENTION_PG_DAILY_SNAPSHOTS": "PostgreSQL日报保留时间",
		"RETENTION_PG_COST_HISTORY":    "PostgreSQL成本历史保留时间",
		"RETENTION_CH_ERROR_LOGS":      "ClickHouse错误日志保留时间",
		"RETENTION_CH_SAMPLED_LOGS":    "ClickHouse采样日志保留时间",
		"RETENTION_CH_TRACE_DATA":      "ClickHouse Trace数据保留时间",
		"RETENTION_CH_ACCESS_LOGS":     "ClickHouse访问日志保留时间",

		// 业务配置 - 成本计算
		"COST_CPU_PRICE":                             "CPU核心小时成本",
		"COST_MEM_PRICE":                             "内存GB小时成本",
		"COST_CALCULATION_INTERVAL":                  "成本计算间隔",
		"COST_AGGREGATION_LEVELS":                    "成本聚合级别",
		"COST_EFFICIENCY_ZOMBIE_THRESHOLD":           "僵尸效率阈值",
		"COST_EFFICIENCY_OVER_PROVISIONED_THRESHOLD": "过剩效率阈值",
		"COST_EFFICIENCY_HEALTHY_THRESHOLD":          "健康效率阈值",
		"COST_EFFICIENCY_DANGER_THRESHOLD":           "危险效率阈值",

		// 业务配置 - SLO
		"SLO_AVAILABILITY_THRESHOLD":    "SLO可用性阈值",
		"SLO_LATENCY_P95_THRESHOLD_MS":  "SLO P95延迟阈值(毫秒)",
		"SLO_SNAPSHOT_WINDOW":           "SLO快照窗口",
		"SLO_TRIGGER_DELAY":             "SLO触发延迟",
		"SLO_EVIDENCE_USER_IMPACT":      "SLO证据收集-用户影响",
		"SLO_EVIDENCE_CHANGE_EVENTS":    "SLO证据收集-变更事件",
		"SLO_EVIDENCE_RESOURCE_METRICS": "SLO证据收集-资源指标",

		// 业务配置 - ROI
		"ROI_BASELINE_DATE":      "ROI基线日期",
		"ROI_TRACKING_FREQUENCY": "ROI追踪频率",
		"ROI_METRICS":            "ROI指标列表",

		// 安全配置
		"SECURITY_CPU_LIMIT":                     "CPU资源限制",
		"SECURITY_MEMORY_LIMIT":                  "内存资源限制",
		"SECURITY_MAX_CONNECTIONS":               "最大连接数限制",
		"SECURITY_PROMETHEUS_QUERIES_PER_MINUTE": "Prometheus每分钟查询限制",
		"SECURITY_K8S_API_CALLS_PER_MINUTE":      "K8S API每分钟调用限制",
		"SECURITY_DATABASE_QUERIES_PER_MINUTE":   "数据库每分钟查询限制",
		"SECURITY_ENABLE_DATA_ENCRYPTION":        "启用数据加密",
		"SECURITY_ENCRYPTION_KEY":                "加密密钥 (敏感信息)",
	}
}
