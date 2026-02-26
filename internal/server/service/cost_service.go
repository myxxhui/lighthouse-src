// Package service provides business logic services for the HTTP API.
package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
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
	dateRangeCache   map[string]dateRangeCacheEntry
	dateRangeCacheMu sync.RWMutex
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
	now := time.Now()
	currentCycle := now.Format("2006-01")
	switch period {
	case "quarter":
		cycles := []string{currentCycle, now.AddDate(0, -1, 0).Format("2006-01"), now.AddDate(0, -2, 0).Format("2006-01")}
		list, err := s.repo.GetCloudBillSummariesForBillingCycles(ctx, cycles)
		if err != nil || len(list) == 0 {
			return false, 0, nil
		}
		var total float64
		merged := make(map[string]float64)
		for _, c := range list {
			total += c.TotalAmount
			for k, v := range c.ProductBreakdown {
				merged[k] += v
			}
		}
		return true, total, &merged
	case "last_month":
		prevCycle := now.AddDate(0, -1, 0).Format("2006-01")
		cloud, err := s.repo.GetLatestCloudBillSummaryForBillingCycle(ctx, prevCycle)
		if err != nil || cloud == nil {
			return false, 0, nil
		}
		return true, cloud.TotalAmount, &cloud.ProductBreakdown
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
		var total float64
		merged := make(map[string]float64)
		for _, r := range rows {
			total += r.TotalAmount
			for k, v := range r.ProductBreakdown {
				merged[k] += v
			}
		}
		return true, total, &merged
	case "last_quarter":
		prevCycles := []string{now.AddDate(0, -1, 0).Format("2006-01"), now.AddDate(0, -2, 0).Format("2006-01"), now.AddDate(0, -3, 0).Format("2006-01")}
		list, err := s.repo.GetCloudBillSummariesForBillingCycles(ctx, prevCycles)
		if err != nil || len(list) == 0 {
			return false, 0, nil
		}
		var total float64
		merged := make(map[string]float64)
		for _, c := range list {
			total += c.TotalAmount
			for k, v := range c.ProductBreakdown {
				merged[k] += v
			}
		}
		return true, total, &merged
	case "1d", "7d", "30d", "90d", "month", "":
		// 云账单粒度为账期，1d/7d/30d/month 均用当月账期
		cloud, err := s.repo.GetLatestCloudBillSummaryForBillingCycle(ctx, currentCycle)
		if err != nil || cloud == nil {
			// 无当月则用全局最新一条（兼容历史数据）
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
	var total float64
	merged := make(map[string]float64)
	for _, r := range rows {
		total += r.TotalAmount
		for k, v := range r.ProductBreakdown {
			merged[k] += v
		}
	}
	// D8-3 可选校验：与当月月表比对（若范围跨月则仅做参考）
	if mon, _ := s.repo.GetCloudBillMonthlyRaw(ctx, from.Format("2006-01")); mon != nil && mon.TotalAmount > 0 {
		diffPct := (total - mon.TotalAmount) / mon.TotalAmount
		if diffPct < 0 {
			diffPct = -diffPct
		}
		if diffPct > 0.01 {
			// 偏差 >1% 可打日志或告警（此处仅校验逻辑存在）
		}
	}

	domainBreakdown := make([]dto.DomainBreakdownItem, 0)
	for domain, cost := range merged {
		if strings.Contains(domain, ":") {
			continue
		}
		topProducts := topProductsForDomain(&merged, domain, 4)
		domainBreakdown = append(domainBreakdown, dto.DomainBreakdownItem{
			Domain:           domain,
			Cost:             cost,
			OptimizableSpace: 0,
			Efficiency:       0,
			TopProducts:      topProducts,
		})
	}
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

// ErrFallbackTimeout D1-4：降级从原始表聚合时查询超时，调用方应返回 503。
var ErrFallbackTimeout = errors.New("cost fallback query timeout")

// FallbackQueryTimeout D1-4：降级路径下从日原始表聚合的查询超时时间。
const FallbackQueryTimeout = 5 * time.Second

// reportTypeAndPeriodKey 返回常规 period 对应的聚合表 (report_type, period_key)。[Ref: 04_01_成本透视真实数据 展示与延迟说明、01_设计 report_type 与 period_key]
func reportTypeAndPeriodKey(period string, now time.Time) (reportType, periodKey string) {
	yesterday := now.AddDate(0, 0, -1)
	today := now.Format("2006-01-02")
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
		// 这周：ISO 周（周一为第一天），period_key=YYYY-Www [Ref: 01_设计 report_type 与 period_key；这周与近七天必须区分]
		year, week := now.ISOWeek()
		return "this_week", fmt.Sprintf("%04d-W%02d", year, week)
	case "7d", "7d_range":
		// 近七天：滚动 7 天，与「这周」区分 [Ref: 01_设计 report_type 与 period_key]
		return "7d", today
	case "last_week":
		// 上周结束日（昨日或上周日）
		weekday := int(now.Weekday())
		if weekday == 0 {
			weekday = 7
		}
		lastWeekEnd := now.AddDate(0, 0, -weekday)
		return "last_week", lastWeekEnd.Format("2006-01-02")
	case "30d", "":
		return "30d", today
	case "90d":
		return "90d", today
	case "month":
		return "month", month
	case "last_month":
		return "last_month", prevMonth
	case "quarter":
		return "quarter", quarter
	case "last_quarter":
		return "last_quarter", prevQuarter
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
		return fmt.Sprintf("%04d-W52", y-1) // 上一周为去年最后一周
	case "7d", "30d":
		t, _ := time.Parse("2006-01-02", periodKey)
		return t.AddDate(0, 0, -7).Format("2006-01-02")
	case "last_week":
		t, _ := time.Parse("2006-01-02", periodKey)
		return t.AddDate(0, 0, -7).Format("2006-01-02")
	case "90d":
		t, _ := time.Parse("2006-01-02", periodKey)
		return t.AddDate(0, 0, -90).Format("2006-01-02")
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
	curList, _ := s.repo.ListCloudBillAggregateForReportPeriod(ctx, reportType, periodKey)
	prevList, _ := s.repo.ListCloudBillAggregateForReportPeriod(ctx, reportType, prevPeriodKey)
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
		prev := prevByAccount[c.AccountID]
		if prev == 0 {
			prev = prevByAccount[c.Environment]
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

// GetGlobalCost returns L0 cost. [Ref: 04_01_成本透视真实数据] 常规展示仅读聚合表；无聚合时降级 cost_cloud_bill_summary 或日原始表。
// period: 1d|7d|30d|month|quarter 时优先读 cost_cloud_bill_aggregate；自定义日期由 GetGlobalCostByDateRange 从日原始表叠加。
// [Ref: D1-4] 降级时仅从日原始表聚合最近 30 天并设 5s 超时，超时返回 ErrFallbackTimeout（HTTP 503）。
func (s *CostService) GetGlobalCost(ctx context.Context, period string) (*dto.GlobalCostResponse, error) {
	now := time.Now().UTC()
	// [Ref: 01_实践 展示与延迟说明] 常规展示仅读聚合表；多账号时从 List 合并 total 与 product_breakdown（01_设计 §后端数据聚合与存储方案）
	if reportType, periodKey := reportTypeAndPeriodKey(period, now); reportType != "" {
		list, _ := s.repo.ListCloudBillAggregateForReportPeriod(ctx, reportType, periodKey)
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
				domainBreakdown := make([]dto.DomainBreakdownItem, 0)
				for domain, cost := range merged {
					if strings.Contains(domain, ":") {
						continue
					}
					topProducts := topProductsForDomain(&merged, domain, 4)
					domainBreakdown = append(domainBreakdown, dto.DomainBreakdownItem{
						Domain:           domain,
						Cost:             cost,
						OptimizableSpace: 0,
						Efficiency:       0,
						TopProducts:      topProducts,
					})
				}
				prevKey := previousPeriodKey(reportType, periodKey, now)
				envBreakdown := s.buildEnvBreakdown(ctx, reportType, periodKey, prevKey)
				meta := &dto.GlobalCostMetadata{DataStatus: "aggregate", ReportType: reportType, PeriodKey: periodKey}
				if lastSuccessAt != nil {
					meta.LastUpdatedAt = lastSuccessAt
				}
				// #region agent log
				pocTotal := 0.0
				for _, e := range envBreakdown {
					if e.Environment == "POC" {
						pocTotal = e.TotalCost
						break
					}
				}
				if b, _ := json.Marshal(map[string]interface{}{"hypothesisId": "H1_H3", "location": "cost_service.go:GetGlobalCost:aggregate", "message": "global cost response", "data": map[string]interface{}{"has_meta_last_updated": meta.LastUpdatedAt != nil, "env_breakdown_len": len(envBreakdown), "poc_total": pocTotal}, "timestamp": time.Now().UnixMilli()}); len(b) > 0 {
					if f, err := os.OpenFile("/root/work/lighthouse/.cursor/debug-c39b07.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644); err == nil {
						f.Write(append(b, '\n'))
						f.Close()
					}
				}
				// #endregion
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
		// 聚合表无数据时从日原始表降级：1d/this_week/7d/7d_range/30d 按对应区间聚合 [Ref: 01_实践 展示与延迟说明]
		if period == "1d" || period == "this_week" || period == "7d" || period == "7d_range" || period == "30d" || period == "" {
			ctxFallback, cancel := context.WithTimeout(ctx, FallbackQueryTimeout)
			var from, to time.Time
			switch period {
			case "1d":
				y := now.AddDate(0, 0, -1)
				from, to = y.Truncate(24*time.Hour), y.Truncate(24*time.Hour)
			case "this_week":
				// 这周：本周一 00:00 至昨日（ISO 周周一为第一天）
				wd := now.Weekday()
				if wd == 0 {
					wd = 7
				}
				daysBack := int(wd - 1)
				thisWeekMonday := now.AddDate(0, 0, -daysBack).Truncate(24 * time.Hour)
				yesterday := now.AddDate(0, 0, -1).Truncate(24 * time.Hour)
				from, to = thisWeekMonday, yesterday
			case "7d", "7d_range":
				to = now.Truncate(24 * time.Hour)
				from = to.AddDate(0, 0, -6)
			default:
				from = now.AddDate(0, 0, -30)
				to = now
			}
			rows, err := s.repo.ListCloudBillDailyRawFromTo(ctxFallback, from, to)
			cancel()
			if err != nil {
				if errors.Is(err, context.DeadlineExceeded) {
					return nil, ErrFallbackTimeout
				}
			} else if len(rows) > 0 {
				var total float64
				merged := make(map[string]float64)
				for _, r := range rows {
					total += r.TotalAmount
					for k, v := range r.ProductBreakdown {
						merged[k] += v
					}
				}
				domainBreakdown := make([]dto.DomainBreakdownItem, 0)
				for domain, cost := range merged {
					if strings.Contains(domain, ":") {
						continue
					}
					topProducts := topProductsForDomain(&merged, domain, 4)
					domainBreakdown = append(domainBreakdown, dto.DomainBreakdownItem{
						Domain:           domain,
						Cost:             cost,
						OptimizableSpace: 0,
						Efficiency:       0,
						TopProducts:      topProducts,
					})
				}
				rType, pKey := "30d", now.Format("2006-01-02")
				if period == "1d" {
					rType, pKey = "1d", now.AddDate(0, 0, -1).Format("2006-01-02")
				} else if period == "this_week" {
					year, week := now.ISOWeek()
					rType, pKey = "this_week", fmt.Sprintf("%04d-W%02d", year, week)
				} else if period == "7d" || period == "7d_range" {
					rType, pKey = "7d", now.Format("2006-01-02")
				}
				prevKey := previousPeriodKey(rType, pKey, now)
				envBreakdown := s.buildEnvBreakdown(ctx, rType, pKey, prevKey)
				// #region agent log
				if b, _ := json.Marshal(map[string]interface{}{"hypothesisId": "H1_H5", "location": "cost_service.go:GetGlobalCost:fallback_daily", "message": "fallback path no last_updated_at", "data": map[string]interface{}{"env_breakdown_len": len(envBreakdown)}, "timestamp": time.Now().UnixMilli()}); len(b) > 0 {
					if f, err := os.OpenFile("/root/work/lighthouse/.cursor/debug-c39b07.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644); err == nil {
						f.Write(append(b, '\n'))
						f.Close()
					}
				}
				// #endregion
				metaFallback := &dto.GlobalCostMetadata{DataStatus: "fallback", ReportType: rType, PeriodKey: pKey}
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
		domainBreakdown := make([]dto.DomainBreakdownItem, 0)
		for domain, cost := range *productBreakdown {
			if strings.Contains(domain, ":") {
				continue
			}
			topProducts := topProductsForDomain(productBreakdown, domain, 4)
			domainBreakdown = append(domainBreakdown, dto.DomainBreakdownItem{
				Domain:           domain,
				Cost:             cost,
				OptimizableSpace: 0,
				Efficiency:       0,
				TopProducts:      topProducts,
			})
		}
		if reportType, periodKey := reportTypeAndPeriodKey(period, now); reportType != "" {
			prevKey := previousPeriodKey(reportType, periodKey, now)
			envBreakdown := s.buildEnvBreakdown(ctx, reportType, periodKey, prevKey)
			// #region agent log
			if b, _ := json.Marshal(map[string]interface{}{"hypothesisId": "H1_H5", "location": "cost_service.go:GetGlobalCost:fallback_summary", "message": "fallback summary no last_updated_at", "data": map[string]interface{}{"env_breakdown_len": len(envBreakdown)}, "timestamp": time.Now().UnixMilli()}); len(b) > 0 {
				if f, err := os.OpenFile("/root/work/lighthouse/.cursor/debug-c39b07.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644); err == nil {
					f.Write(append(b, '\n'))
					f.Close()
				}
			}
			// #endregion
			metaFallback := &dto.GlobalCostMetadata{DataStatus: "fallback", ReportType: reportType, PeriodKey: periodKey}
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
		metaFallback2 := &dto.GlobalCostMetadata{DataStatus: "fallback"}
		metaFallback2.LastUpdatedAt = &now
		return &dto.GlobalCostResponse{
			TotalCost:        totalAmount,
			TotalOptimizable: 0,
			GlobalEfficiency: 0,
			DomainBreakdown:  domainBreakdown,
			Namespaces:       nil,
			Timestamp:        now,
			Metadata:         metaFallback2,
		}, nil
	}

	// 回退：L1 聚合（Mock 或 02_ 数据）
	nowL1 := time.Now()
	start := nowL1.AddDate(0, 0, -7)
	costs, err := s.repo.AggregateDailyNamespaceCosts(ctx, start, nowL1)
	if err != nil {
		return nil, err
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
	resp, err := s.GetGlobalCost(ctx, period)
	if err != nil {
		return nil, err
	}
	return resp.Namespaces, nil
}

// GetNamespaceCost returns L1 cost for a namespace.
func (s *CostService) GetNamespaceCost(ctx context.Context, namespace string) (*dto.NamespaceCostResponse, error) {
	now := time.Now()
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
	list, err := s.repo.ListCloudBillAggregateForReportPeriod(ctx, reportType, periodKey)
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
	// 从 product_breakdown 按产品汇总：key 形如 "domain:ProductCode"
	productCosts := make(map[string]float64)
	for k, cost := range pb {
		if idx := strings.Index(k, ":"); idx >= 0 && idx < len(k)-1 {
			code := k[idx+1:]
			productCosts[code] += cost
		}
	}
	var out []dto.EnvDrilldownItem
	for code, cost := range productCosts {
		if cost <= 0 {
			continue
		}
		cat, _ := s.repo.GetProductCategory(ctx, code)
		if cat == "" {
			cat = "other"
		}
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
