package service

import (
	"context"
	"testing"
	"time"

	"github.com/myxxhui/lighthouse-src/internal/data/postgres"
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
	resp, err := svc.GetGlobalCost(ctx, "month", "consumption")
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

// TestCostService_GetGlobalCost_CloudBill 验证有 cost_cloud_bill_summary 时优先返回云账单 total 与 domain_breakdown（01_）。
func TestCostService_GetGlobalCost_CloudBill(t *testing.T) {
	repo := postgres.NewMockRepository(postgres.DefaultMockConfig())
	ctx := context.Background()
	day := time.Now().UTC().Truncate(24 * time.Hour)
	err := repo.SaveCloudBillSummary(ctx, postgres.CloudBillSummary{
		Day:          day,
		BillingCycle: "2025-01",
		TotalAmount:  125000,
		ProductBreakdown: map[string]float64{
			"计算资源": 85000,
			"存储":   25000,
			"网络":   15000,
		},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	})
	if err != nil {
		t.Fatalf("SaveCloudBillSummary: %v", err)
	}
	svc := NewCostService(repo)
	resp, err := svc.GetGlobalCost(ctx, "month", "consumption")
	if err != nil {
		t.Fatalf("GetGlobalCost: %v", err)
	}
	if resp.TotalCost != 125000 {
		t.Errorf("TotalCost = %v, want 125000 (from cloud bill)", resp.TotalCost)
	}
	// [Ref: 01_设计 §成本分解] domain_breakdown 固定五类：计算资源、存储、网络、安全、其他
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
	if byDomain["安全"] != 0 || byDomain["其他"] != 0 {
		t.Errorf("DomainBreakdown 安全/其他 should be 0 when absent: %v", byDomain)
	}
}

// TestCostService_GetGlobalCost_CloudBillZero 验证有云账单行但 total=0 时仍采用云账单来源（不回退 L1）。
func TestCostService_GetGlobalCost_CloudBillZero(t *testing.T) {
	repo := postgres.NewMockRepository(postgres.DefaultMockConfig())
	ctx := context.Background()
	day := time.Now().UTC().Truncate(24 * time.Hour)
	err := repo.SaveCloudBillSummary(ctx, postgres.CloudBillSummary{
		Day:              day,
		BillingCycle:     "2025-02",
		TotalAmount:      0,
		ProductBreakdown: map[string]float64{},
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
	})
	if err != nil {
		t.Fatalf("SaveCloudBillSummary: %v", err)
	}
	svc := NewCostService(repo)
	resp, err := svc.GetGlobalCost(ctx, "month", "consumption")
	if err != nil {
		t.Fatalf("GetGlobalCost: %v", err)
	}
	if resp.TotalCost != 0 {
		t.Errorf("TotalCost = %v, want 0 (from cloud bill)", resp.TotalCost)
	}
	// [Ref: 01_设计 §成本分解] 无数据时仍返回五类（安全、其他为 0）
	if len(resp.DomainBreakdown) != 5 {
		t.Errorf("DomainBreakdown len = %d, want 5", len(resp.DomainBreakdown))
	}
	for _, d := range resp.DomainBreakdown {
		if d.Cost != 0 {
			t.Errorf("DomainBreakdown %s cost = %v, want 0", d.Domain, d.Cost)
		}
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
