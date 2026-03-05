// Package service provides business logic services for the HTTP API.
package service

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/myxxhui/lighthouse-src/internal/data/postgres"
	"github.com/myxxhui/lighthouse-src/internal/server/dto"
	"github.com/myxxhui/lighthouse-src/pkg/costmodel"
)

// CostService provides cost-related business logic using Mock data and costmodel.
type CostService struct {
	repo postgres.Repository

	// [Ref: D8-7] 日期选择叠加结果短时缓存，key=from:to，TTL 1h
	dateRangeCache    map[string]dateRangeCacheEntry
	dateRangeCacheMu  sync.RWMutex
	dateRangeCacheTTL time.Duration
}

type dateRangeCacheEntry struct {
	resp *dto.GlobalCostResponse
	exp  time.Time
}

// NewCostService creates a new CostService with the given repository.
func NewCostService(repo postgres.Repository) *CostService {
	return &CostService{
		repo:              repo,
		dateRangeCache:    make(map[string]dateRangeCacheEntry),
		dateRangeCacheTTL: time.Hour,
	}
}

// CostDiagnostic 成本数据源诊断，供 GET /api/v1/cost/diagnostic 返回；用于排查「暂无数据」根因。[Ref: 01_实践 §2.4]
type CostDiagnostic struct {
	DailyRawCountLast30   int     `json:"daily_raw_count_last_30d"`   // 近 30 天日原始表行数
	DailyRawCashSumLast30 float64 `json:"daily_raw_cash_sum_last_30d"` // 近 30 天日表 cash_total_amount 代数和
	MonthlyRawCountRecent int     `json:"monthly_raw_count_recent_3"` // 上月/前两月/前三月月原始表存在数（0～3）
	AggregateMonthRows    int     `json:"aggregate_month_rows"`       // 聚合表 report_type=month 当前月 period_key 行数
	AggregateMonthTotal   float64 `json:"aggregate_month_total"`      // 上述行 total_amount 合计
	Hint                  string `json:"hint"`                        // 简要排查建议
}

// GetCostDiagnostic 返回成本数据源健康情况，便于定位「无数据」是 ETL 未写、API 返回 0 还是读路径问题。
func (s *CostService) GetCostDiagnostic(ctx context.Context) (*CostDiagnostic, error) {
	now := time.Now().UTC()
	from30 := now.AddDate(0, 0, -30).Truncate(24 * time.Hour)
	out := &CostDiagnostic{}
	rows, err := s.repo.ListCloudBillDailyRawFromTo(ctx, from30, now)
	if err == nil {
		out.DailyRawCountLast30 = len(rows)
		for _, r := range rows {
			out.DailyRawCashSumLast30 += r.CashTotalAmount
		}
	}
	for i := 1; i <= 3; i++ {
		cycle := now.AddDate(0, -i, 0).Format("2006-01")
		m, _ := s.repo.GetCloudBillMonthlyRaw(ctx, cycle)
		if m != nil {
			out.MonthlyRawCountRecent++
		}
	}
	month := now.Format("2006-01")
	list, _ := s.repo.ListCloudBillAggregateForReportPeriod(ctx, "month", month, "payment")
	out.AggregateMonthRows = len(list)
	for _, a := range list {
		out.AggregateMonthTotal += a.TotalAmount
	}
	// 简要提示
	if out.DailyRawCountLast30 == 0 && out.MonthlyRawCountRecent == 0 {
		out.Hint = "日/月原始表均无数据：请确认 1) CLOUD_BILLING_PROVIDER=aliyun 且已配置 AK/SK；2) 后端启动后 ETL 已执行（查看日志 billing ETL pipeline）；3) 已执行 migrate-02/migrate-03 等迁移"
	} else if out.DailyRawCountLast30 > 0 && out.DailyRawCashSumLast30 == 0 {
		out.Hint = "日表有行但 cash_total_amount 全为 0：可能为按日 API 未返回实付或 ETL 未写 Cash 字段；确认 FetchBillOverviewByDay 与 fallback 已写入 CashTotalAmount/CashProductBreakdown"
	} else if out.AggregateMonthRows == 0 && (out.DailyRawCountLast30 > 0 || out.MonthlyRawCountRecent > 0) {
		out.Hint = "日/月表有数据但聚合表无当月：RunPipeline 聚合步骤可能未执行或失败；查看日志 runAggregateStep"
	} else if out.AggregateMonthTotal > 0 {
		out.Hint = "数据源正常；若前端仍为 0 请检查 period 与 reportType/periodKey 映射及前端请求参数"
	} else {
		out.Hint = "聚合表有行但 total_amount=0；或仅历史月有数据、当月为 0（T+1 延迟或当月暂无消费）"
	}
	return out, nil
}

// toCostmodelDailyNamespaceCost converts postgres.DailyNamespaceCost to costmodel.DailyNamespaceCost.
func toCostmodelDailyNamespaceCost(p postgres.DailyNamespaceCost) costmodel.DailyNamespaceCost {
	return costmodel.DailyNamespaceCost{
		Namespace:     p.Namespace,
		Date:          p.Date,
		BillableCost:  p.BillableCost,
		UsageCost:     p.UsageCost,
		WasteCost:     p.WasteCost,
		PodCount:      p.PodCount,
		NodeCount:     p.NodeCount,
		WorkloadCount: p.WorkloadCount,
	}
}

// domainOrderForBreakdown 成本分解五大类，固定顺序 [Ref: 01_设计 §成本结构、成本分解；service_test 期望五类含「其他」]
var domainOrderForBreakdown = []string{"计算资源", "存储", "网络", "安全", "其他"}

// buildDomainBreakdownNormalized 从 merged（领域/领域:产品码）构建 domain_breakdown：五大类；「其它」归并入「其他」，按固定顺序返回。[Ref: 01_设计 §成本分解]
func buildDomainBreakdownNormalized(merged map[string]float64) []dto.DomainBreakdownItem {
	// 「其它」→「其他」（统一写法）
	if v, ok := merged["其它"]; ok {
		merged["其他"] += v
		delete(merged, "其它")
	}
	var toOther []string
	for k := range merged {
		if strings.HasPrefix(k, "其它:") {
			toOther = append(toOther, k)
		}
	}
	for _, k := range toOther {
		suffix := strings.TrimPrefix(k, "其它:")
		merged["其他:"+suffix] += merged[k]
		delete(merged, k)
	}
	if _, ok := merged["安全"]; !ok {
		merged["安全"] = 0
	}
	if _, ok := merged["其他"]; !ok {
		merged["其他"] = 0
	}
	out := make([]dto.DomainBreakdownItem, 0, len(domainOrderForBreakdown))
	for _, domain := range domainOrderForBreakdown {
		cost := merged[domain]
		topProducts := topProductsForDomain(&merged, domain, 4)
		out = append(out, dto.DomainBreakdownItem{
			Domain:           domain,
			Cost:             cost,
			OptimizableSpace: 0,
			Efficiency:       0,
			TopProducts:      topProducts,
		})
	}
	return out
}

// topProductsForDomain 从 productBreakdown 中取 key 为 "domain:ProductCode" 的项，按金额降序取前 n 个 [Ref: 01_成本透视真实数据]。
func topProductsForDomain(pb *map[string]float64, domain string, n int) []dto.ProductCostItem {
	if pb == nil || n <= 0 {
		return nil
	}
	prefix := domain + ":"
	var list []dto.ProductCostItem
	for k, cost := range *pb {
		if !strings.HasPrefix(k, prefix) || cost <= 0 {
			continue
		}
		product := strings.TrimPrefix(k, prefix)
		list = append(list, dto.ProductCostItem{Product: product, Cost: cost})
	}
	if len(list) == 0 {
		return nil
	}
	sort.Slice(list, func(i, j int) bool { return list[i].Cost > list[j].Cost })
	if len(list) > n {
		list = list[:n]
	}
	return list
}

