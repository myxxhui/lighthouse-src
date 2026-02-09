package config

// GetEnvMapping 获取环境变量映射关系
func GetEnvMapping() map[string]string {
	return map[string]string{
		"ENV":                        "环境类型 (dev/staging/prod)",
		"SERVER_PORT":                "服务端口",
		"SERVER_READ_TIMEOUT":        "服务读取超时",
		"SERVER_WRITE_TIMEOUT":       "服务写入超时",
		"LOG_LEVEL":                  "日志级别",
		"DB_HOST":                    "数据库主机",
		"DB_PORT":                    "数据库端口",
		"DB_USER":                    "数据库用户",
		"DB_PASSWORD":                "数据库密码 (敏感信息)",
		"DB_NAME":                    "数据库名称",
		"DB_SSL_MODE":                "数据库SSL模式",
		"PROMETHEUS_ADDRESS":         "Prometheus地址",
		"PROMETHEUS_QUERY_TIMEOUT":   "Prometheus查询超时",
		"K8S_API_SERVER":             "Kubernetes API服务器地址",
		"K8S_RBAC_ENABLED":           "是否启用RBAC",
		"COST_CPU_PRICE":             "CPU核心小时成本",
		"COST_MEM_PRICE":             "内存GB小时成本",
		"SLO_AVAILABILITY_THRESHOLD": "SLO可用性阈值",
		"SLO_LATENCY_THRESHOLD_MS":   "SLO延迟阈值(毫秒)",
	}
}
