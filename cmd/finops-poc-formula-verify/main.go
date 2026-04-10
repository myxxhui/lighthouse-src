// 临时验证程序：核对「应付 Y / 回帐 H / 净额 N / 实付 S」；支持 OSS（finops_billing_fact）或 API（cost_cloud_bill_line_items）。
// API 模式：可调阿里云汇总 DeductedByCoupons+DeductedByCashCoupons，做 N = SUM(pretax) − C。
// 若 ETL 已按控制台写入「券后」PretaxAmount（与单条 PretaxGross−InvoiceDiscount−DeductedByCoupons 一致），则不要再减 C，否则重复扣券；此时以 -no-coupon-api 或只看 OSS/MONTHLY 对账为准。
//
//	go run ./cmd/finops-poc-formula-verify
//	go run ./cmd/finops-poc-formula-verify -source=api -cycle=2025-12 -env=C66_POC
//	go run ./cmd/finops-poc-formula-verify -source=api -no-coupon-api   // 仅库内 API 行，不调阿里云拉券
//	go run ./cmd/finops-poc-formula-verify -source=oss -no-oss-cash      // 不调 OSS 累加「信用卡实扣」
//	go run ./cmd/finops-poc-formula-verify -source=oss -split-envs=C66_POC,C66_UAT   // 按环境分别汇总 Y/H/N…（与单账号 -account-env 对照）
//	go run ./cmd/finops-poc-formula-verify -source=oss -all-envs          // 从 cost_env_account_config 拉全部 environment 分别汇总
//
// 与现网 FinOps 对齐的三口径（本程序文本报告「三口径」块）：应付消耗=正额行和（Y）、回血=负额行和（H）、实付=控制台已还款同源（BSS Payment+Expense，不足则 cost_cloud_bill_monthly_raw 月表现金）。[Ref: 03_Phase6/01_FinOps] [Ref: cost_service mergeFinanceLedgerPUB / paidPForAccount]
//
// [Ref: 03_Phase6/01_FinOps 临时公式验证 POC] [Ref: 04_采集 §5.4 优惠券]
package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"math"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/myxxhui/lighthouse-src/internal/config"
	"github.com/myxxhui/lighthouse-src/internal/data/cloudbilling/aliyun"
	"github.com/myxxhui/lighthouse-src/internal/data/ossfinops"
)

// FormulaResult 公式输出（便于 -json 与对账）
type FormulaResult struct {
	Source       string  `json:"data_source"`
	BillingCycle string  `json:"billing_cycle"`
	AccountID    string  `json:"account_id"`
	Environment  string  `json:"environment,omitempty"`
	RowCount     int64   `json:"row_count"`
	YingFu       float64 `json:"应付_Y_正数合计"`
	HuiZhang     float64 `json:"回帐_H_负数合计"`
	NetBefore    float64 `json:"扣券前净额_Y加H或SUM行金额"`
	CouponDeduct float64 `json:"优惠券抵扣_阿里云API,omitempty"`
	JingE        float64  `json:"净额_N"`
	ShiFu        float64  `json:"实付_S"`
	// ShiFuYiHuanKuan 与 API 环境卡 ledger_p 同源：BSS Payment+Expense 区间绝对值之和；BSS 为 0 时回退月表现金列（mergeFinanceLedgerPUB 同序）。[Ref: 03_Phase6/01_FinOps]
	ShiFuYiHuanKuan float64 `json:"实付_控制台已还款_BSS或月表现金"`
	// ChongZhi 自然月内 BSS 流水充值：transaction_flow=Income 且 amount>0（含支付宝/余额等入金）。[Ref: 03_Phase6/01_FinOps]
	ChongZhi float64 `json:"充值"`
	// XinYongKaShiKou 仅 -source=oss 且从 OSS CSV 累加「现金支付金额/Cash Payment」列；API 模式为 nil。[Ref: 03_Phase6/01_FinOps]
	XinYongKaShiKou *float64 `json:"信用卡实扣,omitempty"`
	FormulaNote     string   `json:"formula_note"`
	Round2          bool     `json:"金额已四舍五入到2位,omitempty"`
}

