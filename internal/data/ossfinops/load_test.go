package ossfinops

import (
	"archive/zip"
	"bytes"
	"math"
	"sort"
	"strings"
	"testing"
	"time"
)

func TestParseCSVPreferDiscountedAmountAndUSD(t *testing.T) {
	t.Parallel()
	// 与 k8s-finops-billing-poc 类导出：同时存在「应付金额」与「优惠后金额」时取后者；定价币种 USD。[Ref: 03_Phase6/01_FinOps]
	csv := "主键,账单日期,账单月份,产品code,应付金额,优惠后金额,定价币种,标签,资产id\n" +
		"rid-1,2025-10-01,2025-10,ecs,1.50,1.20,USD,env:POC;i-t1,i-t1\n"
	rows, cycle, err := parseCSVToFacts(strings.NewReader(csv), "t.csv", "5823052810429629", "2025-10")
	if err != nil {
		t.Fatal(err)
	}
	if cycle != "2025-10" {
		t.Fatalf("cycle %q", cycle)
	}
	if len(rows) != 1 {
		t.Fatalf("rows %d", len(rows))
	}
	if rows[0].Amount != 1.2 {
		t.Fatalf("amount %v want 1.2 (优惠后金额)", rows[0].Amount)
	}
	if rows[0].Currency != "USD" {
		t.Fatalf("currency %q", rows[0].Currency)
	}
	if rows[0].Env != "POC" {
		t.Fatalf("env %q", rows[0].Env)
	}
}

func TestNormalizeRoundingSum_dedupDuplicatePerRow(t *testing.T) {
	t.Parallel()
	// 控制台整月抹零 0.101269，若 CSV 在 11 行上各重复一次，列和≈1.114，应压成单笔 0.101269
	raw := 11 * 0.101269
	got := normalizeRoundingSum(raw, 0.101269)
	if math.Abs(got-0.101269) > 1e-9 {
		t.Fatalf("got %v want 0.101269", got)
	}
	// 行级分摊：单列 max 很小，不 dedup，保持 sum
	got2 := normalizeRoundingSum(0.101269, 0.000052)
	if math.Abs(got2-0.101269) > 1e-9 {
		t.Fatalf("got2 %v want 0.101269", got2)
	}
	// 按比例多行（如 5×0.02），sum 不大于 10×max，保持 sum
	got3 := normalizeRoundingSum(0.1, 0.02)
	if math.Abs(got3-0.1) > 1e-9 {
		t.Fatalf("got3 %v want 0.1", got3)
	}
}

func TestInstanceConsumeDaySkipsBillLevelCouponRoundingRows(t *testing.T) {
	t.Parallel()
	// instanceconsumeday 即使表头含券/抹零列，也不追加账单级 FINOPS_BILLING_COUPON_DEDUCTION / ROUNDING。[Ref: OSS_BILLING_CONFIG 方案 A]
	csv := "主键,账单日期,账单月份,产品code,应付金额,优惠后金额,优惠券抵扣金额,抹零金额,定价币种,标签,资产id\n" +
		"r1,2025-10-01,2025-10,ecs,130,100,30,0.1,USD,env:POC;i-t1,i-t1\n"
	rows, cycle, err := parseCSVToFacts(strings.NewReader(csv), "acct_instanceconsumeday.csv", "5823052810429629", "2025-10")
	if err != nil {
		t.Fatal(err)
	}
	if cycle != "2025-10" {
		t.Fatalf("cycle %q", cycle)
	}
	if len(rows) != 1 {
		t.Fatalf("want 1 line row only, got %d", len(rows))
	}
	for _, r := range rows {
		if r.ItemCode == "FINOPS_BILLING_COUPON_DEDUCTION" || r.ItemCode == "FINOPS_BILLING_ROUNDING" {
			t.Fatalf("unexpected bill-level row: %+v", r)
		}
	}
}