// aggregateCloudBillByPeriod 按时间范围聚合云账单：month/1d/7d/30d 用当月账期，quarter 用本季度三月汇总。
// 返回 (有云账单, 总金额, 领域分项)；无数据时 (false, 0, nil)。
func (s *CostService) aggregateCloudBillByPeriod(ctx context.Context, period string) (bool, float64, *map[string]float64) {
	now := time.Now().UTC()
	currentCycle := now.Format("2006-01")
	switch period {
	case "quarter":
		// [Ref: 16_ §3.3] 优先从月原始表读本季度三月实付，再降级 cost_cloud_bill_summary
		curQ := (int(now.Month())-1)/3 + 1
		qStartMonth := (curQ-1)*3 + 1
		cycles := []string{
			fmt.Sprintf("%04d-%02d", now.Year(), qStartMonth),
			fmt.Sprintf("%04d-%02d", now.Year(), qStartMonth+1),
			fmt.Sprintf("%04d-%02d", now.Year(), qStartMonth+2),
		}
		var total float64
		merged := make(map[string]float64)
		for _, cycle := range cycles {
			if mon, err := s.repo.GetCloudBillMonthlyRaw(ctx, cycle); err == nil && mon != nil {
				total += mon.CashTotalAmount
				for k, v := range mon.CashProductBreakdown {
					merged[k] += v
				}
			}
		}
		if total != 0 || len(merged) > 0 {
			return true, total, &merged
		}
		list, err := s.repo.GetCloudBillSummariesForBillingCycles(ctx, cycles)
		if err != nil || len(list) == 0 {
			return false, 0, nil
		}
		total = 0
		merged = make(map[string]float64)
		for _, c := range list {
			total += c.TotalAmount
			for k, v := range c.ProductBreakdown {
				merged[k] += v
			}
		}
		return true, total, &merged
	case "last_month":
		prevCycle := now.AddDate(0, -1, 0).Format("2006-01")
		mon, err := s.repo.GetCloudBillMonthlyRaw(ctx, prevCycle)
		if err != nil || mon == nil {
			cloud, _ := s.repo.GetLatestCloudBillSummaryForBillingCycle(ctx, prevCycle)
			if cloud == nil {
				return false, 0, nil
			}
			return true, cloud.TotalAmount, &cloud.ProductBreakdown
		}
		return true, mon.CashTotalAmount, &mon.CashProductBreakdown
	case "last_week":
		// 上周：从日原始表聚合上一周 7 天
		weekday := int(now.Weekday())
		if weekday == 0 {
			weekday = 7
		}
		lastWeekEnd := now.AddDate(0, 0, -weekday)
		lastWeekStart := lastWeekEnd.AddDate(0, 0, -6)
		rows, err := s.repo.ListCloudBillDailyRawFromTo(ctx, lastWeekStart, lastWeekEnd)
		if err != nil || len(rows) == 0 {
			return false, 0, nil
		}
		total, merged := mergeDailyRowsSafe(rows)
		return true, total, &merged
	case "last_quarter":
		// [Ref: 16_ §3.3] 上季度优先从月原始表读实付（与 quarter 一致），再降级 cost_cloud_bill_summary，避免口径混用导致「数据不对」
		curMonth := int(now.Month())
		curYear := now.Year()
		var prevQStartMonth, prevQYear int
		switch {
		case curMonth <= 3:
			prevQStartMonth, prevQYear = 10, curYear-1
		case curMonth <= 6:
			prevQStartMonth, prevQYear = 1, curYear
		case curMonth <= 9:
			prevQStartMonth, prevQYear = 4, curYear
		default:
			prevQStartMonth, prevQYear = 7, curYear
		}
		prevCycles := []string{
			fmt.Sprintf("%04d-%02d", prevQYear, prevQStartMonth),
			fmt.Sprintf("%04d-%02d", prevQYear, prevQStartMonth+1),
			fmt.Sprintf("%04d-%02d", prevQYear, prevQStartMonth+2),
		}
		var total float64
		merged := make(map[string]float64)
		for _, cycle := range prevCycles {
			if mon, err := s.repo.GetCloudBillMonthlyRaw(ctx, cycle); err == nil && mon != nil {
				total += mon.CashTotalAmount
				for k, v := range mon.CashProductBreakdown {
					merged[k] += v
				}
			}
		}
		if total != 0 || len(merged) > 0 {
			return true, total, &merged
		}
		list, err := s.repo.GetCloudBillSummariesForBillingCycles(ctx, prevCycles)
		if err != nil || len(list) == 0 {
			return false, 0, nil
		}
		total = 0
		merged = make(map[string]float64)
		for _, c := range list {
			total += c.TotalAmount
			for k, v := range c.ProductBreakdown {
				merged[k] += v
			}
		}
		return true, total, &merged
	case "this_year":
		// [Ref: 01_实践 §this_year 算法] 历史完整月（月原始表）+ 当月日累加。
		// 严禁纯日粒度年累加（延迟 + 调账双重误差）。
		thisYear := now.Year()
		currentMonth := int(now.Month())
		var total float64
		merged := make(map[string]float64)
		for m := 1; m < currentMonth; m++ {
			cycle := fmt.Sprintf("%04d-%02d", thisYear, m)
			if mon, err := s.repo.GetCloudBillMonthlyRaw(ctx, cycle); err == nil && mon != nil {
				total += mon.CashTotalAmount
				for k, v := range mon.CashProductBreakdown {
					merged[k] += v
				}
			}
		}
		yesterday := now.AddDate(0, 0, -1).Truncate(24 * time.Hour)
		firstOfCurrentMonth := time.Date(thisYear, time.Month(currentMonth), 1, 0, 0, 0, 0, time.UTC)
		rows, _ := s.repo.ListCloudBillDailyRawFromTo(ctx, firstOfCurrentMonth, yesterday)
		if len(rows) > 0 {
			curTotal, curMerged := mergeDailyRowsSafe(rows)
			total += curTotal
			for k, v := range curMerged {
				merged[k] += v
			}
		}
		if total == 0 && len(merged) == 0 {
			return false, 0, nil
		}
		return true, total, &merged
	case "last_year":
		// [Ref: 01_实践 D9-10] 去年全年12个月月账单汇总（月原始表，避免日数据缺漏与调账误差）
		lastYear := now.Year() - 1
		var total float64
		merged := make(map[string]float64)
		for m := 1; m <= 12; m++ {
			cycle := fmt.Sprintf("%04d-%02d", lastYear, m)
			if mon, err := s.repo.GetCloudBillMonthlyRaw(ctx, cycle); err == nil && mon != nil {
				total += mon.CashTotalAmount
				for k, v := range mon.CashProductBreakdown {
					merged[k] += v
				}
			}
		}
		if total == 0 && len(merged) == 0 {
			return false, 0, nil
		}
		return true, total, &merged
	case "1d", "7d", "30d", "90d", "month", "":
		// [Ref: 16_ §3.3] 优先从新流水线表（daily_raw/monthly_raw）读实付，无数据再降级 cost_cloud_bill_summary
		yesterday := now.AddDate(0, 0, -1).Truncate(24 * time.Hour)
		switch period {
		case "month", "":
			if mon, err := s.repo.GetCloudBillMonthlyRaw(ctx, currentCycle); err == nil && mon != nil && (mon.CashTotalAmount != 0 || len(mon.CashProductBreakdown) > 0) {
				return true, mon.CashTotalAmount, &mon.CashProductBreakdown
			}
			// 当月可能尚未写入月表，用日表 1 号至昨日叠加
			firstOfMonth := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
			if rows, err := s.repo.ListCloudBillDailyRawFromTo(ctx, firstOfMonth, yesterday); err == nil && len(rows) > 0 {
				total, merged := mergeDailyRowsSafe(rows)
				if total != 0 || len(merged) > 0 {
					return true, total, &merged
				}
			}
		case "1d":
			if rows, err := s.repo.ListCloudBillDailyRawFromTo(ctx, yesterday, yesterday); err == nil && len(rows) > 0 {
				total, merged := mergeDailyRowsSafe(rows)
				if total != 0 || len(merged) > 0 {
					return true, total, &merged
				}
			}
		case "7d":
			from := yesterday.AddDate(0, 0, -6)
			if rows, err := s.repo.ListCloudBillDailyRawFromTo(ctx, from, yesterday); err == nil && len(rows) > 0 {
				total, merged := mergeDailyRowsSafe(rows)
				if total != 0 || len(merged) > 0 {
					return true, total, &merged
				}
			}
		case "30d":
			from := yesterday.AddDate(0, 0, -29)
			if rows, err := s.repo.ListCloudBillDailyRawFromTo(ctx, from, yesterday); err == nil && len(rows) > 0 {
				total, merged := mergeDailyRowsSafe(rows)
				if total != 0 || len(merged) > 0 {
					return true, total, &merged
				}
			}
		case "90d":
			from := yesterday.AddDate(0, 0, -89)
			if rows, err := s.repo.ListCloudBillDailyRawFromTo(ctx, from, yesterday); err == nil && len(rows) > 0 {
				total, merged := mergeDailyRowsSafe(rows)
				if total != 0 || len(merged) > 0 {
					return true, total, &merged
				}
			}
		}
		// 降级：旧表 cost_cloud_bill_summary（兼容未跑新 ETL 的环境）
		cloud, err := s.repo.GetLatestCloudBillSummaryForBillingCycle(ctx, currentCycle)
		if err != nil || cloud == nil {
			cloud, err = s.repo.GetLatestCloudBillSummary(ctx)
			if err != nil || cloud == nil {
				return false, 0, nil
			}
		}
		return true, cloud.TotalAmount, &cloud.ProductBreakdown
	default:
		cloud, err := s.repo.GetLatestCloudBillSummary(ctx)
		if err != nil || cloud == nil {
			return false, 0, nil
		}
		return true, cloud.TotalAmount, &cloud.ProductBreakdown
	}
}

