// 只读诊断：直接调阿里云 QueryAccountTransactions（与 ETL 中 FetchBSSTransactions 同源），
// 按 TransactionChannel 汇总 amount，并单独给出「信用卡」渠道合计（用于区分 finops 里 OSS 现金列为 0 是「未配置列」还是「API 侧真无扣款」）。
// 默认优先国际站 BSS：先读 lighthouse-deploy YAML 的 bss_endpoint，否则在未指定 -endpoint 时可用 -intl（默认 true）落到 business.ap-southeast-1.aliyuncs.com；中国站请 -intl=false 或配置 CLOUD_BILLING_ENDPOINT_<env>。
//
//	go run ./cmd/bss-api-transactions-report -month=2026-01
//	go run ./cmd/bss-api-transactions-report -start=2026-01-01 -end=2026-01-31 -intl=false
//
// [Ref: 03_Phase6/01_FinOps QueryAccountTransactions]
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/myxxhui/lighthouse-src/internal/config"
	"github.com/myxxhui/lighthouse-src/internal/data/cloudbilling/aliyun"
)

func main() {
	month := flag.String("month", "", "YYYY-MM，整月 UTC 0:00 至月末最后一秒；与 -start/-end 互斥")
	startStr := flag.String("start", "", "开始 UTC 日期 YYYY-MM-DD（需与 -end 同用）")
	endStr := flag.String("end", "", "结束 UTC 日期 YYYY-MM-DD（含当日）")
	env := flag.String("env", "C66_POC", "AK 后缀，与 ALIBABA_CLOUD_ACCESS_KEY_ID_* 一致")
	endpointFlag := flag.String("endpoint", "", "强制 BSS 域名（如 business.ap-southeast-1.aliyuncs.com）；优先于 YAML 与 -intl")
	intl := flag.Bool("intl", true, "未指定 -endpoint 且 YAML bss_endpoint 为空时，使用国际站默认 BSS（PoC 临时流水）")
	flag.Parse()

	envKey := strings.TrimSpace(*env)
	doc, _ := config.LoadLighthouseDeployYAML("")
	ep := strings.TrimSpace(*endpointFlag)
	if ep == "" && doc != nil {
		ep = config.BSSEndpointForEnvironmentKey(doc, envKey)
	}
	if ep == "" && *intl {
		ep = aliyun.DefaultIntlBillingEndpoint
	}

	var start, end time.Time
	switch {
	case strings.TrimSpace(*month) != "":
		t, err := time.Parse("2006-01", strings.TrimSpace(*month))
		if err != nil {
			fmt.Fprintf(os.Stderr, "-month: %v\n", err)
			os.Exit(1)
		}
		start = time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, time.UTC)
		end = start.AddDate(0, 1, 0).Add(-time.Second)
	case strings.TrimSpace(*startStr) != "" && strings.TrimSpace(*endStr) != "":
		var err error
		start, err = time.ParseInLocation("2006-01-02", strings.TrimSpace(*startStr), time.UTC)
		if err != nil {
			fmt.Fprintf(os.Stderr, "-start: %v\n", err)
			os.Exit(1)
		}
		endDay, err := time.ParseInLocation("2006-01-02", strings.TrimSpace(*endStr), time.UTC)
		if err != nil {
			fmt.Fprintf(os.Stderr, "-end: %v\n", err)
			os.Exit(1)
		}
		if endDay.Before(start) {
			fmt.Fprintln(os.Stderr, "-end 必须 >= -start")
			os.Exit(1)
		}
		end = endDay.Add(24*time.Hour - time.Second)
	default:
		now := time.Now().UTC()
		start = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
		end = start.AddDate(0, 1, 0).Add(-time.Second)
	}

	fetcher, ok := aliyun.NewFetcherForEnvWithEndpoint(envKey, ep)
	if !ok {
		fmt.Fprintf(os.Stderr, "创建 BSS Fetcher 失败（env=%q，请配置 ALIBABA_CLOUD_ACCESS_KEY_ID_*）\n", envKey)
		os.Exit(1)
	}
	displayEndpoint := ep
	if displayEndpoint == "" {
		displayEndpoint = aliyun.ResolveBillingEndpointForEnv(envKey)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()

	items, err := fetcher.FetchBSSTransactions(ctx, start, end)
	if err != nil {
		fmt.Fprintf(os.Stderr, "FetchBSSTransactions: %v\n", err)
		os.Exit(1)
	}

	type agg struct {
		sum float64
		n   int
	}
	byChannel := make(map[string]*agg)
	var creditSum float64
	var creditN int

	for _, it := range items {
		ch := strings.TrimSpace(it.TransactionChannel)
		if ch == "" {
			ch = "(empty)"
		}
		if byChannel[ch] == nil {
			byChannel[ch] = &agg{}
		}
		byChannel[ch].sum += it.Amount
		byChannel[ch].n++
		if isCreditCardChannel(ch) {
			creditSum += it.Amount
			creditN++
		}
	}

	channels := make([]string, 0, len(byChannel))
	for k := range byChannel {
		channels = append(channels, k)
	}
	sort.Strings(channels)

	fmt.Println("======== QueryAccountTransactions（API 直连，非库表）========")
	fmt.Printf("环境凭证: %s\n", envKey)
	fmt.Printf("BSS endpoint: %s\n", displayEndpoint)
	fmt.Printf("CreateTime 区间(UTC): %s .. %s\n", start.Format(time.RFC3339), end.Format(time.RFC3339))
	fmt.Printf("返回条数: %d（分页拉全）\n", len(items))
	fmt.Println()
	fmt.Println("按 TransactionChannel 汇总 amount（有符号，与控制台资金流水一致；扣款多为负）:")
	for _, ch := range channels {
		a := byChannel[ch]
		fmt.Printf("  %-44s  n=%-6d  sum=%12.2f\n", ch, a.n, a.sum)
	}
	fmt.Println()
	fmt.Println("「信用卡」渠道启发式匹配: channel 含 credit（不区分大小写）或含「信用」")
	fmt.Printf("  匹配条数: %d\n", creditN)
	fmt.Printf("  amount 合计: %.2f\n", creditSum)
	fmt.Println()
	fmt.Println("说明: finops-poc-formula-verify 的「信用卡实扣」来自 OSS CSV 现金列，与上表无关；")
	fmt.Println("      上表为 BSS API 真实流水；若条数>0 则已拿到数据，合计可为负（净扣款）。")
}

func isCreditCardChannel(ch string) bool {
	c := strings.TrimSpace(ch)
	if c == "" || c == "(empty)" {
		return false
	}
	l := strings.ToLower(c)
	return strings.Contains(l, "credit") || strings.Contains(c, "信用")
}