func TestParseCSVInstanceConsumeDayNoFalseCouponFromAmountColumn(t *testing.T) {
	t.Parallel()
	// 回归：findCol 曾用 Contains(候选,表头)，使 "couponamount" 误命中短表头 "amount"，把应付列当券抵扣列累加。[Ref: 04_采集 §5.4 优惠券]
	csv := "主键,账单日期,账单月份,产品code,amount,优惠后金额,定价币种,标签,资产id\n" +
		"r1,2025-10-01,2025-10,ecs,999,100,USD,env:POC;i-t1,i-t1\n"
	rows, cycle, err := parseCSVToFacts(strings.NewReader(csv), "instanceconsumeday_test.csv", "5823052810429629", "2025-10")
	if err != nil {
		t.Fatal(err)
	}
	if cycle != "2025-10" {
		t.Fatalf("cycle %q", cycle)
	}
	if len(rows) != 1 {
		t.Fatalf("want 1 detail row only, got %d", len(rows))
	}
	for _, r := range rows {
		if r.ItemCode == "FINOPS_BILLING_COUPON_DEDUCTION" {
			t.Fatalf("unexpected coupon row: %+v", r)
		}
	}
	if rows[0].Amount != 100 {
		t.Fatalf("detail amount %v want 100 (优惠后金额)", rows[0].Amount)
	}
}

func TestParseCSVCouponDeductionAppendRow(t *testing.T) {
	t.Parallel()
	// 控制台应付 = 优惠后明细之和 − 优惠券抵扣；列「优惠券抵扣」为正数时补 FINOPS_BILLING_COUPON_DEDUCTION amount=−sum。[Ref: 04_采集 §5.4]
	csv := "主键,账单日期,账单月份,产品code,应付金额,优惠后金额,优惠券抵扣金额,定价币种,标签,资产id\n" +
		"r1,2025-12-01,2025-12,ecs,130,100,30,USD,env:POC;i-t1,i-t1\n"
	rows, cycle, err := parseCSVToFacts(strings.NewReader(csv), "t.csv", "5823052810429629", "2025-12")
	if err != nil {
		t.Fatal(err)
	}
	if cycle != "2025-12" {
		t.Fatalf("cycle %q", cycle)
	}
	if len(rows) != 2 {
		t.Fatalf("want 1 detail + 1 coupon, got %d", len(rows))
	}
	if rows[0].Amount != 100 {
		t.Fatalf("detail amount %v", rows[0].Amount)
	}
	c := rows[1]
	if c.ProductCode != "COUPON" || c.ItemCode != "FINOPS_BILLING_COUPON_DEDUCTION" || c.Amount != -30 {
		t.Fatalf("coupon row: %+v", c)
	}
	if math.Abs(rows[0].Amount+c.Amount-70) > 1e-9 {
		t.Fatalf("sum %v want 70", rows[0].Amount+c.Amount)
	}
}

func TestParseCSVCouponAndRoundingAppendRows(t *testing.T) {
	t.Parallel()
	csv := "主键,账单日期,账单月份,产品code,应付金额,优惠后金额,优惠券抵扣金额,抹零金额,定价币种,标签,资产id\n" +
		"r1,2025-12-01,2025-12,ecs,130,100,272.638946,0.107722,USD,env:POC;i-t1,i-t1\n"
	rows, _, err := parseCSVToFacts(strings.NewReader(csv), "t.csv", "5823052810429629", "2025-12")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 3 {
		t.Fatalf("want 1 detail + coupon + rounding, got %d", len(rows))
	}
	var sum float64
	for _, r := range rows {
		sum += r.Amount
	}
	want := 100.0 - 272.638946 - 0.107722
	if math.Abs(sum-want) > 1e-6 {
		t.Fatalf("sum(amount)=%v want %v (≈控制台应付)", sum, want)
	}
}

func TestParseCSVRoundingAppendRow(t *testing.T) {
	t.Parallel()
	// 抹零列按文件求和，补一行 FINOPS_BILLING_ROUNDING，使 SUM(amount) 与控制台应付一致。[Ref: 03_Phase6/01_FinOps 抹零]
	csv := "主键,账单日期,账单月份,产品code,应付金额,优惠后金额,抹零金额,定价币种,标签,资产id\n" +
		"r1,2025-10-01,2025-10,ecs,10,10,0,USD,env:POC;i-t1,i-t1\n" +
		"r2,2025-10-02,2025-10,ecs,5,5,0.101269,USD,env:POC;i-t2,i-t2\n"
	rows, cycle, err := parseCSVToFacts(strings.NewReader(csv), "t.csv", "5823052810429629", "2025-10")
	if err != nil {
		t.Fatal(err)
	}
	if cycle != "2025-10" {
		t.Fatalf("cycle %q", cycle)
	}
	if len(rows) != 3 {
		t.Fatalf("want 2 detail + 1 rounding, got %d", len(rows))
	}
	var sum float64
	for i := 0; i < 2; i++ {
		sum += rows[i].Amount
	}
	r := rows[2]
	if r.ProductCode != "ROUNDING" || r.ItemCode != "FINOPS_BILLING_ROUNDING" {
		t.Fatalf("rounding row: %+v", r)
	}
	if r.Amount != -0.101269 {
		t.Fatalf("rounding amount %v want -0.101269", r.Amount)
	}
	sum += r.Amount
	want := 10.0 + 5.0 - 0.101269
	if math.Abs(sum-want) > 1e-9 {
		t.Fatalf("sum(amount) %v want %v (10+5-抹零)", sum, want)
	}
}