// GetGlobalCostByDateRange 按日期范围从日原始表叠加（D8-2）；仅支持最近 6 个月内，叠加后与月表或已有聚合做可选校验。
// [Ref: D8-6] 仅通过 ListCloudBillDailyRawFromTo 单次查询（按 bill_date 索引、只取 [from,to] 所选日期），叠加与校验在内存及可选一次月表查询内完成，无全表扫描。
// [Ref: D8-7] 相同日期范围结果短时缓存 1h，重复请求直接命中。
func (s *CostService) GetGlobalCostByDateRange(ctx context.Context, from, to time.Time) (*dto.GlobalCostResponse, error) {
	const maxMonths = 6
	if to.Before(from) {
		from, to = to, from
	}
	if to.Sub(from) > maxMonths*30*24*time.Hour {
		return nil, nil // 超出 6 个月返回 nil，由调用方回退
	}
	key := from.Format("2006-01-02") + ":" + to.Format("2006-01-02")
	now := time.Now().UTC()
	s.dateRangeCacheMu.RLock()
	if e, ok := s.dateRangeCache[key]; ok && e.exp.After(now) {
		s.dateRangeCacheMu.RUnlock()
		return e.resp, nil
	}
	s.dateRangeCacheMu.RUnlock()

	rows, err := s.repo.ListCloudBillDailyRawFromTo(ctx, from, to)
	if err != nil || len(rows) == 0 {
		return nil, nil
	}
	total, merged := mergeDailyRowsSafe(rows)
	// D8-3 可选校验：与当月月表比对（若范围跨月则仅做参考）
	if mon, _ := s.repo.GetCloudBillMonthlyRaw(ctx, from.Format("2006-01")); mon != nil && mon.CashTotalAmount > 0 {
		diffPct := (total - mon.CashTotalAmount) / mon.CashTotalAmount
		if diffPct < 0 {
			diffPct = -diffPct
		}
		if diffPct > 0.01 {
			// 偏差 >1% 可打日志或告警（此处仅校验逻辑存在）
		}
	}

	domainBreakdown := buildDomainBreakdownNormalized(merged)
	// 自定义日期路径无聚合表，env_breakdown 暂返回空；按环境汇总需日原始表带 account_id 后实现 [Ref: 01_设计 §展示与延迟]
	resp := &dto.GlobalCostResponse{
		TotalCost:        total,
		TotalOptimizable: 0,
		GlobalEfficiency: 0,
		DomainBreakdown:  domainBreakdown,
		EnvBreakdown:     []dto.EnvBreakdownItem{},
		Namespaces:       nil,
		Timestamp:        now,
	}
	s.dateRangeCacheMu.Lock()
	s.dateRangeCache[key] = dateRangeCacheEntry{resp: resp, exp: now.Add(s.dateRangeCacheTTL)}
	s.dateRangeCacheMu.Unlock()
	return resp, nil
}

// resolveBillDataStatus 根据 period 类型和时间推断账单对账状态，用于前端三段式状态标识。
// [Ref: 16_云账单动态对账与高可靠处理规范 §三段式聚合策略]
//   - 历史月（periodKey 为上月或更早的 YYYY-MM）→ 从 month_status 表读真实状态
//   - 当前月 / 1d / this_week → "PRELIMINARY"（动态同步中）
func (s *CostService) resolveBillDataStatus(ctx context.Context, reportType, periodKey string, now time.Time) string {
	currentMonth := now.Format("2006-01")
	switch reportType {
	case "month":
		if periodKey == currentMonth {
			return "PRELIMINARY"
		}
		// 历史月：查 month_status 表
		if ms, err := s.repo.GetCloudBillMonthStatus(ctx, periodKey, ""); err == nil && ms != nil {
			return ms.DataStatus
		}
		return "FINALIZED" // 无 month_status 记录时，历史月默认视为已结算
	case "1d", "7d", "this_week":
		return "PRELIMINARY"
	default:
		// quarter/this_year 等跨月周期：任一月仍为 PRELIMINARY 则整体 PRELIMINARY
		return "PRELIMINARY"
	}
}

// mergeDailyRowsSafe 聚合日原始行；以 CashAmount 为基准（实际付款），负值为实际现金退款，代数参与计算。[Ref: 16_ §三、01_实践 §信用调整处理]
func mergeDailyRowsSafe(rows []postgres.CloudBillDailyRaw) (float64, map[string]float64) {
	var total float64
	merged := make(map[string]float64)
	for _, r := range rows {
		total += r.CashTotalAmount
		for k, v := range r.CashProductBreakdown {
			merged[k] += v
		}
	}
	return total, merged
}

// ErrFallbackTimeout D1-4：降级从原始表聚合时查询超时，调用方应返回 503。
var ErrFallbackTimeout = errors.New("cost fallback query timeout")

