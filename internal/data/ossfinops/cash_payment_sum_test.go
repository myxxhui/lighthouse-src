package ossfinops

import (
	"strings"
	"testing"
)

func TestSumCashPaymentColumnInCSV(t *testing.T) {
	t.Parallel()
	csv := "主键,账单日期,账单月份,产品code,优惠后金额,现金支付金额,定价币种\n" +
		"r1,2025-09-01,2025-09,ecs,10.5,2.5,USD\n" +
		"r2,2025-09-02,2025-09,ecs,5,1.25,USD\n"
	sum, found, err := SumCashPaymentColumnInCSV(strings.NewReader(csv), "t.csv", "2025-09", "2025-09")
	if err != nil {
		t.Fatal(err)
	}
	if !found {
		t.Fatal("want cash column found")
	}
	if sum != 3.75 {
		t.Fatalf("sum %v want 3.75", sum)
	}
}

func TestSumCashPaymentColumnInCSV_FilterCycle(t *testing.T) {
	t.Parallel()
	csv := "主键,账单日期,账单月份,现金支付金额\n" +
		"r1,2025-08-01,2025-08,100\n" +
		"r2,2025-09-01,2025-09,50\n"
	sum, found, err := SumCashPaymentColumnInCSV(strings.NewReader(csv), "t.csv", "2025-09", "")
	if err != nil {
		t.Fatal(err)
	}
	if !found {
		t.Fatal("want cash column")
	}
	if sum != 50 {
		t.Fatalf("sum %v want 50", sum)
	}
}
