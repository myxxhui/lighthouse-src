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
	dailyRaw     postgres.CloudBillDailyRaw
	dailyRawCnt  int
	monthlyRaw   *postgres.CloudBillMonthlyRaw
	aggregate    *postgres.CloudBillAggregate // 最后一次写入的聚合（兼容单次断言）
	aggregates   []postgres.CloudBillAggregate // 全部写入的聚合（D9-6 多周期/对比会多次写入）
}

func (m *mockPipelineRepo) SaveCloudBillDailyRaw(ctx context.Context, r postgres.CloudBillDailyRaw) error {
	m.dailyRaw = r
	m.dailyRawCnt = len(r.ProductBreakdown)
	return nil
}
func (m *mockPipelineRepo) GetCloudBillDailyRaw(ctx context.Context, billDate time.Time, accountID string) (*postgres.CloudBillDailyRaw, error) {
	if m.dailyRaw.BillDate.IsZero() {
		return nil, nil
	}
	if m.dailyRaw.BillDate.Truncate(24*time.Hour).Equal(billDate.Truncate(24 * time.Hour)) {
		return &m.dailyRaw, nil
	}
	return nil, nil
}
func (m *mockPipelineRepo) DeleteCloudBillDailyRawForDate(ctx context.Context, billDate time.Time, accountID string) error {
	return nil
}
func (m *mockPipelineRepo) ListMissingCloudBillDailyDates(ctx context.Context, from, to time.Time, accountID string) ([]time.Time, error) {
	return nil, nil
}
func (m *mockPipelineRepo) SaveCloudBillMonthlyRaw(ctx context.Context, r postgres.CloudBillMonthlyRaw) error {
	m.monthlyRaw = &r
	return nil
}
func (m *mockPipelineRepo) GetCloudBillMonthlyRaw(ctx context.Context, billingCycle, accountID string) (*postgres.CloudBillMonthlyRaw, error) {
	if m.monthlyRaw != nil && m.monthlyRaw.BillingCycle == billingCycle {
		return m.monthlyRaw, nil
	}
	return nil, nil
}
func (m *mockPipelineRepo) DeleteCloudBillMonthlyRawOlderThan(ctx context.Context, cutoffBillingCycle string, accountID string) error {
	return nil
}
func (m *mockPipelineRepo) SaveCloudBillAggregate(ctx context.Context, a postgres.CloudBillAggregate) error {
	m.aggregate = &a
	m.aggregates = append(m.aggregates, a)
	return nil
}
func (m *mockPipelineRepo) GetCloudBillAggregate(ctx context.Context, reportType, periodKey string) (*postgres.CloudBillAggregate, error) {
	if m.aggregate != nil && m.aggregate.ReportType == reportType && m.aggregate.PeriodKey == periodKey {
		return m.aggregate, nil
	}
	return nil, nil
}
func (m *mockPipelineRepo) ListCloudBillDailyRawFromTo(ctx context.Context, from, to time.Time, accountID string) ([]postgres.CloudBillDailyRaw, error) {
	if !m.dailyRaw.BillDate.IsZero() && !m.dailyRaw.BillDate.Before(from) && !m.dailyRaw.BillDate.After(to) {
		return []postgres.CloudBillDailyRaw{m.dailyRaw}, nil
	}
	return nil, nil
}
func (m *mockPipelineRepo) DeleteCloudBillAggregateExcept(ctx context.Context, reportType string, keepPeriodKeys []string, accountID string) error {
	return nil
}

// [Ref: 16_云账单动态对账与高可靠处理规范] Mock stubs for new interface methods
func (m *mockPipelineRepo) UpsertCloudBillLineItem(ctx context.Context, item postgres.CloudBillLineItem) error {
	return nil
}
func (m *mockPipelineRepo) ListCloudBillLineItemsByDate(ctx context.Context, billDate time.Time, accountID string) ([]postgres.CloudBillLineItem, error) {
	return nil, nil
}
func (m *mockPipelineRepo) ListCloudBillLineItemsByBillingCycle(ctx context.Context, billingCycle, accountID string) ([]postgres.CloudBillLineItem, error) {
	return nil, nil
}
func (m *mockPipelineRepo) ListDistinctBillingCyclesInDateRange(ctx context.Context, from, to time.Time, accountID string) ([]string, error) {
	return nil, nil
}
func (m *mockPipelineRepo) SumLineItemsCashByBillingCycle(ctx context.Context, billingCycle, accountID string) (float64, error) {
	return 0, nil
}
func (m *mockPipelineRepo) GetProductCategory(ctx context.Context, productCode string) (string, bool) {
	return "other", true
}
func (m *mockPipelineRepo) UpsertProductCategory(ctx context.Context, productCode, category string) error {
	return nil
}
func (m *mockPipelineRepo) DeleteLineItemsOlderThan(ctx context.Context, before time.Time, accountID string) error {
	return nil
}
func (m *mockPipelineRepo) UpsertCloudBillMonthStatus(ctx context.Context, s postgres.CloudBillMonthStatus) error {
	return nil
}
func (m *mockPipelineRepo) GetCloudBillMonthStatus(ctx context.Context, billingCycle, accountID string) (*postgres.CloudBillMonthStatus, error) {
	return nil, nil
}

func (m *mockPipelineRepo) UpsertBSSTransaction(ctx context.Context, tx postgres.BSSTransactionRow) error {
	return nil
}
func (m *mockPipelineRepo) UpsertBSSBalanceSnapshot(ctx context.Context, s postgres.BSSBalanceSnapshotRow) error {
	return nil
}
func (m *mockPipelineRepo) UpsertBillOutstandingMonthly(ctx context.Context, o postgres.BillOutstandingMonthlyRow) error {
	return nil
}
func (m *mockPipelineRepo) RefreshBSSRechargeMonthlyForAccount(ctx context.Context, accountID string) error {
	return nil
}

