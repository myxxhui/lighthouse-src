// Package etl 提供云账单 ETL（Phase4 01_）。含 10步幂等流水线、7天窗口回溯、月度修复 Worker。
// [Ref: 16_云账单动态对账与高可靠处理规范] [Ref: 06_存储架构与ETL规范 §ETL顺序]
package etl

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"strings"
	"time"

	"github.com/myxxhui/lighthouse-src/internal/data/cloudbilling"
	"github.com/myxxhui/lighthouse-src/internal/data/postgres"
)

// CloudBillRepository 仅需云账单落库能力，便于 ETL 依赖。
type CloudBillRepository interface {
	SaveCloudBillSummary(ctx context.Context, s postgres.CloudBillSummary) error
}

// CloudBillPipelineRepository 05表 ETL 10步流水线与对账所需能力（D2/D3 + 16_）。可选实现。
type CloudBillPipelineRepository interface {
	SaveCloudBillDailyRaw(ctx context.Context, r postgres.CloudBillDailyRaw) error
	GetCloudBillDailyRaw(ctx context.Context, billDate time.Time, accountID string) (*postgres.CloudBillDailyRaw, error)
	DeleteCloudBillDailyRawForDate(ctx context.Context, billDate time.Time, accountID string) error
	ListMissingCloudBillDailyDates(ctx context.Context, from, to time.Time, accountID string) ([]time.Time, error)
	SaveCloudBillMonthlyRaw(ctx context.Context, r postgres.CloudBillMonthlyRaw) error
	GetCloudBillMonthlyRaw(ctx context.Context, billingCycle, accountID string) (*postgres.CloudBillMonthlyRaw, error)
	DeleteCloudBillMonthlyRawOlderThan(ctx context.Context, cutoffBillingCycle string, accountID string) error
	SaveCloudBillAggregate(ctx context.Context, a postgres.CloudBillAggregate) error
	GetCloudBillAggregate(ctx context.Context, reportType, periodKey string) (*postgres.CloudBillAggregate, error)
	DeleteCloudBillAggregateExcept(ctx context.Context, reportType string, keepPeriodKeys []string, accountID string) error
	ListCloudBillDailyRawFromTo(ctx context.Context, from, to time.Time, accountID string) ([]postgres.CloudBillDailyRaw, error)
	// 行级流水（16_ 新增）
	UpsertCloudBillLineItem(ctx context.Context, item postgres.CloudBillLineItem) error
	ListCloudBillLineItemsByDate(ctx context.Context, billDate time.Time, accountID string) ([]postgres.CloudBillLineItem, error)
	ListCloudBillLineItemsByBillingCycle(ctx context.Context, billingCycle, accountID string) ([]postgres.CloudBillLineItem, error)
	ListDistinctBillingCyclesInDateRange(ctx context.Context, from, to time.Time, accountID string) ([]string, error)
	SumLineItemsCashByBillingCycle(ctx context.Context, billingCycle, accountID string) (float64, error)
	DeleteLineItemsOlderThan(ctx context.Context, before time.Time, accountID string) error
	// 月度对账状态（16_ 新增）
	UpsertCloudBillMonthStatus(ctx context.Context, s postgres.CloudBillMonthStatus) error
	GetCloudBillMonthStatus(ctx context.Context, billingCycle, accountID string) (*postgres.CloudBillMonthStatus, error)
	// GetProductCategory 用于按账期汇总流水时按分类聚合 product_breakdown [Ref: 16_ §七]
	GetProductCategory(ctx context.Context, productCode string) (category string, ok bool)
	UpsertProductCategory(ctx context.Context, productCode, category string) error
}

// categoryToDomain 将 API 分类（英文）转为 product_breakdown 键前缀（中文领域），与 cost_service domainPrefixToCategory 对应。[Ref: 01_设计 §产品分类 方案 B]
func categoryToDomain(cat string) string {
	switch cat {
	case "compute":
		return "计算资源"
	case "storage":
		return "存储"
	case "network":
		return "网络"
	case "security":
		return "安全"
	case "其它", "其他":
		return "其他"
	default:
		return "其他"
	}
}

// ReconciliationThreshold 对账偏差告警阈值（相对 1%，与云控制台偏差 < 1% 或已记录并告警）。
const ReconciliationThreshold = 0.01

// ReconcileAbsThreshold 月度修复触发阈值（绝对值 0.01 元）。[Ref: 16_ §七]
const ReconcileAbsThreshold = 0.01

// WindowResyncDays 窗口回溯天数（T-2 至 T-7）。[Ref: 16_ §五]
const WindowResyncDays = 7

// DefaultDailyRetentionMonths 日表默认保留月数（36 个月）。[Ref: 01_实践]
const DefaultDailyRetentionMonths = 36

// DefaultMonthlyPullMonths 月表默认拉取/保留月数（60 个月 = 5 年）。[Ref: 01_实践 16_ §5.4]
const DefaultMonthlyPullMonths = 60

// ETLDataConfig 日/月拉取与保留月数，由配置文件或环境变量注入；0 表示使用默认值。[Ref: 01_实践 配置控制拉取与保存长度]
type ETLDataConfig struct {
	DailyPullMonths       int // 日表拉取月数（全量回填）
	DailyRetentionMonths  int // 日表保留月数，超期删除
	MonthlyPullMonths     int // 月表拉取月数，每次全量对比更新
	MonthlyRetentionMonths int // 月表保留月数，超期删除
}

// BillingWorker 云账单 ETL：拉取总账与按产品占比，写入 cost_cloud_bill_summary 或 06_ 三表；支持固定五步流水线（D2）。
// AccountID 非空时写入日/月/聚合表带 account_id，供按环境总账（env_breakdown）匹配 cost_env_account_config。[Ref: 01_设计 §按环境展示]
type BillingWorker struct {
	fetcher             cloudbilling.CloudBillingFetcher
	repo                CloudBillRepository
	pipelineRepo        CloudBillPipelineRepository // 非 nil 时 RunPipeline 可用
	billingCycle        string
	AccountID           string                     // 多账号或按环境拉取时填写（如 POC），与 cost_env_account_config.account_id 对应
	ETLData             *ETLDataConfig             // 拉取/保留月数，nil 时使用默认（日36月60）
	ExpectedTotal       float64
	OnReconcileAlert    func(actual, expected float64, diffPct float64)
	OnPipelineFailAlert func(step string, err error) // D1-1：聚合/ETL 失败时告警，与 04_ 监控集成
}

