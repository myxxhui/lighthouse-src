package etl

import "testing"

func TestBssTransactionsLookbackDays(t *testing.T) {
	cases := []struct {
		name string
		env  string
		want int
	}{
		{"empty", "", bssLookbackDefaultDays},
		{"explicit default", "14", 14},
		{"wider", "45", 45},
		{"invalid", "x", bssLookbackDefaultDays},
		{"zero", "0", bssLookbackDefaultDays},
		{"negative", "-3", bssLookbackDefaultDays},
		{"capped", "99999", bssLookbackMaxDays},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("FINOPS_BSS_TRANSACTIONS_LOOKBACK_DAYS", tc.env)
			if got := BssTransactionsLookbackDays(); got != tc.want {
				t.Fatalf("BssTransactionsLookbackDays() = %d, want %d", got, tc.want)
			}
		})
	}
}
