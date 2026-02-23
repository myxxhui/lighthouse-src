package cloudbilling

import (
	"context"
	"testing"
)

func TestCloudBillingFetcherInterface(t *testing.T) {
	_ = (*CloudBillingFetcher)(nil)
	cfg := CloudBillingConfig{Provider: ""}
	f := NewFetcher(cfg)
	if f != nil {
		t.Fatal("expected nil when Provider is empty")
	}
	cfg.Provider = "aliyun"
	f = NewFetcher(cfg)
	// 无 AK/SK 时返回 nil；有环境变量时返回非 nil（不在此断言，避免依赖环境）
	_ = f
}

func TestFetchAccountSummaryRequestResponse(t *testing.T) {
	req := FetchAccountSummaryRequest{
		BillingCycle: "2025-01",
		PeriodType:   "month",
	}
	if req.BillingCycle != "2025-01" {
		t.Errorf("BillingCycle want 2025-01, got %s", req.BillingCycle)
	}
	resp := &FetchAccountSummaryResponse{
		TotalAmount: 1000,
		Currency:    "CNY",
		ByCategory:  map[string]float64{"compute": 600},
	}
	_ = resp
	_ = context.Background()
}