func (w *BillingWorker) dailyRetentionMonths() int {
	if w.ETLData != nil && w.ETLData.DailyRetentionMonths > 0 {
		return w.ETLData.DailyRetentionMonths
	}
	return DefaultDailyRetentionMonths
}
func (w *BillingWorker) dailyPullMonths() int {
	if w.ETLData != nil && w.ETLData.DailyPullMonths > 0 {
		return w.ETLData.DailyPullMonths
	}
	return DefaultDailyRetentionMonths
}
func (w *BillingWorker) monthlyPullMonths() int {
	if w.ETLData != nil && w.ETLData.MonthlyPullMonths > 0 {
		return w.ETLData.MonthlyPullMonths
	}
	return DefaultMonthlyPullMonths
}
func (w *BillingWorker) monthlyRetentionMonths() int {
	if w.ETLData != nil && w.ETLData.MonthlyRetentionMonths > 0 {
		return w.ETLData.MonthlyRetentionMonths
	}
	return DefaultMonthlyPullMonths
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

// upsertLineItemsForDay 拉取指定日期的行级流水并 Upsert 到 line_items 表。
// 返回：consumptionSum(PretaxAmount 代数和)、cashSum(CashAmount 代数和)、productByCat(PretaxAmount 按类)、cashByCat(CashAmount 按类)。
// [Ref: 16_ §四 步骤①②]
func (w *BillingWorker) upsertLineItemsForDay(ctx context.Context, billDate time.Time) (consumptionSum float64, cashSum float64, productByCat map[string]float64, cashByCat map[string]float64, err error) {
	if w.pipelineRepo == nil || w.fetcher == nil {
		return 0, 0, nil, nil, nil
	}
	dateStr := billDate.Format("2006-01-02")
	resp, err := w.fetcher.FetchLineItems(ctx, cloudbilling.FetchLineItemsRequest{BillingDate: dateStr})
	if err != nil {
		slog.Warn("billing ETL: upsertLineItemsForDay fetch failed", "date", dateStr, "error", err)
		return 0, 0, nil, nil, err
	}

	byCat := make(map[string]float64)
	byCash := make(map[string]float64)
	targetMonth := billDate.Format("2006-01") // 天粒度只认目标天：仅当 billing_cycle 归属目标日所在月才参与日表汇总 [Ref: 16_ §5.2.1]
	for _, it := range resp.Items {
		billingCycle := it.BillingCycle
		if billingCycle == "" && len(dateStr) >= 7 {
			billingCycle = dateStr[:7]
		}
		// 归一化账期为 YYYY-MM 便于比较
		if len(billingCycle) > 7 {
			billingCycle = billingCycle[:7]
		}
		isReversal := it.PretaxAmount < 0
		item := postgres.CloudBillLineItem{
			RecordID:          it.RecordID,
			BillDate:          billDate,
			BillingCycle:      billingCycle,
			ProductCode:       it.ProductCode,
			ProductName:       it.ProductName,
			SubOrderID:        it.SubOrderID,
			InstanceID:        it.InstanceID,
			BillingItem:       it.BillingItem,
			SubscriptionType:  it.SubscriptionType,
			CashAmount:        it.CashAmount,
			PretaxAmount:      it.PretaxAmount,
			PretaxGrossAmount: it.PretaxGrossAmount,
			IsReversal:        isReversal,
			AccountID:         w.AccountID,
		}
		if upsertErr := w.pipelineRepo.UpsertCloudBillLineItem(ctx, item); upsertErr != nil {
			slog.Warn("billing ETL: upsertLineItem failed", "record_id", it.RecordID, "error", upsertErr)
		}
		// 仅归属目标日所在月的条目参与日表汇总；上月冲正不参与天粒度，由月粒度重算覆盖 [Ref: 16_ §5.2.1]
		if billingCycle != targetMonth {
			continue
		}
		// consumption: PretaxAmount 代数和（零值不参与）；同时写「领域:ProductCode」供云产品明细 API 解析 [Ref: 01_设计 D9-8、cost_service GetGlobalDrilldown]
		if it.PretaxAmount != 0 {
			consumptionSum += it.PretaxAmount
			if it.Category != "" {
				byCat[it.Category] += it.PretaxAmount
				if it.ProductCode != "" {
					byCat[it.Category+":"+it.ProductCode] += it.PretaxAmount
				}
			}
		}
		// payment: CashAmount 代数和；同上，写领域:ProductCode 供 drilldown
		cashSum += it.CashAmount
		if it.Category != "" {
			byCash[it.Category] += it.CashAmount
			if it.ProductCode != "" {
				byCash[it.Category+":"+it.ProductCode] += it.CashAmount
			}
		}
	}
	// 自动填充 product_category_mapping，使 rebuildDailyRawFromLineItems/rebuildMonthlyRawFromLineItems 可查询分类
	catSeen := make(map[string]bool)
	for _, it := range resp.Items {
		if it.ProductCode != "" && it.Category != "" && !catSeen[it.ProductCode] {
			catSeen[it.ProductCode] = true
			// Category 来自 aliyun productCodeToDomain（中文领域），转为英文存入映射表
			engCat := "compute"
			switch it.Category {
			case "存储":
				engCat = "storage"
			case "网络":
				engCat = "network"
			case "安全":
				engCat = "security"
			}
			_ = w.pipelineRepo.UpsertProductCategory(ctx, it.ProductCode, engCat)
		}
	}
	slog.Info("billing ETL: upsertLineItemsForDay done", "date", dateStr, "items", len(resp.Items),
		"consumption_sum", consumptionSum, "cash_sum", cashSum)
	return consumptionSum, cashSum, byCat, byCash, nil
}

// rebuildDailyRawFromLineItems 用 API 返回的聚合结果写入 daily_raw。
// 当 API 返回的条目缺少产品明细（仅 1 条且 product_code 为空）时，保留已有 daily_raw（可能由 backfill 写入，含完整产品分解）。
// [Ref: 16_ §四 步骤③④]
func (w *BillingWorker) rebuildDailyRawFromLineItems(ctx context.Context, billDate time.Time, consumptionSum, cashSum float64, byCat, cashByCat map[string]float64) error {
	if w.pipelineRepo == nil {
		return nil
	}
	// 质量检查：若新数据无产品级 breakdown（只有领域级键），且已有 daily_raw 有更好的产品明细，则跳过覆盖
	hasProductDetail := false
	for k := range byCat {
		if strings.Contains(k, ":") {
			hasProductDetail = true
			break
		}
	}
	if !hasProductDetail {
		if existing, _ := w.pipelineRepo.GetCloudBillDailyRaw(ctx, billDate, w.AccountID); existing != nil {
			for k := range existing.ProductBreakdown {
				if strings.Contains(k, ":") {
					slog.Info("billing ETL: rebuildDailyRaw skipped, existing has better product detail", "date", billDate.Format("2006-01-02"))
					return nil
				}
			}
		}
	}
	snap := time.Now()
	return w.pipelineRepo.SaveCloudBillDailyRaw(ctx, postgres.CloudBillDailyRaw{
		BillDate:             billDate,
		TotalAmount:          consumptionSum,
		ProductBreakdown:     byCat,
		CashTotalAmount:      cashSum,
		CashProductBreakdown: cashByCat,
		SnapshotAt:           snap,
		CreatedAt:            snap,
		AccountID:            w.AccountID,
	})
}

// RunPipeline 执行 10步 ETL 顺序（06_ D2 + 16_ §八）：
// ①拉取行级流水（T-1+窗口回溯T-7）→ ②Upsert line_items → ③代数和重算 → ④覆盖 daily_raw（增量仅写昨天及窗口）
// → ⑤校验昨日成功 → ⑥缺日检测 → ⑦删 N 个月前当日（N=日表保留月数，缺日时不删）→ ⑧写月原始（全量对比更新 N 月）→ ⑨触发聚合
// → ⑩月度校验（触发日为每月5/10/15日）。需 pipelineRepo 非 nil。
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
	cutRetention := now.AddDate(0, -w.dailyRetentionMonths(), 0).Truncate(24 * time.Hour)
	startAt := time.Now()
	periodDay := yesterday.Format("2006-01-02")
	periodMonth := now.Format("2006-01")
	slog.Info("billing ETL pipeline: start", "period_day", periodDay, "period_month", periodMonth)
	defer func() {
		slog.Info("billing ETL pipeline: end", "duration_ms", time.Since(startAt).Milliseconds(), "period_day", periodDay, "period_month", periodMonth)
	}()

	// ① 拉取行级流水（T-1 昨日）并 ② Upsert line_items → ③ 代数和 → ④ 覆盖 daily_raw
	// [Ref: 16_ §八 步骤①②③④]
	// 降级逻辑：FetchLineItems 失败或返回 0 条时使用 FetchAccountSummary（兼容历史与 mock）
	useSummaryFallback := false
	yesterdayCashSum, yesterdayPaySum, yesterdayByCat, yesterdayByCash, fetchErr := w.upsertLineItemsForDay(ctx, yesterday)
	if fetchErr != nil {
		slog.Warn("billing ETL pipeline: step1-4 yesterday line items failed, falling back to summary API",
			"yesterday", yesterday.Format("2006-01-02"), "error", fetchErr)
		useSummaryFallback = true
	} else if len(yesterdayByCat) == 0 && yesterdayCashSum == 0 {
		// FetchLineItems 返回 0 条（mock 或当日确实无消费），降级到汇总 API 保留历史 daily_raw 写入行为
		useSummaryFallback = true
	}
	_ = yesterdayByCash // 通过 rebuildDailyRawFromLineItems 写入，此处仅防 lint 报 unused

	if useSummaryFallback {
		reqDay := cloudbilling.FetchAccountSummaryRequest{BillingCycle: yesterday.Format("2006-01-02"), PeriodType: "day"}
		if respDay, err2 := w.fetcher.FetchAccountSummary(ctx, reqDay); err2 == nil {
			merged := make(map[string]float64)
			for k, v := range respDay.ByCategory {
				merged[k] = v
			}
			for _, it := range respDay.Items {
				merged[it.Category+":"+it.ProductCode] = it.Amount
			}
			cashMerged := make(map[string]float64)
			for k, v := range respDay.CashByCategory {
				cashMerged[k] = v
			}
			if respDay.TotalAmount < 0 || respDay.CashTotalAmount < 0 {
				slog.Warn("billing ETL pipeline: writing negative daily total (credit/adjustment)",
					"bill_date", yesterday.Format("2006-01-02"), "total_amount", respDay.TotalAmount, "cash_total", respDay.CashTotalAmount)
			}
			snap := time.Now()
			_ = w.pipelineRepo.SaveCloudBillDailyRaw(ctx, postgres.CloudBillDailyRaw{
				BillDate:             yesterday,
				TotalAmount:          respDay.TotalAmount,
				ProductBreakdown:     merged,
				CashTotalAmount:      respDay.CashTotalAmount,
				CashProductBreakdown: cashMerged,
				SnapshotAt:           snap, CreatedAt: snap, AccountID: w.AccountID,
			})
			if respDay.TotalAmount > 0 && len(respDay.Items) == 0 {
				slog.Warn("billing ETL pipeline: daily items count is 0 but total > 0 (fallback path)", "date", yesterday.Format("2006-01-02"))
				if w.OnPipelineFailAlert != nil {
					w.OnPipelineFailAlert("daily_items_check", fmt.Errorf("daily items count 0 with total_amount %.2f", respDay.TotalAmount))
				}
			}
		}
	} else {
		if err2 := w.rebuildDailyRawFromLineItems(ctx, yesterday, yesterdayCashSum, yesterdayPaySum, yesterdayByCat, yesterdayByCash); err2 != nil {
			slog.Warn("billing ETL pipeline: step4 rebuild daily_raw failed", "date", yesterday.Format("2006-01-02"), "error", err2)
		}
		// [Ref: 01_实践 D4-3 必选] 校验当日 Items 条数（consumption_sum = PretaxAmount 代数和）
		if yesterdayCashSum > 0 && len(yesterdayByCat) == 0 {
			slog.Warn("billing ETL pipeline: step4 consumption_sum > 0 but no category data", "date", yesterday.Format("2006-01-02"))
			if w.OnPipelineFailAlert != nil {
				w.OnPipelineFailAlert("daily_items_check", fmt.Errorf("consumption_sum %.2f but no category breakdown for %s", yesterdayCashSum, yesterday.Format("2006-01-02")))
			}
		}
	}

	// 窗口回溯：T-2 至 T-7，确保历史冲正条目被捕获 [Ref: 16_ §五]
	for i := 2; i <= WindowResyncDays; i++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		targetDay := now.AddDate(0, 0, -i).Truncate(24 * time.Hour)
		conSum, paySum, byCat, byCash, err2 := w.upsertLineItemsForDay(ctx, targetDay)
		if err2 != nil || (conSum == 0 && len(byCat) == 0) {
			// 降级：使用 FetchAccountSummary 更新该天 daily_raw（含冲正场景下重算）；须写入 Cash 字段供聚合实付 [Ref: 16_ §3.3]
			reqDay := cloudbilling.FetchAccountSummaryRequest{BillingCycle: targetDay.Format("2006-01-02"), PeriodType: "day"}
			if respDay, errS := w.fetcher.FetchAccountSummary(ctx, reqDay); errS == nil {
				merged := make(map[string]float64)
				for k, v := range respDay.ByCategory {
					merged[k] = v
				}
				for _, it := range respDay.Items {
					merged[it.Category+":"+it.ProductCode] = it.Amount
				}
				cashMerged := make(map[string]float64)
				for k, v := range respDay.CashByCategory {
					cashMerged[k] = v
				}
				snap := time.Now()
				_ = w.pipelineRepo.SaveCloudBillDailyRaw(ctx, postgres.CloudBillDailyRaw{
					BillDate:             targetDay,
					TotalAmount:          respDay.TotalAmount,
					ProductBreakdown:     merged,
					CashTotalAmount:      respDay.CashTotalAmount,
					CashProductBreakdown: cashMerged,
					SnapshotAt:           snap, CreatedAt: snap, AccountID: w.AccountID,
				})
			}
			if err2 != nil {
				slog.Warn("billing ETL pipeline: window resync failed", "date", targetDay.Format("2006-01-02"), "error", err2)
			}
			continue
		}
		if err2 := w.rebuildDailyRawFromLineItems(ctx, targetDay, conSum, paySum, byCat, byCash); err2 != nil {
			slog.Warn("billing ETL pipeline: window resync rebuild daily_raw failed", "date", targetDay.Format("2006-01-02"), "error", err2)
		}
	}
	slog.Info("billing ETL pipeline: window resync done", "days_back", WindowResyncDays)

	// ⑤ 校验昨日写入成功（CashAmount 可为 0 或负值，仅校验记录存在）
	got, _ := w.pipelineRepo.GetCloudBillDailyRaw(ctx, yesterday, w.AccountID)
	ok := got != nil
	if !ok {
		slog.Warn("billing ETL pipeline: step5 yesterday not valid, skip delete")
	}

	// ⑥ 缺日检测；⑦ 无缺日时删除 10 个月前当日（line_items 同步清理）
	if ok {
		missing, err := w.pipelineRepo.ListMissingCloudBillDailyDates(ctx, cutRetention, now, w.AccountID)
		if err != nil {
			slog.Warn("billing ETL pipeline: step6 list missing failed", "error", err)
		} else if len(missing) > 0 {
			slog.Warn("billing ETL pipeline: step6 missing dates, skip delete", "count", len(missing), "sample", missing[0].Format("2006-01-02"))
		} else {
			if err := w.pipelineRepo.DeleteCloudBillDailyRawForDate(ctx, cutRetention, w.AccountID); err != nil {
				slog.Warn("billing ETL pipeline: step7 delete daily_raw failed", "date", cutRetention.Format("2006-01-02"), "error", err)
			} else {
				_ = w.pipelineRepo.DeleteLineItemsOlderThan(ctx, cutRetention, w.AccountID)
				slog.Info("billing ETL pipeline: step7 deleted old data", "date", cutRetention.Format("2006-01-02"), "retention_months", w.dailyRetentionMonths())
			}
		}
	}

	// ⑧ 写月原始：按配置全量对比更新——拉取最近 N 个月（monthly_pull_months）逐月调用 API 并落库；日表增量仅写昨天+窗口 [Ref: 01_实践 月源数据近5年；配置控制拉取长度]
	nMonth := w.monthlyPullMonths()
	cyclesToWrite := make([]string, 0, nMonth)
	for i := 0; i < nMonth; i++ {
		t := now.AddDate(0, -nMonth+1+i, 0)
		cyclesToWrite = append(cyclesToWrite, t.Format("2006-01"))
	}
	cycle := now.Format("2006-01")
	prevMonthCycle := now.AddDate(0, -1, 0).Format("2006-01")
	for _, writeCycle := range cyclesToWrite {
		reqM := cloudbilling.FetchAccountSummaryRequest{BillingCycle: writeCycle, PeriodType: "month"}
		respM, errM := w.fetcher.FetchAccountSummary(ctx, reqM)
		if errM != nil {
			slog.Warn("billing ETL pipeline: step4 fetch month failed", "billing_cycle", writeCycle, "error", errM)
			continue
		}
		merged := make(map[string]float64)
		for k, v := range respM.ByCategory {
			merged[k] = v
		}
		for _, it := range respM.Items {
			merged[it.Category+":"+it.ProductCode] = it.Amount
		}
		cashMerged := make(map[string]float64)
		for k, v := range respM.CashByCategory {
			cashMerged[k] = v
		}
		// [Ref: 用户需求] 整月现金支付数据不可能是负数；API 若返回负则按 0 落库
		cashTotalWrite := respM.CashTotalAmount
		if cashTotalWrite < 0 {
			cashTotalWrite = 0
			cashMerged = make(map[string]float64)
		}
		snap := time.Now()
		if errS := w.pipelineRepo.SaveCloudBillMonthlyRaw(ctx, postgres.CloudBillMonthlyRaw{
			BillingCycle:         respM.BillingCycle,
			TotalAmount:          respM.TotalAmount,
			ProductBreakdown:     merged,
			CashTotalAmount:      cashTotalWrite,
			CashProductBreakdown: cashMerged,
			SnapshotAt:           snap,
			CreatedAt:            snap,
			AccountID:            w.AccountID,
		}); errS != nil {
			slog.Warn("billing ETL pipeline: step4 save monthly raw failed", "billing_cycle", writeCycle, "error", errS)
		} else {
			slog.Info("billing ETL pipeline: step4 saved monthly raw", "billing_cycle", respM.BillingCycle, "total", respM.TotalAmount, "cash_total", respM.CashTotalAmount)
		}
	}
	// 月表超期删除：保留月数由配置控制 [Ref: 01_实践 配置控制保存长度]
	if cutoff := now.AddDate(0, -w.monthlyRetentionMonths(), 0).Format("2006-01"); cutoff != "" {
		if errDel := w.pipelineRepo.DeleteCloudBillMonthlyRawOlderThan(ctx, cutoff, w.AccountID); errDel != nil {
			slog.Warn("billing ETL pipeline: delete monthly raw older than failed", "cutoff", cutoff, "error", errDel)
		} else {
			slog.Info("billing ETL pipeline: step4 monthly retention applied", "cutoff", cutoff, "retention_months", w.monthlyRetentionMonths())
		}
	}
	// [Ref: 16_ §七 步骤⑧] 由流水表按 billing_cycle 重算上月、当月月原始表，使回退/调账归属到被冲正账期
	for _, writeCycle := range []string{prevMonthCycle, cycle} {
		if err := w.rebuildMonthlyRawFromLineItems(ctx, writeCycle); err != nil {
			slog.Warn("billing ETL pipeline: rebuild monthly from line_items failed", "billing_cycle", writeCycle, "error", err)
		}
	}
	// [Ref: 16_ §七 结合方案] 按窗口内出现的账期重算月表：发现冲正即更新对应月，不限于上月/当月（如上月冲正在今日入账，会更新上月月表）
	windowStart := now.AddDate(0, 0, -WindowResyncDays).Truncate(24 * time.Hour)
	cyclesInWindow, errList := w.pipelineRepo.ListDistinctBillingCyclesInDateRange(ctx, windowStart, yesterday, w.AccountID)
	if errList == nil && len(cyclesInWindow) > 0 {
		cutoffMonth := now.AddDate(0, -w.dailyRetentionMonths(), 0).Format("2006-01")
		for _, c := range cyclesInWindow {
			if c < cutoffMonth {
				continue
			}
			if c == prevMonthCycle || c == cycle {
				continue // 已在上方重算
			}
			if err := w.rebuildMonthlyRawFromLineItems(ctx, c); err != nil {
				slog.Warn("billing ETL pipeline: rebuild monthly from window cycle failed", "billing_cycle", c, "error", err)
			} else {
				slog.Info("billing ETL pipeline: rebuilt monthly from window", "billing_cycle", c)
			}
		}
	}

	// ⑤ 触发聚合（委托给 runAggregateStep，RunPipelineAggregateOnly 复用同一逻辑）
	pipelineErr := w.runAggregateStep(ctx, now, yesterday, periodDay)

	// ⑩ 月度校验（触发日为每月 5/10/15 日）[Ref: 16_ §五 步骤⑩]
	dayOfMonth := now.Day()
	if dayOfMonth == 5 || dayOfMonth == 10 || dayOfMonth == 15 {
		prevCycle := now.AddDate(0, -1, 0).Format("2006-01")
		slog.Info("billing ETL pipeline: step10 monthly reconcile triggered", "billing_cycle", prevCycle, "day_of_month", dayOfMonth)
		if err := w.runMonthlyReconcile(ctx, prevCycle); err != nil {
			slog.Warn("billing ETL pipeline: step10 monthly reconcile failed", "billing_cycle", prevCycle, "error", err)
		}
	}

	// [Ref: 01_实践 D7-3 必选] 审计日志：每次 ETL 流水线结束记录结果便于回放
	result := "ok"
	if pipelineErr != nil {
		result = "fail"
	}
	slog.Info("billing ETL audit: pipeline", "type", "pipeline", "result", result, "error", pipelineErr, "yesterday", periodDay)

	return pipelineErr
}

