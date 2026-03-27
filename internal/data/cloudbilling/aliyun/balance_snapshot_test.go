package aliyun

import (
	"testing"

	"github.com/alibabacloud-go/bssopenapi-20171214/v4/client"
)

func TestBalanceFromQueryAccountBalanceData_fallbackCreditWhenAvailableZero(t *testing.T) {
	avail := "0"
	cash := "10.5"
	cred := "20"
	d := &client.QueryAccountBalanceResponseBodyData{
		AvailableAmount:     &avail,
		AvailableCashAmount: &cash,
		CreditAmount:        &cred,
	}
	got := balanceFromQueryAccountBalanceData(d)
	want := 30.5
	if got < want-1e-6 || got > want+1e-6 {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestParseAliyunFloat_thousandsSeparator(t *testing.T) {
	if got := parseAliyunFloat("7,245.65"); got < 7245.64 || got > 7245.66 {
		t.Fatalf("parseAliyunFloat(7,245.65) got %v want 7245.65", got)
	}
}

func TestBalanceFromQueryAccountBalanceData_internationalCommaAvailable(t *testing.T) {
	avail := "7,245.65"
	d := &client.QueryAccountBalanceResponseBodyData{
		AvailableAmount: &avail,
	}
	got := balanceFromQueryAccountBalanceData(d)
	if got < 7245.64 || got > 7245.66 {
		t.Fatalf("got %v want 7245.65", got)
	}
}

func TestBalanceFromQueryAccountBalanceData_primaryAvailable(t *testing.T) {
	avail := "100"
	cred := "50"
	d := &client.QueryAccountBalanceResponseBodyData{
		AvailableAmount: &avail,
		CreditAmount:    &cred,
	}
	got := balanceFromQueryAccountBalanceData(d)
	if got < 99.99 || got > 100.01 {
		t.Fatalf("got %v want 100 (do not add credit when available non-zero)", got)
	}
}