// FallbackQueryTimeout D1-4：降级路径下从日原始表聚合的查询超时时间。
const FallbackQueryTimeout = 5 * time.Second

// reportTypeAndPeriodKey 返回常规 period 对应的聚合表 (report_type, period_key)。[Ref: 04_01_成本透视真实数据 展示与延迟说明、01_设计 report_type 与 period_key]
func reportTypeAndPeriodKey(period string, now time.Time) (reportType, periodKey string) {
	yesterday := now.AddDate(0, 0, -1)
	yesterdayStr := yesterday.Format("2006-01-02")
	month := now.Format("2006-01")
	prevMonth := now.AddDate(0, -1, 0).Format("2006-01")
	q := (int(now.Month())-1)/3 + 1
	quarter := fmt.Sprintf("%s-Q%d", now.Format("2006"), q)
	prevQ := q - 1
	prevY := now.Year()
	if prevQ <= 0 {
		prevQ = 4
		prevY--
	}
	prevQuarter := fmt.Sprintf("%d-Q%d", prevY, prevQ)
	switch period {
	case "1d":
		return "1d", yesterdayStr
	case "this_week":
		// 这周：ISO 周（周一为第一天），period_key=YYYY-Www [Ref: 01_设计 report_type 与 period_key]
		year, week := now.ISOWeek()
		return "this_week", fmt.Sprintf("%04d-W%02d", year, week)
	case "last_week":
		weekday := int(now.Weekday())
		if weekday == 0 {
			weekday = 7
		}
		lastWeekEnd := now.AddDate(0, 0, -weekday)
		return "last_week", lastWeekEnd.Format("2006-01-02")
	case "month", "":
		return "month", month
	case "last_month":
		return "last_month", prevMonth
	case "quarter":
		return "quarter", quarter
	case "last_quarter":
		return "last_quarter", prevQuarter
	case "this_year":
		// 今年：当年1月1日至昨日（按日累加，T+1 精度）
		return "this_year", now.Format("2006")
	case "last_year":
		return "last_year", now.AddDate(-1, 0, 0).Format("2006")
	default:
		return "", ""
	}
}

// previousPeriodKey 返回上一周期的 period_key（用于 compare_mode=previous）。[Ref: 01_设计 report_type 与 period_key]
func previousPeriodKey(reportType, periodKey string, now time.Time) string {
	switch reportType {
	case "1d":
		t, _ := time.Parse("2006-01-02", periodKey)
		return t.AddDate(0, 0, -1).Format("2006-01-02")
	case "this_week":
		var y, w int
		if _, err := fmt.Sscanf(periodKey, "%04d-W%02d", &y, &w); err != nil {
			return ""
		}
		if w > 1 {
			return fmt.Sprintf("%04d-W%02d", y, w-1)
		}
		// 支持 ISO W53 长年：回推7天取真实 ISO 周号
		refDate := time.Date(y, 1, 4, 0, 0, 0, 0, time.UTC) // ISO 定义 1月4日必属于 W01
		refDate = refDate.AddDate(0, 0, -7)
		prevY, prevW := refDate.ISOWeek()
		return fmt.Sprintf("%04d-W%02d", prevY, prevW)
	case "last_week":
		t, _ := time.Parse("2006-01-02", periodKey)
		return t.AddDate(0, 0, -7).Format("2006-01-02")
	case "month", "last_month":
		t, _ := time.Parse("2006-01", periodKey)
		return t.AddDate(0, -1, 0).Format("2006-01")
	case "quarter", "last_quarter":
		var y int
		var q int
		_, _ = fmt.Sscanf(periodKey, "%d-Q%d", &y, &q)
		if q <= 1 {
			return fmt.Sprintf("%d-Q4", y-1)
		}
		return fmt.Sprintf("%d-Q%d", y, q-1)
	case "this_year", "last_year":
		var y int
		if _, err := fmt.Sscanf(periodKey, "%d", &y); err == nil {
			return fmt.Sprintf("%d", y-1)
		}
		return ""
	default:
		return ""
	}
}

// buildEnvBreakdown 从 cost_env_account_config 与聚合表按环境汇总 env_breakdown。[Ref: 01_设计 §按环境展示、§后端数据聚合与存储方案]
func (s *CostService) buildEnvBreakdown(ctx context.Context, reportType, periodKey, prevPeriodKey string) []dto.EnvBreakdownItem {
	configs, err := s.repo.ListEnvAccountConfig(ctx)
	if err != nil {
		return nil
	}
	envByAccount := make(map[string]postgres.EnvAccountConfig)
	for _, c := range configs {
		envByAccount[c.Environment] = c
	}
	curList, _ := s.repo.ListCloudBillAggregateForReportPeriod(ctx, reportType, periodKey, "payment")
	prevList, _ := s.repo.ListCloudBillAggregateForReportPeriod(ctx, reportType, prevPeriodKey, "payment")
	curByAccount := make(map[string]float64)
	for _, a := range curList {
		k := a.AccountID
		curByAccount[k] += a.TotalAmount
	}
	prevByAccount := make(map[string]float64)
	for _, a := range prevList {
		prevByAccount[a.AccountID] += a.TotalAmount
	}
	order := []string{"POC", "FAT", "UAT", "PROD"}
	out := make([]dto.EnvBreakdownItem, 0, 4)
	for _, env := range order {
		c, ok := envByAccount[env]
		if !ok {
			out = append(out, dto.EnvBreakdownItem{Environment: env, AccountDisplayName: "未配置"})
			continue
		}
		total := curByAccount[c.AccountID]
		if total == 0 {
			total = curByAccount[c.Environment] // 单账号时 ETL 可能写入 environment 名（如 POC），与 config.account_id 不一致时用环境名回退 [Ref: 01_设计 §按环境展示]
		}
		if total == 0 && curByAccount[""] > 0 {
			total = curByAccount[""] // 单账号占位符 '' 时，将总账归入该已配置环境 [Ref: 01_设计 §按环境展示、D1 排查]
		}
		prev := prevByAccount[c.AccountID]
		if prev == 0 {
			prev = prevByAccount[c.Environment]
		}
		if prev == 0 && prevByAccount[""] > 0 {
			prev = prevByAccount[""]
		}
		changePct := 0.0
		if prev > 0 {
			changePct = ((total - prev) / prev) * 100
		}
		displayName := c.DisplayName
		if displayName == "" {
			displayName = c.AccountID
		}
		out = append(out, dto.EnvBreakdownItem{
			Environment:        env,
			AccountID:          c.AccountID,
			AccountDisplayName: displayName,
			TotalCost:          total,
			PreviousPeriodCost: prev,
			ChangePct:          changePct,
		})
	}
	return out
}