func main() {
	source := flag.String("source", "oss", "数据源: oss=finops_billing_fact.amount；api=cost_cloud_bill_line_items.pretax_amount（ingestion_channel=api_query_account_bill）")
	cycle := flag.String("cycle", "2025-11", "账期 YYYY-MM")
	account := flag.String("account", "", "account_id；空则解析 POC（C66_POC / POC）")
	envCred := flag.String("env", "C66_POC", "拉取优惠券抵扣时 NewFetcherForEnv 的环境后缀（与 ALIBABA_CLOUD_ACCESS_KEY_ID_* 一致）")
	accountEnv := flag.String("account-env", "C66_POC", "当 -account 为空时，从 cost_env_account_config.environment 解析 account_id（如 C66_UAT）")
	noCouponAPI := flag.Bool("no-coupon-api", false, "仅 API 源有效：不调阿里云，优惠券抵扣记 0")
	noOssCash := flag.Bool("no-oss-cash", false, "仅 OSS 源有效：不从 OSS 重扫 CSV 累加现金支付列（信用卡实扣）")
	asJSON := flag.Bool("json", false, "仅输出一行 JSON；-split-envs/-all-envs 时为多段 JSON")
	round2 := flag.Bool("round2", true, "Y/H/N/S 及券等金额四舍五入到 2 位小数再输出（与控制台 USD 分位展示对齐比较）")
	splitEnvs := flag.String("split-envs", "", "逗号分隔多个 environment，分别汇总（如 C66_POC,C66_UAT）；与单账号模式互斥")
	allEnvs := flag.Bool("all-envs", false, "从 cost_env_account_config 取全部有 account_id 的 environment 分别汇总")
	flag.Parse()

	cfg := &config.Config{}
	if doc, err := config.LoadLighthouseDeployYAML(""); err == nil && doc != nil {
		config.ApplyLighthouseDeployYAML(cfg, doc)
	}
	fillPostgresFromEnv(cfg)
	if cfg.Postgres.Host == "" {
		fmt.Fprintln(os.Stderr, "需要 POSTGRES_HOST / PG_* 或统一部署 YAML 中 postgres")
		os.Exit(1)
	}

	dsn := buildDSN(cfg.Postgres)
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		fmt.Fprintf(os.Stderr, "open db: %v\n", err)
		os.Exit(1)
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "ping: %v\n", err)
		os.Exit(1)
	}

	envList, err := buildEnvListForSplit(ctx, db, strings.TrimSpace(*splitEnvs), *allEnvs, strings.TrimSpace(*account), strings.TrimSpace(*accountEnv))
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
	if len(envList) > 1 || (len(envList) == 1 && envList[0].modeSplit) {
		runSplitMode(ctx, db, envList, *source, *cycle, strings.TrimSpace(*envCred), *noOssCash, *noCouponAPI, *round2, *asJSON)
		return
	}

	accountID := strings.TrimSpace(*account)
	accountEnvKey := strings.TrimSpace(*accountEnv)
	if accountID == "" {
		aid, err := resolveAccountIDForEnv(ctx, db, accountEnvKey)
		if err != nil {
			fmt.Fprintf(os.Stderr, "解析 account_id（-account-env=%s）: %v（可用 -account= 显式指定）\n", accountEnvKey, err)
			os.Exit(1)
		}
		accountID = aid
		fmt.Fprintf(os.Stderr, "[info] account_id=%q（environment=%q）\n", accountID, accountEnvKey)
	}

	var res *FormulaResult
	switch strings.ToLower(strings.TrimSpace(*source)) {
	case "oss":
		res, err = aggregateOSS(ctx, db, *cycle, accountID, strings.TrimSpace(*envCred), *noOssCash, accountEnvKey)
	case "api":
		res, err = aggregateAPI(ctx, db, *cycle, accountID, strings.TrimSpace(*envCred), *noCouponAPI)
	default:
		fmt.Fprintf(os.Stderr, "-source 仅支持 oss|api，当前=%q\n", *source)
		os.Exit(1)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "查询: %v\n", err)
		os.Exit(1)
	}

	if err := attachConsolePaid(ctx, db, res.BillingCycle, res.AccountID, res); err != nil {
		fmt.Fprintf(os.Stderr, "[warn] 实付(已还款/BSS): %v\n", err)
	}
	finalizeResult(res, *round2)

	if *asJSON {
		b, _ := json.MarshalIndent(res, "", "  ")
		fmt.Println(string(b))
		return
	}

	printTextReport(res, *round2)
}

