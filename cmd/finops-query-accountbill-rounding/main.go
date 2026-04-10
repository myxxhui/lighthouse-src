// 临时：QueryAccountBill（MONTHLY）拉取账期内明细，筛「抹零」与 AdjustAmount，对照控制台抹零金额。[Ref: 03_Phase6/01_FinOps API]
//
//	cd lighthouse-src && set -a && source ../lighthouse-deploy/.env && set +a && go run ./cmd/finops-query-accountbill-rounding -cycle=2025-10 -env=C66_POC
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/myxxhui/lighthouse-src/internal/data/cloudbilling/aliyun"
)

func main() {
	cycle := flag.String("cycle", "2025-10", "账期 YYYY-MM")
	env := flag.String("env", "C66_POC", "与 ALIBABA_CLOUD_ACCESS_KEY_ID_* / CLOUD_BILLING_ENDPOINT_* 后缀一致")
	byProduct := flag.Bool("by-product", false, "IsGroupByProduct=true；默认 false（更粗粒度）。可各跑一遍对比")
	alsoGrouped := flag.Bool("also-grouped", true, "在 by-product=false 跑完后，再拉一遍 by-product=true")
	flag.Parse()

	f, ok := aliyun.NewFetcherForEnv(strings.TrimSpace(*env))
	if !ok {
		fmt.Fprintf(os.Stderr, "NewFetcherForEnv(%q) 失败：请设置 ALIBABA_CLOUD_ACCESS_KEY_ID_%s / SECRET 与 CLOUD_BILLING_ENDPOINT_%s（国际站常见 business.ap-southeast-1.aliyuncs.com）\n",
			*env, *env, *env)
		os.Exit(1)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()

	fmt.Println("======== QueryAccountBill MONTHLY — 抹零/调账核对（临时程序）========")
	fmt.Printf("账期=%s  env=%s\n", *cycle, *env)
	fmt.Println("说明：阿里云文档常见口径为 Granularity=MONTHLY；抹零可能在 ProductName 或体现在 PretaxAmount/AdjustAmount。")
	fmt.Println()

	run := func(label string, grouped bool) {
		fmt.Printf("--- %s (IsGroupByProduct=%v) ---\n", label, grouped)
		items, err := f.FetchQueryAccountBillMonthlyItems(ctx, strings.TrimSpace(*cycle), grouped)
		if err != nil {
			fmt.Fprintf(os.Stderr, "错误: %v\n", err)
			return
		}
		fmt.Printf("条数: %d\n", len(items))
		if len(items) == 1 {
			it := items[0]
			fmt.Printf("（单月汇总仅 1 条）ProductName=%q ProductCode=%q\n",
				strPtr(it.ProductName), strPtr(it.ProductCode))
			fmt.Printf("  PretaxAmount=%v CashAmount=%v AdjustAmount=%v OutstandingAmount=%v\n",
				f32Ptr(it.PretaxAmount), f32Ptr(it.CashAmount), f32Ptr(it.AdjustAmount), f32Ptr(it.OutstandingAmount))
		}
		var sumPretax, sumCash, sumAdjust float64
		for _, it := range items {
			if it.PretaxAmount != nil {
				sumPretax += float64(*it.PretaxAmount)
			}
			if it.CashAmount != nil {
				sumCash += float64(*it.CashAmount)
			}
			if it.AdjustAmount != nil {
				sumAdjust += float64(*it.AdjustAmount)
			}
		}
		fmt.Printf("汇总 PretaxAmount: %.6f  CashAmount: %.6f  AdjustAmount: %.6f\n", sumPretax, sumCash, sumAdjust)

		nShow := 0
		for _, it := range items {
			pn := strPtr(it.ProductName)
			pc := strPtr(it.ProductCode)
			var pretax, cash, adj float64
			if it.PretaxAmount != nil {
				pretax = float64(*it.PretaxAmount)
			}
			if it.CashAmount != nil {
				cash = float64(*it.CashAmount)
			}
			if it.AdjustAmount != nil {
				adj = float64(*it.AdjustAmount)
			}
			hit := strings.Contains(pn, "抹零") || strings.Contains(strings.ToLower(pn), "round") ||
				strings.Contains(strings.ToLower(pc), "round") || adj != 0
			if !hit {
				continue
			}
			nShow++
			fmt.Printf("  [%d] ProductName=%q ProductCode=%q Pretax=%.6f Cash=%.6f Adjust=%.6f\n",
				nShow, pn, pc, pretax, cash, adj)
		}
		if nShow == 0 {
			fmt.Println("  （无 ProductName 含「抹零」/round 且 Adjust 全 0 的命中行；抹零可能合在汇总 Pretax 中，见阿里云说明）")
		}
		fmt.Println()
	}

	if *byProduct {
		run("仅按产品分组", true)
	} else {
		run("不按产品分组（粗）", false)
		if *alsoGrouped {
			run("按产品分组", true)
		}
	}

	fmt.Println("结论摘要：")
	fmt.Println("  • QueryAccountBill MONTHLY + IsGroupByProduct=false 常为「整账期一行」，PretaxAmount 即与控制台「应付」同量级（抹零已含在账期内，未必单独一行）。")
	fmt.Println("  • 若 OSS 行明细之和与上式相差约 0.1 USD，差额即账单级抹零；应以 API 本行 Pretax 或按产品分组汇总为准对账。")
	fmt.Println("Endpoint:", aliyun.ResolveBillingEndpointForEnv(strings.TrimSpace(*env)))
}

func strPtr(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func f32Ptr(p *float32) string {
	if p == nil {
		return "<nil>"
	}
	return fmt.Sprintf("%.6f", *p)
}
