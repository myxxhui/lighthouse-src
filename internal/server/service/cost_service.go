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
		// 本季度三个月账期
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
	case "1d", "7d", "30d", "month", "":
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
	resp := &dto.GlobalCostResponse{
		TotalCost:        total,
		TotalOptimizable: 0,
		GlobalEfficiency: 0,
		DomainBreakdown:  domainBreakdown,
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

// reportTypeAndPeriodKey 返回常规 period 对应的聚合表 (report_type, period_key)。[Ref: 04_01_成本透视真实数据 展示与延迟说明]
func reportTypeAndPeriodKey(period string, now time.Time) (reportType, periodKey string) {
	yesterday := now.AddDate(0, 0, -1)
	today := now.Format("2006-01-02")
	yesterdayStr := yesterday.Format("2006-01-02")
	month := now.Format("2006-01")
	q := (int(now.Month())-1)/3 + 1
	quarter := fmt.Sprintf("%s-Q%d", now.Format("2006"), q)
	switch period {
	case "1d":
		return "1d", yesterdayStr
	case "7d":
		return "7d", today
	case "30d", "":
		return "30d", today
	case "month":
		return "month", month
	case "quarter":
		return "quarter", quarter
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
	case "7d", "30d":
		t, _ := time.Parse("2006-01-02", periodKey)
		return t.AddDate(0, 0, -7).Format("2006-01-02") // 简化：7d 上一段为 7 天前
	case "month":
		t, _ := time.Parse("2006-01", periodKey)
		return t.AddDate(0, -1, 0).Format("2006-01")
	case "quarter":
		// 2026-Q1 -> 2025-Q4
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
		prev := prevByAccount[c.AccountID]
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
	// [Ref: 01_实践 展示与延迟说明] 常规展示仅读聚合表
	if reportType, periodKey := reportTypeAndPeriodKey(period, now); reportType != "" {
		if agg, _ := s.repo.GetCloudBillAggregate(ctx, reportType, periodKey); agg != nil && (agg.TotalAmount > 0 || len(agg.ProductBreakdown) > 0) {
			domainBreakdown := make([]dto.DomainBreakdownItem, 0)
			for domain, cost := range agg.ProductBreakdown {
				if strings.Contains(domain, ":") {
					continue
				}
				topProducts := topProductsForDomain(&agg.ProductBreakdown, domain, 4)
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
			meta := &dto.GlobalCostMetadata{DataStatus: "aggregate"}
			if agg.LastSuccessAt != nil {
				meta.LastUpdatedAt = agg.LastSuccessAt
			}
			return &dto.GlobalCostResponse{
				TotalCost:        agg.TotalAmount,
				TotalOptimizable: 0,
				GlobalEfficiency: 0,
				DomainBreakdown:  domainBreakdown,
				EnvBreakdown:     envBreakdown,
				Namespaces:       nil,
				Timestamp:        now,
				Metadata:         meta,
			}, nil
		}
		// 聚合表无数据时 30d 降级：从日原始表聚合最近 30 天，5s 超时
		if period == "30d" || period == "" {
			ctxFallback, cancel := context.WithTimeout(ctx, FallbackQueryTimeout)
			from30 := now.AddDate(0, 0, -30)
			rows, err := s.repo.ListCloudBillDailyRawFromTo(ctxFallback, from30, now)
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
				reportType, periodKey := "30d", now.Format("2006-01-02")
				prevKey := previousPeriodKey(reportType, periodKey, now)
				envBreakdown := s.buildEnvBreakdown(ctx, reportType, periodKey, prevKey)
				return &dto.GlobalCostResponse{
					TotalCost:        total,
					TotalOptimizable: 0,
					GlobalEfficiency: 0,
					DomainBreakdown:  domainBreakdown,
					EnvBreakdown:     envBreakdown,
					Namespaces:       nil,
					Timestamp:        now,
					Metadata:         &dto.GlobalCostMetadata{DataStatus: "fallback"},
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
			return &dto.GlobalCostResponse{
				TotalCost:        totalAmount,
				TotalOptimizable: 0,
				GlobalEfficiency: 0,
				DomainBreakdown:  domainBreakdown,
				EnvBreakdown:     envBreakdown,
				Namespaces:       nil,
				Timestamp:        now,
				Metadata:         &dto.GlobalCostMetadata{DataStatus: "fallback"},
			}, nil
		}
		return &dto.GlobalCostResponse{
			TotalCost:        totalAmount,
			TotalOptimizable: 0,
			GlobalEfficiency: 0,
			DomainBreakdown:  domainBreakdown,
			Namespaces:       nil,
			Timestamp:        now,
			Metadata:         &dto.GlobalCostMetadata{DataStatus: "fallback"},
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
	return &dto.GlobalCostResponse{
		TotalCost:        sumL1,
		TotalOptimizable: sumOptimizable,
		GlobalEfficiency: globalEff,
		DomainBreakdown:  domainBreakdown,
		Namespaces:       namespaces,
		Timestamp:        time.Now().UTC(),
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