// envSplitItem 单环境拆分模式下的目标 environment（来自 -split-envs / -all-envs）
type envSplitItem struct {
	environment string
	modeSplit   bool // true 表示来自 -split-envs / -all-envs
}

func buildEnvListForSplit(ctx context.Context, db *sql.DB, splitCSV string, allEnvs bool, explicitAccount, defaultAccountEnv string) ([]envSplitItem, error) {
	if allEnvs && strings.TrimSpace(splitCSV) != "" {
		return nil, fmt.Errorf("-all-envs 与 -split-envs 不能同时使用")
	}
	if allEnvs {
		names, err := listConfiguredEnvironments(ctx, db)
		if err != nil {
			return nil, err
		}
		if len(names) == 0 {
			return nil, fmt.Errorf("-all-envs: cost_env_account_config 无可用 environment")
		}
		out := make([]envSplitItem, 0, len(names))
		for _, e := range names {
			out = append(out, envSplitItem{environment: e, modeSplit: true})
		}
		return out, nil
	}
	parts := strings.Split(splitCSV, ",")
	var names []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			names = append(names, p)
		}
	}
	if len(names) == 0 {
		return []envSplitItem{{environment: defaultAccountEnv, modeSplit: false}}, nil
	}
	if len(names) >= 2 && explicitAccount != "" {
		return nil, fmt.Errorf("多环境 -split-envs 时请省略 -account=")
	}
	if len(names) == 1 && explicitAccount != "" {
		return nil, fmt.Errorf("单环境 -split-envs 时请省略 -account=，或不要与多环境混用")
	}
	out := make([]envSplitItem, 0, len(names))
	for _, e := range names {
		out = append(out, envSplitItem{environment: e, modeSplit: true})
	}
	return out, nil
}