// rebuildMonthlyRawFromLineItems 由流水表按 billing_cycle 汇总消耗口径（TotalAmount/ProductBreakdown）写入月原始表，保留 API 月汇总的现金字段不被覆盖。[Ref: 16_ §四、§七 步骤⑧；用户需求 总账单用API月汇总数据]
// 总账单展示用 API 月汇总（步骤④写入的 CashTotalAmount/CashProductBreakdown），rebuild 仅更新消耗口径供明细与对账用。
func (w *BillingWorker) rebuildMonthlyRawFromLineItems(ctx context.Context, billingCycle string) error {
	if w.pipelineRepo == nil {
		return nil
	}
	items, err := w.pipelineRepo.ListCloudBillLineItemsByBillingCycle(ctx, billingCycle, w.AccountID)
	if err != nil {
		return err
	}
	if len(items) == 0 {
		return nil
	}
	var pretaxSum float64
	byCat := make(map[string]float64) // PretaxAmount 按分类；键为中文领域，含「领域:ProductCode」供 drilldown [Ref: 01_设计 D9-8]
	for _, it := range items {
		pretaxSum += it.PretaxAmount
		cat := "other"
		if c, ok := w.pipelineRepo.GetProductCategory(ctx, it.ProductCode); ok && c != "" {
			cat = c
		}
		domain := categoryToDomain(cat)
		byCat[domain] += it.PretaxAmount
		if it.ProductCode != "" {
			byCat[domain+":"+it.ProductCode] += it.PretaxAmount
		}
	}
	// 保留已有行的 API 月汇总现金，不覆盖；无已有行时现金写 0
	existing, _ := w.pipelineRepo.GetCloudBillMonthlyRaw(ctx, billingCycle, w.AccountID)
	cashTotal := 0.0
	cashByCat := make(map[string]float64)
	if existing != nil {
		cashTotal = existing.CashTotalAmount
		if existing.CashProductBreakdown != nil {
			for k, v := range existing.CashProductBreakdown {
				cashByCat[k] = v
			}
		}
	}
	snap := time.Now()
	return w.pipelineRepo.SaveCloudBillMonthlyRaw(ctx, postgres.CloudBillMonthlyRaw{
		BillingCycle:         billingCycle,
		TotalAmount:          pretaxSum,
		ProductBreakdown:     byCat,
		CashTotalAmount:      cashTotal,
		CashProductBreakdown: cashByCat,
		SnapshotAt:           snap,
		CreatedAt:            snap,
		AccountID:            w.AccountID,
	})
}

