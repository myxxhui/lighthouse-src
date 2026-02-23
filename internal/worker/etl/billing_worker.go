// Package etl 提供云账单 ETL（Phase4 01_）。按日/账期拉取，落表 cost_cloud_bill_summary，对账与差异告警（文档 4.1 第3条）。
package etl

import (
	"context"
	"log/slog"
	"math"
	"time"

	"github.com/myxxhui/lighthouse-src/internal/data/cloudbilling"
	"github.com/myxxhui/lighthouse-src/internal/data/postgres"
)

// CloudBillRepository 仅需云账单落库能力，便于 ETL 依赖。
type CloudBillRepository interface {
	SaveCloudBillSummary(ctx context.Context, s postgres.CloudBillSummary) error
}

// ReconciliationThreshold 对账偏差告警阈值（与云控制台偏差 < 1% 或已记录并告警）。
const ReconciliationThreshold = 0.01

// BillingWorker 云账单 ETL：拉取总账与按产品占比，写入 cost_cloud_bill_summary，并对账（差异 >1% 时记录并告警）。
type BillingWorker struct {
	fetcher   cloudbilling.CloudBillingFetcher
	repo      CloudBillRepository
	billingCycle string
	// 对账与告警（文档 4.1 第3条）：ExpectedTotal>0 时与拉取结果比较，超 1% 记录并调用 OnReconcileAlert
	ExpectedTotal   float64
	OnReconcileAlert func(actual, expected float64, diffPct float64)
}

// NewBillingWorker 创建云账单 ETL。fetcher 为 nil 时不执行拉取。
func NewBillingWorker(fetcher cloudbilling.CloudBillingFetcher, repo CloudBillRepository, billingCycle string) *BillingWorker {
	return &BillingWorker{fetcher: fetcher, repo: repo, billingCycle: billingCycle}
}

// Run 执行一次拉取并落库；若设置了 ExpectedTotal>0 则对账，偏差 >1% 时记录并触发 OnReconcileAlert。建议由 cron 每日 02:00 调用。
func (w *BillingWorker) Run(ctx context.Context) error {
	if w.fetcher == nil {
		return nil
	}
	cycle := w.billingCycle
	if cycle == "" {
		cycle = time.Now().Format("2006-01")
	}
	day := time.Now().UTC().Truncate(24 * time.Hour)
	req := cloudbilling.FetchAccountSummaryRequest{
		BillingCycle: cycle,
		PeriodType:   "month",
	}
	resp, err := w.fetcher.FetchAccountSummary(ctx, req)
	if err != nil {
		slog.Warn("billing ETL: fetch failed", "billing_cycle", cycle, "error", err)
		return err
	}
	summary := postgres.CloudBillSummary{
		Day:              day,
		BillingCycle:     resp.BillingCycle,
		TotalAmount:      resp.TotalAmount,
		ProductBreakdown: resp.ByCategory,
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
	}
	if err := w.repo.SaveCloudBillSummary(ctx, summary); err != nil {
		slog.Warn("billing ETL: save failed", "error", err)
		return err
	}
	slog.Info("billing ETL: saved cloud bill summary", "day", day.Format("2006-01-02"), "billing_cycle", resp.BillingCycle, "total", resp.TotalAmount)

	// 对账：与预期总金额比较，偏差 >1% 时记录并告警（文档 4.1 第3条、5.1 云账单接入）
	if w.ExpectedTotal > 0 {
		diffPct := math.Abs(resp.TotalAmount - w.ExpectedTotal) / w.ExpectedTotal
		if diffPct > ReconciliationThreshold {
			slog.Warn("billing ETL: reconciliation diff over 1%", "actual", resp.TotalAmount, "expected", w.ExpectedTotal, "diff_pct", diffPct)
			if w.OnReconcileAlert != nil {
				w.OnReconcileAlert(resp.TotalAmount, w.ExpectedTotal, diffPct)
			}
		}
	}
	return nil
}
