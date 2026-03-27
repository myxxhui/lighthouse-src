// [Ref: 本地验证] 调用 FetchBillOverview（queryBillOverviewMerged）拉取 2025-10、2025-11 并与控制台支付明细对照。
// 用法: 在 lighthouse-deploy 目录执行 source .env && cd ../lighthouse-src && go run ./cmd/verify-fetch-bill
package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/myxxhui/lighthouse-src/internal/data/cloudbilling/aliyun"
)

func main() {
	if os.Getenv("ALIBABA_CLOUD_ACCESS_KEY_ID_POC") == "" || os.Getenv("ALIBABA_CLOUD_ACCESS_KEY_SECRET_POC") == "" {
		fmt.Println("请先 source .env（需 ALIBABA_CLOUD_ACCESS_KEY_ID_POC / _SECRET_POC）")
		os.Exit(1)
	}
	f, ok := aliyun.NewFetcherForEnv("POC")
	if !ok {
		fmt.Println("NewFetcherForEnv(POC) 失败，请检查 POC 凭证与 CLOUD_BILLING_ENDPOINT")
		os.Exit(1)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	fmt.Println("=== FetchBillOverview (queryBillOverviewMerged) 2025-10 / 2025-11 ===")
	fmt.Println("对照：控制台 2025-10 预付费约 2148.59 USD，2025-11 预付费约 471.32 USD")
	fmt.Println()

	for _, cycle := range []string{"2025-10", "2025-11"} {
		// 完整 merged（Subscription + PayAsYouGo）
		res, err := f.FetchBillOverview(ctx, cycle)
		if err != nil {
			fmt.Printf("[%s] 拉取失败: %v\n", cycle, err)
			continue
		}
		fmt.Printf("--- %s (Merged 预付费+后付费) ---\n", cycle)
		fmt.Printf("  TotalAmount (Pretax/消耗):    %.2f\n", res.TotalAmount)
		fmt.Printf("  CashTotalAmount (支付/现金): %.2f\n", res.CashTotalAmount)
		fmt.Printf("  Currency: %s\n", res.Currency)
		if len(res.CashByCategory) > 0 {
			fmt.Println("  CashByCategory (支付):")
			for k, v := range res.CashByCategory {
				fmt.Printf("    %s: %.2f\n", k, v)
			}
		}
		fmt.Printf("  Items 数量: %d\n", len(res.Items))
		fmt.Println()
	}
	fmt.Println("结论: 若 CashTotalAmount 为净额(含退款冲正) 则可能为负；控制台预付费 tab 仅展示正数支付。")
	fmt.Println("      API 口径 = 支付 - 退款；控制台预付费 = 仅支付。两者口径不同属预期。")
}
