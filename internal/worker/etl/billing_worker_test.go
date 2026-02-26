package etl

import (
	"context"
	"testing"
	"time"

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

// mockPipelineRepo 实现 CloudBillPipelineRepository，记录日原始与聚合写入以便 D4-2 等多页/落库校验。
type mockPipelineRepo struct {
	mockBillingRepo
	dailyRaw    postgres.CloudBillDailyRaw
	dailyRawCnt int
	monthlyRaw  *postgres.CloudBillMonthlyRaw
	aggregate   *postgres.CloudBillAggregate
}

func (m *mockPipelineRepo) SaveCloudBillDailyRaw(ctx context.Context, r postgres.CloudBillDailyRaw) error {
	m.dailyRaw = r
	m.dailyRawCnt = len(r.ProductBreakdown)
	return nil
}
func (m *mockPipelineRepo) GetCloudBillDailyRaw(ctx context.Context, billDate time.Time) (*postgres.CloudBillDailyRaw, error) {
	if m.dailyRaw.BillDate.IsZero() {
		return nil, nil
	}
	if m.dailyRaw.BillDate.Truncate(24*time.Hour).Equal(billDate.Truncate(24 * time.Hour)) {
		return &m.dailyRaw, nil
	}
	return nil, nil
}
func (m *mockPipelineRepo) DeleteCloudBillDailyRawForDate(ctx context.Context, billDate time.Time) error { return nil }
func (m *mockPipelineRepo) ListMissingCloudBillDailyDates(ctx context.Context, from, to time.Time) ([]time.Time, error) {
	return nil, nil
}
func (m *mockPipelineRepo) SaveCloudBillMonthlyRaw(ctx context.Context, r postgres.CloudBillMonthlyRaw) error {
	m.monthlyRaw = &r
	return nil
}
func (m *mockPipelineRepo) GetCloudBillMonthlyRaw(ctx context.Context, billingCycle string) (*postgres.CloudBillMonthlyRaw, error) {
	if m.monthlyRaw != nil && m.monthlyRaw.BillingCycle == billingCycle {
		return m.monthlyRaw, nil
	}
	return nil, nil
}
func (m *mockPipelineRepo) SaveCloudBillAggregate(ctx context.Context, a postgres.CloudBillAggregate) error {
	m.aggregate = &a
	return nil
}
func (m *mockPipelineRepo) GetCloudBillAggregate(ctx context.Context, reportType, periodKey string) (*postgres.CloudBillAggregate, error) {
	if m.aggregate != nil && m.aggregate.ReportType == reportType && m.aggregate.PeriodKey == periodKey {
		return m.aggregate, nil
	}
	return nil, nil
}
func (m *mockPipelineRepo) ListCloudBillDailyRawFromTo(ctx context.Context, from, to time.Time) ([]postgres.CloudBillDailyRaw, error) {
	if !m.dailyRaw.BillDate.IsZero() && !m.dailyRaw.BillDate.Before(from) && !m.dailyRaw.BillDate.After(to) {
		return []postgres.CloudBillDailyRaw{m.dailyRaw}, nil
	}
	return nil, nil
}
func (m *mockPipelineRepo) DeleteCloudBillAggregateExcept(ctx context.Context, reportType string, keepPeriodKeys []string) error {
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

// TestBillingWorker_RunPipeline_MultiPageDaily 覆盖「多页返回」mock 场景：模拟合并多页后的日数据落库，校验总金额与条数正确（D4-2）。
func TestBillingWorker_RunPipeline_MultiPageDaily(t *testing.T) {
	repo := &mockPipelineRepo{}
	// 模拟分页拉全后的合并结果：总 150，3 个 category + 5 个 product 项
	yesterday := time.Now().UTC().AddDate(0, 0, -1).Truncate(24 * time.Hour)
	fetcher := &mockBillingFetcher{
		resp: &cloudbilling.FetchAccountSummaryResponse{
			BillingCycle: yesterday.Format("2006-01-02"),
			TotalAmount:  150,
			Currency:     "CNY",
			ByCategory:   map[string]float64{"计算资源": 80, "存储": 50, "网络": 20},
			Items: []cloudbilling.BillItem{
				{ProductCode: "ECS", Amount: 50, Category: "计算资源"},
				{ProductCode: "ACK", Amount: 30, Category: "计算资源"},
				{ProductCode: "OSS", Amount: 50, Category: "存储"},
				{ProductCode: "CDN", Amount: 15, Category: "网络"},
				{ProductCode: "SLB", Amount: 5, Category: "网络"},
			},
		},
	}
	w := NewBillingWorker(fetcher, repo, time.Now().UTC().Format("2006-01"))
	ctx := context.Background()
	if err := w.RunPipeline(ctx); err != nil {
		t.Fatalf("RunPipeline: %v", err)
	}
	// 校验日原始落库：总金额与 product_breakdown 条数（category + category:product）
	if repo.dailyRaw.TotalAmount != 150 {
		t.Errorf("daily raw total: got %v, want 150", repo.dailyRaw.TotalAmount)
	}
	// merged = ByCategory (3) + Items as Category:ProductCode (5) => 至少 5 个 key（category 可能合并）
	if len(repo.dailyRaw.ProductBreakdown) < 5 {
		t.Errorf("daily raw product_breakdown keys: got %v, want >= 5 (multi-page merged)", len(repo.dailyRaw.ProductBreakdown))
	}
	// 聚合 step5 会写入 1d/7d/30d/month/quarter 等；mock 只保留最后一次，故任一有效 report_type 即可 [Ref: 04_01_成本透视真实数据]
	if repo.aggregate == nil || repo.aggregate.TotalAmount != 150 {
		t.Errorf("aggregate not saved or wrong total: %+v", repo.aggregate)
	}
	validReportTypes := map[string]bool{"1d": true, "7d": true, "30d": true, "month": true, "quarter": true, "90d": true, "last_week": true, "last_month": true, "last_quarter": true}
	if !validReportTypes[repo.aggregate.ReportType] {
		t.Errorf("aggregate report_type unexpected: %s", repo.aggregate.ReportType)
	}
}
