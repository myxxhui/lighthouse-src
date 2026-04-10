// 临时验证：从 finops_billing_fact（OSS 计费项明细入库）按账期汇总理论口径
//
// 理论（与入库行 amount 一致，amount 来自 CSV「优惠后金额/应付金额」等列）：
//   应付 = 本账期内 amount > 0 的行求和（一般为正）；
//   回帐 = 本账期内 amount < 0 的行求和（一般为负或 0）；
//   净额 = 应付 + 回帐（= 全量 SUM(amount)）；
//   实付 = 若净额 < 0 则 0（无需再支付）；若净额 >= 0 则为净额（应付侧实付）。
//
// 用法（PG：可先 source lighthouse-deploy/.env；可选 LIGHTHOUSE_DEPLOY_YAML 读 postgres）:
//
//	cd lighthouse-src
//	go run ./cmd/finops-verify-oss-month -cycle=2025-11
//	go run ./cmd/finops-verify-oss-month -cycle=2025-11 -environment=C66_POC
//	go run ./cmd/finops-verify-oss-month -cycle=2025-11 -account=1234567890123456
//	go run ./cmd/finops-verify-oss-month -cycle=2025-11 -finops-env=POC
//	go run ./cmd/finops-verify-oss-month -cycle=2025-11 -by-account
//	go run ./cmd/finops-verify-oss-month -list-config-env
//	# 清洗本账期 POC 映射账号下的明细（需显式 -confirm）
//	go run ./cmd/finops-verify-oss-month -cycle=2025-11 -clean -environment=C66_POC -confirm
//	go run ./cmd/finops-verify-oss-month -cycle=2025-11 -environment=C66_POC -o /tmp/finops-2025-11.txt
// 说明：应付/回帐/实付等为终端打印的汇总结果，不落库；`-o` 将同一份报告写入文件。
//
// [Ref: 03_Phase6/01_FinOps OSS OLAP 验证]
package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"io"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/myxxhui/lighthouse-src/internal/config"
)