func (m *mockPipelineRepo) DeleteFinOpsBillingFactsByBillingCycle(ctx context.Context, billingCycle, accountID string) error {
	return nil
}
func (m *mockPipelineRepo) BulkInsertFinOpsBillingFacts(ctx context.Context, rows []postgres.FinOpsBillingFactRow) error {
	return nil
}
func (m *mockPipelineRepo) ReplaceFinOpsBillingCycleWithFacts(ctx context.Context, billingCycle, accountID string, rows []postgres.FinOpsBillingFactRow) error {
	return nil
}
func (m *mockPipelineRepo) GetFinOpsOSSSyncCheckpoint(ctx context.Context, accountID string) (time.Time, bool, error) {
	return time.Time{}, false, nil
}
func (m *mockPipelineRepo) SetFinOpsOSSSyncCheckpoint(ctx context.Context, accountID string, maxObjectLastModified time.Time) error {
	return nil
}

func (m *mockPipelineRepo) UpdateEnvAccountConfigAccountID(ctx context.Context, environment, aliyunAccountID string) error {
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
	// 月请求时返回请求的 BillingCycle，以便落库的 monthly_raw 主键与 runAggregateStep 查询一致
	out := *m.resp
	if req.PeriodType == "month" && req.BillingCycle != "" {
		out.BillingCycle = req.BillingCycle
	}
	return &out, nil
}

// FetchLineItems 返回空列表（测试中不需要行级流水；真实 ETL 有降级处理）。
func (m *mockBillingFetcher) FetchLineItems(ctx context.Context, req cloudbilling.FetchLineItemsRequest) (*cloudbilling.FetchLineItemsResponse, error) {
	if m.err != nil {
		return nil, m.err
	}
	return &cloudbilling.FetchLineItemsResponse{
		BillingDate:  req.BillingDate,
		BillingCycle: func() string { if len(req.BillingDate) >= 7 { return req.BillingDate[:7] }; return req.BillingDate }(),
		Items:        nil,
	}, nil
}

func (m *mockBillingFetcher) FetchBSSTransactions(ctx context.Context, start, end time.Time) ([]cloudbilling.BSSTransactionItem, error) {
	if m.err != nil {
		return nil, m.err
	}
	return nil, nil
}

func (m *mockBillingFetcher) FetchAccountBalanceSnapshot(ctx context.Context) (float64, string, error) {
	if m.err != nil {
		return 0, "", m.err
	}
	return 0, "CNY", nil
}

func (m *mockBillingFetcher) FetchOutstandingMonthly(ctx context.Context, billingCycle string) (float64, error) {
	if m.err != nil {
		return 0, m.err
	}
	return 0, nil
}

func (m *mockBillingFetcher) FetchCallingAccountID(ctx context.Context, billingCycle string) (string, error) {
	if m.err != nil {
		return "", m.err
	}
	return "", nil
}

func (m *mockBillingFetcher) FetchCouponDeductionMonthly(ctx context.Context, billingCycle string) (float64, float64, error) {
	if m.err != nil {
		return 0, 0, m.err
	}
	return 0, 0, nil
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
	// 固定「当前」为月中 UTC，避免真实运行日在每月 1 日时 firstOfMonth > yesterday 导致当月日区间为空、聚合全为 0。
	fixedNow := time.Date(2026, 3, 15, 12, 0, 0, 0, time.UTC)
	// 模拟分页拉全后的合并结果：总 150，3 个 category + 5 个 product 项
	yesterday := fixedNow.AddDate(0, 0, -1).Truncate(24 * time.Hour)
	fetcher := &mockBillingFetcher{
		resp: &cloudbilling.FetchAccountSummaryResponse{
			BillingCycle:    yesterday.Format("2006-01-02"),
			TotalAmount:     150,
			CashTotalAmount: 150, // [Ref: 16_ §三] 聚合表仅 payment，mock 需提供 Cash 以便校验
			Currency:        "CNY",
			ByCategory:      map[string]float64{"计算资源": 80, "存储": 50, "网络": 20},
			CashByCategory:  map[string]float64{"计算资源": 80, "存储": 50, "网络": 20},
			Items: []cloudbilling.BillItem{
				{ProductCode: "ECS", Amount: 50, Category: "计算资源"},
				{ProductCode: "ACK", Amount: 30, Category: "计算资源"},
				{ProductCode: "OSS", Amount: 50, Category: "存储"},
				{ProductCode: "CDN", Amount: 15, Category: "网络"},
				{ProductCode: "SLB", Amount: 5, Category: "网络"},
			},
		},
	}
	w := NewBillingWorker(fetcher, repo, fixedNow.Format("2006-01"))
	w.NowFunc = func() time.Time { return fixedNow }
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
	// 聚合 step5 会写入 1d/7d/30d/90d/month/quarter 及对比周期；至少有一条 total=150、report_type 有效 [Ref: 04_01_成本透视真实数据、D9-6]
	var found150 bool
	validReportTypes := map[string]bool{"1d": true, "7d": true, "30d": true, "month": true, "quarter": true, "90d": true, "last_week": true, "last_month": true, "last_quarter": true, "this_year": true, "last_year": true}
	for i := range repo.aggregates {
		a := &repo.aggregates[i]
		if a.TotalAmount == 150 && validReportTypes[a.ReportType] {
			found150 = true
			break
		}
	}
	if !found150 {
		t.Errorf("aggregate not saved or wrong total: no row with TotalAmount=150 in %d saves; last: %+v", len(repo.aggregates), repo.aggregate)
	}
}
