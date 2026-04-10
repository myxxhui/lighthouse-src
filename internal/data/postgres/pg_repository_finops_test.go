package postgres

import "testing"

func TestDedupeFinOpsBillingFactRowsForBatchUpsert(t *testing.T) {
	t.Parallel()
	a := FinOpsBillingFactRow{AccountID: "5823", DedupKey: "same", Amount: 1}
	b := FinOpsBillingFactRow{AccountID: "5823", DedupKey: "same", Amount: 2}
	c := FinOpsBillingFactRow{AccountID: "5823", DedupKey: "other", Amount: 3}
	got := dedupeFinOpsBillingFactRowsForBatchUpsert([]FinOpsBillingFactRow{a, b, c})
	if len(got) != 2 {
		t.Fatalf("want 2 rows, got %d", len(got))
	}
	if got[0].Amount != 2 || got[1].Amount != 3 {
		t.Fatalf("last wins for duplicate key: %+v %+v", got[0], got[1])
	}
}
