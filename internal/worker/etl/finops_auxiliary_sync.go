// Package etl — FinOps 辅助同步：OSS 账单 CSV→finops_billing_fact、BSS P/B/U 落库。[Ref: 03_Phase6/01_FinOps 采集与ETL]
package etl

import (
	"context"
	"fmt"
	"log/slog"
	"os"
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
	// OSS CSV→finops_billing_fact 与 BSS API 解耦：无 Fetcher 时仍同步 OSS（仅须 bucket+AK）。OSS 失败不阻断后续 BSS。[Ref: 04_采集 §七]
	var ossErr error
	if err := syncOSSBillingDetailIfConfigured(ctx, w.pipelineRepo, w.EnvKey); err != nil {
		ossErr = err
	}
	if w.fetcher == nil {
		return ossErr
	}

	start := now.AddDate(0, 0, -14).UTC()
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

func envTruthy(name string) bool {
	v := strings.TrimSpace(strings.ToLower(os.Getenv(name)))
	return v == "1" || v == "true" || v == "yes"
}

// syncOSSBillingDetailIfConfigured 未配置 OSS_BILLING_BUCKET 时立即返回；已配置时流式拉取 Prefix 下 CSV→finops_billing_fact（行级 UPSERT）。失败返回 error（由 RunPipeline 告警，不阻断 API 主线）。[Ref: 04_采集 §5.6 §七]
func syncOSSBillingDetailIfConfigured(ctx context.Context, repo CloudBillPipelineRepository, accountID string) error {
	bucket := strings.TrimSpace(os.Getenv("OSS_BILLING_BUCKET"))
	if bucket == "" {
		return nil
	}
	ak, sk := ossfinops.EnvForFinOps(accountID)
	if ak == "" || sk == "" {
		slog.Warn("billing ETL: OSS_BILLING_BUCKET set but missing AK/SK for account", "account_id", accountID,
			"hint", "set ALIBABA_CLOUD_ACCESS_KEY_ID_"+accountID+" / SECRET")
		return nil
	}
	endpoint := strings.TrimSpace(os.Getenv("OSS_ENDPOINT"))
	if endpoint == "" {
		endpoint = "https://oss-ap-southeast-1.aliyuncs.com"
	}
	prefix := strings.TrimSpace(os.Getenv("OSS_BILLING_PREFIX"))
	if prefix == "" {
		prefix = "billing-data/"
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
	if envTruthy("OSS_INCREMENTAL_SYNC") {
		t, ok, err := repo.GetFinOpsOSSSyncCheckpoint(ctx, accountID)
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
		AccountID:        accountID,
		SyncMode:         syncMode,
		Now:              time.Now().UTC(),
		IncrementalSince: incrementalSince,
	})
	if err != nil {
		return err
	}
	if envTruthy("OSS_INCREMENTAL_SYNC") && !maxLM.IsZero() {
		if err := repo.SetFinOpsOSSSyncCheckpoint(ctx, accountID, maxLM); err != nil {
			return fmt.Errorf("oss sync checkpoint write: %w", err)
		}
	}
	return nil
}
