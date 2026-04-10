// Package config — Lighthouse 部署 YAML（单文件：基础设施 + 项目云账号）。[Ref: lighthouse-deploy/config/cloud-accounts-projects.yaml]
package config

import (
	"fmt"
	"os"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// LighthouseDeployYAML 根文档：version≥2 时可含 postgres/clickhouse/cloud_billing/finops/oss；projects 与旧版兼容。
// 密钥使用 ${ENV_VAR}，由进程环境展开（docker-compose 可仅注入少量 Secret）。[Ref: 03_Phase6 项目云账号]
type LighthouseDeployYAML struct {
	Version      int                    `yaml:"version"`
	Postgres     *PostgresDeployYAML    `yaml:"postgres"`
	ClickHouse   *ClickHouseDeployYAML  `yaml:"clickhouse"`
	CloudBilling *CloudBillingDeployYAML `yaml:"cloud_billing"`
	FinOps       *FinOpsDeployYAML      `yaml:"finops"`
	OSS          *OSSDeployYAML         `yaml:"oss"`
	Projects     []CloudAccountsProject `yaml:"projects"`
}

// PostgresDeployYAML 控制平面 PG；docker 中 host 由环境 POSTGRES_HOST=postgres 覆盖。[Ref: 03_00_数据库与存储就绪]
type PostgresDeployYAML struct {
	Host     string `yaml:"host"`
	Port     int    `yaml:"port"`
	User     string `yaml:"user"`
	Password string `yaml:"password"`
	Database string `yaml:"database"`
	SSLMode  string `yaml:"ssl_mode"`
}

// ClickHouseDeployYAML 证据平面（可选）。
type ClickHouseDeployYAML struct {
	Host     string `yaml:"host"`
	Port     int    `yaml:"port"`
	User     string `yaml:"user"`
	Password string `yaml:"password"`
	Database string `yaml:"database"`
}

// CloudBillingDeployYAML 云账单 ETL。
type CloudBillingDeployYAML struct {
	Provider        string `yaml:"provider"`
	Endpoint        string `yaml:"endpoint"`
	PeriodType      string `yaml:"period_type"`
	BillingCycle    string `yaml:"billing_cycle"`
	ETLScheduleCron string `yaml:"etl_schedule_cron"`
	ETLData         *struct {
		DailyPullMonths        int `yaml:"daily_pull_months"`
		DailyRetentionMonths   int `yaml:"daily_retention_months"`
		MonthlyPullMonths      int `yaml:"monthly_pull_months"`
		MonthlyRetentionMonths int `yaml:"monthly_retention_months"`
	} `yaml:"etl_data"`
}

// FinOpsDeployYAML 五维 C/G 与同步。
type FinOpsDeployYAML struct {
	CGSource        string            `yaml:"cg_source"`
	CGSourceByEnv   map[string]string `yaml:"cg_source_by_env"`
	SyncAuxTimeout  string            `yaml:"sync_aux_timeout"`
	SyncJobAPIKey   string            `yaml:"sync_job_api_key"`
}

// OSSDeployYAML 全局 OSS 账单（无 ProjectCloudProfile 时 sync 使用）；与项目内 oss_billing 二选一或并存。
type OSSDeployYAML struct {
	BillingBucket       string `yaml:"billing_bucket"`
	Prefix              string `yaml:"prefix"`
	Endpoint            string `yaml:"endpoint"`
	SyncMode            string `yaml:"sync_mode"`
	BillingCredentialEnv string `yaml:"billing_credential_env"`
	BillingOnlyEnv      string `yaml:"billing_only_env"`
	FullSync            *bool  `yaml:"full_sync"`
	IncrementalSync     *bool  `yaml:"incremental_sync"`
}

// LoadLighthouseDeployYAML 读取统一部署 YAML；path 空则 LIGHTHOUSE_DEPLOY_YAML，其次 CLOUD_ACCOUNTS_PROJECTS_YAML。
func LoadLighthouseDeployYAML(path string) (*LighthouseDeployYAML, error) {
	if strings.TrimSpace(path) == "" {
		path = strings.TrimSpace(os.Getenv("LIGHTHOUSE_DEPLOY_YAML"))
	}
	if path == "" {
		path = strings.TrimSpace(os.Getenv("CLOUD_ACCOUNTS_PROJECTS_YAML"))
	}
	candidates := []string{}
	if path != "" {
		candidates = append(candidates, path)
	}
	candidates = append(candidates,
		"/app/config/cloud-accounts-projects.yaml",
		"./config/cloud-accounts-projects.yaml",
		"configs/cloud-accounts-projects.yaml",
		"../lighthouse-deploy/config/cloud-accounts-projects.yaml",
	)
	for _, p := range candidates {
		if p == "" {
			continue
		}
		b, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		var raw LighthouseDeployYAML
		if err := yaml.Unmarshal(b, &raw); err != nil {
			return nil, fmt.Errorf("lighthouse deploy yaml %s: %w", p, err)
		}
		expandLighthouseDeployYAML(&raw)
		return &raw, nil
	}
	return nil, nil
}

func expandLighthouseDeployYAML(doc *LighthouseDeployYAML) {
	if doc == nil {
		return
	}
	if doc.Postgres != nil {
		doc.Postgres.Host = os.ExpandEnv(doc.Postgres.Host)
		doc.Postgres.User = os.ExpandEnv(doc.Postgres.User)
		doc.Postgres.Password = os.ExpandEnv(doc.Postgres.Password)
		doc.Postgres.Database = os.ExpandEnv(doc.Postgres.Database)
		doc.Postgres.SSLMode = os.ExpandEnv(doc.Postgres.SSLMode)
	}
	if doc.ClickHouse != nil {
		doc.ClickHouse.Host = os.ExpandEnv(doc.ClickHouse.Host)
		doc.ClickHouse.User = os.ExpandEnv(doc.ClickHouse.User)
		doc.ClickHouse.Password = os.ExpandEnv(doc.ClickHouse.Password)
		doc.ClickHouse.Database = os.ExpandEnv(doc.ClickHouse.Database)
	}
	if doc.CloudBilling != nil {
		doc.CloudBilling.Provider = os.ExpandEnv(doc.CloudBilling.Provider)
		doc.CloudBilling.Endpoint = os.ExpandEnv(doc.CloudBilling.Endpoint)
		doc.CloudBilling.PeriodType = os.ExpandEnv(doc.CloudBilling.PeriodType)
		doc.CloudBilling.BillingCycle = os.ExpandEnv(doc.CloudBilling.BillingCycle)
		doc.CloudBilling.ETLScheduleCron = os.ExpandEnv(doc.CloudBilling.ETLScheduleCron)
	}
	if doc.FinOps != nil {
		doc.FinOps.CGSource = os.ExpandEnv(doc.FinOps.CGSource)
		doc.FinOps.SyncAuxTimeout = os.ExpandEnv(doc.FinOps.SyncAuxTimeout)
		doc.FinOps.SyncJobAPIKey = os.ExpandEnv(doc.FinOps.SyncJobAPIKey)
		if doc.FinOps.CGSourceByEnv != nil {
			m := make(map[string]string)
			for k, v := range doc.FinOps.CGSourceByEnv {
				m[k] = os.ExpandEnv(v)
			}
			doc.FinOps.CGSourceByEnv = m
		}
	}
	if doc.OSS != nil {
		doc.OSS.BillingBucket = os.ExpandEnv(doc.OSS.BillingBucket)
		doc.OSS.Prefix = os.ExpandEnv(doc.OSS.Prefix)
		doc.OSS.Endpoint = os.ExpandEnv(doc.OSS.Endpoint)
		doc.OSS.SyncMode = os.ExpandEnv(doc.OSS.SyncMode)
		doc.OSS.BillingCredentialEnv = os.ExpandEnv(doc.OSS.BillingCredentialEnv)
		doc.OSS.BillingOnlyEnv = os.ExpandEnv(doc.OSS.BillingOnlyEnv)
	}
	expandCloudAccountsProjectsEnvLegacy(doc)
}

// ExpandCloudAccountsProjectsEnv 展开 projects 内 ${VAR}（单测与外部在解析后调用）。
func ExpandCloudAccountsProjectsEnv(doc *CloudAccountsProjectsFile) {
	if doc == nil {
		return
	}
	tmp := &LighthouseDeployYAML{Projects: doc.Projects}
	expandCloudAccountsProjectsEnvLegacy(tmp)
}

// expandCloudAccountsProjectsEnvLegacy 展开 projects[].environments 内字段（与旧逻辑一致）。
func expandCloudAccountsProjectsEnvLegacy(doc *LighthouseDeployYAML) {
	for i := range doc.Projects {
		p := &doc.Projects[i]
		for j := range p.Environments {
			e := &p.Environments[j]
			e.AccessKeyID = os.ExpandEnv(e.AccessKeyID)
			e.AccessKeySecret = os.ExpandEnv(e.AccessKeySecret)
			e.BSSEndpoint = os.ExpandEnv(e.BSSEndpoint)
			if e.EnvironmentKey == "" && p.ID != "" && e.Name != "" {
				e.EnvironmentKey = p.ID + "_" + e.Name
			}
			if e.OSSBilling != nil {
				e.OSSBilling.Bucket = os.ExpandEnv(e.OSSBilling.Bucket)
				e.OSSBilling.Prefix = os.ExpandEnv(e.OSSBilling.Prefix)
				e.OSSBilling.Endpoint = os.ExpandEnv(e.OSSBilling.Endpoint)
			}
			e.FinOpsCGSource = strings.TrimSpace(os.ExpandEnv(e.FinOpsCGSource))
		}
	}
}

// ApplyLighthouseDeployYAML 将 YAML 合并入 cfg，并写入进程内 OSS_* / OSS_BILLING_* 等供未使用 ProjectCloudProfile 的 ETL 路径读取。
// 调用方应在 fill*FromEnv 之前调用，使环境变量仍可覆盖 YAML。[Ref: lighthouse-deploy 单文件配置]
func ApplyLighthouseDeployYAML(cfg *Config, doc *LighthouseDeployYAML) {
	if cfg == nil || doc == nil {
		return
	}
	if doc.Postgres != nil {
		p := doc.Postgres
		if p.Host != "" {
			cfg.Postgres.Host = p.Host
		}
		if p.Port > 0 {
			cfg.Postgres.Port = p.Port
		}
		if p.User != "" {
			cfg.Postgres.User = p.User
		}
		if p.Password != "" {
			cfg.Postgres.Password = p.Password
		}
		if p.Database != "" {
			cfg.Postgres.Database = p.Database
		}
		if p.SSLMode != "" {
			cfg.Postgres.SSLMode = p.SSLMode
		}
	}
	if doc.ClickHouse != nil {
		ch := doc.ClickHouse
		if ch.Host != "" {
			cfg.ClickHouse.Host = ch.Host
		}
		if ch.Port > 0 {
			cfg.ClickHouse.Port = ch.Port
		}
		if ch.User != "" {
			cfg.ClickHouse.User = ch.User
		}
		if ch.Password != "" {
			cfg.ClickHouse.Password = ch.Password
		}
		if ch.Database != "" {
			cfg.ClickHouse.Database = ch.Database
		}
	}
	if doc.CloudBilling != nil {
		cb := doc.CloudBilling
		if cb.Provider != "" {
			cfg.CloudBilling.Provider = cb.Provider
		}
		if cb.Endpoint != "" {
			cfg.CloudBilling.Endpoint = cb.Endpoint
		}
		if cb.PeriodType != "" {
			cfg.CloudBilling.PeriodType = cb.PeriodType
		}
		if cb.BillingCycle != "" {
			cfg.CloudBilling.BillingCycle = cb.BillingCycle
		}
		if cb.ETLScheduleCron != "" {
			cfg.CloudBilling.ETLScheduleCron = cb.ETLScheduleCron
		}
		if cb.ETLData != nil {
			if cb.ETLData.DailyPullMonths > 0 {
				cfg.CloudBilling.ETLData.DailyPullMonths = cb.ETLData.DailyPullMonths
			}
			if cb.ETLData.DailyRetentionMonths > 0 {
				cfg.CloudBilling.ETLData.DailyRetentionMonths = cb.ETLData.DailyRetentionMonths
			}
			if cb.ETLData.MonthlyPullMonths > 0 {
				cfg.CloudBilling.ETLData.MonthlyPullMonths = cb.ETLData.MonthlyPullMonths
			}
			if cb.ETLData.MonthlyRetentionMonths > 0 {
				cfg.CloudBilling.ETLData.MonthlyRetentionMonths = cb.ETLData.MonthlyRetentionMonths
			}
		}
	}
	if doc.FinOps != nil {
		f := doc.FinOps
		if f.CGSource != "" {
			cfg.FinOpsCGSource = f.CGSource
		}
		if len(f.CGSourceByEnv) > 0 {
			if cfg.FinOpsCGSourceByEnv == nil {
				cfg.FinOpsCGSourceByEnv = make(map[string]string)
			}
			for k, v := range f.CGSourceByEnv {
				kk := strings.ToUpper(strings.TrimSpace(k))
				if kk == "" {
					continue
				}
				cfg.FinOpsCGSourceByEnv[kk] = EffectiveFinOpsCGSource(v)
			}
		}
		if f.SyncAuxTimeout != "" {
			if d, err := time.ParseDuration(strings.TrimSpace(f.SyncAuxTimeout)); err == nil {
				cfg.FinOpsSyncAuxTimeout = d
			}
		}
		if f.SyncJobAPIKey != "" {
			cfg.FinOpsSyncJobAPIKey = f.SyncJobAPIKey
		}
	}
	if doc.OSS != nil {
		o := doc.OSS
		// 已存在的环境变量优先（compose / Secret 注入覆盖 YAML 默认值）。[Ref: lighthouse-deploy 单文件配置]
		setProcEnv := func(key, val string) {
			if val == "" || strings.TrimSpace(os.Getenv(key)) != "" {
				return
			}
			_ = os.Setenv(key, val)
		}
		setProcEnv("OSS_BILLING_BUCKET", o.BillingBucket)
		setProcEnv("OSS_BILLING_PREFIX", o.Prefix)
		setProcEnv("OSS_ENDPOINT", o.Endpoint)
		setProcEnv("OSS_SYNC_MODE", o.SyncMode)
		setProcEnv("OSS_BILLING_CREDENTIAL_ENV", o.BillingCredentialEnv)
		setProcEnv("OSS_BILLING_ONLY_ENV", o.BillingOnlyEnv)
		if os.Getenv("OSS_FULL_SYNC") == "" && o.FullSync != nil && *o.FullSync {
			_ = os.Setenv("OSS_FULL_SYNC", "1")
		}
		if os.Getenv("OSS_INCREMENTAL_SYNC") == "" && o.IncrementalSync != nil && *o.IncrementalSync {
			_ = os.Setenv("OSS_INCREMENTAL_SYNC", "1")
		}
	}
}

// BSSEndpointForEnvironmentKey 返回 projects 中与 environment_key 匹配环境的 bss_endpoint（已在 LoadLighthouseDeployYAML 中展开 ${VAR}）。[Ref: 03_Phase6/01_FinOps BSS]
func BSSEndpointForEnvironmentKey(doc *LighthouseDeployYAML, environmentKey string) string {
	if doc == nil {
		return ""
	}
	want := strings.ToUpper(strings.TrimSpace(environmentKey))
	for _, p := range doc.Projects {
		for _, e := range p.Environments {
			if strings.ToUpper(strings.TrimSpace(e.EnvironmentKey)) == want {
				return strings.TrimSpace(e.BSSEndpoint)
			}
		}
	}
	return ""
}
