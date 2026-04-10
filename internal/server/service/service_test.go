package service

import (
	"context"
	"math"
	"reflect"
	"testing"
	"time"

	"github.com/myxxhui/lighthouse-src/internal/data/postgres"
	"github.com/myxxhui/lighthouse-src/internal/server/dto"
)

func TestNewCostService(t *testing.T) {
	repo := postgres.NewMockRepository(postgres.DefaultMockConfig())
	svc := NewCostService(repo, "", nil)
	if svc == nil {
		t.Fatal("NewCostService returned nil")
	}
}

func TestFinanceAccountIDsForPUBFromConfigs(t *testing.T) {
	t.Parallel()
	allPh := []postgres.EnvAccountConfig{
		{Environment: "POC", AccountID: "POC"},
		{Environment: "UAT", AccountID: "UAT"},
	}
	if got := financeAccountIDsForPUBFromConfigs(allPh, nil); got != nil {
		t.Fatalf("all placeholders want nil, got %#v", got)
	}
	allReal := []postgres.EnvAccountConfig{
		{Environment: "POC", AccountID: "1234567890123456"},
		{Environment: "UAT", AccountID: "9876543210987654"},
	}
	got := financeAccountIDsForPUBFromConfigs(allReal, nil)
	want := []string{"1234567890123456", "9876543210987654"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v want %#v", got, want)
	}
	mixed := []postgres.EnvAccountConfig{
		{Environment: "POC", AccountID: "POC"},
		{Environment: "UAT", AccountID: "999"},
	}
	if g := financeAccountIDsForPUBFromConfigs(mixed, nil); g != nil {
		t.Fatalf("mixed want nil, got %#v", g)
	}
}

func TestAggregateHeroTotalsFromDedupedList_duplicatePlaceholderAndRealID(t *testing.T) {
	t.Parallel()
	cfg := []postgres.EnvAccountConfig{
		{Environment: "POC", AccountID: "1657988574642393"},
		{Environment: "UAT", AccountID: "UAT"},
	}
	rows := []postgres.CloudBillAggregate{
		{AccountID: "POC", TotalAmount: 100, ProductBreakdown: map[string]float64{"计算资源": 100}},
		{AccountID: "1657988574642393", TotalAmount: 100, ProductBreakdown: map[string]float64{"计算资源": 100}},
		{AccountID: "UAT", TotalAmount: 50, ProductBreakdown: map[string]float64{"存储": 50}},
	}
	total, merged, err := aggregateHeroTotalsFromDedupedList(rows, cfg)
	if err != nil {
		t.Fatal(err)
	}
	// POC 仅计 165 一行（与 buildEnvBreakdownFromList 一致），不 100+100
	if math.Abs(total-150) > 1e-6 {
		t.Fatalf("total %v want 150", total)
	}
	if math.Abs(merged["计算资源"]-100) > 1e-6 || math.Abs(merged["存储"]-50) > 1e-6 {
		t.Fatalf("merged %#v", merged)
	}
}

func TestSumPerEnvFromAccountMap_duplicatePlaceholderAndRealID(t *testing.T) {
	t.Parallel()
	cfg := []postgres.EnvAccountConfig{
		{Environment: "POC", AccountID: "5823052810429629"},
		{Environment: "UAT", AccountID: "UAT"},
	}
	cur := map[string]float64{
		"POC":              3542.85,
		"5823052810429629": 3542.85,
		"UAT":              539.33,
		"1657988574642393": 539.33,
	}
	got := sumPerEnvFromAccountMap(cur, cfg)
	want := 3542.85 + 539.33
	if math.Abs(got-want) > 1e-2 {
		t.Fatalf("sumPerEnvFromAccountMap got %v want %v", got, want)
	}
}

func TestMatchEnvAccountConfig(t *testing.T) {
	t.Parallel()
	cfgF := []postgres.EnvAccountConfig{
		{Environment: "POC", AccountID: "5823052810429629"},
		{Environment: "UAT", AccountID: "1657988574642393"},
	}
	if c := matchEnvAccountConfig(cfgF, dto.EnvBreakdownItem{Environment: "POC", AccountID: "5823052810429629"}); c == nil || c.AccountID != "5823052810429629" {
		t.Fatalf("POC exact match: got %#v", c)
	}
	if c := matchEnvAccountConfig(cfgF, dto.EnvBreakdownItem{Environment: "UAT", AccountID: ""}); c == nil || c.Environment != "UAT" {
		t.Fatalf("UAT single row empty account_id: got %#v", c)
	}
	if c := matchEnvAccountConfig(cfgF, dto.EnvBreakdownItem{Environment: "POC", AccountID: "wrong"}); c != nil {
		t.Fatalf("mismatch account_id want nil, got %#v", c)
	}
}