func TestParseDateFlexibleYYYYMMDD(t *testing.T) {
	t.Parallel()
	got, err := parseDateFlexible("20260128")
	if err != nil {
		t.Fatal(err)
	}
	if got.Year() != 2026 || got.Month() != time.January || got.Day() != 28 {
		t.Fatalf("got %v", got)
	}
}

func TestGuessBillingCycleFromKey(t *testing.T) {
	t.Parallel()
	key := "billing-data/1657988574642393-20260325130300_consumedetailbillv2_2025-12.csv"
	if got := GuessBillingCycleFromKey(key); got != "2025-12" {
		t.Fatalf("got %q want 2025-12 (must not take timestamp 20260325 as cycle)", got)
	}
	if got := GuessBillingCycleFromKey("export_2024-01.csv"); got != "2024-01" {
		t.Fatalf("got %q want 2024-01", got)
	}
}

func TestGuessBillingCycleFromKeyRollingNoMonthSuffix(t *testing.T) {
	t.Parallel()
	key := "billing-data/1657988574642393-20260325130300_consumedetailbillv2.csv"
	if got := GuessBillingCycleFromKey(key); got != "" {
		t.Fatalf("got %q want empty (账期以 CSV 列/行为准，文件名含导出时间)", got)
	}
}

func TestIsClosedMonthCSVFilename(t *testing.T) {
	t.Parallel()
	if !isClosedMonthCSVFilename("foo_2025-09.csv") {
		t.Fatal("want true for _YYYY-MM.csv suffix")
	}
	if isClosedMonthCSVFilename("165798-20260325130300_consumedetailbillv2.csv") {
		t.Fatal("rolling export without month suffix must be false")
	}
}

func TestBaseName(t *testing.T) {
	t.Parallel()
	if baseName("a/b/c_2024-01.csv") != "c_2024-01.csv" {
		t.Fatalf("got %q", baseName("a/b/c_2024-01.csv"))
	}
}

func TestExportTimestampFromConsumeDetailName(t *testing.T) {
	t.Parallel()
	if got := exportTimestampFromConsumeDetailName("1657988574642393-20260326112001_consumedetailbillv2_2026-03.csv"); got != "20260326112001" {
		t.Fatalf("got %q want 20260326112001", got)
	}
	if got := exportTimestampFromConsumeDetailName("5823052810429629-20260330134429_instanceconsumeday.csv"); got != "20260330134429" {
		t.Fatalf("got %q want 20260330134429", got)
	}
	if got := exportTimestampFromConsumeDetailName("plain.csv"); got != "" {
		t.Fatalf("want empty, got %q", got)
	}
}

func TestIsLikelyOSSBillingDataObjectKey(t *testing.T) {
	t.Parallel()
	if !IsLikelyOSSBillingDataObjectKey("finops-billing/a.csv") {
		t.Fatal("want .csv true")
	}
	if !IsLikelyOSSBillingDataObjectKey("finops-billing/5823052810429629_BillingItemDetail_20260329") {
		t.Fatal("want BillingItemDetail without ext true")
	}
	if IsLikelyOSSBillingDataObjectKey("finops-billing/readme.txt") {
		t.Fatal("want non-billing false")
	}
}

func TestGuessBillingCycleFromKey_BillingItemDetailNoExt(t *testing.T) {
	t.Parallel()
	key := "finops-billing/5823052810429629_BillingItemDetail_20260329"
	if got := GuessBillingCycleFromKey(key); got != "2026-03" {
		t.Fatalf("got %q want 2026-03", got)
	}
}