func listConfiguredEnvironments(ctx context.Context, db *sql.DB) ([]string, error) {
	q := `SELECT DISTINCT environment FROM cost_env_account_config
WHERE COALESCE(TRIM(account_id), '') <> ''
ORDER BY environment`
	rows, err := db.QueryContext(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var names []string
	for rows.Next() {
		var e string
		if err := rows.Scan(&e); err != nil {
			return nil, err
		}
		e = strings.TrimSpace(e)
		if e != "" {
			names = append(names, e)
		}
	}
	return names, rows.Err()
}

func runSplitMode(ctx context.Context, db *sql.DB, items []envSplitItem, source, cycle, envCred string, noOssCash, noCouponAPI bool, round2, asJSON bool) {
	var results []*FormulaResult
	var sumH, sumY, sumN, sumPaid float64
	for i := range items {
		envName := strings.TrimSpace(items[i].environment)
		aid, err := resolveAccountIDForEnv(ctx, db, envName)
		if err != nil {
			fmt.Fprintf(os.Stderr, "[skip] environment=%q: %v\n", envName, err)
			continue
		}
		fmt.Fprintf(os.Stderr, "[info] [%d/%d] environment=%q account_id=%q\n", i+1, len(items), envName, aid)
		// OSS 现金列为周期级桶累加，多环境时仅首段拉取，避免每段重复同一总额
		skipOssThis := noOssCash
		if strings.EqualFold(strings.TrimSpace(source), "oss") && i > 0 && !noOssCash {
			skipOssThis = true
		}
		var res *FormulaResult
		var errAgg error
		switch strings.ToLower(strings.TrimSpace(source)) {
		case "oss":
			res, errAgg = aggregateOSS(ctx, db, cycle, aid, envCred, skipOssThis, envName)
		case "api":
			res, errAgg = aggregateAPI(ctx, db, cycle, aid, envCred, noCouponAPI)
		default:
			fmt.Fprintf(os.Stderr, "-source 仅支持 oss|api\n")
			os.Exit(1)
		}
		if errAgg != nil {
			fmt.Fprintf(os.Stderr, "[skip] environment=%q: 查询 %v\n", envName, errAgg)
			continue
		}
		if err := attachConsolePaid(ctx, db, res.BillingCycle, res.AccountID, res); err != nil {
			fmt.Fprintf(os.Stderr, "[warn] environment=%q 实付(BSS): %v\n", envName, err)
		}
		finalizeResult(res, round2)
		res.Environment = envName
		if strings.EqualFold(strings.TrimSpace(source), "oss") && i > 0 && !noOssCash {
			res.FormulaNote += "；多环境拆分：信用卡实扣仅在首段展示（全周期 OSS 列累加，非按账号拆分）。"
		}
		results = append(results, res)
		sumH += res.HuiZhang
		sumY += res.YingFu
		sumN += res.JingE
		sumPaid += res.ShiFuYiHuanKuan
	}
	if len(results) == 0 {
		fmt.Fprintln(os.Stderr, "无成功汇总的 environment，退出")
		os.Exit(1)
	}
	if round2 {
		sumH = roundMoney2(sumH)
		sumY = roundMoney2(sumY)
		sumN = roundMoney2(sumN)
		sumPaid = roundMoney2(sumPaid)
	}
	if asJSON {
		out := map[string]interface{}{
			"per_environment": results,
			"totals": map[string]float64{
				"应付_Y_正数合计":              sumY,
				"回帐_H_负数合计":              sumH,
				"净额_N_合计":                sumN,
				"实付_控制台已还款_合计": sumPaid,
			},
			"formula_note": "各段：Y/H 为 finops/API 正负额；实付_控制台已还款 为 BSS+月表现金（与现网 API 环境卡 ledger_p 同源）。totals 为各环境简单加总。",
		}
		b, _ := json.MarshalIndent(out, "", "  ")
		fmt.Println(string(b))
		return
	}
	fmt.Println("======== finops 公式验证（按环境分开，临时程序）========")
	fmt.Printf("数据源: %s  账期: %s\n", source, cycle)
	fmt.Println()
	for _, res := range results {
		printTextReportSection(res, round2)
		fmt.Println()
	}
	fmt.Println("-------- 各环境简单加总（非前端 G 分摊口径）--------")
	prec := "%.6f\n"
	if round2 {
		prec = "%.2f\n"
	}
	fmt.Printf("Σ 应付 Y:     "+prec, sumY)
	fmt.Printf("Σ 回帐 H:     "+prec, sumH)
	fmt.Printf("Σ 净额 N:     "+prec, sumN)
	fmt.Printf("Σ 实付(已还款): "+prec, sumPaid)
	fmt.Println("说明: 现网前端环境卡 G/P/C 已改为按账号事实直填；本程序 Y/H/实付已还款 与之后端逻辑对齐。")
}

func finalizeResult(res *FormulaResult, round2 bool) {
	res.ShiFu = shiFuFromJingE(res.JingE)
	if round2 {
		applyRoundMoney2(res)
	}
	res.Round2 = round2
}

func printTextReportSection(res *FormulaResult, round2 bool) {
	var title string
	switch {
	case res.Environment != "":
		title = fmt.Sprintf("======== environment=%s (account_id=%s) ========", res.Environment, res.AccountID)
	default:
		title = "======== finops 公式验证（临时程序）========"
	}
	fmt.Println(title)
	fmt.Printf("数据源:     %s\n", res.Source)
	fmt.Printf("账期:       %s\n", res.BillingCycle)
	fmt.Printf("account_id: %s\n", res.AccountID)
	fmt.Printf("行数:       %d\n", res.RowCount)
	if round2 {
		if res.Environment != "" {
			fmt.Println("金额粒度:   四舍五入 2 位小数")
		} else {
			fmt.Println("金额粒度:   四舍五入 2 位小数（-round2=false 可输出全精度）")
		}
	}
	fmt.Println()
	prec := "%.6f\n"
	if round2 {
		prec = "%.2f\n"
	}
	fmt.Println("--- 三口径（与现网 API 环境卡 consumption_cost / ledger_g / ledger_p 同源）---")
	fmt.Printf("应付消耗（正额叠加）:     "+prec, res.YingFu)
	fmt.Printf("回血（负额叠加）:         "+prec, res.HuiZhang)
	fmt.Printf("实付（控制台已还款）:     "+prec, res.ShiFuYiHuanKuan)
	fmt.Println("--- 其它（对账参考）---")
	fmt.Printf("应付 Y (正数行合计):     "+prec, res.YingFu)
	fmt.Printf("回帐 H (负数行合计):     "+prec, res.HuiZhang)
	fmt.Printf("扣券前净额 (Y+H):        "+prec, res.NetBefore)
	if strings.HasPrefix(res.Source, "api") {
		fmt.Printf("优惠券抵扣 C (API 月汇总): "+prec, res.CouponDeduct)
		fmt.Printf("净额 N (扣券前−C):       "+prec, res.JingE)
	} else {
		fmt.Printf("净额 N (=SUM):           "+prec, res.JingE)
	}
	fmt.Printf("校验 Y+H:                "+prec, res.YingFu+res.HuiZhang)
	fmt.Printf("实付 S (旧规则 N<0→0):   "+prec, res.ShiFu)
	fmt.Printf("充值 (BSS Income+):      "+prec, res.ChongZhi)
	if res.XinYongKaShiKou != nil {
		fmt.Printf("信用卡实扣 (OSS CSV 现金支付列累加): "+prec, *res.XinYongKaShiKou)
	}
	fmt.Println()
	fmt.Println(res.FormulaNote)
}

func aggregateOSS(ctx context.Context, db *sql.DB, cycle, accountID, ossCredentialEnv string, skipOssCash bool, accountEnvKey string) (*FormulaResult, error) {
	ids := finopsAccountIDsForQuery(accountID, accountEnvKey)
	ph := make([]string, len(ids))
	args := make([]interface{}, 0, 1+len(ids))
	args = append(args, cycle)
	for i, id := range ids {
		ph[i] = fmt.Sprintf("$%d", i+2)
		args = append(args, id)
	}
	q := fmt.Sprintf(`SELECT
  COUNT(*)::bigint,
  COALESCE(SUM(CASE WHEN amount > 0 THEN amount END), 0),
  COALESCE(SUM(CASE WHEN amount < 0 THEN amount END), 0),
  COALESCE(SUM(amount), 0)
FROM finops_billing_fact
WHERE billing_cycle = $1 AND COALESCE(account_id,'') IN (%s)`, strings.Join(ph, ","))
	var r FormulaResult
	r.Source = "oss_finops_billing_fact"
	r.BillingCycle = cycle
	r.AccountID = accountID
	var net float64
	err := db.QueryRowContext(ctx, q, args...).Scan(&r.RowCount, &r.YingFu, &r.HuiZhang, &net)
	if err != nil {
		return nil, err
	}
	r.NetBefore = net
	r.CouponDeduct = 0
	r.JingE = net
	r.FormulaNote = "OSS: N=SUM(amount)；若 CSV 已含 FINOPS_BILLING_COUPON_DEDUCTION 负行则券已计入 N。实付 S=N<0?0:N"
	if rch, err := sumBssRechargeIncomeForCalendarMonth(ctx, db, cycle, accountID); err != nil {
		fmt.Fprintf(os.Stderr, "[warn] 充值汇总(cost_bss_transactions): %v\n", err)
		r.FormulaNote += "；充值：读库失败。"
	} else {
		r.ChongZhi = rch
		r.FormulaNote += "；充值：同自然月 BSS 流水 Income 且 amount>0 之和。"
	}
	if !skipOssCash {
		if cfg, ok := ossfinops.ConfigFromEnv(ossCredentialEnv); ok {
			cctx, cancel := context.WithTimeout(ctx, 180*time.Second)
			defer cancel()
			cashSum, errC := ossfinops.SumCashPaymentFromOSS(cctx, cfg, strings.TrimSpace(cycle))
			if errC != nil {
				fmt.Fprintf(os.Stderr, "[warn] OSS 现金支付列累加失败: %v（信用卡实扣未填）\n", errC)
				r.FormulaNote += "；信用卡实扣：OSS 列累加失败。"
			} else {
				v := cashSum
				r.XinYongKaShiKou = &v
				r.FormulaNote += "；信用卡实扣：OSS CSV「现金支付金额/Cash Payment」列行级累加（与 finops.amount 独立）。"
			}
		} else {
			r.FormulaNote += "；信用卡实扣：未配置 OSS_BILLING_BUCKET 等，跳过 OSS 列累加。"
		}
	} else {
		r.FormulaNote += "；信用卡实扣：已 -no-oss-cash 跳过。"
	}
	return &r, nil
}

// finopsAccountIDsForQuery finops_billing_fact.account_id 可能与 cost_env_account_config.account_id（阿里云数字）不一致（如存 UAT 短名）；同时按数字与 C66_* 短名匹配。[Ref: 03_Phase6/01_FinOps]
func finopsAccountIDsForQuery(canonicalAccountID, accountEnvKey string) []string {
	seen := make(map[string]bool)
	var out []string
	add := func(s string) {
		s = strings.TrimSpace(s)
		if s == "" || seen[s] {
			return
		}
		seen[s] = true
		out = append(out, s)
	}
	add(canonicalAccountID)
	ek := strings.TrimSpace(accountEnvKey)
	if strings.HasPrefix(strings.ToUpper(ek), "C66_") {
		add(strings.TrimPrefix(strings.TrimPrefix(ek, "C66_"), "c66_"))
	}
	add(ek)
	return out
}

func aggregateAPI(ctx context.Context, db *sql.DB, cycle, accountID, envCred string, noCouponAPI bool) (*FormulaResult, error) {
	q := `SELECT
  COUNT(*)::bigint,
  COALESCE(SUM(CASE WHEN COALESCE(pretax_amount,0) > 0 THEN pretax_amount END), 0),
  COALESCE(SUM(CASE WHEN COALESCE(pretax_amount,0) < 0 THEN pretax_amount END), 0),
  COALESCE(SUM(COALESCE(pretax_amount,0)), 0)
FROM cost_cloud_bill_line_items
WHERE billing_cycle = $1 AND COALESCE(account_id,'') = $2
  AND COALESCE(ingestion_channel,'') = 'api_query_account_bill'`
	var r FormulaResult
	r.Source = "api_cost_cloud_bill_line_items"
	r.BillingCycle = cycle
	r.AccountID = accountID
	var netBefore float64
	err := db.QueryRowContext(ctx, q, cycle, accountID).Scan(&r.RowCount, &r.YingFu, &r.HuiZhang, &netBefore)
	if err != nil {
		return nil, err
	}
	r.NetBefore = netBefore
	r.CouponDeduct = 0
	if !noCouponAPI {
		cctx, ccancel := context.WithTimeout(ctx, 90*time.Second)
		defer ccancel()
		coupon, err := sumCouponDeductionFromAliyunAPI(cctx, cycle, envCred)
		if err != nil {
			fmt.Fprintf(os.Stderr, "[warn] 阿里云 QueryAccountBill 拉取优惠券抵扣失败: %v（N 暂按扣券前净额）\n", err)
		} else {
			r.CouponDeduct = coupon
		}
	}
	// N = 扣券前 SUM(pretax) − 当月券抵扣（与控制台「目录−优惠−券−抹零」中券项对齐；若行级 Pretax 已含券后值，请对照差值避免双扣）
	r.JingE = r.NetBefore - r.CouponDeduct
	r.FormulaNote = "API: 扣券前净额=SUM(pretax)；N=扣券前净额−阿里云MONTHLY汇总的(DeductedByCoupons+DeductedByCashCoupons)。若 ETL 已写入券后 Pretax，N 可能偏小，以对账说明为准。S=N<0?0:N"
	if rch, err := sumBssRechargeIncomeForCalendarMonth(ctx, db, cycle, accountID); err != nil {
		fmt.Fprintf(os.Stderr, "[warn] 充值汇总(cost_bss_transactions): %v\n", err)
		r.FormulaNote += "；充值：读库失败。"
	} else {
		r.ChongZhi = rch
		r.FormulaNote += "；充值：同自然月 BSS 流水 Income 且 amount>0 之和。"
	}
	return &r, nil
}

// billingCycleMonthBoundsUTC 账期 YYYY-MM 对应自然月 [start,end] UTC，供 BSS transaction_time 过滤。[Ref: 03_Phase6/01_FinOps]
func billingCycleMonthBoundsUTC(yyyyMM string) (start, end time.Time, err error) {
	t, err := time.Parse("2006-01", strings.TrimSpace(yyyyMM))
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	start = time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, time.UTC)
	end = start.AddDate(0, 1, 0).Add(-time.Nanosecond)
	return start, end, nil
}

