package cloudbilling

import (
	"context"
	"testing"
)

func TestCloudBillingFetcherInterface(t *testing.T) {
	// 占位包：确保 interface 与 factory 可编译，Phase4 实现真实拉取
	_ = (*CloudBillingFetcher)(nil)
	cfg := CloudBillingConfig{Provider: ""}
	f := NewFetcher(cfg)
	if f != nil {
		t.Fatal("expected nil when Provider is empty")
	}
	cfg.Provider = "aliyun"
	f = NewFetcher(cfg)
	// Phase3 占位返回 nil
	if f != nil {
		t.Fatal("Phase3 placeholder: expected nil for aliyun")
	}
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