// runAggregateStep 执行步骤⑨：从 daily_raw/monthly_raw 重算全部时间范围聚合数据并写入 aggregate 表。
// 独立方法，RunPipeline 和 RunPipelineAggregateOnly 共同调用。
func (w *BillingWorker) runAggregateStep(ctx context.Context, now, yesterday time.Time, periodDay string) error {
	successAt := time.Now()
	_ = periodDay
	yesterdayT := yesterday
	month := now.Format("2006-01")
	q := (int(now.Month())-1)/3 + 1
	quarterKey := fmt.Sprintf("%s-Q%d", now.Format("2006"), q)

	var aggErr error
	recordErr := func(err error) {
		if err != nil && aggErr == nil {
			aggErr = err
			slog.Warn("billing ETL aggregate: write failed", "error", err)
		}
	}

	// saveAggWithPrev 写入 metric_type='consumption' 聚合行，保留当前/上一周期；cash 参数保留兼容调用，仅用 con。[Ref: 用户确认 消耗口径]
	saveAggWithPrev := func(
		reportType, curKey string,
		curConTotal float64, curConByCat map[string]float64,
		curCashTotal float64, curCashByCat map[string]float64,
		prevKey string,
		prevConTotal float64, prevConByCat map[string]float64,
		prevCashTotal float64, prevCashByCat map[string]float64,
	) error {
		const metricType = "consumption"
		_ = curCashTotal
		_ = curCashByCat
		_ = prevCashTotal
		_ = prevCashByCat
		curTotal := curConTotal
		curByCat := curConByCat
		prevTotal := prevConTotal
		prevByCat := prevConByCat
		if err := w.pipelineRepo.SaveCloudBillAggregate(ctx, postgres.CloudBillAggregate{
			ReportType: reportType, PeriodKey: curKey, MetricType: metricType,
			TotalAmount: curTotal, ProductBreakdown: curByCat,
			LastSuccessAt: &successAt, CreatedAt: successAt, UpdatedAt: successAt, AccountID: w.AccountID,
		}); err != nil {
			return err
		}
		if prevKey != curKey {
			if err := w.pipelineRepo.SaveCloudBillAggregate(ctx, postgres.CloudBillAggregate{
				ReportType: reportType, PeriodKey: prevKey, MetricType: metricType,
				TotalAmount: prevTotal, ProductBreakdown: prevByCat,
				LastSuccessAt: &successAt, CreatedAt: successAt, UpdatedAt: successAt, AccountID: w.AccountID,
			}); err != nil {
				return err
			}
		}
		keep := []string{curKey}
		if prevKey != curKey {
			keep = append(keep, prevKey)
		}
		return w.pipelineRepo.DeleteCloudBillAggregateExcept(ctx, reportType, keep, w.AccountID)
	}

	// mergeDailyRowsWithCallbackReplacement 汇总日原始行（消耗口径）；total_amount < 0 的日视为回调日，用正常日日均替代。[Ref: 用户确认 回调日替代]
	mergeDailyRowsWithCallbackReplacement := func(rows []postgres.CloudBillDailyRaw) (total float64, byCat map[string]float64) {
		byCat = make(map[string]float64)
		if len(rows) == 0 {
			return 0, byCat
		}
		var normalRows []postgres.CloudBillDailyRaw
		for _, r := range rows {
			if r.TotalAmount >= 0 {
				normalRows = append(normalRows, r)
			}
		}
		if len(normalRows) == 0 {
			return 0, byCat
		}
		var sumTotal float64
		avgPB := make(map[string]float64)
		for _, r := range normalRows {
			sumTotal += r.TotalAmount
			for k, v := range r.ProductBreakdown {
				avgPB[k] += v
			}
		}
		nNormal := float64(len(normalRows))
		avgTotal := sumTotal / nNormal
		for k := range avgPB {
			avgPB[k] /= nNormal
		}
		var sumPB float64
		for _, v := range avgPB {
			sumPB += v
		}
		if sumPB != 0 && math.Abs(avgTotal) > 1e-9 {
			scale := avgTotal / sumPB
			for k := range avgPB {
				avgPB[k] *= scale
			}
		}
		nCallback := len(rows) - len(normalRows)
		total = sumTotal + avgTotal*float64(nCallback)
		for _, r := range normalRows {
			for k, v := range r.ProductBreakdown {
				byCat[k] += v
			}
		}
		for k, v := range avgPB {
			byCat[k] += v * float64(nCallback)
		}
		return total, byCat
	}

	// mergeDailyRows 汇总日原始行（仅用于 last_quarter 等不需要回调替代的路径，保留双指标）
	mergeDailyRows := func(rows []postgres.CloudBillDailyRaw) (total float64, byCat map[string]float64, cashTotal float64, cashByCat map[string]float64) {
		byCat = make(map[string]float64)
		cashByCat = make(map[string]float64)
		for _, r := range rows {
			total += r.TotalAmount
			for k, v := range r.ProductBreakdown {
				byCat[k] += v
			}
			cashTotal += r.CashTotalAmount
			for k, v := range r.CashProductBreakdown {
				cashByCat[k] += v
			}
		}
		return
	}

	// sumMonthlyRaw 从月原始表读取指定账期的双指标（consumption + payment）
	sumMonthlyRaw := func(cycle string) (conTotal float64, conByCat map[string]float64, payTotal float64, payByCat map[string]float64) {
		conByCat = make(map[string]float64)
		payByCat = make(map[string]float64)
		if mon, _ := w.pipelineRepo.GetCloudBillMonthlyRaw(ctx, cycle, w.AccountID); mon != nil {
			conTotal = mon.TotalAmount
			for k, v := range mon.ProductBreakdown {
				conByCat[k] = v
			}
			payTotal = mon.CashTotalAmount
			for k, v := range mon.CashProductBreakdown {
				payByCat[k] = v
			}
		}
		return
	}

	// month：当月 1 日至昨日，日原始表叠加；回调日（total_amount<0）用正常日日均替代 [Ref: 用户确认 本月+回调日替代]
	firstOfMonth := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	rowsMonth, _ := w.pipelineRepo.ListCloudBillDailyRawFromTo(ctx, firstOfMonth, yesterdayT, w.AccountID)
	totalMonth, byCatMonth := mergeDailyRowsWithCallbackReplacement(rowsMonth)
	prevMonth := now.AddDate(0, -1, 0).Format("2006-01")
	pmCon, pmConByCat, _, _ := sumMonthlyRaw(prevMonth)
	recordErr(saveAggWithPrev("month", month,
		totalMonth, byCatMonth, 0, nil,
		prevMonth,
		pmCon, pmConByCat, 0, nil))

	// last_quarter 数据先算，供 quarter 的上一周期及 last_quarter 使用 [Ref: 01_设计 report_type 与 period_key]
	currQ := (int(now.Month())-1)/3 + 1
	prevQ := currQ - 1
	prevY := now.Year()
	if prevQ <= 0 {
		prevQ = 4
		prevY = now.Year() - 1
	}
	prevQuarterKey := fmt.Sprintf("%d-Q%d", prevY, prevQ)
	prevQStartMonth := (prevQ-1)*3 + 1
	var totalPrevQ, cashPrevQ float64
	byCatPrevQ := make(map[string]float64)
	cashByCatPrevQ := make(map[string]float64)
	for _, c := range []string{
		fmt.Sprintf("%04d-%02d", prevY, prevQStartMonth),
		fmt.Sprintf("%04d-%02d", prevY, prevQStartMonth+1),
		fmt.Sprintf("%04d-%02d", prevY, prevQStartMonth+2),
	} {
		ct, cb, pt, pb := sumMonthlyRaw(c)
		totalPrevQ += ct
		cashPrevQ += pt
		for k, v := range cb {
			byCatPrevQ[k] += v
		}
		for k, v := range pb {
			cashByCatPrevQ[k] += v
		}
	}

	// quarter：完整月用月原始表（消耗），当月用日原始表+回调日替代 [Ref: 用户确认 这季度+消耗口径]
	var totalQ float64
	byCatQ := make(map[string]float64)
	qStartMonth := (q-1)*3 + 1
	for m := 0; m < 3; m++ {
		cycle := fmt.Sprintf("%04d-%02d", now.Year(), qStartMonth+m)
		if cycle == month {
			firstOfCurMonth := time.Date(now.Year(), time.Month(qStartMonth+m), 1, 0, 0, 0, 0, time.UTC)
			rowsCur, _ := w.pipelineRepo.ListCloudBillDailyRawFromTo(ctx, firstOfCurMonth, yesterdayT, w.AccountID)
			t, pb := mergeDailyRowsWithCallbackReplacement(rowsCur)
			totalQ += t
			for k, v := range pb {
				byCatQ[k] += v
			}
		} else {
			ct, cb, _, _ := sumMonthlyRaw(cycle)
			totalQ += ct
			for k, v := range cb {
				byCatQ[k] += v
			}
		}
	}
	recordErr(saveAggWithPrev("quarter", quarterKey,
		totalQ, byCatQ, 0, nil,
		prevQuarterKey,
		totalPrevQ, byCatPrevQ, 0, nil))

	// last_month：上月（月原始表）
	prevPrevMonth := now.AddDate(0, -2, 0).Format("2006-01")
	ppmCon, ppmConByCat, ppmPay, ppmPayByCat := sumMonthlyRaw(prevPrevMonth)
	if monCur, _ := w.pipelineRepo.GetCloudBillMonthlyRaw(ctx, prevMonth, w.AccountID); monCur != nil && (monCur.TotalAmount != 0 || len(monCur.ProductBreakdown) > 0) {
		recordErr(saveAggWithPrev("last_month", prevMonth,
			monCur.TotalAmount, monCur.ProductBreakdown, monCur.CashTotalAmount, monCur.CashProductBreakdown,
			prevPrevMonth,
			ppmCon, ppmConByCat, ppmPay, ppmPayByCat))
	}

	// last_quarter：上季度三月汇总
	prevPrevQ := prevQ - 1
	prevPrevY := prevY
	if prevPrevQ <= 0 {
		prevPrevQ = 4
		prevPrevY = prevY - 1
	}
	prevPrevQuarterKey := fmt.Sprintf("%d-Q%d", prevPrevY, prevPrevQ)
	ppQStartMonth := (prevPrevQ-1)*3 + 1
	var totalPrevPrevQ, cashPrevPrevQ float64
	byCatPrevPrevQ := make(map[string]float64)
	cashByCatPrevPrevQ := make(map[string]float64)
	for _, c := range []string{
		fmt.Sprintf("%04d-%02d", prevPrevY, ppQStartMonth),
		fmt.Sprintf("%04d-%02d", prevPrevY, ppQStartMonth+1),
		fmt.Sprintf("%04d-%02d", prevPrevY, ppQStartMonth+2),
	} {
		ct, cb, pt, pb := sumMonthlyRaw(c)
		totalPrevPrevQ += ct
		cashPrevPrevQ += pt
		for k, v := range cb {
			byCatPrevPrevQ[k] += v
		}
		for k, v := range pb {
			cashByCatPrevPrevQ[k] += v
		}
	}
	if totalPrevQ != 0 || len(byCatPrevQ) > 0 {
		recordErr(saveAggWithPrev("last_quarter", prevQuarterKey,
			totalPrevQ, byCatPrevQ, cashPrevQ, cashByCatPrevQ,
			prevPrevQuarterKey,
			totalPrevPrevQ, byCatPrevPrevQ, cashPrevPrevQ, cashByCatPrevPrevQ))
	}

	// this_year：历史完整月（月原始表）+ 当月日累加 [Ref: 01_实践 §this_year 算法]
	thisYearNum := now.Year()
	currentMonthNum := int(now.Month())
	var totalThisYear, cashThisYear float64
	byCatThisYear := make(map[string]float64)
	cashByCatThisYear := make(map[string]float64)
	for m := 1; m < currentMonthNum; m++ {
		cycle := fmt.Sprintf("%04d-%02d", thisYearNum, m)
		ct, cb, pt, pb := sumMonthlyRaw(cycle)
		totalThisYear += ct
		cashThisYear += pt
		for k, v := range cb {
			byCatThisYear[k] += v
		}
		for k, v := range pb {
			cashByCatThisYear[k] += v
		}
	}
	firstOfCurrentMonth := time.Date(thisYearNum, time.Month(currentMonthNum), 1, 0, 0, 0, 0, time.UTC)
	rowsCurrentMonth, _ := w.pipelineRepo.ListCloudBillDailyRawFromTo(ctx, firstOfCurrentMonth, yesterdayT, w.AccountID)
	curMonthTotal, curMonthByCat, curMonthCash, curMonthCashByCat := mergeDailyRows(rowsCurrentMonth)
	totalThisYear += curMonthTotal
	cashThisYear += curMonthCash
	for k, v := range curMonthByCat {
		byCatThisYear[k] += v
	}
	for k, v := range curMonthCashByCat {
		cashByCatThisYear[k] += v
	}
	thisYearKey := fmt.Sprintf("%d", thisYearNum)

	// last_year：去年全年12个月月账单汇总
	lastYearNum := thisYearNum - 1
	var totalLastYear, cashLastYear float64
	byCatLastYear := make(map[string]float64)
	cashByCatLastYear := make(map[string]float64)
	for m := 1; m <= 12; m++ {
		cycle := fmt.Sprintf("%04d-%02d", lastYearNum, m)
		ct, cb, pt, pb := sumMonthlyRaw(cycle)
		totalLastYear += ct
		cashLastYear += pt
		for k, v := range cb {
			byCatLastYear[k] += v
		}
		for k, v := range pb {
			cashByCatLastYear[k] += v
		}
	}
	if totalThisYear != 0 || len(byCatThisYear) > 0 || cashThisYear != 0 {
		lastYearKey := fmt.Sprintf("%d", lastYearNum)
		recordErr(saveAggWithPrev("this_year", thisYearKey,
			totalThisYear, byCatThisYear, cashThisYear, cashByCatThisYear,
			lastYearKey,
			totalLastYear, byCatLastYear, cashLastYear, cashByCatLastYear))
	}
	if totalLastYear != 0 || len(byCatLastYear) > 0 || cashLastYear != 0 {
		lastYearKey := fmt.Sprintf("%d", lastYearNum)
		prevYearKey := fmt.Sprintf("%d", lastYearNum-1)
		var totalPrevYear, cashPrevYear float64
		byCatPrevYear := make(map[string]float64)
		cashByCatPrevYear := make(map[string]float64)
		for m := 1; m <= 12; m++ {
			cycle := fmt.Sprintf("%04d-%02d", lastYearNum-1, m)
			ct, cb, pt, pb := sumMonthlyRaw(cycle)
			totalPrevYear += ct
			cashPrevYear += pt
			for k, v := range cb {
				byCatPrevYear[k] += v
			}
			for k, v := range pb {
				cashByCatPrevYear[k] += v
			}
		}
		recordErr(saveAggWithPrev("last_year", lastYearKey,
			totalLastYear, byCatLastYear, cashLastYear, cashByCatLastYear,
			prevYearKey,
			totalPrevYear, byCatPrevYear, cashPrevYear, cashByCatPrevYear))
	}

	slog.Info("billing ETL: runAggregateStep done", "error", aggErr)
	return aggErr
}

