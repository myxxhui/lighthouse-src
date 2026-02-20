// Package service provides business logic services for the HTTP API.
package service

import (
	"context"
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

// GetGlobalCost returns L0 aggregated cost using L1 (namespace) data from Mock.
// L0 is computed from L1 by costmodel.AggregateGlobal; no direct Prometheus query.
func (s *CostService) GetGlobalCost(ctx context.Context) (*dto.GlobalCostResponse, error) {
	now := time.Now()
	start := now.AddDate(0, 0, -7)

	costs, err := s.repo.AggregateDailyNamespaceCosts(ctx, start, now)
	if err != nil {
		return nil, err
	}

	// Convert to costmodel format for L0 aggregation
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

	// L0 total = sum of L1 (100% data consistency)
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
func (s *CostService) ListNamespaces(ctx context.Context) ([]dto.NamespaceCostSummary, error) {
	resp, err := s.GetGlobalCost(ctx)
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