// GetGlobalCost returns L0 cost. [Ref: 04_01_成本透视真实数据、16_ §三] 常规展示仅读聚合表（仅实际付款）；无聚合时降级 cost_cloud_bill_summary 或日原始表。
// period: 1d|7d|30d|month|quarter 时优先读 cost_cloud_bill_aggregate；自定义日期由 GetGlobalCostByDateRange 从日原始表叠加。
// [Ref: D1-4] 降级时仅从日原始表聚合最近 30 天并设 5s 超时，超时返回 ErrFallbackTimeout（HTTP 503）。
// metricType 已废弃，固定使用 "payment"（实际付款金额）；统计口径已移除。
func (s *CostService) GetGlobalCost(ctx context.Context, period string, metricType string) (*dto.GlobalCostResponse, error) {
	metricType = "payment"
	now := time.Now().UTC()
	// [Ref: 01_实践 展示与延迟说明] 常规展示仅读聚合表；多账号时从 List 合并 total 与 product_breakdown（01_设计 §后端数据聚合与存储方案）
	if reportType, periodKey := reportTypeAndPeriodKey(period, now); reportType != "" {
		list, _ := s.repo.ListCloudBillAggregateForReportPeriod(ctx, reportType, periodKey, metricType)
		if len(list) > 0 {
			var totalAmount float64
			merged := make(map[string]float64)
			var lastSuccessAt *time.Time
			for _, a := range list {
				totalAmount += a.TotalAmount
				for k, v := range a.ProductBreakdown {
					merged[k] += v
				}
				if a.LastSuccessAt != nil && (lastSuccessAt == nil || a.LastSuccessAt.After(*lastSuccessAt)) {
					lastSuccessAt = a.LastSuccessAt
				}
			}
		if totalAmount > 0 || len(merged) > 0 {
			domainBreakdown := buildDomainBreakdownNormalized(merged)
			prevKey := previousPeriodKey(reportType, periodKey, now)
			envBreakdown := s.buildEnvBreakdown(ctx, reportType, periodKey, prevKey)
			meta := &dto.GlobalCostMetadata{DataStatus: "aggregate", ReportType: reportType, PeriodKey: periodKey}
			if lastSuccessAt != nil {
				meta.LastUpdatedAt = lastSuccessAt
			}
			// [Ref: 16_云账单动态对账与高可靠处理规范 §三段式] 注入账单对账状态供前端区分"已财务核算"/"动态同步中"
			meta.BillDataStatus = s.resolveBillDataStatus(ctx, reportType, periodKey, now)
			return &dto.GlobalCostResponse{
				TotalCost:        totalAmount,
				TotalOptimizable: 0,
				GlobalEfficiency: 0,
				DomainBreakdown:  domainBreakdown,
				EnvBreakdown:     envBreakdown,
				Namespaces:       nil,
				Timestamp:        now,
				Metadata:         meta,
			}, nil
		}
		}
	// 聚合表无数据时从日原始表降级：1d/this_week 按对应区间聚合
	if period == "1d" || period == "this_week" {
		ctxFallback, cancel := context.WithTimeout(ctx, FallbackQueryTimeout)
		yesterday := now.AddDate(0, 0, -1).Truncate(24 * time.Hour)
		var from, to time.Time
		switch period {
		case "1d":
			from, to = yesterday, yesterday
		default: // this_week
			// 这周：本周一 00:00 至昨日（ISO 周周一为第一天）
			wd := now.Weekday()
			if wd == 0 {
				wd = 7
			}
			daysBack := int(wd - 1)
			thisWeekMonday := now.AddDate(0, 0, -daysBack).Truncate(24 * time.Hour)
			from, to = thisWeekMonday, yesterday
		}
		rows, err := s.repo.ListCloudBillDailyRawFromTo(ctxFallback, from, to)
		cancel()
		if err != nil {
			if errors.Is(err, context.DeadlineExceeded) {
				return nil, ErrFallbackTimeout
			}
		} else if len(rows) > 0 {
			total, merged := mergeDailyRowsSafe(rows)
			domainBreakdown := buildDomainBreakdownNormalized(merged)
			rType, pKey := "1d", yesterday.Format("2006-01-02")
			if period == "this_week" {
				year, week := now.ISOWeek()
				rType, pKey = "this_week", fmt.Sprintf("%04d-W%02d", year, week)
			}
			prevKey := previousPeriodKey(rType, pKey, now)
			envBreakdown := s.buildEnvBreakdown(ctx, rType, pKey, prevKey)
			metaFallback := &dto.GlobalCostMetadata{DataStatus: "fallback", BillDataStatus: "PRELIMINARY", ReportType: rType, PeriodKey: pKey}
			metaFallback.LastUpdatedAt = &now
			return &dto.GlobalCostResponse{
				TotalCost:        total,
				TotalOptimizable: 0,
				GlobalEfficiency: 0,
				DomainBreakdown:  domainBreakdown,
				EnvBreakdown:     envBreakdown,
				Namespaces:       nil,
				Timestamp:        now,
				Metadata:         metaFallback,
			}, nil
		}
	}
	}
	// 聚合表无数据时降级：cost_cloud_bill_summary（账期汇总）
	cloud, totalAmount, productBreakdown := s.aggregateCloudBillByPeriod(ctx, period)
	if cloud && productBreakdown != nil {
		domainBreakdown := buildDomainBreakdownNormalized(*productBreakdown)
		if reportType, periodKey := reportTypeAndPeriodKey(period, now); reportType != "" {
			prevKey := previousPeriodKey(reportType, periodKey, now)
			envBreakdown := s.buildEnvBreakdown(ctx, reportType, periodKey, prevKey)
			// [Ref: 01_成本透视] 降级读 summary 时聚合表常为 0，env_breakdown 全 0 导致前端“没数据”；将 summary 总账归入第一个已配置环境
			if totalAmount > 0 && len(envBreakdown) > 0 {
				var sum float64
				for i := range envBreakdown {
					sum += envBreakdown[i].TotalCost
				}
				if sum == 0 {
					for i := range envBreakdown {
						if envBreakdown[i].AccountDisplayName != "未配置" {
							envBreakdown[i].TotalCost = totalAmount
							break
						}
					}
					if len(envBreakdown) > 0 && envBreakdown[0].TotalCost == 0 {
						envBreakdown[0].TotalCost = totalAmount
					}
				}
			}
		metaFallback := &dto.GlobalCostMetadata{DataStatus: "fallback", BillDataStatus: "PRELIMINARY", ReportType: reportType, PeriodKey: periodKey}
		metaFallback.LastUpdatedAt = &now
			return &dto.GlobalCostResponse{
				TotalCost:        totalAmount,
				TotalOptimizable: 0,
				GlobalEfficiency: 0,
				DomainBreakdown:  domainBreakdown,
				EnvBreakdown:     envBreakdown,
				Namespaces:       nil,
				Timestamp:        now,
				Metadata:         metaFallback,
			}, nil
		}
	// period 为 7d/30d/90d 时无 reportType，仍返回总账与领域分解；env_breakdown 返回空数组保持结构一致
		metaFallback2 := &dto.GlobalCostMetadata{DataStatus: "fallback", BillDataStatus: "PRELIMINARY"}
		metaFallback2.LastUpdatedAt = &now
		return &dto.GlobalCostResponse{
			TotalCost:        totalAmount,
			TotalOptimizable: 0,
			GlobalEfficiency: 0,
			DomainBreakdown:  domainBreakdown,
			EnvBreakdown:     []dto.EnvBreakdownItem{},
			Namespaces:       nil,
			Timestamp:        now,
			Metadata:         metaFallback2,
		}, nil
	}

	// 回退：L1 聚合（Mock 或 02_ 数据）；L1 查询失败（如表不存在、库空）时返回 200 空结构，避免 500 导致前端「加载全局指标失败」[Ref: 01_实践]
	nowL1 := time.Now()
	start := nowL1.AddDate(0, 0, -7)
	costs, err := s.repo.AggregateDailyNamespaceCosts(ctx, start, nowL1)
	if err != nil {
		nowUTC := time.Now().UTC()
		rType, pKey := reportTypeAndPeriodKey(period, nowUTC)
		prevKey := previousPeriodKey(rType, pKey, nowUTC)
		envBreakdown := s.buildEnvBreakdown(ctx, rType, pKey, prevKey)
		return &dto.GlobalCostResponse{
			TotalCost:        0,
			TotalOptimizable: 0,
			GlobalEfficiency: 0,
			DomainBreakdown:  []dto.DomainBreakdownItem{},
			EnvBreakdown:     envBreakdown,
			Namespaces:       nil,
			Timestamp:        nowUTC,
		}, nil
	}
	modelCosts := make([]costmodel.DailyNamespaceCost, 0, len(costs))
	for _, c := range costs {
		modelCosts = append(modelCosts, toCostmodelDailyNamespaceCost(c))
	}
	_, err = costmodel.AggregateGlobal(modelCosts)
	if err != nil {
		return nil, err
	}
	breakdown, err := costmodel.CalculateDomainBreakdown(modelCosts)
	if err != nil {
		return nil, err
	}
	namespaces := make([]dto.NamespaceCostSummary, 0, len(breakdown))
	domainBreakdown := make([]dto.DomainBreakdownItem, 0, len(breakdown))
	var sumL1, sumOptimizable float64
	for _, b := range breakdown {
		eff := 0.0
		if b.BillableCost > 0 {
			eff = (b.UsageCost / b.BillableCost) * 100
		}
		grade := ""
		switch {
		case eff < 10:
			grade = "Zombie"
		case eff < 40:
			grade = "OverProvisioned"
		case eff < 90:
			grade = "Healthy"
		default:
			grade = "Risk"
		}
		nsCost := b.BillableCost + b.UsageCost + b.WasteCost
		sumL1 += nsCost
		sumOptimizable += b.WasteCost
		namespaces = append(namespaces, dto.NamespaceCostSummary{
			Name:      b.DomainName,
			Cost:      nsCost,
			Grade:     grade,
			PodCount:  b.PodCount,
			NodeCount: 0,
		})
		domainBreakdown = append(domainBreakdown, dto.DomainBreakdownItem{
			Domain:           b.DomainName,
			Cost:             nsCost,
			OptimizableSpace: b.WasteCost,
			Efficiency:       eff,
		})
	}
	globalEff := 0.0
	if sumL1 > 0 {
		globalEff = ((sumL1 - sumOptimizable) / sumL1) * 100
	}
	// L1 回退时也返回 env_breakdown（四环境槽位），与 12_API 契约一致 [Ref: 01_实践 §5.1]
	nowUTC := time.Now().UTC()
	rType, pKey := reportTypeAndPeriodKey(period, nowUTC)
	prevKey := previousPeriodKey(rType, pKey, nowUTC)
	envBreakdown := s.buildEnvBreakdown(ctx, rType, pKey, prevKey)
	return &dto.GlobalCostResponse{
		TotalCost:        sumL1,
		TotalOptimizable: sumOptimizable,
		GlobalEfficiency: globalEff,
		DomainBreakdown:  domainBreakdown,
		EnvBreakdown:     envBreakdown,
		Namespaces:       namespaces,
		Timestamp:        nowUTC,
	}, nil
}

