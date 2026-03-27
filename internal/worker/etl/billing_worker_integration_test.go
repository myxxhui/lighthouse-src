//go:build integration
// +build integration

// [Ref: 04_Phase4/01_成本透视真实数据 T4.2] 集成测试：Mock Fetcher + 真实 PG，验证 ETL 落库后 GetGlobalCost 与 Mock 数据一致。
// 需 Docker。运行：go test -tags=integration -v ./internal/worker/etl/... -run Integration

package etl

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/myxxhui/lighthouse-src/internal/data/cloudbilling"
	"github.com/myxxhui/lighthouse-src/internal/data/postgres"
	"github.com/myxxhui/lighthouse-src/internal/server/service"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
)

func TestBillingWorker_Integration_RealPG_ETLThenGetGlobalCost(t *testing.T) {
	ctx := context.Background()
	initScript, err := filepath.Abs(filepath.Join("testdata", "init_pg_integration.sql"))
	if err != nil {
		t.Skipf("resolve init script: %v", err)
	}
	ctr, err := tcpostgres.Run(ctx,
		"postgres:15-alpine",
		tcpostgres.WithDatabase("lighthouse"),
		tcpostgres.WithUsername("lighthouse"),
		tcpostgres.WithPassword("lighthouse"),
		tcpostgres.WithInitScripts(initScript),
		tcpostgres.BasicWaitStrategies(),
	)
	if err != nil {
		t.Skipf("start postgres container: %v (need Docker)", err)
	}
	defer func() {
		_ = ctr.Terminate(ctx)
	}()

	connStr, err := ctr.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("connection string: %v", err)
	}
	pgRepo, err := postgres.NewPGRepositoryFromDSN(connStr, 0, 0)
	if err != nil {
		t.Fatalf("NewPGRepositoryFromDSN: %v", err)
	}
	defer func() { _ = pgRepo.Close() }()
	var repo postgres.Repository = pgRepo

	// Mock Fetcher 返回固定数据
	fixedTotal := 88888.0
	fetcher := &mockBillingFetcher{
		resp: &cloudbilling.FetchAccountSummaryResponse{
			BillingCycle: "2025-01",
			TotalAmount:  fixedTotal,
			Currency:     "CNY",
			ByCategory:   map[string]float64{"计算资源": 50000, "存储": 25000, "网络": 13888},
		},
	}
	worker := NewBillingWorker(fetcher, repo, "2025-01")
	if err := worker.Run(ctx); err != nil {
		t.Fatalf("BillingWorker.Run: %v", err)
	}

	// CostService 读同一 repo，应得到刚落库的数据
	costSvc := service.NewCostService(repo, "", nil)
	resp, err := costSvc.GetGlobalCost(ctx, "month", "payment", nil, "")
	if err != nil {
		t.Fatalf("GetGlobalCost: %v", err)
	}
	if resp.TotalCost != fixedTotal {
		t.Errorf("TotalCost = %v, want %v", resp.TotalCost, fixedTotal)
	}
	if len(resp.DomainBreakdown) != 3 {
		t.Errorf("DomainBreakdown len = %v, want 3", len(resp.DomainBreakdown))
	}
	byDomain := make(map[string]float64)
	for _, d := range resp.DomainBreakdown {
		byDomain[d.Domain] = d.Cost
	}
	if byDomain["计算资源"] != 50000 || byDomain["存储"] != 25000 || byDomain["网络"] != 13888 {
		t.Errorf("DomainBreakdown = %+v", resp.DomainBreakdown)
	}
}