// sumBssPaymentExpenseInRange 与 PGRepository.SumBSSPaymentExpenseByDateRange 一致：Payment+Expense 绝对值之和。[Ref: 03_Phase6/01_FinOps]
func sumBssPaymentExpenseInRange(ctx context.Context, db *sql.DB, from, to time.Time, accountID string) (float64, error) {
	fromStr := from.UTC().Format("2006-01-02 15:04:05")
	toStr := to.UTC().Format("2006-01-02 15:04:05")
	q := `SELECT COALESCE(SUM(ABS(amount)), 0)::float8 FROM cost_bss_transactions
WHERE transaction_time >= $1::timestamp AND transaction_time <= $2::timestamp
  AND LOWER(COALESCE(transaction_type,'')) = 'payment' AND LOWER(COALESCE(transaction_flow,'')) = 'expense'
  AND COALESCE(account_id,'') = $3`
	var v float64
	err := db.QueryRowContext(ctx, q, fromStr, toStr, strings.TrimSpace(accountID)).Scan(&v)
	return v, err
}

// sumMonthlyCashForCycle 与 SumMonthlyCashTotalForBillingCycles 单账期单账号。[Ref: 03_Phase6/01_FinOps]
func sumMonthlyCashForCycle(ctx context.Context, db *sql.DB, billingCycle, accountID string) (float64, error) {
	q := `SELECT COALESCE(SUM(cash_total_amount), 0)::float8 FROM cost_cloud_bill_monthly_raw
WHERE billing_cycle = $1 AND COALESCE(account_id,'') = $2`
	var v float64
	err := db.QueryRowContext(ctx, q, strings.TrimSpace(billingCycle), strings.TrimSpace(accountID)).Scan(&v)
	return v, err
}