// MixedQueryTimeSeries 混合查询：历史 cost_hourly_workload + 当日 Prometheus 合并的时间序列（占位）。
// 供趋势/全域视图使用；Phase4 实现历史表与当日实时数据合并。
func (s *CostService) MixedQueryTimeSeries(ctx context.Context, start, end time.Time, namespace string) ([]dto.GranularCostDataPoint, error) {
	// Phase3 占位：返回空切片；实现时合并 repo.AggregateHourlyWorkloadStats(start,end) 与当日 Prometheus 数据
	return nil, nil
}

// ListNamespaces returns all namespaces with cost summary for the frontend cost table.
func (s *CostService) ListNamespaces(ctx context.Context, period string) ([]dto.NamespaceCostSummary, error) {
	resp, err := s.GetGlobalCost(ctx, period, "payment")
	if err != nil {
		return nil, err
	}
	return resp.Namespaces, nil
}

// GetNamespaceCost returns L1 cost for a namespace.
func (s *CostService) GetNamespaceCost(ctx context.Context, namespace string) (*dto.NamespaceCostResponse, error) {
	now := time.Now().UTC()
	start := now.AddDate(0, 0, -7)

	costs, err := s.repo.AggregateDailyNamespaceCosts(ctx, start, now)
	if err != nil {
		return nil, err
	}

	var totalBillable, totalUsage, totalWaste float64
	for _, c := range costs {
		if c.Namespace == namespace {
			totalBillable += c.BillableCost
			totalUsage += c.UsageCost
			totalWaste += c.WasteCost
		}
	}

	efficiency := 0.0
	if totalBillable > 0 {
		efficiency = (totalUsage / totalBillable) * 100
	}

	return &dto.NamespaceCostResponse{
		Namespace: namespace,
		Cost: dto.CostBreakdown{
			Total:      totalBillable + totalUsage + totalWaste,
			Billable:   totalBillable,
			Usage:      totalUsage,
			Waste:      totalWaste,
			Efficiency: efficiency,
		},
		Timestamp: time.Now().UTC(),
	}, nil
}

// GetEnvDrilldown 按环境云产品钻取：返回该环境对应 account 的云产品成本列表。[Ref: 01_设计 §产品分类与按环境钻取、12_API]
// envId: POC|FAT|UAT|PROD；category 可选 compute|network|storage|security；sort 默认 cost_desc。
func (s *CostService) GetEnvDrilldown(ctx context.Context, envId, reportType, periodKey, category, sortOrder string) ([]dto.EnvDrilldownItem, error) {
	configs, err := s.repo.ListEnvAccountConfig(ctx)
	if err != nil {
		return nil, err
	}
	var accountID string
	for _, c := range configs {
		if c.Environment == envId {
			accountID = c.AccountID
			break
		}
	}
	if accountID == "" && envId != "" {
		// 未配置该环境，返回空
		return []dto.EnvDrilldownItem{}, nil
	}
	list, err := s.repo.ListCloudBillAggregateForReportPeriod(ctx, reportType, periodKey, "payment")
	if err != nil || len(list) == 0 {
		return []dto.EnvDrilldownItem{}, nil
	}
	var pb map[string]float64
	for _, a := range list {
		if a.AccountID == accountID {
			pb = a.ProductBreakdown
			break
		}
	}
	if pb == nil {
		return []dto.EnvDrilldownItem{}, nil
	}
	// 从 product_breakdown 按产品汇总：key 形如 "domain:ProductCode"；方案 B 键前缀映射 category [Ref: 01_设计 §产品分类 方案 B]
	productCosts := make(map[string]float64)
	categoryFromPrefix := make(map[string]string)
	for k, cost := range pb {
		if idx := strings.Index(k, ":"); idx >= 0 && idx < len(k)-1 {
			code := k[idx+1:]
			productCosts[code] += cost
			prefix := k[:idx]
			if cat := domainPrefixToCategory(prefix); cat != "" && categoryFromPrefix[code] == "" {
				categoryFromPrefix[code] = cat
			}
		}
	}
	var out []dto.EnvDrilldownItem
	for code, cost := range productCosts {
		if cost <= 0 {
			continue
		}
		cat := s.resolveDrilldownCategory(ctx, code, categoryFromPrefix[code])
		if category != "" && cat != category {
			continue
		}
		out = append(out, dto.EnvDrilldownItem{
			ProductCode: code,
			ProductName: code,
			Cost:        cost,
			Category:    cat,
		})
	}
	if sortOrder != "cost_asc" {
		sort.Slice(out, func(i, j int) bool { return out[i].Cost > out[j].Cost })
	} else {
		sort.Slice(out, func(i, j int) bool { return out[i].Cost < out[j].Cost })
	}
	return out, nil
}

