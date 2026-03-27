// 本地直连阿里云 BSS QueryAccountBalance（与 ETL FetchAccountBalanceSnapshot 同源）。
// 用法: cd lighthouse-deploy && set -a && source .env && set +a && cd ../lighthouse-src && go run ./cmd/verify-account-balance
//
// [Ref: 03_Phase6/01_FinOps QueryAccountBalance]
package main

import (
	"context"
	"fmt"
	"time"

	"github.com/myxxhui/lighthouse-src/internal/data/cloudbilling/aliyun"
)

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	for _, env := range []string{"POC", "UAT"} {
		fmt.Printf("=== %s QueryAccountBalance（本地可复现）===\n", env)
		fmt.Printf("  解析到的 BSS Endpoint: %s\n", aliyun.ResolveBillingEndpointForEnv(env))
		f, ok := aliyun.NewFetcherForEnv(env)
		if !ok {
			fmt.Printf("  跳过: 未配置 ALIBABA_CLOUD_ACCESS_KEY_ID_%s / _SECRET_%s\n\n", env, env)
			continue
		}
		diag, errD := f.DiagnosticsQueryAccountBalance(ctx)
		if errD != nil {
			fmt.Printf("  SDK/网络错误: %v\n\n", errD)
			continue
		}
		if diag != nil {
			fmt.Printf("  Body.Success: %v\n", diag.Success)
			fmt.Printf("  Body.Message: %q\n", diag.APIMessage)
			fmt.Printf("  Data 原始字段: AvailableAmount=%q AvailableCashAmount=%q CreditAmount=%q MybankCreditAmount=%q\n",
				diag.AvailableAmountRaw, diag.AvailableCashRaw, diag.CreditAmountRaw, diag.MybankCreditAmountRaw)
			fmt.Printf("  Data.Currency: %s\n", diag.Currency)
			fmt.Printf("  balanceFromQueryAccountBalanceData 解析值: %.6f\n", diag.ParsedByCode)
		}
		avail, cur, err := f.FetchAccountBalanceSnapshot(ctx)
		if err != nil {
			fmt.Printf("  FetchAccountBalanceSnapshot（含熔断）错误: %v\n\n", err)
			continue
		}
		fmt.Printf("  FetchAccountBalanceSnapshot 返回值: %.6f %s\n\n", avail, cur)
	}
	fmt.Println("说明: Diagnostics 为同一次 QueryAccountBalance 的 Body；FetchAccountBalanceSnapshot 与 ETL 一致（含熔断）。")
}