// attachConsolePaid 写入实付_控制台已还款（BSS 优先，0 则月表现金）。[Ref: 03_Phase6/01_FinOps]
func attachConsolePaid(ctx context.Context, db *sql.DB, billingCycle, canonicalAccountID string, r *FormulaResult) error {
	aid := strings.TrimSpace(canonicalAccountID)
	if aid == "" {
		return nil
	}
	start, end, err := billingCycleMonthBoundsUTC(billingCycle)
	if err != nil {
		return err
	}
	p, errP := sumBssPaymentExpenseInRange(ctx, db, start, end, aid)
	if errP != nil {
		r.FormulaNote += "；实付(已还款)：读 cost_bss_transactions 失败。"
		return errP
	}
	pCash, errCash := sumMonthlyCashForCycle(ctx, db, billingCycle, aid)
	if errCash != nil {
		pCash = 0
	}
	chosen := p
	note := "；实付(已还款)：BSS Payment+Expense 区间合计（与后端 paidPForAccount 同源）。"
	if math.Abs(p) < 1e-9 && math.Abs(pCash) > 1e-9 {
		chosen = pCash
		note = "；实付(已还款)：BSS 为 0，已用 cost_cloud_bill_monthly_raw.cash_total_amount。"
	} else if math.Abs(chosen) < 1e-9 {
		note = "；实付(已还款)：BSS 与月表现金均为 0（可检查流水是否同步）。"
	}
	r.ShiFuYiHuanKuan = chosen
	r.FormulaNote += note
	return nil
}