// domainPrefixToCategory 方案 B：product_breakdown 键前缀（中文领域）映射为 API category；仅四大类，其它/其他已合并入计算资源。[Ref: 01_设计 §产品分类；用户需求 仅四大分类]
func domainPrefixToCategory(prefix string) string {
	switch prefix {
	case "计算资源":
		return "compute"
	case "存储":
		return "storage"
	case "网络":
		return "network"
	case "安全":
		return "security"
	case "其它", "其他":
		return "compute"
	default:
		return ""
	}
}

// resolveDrilldownCategory 方案 B：前缀优先，否则查 product_category_mapping；未命中时归入计算资源（仅四大类）。[Ref: 01_设计 §产品分类；用户需求 仅四大分类]
func (s *CostService) resolveDrilldownCategory(ctx context.Context, code string, categoryFromPrefix string) string {
	if categoryFromPrefix != "" {
		return categoryFromPrefix
	}
	cat, _ := s.repo.GetProductCategory(ctx, code)
	if cat != "" && (cat == "compute" || cat == "storage" || cat == "network" || cat == "security") {
		return cat
	}
	return "compute"
}

// drilldownPeriodToDateRange 将 report_type+period_key 转为日期范围，用于聚合表无数据时从日表降级。[Ref: 01_实践 展示与延迟说明]
func drilldownPeriodToDateRange(reportType, periodKey string, now time.Time) (from, to time.Time, ok bool) {
	yesterday := now.AddDate(0, 0, -1).Truncate(24 * time.Hour)
	switch reportType {
	case "1d":
		t, err := time.Parse("2006-01-02", periodKey)
		if err != nil {
			return from, to, false
		}
		t = t.Truncate(24 * time.Hour)
		return t, t, true
	case "this_week":
		var y, w int
		if _, err := fmt.Sscanf(periodKey, "%04d-W%02d", &y, &w); err != nil {
			return from, to, false
		}
		// ISO 周：W01 为含 1 月 4 日的周，其周一为 from
		jan4 := time.Date(y, 1, 4, 0, 0, 0, 0, time.UTC)
		wd := int(jan4.Weekday())
		if wd == 0 {
			wd = 7
		}
		mondayW01 := jan4.AddDate(0, 0, -(wd - 1))
		monday := mondayW01.AddDate(0, 0, (w-1)*7)
		sunday := monday.AddDate(0, 0, 6)
		if yesterday.Before(monday) {
			return from, to, false
		}
		if yesterday.Before(sunday) {
			return monday, yesterday, true
		}
		return monday, sunday, true
	case "7d", "7d_range":
		t, err := time.Parse("2006-01-02", periodKey)
		if err != nil {
			return from, to, false
		}
		to = t.Truncate(24 * time.Hour)
		from = to.AddDate(0, 0, -6)
		return from, to, true
	case "30d":
		t, err := time.Parse("2006-01-02", periodKey)
		if err != nil {
			return from, to, false
		}
		to = t.Truncate(24 * time.Hour)
		from = to.AddDate(0, 0, -29)
		return from, to, true
	case "90d":
		t, err := time.Parse("2006-01-02", periodKey)
		if err != nil {
			return from, to, false
		}
		to = t.Truncate(24 * time.Hour)
		from = to.AddDate(0, 0, -89)
		return from, to, true
	case "last_week":
		t, err := time.Parse("2006-01-02", periodKey)
		if err != nil {
			return from, to, false
		}
		to = t.Truncate(24 * time.Hour)
		from = to.AddDate(0, 0, -6)
		return from, to, true
	// [Ref: 04_01_成本透视真实数据] 季度与年度环比降级：聚合表无历史记录时从日原始表计算
	case "quarter", "last_quarter":
		var y, q int
		if _, err := fmt.Sscanf(periodKey, "%d-Q%d", &y, &q); err != nil || q < 1 || q > 4 {
			return from, to, false
		}
		from = time.Date(y, time.Month((q-1)*3+1), 1, 0, 0, 0, 0, time.UTC)
		// 季度末：Q1=3月末, Q2=6月末, Q3=9月末, Q4=12月末
		lastMonth := time.Month(q * 3)
		to = time.Date(y, lastMonth+1, 0, 0, 0, 0, 0, time.UTC) // 下月第0天 = 本月最后一天
		if to.After(yesterday) {
			to = yesterday
		}
		if from.After(to) {
			return from, to, false
		}
		return from, to, true
	case "this_year", "last_year":
		var y int
		if _, err := fmt.Sscanf(periodKey, "%d", &y); err != nil {
			return from, to, false
		}
		from = time.Date(y, 1, 1, 0, 0, 0, 0, time.UTC)
		to = time.Date(y, 12, 31, 0, 0, 0, 0, time.UTC)
		if to.After(yesterday) {
			to = yesterday
		}
		if from.After(to) {
			return from, to, false
		}
		return from, to, true
	default:
		return from, to, false
	}
}

// GetGlobalDrilldown 全环境云产品明细：合并所有 account 的 product_breakdown 按产品汇总并打 category；env 非 all 时仅汇总该环境对应 account_id。[Ref: 01_设计 D9-8、D6 云产品成本明细索引、12_API；方案 B 键前缀映射]
// 聚合表无数据时对 1d/this_week/7d/7d_range/30d/90d/last_week 从日原始表降级聚合，与 GetGlobalCost 行为一致。
func (s *CostService) GetGlobalDrilldown(ctx context.Context, reportType, periodKey, category, sortOrder, env string) ([]dto.EnvDrilldownItem, error) {
	list, err := s.repo.ListCloudBillAggregateForReportPeriod(ctx, reportType, periodKey, "payment")
	if err != nil {
		return nil, err
	}
	if len(list) == 0 {
		from, to, ok := drilldownPeriodToDateRange(reportType, periodKey, time.Now().UTC())
		if ok {
			ctxFallback, cancel := context.WithTimeout(ctx, FallbackQueryTimeout)
			defer cancel()
			out, _ := s.GetGlobalDrilldownByDateRange(ctxFallback, from, to, category, sortOrder, env)
			return out, nil
		}
		return []dto.EnvDrilldownItem{}, nil
	}
	var accountIDs map[string]bool // env != "all" 时仅包含该环境映射的 account_id
	if env != "" && env != "all" {
		configs, err := s.repo.ListEnvAccountConfig(ctx)
		if err != nil {
			return nil, err
		}
		accountIDs = make(map[string]bool)
		for _, c := range configs {
			if c.Environment == env {
				accountIDs[c.AccountID] = true
				break
			}
		}
		if len(accountIDs) == 0 {
			return []dto.EnvDrilldownItem{}, nil
		}
	}
	productCosts := make(map[string]float64)
	categoryFromPrefix := make(map[string]string)
	for _, a := range list {
		if accountIDs != nil && !accountIDs[a.AccountID] {
			continue
		}
		for k, cost := range a.ProductBreakdown {
			if idx := strings.Index(k, ":"); idx >= 0 && idx < len(k)-1 {
				code := k[idx+1:]
				productCosts[code] += cost
				prefix := k[:idx]
				if cat := domainPrefixToCategory(prefix); cat != "" && categoryFromPrefix[code] == "" {
					categoryFromPrefix[code] = cat
				}
			}
		}
	}
	var out []dto.EnvDrilldownItem
	for code, cost := range productCosts {
		// 严格过滤：仅展示真正有消费成本的产品（>0.005，低于此值的四舍五入为零）
		if cost < 0.005 {
			continue
		}
		cat := s.resolveDrilldownCategory(ctx, code, categoryFromPrefix[code])
		if category != "" && cat != category {
			continue
		}
		out = append(out, dto.EnvDrilldownItem{
			ProductCode: code,
			ProductName: code,
			Cost:        cost,
			Category:    cat,
		})
	}
	if sortOrder != "cost_asc" {
		sort.Slice(out, func(i, j int) bool { return out[i].Cost > out[j].Cost })
	} else {
		sort.Slice(out, func(i, j int) bool { return out[i].Cost < out[j].Cost })
	}
	return out, nil
}

