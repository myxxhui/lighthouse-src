// Package etl — FinOps 辅助同步：OSS 账单 CSV→finops_billing_fact、BSS P/B/U 落库。[Ref: 03_Phase6/01_FinOps 采集与ETL]
package etl

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/myxxhui/lighthouse-src/internal/data/ossfinops"
	"github.com/myxxhui/lighthouse-src/internal/data/postgres"
)

// SyncFinOpsAuxiliary 拉取 BSS 流水/余额快照/账期应付并落库；与 RunPipeline ⓪ 步一致，可供 HTTP 触发刷新。[Ref: 03_Phase6/01_FinOps]
func (w *BillingWorker) SyncFinOpsAuxiliary(ctx context.Context, now time.Time) error {
	if w.pipelineRepo == nil {
		return nil
	}
	// 有 Fetcher 时 BSS 解析主账号；仅 OSS 时从 cost_env_account_config 读已写入的 account_id，保证 finops_billing_fact 与聚合表 account_id 一致。[Ref: 03_Phase6/01_FinOps]
	if w.fetcher != nil {
		w.resolveAliyunAccountIDOnce(ctx)
	} else {
		w.resolveAccountIDFromEnvConfig(ctx)
	}
	// OSS CSV→finops_billing_fact 与 BSS API 解耦：无 Fetcher 时仍同步 OSS（仅须 bucket+AK）。OSS 失败不阻断后续 BSS。[Ref: 04_采集 §七]
	var ossErr error
	if err := syncOSSBillingDetailIfConfigured(ctx, w.pipelineRepo, w.EnvKey, w.dbAccountID(), w.ProjectCloudProfile); err != nil {
		ossErr = err
	}
	if w.fetcher == nil {
		return ossErr
	}

	// 日常仅拉「近 N 天」：控制 BSS QueryAccountTransactions 调用量与单次辅助同步耗时；历史区间由 cmd/bss-transactions-backfill 补齐（Upsert 幂等）。[Ref: 03_Phase6/01_FinOps] [Ref: 04_采集 §七 R15]
	days := BssTransactionsLookbackDays()
	start := now.AddDate(0, 0, -days).UTC()
	end := now.UTC()
	items, err := w.fetcher.FetchBSSTransactions(ctx, start, end)
	if err != nil {
		slog.Warn("billing ETL: FetchBSSTransactions failed", "error", err, "env_key", w.EnvKey, "db_account_id", w.dbAccountID())
	} else {
		for _, it := range items {
			row := postgres.BSSTransactionRow{
				TransactionNumber: it.TransactionNumber,
				AccountID:         w.dbAccountID(),
				TransactionTime:   it.TransactionTime,
				Amount:            it.Amount,
				TransactionType:   it.TransactionType,
				TransactionFlow:   it.TransactionFlow,
				RecordID:          it.RecordID,
				BillingCycle:      it.BillingCycle,
				Currency:          it.Currency,
				TransactionChannel: it.TransactionChannel,
				FundType:           it.FundType,
				Remarks:            it.Remarks,
			}
			if row.Currency == "" {
				row.Currency = "CNY"
			}
			if err := w.pipelineRepo.UpsertBSSTransaction(ctx, row); err != nil {
				slog.Warn("billing ETL: UpsertBSSTransaction failed", "error", err, "txn", row.TransactionNumber)
			}
		}
		if len(items) > 0 {
			slog.Info("billing ETL: bss transactions upserted", "count", len(items), "env_key", w.EnvKey, "db_account_id", w.dbAccountID())
		}
		if err := w.pipelineRepo.RefreshBSSRechargeMonthlyForAccount(ctx, w.dbAccountID()); err != nil {
			slog.Warn("billing ETL: RefreshBSSRechargeMonthlyForAccount failed", "error", err, "db_account_id", w.dbAccountID())
		}
	}

	if avail, cur, err := w.fetcher.FetchAccountBalanceSnapshot(ctx); err != nil {
		slog.Warn("billing ETL: FetchAccountBalanceSnapshot failed", "error", err, "env_key", w.EnvKey, "db_account_id", w.dbAccountID())
	} else {
		snap := postgres.BSSBalanceSnapshotRow{
			AccountID:       w.dbAccountID(),
			SnapshotDate:    now.UTC().Truncate(24 * time.Hour),
			AvailableAmount: avail,
			Currency:        cur,
		}
		if snap.Currency == "" {
			snap.Currency = "CNY"
		}
		if err := w.pipelineRepo.UpsertBSSBalanceSnapshot(ctx, snap); err != nil {
			slog.Warn("billing ETL: UpsertBSSBalanceSnapshot failed", "error", err)
		}
	}

	nMonth := w.monthlyPullMonths()
	for i := 0; i < nMonth; i++ {
		t := now.AddDate(0, -nMonth+1+i, 0)
		cycle := t.Format("2006-01")
		u, err := w.fetcher.FetchOutstandingMonthly(ctx, cycle)
		if err != nil {
			slog.Warn("billing ETL: FetchOutstandingMonthly failed", "billing_cycle", cycle, "error", err)
			continue
		}
		o := postgres.BillOutstandingMonthlyRow{
			BillingCycle:      cycle,
			AccountID:         w.dbAccountID(),
			OutstandingAmount: u,
		}
		if err := w.pipelineRepo.UpsertBillOutstandingMonthly(ctx, o); err != nil {
			slog.Warn("billing ETL: UpsertBillOutstandingMonthly failed", "error", err, "billing_cycle", cycle)
		}
	}
	return ossErr
}

