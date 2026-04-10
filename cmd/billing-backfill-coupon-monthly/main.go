// 历史回补：对已存在的 cost_cloud_bill_monthly_raw 行写入 QueryAccountBill 汇总的 deducted_by_coupons / deducted_by_cash_coupons（不删不改 total_amount）。
// 需先执行 lighthouse-deploy/scripts/migrate-05-coupon-monthly-raw.sql。
//
//	cd lighthouse-src && set -a && source ../lighthouse-deploy/.env && set +a
//	go run ./cmd/billing-backfill-coupon-monthly -months=60 -env=C66_POC
//
// [Ref: 04_采集 §5.4 优惠券]
package main

import (
	"context"
	"flag"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/myxxhui/lighthouse-src/internal/config"
	"github.com/myxxhui/lighthouse-src/internal/data/cloudbilling"
	"github.com/myxxhui/lighthouse-src/internal/data/postgres"
)

func main() {
	months := flag.Int("months", 60, "向前回补月数")
	envCred := flag.String("env", "C66_POC", "AK 后缀，与 NewFetcherForEnv 一致")
	account := flag.String("account", "", "account_id；空则从 cost_env_account_config 解析 POC")
	dry := flag.Bool("dry-run", false, "只打印不写库")
	flag.Parse()

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
	db, err := postgres.NewPGRepositoryFromDSN(dsn, 0, 0)
	if err != nil {
		fmt.Fprintf(os.Stderr, "pg: %v\n", err)
		os.Exit(1)
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 3600*time.Second)
	defer cancel()

	accountID := strings.TrimSpace(*account)
	if accountID == "" {
		accountID, err = resolvePOCAccountID(ctx, db)
		if err != nil {
			fmt.Fprintf(os.Stderr, "account_id: %v\n", err)
			os.Exit(1)
		}
		fmt.Fprintf(os.Stderr, "[info] account_id=%q\n", accountID)
	}

	f := cloudbilling.NewFetcherForEnv(strings.TrimSpace(*envCred))
	if f == nil {
		fmt.Fprintf(os.Stderr, "NewFetcherForEnv(%q) 失败\n", *envCred)
		os.Exit(1)
	}

	now := time.Now().UTC()
	for i := 0; i < *months; i++ {
		ref := now.AddDate(0, -i, 0)
		cycle := fmt.Sprintf("%04d-%02d", ref.Year(), ref.Month())
		row, err := db.GetCloudBillMonthlyRaw(ctx, cycle, accountID)
		if err != nil {
			fmt.Fprintf(os.Stderr, "get %s: %v\n", cycle, err)
			continue
		}
		if row == nil {
			fmt.Printf("skip %s (no monthly_raw row)\n", cycle)
			continue
		}
		c, cc, err := f.FetchCouponDeductionMonthly(ctx, cycle)
		if err != nil {
			fmt.Fprintf(os.Stderr, "api %s: %v\n", cycle, err)
			continue
		}
		ts := time.Now().UTC()
		row.DeductedByCoupons = &c
		row.DeductedByCashCoupons = &cc
		row.CouponSyncedAt = &ts
		if *dry {
			fmt.Printf("[dry] %s coupon=%.6f cash_coupon=%.6f\n", cycle, c, cc)
			continue
		}
		if err := db.SaveCloudBillMonthlyRaw(ctx, *row); err != nil {
			fmt.Fprintf(os.Stderr, "save %s: %v\n", cycle, err)
			continue
		}
		fmt.Printf("ok %s coupon=%.6f cash_coupon=%.6f\n", cycle, c, cc)
	}
}

func resolvePOCAccountID(ctx context.Context, db *postgres.PGRepository) (string, error) {
	// 使用 repository 的底层 DB 需暴露 — 用简单 SQL 通过 NewPGRepository 无 QueryRow
	// 用 ListEnvAccountConfig 若存在
	cfg, err := db.ListEnvAccountConfig(ctx)
	if err != nil {
		return "", err
	}
	for _, e := range []string{"C66_POC", "POC"} {
		for _, c := range cfg {
			if strings.EqualFold(c.Environment, e) && strings.TrimSpace(c.AccountID) != "" {
				return strings.TrimSpace(c.AccountID), nil
			}
		}
	}
	return "", fmt.Errorf("no C66_POC/POC account_id")
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
