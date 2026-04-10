// 历史回填：调用阿里云 QueryAccountTransactions，按时间区间分页拉取并 UPSERT cost_bss_transactions（资金流水，与账期/实例账单解耦）。
// 与日常 ETL SyncFinOpsAuxiliary 中 FINOPS_BSS_TRANSACTIONS_LOOKBACK_DAYS（默认 14 天）增量互补；支持 TransactionChannel（如 CreditCard）等对账字段。
// BSS 域名：-endpoint 或统一部署 YAML 中对应 environment_key 的 bss_endpoint；否则可用 -intl 使用国际站默认（与 bss-api-transactions-report 一致）。
//
//	cd lighthouse-src && set -a && source ../lighthouse-deploy/.env && set +a
//	go run ./cmd/bss-transactions-backfill -start=2025-01-01 -end=2026-03-31 -credential-env=C66_POC -intl
//	go run ./cmd/bss-transactions-backfill -start=2025-01-01 -end=2026-03-31 -dry-run
//
// 首次需执行 DB 迁移：lighthouse-deploy/scripts/migrate-08-bss-transaction-channel.sql
//
// [Ref: 03_Phase6/01_FinOps QueryAccountTransactions]
package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
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

func main() {
	startStr := flag.String("start", "", "开始日期 UTC，YYYY-MM-DD（必填）")
	endStr := flag.String("end", "", "结束日期 UTC，YYYY-MM-DD（含当日，必填）")
	chunkDays := flag.Int("chunk-days", 31, "单次 API 查询区间长度（天），过大可能被限流")
	credentialEnv := flag.String("credential-env", "C66_POC", "AK 后缀，与 ALIBABA_CLOUD_ACCESS_KEY_ID_* 一致")
	account := flag.String("account", "", "cost_bss_transactions.account_id；空则从 cost_env_account_config POC 解析")
	dryRun := flag.Bool("dry-run", false, "只打印区间与批次数，不写库")
	endpointFlag := flag.String("endpoint", "", "强制 BSS 域名；优先于 YAML 与 -intl")
	intl := flag.Bool("intl", false, "未指定 -endpoint 且 YAML bss_endpoint 为空时，使用国际站默认 BSS")
	flag.Parse()

	if strings.TrimSpace(*startStr) == "" || strings.TrimSpace(*endStr) == "" {
		fmt.Fprintln(os.Stderr, "需要 -start=YYYY-MM-DD -end=YYYY-MM-DD")
		os.Exit(1)
	}
	startT, err := time.ParseInLocation("2006-01-02", strings.TrimSpace(*startStr), time.UTC)
	if err != nil {
		fmt.Fprintf(os.Stderr, "-start: %v\n", err)
		os.Exit(1)
	}
	endDay, err := time.ParseInLocation("2006-01-02", strings.TrimSpace(*endStr), time.UTC)
	if err != nil {
		fmt.Fprintf(os.Stderr, "-end: %v\n", err)
		os.Exit(1)
	}
	if endDay.Before(startT) {
		fmt.Fprintln(os.Stderr, "-end 必须 >= -start")
		os.Exit(1)
	}
	endT := endDay.Add(24*time.Hour - time.Second)

	cfg := &config.Config{}
	var deployDoc *config.LighthouseDeployYAML
	if doc, err := config.LoadLighthouseDeployYAML(""); err == nil && doc != nil {
		deployDoc = doc
		config.ApplyLighthouseDeployYAML(cfg, doc)
	}
	fillPostgresFromEnv(cfg)

	credEnv := strings.TrimSpace(*credentialEnv)
	ep := strings.TrimSpace(*endpointFlag)
	if ep == "" && deployDoc != nil {
		ep = config.BSSEndpointForEnvironmentKey(deployDoc, credEnv)
	}
	if ep == "" && *intl {
		ep = aliyun.DefaultIntlBillingEndpoint
	}
	fetcher, ok := aliyun.NewFetcherForEnvWithEndpoint(credEnv, ep)
	if !ok {
		fmt.Fprintf(os.Stderr, "创建 BSS Fetcher 失败（credential-env=%q）\n", credEnv)
		os.Exit(1)
	}
	if ep != "" {
		fmt.Fprintf(os.Stderr, "[info] BSS endpoint=%q（QueryAccountTransactions 国际站/显式域名）\n", ep)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Hour)
	defer cancel()

	var repo *postgres.PGRepository
	var accountID string
	if !*dryRun {
		if cfg.Postgres.Host == "" {
			fmt.Fprintln(os.Stderr, "需要 POSTGRES_HOST / PG_*（-dry-run 可跳过）")
			os.Exit(1)
		}
		dsn := buildDSN(cfg.Postgres)
		db, err := sql.Open("pgx", dsn)
		if err != nil {
			fmt.Fprintf(os.Stderr, "open db: %v\n", err)
			os.Exit(1)
		}
		defer db.Close()
		if err := db.PingContext(ctx); err != nil {
			fmt.Fprintf(os.Stderr, "ping: %v\n", err)
			os.Exit(1)
		}
		repo, err = postgres.NewPGRepository(cfg.Postgres)
		if err != nil {
			fmt.Fprintf(os.Stderr, "repo: %v\n", err)
			os.Exit(1)
		}
		defer repo.Close()

		accountID = strings.TrimSpace(*account)
		if accountID == "" {
			accountID, err = resolveAccountIDForEnv(ctx, db, strings.TrimSpace(*credentialEnv))
			if err != nil {
				fmt.Fprintf(os.Stderr, "account_id: %v（可用 -account= 或核对 -credential-env 与 cost_env_account_config.environment）\n", err)
				os.Exit(1)
			}
			fmt.Fprintf(os.Stderr, "[info] account_id=%q (environment=%q)\n", accountID, strings.TrimSpace(*credentialEnv))
		}
	}

	chunk := time.Duration(*chunkDays) * 24 * time.Hour
	var batches int
	var totalRows int
	for t0 := startT; ; {
		t1 := t0.Add(chunk).Add(-time.Second)
		if t1.After(endT) {
			t1 = endT
		}
		batches++
		if *dryRun {
			fmt.Printf("[dry-run] batch %d: %s .. %s\n", batches, t0.UTC().Format(time.RFC3339), t1.UTC().Format(time.RFC3339))
		} else {
			items, err := fetcher.FetchBSSTransactions(ctx, t0, t1)
			if err != nil {
				fmt.Fprintf(os.Stderr, "FetchBSSTransactions %s..%s: %v\n", t0.Format(time.RFC3339), t1.Format(time.RFC3339), err)
				os.Exit(1)
			}
			for _, it := range items {
				row := postgres.BSSTransactionRow{
					TransactionNumber: it.TransactionNumber,
					AccountID:         accountID,
					TransactionTime:   it.TransactionTime,
					Amount:            it.Amount,
					TransactionType:   it.TransactionType,
					TransactionFlow:   it.TransactionFlow,
					RecordID:          it.RecordID,
					BillingCycle:      it.BillingCycle,
					Currency:          it.Currency,
					TransactionChannel: it.TransactionChannel,
					FundType:           it.FundType,
					Remarks:            it.Remarks,
				}
				if row.Currency == "" {
					row.Currency = "CNY"
				}
				if err := repo.UpsertBSSTransaction(ctx, row); err != nil {
					fmt.Fprintf(os.Stderr, "UpsertBSSTransaction %s: %v\n", row.TransactionNumber, err)
					os.Exit(1)
				}
			}
			totalRows += len(items)
			fmt.Printf("batch %d: %s .. %s → %d 条\n", batches, t0.Format("2006-01-02"), t1.Format("2006-01-02"), len(items))
		}
		if !t1.Before(endT) {
			break
		}
		t0 = t1.Add(time.Second)
	}
	if *dryRun {
		fmt.Printf("bss-transactions-backfill: dry-run 完成，共 %d 个区间\n", batches)
		return
	}
	fmt.Printf("bss-transactions-backfill: ok，共 %d 批次，累计 UPSERT %d 条\n", batches, totalRows)
	if err := repo.RefreshBSSRechargeMonthlyForAccount(ctx, accountID); err != nil {
		fmt.Fprintf(os.Stderr, "RefreshBSSRechargeMonthlyForAccount: %v\n", err)
		os.Exit(1)
	}
	fmt.Fprintf(os.Stderr, "[info] cost_bss_recharge_monthly 已按 BSS 流水重算（Income+）\n")
}

func resolveAccountIDForEnv(ctx context.Context, db *sql.DB, environment string) (string, error) {
	env := strings.TrimSpace(environment)
	if env == "" {
		return "", fmt.Errorf("credential-env 为空")
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
		if err == nil && strings.TrimSpace(aid) != "" {
			return strings.TrimSpace(aid), nil
		}
	}
	return "", fmt.Errorf("cost_env_account_config 无 environment∈%v", candidates)
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
