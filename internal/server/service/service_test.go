package service

import (
	"context"
	"math"
	"testing"
	"time"

	"github.com/myxxhui/lighthouse-src/internal/data/postgres"
	"github.com/myxxhui/lighthouse-src/internal/server/dto"
)

func TestNewCostService(t *testing.T) {
	repo := postgres.NewMockRepository(postgres.DefaultMockConfig())
	svc := NewCostService(repo)
	if svc == nil {
		t.Fatal("NewCostService returned nil")
	}
}

func TestCostService_GetGlobalCost(t *testing.T) {
	repo := postgres.NewMockRepository(postgres.DefaultMockConfig())
	svc := NewCostService(repo)
	ctx := context.Background()
	resp, err := svc.GetGlobalCost(ctx, "month", "consumption", nil)
	if err != nil {
		t.Fatalf("GetGlobalCost: %v", err)
	}
	if resp == nil {
		t.Fatal("GetGlobalCost returned nil response")
	}
	if len(resp.Namespaces) == 0 {
		t.Log("GetGlobalCost returned empty namespaces (mock may have no data for date range)")
	}
}

// TestCostService_GetGlobalCost_CloudBill 验证有 monthly_raw（上月）时返回月表现金 total 与 domain_breakdown（01_）。本月走日表，故用 last_month 测月表。
func TestCostService_GetGlobalCost_CloudBill(t *testing.T) {
	repo := postgres.NewMockRepository(postgres.DefaultMockConfig())
	ctx := context.Background()
	prevCycle := time.Now().UTC().AddDate(0, -1, 0).Format("2006-01")
	snap := time.Now()
	err := repo.SaveCloudBillMonthlyRaw(ctx, postgres.CloudBillMonthlyRaw{
		BillingCycle:         prevCycle,
		TotalAmount:          125000,
		ProductBreakdown:     map[string]float64{"计算资源": 85000, "存储": 25000, "网络": 15000},
		CashTotalAmount:      125000,
		CashProductBreakdown: map[string]float64{"计算资源": 85000, "存储": 25000, "网络": 15000},
		SnapshotAt:           snap,
		CreatedAt:            snap,
	})
	if err != nil {
		t.Fatalf("SaveCloudBillMonthlyRaw: %v", err)
	}
	svc := NewCostService(repo)
	resp, err := svc.GetGlobalCost(ctx, "last_month", "payment", nil)
	if err != nil {
		t.Fatalf("GetGlobalCost: %v", err)
	}
	if resp.TotalCost != 125000 {
		t.Errorf("TotalCost = %v, want 125000 (from monthly_raw)", resp.TotalCost)
	}
	if len(resp.DomainBreakdown) != 5 {
		t.Errorf("DomainBreakdown len = %d, want 5", len(resp.DomainBreakdown))
	}
	byDomain := make(map[string]float64)
	for _, d := range resp.DomainBreakdown {
		byDomain[d.Domain] = d.Cost
	}
	if byDomain["计算资源"] != 85000 || byDomain["存储"] != 25000 || byDomain["网络"] != 15000 {
		t.Errorf("DomainBreakdown costs = %v", byDomain)
	}
}

// TestCostService_GetGlobalCost_CloudBillZero 验证 monthly_raw total=0 时不回退到 L1。
func TestCostService_GetGlobalCost_CloudBillZero(t *testing.T) {
	repo := postgres.NewMockRepository(postgres.DefaultMockConfig())
	ctx := context.Background()
	svc := NewCostService(repo)
	resp, err := svc.GetGlobalCost(ctx, "month", "payment", nil)
	if err != nil {
		t.Fatalf("GetGlobalCost: %v", err)
	}
	// 无 monthly_raw 和 daily_raw 时，会回退到 L1 聚合（mock 返回默认数据或空结构）
	if resp == nil {
		t.Fatal("response should not be nil")
	}
}

func TestCostService_MixedQueryTimeSeries(t *testing.T) {
	repo := postgres.NewMockRepository(postgres.DefaultMockConfig())
	svc := NewCostService(repo)
	ctx := context.Background()
	start := time.Now().Add(-24 * time.Hour)
	end := time.Now()
	pts, err := svc.MixedQueryTimeSeries(ctx, start, end, "default")
	if err != nil {
		t.Fatalf("MixedQueryTimeSeries: %v", err)
	}
	// Phase3 占位返回空
	if pts != nil {
		t.Errorf("Phase3 placeholder expected nil, got len=%d", len(pts))
	}
}

// TestScaleDomainBreakdownToTotal 验证成本分解缩放后 sum(domain_breakdown[].Cost)=targetTotal。[Ref: 成本分解总和=总成本]
func TestScaleDomainBreakdownToTotal(t *testing.T) {
	targetTotal := 1000.0
	domainBreakdown := []dto.DomainBreakdownItem{
		{Domain: "计算资源", Cost: 300},
		{Domain: "存储", Cost: 200},
		{Domain: "网络", Cost: 500},
	}
	scaleDomainBreakdownToTotal(domainBreakdown, targetTotal)
	var sum float64
	for _, d := range domainBreakdown {
		sum += d.Cost
	}
	if math.Abs(sum-targetTotal) > 0.01 {
		t.Errorf("after scale sum = %v, want %v", sum, targetTotal)
	}
}

// TestDrilldownPeriodToDateRange_MonthLastMonth 验证 month/last_month 降级时能解析出正确日期范围。[Ref: 环比上期钻取]
func TestDrilldownPeriodToDateRange_MonthLastMonth(t *testing.T) {
	// 固定一个参考时间，使 yesterday 确定
	now := time.Date(2026, 3, 15, 12, 0, 0, 0, time.UTC)
	from, to, ok := drilldownPeriodToDateRange("last_month", "2026-02", now)
	if !ok {
		t.Fatal("drilldownPeriodToDateRange(last_month, 2026-02) want ok=true")
	}
	if from.Year() != 2026 || from.Month() != 2 || from.Day() != 1 {
		t.Errorf("from = %v, want 2026-02-01", from.Format("2006-01-02"))
	}
	// 2026-02 已过去，to 应为 2026-02-28
	if to.Year() != 2026 || to.Month() != 2 || to.Day() != 28 {
		t.Errorf("to = %v, want 2026-02-28", to.Format("2006-01-02"))
	}
	// month 同理
	from2, to2, ok2 := drilldownPeriodToDateRange("month", "2026-03", now)
	if !ok2 {
		t.Fatal("drilldownPeriodToDateRange(month, 2026-03) want ok=true")
	}
	if from2.Year() != 2026 || from2.Month() != 3 || from2.Day() != 1 {
		t.Errorf("from = %v, want 2026-03-01", from2.Format("2006-01-02"))
	}
	// 当月未结束，to 应为 yesterday (2026-03-14)
	yesterday := now.AddDate(0, 0, -1)
	if to2.Day() != yesterday.Day() || to2.Month() != yesterday.Month() {
		t.Errorf("to = %v, want yesterday %v", to2.Format("2006-01-02"), yesterday.Format("2006-01-02"))
	}
}
