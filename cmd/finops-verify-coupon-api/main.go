// 本地实践验证：仅调阿里云 BSS QueryAccountBill，检查能否拿到「优惠券抵扣」字段（DeductedByCoupons / DeductedByCashCoupons）。
// 不依赖 PostgreSQL。需配置 ALIBABA_CLOUD_ACCESS_KEY_ID_<env> / SECRET 与 Endpoint（国际站常见 business.ap-southeast-1.aliyuncs.com）。
//
//	cd lighthouse-src && set -a && source ../lighthouse-deploy/.env && set +a
//	go run ./cmd/finops-verify-coupon-api -cycle=2025-12 -env=C66_POC
//
// [Ref: 04_采集 §5.4 优惠券、QueryAccountBill]
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/myxxhui/lighthouse-src/internal/data/cloudbilling/aliyun"
)

func main() {
	cycle := flag.String("cycle", "2025-12", "账期 YYYY-MM")
	env := flag.String("env", "C66_POC", "AK 后缀，与 ALIBABA_CLOUD_ACCESS_KEY_ID_* 一致")
	asJSON := flag.Bool("json", false, "一行 JSON 输出")
	flag.Parse()

	f, ok := aliyun.NewFetcherForEnv(strings.TrimSpace(*env))
	if !ok {
		fmt.Fprintf(os.Stderr, "NewFetcherForEnv(%q) 失败：请设置 ALIBABA_CLOUD_ACCESS_KEY_ID_%s 与 ALIBABA_CLOUD_ACCESS_KEY_SECRET_%s（及可选 CLOUD_BILLING_ENDPOINT_%s）\n",
			*env, *env, *env, *env)
		os.Exit(1)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	cy := strings.TrimSpace(*cycle)
	items, err := f.FetchQueryAccountBillMonthlyItems(ctx, cy, false)
	if err != nil {
		fmt.Fprintf(os.Stderr, "QueryAccountBill MONTHLY: %v\n", err)
		os.Exit(1)
	}

	var monthlyCoupon float64
	for _, it := range items {
		if it == nil {
			continue
		}
		if it.DeductedByCoupons != nil {
			monthlyCoupon += float64(*it.DeductedByCoupons)
		}
		if it.DeductedByCashCoupons != nil {
			monthlyCoupon += float64(*it.DeductedByCashCoupons)
		}
	}

	total, err := f.SumCouponDeductionForBillingCycle(ctx, cy)
	if err != nil {
		fmt.Fprintf(os.Stderr, "SumCouponDeductionForBillingCycle: %v\n", err)
		os.Exit(1)
	}

	out := map[string]interface{}{
		"billing_cycle":                    cy,
		"credential_env":                   strings.TrimSpace(*env),
		"bss_endpoint":                     aliyun.ResolveBillingEndpointForEnv(strings.TrimSpace(*env)),
		"monthly_row_count":                len(items),
		"coupon_sum_monthly_only":          monthlyCoupon,
		"coupon_sum_with_daily_fallback":   total,
		"got_coupon_data":                  monthlyCoupon > 1e-9 || total > 1e-9,
		"note":                             "DeductedByCoupons 与 DeductedByCashCoupons 为正数时即表示 API 返回了抵扣额；monthly 为 0 时可能走按日 DAILY 累加",
	}

	if *asJSON {
		b, _ := json.MarshalIndent(out, "", "  ")
		fmt.Println(string(b))
		return
	}

	fmt.Println("======== 本地实践：优惠券抵扣金额（仅 API，无数据库）========")
	fmt.Printf("账期:              %s\n", cy)
	fmt.Printf("凭证环境:          %s\n", *env)
	fmt.Printf("BSS Endpoint:      %s\n", out["bss_endpoint"])
	fmt.Printf("QueryAccountBill MONTHLY 行数(IsGroupByProduct=false): %d\n", len(items))
	fmt.Printf("MONTHLY 券抵扣合计: %.6f  (DeductedByCoupons + DeductedByCashCoupons)\n", monthlyCoupon)
	fmt.Printf("SumCouponDeductionForBillingCycle: %.6f  (MONTHLY>0 则用月；否则按日 DAILY 累加)\n", total)
	if len(items) > 0 && items[0] != nil {
		it := items[0]
		fmt.Println("-------- 首行样例（字段是否存在）--------")
		if it.DeductedByCoupons != nil {
			fmt.Printf("  DeductedByCoupons:     %v\n", *it.DeductedByCoupons)
		} else {
			fmt.Println("  DeductedByCoupons:     <nil>")
		}
		if it.DeductedByCashCoupons != nil {
			fmt.Printf("  DeductedByCashCoupons: %v\n", *it.DeductedByCashCoupons)
		} else {
			fmt.Println("  DeductedByCashCoupons: <nil>")
		}
		if it.PretaxAmount != nil {
			fmt.Printf("  PretaxAmount:          %v\n", *it.PretaxAmount)
		}
		if it.PretaxGrossAmount != nil {
			fmt.Printf("  PretaxGrossAmount:     %v\n", *it.PretaxGrossAmount)
		}
		if it.InvoiceDiscount != nil {
			fmt.Printf("  InvoiceDiscount:       %v\n", *it.InvoiceDiscount)
		}
	}
	fmt.Println()
	if total > 1e-9 || monthlyCoupon > 1e-9 {
		fmt.Println("结论: 已拿到非零优惠券/代金券抵扣汇总，可与控制台「优惠券抵扣金额」对账。")
	} else {
		fmt.Println("结论: 当前 MONTHLY 与 DAILY 兜底合计均为 0。可能该账期无券、或字段在其它粒度/账号下；请核对账期与主账号凭证。")
	}
}
