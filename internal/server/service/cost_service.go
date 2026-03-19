// Package service provides business logic services for the HTTP API.
package service

import (
	"context"
	"errors"
	"fmt"
	"math"
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
	rows, err := s.repo.ListCloudBillDailyRawFromTo(ctx, from30, now, "")
	if err == nil {
		out.DailyRawCountLast30 = len(rows)
		for _, r := range rows {
			out.DailyRawCashSumLast30 += r.CashTotalAmount
		}
	}
	for i := 1; i <= 3; i++ {
		cycle := now.AddDate(0, -i, 0).Format("2006-01")
		list, _ := s.repo.ListCloudBillMonthlyRawByCycle(ctx, cycle)
		if len(list) > 0 {
			out.MonthlyRawCountRecent++
		}
	}
	month := now.Format("2006-01")
	list, _ := s.repo.ListCloudBillAggregateForReportPeriod(ctx, "month", month, "consumption")
	out.AggregateMonthRows = len(list)
	for _, a := range list {
		out.AggregateMonthTotal += a.TotalAmount
	}
	// 简要提示（当前使用消耗口径 total_amount）[Ref: 用户确认 消耗口径]
	if out.DailyRawCountLast30 == 0 && out.MonthlyRawCountRecent == 0 {
		out.Hint = "日/月原始表均无数据：请确认 1) CLOUD_BILLING_PROVIDER=aliyun 且已配置 AK/SK；2) 后端启动后 ETL 已执行（查看日志 billing ETL pipeline）；3) 已执行 migrate-02/migrate-03 等迁移"
	} else if out.DailyRawCountLast30 > 0 && out.DailyRawCashSumLast30 == 0 {
		out.Hint = "当前使用消耗口径（total_amount）；日表 cash 为 0 不影响展示"
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
		topLevelCost := merged[domain]
		var productSum float64
		prefix := domain + ":"
		for k, v := range merged {
			if strings.HasPrefix(k, prefix) {
				productSum += v
			}
		}
		cost := topLevelCost
		if math.Abs(productSum) > math.Abs(topLevelCost) {
			cost = productSum
		}
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

// scaleMergedToTotal 将 merged 按 targetTotal 等比例缩放，使 sum(merged)=targetTotal，保证 total_cost 与 sum(products) 一致。[Ref: 用户确认 月表权威值+总成本=产品之和]
func scaleMergedToTotal(merged map[string]float64, targetTotal float64) {
	if len(merged) == 0 {
		return
	}
	var sum float64
	for _, v := range merged {
		sum += v
	}
	if math.Abs(sum) < 1e-9 {
		return
	}
	scale := targetTotal / sum
	for k := range merged {
		merged[k] *= scale
	}
}

// scaleDomainBreakdownToTotal 将 domain_breakdown 各 Cost 按 targetTotal 等比例缩放，使 sum(domain_breakdown[].Cost)=targetTotal。[Ref: 成本分解总和=总成本]
func scaleDomainBreakdownToTotal(domainBreakdown []dto.DomainBreakdownItem, targetTotal float64) {
	if len(domainBreakdown) == 0 {
		return
	}
	var sum float64
	for _, d := range domainBreakdown {
		sum += d.Cost
	}
	if math.Abs(sum) < 1e-9 {
		return
	}
	scale := targetTotal / sum
	for i := range domainBreakdown {
		domainBreakdown[i].Cost *= scale
	}
}

// scaleEnvBreakdownToTotal 将 env_breakdown 各 TotalCost 按 targetTotal 等比例缩放，使 sum(env_breakdown)=总环境成本。[Ref: 用户需求 环境成本与全环境总成本一致]
func scaleEnvBreakdownToTotal(envBreakdown []dto.EnvBreakdownItem, targetTotal float64) {
	if len(envBreakdown) == 0 {
		return
	}
	if targetTotal <= 0 {
		for i := range envBreakdown {
			envBreakdown[i].TotalCost = 0
		}
		return
	}
	var sum float64
	for i := range envBreakdown {
		if envBreakdown[i].TotalCost < 0 {
			envBreakdown[i].TotalCost = 0
		}
		sum += envBreakdown[i].TotalCost
	}
	if math.Abs(sum) < 1e-9 {
		// 聚合表无当期数据时 env 全 0，将总账归入第一项，保证 sum(env_breakdown)=总环境成本 [Ref: 用户需求]
		if targetTotal > 0 && len(envBreakdown) > 0 {
			envBreakdown[0].TotalCost = targetTotal
		}
		return
	}
	scale := targetTotal / sum
	for i := range envBreakdown {
		envBreakdown[i].TotalCost *= scale
	}
}

// scaleDrilldownListToTotal 将云产品明细列表按 targetTotal 等比例缩放，使 sum(items.Cost)=targetTotal，与总环境成本一致。[Ref: 用户需求 云产品成本明细叠加=总环境成本]
func scaleDrilldownListToTotal(items []dto.EnvDrilldownItem, targetTotal float64) {
	if len(items) == 0 {
		return
	}
	if targetTotal <= 0 {
		for i := range items {
			items[i].Cost = 0
		}
		return
	}
	var sum float64
	for i := range items {
		sum += items[i].Cost
	}
	if math.Abs(sum) < 1e-9 {
		return
	}
	scale := targetTotal / sum
	for i := range items {
		items[i].Cost *= scale
	}
}

// topProductsForDomain 从 productBreakdown 中取 key 为 "domain:ProductCode" 的项，按绝对值降序取前 n 个。
// 含冲正/退款月份可能全为负值，仍需展示以帮助用户了解成本构成。[Ref: 01_成本透视真实数据]
func topProductsForDomain(pb *map[string]float64, domain string, n int) []dto.ProductCostItem {
	if pb == nil || n <= 0 {
		return nil
	}
	prefix := domain + ":"
	var list []dto.ProductCostItem
	for k, cost := range *pb {
		if !strings.HasPrefix(k, prefix) || cost == 0 {
			continue
		}
		product := strings.TrimPrefix(k, prefix)
		list = append(list, dto.ProductCostItem{Product: product, Cost: cost})
	}
	if len(list) == 0 {
		return nil
	}
	sort.Slice(list, func(i, j int) bool {
		return math.Abs(list[i].Cost) > math.Abs(list[j].Cost)
	})
	if len(list) > n {
		list = list[:n]
	}
	return list
}

// avgDailyConsumptionLastMonth 上月天粒度消耗汇总平均数，用于回调日替代。[Ref: 用户需求 回调导致天汇总<0则按上月天粒度消耗汇总平均数代替]
func (s *CostService) avgDailyConsumptionLastMonth(ctx context.Context, now time.Time) float64 {
	lastMonth := now.AddDate(0, -1, 0)
	first := time.Date(lastMonth.Year(), lastMonth.Month(), 1, 0, 0, 0, 0, time.UTC)
	lastDay := first.AddDate(0, 1, -1)
	rows, err := s.repo.ListCloudBillDailyRawFromTo(ctx, first, lastDay, "")
	if err != nil || len(rows) == 0 {
		return 0
	}
	daysInMonth := lastDay.Day()
	if daysInMonth <= 0 {
		return 0
	}
	var sum float64
	for _, r := range rows {
		sum += r.TotalAmount
	}
	return sum / float64(daysInMonth)
}

// aggregateCloudBillByPeriod 按时间范围聚合云账单：总账单用 API 月汇总聚合叠加，天数据用天消耗汇总，回调日（天汇总<0）用上月天粒度消耗日均替代。[Ref: 用户需求]
// 返回 (有云账单, 总金额, 领域分项)；无数据时 (false, 0, nil)。
func (s *CostService) aggregateCloudBillByPeriod(ctx context.Context, period string) (bool, float64, *map[string]float64) {
	now := time.Now().UTC()
	switch period {
	case "quarter":
		// [Ref: 用户需求] 整月用 API 月汇总（现金）；当月未结用天消耗+回调日用上月日均替代
		curQ := (int(now.Month())-1)/3 + 1
		qStartMonth := (curQ-1)*3 + 1
		lastMonthAvg := s.avgDailyConsumptionLastMonth(ctx, now)
		var total float64
		merged := make(map[string]float64)
		for m := 0; m < 3; m++ {
			cycle := fmt.Sprintf("%04d-%02d", now.Year(), qStartMonth+m)
			if cycle == now.Format("2006-01") {
				firstOfMonth := time.Date(now.Year(), time.Month(qStartMonth+m), 1, 0, 0, 0, 0, time.UTC)
				yesterday := now.AddDate(0, 0, -1).Truncate(24 * time.Hour)
				rows, _ := s.repo.ListCloudBillDailyRawFromTo(ctx, firstOfMonth, yesterday, "")
				if len(rows) > 0 {
					t, pb := mergeDailyRowsWithCallbackReplacement(rows, lastMonthAvg)
					total += t
					for k, v := range pb {
						merged[k] += v
					}
				}
			} else if t, pb := s.mergeMonthlyRawByCycle(ctx, cycle); t != 0 || len(pb) > 0 {
				total += t
				for k, v := range pb {
					merged[k] += v
				}
			}
		}
		if total != 0 || len(merged) > 0 {
			return true, total, &merged
		}
		return false, 0, nil
	case "last_month":
		// [Ref: 用户需求] 上月=整月，只用月粒度现金/已付；多账户汇总 [Ref: 01_多环境 UAT]
		prevCycle := now.AddDate(0, -1, 0).Format("2006-01")
		t, pb := s.mergeMonthlyRawByCycle(ctx, prevCycle)
		if t == 0 && len(pb) == 0 {
			return false, 0, nil
		}
		return true, t, &pb
	case "last_quarter":
		// [Ref: 用户需求] 上季度=整月叠加，只用月粒度现金/已付，绝对准确
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
			t, pb := s.mergeMonthlyRawByCycle(ctx, cycle)
			total += t
			for k, v := range pb {
				merged[k] += v
			}
		}
		if total != 0 || len(merged) > 0 {
			return true, total, &merged
		}
		return false, 0, nil
	case "this_year":
		// [Ref: 用户需求] 已完成月用月粒度现金/已付；当月用天叠+消耗+回调日替代（支出参考）
		thisYear := now.Year()
		currentMonth := int(now.Month())
		var total float64
		merged := make(map[string]float64)
		for m := 1; m < currentMonth; m++ {
			cycle := fmt.Sprintf("%04d-%02d", thisYear, m)
			t, pb := s.mergeMonthlyRawByCycle(ctx, cycle)
			total += t
			for k, v := range pb {
				merged[k] += v
			}
		}
		yesterday := now.AddDate(0, 0, -1).Truncate(24 * time.Hour)
		firstOfCurrentMonth := time.Date(thisYear, time.Month(currentMonth), 1, 0, 0, 0, 0, time.UTC)
		lastMonthAvg := s.avgDailyConsumptionLastMonth(ctx, now)
		rows, _ := s.repo.ListCloudBillDailyRawFromTo(ctx, firstOfCurrentMonth, yesterday, "")
		if len(rows) > 0 {
			curTotal, curMerged := mergeDailyRowsSafe(rows, lastMonthAvg)
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
		// [Ref: 用户需求] 去年=整月叠加，只用月粒度现金/已付，绝对准确
		lastYear := now.Year() - 1
		var total float64
		merged := make(map[string]float64)
		for m := 1; m <= 12; m++ {
			cycle := fmt.Sprintf("%04d-%02d", lastYear, m)
			t, pb := s.mergeMonthlyRawByCycle(ctx, cycle)
			total += t
			for k, v := range pb {
				merged[k] += v
			}
		}
		if total == 0 && len(merged) == 0 {
			return false, 0, nil
		}
		return true, total, &merged
	case "month", "":
		// [Ref: 用户需求] 本月=仅天叠，天消耗+回调日用上月天粒度消耗日均替代
		yesterday := now.AddDate(0, 0, -1).Truncate(24 * time.Hour)
		firstOfMonth := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
		lastMonthAvg := s.avgDailyConsumptionLastMonth(ctx, now)
		if rows, err := s.repo.ListCloudBillDailyRawFromTo(ctx, firstOfMonth, yesterday, ""); err == nil && len(rows) > 0 {
			total, merged := mergeDailyRowsSafe(rows, lastMonthAvg)
			if total != 0 || len(merged) > 0 {
				return true, total, &merged
			}
		}
		return false, 0, nil
	default:
		return false, 0, nil
	}
}

// GetGlobalCostByDateRange 按月份范围从月原始表叠加，支持最多 5 年（60 个月）。[Ref: 01_实践 月源数据保留近5年]
// [Ref: D8-7] 相同日期范围结果短时缓存 1h，重复请求直接命中。
func (s *CostService) GetGlobalCostByDateRange(ctx context.Context, from, to time.Time) (*dto.GlobalCostResponse, error) {
	const maxMonths = 60
	if to.Before(from) {
		from, to = to, from
	}
	fromCycle := from.Format("2006-01")
	toCycle := to.Format("2006-01")
	key := fromCycle + ":" + toCycle
	now := time.Now().UTC()
	s.dateRangeCacheMu.RLock()
	if e, ok := s.dateRangeCache[key]; ok && e.exp.After(now) {
		s.dateRangeCacheMu.RUnlock()
		return e.resp, nil
	}
	s.dateRangeCacheMu.RUnlock()

	var total float64
	merged := make(map[string]float64)
	cur := time.Date(from.Year(), from.Month(), 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(to.Year(), to.Month(), 1, 0, 0, 0, 0, time.UTC)
	count := 0
	for !cur.After(end) && count < maxMonths {
		cycle := cur.Format("2006-01")
		t, pb := s.mergeMonthlyRawByCycle(ctx, cycle)
		total += t
		for k, v := range pb {
			merged[k] += v
		}
		cur = cur.AddDate(0, 1, 0)
		count++
	}
	if total == 0 && len(merged) == 0 {
		return nil, nil
	}
	scaleMergedToTotal(merged, total)
	domainBreakdown := buildDomainBreakdownNormalized(merged)
	scaleDomainBreakdownToTotal(domainBreakdown, total)
	// [Ref: 用户需求] 自定义月范围现金合计为负时展示为 0 并注明净退款已抵减
	displayTotal := total
	var meta *dto.GlobalCostMetadata
	if total < 0 {
		displayTotal = 0
		for i := range domainBreakdown {
			domainBreakdown[i].Cost = 0
		}
		meta = &dto.GlobalCostMetadata{DataStatus: "fallback", DisplayNote: "该周期净退款已抵减"}
	}
	resp := &dto.GlobalCostResponse{
		TotalCost:        displayTotal,
		TotalOptimizable: 0,
		GlobalEfficiency: 0,
		DomainBreakdown:  domainBreakdown,
		EnvBreakdown:     []dto.EnvBreakdownItem{},
		Namespaces:       nil,
		Timestamp:        now,
		Metadata:         meta,
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
	default:
		// quarter/this_year 等跨月周期：任一月仍为 PRELIMINARY 则整体 PRELIMINARY
		return "PRELIMINARY"
	}
}

// mergeDailyRowsWithCallbackReplacement 聚合日原始行（消耗口径）；某日 total_amount < 0 视为回调日。
// 若 lastMonthAvg > 0 则用上月天粒度消耗汇总平均数替代回调日，否则用当前周期内正常日日均替代。[Ref: 用户需求 回调导致天汇总<0则按上月天粒度消耗汇总平均数代替]
func mergeDailyRowsWithCallbackReplacement(rows []postgres.CloudBillDailyRaw, lastMonthAvg float64) (float64, map[string]float64) {
	if len(rows) == 0 {
		return 0, nil
	}
	var normalRows []postgres.CloudBillDailyRaw
	for _, r := range rows {
		if r.TotalAmount >= 0 {
			normalRows = append(normalRows, r)
		}
	}
	if len(normalRows) == 0 {
		return 0, make(map[string]float64)
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
	// 回调日替代：优先用上月天粒度消耗日均；否则用当前正常日日均
	replaceAvg := avgTotal
	if lastMonthAvg > 0 {
		replaceAvg = lastMonthAvg
	}
	total := sumTotal + replaceAvg*float64(nCallback)
	merged := make(map[string]float64)
	for _, r := range normalRows {
		for k, v := range r.ProductBreakdown {
			merged[k] += v
		}
	}
	// 回调日贡献按当前正常日分类比例分配
	if replaceAvg > 0 && math.Abs(avgTotal) > 1e-9 {
		ratio := replaceAvg / avgTotal
		for k, v := range avgPB {
			merged[k] += v * ratio * float64(nCallback)
		}
	} else {
		for k, v := range avgPB {
			merged[k] += v * float64(nCallback)
		}
	}
	return total, merged
}

// mergeDailyRowsSafe 聚合日原始行；已统一为消耗口径，含回调日替代（无上月日均时用当前正常日日均）。[Ref: 16_ §三；用户确认 消耗口径]
func mergeDailyRowsSafe(rows []postgres.CloudBillDailyRaw, lastMonthAvg float64) (float64, map[string]float64) {
	return mergeDailyRowsWithCallbackReplacement(rows, lastMonthAvg)
}

// pickMonthlyData 月表消耗口径（TotalAmount/ProductBreakdown），仅用于天叠部分的参考。[Ref: 天粒度可用消耗]
func pickMonthlyData(mon *postgres.CloudBillMonthlyRaw) (total float64, pb map[string]float64) {
	if mon == nil {
		return 0, nil
	}
	return mon.TotalAmount, mon.ProductBreakdown
}

// pickMonthlyDataCash 月表现金/已付口径（CashTotalAmount/CashProductBreakdown），用于整月及整月叠加的绝对准确数据。[Ref: 用户需求 月粒度只算现金支付或已付，不被回调恶心]
func pickMonthlyDataCash(mon *postgres.CloudBillMonthlyRaw) (total float64, pb map[string]float64) {
	if mon == nil {
		return 0, nil
	}
	total = mon.CashTotalAmount
	pb = mon.CashProductBreakdown
	if pb == nil {
		pb = make(map[string]float64)
	}
	return total, pb
}

// mergeMonthlyRawByCycle 汇总指定账期下所有 account 的月原始行（总金额与领域合并），供降级路径多环境总账。[Ref: 01_多环境 UAT]
func (s *CostService) mergeMonthlyRawByCycle(ctx context.Context, billingCycle string) (total float64, breakdown map[string]float64) {
	list, err := s.repo.ListCloudBillMonthlyRawByCycle(ctx, billingCycle)
	if err != nil || len(list) == 0 {
		return 0, nil
	}
	breakdown = make(map[string]float64)
	for i := range list {
		t, pb := pickMonthlyDataCash(&list[i])
		total += t
		for k, v := range pb {
			breakdown[k] += v
		}
	}
	return total, breakdown
}

// ErrFallbackTimeout D1-4：降级从原始表聚合时查询超时，调用方应返回 503。
var ErrFallbackTimeout = errors.New("cost fallback query timeout")

// FallbackQueryTimeout D1-4：降级路径下从日原始表聚合的查询超时时间。
const FallbackQueryTimeout = 5 * time.Second

// reportTypeAndPeriodKey 返回常规 period 对应的聚合表 (report_type, period_key)。[Ref: 04_01_成本透视真实数据 展示与延迟说明、01_设计 report_type 与 period_key]
func reportTypeAndPeriodKey(period string, now time.Time) (reportType, periodKey string) {
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
	curList, _ := s.repo.ListCloudBillAggregateForReportPeriod(ctx, reportType, periodKey, "consumption")
	prevList, _ := s.repo.ListCloudBillAggregateForReportPeriod(ctx, reportType, prevPeriodKey, "consumption")
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

// accountIDsForEnvs 返回所选环境对应的 account_id 集合（用于按环境过滤）。[Ref: 用户需求 环境多选]
func (s *CostService) accountIDsForEnvs(ctx context.Context, envs []string) map[string]bool {
	if len(envs) == 0 {
		return nil
	}
	configs, err := s.repo.ListEnvAccountConfig(ctx)
	if err != nil {
		return nil
	}
	envSet := make(map[string]bool)
	for _, e := range envs {
		if e != "" {
			envSet[e] = true
		}
	}
	ids := make(map[string]bool)
	for _, c := range configs {
		if envSet[c.Environment] {
			ids[c.AccountID] = true
			ids[c.Environment] = true // 单账号时 ETL 可能写 environment 名
		}
	}
	return ids
}

// normalizePeriod 规范化前端传入的 period，避免因大小写或别名导致 reportType 为空、env_breakdown 全 0。[Ref: 01_成本透视 环境卡片无数据]
func normalizePeriod(p string) string {
	p = strings.TrimSpace(p)
	switch strings.ToLower(p) {
	case "lastmonth", "last_month":
		return "last_month"
	case "lastquarter", "last_quarter":
		return "last_quarter"
	case "lastyear", "last_year":
		return "last_year"
	case "thisyear", "this_year":
		return "this_year"
	default:
		return p
	}
}

// GetGlobalCost returns L0 cost. [Ref: 04_01_成本透视真实数据、16_ §三]
// [Ref: 用户需求] 上月/上季度/去年/自定义月=月粒度现金/已付（绝对准确），不读聚合表；本月/这季度/今年=天叠可用消耗+回调日替代（支出参考），可读聚合表或降级。
// envs 非空时仅汇总所选环境的成本（仅聚合表路径支持；降级路径无按环境数据则忽略过滤）。metricType 已废弃。
func (s *CostService) GetGlobalCost(ctx context.Context, period string, metricType string, envs []string) (*dto.GlobalCostResponse, error) {
	period = normalizePeriod(period)
	now := time.Now().UTC()
	reportType, periodKey := reportTypeAndPeriodKey(period, now)
	// 整月/整月叠加范围：只用月表现金（或月表现金+当月天叠），不走聚合表。这季度与今年在 Q1 时间范围一致，需用同一套聚合逻辑（月表现金+当月天叠），避免数据不一致。
	useMonthCashOnly := reportType == "last_month" || reportType == "last_quarter" || reportType == "last_year" || reportType == "this_year" || reportType == "quarter"
	if useMonthCashOnly {
		reportType = ""
		periodKey = ""
	}
	// [Ref: 01_实践] 本月/这季度/今年可读聚合表（消耗）；上月/上季度/去年直接走 aggregateCloudBillByPeriod（月表现金）
	if reportType != "" && periodKey != "" {
		list, _ := s.repo.ListCloudBillAggregateForReportPeriod(ctx, reportType, periodKey, "consumption")
		if len(envs) > 0 {
			allowed := s.accountIDsForEnvs(ctx, envs)
			if len(allowed) > 0 {
				filtered := list[:0]
				for _, a := range list {
					if allowed[a.AccountID] {
						filtered = append(filtered, a)
					}
				}
				list = filtered
			}
		}
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
		if totalAmount != 0 || len(merged) > 0 {
			scaleMergedToTotal(merged, totalAmount)
			domainBreakdown := buildDomainBreakdownNormalized(merged)
			scaleDomainBreakdownToTotal(domainBreakdown, totalAmount)
			prevKey := previousPeriodKey(reportType, periodKey, now)
			envBreakdown := s.buildEnvBreakdown(ctx, reportType, periodKey, prevKey)
			// [Ref: 用户需求] 聚合表返回为负时同样展示为 0 并注明净退款已抵减（如今年、这季度）
			displayTotal := totalAmount
			displayNote := ""
			if totalAmount < 0 {
				displayTotal = 0
				displayNote = "该周期净退款已抵减"
				for i := range domainBreakdown {
					domainBreakdown[i].Cost = 0
				}
				for i := range envBreakdown {
					if envBreakdown[i].TotalCost < 0 {
						envBreakdown[i].TotalCost = 0
					}
				}
			}
			scaleEnvBreakdownToTotal(envBreakdown, displayTotal)
			meta := &dto.GlobalCostMetadata{DataStatus: "aggregate", ReportType: reportType, PeriodKey: periodKey, DisplayNote: displayNote}
			if lastSuccessAt != nil {
				meta.LastUpdatedAt = lastSuccessAt
			}
			// [Ref: 16_云账单动态对账与高可靠处理规范 §三段式] 注入账单对账状态供前端区分"已财务核算"/"动态同步中"
			meta.BillDataStatus = s.resolveBillDataStatus(ctx, reportType, periodKey, now)
			return &dto.GlobalCostResponse{
				TotalCost:        displayTotal,
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
	}
	// 聚合表无数据时降级：从 daily_raw/monthly_raw 读消耗口径（含回调日替代）[Ref: 01_实践；用户确认 消耗口径]
	cloud, totalAmountFB, productBreakdown := s.aggregateCloudBillByPeriod(ctx, period)
	if cloud && productBreakdown != nil {
		scaleMergedToTotal(*productBreakdown, totalAmountFB)
		domainBreakdown := buildDomainBreakdownNormalized(*productBreakdown)
		scaleDomainBreakdownToTotal(domainBreakdown, totalAmountFB)
		// [Ref: 用户需求] 月/季度/年周期现金合计为负时展示为 0 并注明净退款已抵减，避免界面出现负金额
		displayTotal := totalAmountFB
		displayNote := ""
		if totalAmountFB < 0 {
			displayTotal = 0
			displayNote = "该周期净退款已抵减"
			for i := range domainBreakdown {
				domainBreakdown[i].Cost = 0
			}
		}
		rType, pKey := reportTypeAndPeriodKey(period, now)
		if rType == "" {
			rType, pKey = reportTypeAndPeriodKey(strings.ToLower(period), now)
		}
		if rType != "" {
			prevKey := previousPeriodKey(rType, pKey, now)
			envBreakdown := s.buildEnvBreakdown(ctx, rType, pKey, prevKey)
			// [Ref: 01_成本透视] 降级读 summary 时聚合表常为 0，env_breakdown 全 0 导致前端“没数据”；将 summary 总账归入第一个已配置环境
			if displayTotal > 0 && len(envBreakdown) > 0 {
				var sum float64
				for i := range envBreakdown {
					sum += envBreakdown[i].TotalCost
				}
				if sum == 0 {
					for i := range envBreakdown {
						if envBreakdown[i].AccountDisplayName != "未配置" {
							envBreakdown[i].TotalCost = displayTotal
							break
						}
					}
					if len(envBreakdown) > 0 && envBreakdown[0].TotalCost == 0 {
						envBreakdown[0].TotalCost = displayTotal
					}
				}
			}
			if displayTotal == 0 && totalAmountFB < 0 {
				for i := range envBreakdown {
					if envBreakdown[i].TotalCost < 0 {
						envBreakdown[i].TotalCost = 0
					}
				}
			}
			var envSum float64
			for i := range envBreakdown {
				if envBreakdown[i].TotalCost < 0 {
					envBreakdown[i].TotalCost = 0
				}
				envSum += envBreakdown[i].TotalCost
			}
			// 多账户时以聚合表各环境之和为总成本，不缩放 env_breakdown，保证一环境一账户 [Ref: 用户需求 环境卡片对应各自账户]
			if envSum > 1e-9 {
				displayTotal = envSum
				scaleDomainBreakdownToTotal(domainBreakdown, displayTotal)
			} else {
				scaleEnvBreakdownToTotal(envBreakdown, displayTotal)
			}
			metaFallback := &dto.GlobalCostMetadata{DataStatus: "fallback", BillDataStatus: "PRELIMINARY", ReportType: rType, PeriodKey: pKey, DisplayNote: displayNote}
			metaFallback.LastUpdatedAt = &now
			return &dto.GlobalCostResponse{
				TotalCost:        displayTotal,
				TotalOptimizable: 0,
				GlobalEfficiency: 0,
				DomainBreakdown:  domainBreakdown,
				EnvBreakdown:     envBreakdown,
				Namespaces:       nil,
				Timestamp:        now,
				Metadata:         metaFallback,
			}, nil
		}
		// reportType 仍为空：用上月聚合键构建四环境槽位，将总账归入第一个已配置环境，避免环境卡片全 0 [Ref: 01_成本透视 环境卡片无数据]
		prevMonth := now.AddDate(0, -1, 0).Format("2006-01")
		prevPrevMonth := now.AddDate(0, -2, 0).Format("2006-01")
		envBreakdown := s.buildEnvBreakdown(ctx, "last_month", prevMonth, prevPrevMonth)
		var envSum2 float64
		for i := range envBreakdown {
			if envBreakdown[i].TotalCost < 0 {
				envBreakdown[i].TotalCost = 0
			}
			envSum2 += envBreakdown[i].TotalCost
		}
		if envSum2 > 1e-9 {
			displayTotal = envSum2
			scaleDomainBreakdownToTotal(domainBreakdown, displayTotal)
		} else if len(envBreakdown) > 0 && displayTotal > 0 {
			for i := range envBreakdown {
				if envBreakdown[i].AccountDisplayName != "未配置" {
					envBreakdown[i].TotalCost = displayTotal
					break
				}
			}
			if envBreakdown[0].TotalCost == 0 {
				envBreakdown[0].TotalCost = displayTotal
			}
			scaleEnvBreakdownToTotal(envBreakdown, displayTotal)
		}
		metaFallback2 := &dto.GlobalCostMetadata{DataStatus: "fallback", BillDataStatus: "PRELIMINARY", DisplayNote: displayNote}
		metaFallback2.LastUpdatedAt = &now
		return &dto.GlobalCostResponse{
			TotalCost:        displayTotal,
			TotalOptimizable: 0,
			GlobalEfficiency: 0,
			DomainBreakdown:  domainBreakdown,
			EnvBreakdown:     envBreakdown,
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
	scaleEnvBreakdownToTotal(envBreakdown, sumL1)
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
	resp, err := s.GetGlobalCost(ctx, period, "payment", nil)
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
	list, err := s.repo.ListCloudBillAggregateForReportPeriod(ctx, reportType, periodKey, "consumption")
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
		if math.Abs(cost) < 0.005 {
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
		sort.Slice(out, func(i, j int) bool { return math.Abs(out[i].Cost) > math.Abs(out[j].Cost) })
	} else {
		sort.Slice(out, func(i, j int) bool { return math.Abs(out[i].Cost) < math.Abs(out[j].Cost) })
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
	// [Ref: 01_实践] 本月/上月钻取降级：period_key 为 YYYY-MM，从月原始表按单月取数
	case "month", "last_month":
		t, err := time.Parse("2006-01", periodKey)
		if err != nil {
			return from, to, false
		}
		from = time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, time.UTC)
		lastDay := time.Date(t.Year(), t.Month()+1, 0, 0, 0, 0, 0, time.UTC)
		if lastDay.After(yesterday) {
			to = yesterday
		} else {
			to = lastDay
		}
		if from.After(to) {
			return from, to, false
		}
		return from, to, true
	// [Ref: 04_01_成本透视真实数据] 季度与年度环比降级：聚合表无历史记录时从月原始表计算
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
// 上月/上季度/去年/今年/这季度与 GetGlobalCost 同源（月表现金+天叠），使云产品明细叠加=总环境成本。[Ref: 用户需求 云产品成本明细与总环境成本一致]
func (s *CostService) GetGlobalDrilldown(ctx context.Context, reportType, periodKey, category, sortOrder, env string) ([]dto.EnvDrilldownItem, error) {
	useMonthCashReportTypes := map[string]bool{"last_month": true, "last_quarter": true, "last_year": true, "this_year": true, "quarter": true}
	if useMonthCashReportTypes[reportType] && (env == "" || env == "all") {
		cloud, total, pb := s.aggregateCloudBillByPeriod(ctx, reportType)
		if cloud && pb != nil && total > 0 {
			productCosts := *pb
			categoryFromPrefix := make(map[string]string)
			for k := range productCosts {
				if idx := strings.Index(k, ":"); idx >= 0 && idx < len(k)-1 {
					code := k[idx+1:]
					prefix := k[:idx]
					if cat := domainPrefixToCategory(prefix); cat != "" && categoryFromPrefix[code] == "" {
						categoryFromPrefix[code] = cat
					}
				}
			}
			var out []dto.EnvDrilldownItem
			for code, cost := range productCosts {
				if math.Abs(cost) < 0.005 {
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
			scaleDrilldownListToTotal(out, total)
			if sortOrder != "cost_asc" {
				sort.Slice(out, func(i, j int) bool { return math.Abs(out[i].Cost) > math.Abs(out[j].Cost) })
			} else {
				sort.Slice(out, func(i, j int) bool { return math.Abs(out[i].Cost) < math.Abs(out[j].Cost) })
			}
			return out, nil
		}
	}
	list, err := s.repo.ListCloudBillAggregateForReportPeriod(ctx, reportType, periodKey, "consumption")
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
	var accountIDs map[string]bool // env 为单个或逗号分隔（如 POC,FAT）时仅包含这些环境映射的 account_id [Ref: 用户需求 环境多选]
	if env != "" && env != "all" {
		envNames := strings.Split(env, ",")
		for i, e := range envNames {
			envNames[i] = strings.TrimSpace(e)
		}
		accountIDs = s.accountIDsForEnvs(ctx, envNames)
		if len(accountIDs) == 0 {
			return []dto.EnvDrilldownItem{}, nil
		}
	}
	var periodTotal float64
	productCosts := make(map[string]float64)
	categoryFromPrefix := make(map[string]string)
	for _, a := range list {
		if accountIDs != nil && !accountIDs[a.AccountID] {
			continue
		}
		periodTotal += a.TotalAmount
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
		if math.Abs(cost) < 0.005 {
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
	scaleDrilldownListToTotal(out, periodTotal)
	if sortOrder != "cost_asc" {
		sort.Slice(out, func(i, j int) bool { return math.Abs(out[i].Cost) > math.Abs(out[j].Cost) })
	} else {
		sort.Slice(out, func(i, j int) bool { return math.Abs(out[i].Cost) < math.Abs(out[j].Cost) })
	}
	return out, nil
}

// GetGlobalDrilldownByDateRange 全环境云产品明细（自定义日期）：从月原始表按 [from,to] 逐月聚合并打 category；支持最多 5 年（60 个月）。[Ref: 01_实践 月源数据保留近5年] 明细和缩放至同期总环境成本。
func (s *CostService) GetGlobalDrilldownByDateRange(ctx context.Context, from, to time.Time, category, sortOrder, env string) ([]dto.EnvDrilldownItem, error) {
	const maxMonths = 60
	if to.Before(from) {
		from, to = to, from
	}

	var periodTotal float64
	productCosts := make(map[string]float64)
	categoryFromPrefix := make(map[string]string)
	cur := time.Date(from.Year(), from.Month(), 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(to.Year(), to.Month(), 1, 0, 0, 0, 0, time.UTC)
	count := 0
	for !cur.After(end) && count < maxMonths {
		cycle := cur.Format("2006-01")
		t, pb := s.mergeMonthlyRawByCycle(ctx, cycle)
		periodTotal += t
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
		cur = cur.AddDate(0, 1, 0)
		count++
	}
	var out []dto.EnvDrilldownItem
	for code, cost := range productCosts {
		if math.Abs(cost) < 0.005 {
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
	scaleDrilldownListToTotal(out, periodTotal)
	if sortOrder != "cost_asc" {
		sort.Slice(out, func(i, j int) bool { return math.Abs(out[i].Cost) > math.Abs(out[j].Cost) })
	} else {
		sort.Slice(out, func(i, j int) bool { return math.Abs(out[i].Cost) < math.Abs(out[j].Cost) })
	}
	return out, nil
}

// CostTrendMaxDays 趋势 API 最大天数 [Ref: 12_API GET /api/v1/cost/trend]
const CostTrendMaxDays = 90

// CostTrendTimeout 趋势查询超时 [Ref: 01_设计 §成本趋势 API]
const CostTrendTimeout = 10 * time.Second

// GetCostTrend 成本结构趋势：按日/按月返回序列。[Ref: 01_设计 D9-9、12_API GET /api/v1/cost/trend]
// 月基时间范围（last_month/last_quarter/last_year/quarter/this_year）→ 按月数据点从 monthly_raw 读取。
// 日基时间范围（7d/30d/90d/custom）→ 按日数据点从 daily_raw 读取，最大 90 天、超时 10s。
// envFilter 非空且非"all"时按环境 account_id 过滤。
func (s *CostService) GetCostTrend(ctx context.Context, period string, dateFrom, dateTo *time.Time, envFilter string) (*dto.CostTrendResponse, error) {
	// 自定义月份范围优先 → 月粒度趋势
	if dateFrom != nil && dateTo != nil {
		fromY, fromM := dateFrom.Year(), int(dateFrom.Month())
		toY, toM := dateTo.Year(), int(dateTo.Month())
		return s.monthlyTrend(ctx, fromY, fromM, toY, toM)
	}
	// [Ref: 16_ §七] 单月趋势用日粒度（趋势图需多数据点），多月趋势用月粒度
	switch period {
	case "last_month":
		prev := time.Now().UTC().AddDate(0, -1, 0)
		from := time.Date(prev.Year(), prev.Month(), 1, 0, 0, 0, 0, time.UTC)
		to := from.AddDate(0, 1, -1)
		return s.dailyTrend(ctx, from, to, "")
	case "month", "":
		now := time.Now().UTC()
		from := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
		to := now.AddDate(0, 0, -1).Truncate(24 * time.Hour)
		if to.Before(from) {
			to = from
		}
		return s.dailyTrend(ctx, from, to, "")
	case "quarter":
		now := time.Now().UTC()
		q := (int(now.Month())-1)/3 + 1
		sm := (q-1)*3 + 1
		return s.monthlyTrend(ctx, now.Year(), sm, now.Year(), sm+2)
	case "last_quarter":
		now := time.Now().UTC()
		curMonth := int(now.Month())
		curYear := now.Year()
		var sm, sy int
		switch {
		case curMonth <= 3:
			sm, sy = 10, curYear-1
		case curMonth <= 6:
			sm, sy = 1, curYear
		case curMonth <= 9:
			sm, sy = 4, curYear
		default:
			sm, sy = 7, curYear
		}
		return s.monthlyTrend(ctx, sy, sm, sy, sm+2)
	case "this_year":
		y := time.Now().UTC().Year()
		return s.monthlyTrend(ctx, y, 1, y, int(time.Now().UTC().Month()))
	case "last_year":
		y := time.Now().UTC().Year() - 1
		return s.monthlyTrend(ctx, y, 1, y, 12)
	}

	// 其他情况默认本月日趋势
	now2 := time.Now().UTC()
	from := time.Date(now2.Year(), now2.Month(), 1, 0, 0, 0, 0, time.UTC)
	to := now2.AddDate(0, 0, -1).Truncate(24 * time.Hour)
	if to.Before(from) {
		to = from
	}

	// [Ref: 01_设计 §环境与云账号配置] 按环境过滤
	var filterAccountID string
	if envFilter != "" && envFilter != "all" {
		if configs, err := s.repo.ListEnvAccountConfig(ctx); err == nil {
			for _, cfg := range configs {
				if strings.EqualFold(cfg.Environment, envFilter) {
					filterAccountID = cfg.AccountID
					break
				}
			}
		}
		if filterAccountID == "" {
			var empty []dto.CostTrendDataPoint
			for t := from; !t.After(to); t = t.AddDate(0, 0, 1) {
				empty = append(empty, dto.CostTrendDataPoint{Date: t.Format("2006-01-02")})
			}
			return &dto.CostTrendResponse{Data: empty}, nil
		}
	}

	ctxTrend, cancel := context.WithTimeout(ctx, CostTrendTimeout)
	defer cancel()
	rows, err := s.repo.ListCloudBillDailyRawFromTo(ctxTrend, from, to, "")
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return nil, ErrFallbackTimeout
		}
		return nil, err
	}
	byDate := make(map[string]*dto.CostTrendDataPoint)
	for _, r := range rows {
		if filterAccountID != "" && r.AccountID != filterAccountID {
			continue
		}
		d := r.BillDate.Format("2006-01-02")
		if byDate[d] == nil {
			byDate[d] = &dto.CostTrendDataPoint{Date: d, ByDomain: make(map[string]float64), ByProduct: make(map[string]float64)}
		}
		// [Ref: 用户确认] 消耗口径：仅用 TotalAmount/ProductBreakdown
		amt := r.TotalAmount
		byDate[d].TotalCost += amt
		for k, cost := range r.ProductBreakdown {
			if cost == 0 {
				continue
			}
			if strings.Contains(k, ":") {
				code := k[strings.Index(k, ":")+1:]
				byDate[d].ByProduct[code] += cost
			} else {
				byDate[d].ByDomain[k] += cost
			}
		}
	}
	var data []dto.CostTrendDataPoint
	for t := from; !t.After(to); t = t.AddDate(0, 0, 1) {
		d := t.Format("2006-01-02")
		if p, ok := byDate[d]; ok {
			data = append(data, *p)
		} else {
			data = append(data, dto.CostTrendDataPoint{Date: d})
		}
	}
	return &dto.CostTrendResponse{Data: data}, nil
}

// dailyTrend 按日数据点构建趋势，用于单月时间范围（last_month/month）。[Ref: 16_ §七]
func (s *CostService) dailyTrend(ctx context.Context, from, to time.Time, envFilter string) (*dto.CostTrendResponse, error) {
	var filterAccountID string
	if envFilter != "" && envFilter != "all" {
		if configs, err := s.repo.ListEnvAccountConfig(ctx); err == nil {
			for _, cfg := range configs {
				if strings.EqualFold(cfg.Environment, envFilter) {
					filterAccountID = cfg.AccountID
					break
				}
			}
		}
		if filterAccountID == "" {
			var empty []dto.CostTrendDataPoint
			for t := from; !t.After(to); t = t.AddDate(0, 0, 1) {
				empty = append(empty, dto.CostTrendDataPoint{Date: t.Format("2006-01-02")})
			}
			return &dto.CostTrendResponse{Data: empty}, nil
		}
	}
	ctxTrend, cancel := context.WithTimeout(ctx, CostTrendTimeout)
	defer cancel()
	rows, err := s.repo.ListCloudBillDailyRawFromTo(ctxTrend, from, to, "")
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return nil, ErrFallbackTimeout
		}
		return nil, err
	}
	byDate := make(map[string]*dto.CostTrendDataPoint)
	for _, r := range rows {
		if filterAccountID != "" && r.AccountID != filterAccountID {
			continue
		}
		d := r.BillDate.Format("2006-01-02")
		if byDate[d] == nil {
			byDate[d] = &dto.CostTrendDataPoint{Date: d, ByDomain: make(map[string]float64), ByProduct: make(map[string]float64)}
		}
		amt := r.TotalAmount
		byDate[d].TotalCost += amt
		for k, cost := range r.ProductBreakdown {
			if cost == 0 {
				continue
			}
			if strings.Contains(k, ":") {
				code := k[strings.Index(k, ":")+1:]
				byDate[d].ByProduct[code] += cost
			} else {
				byDate[d].ByDomain[k] += cost
			}
		}
	}
	var data []dto.CostTrendDataPoint
	for t := from; !t.After(to); t = t.AddDate(0, 0, 1) {
		d := t.Format("2006-01-02")
		if p, ok := byDate[d]; ok {
			data = append(data, *p)
		} else {
			data = append(data, dto.CostTrendDataPoint{Date: d})
		}
	}
	return &dto.CostTrendResponse{Data: data}, nil
}

// monthlyTrend 按月数据点构建趋势。[Ref: 16_ §七 步骤⑨]
func (s *CostService) monthlyTrend(ctx context.Context, startYear, startMonth, endYear, endMonth int) (*dto.CostTrendResponse, error) {
	var data []dto.CostTrendDataPoint
	y, m := startYear, startMonth
	for {
		cycle := fmt.Sprintf("%04d-%02d", y, m)
		pt := dto.CostTrendDataPoint{Date: cycle, ByDomain: make(map[string]float64), ByProduct: make(map[string]float64)}
		t, pb := s.mergeMonthlyRawByCycle(ctx, cycle)
		pt.TotalCost = t
		for k, v := range pb {
			if v == 0 {
				continue
			}
			if strings.Contains(k, ":") {
				code := k[strings.Index(k, ":")+1:]
				pt.ByProduct[code] += v
			} else {
				pt.ByDomain[k] = v
			}
		}
		data = append(data, pt)
		if y > endYear || (y == endYear && m >= endMonth) {
			break
		}
		m++
		if m > 12 {
			m = 1
			y++
		}
	}
	return &dto.CostTrendResponse{Data: data}, nil
}