// runMonthlyReconcile 月度对账：对比 line_items CashAmount 代数和 vs QueryBillOverview 月总额。
// 差额 > ReconcileAbsThreshold 时标记 DIRTY 并触发异步 ReconcileWorker。[Ref: 16_ §五]
func (w *BillingWorker) runMonthlyReconcile(ctx context.Context, billingCycle string) error {
	if w.pipelineRepo == nil || w.fetcher == nil {
		return nil
	}
	// 获取月 API 总额
	reqMon := cloudbilling.FetchAccountSummaryRequest{BillingCycle: billingCycle, PeriodType: "month"}
	respMon, err := w.fetcher.FetchAccountSummary(ctx, reqMon)
	if err != nil {
		slog.Warn("billing monthly reconcile: fetch month API failed", "billing_cycle", billingCycle, "error", err)
		return err
	}
	monthlyAPITotal := respMon.TotalAmount

	// 获取本地 line_items 代数和
	lineItemsSum, err := w.pipelineRepo.SumLineItemsCashByBillingCycle(ctx, billingCycle, w.AccountID)
	if err != nil {
		slog.Warn("billing monthly reconcile: sum line_items failed", "billing_cycle", billingCycle, "error", err)
		return err
	}

	drift := math.Abs(monthlyAPITotal - lineItemsSum)
	now := time.Now()
	status := postgres.CloudBillMonthStatus{
		BillingCycle:    billingCycle,
		AccountID:       w.AccountID,
		LineItemsSum:    lineItemsSum,
		MonthlyAPITotal: monthlyAPITotal,
		DriftAmount:     monthlyAPITotal - lineItemsSum,
		LastReconciledAt: &now,
	}

	if drift > ReconcileAbsThreshold {
		slog.Warn("billing monthly reconcile: drift exceeds threshold, marking DIRTY",
			"billing_cycle", billingCycle, "line_items_sum", lineItemsSum,
			"monthly_api_total", monthlyAPITotal, "drift", drift)
		status.DataStatus = "DIRTY"
		if err := w.pipelineRepo.UpsertCloudBillMonthStatus(ctx, status); err != nil {
			slog.Warn("billing monthly reconcile: upsert month_status failed", "error", err)
		}
		if w.OnReconcileAlert != nil {
			w.OnReconcileAlert(lineItemsSum, monthlyAPITotal, drift)
		}
		// 触发修复 Worker（异步，此处同步执行；生产可改为投入队列）
		go func() {
			repairCtx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
			defer cancel()
			if err := w.RunReconcileWorker(repairCtx, billingCycle); err != nil {
				slog.Warn("billing reconcile worker: failed", "billing_cycle", billingCycle, "error", err)
			}
		}()
	} else {
		slog.Info("billing monthly reconcile: within threshold, marking FINALIZED",
			"billing_cycle", billingCycle, "drift", drift)
		finalizedAt := now
		status.DataStatus = "FINALIZED"
		status.FinalizedAt = &finalizedAt
		if err := w.pipelineRepo.UpsertCloudBillMonthStatus(ctx, status); err != nil {
			slog.Warn("billing monthly reconcile: upsert month_status failed", "error", err)
		}
		// 同步更新聚合表 data_status
		w.setAggregateDataStatus(ctx, billingCycle, "FINALIZED")
	}
	return nil
}