// GetGlobalDrilldownByDateRange 全环境云产品明细（自定义日期）：从日原始表按 [from,to] 聚合并打 category；env 过滤按 account_id。[Ref: 01_设计 GET /api/v1/cost/drilldown/global date_from/date_to；方案 B 键前缀映射]
func (s *CostService) GetGlobalDrilldownByDateRange(ctx context.Context, from, to time.Time, category, sortOrder, env string) ([]dto.EnvDrilldownItem, error) {
	const maxMonths = 6
	if to.Before(from) {
		from, to = to, from
	}
	if to.Sub(from) > maxMonths*30*24*time.Hour {
		return []dto.EnvDrilldownItem{}, nil
	}
	rows, err := s.repo.ListCloudBillDailyRawFromTo(ctx, from, to)
	if err != nil || len(rows) == 0 {
		return []dto.EnvDrilldownItem{}, nil
	}

	// 按环境过滤 account_id（与 GetGlobalDrilldown 逻辑一致）
	var filterAccountID string
	if env != "" && env != "all" {
		if configs, cerr := s.repo.ListEnvAccountConfig(ctx); cerr == nil {
			for _, c := range configs {
				if strings.EqualFold(c.Environment, env) {
					filterAccountID = c.AccountID
					break
				}
			}
		}
		if filterAccountID == "" {
			return []dto.EnvDrilldownItem{}, nil
		}
	}

	productCosts := make(map[string]float64)
	categoryFromPrefix := make(map[string]string)
	for _, r := range rows {
		if filterAccountID != "" && r.AccountID != filterAccountID {
			continue
		}
		for k, cost := range r.ProductBreakdown {
			if idx := strings.Index(k, ":"); idx >= 0 && idx < len(k)-1 {
				code := k[idx+1:]
				productCosts[code] += cost
				prefix := k[:idx]
				if cat := domainPrefixToCategory(prefix); cat != "" && categoryFromPrefix[code] == "" {
					categoryFromPrefix[code] = cat
				}
			}
		}
	}
	var out []dto.EnvDrilldownItem
	for code, cost := range productCosts {
		if cost < 0.005 {
			continue
		}
		cat := s.resolveDrilldownCategory(ctx, code, categoryFromPrefix[code])
		if category != "" && cat != category {
			continue
		}
		out = append(out, dto.EnvDrilldownItem{
			ProductCode: code,
			ProductName: code,
			Cost:        cost,
			Category:    cat,
		})
	}
	if sortOrder != "cost_asc" {
		sort.Slice(out, func(i, j int) bool { return out[i].Cost > out[j].Cost })
	} else {
		sort.Slice(out, func(i, j int) bool { return out[i].Cost < out[j].Cost })
	}
	return out, nil
}

// CostTrendMaxDays 趋势 API 最大天数 [Ref: 12_API GET /api/v1/cost/trend]
const CostTrendMaxDays = 90

// CostTrendTimeout 趋势查询超时 [Ref: 01_设计 §成本趋势 API]
const CostTrendTimeout = 10 * time.Second

// GetCostTrend 成本结构趋势：从日原始表按 [start,end] 聚合，返回按日序列；最大 90 天、超时 10s。
// envFilter 非空且非"all"时按环境 account_id 过滤。[Ref: 01_设计 D9-9、12_API GET /api/v1/cost/trend]
func (s *CostService) GetCostTrend(ctx context.Context, period string, dateFrom, dateTo *time.Time, envFilter string) (*dto.CostTrendResponse, error) {
	var from, to time.Time
	if dateFrom != nil && dateTo != nil {
		from = dateFrom.Truncate(24 * time.Hour)
		to = dateTo.Truncate(24 * time.Hour)
	} else {
		now := time.Now().UTC().Truncate(24 * time.Hour)
		yesterday := now.AddDate(0, 0, -1)
		switch period {
		case "7d", "7d_range":
			to = yesterday
			from = to.AddDate(0, 0, -6)
		case "30d":
			to = yesterday
			from = to.AddDate(0, 0, -29)
		case "90d":
			to = yesterday
			from = to.AddDate(0, 0, -89)
		default:
			to = yesterday
			from = to.AddDate(0, 0, -29)
		}
	}
	if to.Sub(from) > CostTrendMaxDays*24*time.Hour {
		from = to.AddDate(0, 0, -CostTrendMaxDays+1)
	}

	// [Ref: 01_设计 §环境与云账号配置] 按环境过滤：查找对应 account_id；未配置环境返回空趋势
	var filterAccountID string
	var envRequested bool
	if envFilter != "" && envFilter != "all" {
		envRequested = true
		if configs, err := s.repo.ListEnvAccountConfig(ctx); err == nil {
			for _, cfg := range configs {
				if strings.EqualFold(cfg.Environment, envFilter) {
					filterAccountID = cfg.AccountID
					break
				}
			}
		}
		// 环境已请求但无对应账号配置 → 该环境无账单，返回空序列
		if filterAccountID == "" {
			var empty []dto.CostTrendDataPoint
			for t := from; !t.After(to); t = t.AddDate(0, 0, 1) {
				empty = append(empty, dto.CostTrendDataPoint{Date: t.Format("2006-01-02"), TotalCost: 0})
			}
			return &dto.CostTrendResponse{Data: empty}, nil
		}
	}
	_ = envRequested

	ctxTrend, cancel := context.WithTimeout(ctx, CostTrendTimeout)
	defer cancel()
	rows, err := s.repo.ListCloudBillDailyRawFromTo(ctxTrend, from, to)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return nil, ErrFallbackTimeout
		}
		return nil, err
	}
	byDate := make(map[string]*dto.CostTrendDataPoint)
	for _, r := range rows {
		// 按环境过滤：若指定了 filterAccountID，只聚合匹配的行
		if filterAccountID != "" && r.AccountID != filterAccountID {
			continue
		}
		// [Ref: 16_ §三] 趋势展示实际付款（CashAmount）；允许负值代数参与
		d := r.BillDate.Format("2006-01-02")
		if byDate[d] == nil {
			byDate[d] = &dto.CostTrendDataPoint{Date: d, ByDomain: make(map[string]float64)}
		}
		byDate[d].TotalCost += r.CashTotalAmount
		for domain, cost := range r.CashProductBreakdown {
			if strings.Contains(domain, ":") || cost == 0 {
				continue
			}
			byDate[d].ByDomain[domain] += cost
		}
	}
	var data []dto.CostTrendDataPoint
	for t := from; !t.After(to); t = t.AddDate(0, 0, 1) {
		d := t.Format("2006-01-02")
		if p, ok := byDate[d]; ok {
			data = append(data, *p)
		} else {
			data = append(data, dto.CostTrendDataPoint{Date: d, TotalCost: 0, ByDomain: nil})
		}
	}
	return &dto.CostTrendResponse{Data: data}, nil
}
