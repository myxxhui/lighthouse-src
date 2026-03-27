package dto

import "time"

// FinOpsEffectiveConfigResponse 只读：当前进程生效的 FinOps/ETL 相关配置快照（与部署环境变量一致，不含密钥）。[Ref: 03_Phase6/01_FinOps 主动同步]
type FinOpsEffectiveConfigResponse struct {
	FinOpsCGSource       string            `json:"finops_cg_source"`
	FinOpsCGSourceByEnv  map[string]string `json:"finops_cg_source_by_env,omitempty"`
	CloudBillingProvider string            `json:"cloud_billing_provider"`
	BillingCycle         string            `json:"billing_cycle"`
	ETLScheduleCron      string            `json:"etl_schedule_cron"`
	ETLDailyPullMonths   int               `json:"etl_daily_pull_months"`
	ETLDailyRetentionMonths int            `json:"etl_daily_retention_months"`
	ETLMonthlyPullMonths int               `json:"etl_monthly_pull_months"`
	ETLMonthlyRetentionMonths int          `json:"etl_monthly_retention_months"`
	// SyncAuxTimeoutSeconds sync_auxiliary 阶段超时秒数（与 FINOPS_SYNC_AUX_TIMEOUT / 默认 30m 一致）。[Ref: 03_Phase6/01_FinOps 主动同步]
	SyncAuxTimeoutSeconds int `json:"sync_aux_timeout_seconds"`
	// SyncJobAuthRequired 为 true 时 POST /finops/sync-jobs 须带 X-FinOps-Sync-Key（进程已配置 FINOPS_SYNC_JOB_API_KEY）。[Ref: 03_Phase6/01_FinOps 主动同步]
	SyncJobAuthRequired bool `json:"sync_job_auth_required"`
}

// FinOpsSyncJobCreateResponse POST /finops/sync-jobs 202 响应。[Ref: 03_Phase6/01_FinOps 主动同步]
type FinOpsSyncJobCreateResponse struct {
	JobID int64 `json:"job_id"`
}

// FinOpsSyncJobStatusResponse GET /finops/sync-jobs/:id。[Ref: 03_Phase6/01_FinOps 主动同步]
type FinOpsSyncJobStatusResponse struct {
	JobID          int64      `json:"job_id"`
	Status         string     `json:"status"`
	Phase          string     `json:"phase"`
	Warnings       []string   `json:"warnings,omitempty"`
	ErrorMessage   string     `json:"error_message,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
	StartedAt      *time.Time `json:"started_at,omitempty"`
	CompletedAt    *time.Time `json:"completed_at,omitempty"`
	DataVersion    int64      `json:"data_version"`
	ConfigSnapshot string     `json:"config_snapshot,omitempty"`
	// ProgressCurrent/ProgressTotal：步骤进度（辅助同步 1 步 + 每环境流水线各 1 步），非耗时占比。[Ref: 03_Phase6/01_FinOps 主动同步]
	ProgressCurrent int    `json:"progress_current"`
	ProgressTotal   int    `json:"progress_total"`
	PhaseDetail     string `json:"phase_detail,omitempty"`
}
