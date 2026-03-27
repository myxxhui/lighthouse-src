// Package service — FinOpsSyncRunner 异步执行「辅助同步 + 多环境账单流水线」，与定时 ETL 同源。[Ref: 03_Phase6/01_FinOps 主动同步]
package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/myxxhui/lighthouse-src/internal/config"
	"github.com/myxxhui/lighthouse-src/internal/data/postgres"
	"github.com/myxxhui/lighthouse-src/internal/server/dto"
	"github.com/myxxhui/lighthouse-src/internal/worker/etl"
)

// ErrFinOpsSyncActive 已有 queued/running Job 时拒绝新建。[Ref: 03_Phase6/01_FinOps 主动同步]
var ErrFinOpsSyncActive = errors.New("finops sync job already active")

const finOpsETLMaxDuration = 2 * time.Hour

// FinOpsSyncRunner 主动 FinOps 同步 Job（持久化 finops_sync_job，单实例互斥以 DB 为准）。
type FinOpsSyncRunner struct {
	repo        postgres.Repository
	cfg         *config.Config
	workers     []*etl.BillingWorker
	auxiliary   func(context.Context) error
	auxTimeout  time.Duration // sync_auxiliary 阶段上限，默认见 config.EffectiveFinOpsSyncAuxTimeout
}

// NewFinOpsSyncRunner auxiliary 可为 nil（无 AK 时）；workers 可为空。[Ref: 03_Phase6/01_FinOps 主动同步]
func NewFinOpsSyncRunner(repo postgres.Repository, cfg *config.Config, workers []*etl.BillingWorker, auxiliary func(context.Context) error) *FinOpsSyncRunner {
	return &FinOpsSyncRunner{
		repo:       repo,
		cfg:        cfg,
		workers:    workers,
		auxiliary:  auxiliary,
		auxTimeout: config.EffectiveFinOpsSyncAuxTimeout(cfg.FinOpsSyncAuxTimeout),
	}
}

// EffectiveConfig 构建只读生效配置（供 GET /finops/effective-config）。
func (r *FinOpsSyncRunner) EffectiveConfig() dto.FinOpsEffectiveConfigResponse {
	return BuildFinOpsEffectiveConfigDTO(r.cfg)
}

// BuildFinOpsEffectiveConfigDTO 从进程内 config 生成 DTO（无 Runner 时也可用于 HTTP 层）。[Ref: 03_Phase6/01_FinOps 主动同步]
func BuildFinOpsEffectiveConfigDTO(cfg *config.Config) dto.FinOpsEffectiveConfigResponse {
	if cfg == nil {
		return dto.FinOpsEffectiveConfigResponse{}
	}
	by := config.BuildFinOpsCGSourceByEnvMap(cfg.FinOpsCGSourceByEnv)
	auxTO := config.EffectiveFinOpsSyncAuxTimeout(cfg.FinOpsSyncAuxTimeout)
	authReq := strings.TrimSpace(cfg.FinOpsSyncJobAPIKey) != ""
	return dto.FinOpsEffectiveConfigResponse{
		FinOpsCGSource:            config.EffectiveFinOpsCGSource(cfg.FinOpsCGSource),
		FinOpsCGSourceByEnv:       by,
		CloudBillingProvider:      cfg.CloudBilling.Provider,
		BillingCycle:              cfg.CloudBilling.BillingCycle,
		ETLScheduleCron:           cfg.CloudBilling.EffectiveETLScheduleCron(),
		ETLDailyPullMonths:        cfg.CloudBilling.ETLData.DailyPullMonths,
		ETLDailyRetentionMonths:   cfg.CloudBilling.ETLData.DailyRetentionMonths,
		ETLMonthlyPullMonths:      cfg.CloudBilling.ETLData.MonthlyPullMonths,
		ETLMonthlyRetentionMonths: cfg.CloudBilling.ETLData.MonthlyRetentionMonths,
		SyncAuxTimeoutSeconds:     int(auxTO.Seconds()),
		SyncJobAuthRequired:       authReq,
	}
}