// setAggregateDataStatus 更新指定账期相关 aggregate 行的 data_status。
func (w *BillingWorker) setAggregateDataStatus(ctx context.Context, billingCycle, status string) {
	if w.pipelineRepo == nil {
		return
	}
	// last_month 对应 billingCycle；更新相关 period_key 的聚合行
	now := time.Now()
	agg := postgres.CloudBillAggregate{
		ReportType: "last_month", PeriodKey: billingCycle,
		DataStatus: status, UpdatedAt: now, AccountID: w.AccountID,
	}
	// 通过 SaveCloudBillAggregate ON CONFLICT 更新 data_status（需聚合行已存在）
	if existing, _ := w.pipelineRepo.GetCloudBillAggregate(ctx, "last_month", billingCycle); existing != nil {
		existing.DataStatus = status
		existing.UpdatedAt = now
		_ = w.pipelineRepo.SaveCloudBillAggregate(ctx, *existing)
	}
	_ = agg // suppress unused
}

// RunReconcileWorker 修复 Worker：全量重拉指定账期每日流水 → 重算 daily_raw → 重算 aggregate。
// [Ref: 16_云账单动态对账与高可靠处理规范 §七]
func (w *BillingWorker) RunReconcileWorker(ctx context.Context, billingCycle string) error {
	if w.pipelineRepo == nil || w.fetcher == nil {
		return nil
	}
	slog.Info("billing reconcile worker: start", "billing_cycle", billingCycle)
	startAt := time.Now()

	// 标记 RECONCILING
	reconcilingStatus := postgres.CloudBillMonthStatus{
		BillingCycle: billingCycle, AccountID: w.AccountID, DataStatus: "RECONCILING",
	}
	_ = w.pipelineRepo.UpsertCloudBillMonthStatus(ctx, reconcilingStatus)

	// 确定账期的起止日期
	cycleTime, err := time.Parse("2006-01", billingCycle)
	if err != nil {
		slog.Warn("billing reconcile worker: invalid billing_cycle", "billing_cycle", billingCycle, "error", err)
		return err
	}
	firstDay := time.Date(cycleTime.Year(), cycleTime.Month(), 1, 0, 0, 0, 0, time.UTC)
	// 计算该月最后一天
	lastDay := firstDay.AddDate(0, 1, -1)
	// 不超过昨日
	yesterday := time.Now().UTC().AddDate(0, 0, -1).Truncate(24 * time.Hour)
	if lastDay.After(yesterday) {
		lastDay = yesterday
	}

	// 全量重拉每日流水
	for d := firstDay; !d.After(lastDay); d = d.AddDate(0, 0, 1) {
		if err := ctx.Err(); err != nil {
			return err
		}
		conSum, paySum, byCat, byCash, err2 := w.upsertLineItemsForDay(ctx, d)
		if err2 != nil {
			slog.Warn("billing reconcile worker: fetch day failed", "date", d.Format("2006-01-02"), "error", err2)
			time.Sleep(200 * time.Millisecond)
			continue
		}
		if err2 := w.rebuildDailyRawFromLineItems(ctx, d, conSum, paySum, byCat, byCash); err2 != nil {
			slog.Warn("billing reconcile worker: rebuild daily_raw failed", "date", d.Format("2006-01-02"), "error", err2)
		}
		time.Sleep(100 * time.Millisecond) // 限流保护
	}

	// 重算 aggregate
	_ = w.RunPipelineAggregateOnly(ctx)

	// 重新对账
	lineItemsSum, _ := w.pipelineRepo.SumLineItemsCashByBillingCycle(ctx, billingCycle, w.AccountID)
	reqMon := cloudbilling.FetchAccountSummaryRequest{BillingCycle: billingCycle, PeriodType: "month"}
	respMon, err := w.fetcher.FetchAccountSummary(ctx, reqMon)
	if err != nil {
		slog.Warn("billing reconcile worker: re-fetch month API failed", "billing_cycle", billingCycle, "error", err)
		return err
	}
	drift := math.Abs(respMon.TotalAmount - lineItemsSum)
	now2 := time.Now()
	finalStatus := postgres.CloudBillMonthStatus{
		BillingCycle:    billingCycle,
		AccountID:       w.AccountID,
		LineItemsSum:    lineItemsSum,
		MonthlyAPITotal: respMon.TotalAmount,
		DriftAmount:     respMon.TotalAmount - lineItemsSum,
		LastReconciledAt: &now2,
		LastFullSyncAt:   &now2,
	}
	if drift <= ReconcileAbsThreshold {
		finalStatus.DataStatus = "FINALIZED"
		finalStatus.FinalizedAt = &now2
		slog.Info("billing reconcile worker: FINALIZED", "billing_cycle", billingCycle, "drift", drift,
			"duration_ms", time.Since(startAt).Milliseconds())
		w.setAggregateDataStatus(ctx, billingCycle, "FINALIZED")
	} else {
		finalStatus.DataStatus = "DIRTY"
		finalStatus.Notes = fmt.Sprintf("修复后仍存在差异 %.4f 元，需人工裁决", drift)
		slog.Warn("billing reconcile worker: still DIRTY after repair", "billing_cycle", billingCycle, "drift", drift)
	}
	_ = w.pipelineRepo.UpsertCloudBillMonthStatus(ctx, finalStatus)
	slog.Info("billing reconcile worker: done", "billing_cycle", billingCycle, "status", finalStatus.DataStatus,
		"duration_ms", time.Since(startAt).Milliseconds())
	return nil
}

