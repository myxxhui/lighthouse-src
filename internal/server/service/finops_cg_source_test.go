package service

import (
	"context"
	"testing"
	"time"

	"github.com/myxxhui/lighthouse-src/internal/data/postgres"
	"github.com/myxxhui/lighthouse-src/internal/server/dto"
)

// cgLedgerStub 覆盖 C/G 相关读路径，嵌入 Mock 满足 Repository。[Ref: 03_Phase6/01_FinOps FINOPS_CG_SOURCE]
type cgLedgerStub struct {
	*postgres.MockRepository
	nFact                    int64
	factC, factG             float64
	ossC, ossG               float64
	apiC, apiG               float64
	lastChannelForCG         string
	envConfigs               []postgres.EnvAccountConfig
}

func (c *cgLedgerStub) ListEnvAccountConfig(ctx context.Context) ([]postgres.EnvAccountConfig, error) {
	if c.envConfigs != nil {
		return c.envConfigs, nil
	}
	return c.MockRepository.ListEnvAccountConfig(ctx)
}

func (c *cgLedgerStub) CountFinOpsBillingFactsInDateRange(ctx context.Context, from, to time.Time, accountIDs []string) (int64, error) {
	return c.nFact, nil
}

func (c *cgLedgerStub) SumFinOpsFactPretaxCGByDateRange(ctx context.Context, from, to time.Time, accountIDs []string) (float64, float64, error) {
	return c.factC, c.factG, nil
}

func (c *cgLedgerStub) SumLineItemsPretaxCGByDateRangeWithChannel(ctx context.Context, from, to time.Time, accountIDs []string, channel string) (float64, float64, error) {
	c.lastChannelForCG = channel
	switch channel {
	case "oss_detail":
		return c.ossC, c.ossG, nil
	case "api_query_account_bill":
		return c.apiC, c.apiG, nil
	default:
		return 0, 0, nil
	}
}

func TestFillTechnicalLedgerCG_FactsUseFinOpsFactPath(t *testing.T) {
	t.Parallel()
	st := &cgLedgerStub{
		MockRepository: postgres.NewMockRepository(postgres.DefaultMockConfig()),
		nFact:          100,
		factC:          100,
		factG:          -10,
	}
	svc := NewCostService(st, "oss", nil)
	resp := &dto.GlobalCostResponse{}
	now := time.Date(2025, 3, 15, 0, 0, 0, 0, time.UTC)
	svc.fillTechnicalLedgerCG(context.Background(), resp, "month", "2025-01", now, []string{"acc1"}, nil)
	if resp.Ledger == nil || resp.Ledger.C == nil || resp.Ledger.G == nil {
		t.Fatal("expected ledger C/G")
	}
	if *resp.Ledger.C != 100 || *resp.Ledger.G != -10 {
		t.Fatalf("ledger C/G=%v,%v want 100,-10", *resp.Ledger.C, *resp.Ledger.G)
	}
}

func TestFillTechnicalLedgerCG_OSSSourceWithFactsUsesFact(t *testing.T) {
	t.Parallel()
	st := &cgLedgerStub{
		MockRepository: postgres.NewMockRepository(postgres.DefaultMockConfig()),
		nFact:          3,
		factC:          42,
		factG:          -7,
	}
	svc := NewCostService(st, "oss", nil)
	resp := &dto.GlobalCostResponse{}
	now := time.Date(2025, 3, 15, 0, 0, 0, 0, time.UTC)
	svc.fillTechnicalLedgerCG(context.Background(), resp, "month", "2025-01", now, []string{"acc1"}, nil)
	if resp.Ledger == nil || resp.Ledger.C == nil {
		t.Fatal("expected ledger C")
	}
	if *resp.Ledger.C != 42 || *resp.Ledger.G != -7 {
		t.Fatalf("ledger C/G=%v,%v want 42,-7", *resp.Ledger.C, *resp.Ledger.G)
	}
}