func TestCostService_GetGlobalCost(t *testing.T) {
	repo := postgres.NewMockRepository(postgres.DefaultMockConfig())
	svc := NewCostService(repo, "", nil)
	ctx := context.Background()
	resp, err := svc.GetGlobalCost(ctx, "month", "consumption", nil, nil, "")
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

// TestCostService_GetGlobalCost_CloudBill 验证有 cost_cloud_bill_aggregate（上月 payment）时返回 total 与 domain_breakdown。[Ref: 聚合表主路径]
func TestCostService_GetGlobalCost_CloudBill(t *testing.T) {
	repo := postgres.NewMockRepository(postgres.DefaultMockConfig())
	ctx := context.Background()
	_, prevCycle := reportTypeAndPeriodKey("last_month", time.Now().UTC())
	snap := time.Now()
	err := repo.SaveCloudBillAggregate(ctx, postgres.CloudBillAggregate{
		ReportType:       "last_month",
		PeriodKey:        prevCycle,
		MetricType:       "payment",
		TotalAmount:      125000,
		ProductBreakdown: map[string]float64{"计算资源": 85000, "存储": 25000, "网络": 15000},
		AccountID:        "default",
		LastSuccessAt:    &snap,
		CreatedAt:        snap,
		UpdatedAt:        snap,
	})
	if err != nil {
		t.Fatalf("SaveCloudBillAggregate: %v", err)
	}
	svc := NewCostService(repo, "", nil)
	resp, err := svc.GetGlobalCost(ctx, "last_month", "payment", nil, nil, "")
	if err != nil {
		t.Fatalf("GetGlobalCost: %v", err)
	}
	if resp.TotalCost != 125000 {
		t.Errorf("TotalCost = %v, want 125000 (from aggregate)", resp.TotalCost)
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

// TestCostService_GetGlobalCost_LastMonthFinance_LedgerPFromAggregateFallback BSS/月表现金为 0 时 ledger.P 仍与聚合 payment 一致（避免上月实付全 0）。[Ref: 03_Phase6/01_FinOps]
func TestCostService_GetGlobalCost_LastMonthFinance_LedgerPFromAggregateFallback(t *testing.T) {
	repo := postgres.NewMockRepository(postgres.DefaultMockConfig())
	ctx := context.Background()
	_, prevCycle := reportTypeAndPeriodKey("last_month", time.Now().UTC())
	snap := time.Now()
	wantP := 3200.0
	if err := repo.SaveCloudBillAggregate(ctx, postgres.CloudBillAggregate{
		ReportType:       "last_month",
		PeriodKey:        prevCycle,
		MetricType:       "payment",
		TotalAmount:      wantP,
		ProductBreakdown: map[string]float64{"计算资源": wantP},
		AccountID:        "default",
		LastSuccessAt:    &snap,
		CreatedAt:        snap,
		UpdatedAt:        snap,
	}); err != nil {
		t.Fatalf("SaveCloudBillAggregate: %v", err)
	}
	svc := NewCostService(repo, "", nil)
	resp, err := svc.GetGlobalCost(ctx, "last_month", "payment", nil, nil, "finance")
	if err != nil {
		t.Fatalf("GetGlobalCost: %v", err)
	}
	if resp.Ledger == nil || resp.Ledger.P == nil {
		t.Fatalf("Ledger.P want non-nil from aggregate fallback, got %+v", resp.Ledger)
	}
	if math.Abs(*resp.Ledger.P-wantP) > 1e-6 {
		t.Fatalf("Ledger.P = %v want %v", *resp.Ledger.P, wantP)
	}
}

// TestCostService_GetGlobalCost_EffectiveTrack 合法 track 时写入 metadata.effective_track。[Ref: 03_Phase6/01_FinOps双轨与全域成本重构/01_设计 §API、track 与 UX]
func TestCostService_GetGlobalCost_EffectiveTrack(t *testing.T) {
	repo := postgres.NewMockRepository(postgres.DefaultMockConfig())
	svc := NewCostService(repo, "", nil)
	ctx := context.Background()
	resp, err := svc.GetGlobalCost(ctx, "month", "payment", nil, nil, "technical")
	if err != nil {
		t.Fatalf("GetGlobalCost: %v", err)
	}
	if resp.Metadata == nil || resp.Metadata.EffectiveTrack != "technical" {
		t.Fatalf("EffectiveTrack want technical, got %+v", resp.Metadata)
	}
	resp2, err := svc.GetGlobalCost(ctx, "month", "payment", nil, nil, "")
	if err != nil {
		t.Fatalf("GetGlobalCost: %v", err)
	}
	if resp2.Metadata != nil && resp2.Metadata.EffectiveTrack != "" {
		t.Fatalf("legacy client should not set EffectiveTrack, got %+v", resp2.Metadata)
	}
}

// TestCostService_GetGlobalCost_CloudBillZero 验证聚合表无数据时返回空结构，不降级。[Ref: 聚合表主路径]
func TestCostService_GetGlobalCost_CloudBillZero(t *testing.T) {
	repo := postgres.NewMockRepository(postgres.DefaultMockConfig())
	ctx := context.Background()
	svc := NewCostService(repo, "", nil)
	resp, err := svc.GetGlobalCost(ctx, "month", "payment", nil, nil, "")
	if err != nil {
		t.Fatalf("GetGlobalCost: %v", err)
	}
	if resp == nil {
		t.Fatal("response should not be nil")
	}
	if resp.TotalCost != 0 {
		t.Errorf("TotalCost = %v, want 0 (aggregate empty)", resp.TotalCost)
	}
}

func TestCostService_MixedQueryTimeSeries(t *testing.T) {
	repo := postgres.NewMockRepository(postgres.DefaultMockConfig())
	svc := NewCostService(repo, "", nil)
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

// TestGlobalMetricTypeForTrack_Contract 全域聚合读数与 drilldown 双轨一致：technical→consumption；finance→metricTypeForPeriod；空 track 保持旧语义。[Ref: 03_Phase6/01_FinOps]
func TestGlobalMetricTypeForTrack_Contract(t *testing.T) {
	tests := []struct {
		track, reportType, want string
	}{
		{"technical", "last_month", "consumption"},
		{"finance", "last_month", "payment"},
		{"", "last_month", "payment"},
		{"finance", "month", "consumption"},
		{"technical", "month", "consumption"},
		{"", "month", "consumption"},
	}
	for _, tc := range tests {
		got := globalMetricTypeForTrack(tc.track, tc.reportType)
		if got != tc.want {
			t.Errorf("globalMetricTypeForTrack(%q,%q)=%q want %q", tc.track, tc.reportType, got, tc.want)
		}
	}
}

// TestCostService_GetGlobalCost_DomainBreakdownUsesTrackMetric 同账期 payment 与 consumption 并存时，track 决定读哪条聚合行（成本分解与 Hero 同源）。[Ref: 03_Phase6/01_FinOps]
func TestCostService_GetGlobalCost_DomainBreakdownUsesTrackMetric(t *testing.T) {
	repo := postgres.NewMockRepository(postgres.DefaultMockConfig())
	ctx := context.Background()
	_, prevCycle := reportTypeAndPeriodKey("last_month", time.Now().UTC())
	snap := time.Now()
	acct := "a1"
	if err := repo.SaveCloudBillAggregate(ctx, postgres.CloudBillAggregate{
		ReportType:       "last_month",
		PeriodKey:        prevCycle,
		MetricType:       "payment",
		TotalAmount:      100,
		ProductBreakdown: map[string]float64{"计算资源": 100},
		AccountID:        acct,
		LastSuccessAt:    &snap,
		CreatedAt:        snap,
		UpdatedAt:        snap,
	}); err != nil {
		t.Fatalf("SaveCloudBillAggregate payment: %v", err)
	}
	if err := repo.SaveCloudBillAggregate(ctx, postgres.CloudBillAggregate{
		ReportType:       "last_month",
		PeriodKey:        prevCycle,
		MetricType:       "consumption",
		TotalAmount:      500,
		ProductBreakdown: map[string]float64{"计算资源": 500},
		AccountID:        acct,
		LastSuccessAt:    &snap,
		CreatedAt:        snap,
		UpdatedAt:        snap,
	}); err != nil {
		t.Fatalf("SaveCloudBillAggregate consumption: %v", err)
	}
	svc := NewCostService(repo, "", nil)
	respFin, err := svc.GetGlobalCost(ctx, "last_month", "payment", nil, nil, "finance")
	if err != nil {
		t.Fatalf("GetGlobalCost finance: %v", err)
	}
	if respFin.TotalCost != 100 {
		t.Errorf("finance TotalCost=%v want 100", respFin.TotalCost)
	}
	respTech, err := svc.GetGlobalCost(ctx, "last_month", "payment", nil, nil, "technical")
	if err != nil {
		t.Fatalf("GetGlobalCost technical: %v", err)
	}
	if respTech.TotalCost != 500 {
		t.Errorf("technical TotalCost=%v want 500", respTech.TotalCost)
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

func f64ptr(f float64) *float64 { return &f }

// TestEnrichProjectBreakdownFromEnvBreakdown_consumptionWeights 项目卡 P 等于成员环境 env 行 LedgerP 之和（与 env 卡一致）；不再用全局 Hero P 按比例回填。[Ref: 03_Phase6/03_前端全域成本透视 R16]
func TestEnrichProjectBreakdownFromEnvBreakdown_consumptionWeights(t *testing.T) {
	t.Parallel()
	gp := 334.46
	pocP := gp * 512 / (512 + 334.46)
	uatP := gp - pocP
	projects := []postgres.CostProject{
		{ID: 1, Environments: []string{"POC"}},
		{ID: 2, Environments: []string{"UAT"}},
	}
	resp := &dto.GlobalCostResponse{
		EnvBreakdown: []dto.EnvBreakdownItem{
			{Environment: "POC", TotalCost: 0, ConsumptionCost: f64ptr(512), LedgerP: f64ptr(pocP)},
			{Environment: "UAT", TotalCost: 0, ConsumptionCost: f64ptr(334.46), LedgerP: f64ptr(uatP)},
		},
		ProjectBreakdown: []dto.ProjectBreakdownItem{
			{ProjectID: 1, Name: "K8s"},
			{ProjectID: 2, Name: "C66"},
		},
		Ledger: &dto.FinOpsLedger{P: &gp},
	}
	enrichProjectBreakdownFromEnvBreakdown(resp, projects, "finance")
	if resp.ProjectBreakdown[0].LedgerP == nil || resp.ProjectBreakdown[1].LedgerP == nil {
		t.Fatal("expected LedgerP on projects")
	}
	pK8s := *resp.ProjectBreakdown[0].LedgerP
	pC66 := *resp.ProjectBreakdown[1].LedgerP
	if math.Abs(pK8s-pocP) > 0.02 {
		t.Fatalf("K8s P %v want ~%v", pK8s, pocP)
	}
	if math.Abs(pC66-uatP) > 0.02 {
		t.Fatalf("C66 P %v want ~%v", pC66, uatP)
	}
	if math.Abs(pK8s+pC66-gp) > 0.02 {
		t.Fatalf("sum P %v + %v want %v", pK8s, pC66, gp)
	}
}

func TestEnrichProjectBreakdownFromEnvBreakdown_zeroWeightProjectGetsNoP(t *testing.T) {
	t.Parallel()
	gp := 334.0
	projects := []postgres.CostProject{
		{ID: 1, Environments: []string{"POC"}},
		{ID: 2, Environments: []string{"FAT"}},
	}
	resp := &dto.GlobalCostResponse{
		EnvBreakdown: []dto.EnvBreakdownItem{
			{Environment: "POC", TotalCost: 0, ConsumptionCost: f64ptr(100)},
			{Environment: "FAT", TotalCost: 0, ConsumptionCost: f64ptr(0)},
		},
		ProjectBreakdown: []dto.ProjectBreakdownItem{
			{ProjectID: 1, Name: "K8s"},
			{ProjectID: 2, Name: "Laroplus"},
		},
		Ledger: &dto.FinOpsLedger{P: &gp},
	}
	enrichProjectBreakdownFromEnvBreakdown(resp, projects, "finance")
	if resp.ProjectBreakdown[1].LedgerP == nil || math.Abs(*resp.ProjectBreakdown[1].LedgerP) > 1e-6 {
		t.Fatalf("zero-weight project P want 0 got %v", resp.ProjectBreakdown[1].LedgerP)
	}
}