func main() {
	cycle := flag.String("cycle", "2025-11", "账期 YYYY-MM，如 2025-11")
	account := flag.String("account", "", "可选：仅统计该 finops_billing_fact.account_id；与 -environment 二选一优先组合")
	environment := flag.String("environment", "", "从 cost_env_account_config.environment 解析 account_id（如 C66_POC、POC）")
	finopsEnv := flag.String("finops-env", "", "可选：再按 finops_billing_fact.env 列过滤（标签解析出的环境，如 POC）")
	byAccount := flag.Bool("by-account", false, "按 account_id 分组输出")
	listCfg := flag.Bool("list-config-env", false, "打印 cost_env_account_config 后退出")
	clean := flag.Bool("clean", false, "删除本账期匹配 finops_billing_fact 行（需 -confirm）")
	cleanCheckpoint := flag.Bool("clean-checkpoint", false, "与 -clean 联用：同时删除 finops_oss_sync_checkpoint 中对应 account_id")
	confirm := flag.Bool("confirm", false, "与 -clean 联用，确认执行删除")
	outPath := flag.String("o", "", "将统计报告同时写入该文件（终端仍会打印一份）")
	flag.Parse()

	reportOut := io.Writer(os.Stdout)
	if strings.TrimSpace(*outPath) != "" {
		f, err := os.Create(strings.TrimSpace(*outPath))
		if err != nil {
			fmt.Fprintf(os.Stderr, "创建报告文件: %v\n", err)
			os.Exit(1)
		}
		defer f.Close()
		reportOut = io.MultiWriter(os.Stdout, f)
		fmt.Fprintf(os.Stderr, "[info] 报告同时写入: %s\n", strings.TrimSpace(*outPath))
	}

	cfg := &config.Config{}
	if doc, err := config.LoadLighthouseDeployYAML(""); err == nil && doc != nil {
		config.ApplyLighthouseDeployYAML(cfg, doc)
	}
	fillPostgresFromEnv(cfg)
	if cfg.Postgres.Host == "" {
		fmt.Fprintln(os.Stderr, "finops-verify-oss-month: 需要 PG_HOST 或 POSTGRES_HOST（或统一部署 YAML 中 postgres.host）")
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

	if *listCfg {
		if err := runListConfigEnv(ctx, db, reportOut); err != nil {
			fmt.Fprintf(os.Stderr, "%v\n", err)
			os.Exit(1)
		}
		return
	}

	accountID := strings.TrimSpace(*account)
	if strings.TrimSpace(*environment) != "" {
		aid, err := resolveAccountIDFromEnvironment(ctx, db, strings.TrimSpace(*environment))
		if err != nil {
			fmt.Fprintf(os.Stderr, "解析 environment=%q: %v\n", *environment, err)
			os.Exit(1)
		}
		if accountID != "" && !strings.EqualFold(accountID, aid) {
			fmt.Fprintf(os.Stderr, "冲突: -account=%q 与 cost_env 中 %q 的 account_id=%q 不一致\n", accountID, *environment, aid)
			os.Exit(1)
		}
		accountID = aid
		fmt.Fprintf(os.Stderr, "[info] -environment=%q -> account_id=%q\n", *environment, accountID)
	}

	if *clean {
		if !*confirm {
			fmt.Fprintln(os.Stderr, "拒绝: -clean 必须同时传入 -confirm")
			os.Exit(1)
		}
		if accountID == "" && strings.TrimSpace(*finopsEnv) == "" {
			fmt.Fprintln(os.Stderr, "拒绝: -clean 需指定 -account 或 -environment 或 -finops-env，避免误删全表")
			os.Exit(1)
		}
		if err := runClean(ctx, db, *cycle, accountID, strings.TrimSpace(*finopsEnv), *cleanCheckpoint); err != nil {
			fmt.Fprintf(os.Stderr, "%v\n", err)
			os.Exit(1)
		}
		fmt.Println("清洗完成。")
	}

	if *byAccount {
		if err := runByAccount(ctx, db, reportOut, *cycle, strings.TrimSpace(*finopsEnv)); err != nil {
			fmt.Fprintf(os.Stderr, "%v\n", err)
			os.Exit(1)
		}
		return
	}

	if err := runAggregate(ctx, db, reportOut, *cycle, accountID, strings.TrimSpace(*finopsEnv)); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
}

func resolveAccountIDFromEnvironment(ctx context.Context, db *sql.DB, environment string) (string, error) {
	var aid string
	err := db.QueryRowContext(ctx,
		`SELECT COALESCE(TRIM(account_id::text), '') FROM cost_env_account_config WHERE environment = $1`,
		environment).Scan(&aid)
	if err == sql.ErrNoRows {
		return "", fmt.Errorf("cost_env_account_config 无 environment=%q", environment)
	}
	if err != nil {
		return "", err
	}
	if aid == "" {
		return "", fmt.Errorf("cost_env_account_config.environment=%q 的 account_id 为空", environment)
	}
	return aid, nil
}

func runListConfigEnv(ctx context.Context, db *sql.DB, w io.Writer) error {
	rows, err := db.QueryContext(ctx, `SELECT environment, account_id, display_name, sort_order FROM cost_env_account_config ORDER BY sort_order, environment`)
	if err != nil {
		return err
	}
	defer rows.Close()
	fmt.Fprintln(w, "cost_env_account_config（environment -> account_id）")
	fmt.Fprintf(w, "%-20s %-24s %s\n", "environment", "account_id", "display_name")
	for rows.Next() {
		var env, aid, dn string
		var so int
		if err := rows.Scan(&env, &aid, &dn, &so); err != nil {
			return err
		}
		fmt.Fprintf(w, "%-20s %-24s %s\n", env, aid, dn)
	}
	return rows.Err()
}

func runClean(ctx context.Context, db *sql.DB, cycle, accountID, finopsEnv string, dropCheckpoint bool) error {
	q := `DELETE FROM finops_billing_fact WHERE billing_cycle = $1`
	args := []interface{}{cycle}
	n := 2
	if accountID != "" {
		q += fmt.Sprintf(` AND COALESCE(account_id,'') = $%d`, n)
		args = append(args, accountID)
		n++
	}
	if finopsEnv != "" {
		q += fmt.Sprintf(` AND env = $%d`, n)
		args = append(args, strings.ToUpper(strings.TrimSpace(finopsEnv)))
	}
	res, err := db.ExecContext(ctx, q, args...)
	if err != nil {
		return fmt.Errorf("delete finops_billing_fact: %w", err)
	}
	ra, _ := res.RowsAffected()
	fmt.Printf("已删除 finops_billing_fact 行数: %d (WHERE billing_cycle=%q", ra, cycle)
	if accountID != "" {
		fmt.Printf(", account_id=%q", accountID)
	}
	if finopsEnv != "" {
		fmt.Printf(", env=%q", strings.ToUpper(strings.TrimSpace(finopsEnv)))
	}
	fmt.Println(")")

	if dropCheckpoint && accountID != "" {
		_, err := db.ExecContext(ctx, `DELETE FROM finops_oss_sync_checkpoint WHERE account_id = $1`, accountID)
		if err != nil {
			return fmt.Errorf("delete finops_oss_sync_checkpoint: %w", err)
		}
		fmt.Printf("已删除 finops_oss_sync_checkpoint.account_id=%q\n", accountID)
	}
	return nil
}

func runAggregate(ctx context.Context, db *sql.DB, w io.Writer, cycle, accountID, finopsEnv string) error {
	q := `SELECT
  COUNT(*)::bigint,
  COALESCE(SUM(CASE WHEN amount > 0 THEN amount END), 0),
  COALESCE(SUM(CASE WHEN amount < 0 THEN amount END), 0),
  COALESCE(SUM(amount), 0)
FROM finops_billing_fact
WHERE billing_cycle = $1`
	args := []interface{}{cycle}
	n := 2
	if accountID != "" {
		q += fmt.Sprintf(` AND COALESCE(account_id,'') = $%d`, n)
		args = append(args, accountID)
		n++
	}
	if finopsEnv != "" {
		q += fmt.Sprintf(` AND env = $%d`, n)
		args = append(args, strings.ToUpper(strings.TrimSpace(finopsEnv)))
	}
	var rowN int64
	var payable, refund, net float64
	if err := db.QueryRowContext(ctx, q, args...).Scan(&rowN, &payable, &refund, &net); err != nil {
		return fmt.Errorf("query: %w", err)
	}
	actual := net
	if actual < 0 {
		actual = 0
	}

	fmt.Fprintln(w, "========== FinOps OSS 事实表理论验证 ==========")
	fmt.Fprintln(w, "数据来源: finops_billing_fact.amount（OSS CSV 解析：优惠后金额/应付金额等）")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "【定义】")
	fmt.Fprintln(w, "  应付  = 本账期内所有 amount > 0 的行之和")
	fmt.Fprintln(w, "  回帐  = 本账期内所有 amount < 0 的行之和（≤ 0，一般为负）")
	fmt.Fprintln(w, "  净额  = 应付 + 回帐（等价于 SUM(amount)）")
	fmt.Fprintln(w, "  实付  = 若净额 < 0 则为 0；若净额 ≥ 0 则等于净额")
	fmt.Fprintln(w)
	fmt.Fprintf(w, "【账期】 %s\n", cycle)
	if accountID != "" {
		fmt.Fprintf(w, "【筛选】 account_id = %s\n", accountID)
	} else {
		fmt.Fprintln(w, "【筛选】 account_id = (未过滤，全表该账期)")
	}
	if finopsEnv != "" {
		fmt.Fprintf(w, "【筛选】 finops_billing_fact.env = %s\n", strings.ToUpper(strings.TrimSpace(finopsEnv)))
	}
	fmt.Fprintln(w)
	fmt.Fprintf(w, "行数:                    %d\n", rowN)
	fmt.Fprintf(w, "应付(正数行合计):       %.6f\n", payable)
	fmt.Fprintf(w, "回帐(负数行合计):       %.6f\n", refund)
	fmt.Fprintf(w, "净额(应付+回帐):         %.6f\n", net)
	fmt.Fprintf(w, "实付(max(0,净额)):      %.6f\n", actual)
	fmt.Fprintln(w)
	fmt.Fprintln(w, "【校验】 净额 应等于 应付+回帐（与 SUM(amount) 一致）。")
	return nil
}