// RunPipelineAggregateOnly 仅执行聚合步骤（步骤⑨），复用 RunPipeline 的聚合逻辑。
// 用于 ReconcileWorker 在全量重拉后重算 aggregate 表，不触发数据采集步骤。
func (w *BillingWorker) RunPipelineAggregateOnly(ctx context.Context) error {
	if w.pipelineRepo == nil {
		return nil
	}
	slog.Info("billing ETL: RunPipelineAggregateOnly start")
	now := time.Now().UTC()
	yesterday := now.AddDate(0, 0, -1).Truncate(24 * time.Hour)
	periodDay := yesterday.Format("2006-01-02")
	return w.runAggregateStep(ctx, now, yesterday, periodDay)
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
	rows, err := w.pipelineRepo.ListCloudBillDailyRawFromTo(ctx, from30, now, w.AccountID)
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
		m, _ := w.pipelineRepo.GetCloudBillMonthlyRaw(ctx, cycle, w.AccountID)
		if m == nil {
			slog.Info("billing full check: need full backfill", "reason", "monthly_missing", "cycle", cycle, "recent_months", RecentMonthsForIncremental)
			return true, nil
		}
	}
	slog.Debug("billing full check: incremental OK", "daily_rows_recent", len(rows))
	return false, nil
}

// FullBackfillRateLimit 全量回填时请求间隔（15_ 约 10 笔/秒，此处取 100ms）。[Ref: D2-6]
const FullBackfillRateLimit = 100 * time.Millisecond