func TestFillTechnicalLedgerCG_OSSSourceNoFactsUsesOSSDetailOnly(t *testing.T) {
	t.Parallel()
	st := &cgLedgerStub{
		MockRepository: postgres.NewMockRepository(postgres.DefaultMockConfig()),
		nFact:          0,
		ossC:           5,
		ossG:           -1,
	}
	svc := NewCostService(st, "oss", nil)
	resp := &dto.GlobalCostResponse{}
	now := time.Date(2025, 3, 15, 0, 0, 0, 0, time.UTC)
	svc.fillTechnicalLedgerCG(context.Background(), resp, "month", "2025-01", now, []string{"acc1"}, nil)
	if resp.Ledger == nil || resp.Ledger.C == nil {
		t.Fatal("expected ledger C")
	}
	if *resp.Ledger.C != 5 || *resp.Ledger.G != -1 {
		t.Fatalf("ledger C/G=%v,%v want 5,-1", *resp.Ledger.C, *resp.Ledger.G)
	}
	if st.lastChannelForCG != "oss_detail" {
		t.Fatalf("channel=%q want oss_detail", st.lastChannelForCG)
	}
}

func TestFillTechnicalLedgerCG_MultiEnvOSSOnly(t *testing.T) {
	t.Parallel()
	st := &cgLedgerStub{
		MockRepository: postgres.NewMockRepository(postgres.DefaultMockConfig()),
		nFact:          0,
		ossC:           20,
		ossG:           -2,
		apiC:           20,
		apiG:           -2,
		envConfigs: []postgres.EnvAccountConfig{
			{Environment: "POC", AccountID: "POC"},
			{Environment: "UAT", AccountID: "UAT"},
		},
	}
	svc := NewCostService(st, "oss", map[string]string{"POC": "api"})
	resp := &dto.GlobalCostResponse{}
	now := time.Date(2025, 3, 15, 0, 0, 0, 0, time.UTC)
	svc.fillTechnicalLedgerCG(context.Background(), resp, "month", "2025-01", now, nil, nil)
	if resp.Ledger == nil || resp.Ledger.C == nil || resp.Ledger.G == nil {
		t.Fatal("expected ledger C/G")
	}
	// POC→api 走 api_query_account_bill，UAT→oss 走 oss_detail；桩同值 → 20+20, -2+-2 [Ref: 03_Phase6/01_FinOps]
	if *resp.Ledger.C != 40 || *resp.Ledger.G != -4 {
		t.Fatalf("ledger C/G=%v,%v want 40,-4", *resp.Ledger.C, *resp.Ledger.G)
	}
	if st.lastChannelForCG != "oss_detail" {
		t.Fatalf("last channel=%q want oss_detail (UAT second)", st.lastChannelForCG)
	}
}

func TestFillTechnicalLedgerCG_APISourceUsesAPIChannel(t *testing.T) {
	t.Parallel()
	st := &cgLedgerStub{
		MockRepository: postgres.NewMockRepository(postgres.DefaultMockConfig()),
		nFact:          0,
		apiC:           7,
		apiG:           -3,
	}
	svc := NewCostService(st, "api", nil)
	resp := &dto.GlobalCostResponse{}
	now := time.Date(2025, 3, 15, 0, 0, 0, 0, time.UTC)
	svc.fillTechnicalLedgerCG(context.Background(), resp, "month", "2025-01", now, []string{"acc1"}, nil)
	if resp.Ledger == nil || resp.Ledger.C == nil || resp.Ledger.G == nil {
		t.Fatal("expected ledger C/G")
	}
	if *resp.Ledger.C != 7 || *resp.Ledger.G != -3 {
		t.Fatalf("ledger C/G=%v,%v want 7,-3", *resp.Ledger.C, *resp.Ledger.G)
	}
	if st.lastChannelForCG != "api_query_account_bill" {
		t.Fatalf("channel=%q want api_query_account_bill", st.lastChannelForCG)
	}
}