func runByAccount(ctx context.Context, db *sql.DB, w io.Writer, cycle, finopsEnv string) error {
	q := `SELECT COALESCE(account_id,'') AS aid,
  COUNT(*)::bigint,
  COALESCE(SUM(CASE WHEN amount > 0 THEN amount END), 0),
  COALESCE(SUM(CASE WHEN amount < 0 THEN amount END), 0),
  COALESCE(SUM(amount), 0)
FROM finops_billing_fact
WHERE billing_cycle = $1`
	args := []interface{}{cycle}
	if finopsEnv != "" {
		q += ` AND env = $2`
		args = append(args, strings.ToUpper(strings.TrimSpace(finopsEnv)))
	}
	q += ` GROUP BY COALESCE(account_id,'')
ORDER BY aid`
	rows, err := db.QueryContext(ctx, q, args...)
	if err != nil {
		return fmt.Errorf("query: %w", err)
	}
	defer rows.Close()
	fmt.Fprintf(w, "账期 %s — 按 account_id 分组", cycle)
	if finopsEnv != "" {
		fmt.Fprintf(w, "（且 env=%s）", strings.ToUpper(strings.TrimSpace(finopsEnv)))
	}
	fmt.Fprintln(w)
	fmt.Fprintln(w)
	fmt.Fprintf(w, "%-24s %10s %18s %18s %18s %12s\n", "account_id", "行数", "应付(>0)", "回帐(<0)", "净额", "实付")
	for rows.Next() {
		var aid string
		var n int64
		var payable, refund, net float64
		if err := rows.Scan(&aid, &n, &payable, &refund, &net); err != nil {
			return err
		}
		ap := net
		if ap < 0 {
			ap = 0
		}
		if aid == "" {
			aid = "(空)"
		}
		fmt.Fprintf(w, "%-24s %10d %18.6f %18.6f %18.6f %12.6f\n", aid, n, payable, refund, net, ap)
	}
	return rows.Err()
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
	user := url.QueryEscape(cfg.User)
	pass := url.QueryEscape(cfg.Password)
	host := cfg.Host
	dbn := cfg.Database
	return fmt.Sprintf("postgres://%s:%s@%s:%d/%s?sslmode=%s",
		user, pass, host, port, dbn, ssl)
}