type finopsJobConfigSnapshot struct {
	FinOpsCGSource       string            `json:"finops_cg_source"`
	FinOpsCGSourceByEnv  map[string]string `json:"finops_cg_source_by_env,omitempty"`
	CloudBillingProvider string            `json:"cloud_billing_provider"`
	BillingCycle         string            `json:"billing_cycle"`
	ETLScheduleCron      string            `json:"etl_schedule_cron"`
	ETLData              config.CloudBillingETLData `json:"etl_data"`
}

func (r *FinOpsSyncRunner) buildConfigSnapshotJSON() string {
	d := BuildFinOpsEffectiveConfigDTO(r.cfg)
	snap := finopsJobConfigSnapshot{
		FinOpsCGSource:       d.FinOpsCGSource,
		FinOpsCGSourceByEnv:  d.FinOpsCGSourceByEnv,
		CloudBillingProvider: d.CloudBillingProvider,
		BillingCycle:         d.BillingCycle,
		ETLScheduleCron:      d.ETLScheduleCron,
		ETLData: config.CloudBillingETLData{
			DailyPullMonths:        d.ETLDailyPullMonths,
			DailyRetentionMonths:   d.ETLDailyRetentionMonths,
			MonthlyPullMonths:      d.ETLMonthlyPullMonths,
			MonthlyRetentionMonths: d.ETLMonthlyRetentionMonths,
		},
	}
	b, _ := json.Marshal(snap)
	return string(b)
}

// CreateJob 插入 queued Job 并异步执行；已有 active Job 时返回 ErrFinOpsSyncActive。
func (r *FinOpsSyncRunner) CreateJob(ctx context.Context) (int64, error) {
	n, err := r.repo.CountActiveFinOpsSyncJobs(ctx)
	if err != nil {
		return 0, err
	}
	if n > 0 {
		slog.Warn("finops_sync_job rejected: another active job exists")
		return 0, ErrFinOpsSyncActive
	}
	id, err := r.repo.InsertFinOpsSyncJob(ctx, postgres.FinOpsSyncJobRow{
		Status:         "queued",
		Phase:          "",
		ConfigSnapshot: r.buildConfigSnapshotJSON(),
		Warnings:       "[]",
		DataVersion:    0,
	})
	if err != nil {
		return 0, err
	}
	slog.Info("finops_sync_job queued", "job_id", id)
	go r.runJob(id)
	return id, nil
}

// ActiveJobID 返回当前 queued/running 的 Job id；无则 sql.ErrNoRows。[Ref: 03_Phase6/01_FinOps 主动同步]
func (r *FinOpsSyncRunner) ActiveJobID(ctx context.Context) (int64, error) {
	return r.repo.GetActiveFinOpsSyncJobID(ctx)
}

// GetJob 返回 Job 状态；不存在时 error 为 sql.ErrNoRows（由调用方映射 404）。
func (r *FinOpsSyncRunner) GetJob(ctx context.Context, id int64) (dto.FinOpsSyncJobStatusResponse, error) {
	row, err := r.repo.GetFinOpsSyncJob(ctx, id)
	if err != nil {
		return dto.FinOpsSyncJobStatusResponse{}, err
	}
	var warns []string
	if strings.TrimSpace(row.Warnings) != "" {
		_ = json.Unmarshal([]byte(row.Warnings), &warns)
	}
	return dto.FinOpsSyncJobStatusResponse{
		JobID:             row.ID,
		Status:            row.Status,
		Phase:             row.Phase,
		Warnings:          warns,
		ErrorMessage:      row.ErrorMessage,
		CreatedAt:         row.CreatedAt,
		StartedAt:         row.StartedAt,
		CompletedAt:       row.CompletedAt,
		DataVersion:       row.DataVersion,
		ConfigSnapshot:    row.ConfigSnapshot,
		ProgressCurrent:   row.ProgressCurrent,
		ProgressTotal:     row.ProgressTotal,
		PhaseDetail:       row.PhaseDetail,
	}, nil
}