const (
	bssLookbackDefaultDays = 14
	bssLookbackMaxDays     = 731 // 约两年，防止误配极大值拖垮辅助同步
)

// BssTransactionsLookbackDays 返回 SyncFinOpsAuxiliary 拉 BSS 流水的起始偏移（整天，含与 end 之间的区间）。
// 环境变量 FINOPS_BSS_TRANSACTIONS_LOOKBACK_DAYS：默认 14；非法或 <1 回退默认；> bssLookbackMaxDays 截断并 WARN。[Ref: 03_Phase6/01_FinOps] [Ref: 04_采集 §七 R15]
func BssTransactionsLookbackDays() int {
	raw := strings.TrimSpace(os.Getenv("FINOPS_BSS_TRANSACTIONS_LOOKBACK_DAYS"))
	if raw == "" {
		return bssLookbackDefaultDays
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 1 {
		slog.Warn("billing ETL: FINOPS_BSS_TRANSACTIONS_LOOKBACK_DAYS invalid, using default", "raw", raw, "default", bssLookbackDefaultDays)
		return bssLookbackDefaultDays
	}
	if n > bssLookbackMaxDays {
		slog.Warn("billing ETL: FINOPS_BSS_TRANSACTIONS_LOOKBACK_DAYS capped", "requested", n, "max", bssLookbackMaxDays)
		return bssLookbackMaxDays
	}
	return n
}

func envTruthy(name string) bool {
	v := strings.TrimSpace(strings.ToLower(os.Getenv(name)))
	return v == "1" || v == "true" || v == "yes"
}

// ossBillingCredentialEnv 返回用于连接 OSS 的环境名（AK 后缀）。未设 OSS_BILLING_CREDENTIAL_ENV 时与 Worker 一致；可设为 UAT 等以使用对 bucket 有读权限的 RAM 用户，与 dbAccountID 落库键解耦。[Ref: 04_采集 §七 POC 403]
func ossBillingCredentialEnv(workerEnvKey string) string {
	v := strings.TrimSpace(os.Getenv("OSS_BILLING_CREDENTIAL_ENV"))
	if v != "" {
		return v
	}
	return strings.TrimSpace(workerEnvKey)
}

// syncOSSBillingDetailIfConfigured 未配置 OSS 时立即返回；YAML 模式用 profile 桶+凭证；否则用全局 OSS_BILLING_* + EnvForFinOps。
// YAML Worker 若未填 oss_billing.bucket 则不同步 OSS（不回退全局桶，避免跨主账号误读）。[Ref: 04_采集 §5.6 §七] [Ref: 03_Phase6 项目云账号]
func syncOSSBillingDetailIfConfigured(ctx context.Context, repo CloudBillPipelineRepository, envKey, dbAccountID string, profile *ProjectCloudProfile) error {
	if profile != nil && strings.TrimSpace(profile.OSSBucket) == "" {
		slog.Debug("billing ETL: OSS sync skipped (yaml profile without oss_billing.bucket)", "env_key", envKey)
		return nil
	}
	useYAML := profile != nil
	var bucket, prefix, endpoint string
	var ak, sk string
	if useYAML {
		bucket = strings.TrimSpace(profile.OSSBucket)
		prefix = strings.TrimSpace(profile.OSSPrefix)
		if prefix == "" {
			prefix = "billing-data/"
		}
		endpoint = strings.TrimSpace(profile.OSSEndpoint)
		if endpoint == "" {
			endpoint = "https://oss-ap-southeast-1.aliyuncs.com"
		}
		ak = strings.TrimSpace(profile.AccessKeyID)
		sk = strings.TrimSpace(profile.AccessKeySecret)
		slog.Info("billing ETL: OSS sync start", "mode", "yaml_profile", "env_key", envKey, "project_id", profile.ProjectID, "db_account_id", dbAccountID, "bucket", bucket)
	} else {
		bucket = strings.TrimSpace(os.Getenv("OSS_BILLING_BUCKET"))
		if bucket == "" {
			return nil
		}
		only := strings.TrimSpace(os.Getenv("OSS_BILLING_ONLY_ENV"))
		if only != "" && !strings.EqualFold(only, strings.TrimSpace(envKey)) {
			slog.Debug("billing ETL: OSS sync skipped (OSS_BILLING_ONLY_ENV)", "env_key", envKey, "only", only)
			return nil
		}
		credEnv := ossBillingCredentialEnv(envKey)
		ak, sk = ossfinops.EnvForFinOps(credEnv)
		if ak == "" || sk == "" {
			slog.Warn("billing ETL: OSS_BILLING_BUCKET set but missing AK/SK for OSS credential env", "worker_env", envKey, "oss_credential_env", credEnv,
				"hint", "set OSS_BILLING_CREDENTIAL_ENV or ALIBABA_CLOUD_ACCESS_KEY_ID_"+credEnv+" / SECRET")
			return nil
		}
		slog.Info("billing ETL: OSS sync start", "mode", "global_env", "worker_env", envKey, "oss_credential_env", credEnv, "db_account_id", dbAccountID)
		endpoint = strings.TrimSpace(os.Getenv("OSS_ENDPOINT"))
		if endpoint == "" {
			endpoint = "https://oss-ap-southeast-1.aliyuncs.com"
		}
		prefix = strings.TrimSpace(os.Getenv("OSS_BILLING_PREFIX"))
		if prefix == "" {
			prefix = "billing-data/"
		}
	}
	if ak == "" || sk == "" {
		slog.Warn("billing ETL: OSS sync missing AK/SK", "env_key", envKey)
		return nil
	}
	syncMode := strings.TrimSpace(os.Getenv("OSS_SYNC_MODE"))
	if syncMode == "" {
		syncMode = "all"
	}
	// 窄接口避免与 mock 强耦合（含关账全量 Replace）。[Ref: 04_采集 §六]
	fw, ok := repo.(interface {
		DeleteFinOpsBillingFactsByBillingCycle(ctx context.Context, billingCycle, accountID string) error
		BulkInsertFinOpsBillingFacts(ctx context.Context, rows []postgres.FinOpsBillingFactRow) error
		ReplaceFinOpsBillingCycleWithFacts(ctx context.Context, billingCycle, accountID string, rows []postgres.FinOpsBillingFactRow) error
	})
	if !ok {
		slog.Warn("billing ETL: pipeline repo does not support finops_billing_fact")
		return nil
	}
	var incrementalSince time.Time
	if envTruthy("OSS_INCREMENTAL_SYNC") && !envTruthy("OSS_FULL_SYNC") {
		t, ok, err := repo.GetFinOpsOSSSyncCheckpoint(ctx, dbAccountID)
		if err != nil {
			return fmt.Errorf("oss sync checkpoint read: %w", err)
		}
		if ok {
			incrementalSince = t
		}
	}
	maxLM, err := ossfinops.LoadBillingCSVsFromOSS(ctx, fw, ossfinops.Config{
		Endpoint:         endpoint,
		AccessKey:        ak,
		SecretKey:        sk,
		Bucket:           bucket,
		Prefix:           prefix,
		AccountID:        dbAccountID,
		SyncMode:         syncMode,
		Now:              time.Now().UTC(),
		IncrementalSince: incrementalSince,
	})
	if err != nil {
		return err
	}
	if envTruthy("OSS_INCREMENTAL_SYNC") && !envTruthy("OSS_FULL_SYNC") && !maxLM.IsZero() {
		if err := repo.SetFinOpsOSSSyncCheckpoint(ctx, dbAccountID, maxLM); err != nil {
			return fmt.Errorf("oss sync checkpoint write: %w", err)
		}
	}
	return nil
}

// RunOSSBillingSyncFromEnv 仅执行 OSS→finops_billing_fact（与 Pipeline ⓪ 步中 OSS 段一致），供 CLI 手工补数；profile 非 nil 时使用 YAML 桶。[Ref: 04_采集 §七]
func RunOSSBillingSyncFromEnv(ctx context.Context, repo CloudBillPipelineRepository, envKey, dbAccountID string, profile *ProjectCloudProfile) error {
	return syncOSSBillingDetailIfConfigured(ctx, repo, envKey, dbAccountID, profile)
}
