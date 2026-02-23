package etl

import (
	"context"
	"testing"

	"github.com/myxxhui/lighthouse-src/internal/data/cloudbilling"
	"github.com/myxxhui/lighthouse-src/internal/data/postgres"
)

// mockBillingRepo 用于测试的云账单仓库，记录最后一次保存的汇总。
type mockBillingRepo struct {
	last postgres.CloudBillSummary
}

func (m *mockBillingRepo) SaveCloudBillSummary(ctx context.Context, s postgres.CloudBillSummary) error {
	m.last = s
	return nil
}

// mockBillingFetcher 返回固定数据的拉取器。
type mockBillingFetcher struct {
	resp *cloudbilling.FetchAccountSummaryResponse
	err  error
}

func (m *mockBillingFetcher) FetchAccountSummary(ctx context.Context, req cloudbilling.FetchAccountSummaryRequest) (*cloudbilling.FetchAccountSummaryResponse, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.resp, nil
}

func TestBillingWorker_Run_NilFetcher(t *testing.T) {
	repo := &mockBillingRepo{}
	w := NewBillingWorker(nil, repo, "2025-01")
	ctx := context.Background()
	if err := w.Run(ctx); err != nil {
		t.Fatalf("Run with nil fetcher should succeed: %v", err)
	}
}

func TestBillingWorker_Run_SaveAndReconcile(t *testing.T) {
	repo := &mockBillingRepo{}
	fetcher := &mockBillingFetcher{
		resp: &cloudbilling.FetchAccountSummaryResponse{
			BillingCycle: "2025-01",
			TotalAmount:  1000,
			Currency:     "CNY",
			ByCategory:   map[string]float64{"compute": 600},
		},
	}
	w := NewBillingWorker(fetcher, repo, "2025-01")
	ctx := context.Background()
	if err := w.Run(ctx); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if repo.last.TotalAmount != 1000 {
		t.Errorf("SaveCloudBillSummary: total = %v, want 1000", repo.last.TotalAmount)
	}
}

func TestBillingWorker_Run_ReconcileAlertWhenOverThreshold(t *testing.T) {
	repo := &mockBillingRepo{}
	// 实际 1000，预期 900 → 约 11% 偏差 > 1%
	fetcher := &mockBillingFetcher{
		resp: &cloudbilling.FetchAccountSummaryResponse{
			BillingCycle: "2025-01",
			TotalAmount:  1000,
			Currency:     "CNY",
			ByCategory:   map[string]float64{},
		},
	}
	w := NewBillingWorker(fetcher, repo, "2025-01")
	w.ExpectedTotal = 900
	var alerted bool
	w.OnReconcileAlert = func(actual, expected float64, diffPct float64) {
		alerted = true
		if actual != 1000 || expected != 900 {
			t.Errorf("OnReconcileAlert: actual=%v expected=%v", actual, expected)
		}
		if diffPct < 0.1 {
			t.Errorf("diffPct should be > 0.1, got %v", diffPct)
		}
	}
	ctx := context.Background()
	if err := w.Run(ctx); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !alerted {
		t.Error("OnReconcileAlert should have been called when diff > 1%")
	}
}

func TestBillingWorker_Run_NoAlertWhenWithinThreshold(t *testing.T) {
	repo := &mockBillingRepo{}
	// 实际 1000，预期 995 → 约 0.5% 偏差 < 1%
	fetcher := &mockBillingFetcher{
		resp: &cloudbilling.FetchAccountSummaryResponse{
			BillingCycle: "2025-01",
			TotalAmount:  1000,
			Currency:     "CNY",
			ByCategory:   map[string]float64{},
		},
	}
	w := NewBillingWorker(fetcher, repo, "2025-01")
	w.ExpectedTotal = 995
	var alerted bool
	w.OnReconcileAlert = func(_, _ float64, _ float64) { alerted = true }
	ctx := context.Background()
	if err := w.Run(ctx); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if alerted {
		t.Error("OnReconcileAlert should not be called when diff <= 1%")
	}
}