// RunFullBackfill 首次或按需全量回填（D2-6）：按日限流拉取近 12 个月日数据、5 年月数据，落库后执行一次聚合；日表 12 个月支持自定义 6 个月、环比、环比趋势。[Ref: 01_实践] 部署/每日凌晨检查不通过时自动调用。
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

	// 按配置拉取日数据：最近 daily_pull_months 个月逐日拉取并落库 [Ref: 01_实践 配置控制拉取长度]
	fromDay := now.AddDate(0, -w.dailyPullMonths(), 0)
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
		cashMerged := make(map[string]float64)
		for k, v := range resp.CashByCategory {
			cashMerged[k] = v
		}
		snap := time.Now()
		if err := w.pipelineRepo.SaveCloudBillDailyRaw(ctx, postgres.CloudBillDailyRaw{
			BillDate:             d,
			TotalAmount:          resp.TotalAmount,
			ProductBreakdown:     merged,
			CashTotalAmount:      resp.CashTotalAmount,
			CashProductBreakdown: cashMerged,
			SnapshotAt:           snap,
			CreatedAt:            snap,
			AccountID:            w.AccountID,
		}); err != nil {
			slog.Warn("billing full backfill: save daily failed", "date", d.Format("2006-01-02"), "error", err)
		}
		time.Sleep(FullBackfillRateLimit)
	}

	// 按配置拉取月数据：最近 monthly_pull_months 个月逐月拉取并落库 [Ref: 01_实践 月源数据近5年 配置控制]
	nMonth := w.monthlyPullMonths()
	for i := 0; i < nMonth; i++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		t := now.AddDate(0, -nMonth+1+i, 0)
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
		cashMergedBf := make(map[string]float64)
		for k, v := range resp.CashByCategory {
			cashMergedBf[k] = v
		}
		cashTotalWrite := resp.CashTotalAmount
		if cashTotalWrite < 0 {
			cashTotalWrite = 0
			cashMergedBf = make(map[string]float64)
		}
		snap := time.Now()
		if err := w.pipelineRepo.SaveCloudBillMonthlyRaw(ctx, postgres.CloudBillMonthlyRaw{
			BillingCycle:         resp.BillingCycle,
			TotalAmount:          resp.TotalAmount,
			ProductBreakdown:     merged,
			CashTotalAmount:      cashTotalWrite,
			CashProductBreakdown: cashMergedBf,
			SnapshotAt:           snap,
			CreatedAt:            snap,
			AccountID:            w.AccountID,
		}); err != nil {
			slog.Warn("billing full backfill: save monthly failed", "billing_cycle", cycle, "error", err)
		}
		time.Sleep(FullBackfillRateLimit)
	}
	// 全量回填后按保留月数清理月表 [Ref: 01_实践 配置控制保存长度]
	if cutoff := now.AddDate(0, -w.monthlyRetentionMonths(), 0).Format("2006-01"); cutoff != "" {
		_ = w.pipelineRepo.DeleteCloudBillMonthlyRawOlderThan(ctx, cutoff, w.AccountID)
	}

	// 全量聚合：执行 RunPipeline 的 step5，写入全部 report_type 及对比周期 [Ref: 01_设计 §展示与延迟、首次回填与常规部署]
	yesterday := now.AddDate(0, 0, -1).Truncate(24 * time.Hour)
	if err := w.RunPipeline(ctx); err != nil {
		slog.Warn("billing full backfill: pipeline after backfill failed", "error", err)
		return err
	}
	slog.Info("billing full backfill: pipeline completed", "yesterday", yesterday.Format("2006-01-02"))
	return nil
}

// ReconcileThreshold 日/月日表对账偏差告警阈值 1%（06_ D3）。
const ReconcileThreshold = 0.01

// RunReconcile 每日对账：日原始表当月 sum(total_amount) vs 月原始表该月 total_amount；偏差 >1% 告警并记日志（D3-1）。
// [Ref: 01_实践 D3-2、D7-3 必选] 聚合表「本月」与日表交叉校验；审计日志记录校验结果便于回放。
func (w *BillingWorker) RunReconcile(ctx context.Context) error {
	if w.pipelineRepo == nil {
		return nil
	}
	now := time.Now().UTC()
	cycle := now.Format("2006-01")
	first := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	yesterday := now.AddDate(0, 0, -1).Truncate(24 * time.Hour)
	rows, err := w.pipelineRepo.ListCloudBillDailyRawFromTo(ctx, first, now, w.AccountID)
	if err != nil {
		slog.Warn("billing reconcile: list daily raw failed", "error", err)
		return err
	}
	var daySum float64
	for _, r := range rows {
		daySum += r.TotalAmount
	}
	mon, err := w.pipelineRepo.GetCloudBillMonthlyRaw(ctx, cycle, w.AccountID)
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
	aggPct := 0.0
	aggTotal := 0.0
	// [Ref: 01_实践 D3-2 必选] 聚合表「本月」与日表当月1日至昨日 sum 交叉校验；偏差 >1% 告警
	if agg, _ := w.pipelineRepo.GetCloudBillAggregate(ctx, "month", cycle); agg != nil && agg.TotalAmount > 0 {
		aggTotal = agg.TotalAmount
		aggPct = math.Abs(daySumToYesterday-agg.TotalAmount) / agg.TotalAmount
		if aggPct > ReconcileThreshold {
			slog.Warn("billing reconcile: aggregate month vs daily sum (to yesterday) diff over 1%", "daily_sum_to_yesterday", daySumToYesterday, "aggregate_total", agg.TotalAmount, "diff_pct", aggPct)
			if w.OnReconcileAlert != nil {
				w.OnReconcileAlert(daySumToYesterday, agg.TotalAmount, aggPct)
			}
		}
	}
	// [Ref: 01_实践 D7-3 必选] 审计日志：每次 ETL 校验结果便于回放
	slog.Info("billing ETL audit: reconcile",
		"billing_cycle", cycle,
		"daily_sum", daySum,
		"month_total", monthTotal,
		"diff_pct", diffPct,
		"daily_sum_to_yesterday", daySumToYesterday,
		"aggregate_total", aggTotal,
		"aggregate_diff_pct", aggPct,
		"day_month_ok", diffPct <= ReconcileThreshold,
		"agg_ok", aggPct <= ReconcileThreshold)
	return nil
}
