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
	resp, err := svc.GetGlobalCost(ctx)
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
