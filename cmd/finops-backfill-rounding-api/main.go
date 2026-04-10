// 历史账期：OSS 明细无抹零列时，用 QueryAccountBill（MONTHLY、整月一行）的 PretaxAmount 与库内 OSS 明细之和的差额，写入一条 API 抹零补单行。
// 后续 OSS 若已含抹零列，可与本行并存；若需以 OSS 为准可删除 item_code=FINOPS_API_ROUNDING_BACKFILL 行后重灌。
//
//	go run ./cmd/finops-backfill-rounding-api -cycle=2025-10 -env=C66_POC
//	go run ./cmd/finops-backfill-rounding-api -cycle=2025-10 -account=5823052810429629 -dry-run
//
// [Ref: 04_采集 §5.4 抹零、03_Phase6/01_FinOps]
package main

import (
	"context"
	"database/sql"
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
	"github.com/myxxhui/lighthouse-src/internal/data/postgres"
)

const (
	itemAPIRoundingBackfill = "FINOPS_API_ROUNDING_BACKFILL"
	sourceAPIRounding       = "queryaccountbill:MONTHLY"
)

func main() {
	cycle := flag.String("cycle", "", "账期 YYYY-MM（必填）")
	envCred := flag.String("credential-env", "C66_POC", "AK 后缀，与 ALIBABA_CLOUD_ACCESS_KEY_ID_* 一致")
	account := flag.String("account", "", "finops_billing_fact.account_id；空则从 cost_env_account_config 解析 POC")
	dry := flag.Bool("dry-run", false, "只打印不算写入")
	flag.Parse()
	if strings.TrimSpace(*cycle) == "" {
		fmt.Fprintln(os.Stderr, "需要 -cycle=YYYY-MM")
		os.Exit(1)
	}

	cfg := &config.Config{}
	if doc, err := config.LoadLighthouseDeployYAML(""); err == nil && doc != nil {
		config.ApplyLighthouseDeployYAML(cfg, doc)
	}
	fillPostgresFromEnv(cfg)
	if cfg.Postgres.Host == "" {
		fmt.Fprintln(os.Stderr, "需要 POSTGRES_HOST / PG_*")
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

	accountID := strings.TrimSpace(*account)
	if accountID == "" {
		accountID, err = resolvePOCAccountID(ctx, db)
		if err != nil {
			fmt.Fprintf(os.Stderr, "account_id: %v\n", err)
			os.Exit(1)
		}
		fmt.Fprintf(os.Stderr, "[info] account_id=%q (from cost_env_account_config POC/C66_POC)\n", accountID)
	}

	f, ok := aliyun.NewFetcherForEnv(strings.TrimSpace(*envCred))
	if !ok {
		fmt.Fprintf(os.Stderr, "NewFetcherForEnv(%q) 失败\n", *envCred)
		os.Exit(1)
	}

	items, err := f.FetchQueryAccountBillMonthlyItems(ctx, strings.TrimSpace(*cycle), false)
	if err != nil {
		fmt.Fprintf(os.Stderr, "QueryAccountBill: %v\n", err)
		os.Exit(1)
	}
	if len(items) == 0 {
		fmt.Fprintln(os.Stderr, "API 返回 0 条 MONTHLY 汇总，跳过")
		os.Exit(1)
	}
	var apiTotal float64
	for _, it := range items {
		if it.PretaxAmount != nil {
			apiTotal += float64(*it.PretaxAmount)
		}
	}

	ossSum, nRows, err := sumFinOpsExcludingRounding(ctx, db, *cycle, accountID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "query finops: %v\n", err)
		os.Exit(1)
	}
	if nRows == 0 {
		fmt.Fprintln(os.Stderr, "该账期该 account 无 OSS 明细行，不补抹零（请先 OSS 入库）")
		os.Exit(1)
	}

	var ossCSVRounding, ossCSVCoupon int
	_ = db.QueryRowContext(ctx,
		`SELECT COUNT(*)::int FROM finops_billing_fact WHERE billing_cycle=$1 AND account_id=$2
		 AND item_code='FINOPS_BILLING_ROUNDING' AND COALESCE(source_object,'') NOT LIKE 'queryaccountbill%'`,
		*cycle, accountID).Scan(&ossCSVRounding)
	_ = db.QueryRowContext(ctx,
		`SELECT COUNT(*)::int FROM finops_billing_fact WHERE billing_cycle=$1 AND account_id=$2
		 AND item_code='FINOPS_BILLING_COUPON_DEDUCTION' AND COALESCE(source_object,'') NOT LIKE 'queryaccountbill%'`,
		*cycle, accountID).Scan(&ossCSVCoupon)
	if ossCSVRounding > 0 || ossCSVCoupon > 0 {
		fmt.Fprintln(os.Stderr, "已存在 OSS 解析的抹零行（FINOPS_BILLING_ROUNDING）或优惠券抵扣行（FINOPS_BILLING_COUPON_DEDUCTION，非 API），跳过 API 补全。")
		os.Exit(0)
	}

	delta := apiTotal - ossSum
	fmt.Println("======== finops API 抹零差额补全（临时）========")
	fmt.Printf("账期:          %s\n", *cycle)
	fmt.Printf("account_id:    %s\n", accountID)
	fmt.Printf("API Pretax 合计: %.6f (QueryAccountBill MONTHLY, IsGroupByProduct=false)\n", apiTotal)
	fmt.Printf("OSS 明细之和:    %.6f (已排除抹零/优惠券汇总行与 API 补全行, 行数=%d)\n", ossSum, nRows)
	fmt.Printf("差额 delta:     %.6f  (将写入一条补单行 amount=delta)\n", delta)

	if math.Abs(delta) < 1e-6 {
		fmt.Println("差额可忽略，不写库。")
		os.Exit(0)
	}

	cur := "USD"
	if r := firstCurrencyInCycle(ctx, db, *cycle, accountID); r != "" {
		cur = r
	}

	ud := lastDayOfCycle(*cycle)
	dedup := "API_ROUNDING_BACKFILL|" + *cycle + "|" + accountID
	row := postgres.FinOpsBillingFactRow{
		BillingCycle: *cycle,
		UsageDate:    ud,
		AccountAlias: "",
		AccountID:    accountID,
		Env:          "UNTAGGED",
		ProductCode:  "ROUNDING",
		InstanceID:   "",
		ItemCode:     itemAPIRoundingBackfill,
		Amount:       delta,
		Currency:     cur,
		TagsJSON:     nil,
		SourceObject: sourceAPIRounding,
		DedupKey:     dedup,
	}

	if *dry {
		fmt.Println("[dry-run] 未写入")
		os.Exit(0)
	}

	repo, err := postgres.NewPGRepository(cfg.Postgres)
	if err != nil {
		fmt.Fprintf(os.Stderr, "repo: %v\n", err)
		os.Exit(1)
	}
	defer repo.Close()
	if err := repo.BulkInsertFinOpsBillingFacts(ctx, []postgres.FinOpsBillingFactRow{row}); err != nil {
		fmt.Fprintf(os.Stderr, "upsert: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("已 UPSERT finops_billing_fact 一条（dedup_key=", dedup, "）")
}

func sumFinOpsExcludingRounding(ctx context.Context, db *sql.DB, cycle, accountID string) (sum float64, rowCount int, err error) {
	q := `SELECT COALESCE(SUM(amount), 0), COUNT(*)::bigint
FROM finops_billing_fact
WHERE billing_cycle = $1 AND COALESCE(account_id,'') = $2
  AND COALESCE(item_code,'') NOT IN ('FINOPS_BILLING_ROUNDING', 'FINOPS_API_ROUNDING_BACKFILL', 'FINOPS_BILLING_COUPON_DEDUCTION')`
	err = db.QueryRowContext(ctx, q, cycle, accountID).Scan(&sum, &rowCount)
	return
}

func firstCurrencyInCycle(ctx context.Context, db *sql.DB, cycle, accountID string) string {
	var cur sql.NullString
	_ = db.QueryRowContext(ctx,
		`SELECT currency FROM finops_billing_fact WHERE billing_cycle=$1 AND account_id=$2 AND COALESCE(currency,'')<>'' LIMIT 1`,
		cycle, accountID).Scan(&cur)
	if cur.Valid && cur.String != "" {
		return strings.ToUpper(strings.TrimSpace(cur.String))
	}
	return ""
}

func lastDayOfCycle(cycle string) time.Time {
	t, err := time.ParseInLocation("2006-01", strings.TrimSpace(cycle), time.UTC)
	if err != nil {
		return time.Now().UTC().Truncate(24 * time.Hour)
	}
	return t.AddDate(0, 1, -1)
}

func resolvePOCAccountID(ctx context.Context, db *sql.DB) (string, error) {
	q := `SELECT account_id FROM cost_env_account_config
WHERE environment IN ('C66_POC', 'POC')
ORDER BY CASE environment WHEN 'C66_POC' THEN 0 WHEN 'POC' THEN 1 ELSE 2 END
LIMIT 1`
	var aid string
	err := db.QueryRowContext(ctx, q).Scan(&aid)
	if err == sql.ErrNoRows {
		return "", fmt.Errorf("cost_env_account_config 无 POC/C66_POC")
	}
	if err != nil {
		return "", err
	}
	aid = strings.TrimSpace(aid)
	if aid == "" {
		return "", fmt.Errorf("POC account_id 为空")
	}
	return aid, nil
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