func sumBssRechargeIncomeForCalendarMonth(ctx context.Context, db *sql.DB, calendarMonthYYYYMM, accountID string) (float64, error) {
	q := `SELECT COALESCE(SUM(amount), 0)::float8 FROM cost_bss_transactions
WHERE COALESCE(account_id,'') = $1
  AND to_char(transaction_time AT TIME ZONE 'UTC', 'YYYY-MM') = $2
  AND LOWER(COALESCE(transaction_flow,'')) = 'income'
  AND amount > 0`
	var v float64
	err := db.QueryRowContext(ctx, q, strings.TrimSpace(accountID), strings.TrimSpace(calendarMonthYYYYMM)).Scan(&v)
	return v, err
}

func sumCouponDeductionFromAliyunAPI(ctx context.Context, billingCycle, env string) (float64, error) {
	f, ok := aliyun.NewFetcherForEnv(env)
	if !ok {
		return 0, fmt.Errorf("NewFetcherForEnv(%q) 失败，请配置 ALIBABA_CLOUD_ACCESS_KEY_ID_%s", env, env)
	}
	return f.SumCouponDeductionForBillingCycle(ctx, strings.TrimSpace(billingCycle))
}

func shiFuFromJingE(jingE float64) float64 {
	if jingE < 0 {
		return 0
	}
	return jingE
}

