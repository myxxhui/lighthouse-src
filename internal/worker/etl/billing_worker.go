// Package etl 提供云账单 ETL（Phase4 01_）。按日/账期拉取，落表 cost_cloud_bill_summary 或 06_ 三表；固定 ETL 顺序与缺日保护（D2）。
package etl

import (
	"context"
	"fmt"
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

// CloudBillPipelineRepository 06_ 三表 ETL 五步流水线与对账所需能力（D2/D3）。可选实现。
type CloudBillPipelineRepository interface {
	SaveCloudBillDailyRaw(ctx context.Context, r postgres.CloudBillDailyRaw) error
	GetCloudBillDailyRaw(ctx context.Context, billDate time.Time) (*postgres.CloudBillDailyRaw, error)
	DeleteCloudBillDailyRawForDate(ctx context.Context, billDate time.Time) error
	ListMissingCloudBillDailyDates(ctx context.Context, from, to time.Time) ([]time.Time, error)
	SaveCloudBillMonthlyRaw(ctx context.Context, r postgres.CloudBillMonthlyRaw) error
	GetCloudBillMonthlyRaw(ctx context.Context, billingCycle string) (*postgres.CloudBillMonthlyRaw, error)
	SaveCloudBillAggregate(ctx context.Context, a postgres.CloudBillAggregate) error
	GetCloudBillAggregate(ctx context.Context, reportType, periodKey string) (*postgres.CloudBillAggregate, error)
	DeleteCloudBillAggregateExcept(ctx context.Context, reportType string, keepPeriodKeys []string) error
	ListCloudBillDailyRawFromTo(ctx context.Context, from, to time.Time) ([]postgres.CloudBillDailyRaw, error)
}

// ReconciliationThreshold 对账偏差告警阈值（与云控制台偏差 < 1% 或已记录并告警）。
const ReconciliationThreshold = 0.01

// BillingWorker 云账单 ETL：拉取总账与按产品占比，写入 cost_cloud_bill_summary 或 06_ 三表；支持固定五步流水线（D2）。
// AccountID 非空时写入日/月/聚合表带 account_id，供按环境总账（env_breakdown）匹配 cost_env_account_config。[Ref: 01_设计 §按环境展示]
type BillingWorker struct {
	fetcher             cloudbilling.CloudBillingFetcher
	repo                CloudBillRepository
	pipelineRepo        CloudBillPipelineRepository // 非 nil 时 RunPipeline 可用
	billingCycle        string
	AccountID           string                     // 多账号或按环境拉取时填写（如 POC），与 cost_env_account_config.account_id 对应
	ExpectedTotal       float64
	OnReconcileAlert    func(actual, expected float64, diffPct float64)
	OnPipelineFailAlert func(step string, err error) // D1-1：聚合/ETL 失败时告警，与 04_ 监控集成
}

// NewBillingWorker 创建云账单 ETL。fetcher 为 nil 时不执行拉取。repo 若实现 CloudBillPipelineRepository 则支持 RunPipeline。
func NewBillingWorker(fetcher cloudbilling.CloudBillingFetcher, repo CloudBillRepository, billingCycle string) *BillingWorker {
	w := &BillingWorker{fetcher: fetcher, repo: repo, billingCycle: billingCycle}
	if pr, ok := repo.(CloudBillPipelineRepository); ok {
		w.pipelineRepo = pr
	}
	return w
}

// Run 执行一次拉取并落库；若设置了 ExpectedTotal>0 则对账，偏差 >1% 时记录并触发 OnReconcileAlert。建议由 cron 每日 02:00 调用。D7-1：打点开始/结束、period、错误码。
func (w *BillingWorker) Run(ctx context.Context) error {
	if w.fetcher == nil {
		return nil
	}
	cycle := w.billingCycle
	if cycle == "" {
		cycle = time.Now().Format("2006-01")
	}
	day := time.Now().UTC().Truncate(24 * time.Hour)
	startAt := time.Now()
	slog.Info("billing ETL: start", "billing_cycle", cycle, "day", day.Format("2006-01-02"))
	defer func() {
		slog.Info("billing ETL: end", "billing_cycle", cycle, "duration_ms", time.Since(startAt).Milliseconds())
	}()
	req := cloudbilling.FetchAccountSummaryRequest{
		BillingCycle: cycle,
		PeriodType:   "month",
	}
	resp, err := w.fetcher.FetchAccountSummary(ctx, req)
	if err != nil {
		slog.Warn("billing ETL: fetch failed", "billing_cycle", cycle, "error", err)
		return err
	}
	// 合并领域汇总与产品级明细：product_breakdown 存 domain 与 "domain:ProductCode"，供 API 返回 top4 产品 [Ref: 01_成本透视真实数据]
	merged := make(map[string]float64)
	for k, v := range resp.ByCategory {
		merged[k] = v
	}
	for _, it := range resp.Items {
		merged[it.Category+":"+it.ProductCode] = it.Amount
	}
	summary := postgres.CloudBillSummary{
		Day:              day,
		BillingCycle:     resp.BillingCycle,
		TotalAmount:      resp.TotalAmount,
		ProductBreakdown: merged,
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

// RunPipeline 执行固定 ETL 顺序（06_ D2）：① 写昨日日原始 ② 校验昨日成功 ③ 删 10 个月前当日（缺日时不删）④ 写月原始 ⑤ 触发聚合。需 pipelineRepo 非 nil。D7-1：打点开始/结束、period、行数、错误码。
func (w *BillingWorker) RunPipeline(ctx context.Context) error {
	if w.pipelineRepo == nil {
		slog.Debug("billing ETL: pipeline repo nil, skip RunPipeline")
		return nil
	}
	if w.fetcher == nil {
		return nil
	}
	now := time.Now().UTC()
	yesterday := now.AddDate(0, 0, -1).Truncate(24 * time.Hour)
	cut10m := now.AddDate(0, -10, 0).Truncate(24 * time.Hour)
	startAt := time.Now()
	periodDay := yesterday.Format("2006-01-02")
	periodMonth := now.Format("2006-01")
	slog.Info("billing ETL pipeline: start", "period_day", periodDay, "period_month", periodMonth)
	defer func() {
		slog.Info("billing ETL pipeline: end", "duration_ms", time.Since(startAt).Milliseconds(), "period_day", periodDay, "period_month", periodMonth)
	}()

	// ① 写昨日日原始
	reqDay := cloudbilling.FetchAccountSummaryRequest{
		BillingCycle: yesterday.Format("2006-01-02"),
		PeriodType:   "day",
	}
	respDay, err := w.fetcher.FetchAccountSummary(ctx, reqDay)
	if err != nil {
		slog.Warn("billing ETL pipeline: step1 fetch yesterday failed", "yesterday", yesterday.Format("2006-01-02"), "error", err)
		// 继续执行 ②③④⑤，步骤 ② 会因无昨日数据而跳过删除
	} else {
		merged := make(map[string]float64)
		for k, v := range respDay.ByCategory {
			merged[k] = v
		}
		for _, it := range respDay.Items {
			merged[it.Category+":"+it.ProductCode] = it.Amount
		}
		snap := time.Now()
		err = w.pipelineRepo.SaveCloudBillDailyRaw(ctx, postgres.CloudBillDailyRaw{
			BillDate:         yesterday,
			TotalAmount:      respDay.TotalAmount,
			ProductBreakdown: merged,
			SnapshotAt:       snap,
			CreatedAt:        snap,
			AccountID:        w.AccountID,
		})
		if err != nil {
			slog.Warn("billing ETL pipeline: step1 save daily raw failed", "error", err)
		} else {
			slog.Info("billing ETL pipeline: step1 saved daily raw", "bill_date", yesterday.Format("2006-01-02"), "total", respDay.TotalAmount)
		}
	}

	// ② 校验昨日写入成功
	got, _ := w.pipelineRepo.GetCloudBillDailyRaw(ctx, yesterday)
	ok := got != nil && got.TotalAmount >= 0
	if !ok {
		slog.Warn("billing ETL pipeline: step2 yesterday not valid, skip delete")
	}

	// ③ 缺日检测；无缺日时删除 10 个月前当日
	if ok {
		missing, err := w.pipelineRepo.ListMissingCloudBillDailyDates(ctx, cut10m, now)
		if err != nil {
			slog.Warn("billing ETL pipeline: step3 list missing failed", "error", err)
		} else if len(missing) > 0 {
			slog.Warn("billing ETL pipeline: step3 missing dates, skip delete", "count", len(missing), "sample", missing[0].Format("2006-01-02"))
		} else {
			if err := w.pipelineRepo.DeleteCloudBillDailyRawForDate(ctx, cut10m); err != nil {
				slog.Warn("billing ETL pipeline: step3 delete failed", "date", cut10m.Format("2006-01-02"), "error", err)
			} else {
				slog.Info("billing ETL pipeline: step3 deleted daily raw", "date", cut10m.Format("2006-01-02"))
			}
		}
	}

	// ④ 写月原始（当前账期）
	cycle := w.billingCycle
	if cycle == "" {
		cycle = now.Format("2006-01")
	}
	reqMon := cloudbilling.FetchAccountSummaryRequest{BillingCycle: cycle, PeriodType: "month"}
	respMon, err := w.fetcher.FetchAccountSummary(ctx, reqMon)
	if err != nil {
		slog.Warn("billing ETL pipeline: step4 fetch month failed", "billing_cycle", cycle, "error", err)
	} else {
		merged := make(map[string]float64)
		for k, v := range respMon.ByCategory {
			merged[k] = v
		}
		for _, it := range respMon.Items {
			merged[it.Category+":"+it.ProductCode] = it.Amount
		}
		snap := time.Now()
		if err := w.pipelineRepo.SaveCloudBillMonthlyRaw(ctx, postgres.CloudBillMonthlyRaw{
			BillingCycle:     respMon.BillingCycle,
			TotalAmount:      respMon.TotalAmount,
			ProductBreakdown: merged,
			SnapshotAt:       snap,
			CreatedAt:        snap,
			AccountID:        w.AccountID,
		}); err != nil {
			slog.Warn("billing ETL pipeline: step4 save monthly raw failed", "error", err)
		} else {
			slog.Info("billing ETL pipeline: step4 saved monthly raw", "billing_cycle", respMon.BillingCycle, "total", respMon.TotalAmount)
		}
	}

	// ⑤ 触发聚合 [Ref: 04_01_成本透视真实数据 展示与延迟说明]：1d/7d/30d/month/quarter 写入 cost_cloud_bill_aggregate，API 常规展示仅读此表
	successAt := time.Now()
	today := now.Format("2006-01-02")
	yesterdayStr := now.AddDate(0, 0, -1).Format("2006-01-02")
	month := now.Format("2006-01")
	q := (int(now.Month())-1)/3 + 1
	quarterKey := fmt.Sprintf("%s-Q%d", now.Format("2006"), q)

	saveAgg := func(reportType, periodKey string, total float64, byCat map[string]float64) error {
		if err := w.pipelineRepo.SaveCloudBillAggregate(ctx, postgres.CloudBillAggregate{
			ReportType:       reportType,
			PeriodKey:        periodKey,
			TotalAmount:      total,
			ProductBreakdown: byCat,
			LastSuccessAt:    &successAt,
			CreatedAt:        successAt,
			UpdatedAt:        successAt,
			AccountID:        w.AccountID,
		}); err != nil {
			return err
		}
		return w.pipelineRepo.DeleteCloudBillAggregateExcept(ctx, reportType, []string{periodKey})
	}
	// saveAggWithPrev 写入当前与上一周期并保留两行，供 compare_mode=previous 使用 [Ref: 01_设计 §上一周期推导规则]
	saveAggWithPrev := func(reportType, curKey string, curTotal float64, curByCat map[string]float64, prevKey string, prevTotal float64, prevByCat map[string]float64) error {
		if err := w.pipelineRepo.SaveCloudBillAggregate(ctx, postgres.CloudBillAggregate{
			ReportType: reportType, PeriodKey: curKey, TotalAmount: curTotal, ProductBreakdown: curByCat,
			LastSuccessAt: &successAt, CreatedAt: successAt, UpdatedAt: successAt, AccountID: w.AccountID,
		}); err != nil {
			return err
		}
		if prevKey != curKey {
			if err := w.pipelineRepo.SaveCloudBillAggregate(ctx, postgres.CloudBillAggregate{
				ReportType: reportType, PeriodKey: prevKey, TotalAmount: prevTotal, ProductBreakdown: prevByCat,
				LastSuccessAt: &successAt, CreatedAt: successAt, UpdatedAt: successAt, AccountID: w.AccountID,
			}); err != nil {
				return err
			}
		}
		keep := []string{curKey}
		if prevKey != curKey {
			keep = append(keep, prevKey)
		}
		return w.pipelineRepo.DeleteCloudBillAggregateExcept(ctx, reportType, keep)
	}

	mergeDailyRows := func(rows []postgres.CloudBillDailyRaw) (float64, map[string]float64) {
		var total float64
		byCat := make(map[string]float64)
		for _, r := range rows {
			total += r.TotalAmount
			for k, v := range r.ProductBreakdown {
				byCat[k] += v
			}
		}
		return total, byCat
	}

	var pipelineErr error

	// 30d：最近 30 天
	from30 := now.AddDate(0, 0, -30)
	rows30, err := w.pipelineRepo.ListCloudBillDailyRawFromTo(ctx, from30, now)
	if err != nil {
		slog.Warn("billing ETL pipeline: step5 list daily raw failed", "error", err, "error_code", errCode(err))
		return nil
	}
	total30, byCat30 := mergeDailyRows(rows30)
	from30Prev := now.AddDate(0, 0, -60)
	to30Prev := now.AddDate(0, 0, -30)
	rows30Prev, _ := w.pipelineRepo.ListCloudBillDailyRawFromTo(ctx, from30Prev, to30Prev)
	total30Prev, byCat30Prev := mergeDailyRows(rows30Prev)
	if err := saveAggWithPrev("30d", today, total30, byCat30, to30Prev.Format("2006-01-02"), total30Prev, byCat30Prev); err != nil {
		slog.Warn("billing ETL pipeline: step5 save aggregate 30d failed", "error", err)
		pipelineErr = err
		if w.OnPipelineFailAlert != nil {
			w.OnPipelineFailAlert("aggregate", err)
		}
	} else {
		slog.Info("billing ETL pipeline: step5 saved aggregate", "report_type", "30d", "period_key", today, "total", total30, "rows_processed", len(rows30))
	}

	// 1d：昨日；保留上一日供对比
	yesterdayT, _ := time.Parse("2006-01-02", yesterdayStr)
	rows1d, _ := w.pipelineRepo.ListCloudBillDailyRawFromTo(ctx, yesterdayT, yesterdayT)
	total1d, byCat1d := mergeDailyRows(rows1d)
	dayBefore := yesterdayT.AddDate(0, 0, -1)
	rows1dPrev, _ := w.pipelineRepo.ListCloudBillDailyRawFromTo(ctx, dayBefore, dayBefore)
	total1dPrev, byCat1dPrev := mergeDailyRows(rows1dPrev)
	_ = saveAggWithPrev("1d", yesterdayStr, total1d, byCat1d, dayBefore.Format("2006-01-02"), total1dPrev, byCat1dPrev)

	// 7d：近 7 天；保留上一段 7 天供对比
	from7 := now.AddDate(0, 0, -7)
	rows7, _ := w.pipelineRepo.ListCloudBillDailyRawFromTo(ctx, from7, now)
	total7, byCat7 := mergeDailyRows(rows7)
	from7Prev := now.AddDate(0, 0, -14)
	to7Prev := now.AddDate(0, 0, -7)
	rows7Prev, _ := w.pipelineRepo.ListCloudBillDailyRawFromTo(ctx, from7Prev, to7Prev)
	total7Prev, byCat7Prev := mergeDailyRows(rows7Prev)
	_ = saveAggWithPrev("7d", today, total7, byCat7, to7Prev.Format("2006-01-02"), total7Prev, byCat7Prev)

	// 上周（ISO 周）日期范围，供 this_week 上一周期与 last_week 使用
	weekday := int(now.Weekday())
	if weekday == 0 {
		weekday = 7
	}
	lastWeekEnd := now.AddDate(0, 0, -weekday)
	lastWeekStart := lastWeekEnd.AddDate(0, 0, -6)
	rowsLastWeek, _ := w.pipelineRepo.ListCloudBillDailyRawFromTo(ctx, lastWeekStart, lastWeekEnd)
	totalLastWeek, byCatLastWeek := mergeDailyRows(rowsLastWeek)
	lastWeekEndStr := lastWeekEnd.Format("2006-01-02")

	// this_week：这周（ISO 周周一至昨日）[Ref: 01_设计 这周与近七天必须区分]
	daysBack := int(weekday - 1)
	thisWeekMonday := now.AddDate(0, 0, -daysBack).Truncate(24 * time.Hour)
	rowsThisWeek, _ := w.pipelineRepo.ListCloudBillDailyRawFromTo(ctx, thisWeekMonday, yesterday)
	totalThisWeek, byCatThisWeek := mergeDailyRows(rowsThisWeek)
	year, week := now.ISOWeek()
	thisWeekKey := fmt.Sprintf("%04d-W%02d", year, week)
	var thisWeekPrevKey string
	if week > 1 {
		thisWeekPrevKey = fmt.Sprintf("%04d-W%02d", year, week-1)
	} else {
		thisWeekPrevKey = fmt.Sprintf("%04d-W52", year-1)
	}
	_ = saveAggWithPrev("this_week", thisWeekKey, totalThisWeek, byCatThisWeek, thisWeekPrevKey, totalLastWeek, byCatLastWeek)

	// last_week：上周；保留上上周供对比
	lastWeekStart2 := lastWeekStart.AddDate(0, 0, -7)
	lastWeekEnd2 := lastWeekEnd.AddDate(0, 0, -7)
	rowsLastWeek2, _ := w.pipelineRepo.ListCloudBillDailyRawFromTo(ctx, lastWeekStart2, lastWeekEnd2)
	totalLastWeek2, byCatLastWeek2 := mergeDailyRows(rowsLastWeek2)
	_ = saveAggWithPrev("last_week", lastWeekEndStr, totalLastWeek, byCatLastWeek, lastWeekEnd2.Format("2006-01-02"), totalLastWeek2, byCatLastWeek2)

	// month：当月 1 日至昨日，日原始表叠加 [Ref: 01_设计 §后端数据聚合与存储方案 这月数据来源日原始表]
	firstOfMonth := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	rowsMonth, _ := w.pipelineRepo.ListCloudBillDailyRawFromTo(ctx, firstOfMonth, yesterdayT)
	totalMonth, byCatMonth := mergeDailyRows(rowsMonth)
	if totalMonth > 0 || len(byCatMonth) > 0 {
		_ = saveAgg("month", month, totalMonth, byCatMonth)
	}

	// quarter：本季度 1 日至昨日，日原始表叠加 [Ref: 01_设计 §后端数据聚合与存储方案 这季度数据来源日原始表]
	firstOfQuarter := time.Date(now.Year(), time.Month((q-1)*3+1), 1, 0, 0, 0, 0, time.UTC)
	if firstOfQuarter.After(yesterdayT) {
		firstOfQuarter = time.Date(now.Year()-1, 10, 1, 0, 0, 0, 0, time.UTC) // 本季首日未到则用上季（不应出现）
	}
	rowsQuarter, _ := w.pipelineRepo.ListCloudBillDailyRawFromTo(ctx, firstOfQuarter, yesterdayT)
	totalQ, byCatQ := mergeDailyRows(rowsQuarter)
	if totalQ > 0 || len(byCatQ) > 0 {
		_ = saveAgg("quarter", quarterKey, totalQ, byCatQ)
	}

	// 90d：近 90 天 [Ref: 01_设计 report_type 与 period_key]
	from90 := now.AddDate(0, 0, -90)
	rows90, _ := w.pipelineRepo.ListCloudBillDailyRawFromTo(ctx, from90, now)
	total90, byCat90 := mergeDailyRows(rows90)
	_ = saveAgg("90d", today, total90, byCat90)

	// last_month：上月（月原始表）[Ref: 01_设计 §后端数据聚合与存储方案]
	prevMonth := now.AddDate(0, -1, 0).Format("2006-01")
	if mon, _ := w.pipelineRepo.GetCloudBillMonthlyRaw(ctx, prevMonth); mon != nil && (mon.TotalAmount > 0 || len(mon.ProductBreakdown) > 0) {
		_ = saveAgg("last_month", prevMonth, mon.TotalAmount, mon.ProductBreakdown)
	}

	// last_quarter：上季度三月汇总
	prevCycles := []string{now.AddDate(0, -1, 0).Format("2006-01"), now.AddDate(0, -2, 0).Format("2006-01"), now.AddDate(0, -3, 0).Format("2006-01")}
	var totalPrevQ float64
	byCatPrevQ := make(map[string]float64)
	for _, c := range prevCycles {
		m, _ := w.pipelineRepo.GetCloudBillMonthlyRaw(ctx, c)
		if m != nil {
			totalPrevQ += m.TotalAmount
			for k, v := range m.ProductBreakdown {
				byCatPrevQ[k] += v
			}
		}
	}
	currQ := (int(now.Month())-1)/3 + 1
	prevQ := currQ - 1
	prevY := now.Year()
	if prevQ <= 0 {
		prevQ = 4
		prevY = now.Year() - 1
	}
	prevQuarterKey := fmt.Sprintf("%d-Q%d", prevY, prevQ)
	if totalPrevQ > 0 || len(byCatPrevQ) > 0 {
		_ = saveAgg("last_quarter", prevQuarterKey, totalPrevQ, byCatPrevQ)
	}

	return pipelineErr
}

func errCode(err error) string {
	if err == nil {
		return ""
	}
	return "err"
}

// MinDailyRowsForIncremental 全量检查：近 30 天内至少有的日原始条数，不足则执行全量回填。[Ref: 01_实践 部署与每日凌晨全量检查]
const MinDailyRowsForIncremental = 7

// RecentMonthsForIncremental 全量检查：最近 N 个月月原始均存在才认为不需全量；月账单当月通常未出，故检查上月、前两月、前三月。[Ref: 01_实践 部署与每日凌晨全量检查]
const RecentMonthsForIncremental = 3

// NeedsFullBackfill 判断全量数据是否按预期存在：近 30 天日原始不少于 MinDailyRowsForIncremental，且最近 RecentMonthsForIncremental 个月（上月、前两月、前三月）月原始均存在。不满足则需执行全量拉取+聚合。
func (w *BillingWorker) NeedsFullBackfill(ctx context.Context) (bool, error) {
	if w.pipelineRepo == nil {
		return false, nil
	}
	now := time.Now().UTC()
	from30 := now.AddDate(0, 0, -30)
	rows, err := w.pipelineRepo.ListCloudBillDailyRawFromTo(ctx, from30, now)
	if err != nil {
		return true, err
	}
	if len(rows) < MinDailyRowsForIncremental {
		slog.Info("billing full check: need full backfill", "reason", "daily_rows_below_min", "count", len(rows), "min", MinDailyRowsForIncremental)
		return true, nil
	}
	// 月账单当月通常未出，检查最近三个月（上月、前两月、前三月）是否存在
	for i := 1; i <= RecentMonthsForIncremental; i++ {
		cycle := now.AddDate(0, -i, 0).Format("2006-01")
		m, _ := w.pipelineRepo.GetCloudBillMonthlyRaw(ctx, cycle)
		if m == nil {
			slog.Info("billing full check: need full backfill", "reason", "monthly_missing", "cycle", cycle, "recent_months", RecentMonthsForIncremental)
			return true, nil
		}
	}
	slog.Debug("billing full check: incremental OK", "daily_rows_30d", len(rows))
	return false, nil
}

// FullBackfillRateLimit 全量回填时请求间隔（15_ 约 10 笔/秒，此处取 100ms）。[Ref: D2-6]
const FullBackfillRateLimit = 100 * time.Millisecond

// RunFullBackfill 首次或按需全量回填（D2-6）：按日限流拉取 10 个月日数据、5 年月数据，落库后执行一次聚合。部署/每日凌晨检查不通过时自动调用。
// 需 pipelineRepo 非 nil、fetcher 非 nil。建议业务低峰执行。
func (w *BillingWorker) RunFullBackfill(ctx context.Context) error {
	if w.pipelineRepo == nil || w.fetcher == nil {
		slog.Info("billing full backfill: pipelineRepo or fetcher nil, skip")
		return nil
	}
	now := time.Now().UTC()
	startAt := time.Now()
	slog.Info("billing full backfill: start")
	defer func() {
		slog.Info("billing full backfill: end", "duration_ms", time.Since(startAt).Milliseconds())
	}()

	// 10 个月日数据：按日循环 + 限流
	fromDay := now.AddDate(0, -10, 0)
	fromDay = time.Date(fromDay.Year(), fromDay.Month(), fromDay.Day(), 0, 0, 0, 0, time.UTC)
	for d := fromDay; !d.After(now); d = d.AddDate(0, 0, 1) {
		if err := ctx.Err(); err != nil {
			return err
		}
		req := cloudbilling.FetchAccountSummaryRequest{BillingCycle: d.Format("2006-01-02"), PeriodType: "day"}
		resp, err := w.fetcher.FetchAccountSummary(ctx, req)
		if err != nil {
			slog.Warn("billing full backfill: fetch day failed", "date", d.Format("2006-01-02"), "error", err)
			time.Sleep(FullBackfillRateLimit * 2) // 退避后继续
			continue
		}
		merged := make(map[string]float64)
		for k, v := range resp.ByCategory {
			merged[k] = v
		}
		for _, it := range resp.Items {
			merged[it.Category+":"+it.ProductCode] = it.Amount
		}
		snap := time.Now()
		if err := w.pipelineRepo.SaveCloudBillDailyRaw(ctx, postgres.CloudBillDailyRaw{
			BillDate:         d,
			TotalAmount:      resp.TotalAmount,
			ProductBreakdown: merged,
			SnapshotAt:       snap,
			CreatedAt:        snap,
			AccountID:        w.AccountID,
		}); err != nil {
			slog.Warn("billing full backfill: save daily failed", "date", d.Format("2006-01-02"), "error", err)
		}
		time.Sleep(FullBackfillRateLimit)
	}

	// 5 年月数据
	for i := 0; i < 60; i++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		t := now.AddDate(0, -59+i, 0)
		cycle := t.Format("2006-01")
		req := cloudbilling.FetchAccountSummaryRequest{BillingCycle: cycle, PeriodType: "month"}
		resp, err := w.fetcher.FetchAccountSummary(ctx, req)
		if err != nil {
			slog.Warn("billing full backfill: fetch month failed", "billing_cycle", cycle, "error", err)
			time.Sleep(FullBackfillRateLimit * 2)
			continue
		}
		merged := make(map[string]float64)
		for k, v := range resp.ByCategory {
			merged[k] = v
		}
		for _, it := range resp.Items {
			merged[it.Category+":"+it.ProductCode] = it.Amount
		}
		snap := time.Now()
		if err := w.pipelineRepo.SaveCloudBillMonthlyRaw(ctx, postgres.CloudBillMonthlyRaw{
			BillingCycle:     resp.BillingCycle,
			TotalAmount:      resp.TotalAmount,
			ProductBreakdown: merged,
			SnapshotAt:       snap,
			CreatedAt:        snap,
			AccountID:        w.AccountID,
		}); err != nil {
			slog.Warn("billing full backfill: save monthly failed", "billing_cycle", cycle, "error", err)
		}
		time.Sleep(FullBackfillRateLimit)
	}

	// 一次聚合：最近 30 天 → 30d
	from30 := now.AddDate(0, 0, -30)
	rows, err := w.pipelineRepo.ListCloudBillDailyRawFromTo(ctx, from30, now)
	if err != nil {
		slog.Warn("billing full backfill: list daily for aggregate failed", "error", err)
		return err
	}
	var total float64
	byCat := make(map[string]float64)
	for _, r := range rows {
		total += r.TotalAmount
		for k, v := range r.ProductBreakdown {
			byCat[k] += v
		}
	}
	successAt := time.Now()
	if err := w.pipelineRepo.SaveCloudBillAggregate(ctx, postgres.CloudBillAggregate{
		ReportType:       "30d",
		PeriodKey:        now.Format("2006-01-02"),
		TotalAmount:      total,
		ProductBreakdown: byCat,
		LastSuccessAt:    &successAt,
		CreatedAt:        successAt,
		UpdatedAt:        successAt,
		AccountID:        w.AccountID,
	}); err != nil {
		slog.Warn("billing full backfill: save aggregate failed", "error", err)
		return err
	}
	slog.Info("billing full backfill: aggregate saved", "report_type", "30d", "rows", len(rows), "total", total)
	if err := w.pipelineRepo.DeleteCloudBillAggregateExcept(ctx, "30d", []string{now.Format("2006-01-02")}); err != nil {
		slog.Warn("billing full backfill: delete old aggregate failed", "error", err)
	}
	return nil
}

// ReconcileThreshold 对账偏差告警阈值 1%（06_ D3）。
const ReconcileThreshold = 0.01

// RunReconcile 每日对账：日原始表当月 sum(total_amount) vs 月原始表该月 total_amount；偏差 >1% 告警并记日志（D3-1）。
// 可选：聚合表「本月」= 当月1日至昨日日表叠加 [Ref: 01_设计]，与日表当月1日至昨日 sum 交叉校验。
func (w *BillingWorker) RunReconcile(ctx context.Context) error {
	if w.pipelineRepo == nil {
		return nil
	}
	now := time.Now().UTC()
	cycle := now.Format("2006-01")
	first := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	yesterday := now.AddDate(0, 0, -1).Truncate(24 * time.Hour)
	rows, err := w.pipelineRepo.ListCloudBillDailyRawFromTo(ctx, first, now)
	if err != nil {
		slog.Warn("billing reconcile: list daily raw failed", "error", err)
		return err
	}
	var daySum float64
	for _, r := range rows {
		daySum += r.TotalAmount
	}
	mon, err := w.pipelineRepo.GetCloudBillMonthlyRaw(ctx, cycle)
	if err != nil || mon == nil {
		slog.Debug("billing reconcile: no monthly raw for cycle", "billing_cycle", cycle)
		return nil
	}
	monthTotal := mon.TotalAmount
	if monthTotal <= 0 {
		return nil
	}
	diffPct := math.Abs(daySum - monthTotal) / monthTotal
	if diffPct > ReconcileThreshold {
		slog.Warn("billing reconcile: diff over 1%", "daily_sum", daySum, "month_total", monthTotal, "diff_pct", diffPct, "billing_cycle", cycle)
		if w.OnReconcileAlert != nil {
			w.OnReconcileAlert(daySum, monthTotal, diffPct)
		}
	}
	// D3-2：聚合表「本月」= 当月1日至昨日叠加，与日表当月1日至昨日 sum 一致
	var daySumToYesterday float64
	for _, r := range rows {
		if !r.BillDate.After(yesterday) {
			daySumToYesterday += r.TotalAmount
		}
	}
	if agg, _ := w.pipelineRepo.GetCloudBillAggregate(ctx, "month", cycle); agg != nil && agg.TotalAmount > 0 {
		aggPct := math.Abs(daySumToYesterday-agg.TotalAmount) / agg.TotalAmount
		if aggPct > ReconcileThreshold {
			slog.Warn("billing reconcile: aggregate month vs daily sum (to yesterday) diff over 1%", "daily_sum_to_yesterday", daySumToYesterday, "aggregate_total", agg.TotalAmount, "diff_pct", aggPct)
		}
	}
	return nil
}