func (r *FinOpsSyncRunner) runJob(jobID int64) {
	ctx := context.Background()
	update := func(j *postgres.FinOpsSyncJobRow) {
		if j == nil {
			return
		}
		_ = r.repo.UpdateFinOpsSyncJob(ctx, *j)
	}
	j, err := r.repo.GetFinOpsSyncJob(ctx, jobID)
	if err != nil {
		return
	}
	totalSteps := 1 + len(r.workers)
	if totalSteps < 1 {
		totalSteps = 1
	}
	slog.Info("finops_sync_job running", "job_id", jobID, "aux_timeout", r.auxTimeout.String(), "workers", len(r.workers), "progress_total", totalSteps)
	now := time.Now().UTC()
	j.Status = "running"
	j.Phase = "sync_auxiliary"
	j.StartedAt = &now
	j.ProgressTotal = totalSteps
	j.ProgressCurrent = 0
	j.PhaseDetail = "辅助同步"
	update(j)

	var warnings []string
	if r.auxiliary != nil {
		auxCtx, cancel := context.WithTimeout(ctx, r.auxTimeout)
		err := r.auxiliary(auxCtx)
		cancel()
		if err != nil {
			if errors.Is(err, context.DeadlineExceeded) {
				msg := "sync_auxiliary: deadline exceeded (FINOPS_SYNC_AUX_TIMEOUT)"
				warnings = append(warnings, msg)
				slog.Warn("finops_sync_job auxiliary timeout", "job_id", jobID, "timeout", r.auxTimeout.String())
			} else {
				warnings = append(warnings, "sync_auxiliary:"+err.Error())
				slog.Warn("finops_sync_job auxiliary failed", "job_id", jobID, "error", err.Error())
			}
		}
	} else {
		warnings = append(warnings, "sync_auxiliary skipped (no auxiliary sync registered)")
	}

	j, _ = r.repo.GetFinOpsSyncJob(ctx, jobID)
	j.Phase = "pipeline"
	j.ProgressCurrent = 1
	j.PhaseDetail = "账单流水线"
	j.Warnings = encodeWarningsJSON(warnings)
	update(j)

	pipelineOK := 0
	pipelineFail := 0
	for _, w := range r.workers {
		label := w.EnvKey
		if label == "" {
			label = "default"
		}
		j, _ = r.repo.GetFinOpsSyncJob(ctx, jobID)
		j.PhaseDetail = fmt.Sprintf("环境 %s", label)
		update(j)

		warns, err := etl.RunFullETLCycle(ctx, w, finOpsETLMaxDuration)
		for _, m := range warns {
			warnings = append(warnings, fmt.Sprintf("[%s] %s", label, m))
		}
		if err != nil {
			pipelineFail++
			warnings = append(warnings, fmt.Sprintf("[%s] pipeline: %v", label, err))
			slog.Warn("finops_sync_job pipeline worker failed", "job_id", jobID, "env", label, "error", err.Error())
		} else {
			pipelineOK++
			slog.Info("finops_sync_job pipeline worker ok", "job_id", jobID, "env", label, "soft_warnings", len(warns))
		}
		j, _ = r.repo.GetFinOpsSyncJob(ctx, jobID)
		j.ProgressCurrent++
		j.Warnings = encodeWarningsJSON(warnings)
		update(j)
	}
	if len(r.workers) == 0 {
		warnings = append(warnings, "pipeline skipped (no billing workers)")
	}

	j, _ = r.repo.GetFinOpsSyncJob(ctx, jobID)
	j.Warnings = encodeWarningsJSON(warnings)
	done := time.Now().UTC()
	j.CompletedAt = &done
	j.DataVersion = done.UnixMilli()

	allPipelineFailed := len(r.workers) > 0 && pipelineOK == 0 && pipelineFail > 0
	if allPipelineFailed {
		j.Status = "failed"
		j.Phase = "pipeline"
		if j.ErrorMessage == "" {
			j.ErrorMessage = "all billing pipeline runs failed"
		}
		slog.Error("finops_sync_job failed", "job_id", jobID, "error_message", j.ErrorMessage, "warning_count", len(warnings))
		update(j)
		return
	}

	if len(warnings) > 0 {
		j.Status = "succeeded_with_warnings"
	} else {
		j.Status = "succeeded"
	}
	j.Phase = "done"
	update(j)
	slog.Info("finops_sync_job completed", "job_id", jobID, "status", j.Status, "warning_count", len(warnings), "data_version", j.DataVersion)
}

func encodeWarningsJSON(warnings []string) string {
	if warnings == nil {
		warnings = []string{}
	}
	b, _ := json.Marshal(warnings)
	return string(b)
}
