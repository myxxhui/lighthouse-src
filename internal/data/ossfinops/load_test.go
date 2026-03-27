package ossfinops

import (
	"sort"
	"testing"
	"time"
)

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
	if got := guessBillingCycleFromKey(key); got != "2025-12" {
		t.Fatalf("got %q want 2025-12 (must not take timestamp 20260325 as cycle)", got)
	}
	if got := guessBillingCycleFromKey("export_2024-01.csv"); got != "2024-01" {
		t.Fatalf("got %q want 2024-01", got)
	}
}

func TestGuessBillingCycleFromKeyRollingNoMonthSuffix(t *testing.T) {
	t.Parallel()
	key := "billing-data/1657988574642393-20260325130300_consumedetailbillv2.csv"
	if got := guessBillingCycleFromKey(key); got != "" {
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
	if got := exportTimestampFromConsumeDetailName("plain.csv"); got != "" {
		t.Fatalf("want empty, got %q", got)
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