func TestParseCSVCycleColumnOnlyNoDayColumn(t *testing.T) {
	t.Parallel()
	// BSS BillingItemDetail 类：仅有账单月份 + 金额，无「账单日期」列。[Ref: 04_采集 §七 R10]
	csv := "主键,账单月份,产品名称,优惠后金额,定价币种\n" +
		"r1,2026-04,ecs,12.5,USD\n"
	rows, cycle, err := parseCSVToFacts(strings.NewReader(csv), "detail.csv", "5823052810429629", "2026-04")
	if err != nil {
		t.Fatal(err)
	}
	if cycle != "2026-04" {
		t.Fatalf("cycle %q", cycle)
	}
	if len(rows) != 1 {
		t.Fatalf("rows %d", len(rows))
	}
	if rows[0].Amount != 12.5 || rows[0].UsageDate.Format("2006-01-02") != "2026-04-01" {
		t.Fatalf("row %+v", rows[0])
	}
}

func TestBillingItemDetailZipInnerCSVParsed(t *testing.T) {
	t.Parallel()
	// 与 ingestBillingItemDetailZip 一致：zip 内嵌 .csv 再走 parseCSVToFacts。[Ref: 04_采集 §七 R13]
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, err := zw.Create("inner.csv")
	if err != nil {
		t.Fatal(err)
	}
	csv := "主键,账单月份,产品code,优惠后金额,定价币种,标签,资产id\n" +
		"r1,2026-04,oss,10.5,USD,env:POC,x1\n"
	if _, err := w.Write([]byte(csv)); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	zr, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, f := range zr.File {
		if f.FileInfo().IsDir() {
			continue
		}
		if !strings.HasSuffix(strings.ToLower(f.Name), ".csv") {
			continue
		}
		fr, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		rows, cycle, err := parseCSVToFacts(fr, "BillingItemDetail_202604.zip#"+f.Name, "5823052810429629", "2026-04")
		_ = fr.Close()
		if err != nil {
			t.Fatal(err)
		}
		if cycle != "2026-04" {
			t.Fatalf("cycle %q", cycle)
		}
		if len(rows) != 1 || rows[0].Amount != 10.5 {
			t.Fatalf("rows %+v", rows)
		}
		found = true
	}
	if !found {
		t.Fatal("no .csv inside zip")
	}
}

func TestSortObjectsForIngestionTieBreak(t *testing.T) {
	t.Parallel()
	// 相同 LastModified 时按文件名内 14 位导出时间升序，使较新导出后处理
	t1 := time.Date(2026, 3, 26, 10, 0, 0, 0, time.UTC)
	a := []fakeObj{
		{Key: "p/a-20260326110000_consumedetail.csv", LM: t1},
		{Key: "p/b-20260326120000_consumedetail.csv", LM: t1},
	}
	sortObjectsForIngestionFake(a)
	if a[0].Key != "p/a-20260326110000_consumedetail.csv" || a[1].Key != "p/b-20260326120000_consumedetail.csv" {
		t.Fatalf("order: %+v", a)
	}
}

// fakeObj mirrors oss.ObjectProperties fields used by sortObjectsForIngestion — tested via duplicate sort logic in test
type fakeObj struct {
	Key string
	LM  time.Time
}

func sortObjectsForIngestionFake(all []fakeObj) {
	sort.Slice(all, func(i, j int) bool {
		ai, aj := all[i], all[j]
		if !ai.LM.Equal(aj.LM) {
			return ai.LM.Before(aj.LM)
		}
		ti := exportTimestampFromConsumeDetailName(baseName(ai.Key))
		tj := exportTimestampFromConsumeDetailName(baseName(aj.Key))
		if ti != tj {
			return ti < tj
		}
		return ai.Key < aj.Key
	})
}

func TestStableFinOpsDedupKey(t *testing.T) {
	t.Parallel()
	a := stableFinOpsDedupKey("UAT", "rid-1", "2026-03", "2026-03-01", "i-1", "item", "ecs")
	b := stableFinOpsDedupKey("UAT", "rid-1", "2026-03", "2026-03-02", "i-2", "x", "y")
	if a != b {
		t.Fatalf("same RecordID must same dedup key, got %q vs %q", a, b)
	}
	c := stableFinOpsDedupKey("UAT", "", "2026-03", "2026-03-01", "i-1", "it", "ecs")
	d := stableFinOpsDedupKey("UAT", "", "2026-03", "2026-03-01", "i-1", "it", "ecs")
	if c != d {
		t.Fatalf("natural key stable: %q vs %q", c, d)
	}
	e := stableFinOpsDedupKey("UAT", "", "2026-03", "2026-03-01", "i-2", "it", "ecs")
	if c == e {
		t.Fatal("different instance_id must differ")
	}
}
