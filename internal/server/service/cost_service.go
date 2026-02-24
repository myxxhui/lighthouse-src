// Package service provides business logic services for the HTTP API.
package service

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/myxxhui/lighthouse-src/internal/data/postgres"
	"github.com/myxxhui/lighthouse-src/internal/server/dto"
	"github.com/myxxhui/lighthouse-src/pkg/costmodel"
)

// CostService provides cost-related business logic using Mock data and costmodel.
type CostService struct {
	repo postgres.Repository
}

// NewCostService creates a new CostService with the given repository.
func NewCostService(repo postgres.Repository) *CostService {
	return &CostService{repo: repo}
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

// GetGlobalCost returns L0 cost. 优先使用 cost_cloud_bill_summary（Phase4 01_ 云账单）；无云账单时回退到 L1 聚合。
// period: 1d|7d|30d|month|quarter；云账单按账期聚合：month/1d/7d/30d 用当月账期，quarter 用本季度三月汇总。
// [Ref: 03_01_成本透视真实数据] 真实数据不臆造可优化空间/效率，无数据支撑时返回 0，前端展示「—」。
func (s *CostService) GetGlobalCost(ctx context.Context, period string) (*dto.GlobalCostResponse, error) {
	cloud, totalAmount, productBreakdown := s.aggregateCloudBillByPeriod(ctx, period)
	if cloud && productBreakdown != nil {
		// 仅领域键（不含 ":"）参与 domain 汇总；含 ":" 的为 "domain:ProductCode" 产品级明细，用于 top_products [Ref: 01_成本透视真实数据]
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
		return &dto.GlobalCostResponse{
			TotalCost:        totalAmount,
			TotalOptimizable: 0,
			GlobalEfficiency: 0,
			DomainBreakdown:  domainBreakdown,
			Namespaces:       nil,
			Timestamp:        time.Now().UTC(),
		}, nil
	}

	// 回退：L1 聚合（Mock 或 02_ 数据）
	now := time.Now()
	start := now.AddDate(0, 0, -7)
	costs, err := s.repo.AggregateDailyNamespaceCosts(ctx, start, now)
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
