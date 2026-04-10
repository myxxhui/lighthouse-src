package aliyun

import (
	"math"
	"testing"

	"github.com/alibabacloud-go/bssopenapi-20171214/v4/client"
)

func TestEffectivePretaxPayableFromItem_couponSubtractsFromGross(t *testing.T) {
	t.Parallel()
	g := float32(6280.605666)
	disc := float32(3895.968998)
	coup := float32(272.638946)
	pretaxAfterDiscountOnly := float32(2384.636668)
	it := &client.QueryAccountBillResponseBodyDataItemsItem{
		PretaxGrossAmount:   &g,
		InvoiceDiscount:     &disc,
		DeductedByCoupons:   &coup,
		PretaxAmount:        &pretaxAfterDiscountOnly,
	}
	got := effectivePretaxPayableFromItem(it)
	want := float64(g) - float64(disc) - float64(coup)
	if math.Abs(got-want) > 1e-3 {
		t.Fatalf("got %v want %v (≈控制台应付)", got, want)
	}
}

func TestEffectivePretaxPayableFromItem_noSplitUsesPretax(t *testing.T) {
	t.Parallel()
	p := float32(2111.89)
	g := float32(6280.0)
	it := &client.QueryAccountBillResponseBodyDataItemsItem{
		PretaxGrossAmount: &g,
		PretaxAmount:      &p,
	}
	if got := effectivePretaxPayableFromItem(it); math.Abs(got-float64(p)) > 1e-6 {
		t.Fatalf("got %v want pretax %v when no discount/coupon columns", got, p)
	}
}