// roundMoney2 四舍五入到 2 位小数（与控制台货币展示同粒度，便于对账）。
func roundMoney2(x float64) float64 {
	return math.Round(x*100) / 100
}

// applyRoundMoney2 对 Y/H/净额/券/N/S 统一取 2 位；S 在 JingE 取 2 位后再按 N<0→0。
func applyRoundMoney2(res *FormulaResult) {
	res.YingFu = roundMoney2(res.YingFu)
	res.HuiZhang = roundMoney2(res.HuiZhang)
	res.NetBefore = roundMoney2(res.NetBefore)
	res.CouponDeduct = roundMoney2(res.CouponDeduct)
	res.JingE = roundMoney2(res.JingE)
	res.ShiFu = roundMoney2(shiFuFromJingE(res.JingE))
	res.ShiFuYiHuanKuan = roundMoney2(res.ShiFuYiHuanKuan)
	res.ChongZhi = roundMoney2(res.ChongZhi)
	if res.XinYongKaShiKou != nil {
		x := roundMoney2(*res.XinYongKaShiKou)
		res.XinYongKaShiKou = &x
	}
	if res.FormulaNote != "" {
		res.FormulaNote += "；输出已 round2。"
	}
}

func resolveAccountIDForEnv(ctx context.Context, db *sql.DB, environment string) (string, error) {
	env := strings.TrimSpace(environment)
	if env == "" {
		return "", fmt.Errorf("account-env 为空")
	}
	candidates := []string{env}
	if strings.HasPrefix(strings.ToUpper(env), "C66_") {
		candidates = append(candidates, strings.TrimPrefix(env, "C66_"), strings.TrimPrefix(env, "c66_"))
	}
	q := `SELECT account_id FROM cost_env_account_config WHERE environment = $1 LIMIT 1`
	for _, c := range candidates {
		c = strings.TrimSpace(c)
		if c == "" {
			continue
		}
		var aid string
		err := db.QueryRowContext(ctx, q, c).Scan(&aid)
		if err != nil && err != sql.ErrNoRows {
			return "", err
		}
		if err == nil {
			aid = strings.TrimSpace(aid)
			if aid != "" {
				return aid, nil
			}
		}
	}
	return "", fmt.Errorf("cost_env_account_config 无 environment∈%v", candidates)
}

func printTextReport(res *FormulaResult, round2 bool) {
	printTextReportSection(res, round2)
}

func fillPostgresFromEnv(cfg *config.Config) {
	get := func(keys ...string) string {
		for _, k := range keys {
			if v := os.Getenv(k); v != "" {
				return v
			}
		}
		return ""
	}
	if v := get("PG_HOST", "POSTGRES_HOST"); v != "" {
		cfg.Postgres.Host = v
	}
	if v := get("PG_PORT", "POSTGRES_PORT"); v != "" {
		if p, err := strconv.Atoi(v); err == nil {
			cfg.Postgres.Port = p
		}
	}
	if v := get("PG_USER", "POSTGRES_USER"); v != "" {
		cfg.Postgres.User = v
	}
	if v := get("PG_PASSWORD", "POSTGRES_PASSWORD"); v != "" {
		cfg.Postgres.Password = v
	}
	if v := get("PG_DATABASE", "POSTGRES_DB"); v != "" {
		cfg.Postgres.Database = v
	}
	if v := get("PG_SSL_MODE"); v != "" {
		cfg.Postgres.SSLMode = v
	}
}

func buildDSN(cfg config.PostgresConfig) string {
	port := cfg.Port
	if port <= 0 {
		port = 5432
	}
	ssl := cfg.SSLMode
	if ssl == "" {
		ssl = "disable"
	}
	return fmt.Sprintf("postgres://%s:%s@%s:%d/%s?sslmode=%s",
		url.QueryEscape(cfg.User), url.QueryEscape(cfg.Password), cfg.Host, port, cfg.Database, ssl)
}
