// Package postgres: PGRepository 实现基于 PostgreSQL 的 Repository（Phase4 01_ 成本透视真实数据）。
// [Ref: 03_06_存储架构与ETL规范] [Ref: 04_Phase4/01_成本透视真实数据]
package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/myxxhui/lighthouse-src/internal/config"
)

// PGRepository 使用 database/sql + pgx 实现 Repository，满足 01_ 云账单与 CostService 读库需求。
type PGRepository struct {
	db *sql.DB
}

// NewPGRepository 根据 PostgresConfig 创建 PGRepository。连接失败时返回错误。
func NewPGRepository(cfg config.PostgresConfig) (*PGRepository, error) {
	if cfg.Host == "" {
		return nil, errors.New("postgres host is required")
	}
	port := cfg.Port
	if port <= 0 {
		port = 5432
	}
	dsn := fmt.Sprintf("postgres://%s:%s@%s:%d/%s?sslmode=%s",
		cfg.User, cfg.Password, cfg.Host, port, cfg.Database, sslMode(cfg.SSLMode))
	return NewPGRepositoryFromDSN(dsn, cfg.MaxOpenConns, cfg.MaxIdleConns)
}

// NewPGRepositoryFromDSN 根据 DSN 创建 PGRepository（用于测试或已知连接串场景）。
func NewPGRepositoryFromDSN(dsn string, maxOpen, maxIdle int) (*PGRepository, error) {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, err
	}
	if err := db.PingContext(context.Background()); err != nil {
		_ = db.Close()
		return nil, err
	}
	if maxOpen > 0 {
		db.SetMaxOpenConns(maxOpen)
	}
	if maxIdle > 0 {
		db.SetMaxIdleConns(maxIdle)
	}
	return &PGRepository{db: db}, nil
}

func sslMode(mode string) string {
	if mode == "" {
		return "disable"
	}
	return mode
}

// Close 关闭数据库连接（可选，用于测试或 graceful shutdown）。
func (p *PGRepository) Close() error {
	return p.db.Close()
}

// --- CloudBillSummary (01_ cost_cloud_bill_summary) ---

func (p *PGRepository) SaveCloudBillSummary(ctx context.Context, s CloudBillSummary) error {
	day := s.Day.Truncate(24 * time.Hour).Format("2006-01-02")
	js, err := json.Marshal(s.ProductBreakdown)
	if err != nil {
		return err
	}
	_, err = p.db.ExecContext(ctx,
		`INSERT INTO cost_cloud_bill_summary (day, billing_cycle, total_amount, product_breakdown, created_at, updated_at)
		 VALUES ($1::date, $2, $3, $4, $5, $6)
		 ON CONFLICT (day, billing_cycle) DO UPDATE SET total_amount = EXCLUDED.total_amount, product_breakdown = EXCLUDED.product_breakdown, updated_at = EXCLUDED.updated_at`,
		day, s.BillingCycle, s.TotalAmount, js, s.CreatedAt, s.UpdatedAt)
	return err
}

func (p *PGRepository) GetCloudBillSummary(ctx context.Context, day time.Time, billingCycle string) (*CloudBillSummary, error) {
	dayStr := day.Truncate(24 * time.Hour).Format("2006-01-02")
	var totalAmount float64
	var breakdown []byte
	var createdAt, updatedAt time.Time
	err := p.db.QueryRowContext(ctx,
		`SELECT total_amount, product_breakdown, created_at, updated_at FROM cost_cloud_bill_summary WHERE day = $1::date AND billing_cycle = $2`,
		dayStr, billingCycle).Scan(&totalAmount, &breakdown, &createdAt, &updatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	var m map[string]float64
	if len(breakdown) > 0 {
		_ = json.Unmarshal(breakdown, &m)
	}
	if m == nil {
		m = make(map[string]float64)
	}
	return &CloudBillSummary{
		Day:              day.Truncate(24 * time.Hour),
		BillingCycle:     billingCycle,
		TotalAmount:      totalAmount,
		ProductBreakdown: m,
		CreatedAt:        createdAt,
		UpdatedAt:        updatedAt,
	}, nil
}

func (p *PGRepository) GetLatestCloudBillSummary(ctx context.Context) (*CloudBillSummary, error) {
	var day time.Time
	var billingCycle string
	var totalAmount float64
	var breakdown []byte
	var createdAt, updatedAt time.Time
	err := p.db.QueryRowContext(ctx,
		`SELECT day, billing_cycle, total_amount, product_breakdown, created_at, updated_at FROM cost_cloud_bill_summary ORDER BY day DESC, billing_cycle DESC LIMIT 1`).
		Scan(&day, &billingCycle, &totalAmount, &breakdown, &createdAt, &updatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	var m map[string]float64
	if len(breakdown) > 0 {
		_ = json.Unmarshal(breakdown, &m)
	}
	if m == nil {
		m = make(map[string]float64)
	}
	return &CloudBillSummary{
		Day:              day,
		BillingCycle:     billingCycle,
		TotalAmount:      totalAmount,
		ProductBreakdown: m,
		CreatedAt:        createdAt,
		UpdatedAt:        updatedAt,
	}, nil
}

func (p *PGRepository) GetLatestCloudBillSummaryForBillingCycle(ctx context.Context, billingCycle string) (*CloudBillSummary, error) {
	var day time.Time
	var totalAmount float64
	var breakdown []byte
	var createdAt, updatedAt time.Time
	err := p.db.QueryRowContext(ctx,
		`SELECT day, total_amount, product_breakdown, created_at, updated_at FROM cost_cloud_bill_summary WHERE billing_cycle = $1 ORDER BY day DESC LIMIT 1`,
		billingCycle).Scan(&day, &totalAmount, &breakdown, &createdAt, &updatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	var m map[string]float64
	if len(breakdown) > 0 {
		_ = json.Unmarshal(breakdown, &m)
	}
	if m == nil {
		m = make(map[string]float64)
	}
	return &CloudBillSummary{
		Day:              day,
		BillingCycle:     billingCycle,
		TotalAmount:      totalAmount,
		ProductBreakdown: m,
		CreatedAt:        createdAt,
		UpdatedAt:        updatedAt,
	}, nil
}

func (p *PGRepository) GetCloudBillSummariesForBillingCycles(ctx context.Context, billingCycles []string) ([]*CloudBillSummary, error) {
	if len(billingCycles) == 0 {
		return nil, nil
	}
	var out []*CloudBillSummary
	for _, cycle := range billingCycles {
		one, err := p.GetLatestCloudBillSummaryForBillingCycle(ctx, cycle)
		if err != nil {
			return nil, err
		}
		if one != nil {
			out = append(out, one)
		}
	}
	return out, nil
}

// --- [Ref: 06_ 成本云账单三表] 日原始、月原始、聚合 ---

func (p *PGRepository) SaveCloudBillDailyRaw(ctx context.Context, r CloudBillDailyRaw) error {
	d := r.BillDate.Truncate(24 * time.Hour).Format("2006-01-02")
	js, err := json.Marshal(r.ProductBreakdown)
	if err != nil {
		return err
	}
	cashJS, _ := json.Marshal(r.CashProductBreakdown)
	if cashJS == nil {
		cashJS = []byte("{}")
	}
	accIDVal := ""
	if r.AccountID != "" {
		accIDVal = r.AccountID
	}
	_, err = p.db.ExecContext(ctx,
		`INSERT INTO cost_cloud_bill_daily_raw
		   (bill_date, total_amount, product_breakdown, cash_total_amount, cash_product_breakdown, snapshot_at, created_at, account_id)
		 VALUES ($1::date, $2, $3, $4, $5, $6, $7, $8)
		 ON CONFLICT (bill_date, account_id) DO UPDATE SET
		   total_amount            = EXCLUDED.total_amount,
		   product_breakdown       = EXCLUDED.product_breakdown,
		   cash_total_amount       = EXCLUDED.cash_total_amount,
		   cash_product_breakdown  = EXCLUDED.cash_product_breakdown,
		   snapshot_at             = EXCLUDED.snapshot_at`,
		d, r.TotalAmount, js, r.CashTotalAmount, cashJS, r.SnapshotAt, r.CreatedAt, accIDVal)
	return err
}

func (p *PGRepository) GetCloudBillDailyRaw(ctx context.Context, billDate time.Time, accountID string) (*CloudBillDailyRaw, error) {
	d := billDate.Truncate(24 * time.Hour).Format("2006-01-02")
	acc := accountID
	if acc == "" {
		acc = ""
	}
	var totalAmount, cashTotalAmount float64
	var breakdown, cashBreakdown []byte
	var snapshotAt, createdAt time.Time
	err := p.db.QueryRowContext(ctx,
		`SELECT total_amount, product_breakdown, COALESCE(cash_total_amount,0), COALESCE(cash_product_breakdown,'{}'), snapshot_at, created_at
		 FROM cost_cloud_bill_daily_raw WHERE bill_date = $1::date AND COALESCE(account_id,'') = $2`, d, acc).
		Scan(&totalAmount, &breakdown, &cashTotalAmount, &cashBreakdown, &snapshotAt, &createdAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	m := unmarshalBreakdown(breakdown)
	cashM := unmarshalBreakdown(cashBreakdown)
	t, _ := time.Parse("2006-01-02", d)
	return &CloudBillDailyRaw{
		BillDate: t, TotalAmount: totalAmount, ProductBreakdown: m,
		CashTotalAmount: cashTotalAmount, CashProductBreakdown: cashM,
		SnapshotAt: snapshotAt, CreatedAt: createdAt, AccountID: accountID,
	}, nil
}

func (p *PGRepository) DeleteCloudBillDailyRawForDate(ctx context.Context, billDate time.Time, accountID string) error {
	d := billDate.Truncate(24 * time.Hour).Format("2006-01-02")
	acc := accountID
	if acc == "" {
		acc = ""
	}
	_, err := p.db.ExecContext(ctx, `DELETE FROM cost_cloud_bill_daily_raw WHERE bill_date = $1::date AND COALESCE(account_id,'') = $2`, d, acc)
	return err
}

func (p *PGRepository) ListMissingCloudBillDailyDates(ctx context.Context, from, to time.Time, accountID string) ([]time.Time, error) {
	acc := accountID
	if acc == "" {
		acc = ""
	}
	rows, err := p.db.QueryContext(ctx,
		`SELECT d::date FROM generate_series($1::date, $2::date, '1 day'::interval) d
		 LEFT JOIN cost_cloud_bill_daily_raw r ON r.bill_date = d::date AND COALESCE(r.account_id,'') = $3 WHERE r.bill_date IS NULL ORDER BY 1`,
		from.Format("2006-01-02"), to.Format("2006-01-02"), acc)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []time.Time
	for rows.Next() {
		var d time.Time
		if err := rows.Scan(&d); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

func (p *PGRepository) SaveCloudBillMonthlyRaw(ctx context.Context, r CloudBillMonthlyRaw) error {
	js, err := json.Marshal(r.ProductBreakdown)
	if err != nil {
		return err
	}
	cashJS, _ := json.Marshal(r.CashProductBreakdown)
	if cashJS == nil {
		cashJS = []byte("{}")
	}
	accIDVal := ""
	if r.AccountID != "" {
		accIDVal = r.AccountID
	}
	_, err = p.db.ExecContext(ctx,
		`INSERT INTO cost_cloud_bill_monthly_raw
		   (billing_cycle, total_amount, product_breakdown, cash_total_amount, cash_product_breakdown, snapshot_at, created_at, account_id)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		 ON CONFLICT (billing_cycle, account_id) DO UPDATE SET
		   total_amount            = EXCLUDED.total_amount,
		   product_breakdown       = EXCLUDED.product_breakdown,
		   cash_total_amount       = EXCLUDED.cash_total_amount,
		   cash_product_breakdown  = EXCLUDED.cash_product_breakdown,
		   snapshot_at             = EXCLUDED.snapshot_at`,
		r.BillingCycle, r.TotalAmount, js, r.CashTotalAmount, cashJS, r.SnapshotAt, r.CreatedAt, accIDVal)
	return err
}

func (p *PGRepository) GetCloudBillMonthlyRaw(ctx context.Context, billingCycle, accountID string) (*CloudBillMonthlyRaw, error) {
	acc := accountID
	if acc == "" {
		acc = ""
	}
	var totalAmount, cashTotalAmount float64
	var breakdown, cashBreakdown []byte
	var snapshotAt, createdAt time.Time
	err := p.db.QueryRowContext(ctx,
		`SELECT total_amount, product_breakdown, COALESCE(cash_total_amount,0), COALESCE(cash_product_breakdown,'{}'), snapshot_at, created_at
		 FROM cost_cloud_bill_monthly_raw WHERE billing_cycle = $1 AND COALESCE(account_id,'') = $2`, billingCycle, acc).
		Scan(&totalAmount, &breakdown, &cashTotalAmount, &cashBreakdown, &snapshotAt, &createdAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	m := unmarshalBreakdown(breakdown)
	cashM := unmarshalBreakdown(cashBreakdown)
	return &CloudBillMonthlyRaw{
		BillingCycle: billingCycle, TotalAmount: totalAmount, ProductBreakdown: m,
		CashTotalAmount: cashTotalAmount, CashProductBreakdown: cashM,
		SnapshotAt: snapshotAt, CreatedAt: createdAt, AccountID: accountID,
	}, nil
}

// ListCloudBillMonthlyRawByCycle 返回指定账期下所有 account 的月原始行。[Ref: 01_多环境 UAT]
func (p *PGRepository) ListCloudBillMonthlyRawByCycle(ctx context.Context, billingCycle string) ([]CloudBillMonthlyRaw, error) {
	rows, err := p.db.QueryContext(ctx,
		`SELECT account_id, total_amount, product_breakdown, COALESCE(cash_total_amount,0), COALESCE(cash_product_breakdown,'{}'), snapshot_at, created_at
		 FROM cost_cloud_bill_monthly_raw WHERE billing_cycle = $1`, billingCycle)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []CloudBillMonthlyRaw
	for rows.Next() {
		var accID string
		var totalAmount, cashTotalAmount float64
		var breakdown, cashBreakdown []byte
		var snapshotAt, createdAt time.Time
		if err := rows.Scan(&accID, &totalAmount, &breakdown, &cashTotalAmount, &cashBreakdown, &snapshotAt, &createdAt); err != nil {
			return nil, err
		}
		m := unmarshalBreakdown(breakdown)
		cashM := unmarshalBreakdown(cashBreakdown)
		out = append(out, CloudBillMonthlyRaw{
			BillingCycle: billingCycle, AccountID: accID, TotalAmount: totalAmount, ProductBreakdown: m,
			CashTotalAmount: cashTotalAmount, CashProductBreakdown: cashM,
			SnapshotAt: snapshotAt, CreatedAt: createdAt,
		})
	}
	return out, rows.Err()
}

// DeleteCloudBillMonthlyRawOlderThan 删除 billing_cycle < cutoff 的该 account 月原始行。[Ref: 01_实践 月表保留由配置控制]
func (p *PGRepository) DeleteCloudBillMonthlyRawOlderThan(ctx context.Context, cutoffBillingCycle string, accountID string) error {
	acc := accountID
	if acc == "" {
		acc = ""
	}
	_, err := p.db.ExecContext(ctx, `DELETE FROM cost_cloud_bill_monthly_raw WHERE billing_cycle < $1 AND COALESCE(account_id,'') = $2`, cutoffBillingCycle, acc)
	return err
}

func (p *PGRepository) SaveCloudBillAggregate(ctx context.Context, a CloudBillAggregate) error {
	js, _ := json.Marshal(a.ProductBreakdown)
	now := time.Now()
	metricType := a.MetricType
	if metricType == "" {
		metricType = "payment" // [Ref: 16_ §四] 聚合表仅实际支付，默认 payment
	}
	_, err := p.db.ExecContext(ctx,
		`INSERT INTO cost_cloud_bill_aggregate
		   (report_type, period_key, account_id, metric_type, total_amount, product_breakdown, last_success_at, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		 ON CONFLICT (report_type, period_key, account_id, metric_type) DO UPDATE SET
		   total_amount      = EXCLUDED.total_amount,
		   product_breakdown = EXCLUDED.product_breakdown,
		   last_success_at   = EXCLUDED.last_success_at,
		   updated_at        = EXCLUDED.updated_at`,
		a.ReportType, a.PeriodKey, a.AccountID, metricType, a.TotalAmount, js, a.LastSuccessAt, now, now)
	return err
}

func (p *PGRepository) GetCloudBillAggregate(ctx context.Context, reportType, periodKey string) (*CloudBillAggregate, error) {
	list, err := p.ListCloudBillAggregateForReportPeriod(ctx, reportType, periodKey, "consumption", nil)
	if err != nil || len(list) == 0 {
		return nil, err
	}
	return &list[0], nil
}

// ListCloudBillAggregateForReportPeriod 返回指定 report_type+period_key+metric_type 下 account 的聚合行；accountIDs 非空时仅返回其内 account。[Ref: 聚合表主路径 方案A]
func (p *PGRepository) ListCloudBillAggregateForReportPeriod(ctx context.Context, reportType, periodKey, metricType string, accountIDs []string) ([]CloudBillAggregate, error) {
	if metricType == "" {
		metricType = "consumption"
	}
	q := `SELECT total_amount, product_breakdown, last_success_at, created_at, updated_at, COALESCE(account_id,''), metric_type
		 FROM cost_cloud_bill_aggregate
		 WHERE report_type = $1 AND period_key = $2 AND metric_type = $3`
	args := []interface{}{reportType, periodKey, metricType}
	if len(accountIDs) > 0 {
		ph := make([]string, len(accountIDs))
		for i, id := range accountIDs {
			ph[i] = fmt.Sprintf("$%d", len(args)+1)
			args = append(args, id)
		}
		q += ` AND account_id IN (` + strings.Join(ph, ",") + `)`
	}
	rows, err := p.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []CloudBillAggregate
	for rows.Next() {
		var totalAmount float64
		var breakdown []byte
		var lastSuccessAt sql.NullTime
		var createdAt, updatedAt time.Time
		var accountID, mt string
		if err := rows.Scan(&totalAmount, &breakdown, &lastSuccessAt, &createdAt, &updatedAt, &accountID, &mt); err != nil {
			return nil, err
		}
		m := unmarshalBreakdown(breakdown)
		var last *time.Time
		if lastSuccessAt.Valid {
			last = &lastSuccessAt.Time
		}
		out = append(out, CloudBillAggregate{
			ReportType:       reportType,
			PeriodKey:        periodKey,
			MetricType:       mt,
			TotalAmount:      totalAmount,
			ProductBreakdown: m,
			LastSuccessAt:    last,
			CreatedAt:        createdAt,
			UpdatedAt:        updatedAt,
			AccountID:        accountID,
		})
	}
	return out, rows.Err()
}

func (p *PGRepository) ListEnvAccountConfig(ctx context.Context) ([]EnvAccountConfig, error) {
	rows, err := p.db.QueryContext(ctx,
		`SELECT environment, account_id, COALESCE(display_name,''), COALESCE(sort_order,0), created_at FROM cost_env_account_config ORDER BY sort_order, environment`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []EnvAccountConfig
	for rows.Next() {
		var e EnvAccountConfig
		if err := rows.Scan(&e.Environment, &e.AccountID, &e.DisplayName, &e.SortOrder, &e.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// UpdateEnvAccountConfigAccountID 将 ETL/BSS 解析出的阿里云主账号 ID 写回 cost_env_account_config。[Ref: 03_Phase6/01_FinOps]
func (p *PGRepository) UpdateEnvAccountConfigAccountID(ctx context.Context, environment, aliyunAccountID string) error {
	environment = strings.TrimSpace(environment)
	aliyunAccountID = strings.TrimSpace(aliyunAccountID)
	if environment == "" || aliyunAccountID == "" {
		return nil
	}
	_, err := p.db.ExecContext(ctx,
		`UPDATE cost_env_account_config SET account_id = $1 WHERE environment = $2`,
		aliyunAccountID, environment)
	return err
}

func (p *PGRepository) GetProductCategory(ctx context.Context, productCode string) (string, bool) {
	var category string
	err := p.db.QueryRowContext(ctx, `SELECT category FROM product_category_mapping WHERE product_code = $1`, productCode).Scan(&category)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", false
		}
		return "", false
	}
	return category, true
}

func (p *PGRepository) UpsertProductCategory(ctx context.Context, productCode, category string) error {
	_, err := p.db.ExecContext(ctx,
		`INSERT INTO product_category_mapping (product_code, category, created_at)
		 VALUES ($1, $2, NOW())
		 ON CONFLICT (product_code) DO UPDATE SET category = EXCLUDED.category`,
		productCode, category)
	return err
}

func (p *PGRepository) DeleteCloudBillAggregateExcept(ctx context.Context, reportType string, keepPeriodKeys []string, accountID string) error {
	base := `DELETE FROM cost_cloud_bill_aggregate WHERE report_type = $1`
	args := []interface{}{reportType}
	n := 2
	if accountID != "" {
		base += ` AND account_id = $2`
		args = append(args, accountID)
		n = 3
	}
	if len(keepPeriodKeys) == 0 {
		_, err := p.db.ExecContext(ctx, base, args...)
		return err
	}
	for _, k := range keepPeriodKeys {
		args = append(args, k)
	}
	placeholders := ""
	for i := 0; i < len(keepPeriodKeys); i++ {
		if i > 0 {
			placeholders += ","
		}
		placeholders += fmt.Sprintf("$%d", n+i)
	}
	_, err := p.db.ExecContext(ctx, base+` AND period_key NOT IN (`+placeholders+`)`, args...)
	return err
}

// ListCloudBillDailyRawFromTo 按日期范围查询日原始表；accountID 为空时返回所有 account，非空时仅该 account。[Ref: 01_多环境 UAT]
func (p *PGRepository) ListCloudBillDailyRawFromTo(ctx context.Context, from, to time.Time, accountID string) ([]CloudBillDailyRaw, error) {
	var rows *sql.Rows
	var err error
	if accountID != "" {
		rows, err = p.db.QueryContext(ctx,
			`SELECT bill_date, total_amount, product_breakdown,
			        COALESCE(cash_total_amount,0), COALESCE(cash_product_breakdown,'{}'),
			        snapshot_at, created_at, COALESCE(account_id,'')
			 FROM cost_cloud_bill_daily_raw
			 WHERE bill_date >= $1::date AND bill_date <= $2::date AND COALESCE(account_id,'') = $3 ORDER BY bill_date`,
			from.Format("2006-01-02"), to.Format("2006-01-02"), accountID)
	} else {
		rows, err = p.db.QueryContext(ctx,
			`SELECT bill_date, total_amount, product_breakdown,
			        COALESCE(cash_total_amount,0), COALESCE(cash_product_breakdown,'{}'),
			        snapshot_at, created_at, COALESCE(account_id,'')
			 FROM cost_cloud_bill_daily_raw
			 WHERE bill_date >= $1::date AND bill_date <= $2::date ORDER BY bill_date, account_id`,
			from.Format("2006-01-02"), to.Format("2006-01-02"))
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []CloudBillDailyRaw
	for rows.Next() {
		var billDate time.Time
		var totalAmount, cashTotalAmount float64
		var breakdown, cashBreakdown []byte
		var snapshotAt, createdAt time.Time
		var accID string
		if err := rows.Scan(&billDate, &totalAmount, &breakdown, &cashTotalAmount, &cashBreakdown, &snapshotAt, &createdAt, &accID); err != nil {
			return nil, err
		}
		m := unmarshalBreakdown(breakdown)
		cashM := unmarshalBreakdown(cashBreakdown)
		out = append(out, CloudBillDailyRaw{
			BillDate: billDate, TotalAmount: totalAmount, ProductBreakdown: m,
			CashTotalAmount: cashTotalAmount, CashProductBreakdown: cashM,
			SnapshotAt: snapshotAt, CreatedAt: createdAt, AccountID: accID,
		})
	}
	return out, rows.Err()
}

// unmarshalBreakdown 安全地将 JSON 字节反序列化为 map[string]float64。
func unmarshalBreakdown(data []byte) map[string]float64 {
	m := make(map[string]float64)
	if len(data) > 0 {
		_ = json.Unmarshal(data, &m)
	}
	return m
}

// --- HealthCheck ---

func (p *PGRepository) HealthCheck(ctx context.Context) error {
	return p.db.PingContext(ctx)
}

// --- FinOpsSyncJob [Ref: 03_Phase6/01_FinOps 主动同步] ---

func jsonbOrEmpty(s string, empty []byte) []byte {
	if strings.TrimSpace(s) == "" {
		return empty
	}
	return []byte(s)
}

func (p *PGRepository) InsertFinOpsSyncJob(ctx context.Context, j FinOpsSyncJobRow) (int64, error) {
	snap := jsonbOrEmpty(j.ConfigSnapshot, []byte("{}"))
	warn := jsonbOrEmpty(j.Warnings, []byte("[]"))
	var id int64
	err := p.db.QueryRowContext(ctx,
		`INSERT INTO finops_sync_job (status, phase, config_snapshot, warnings, data_version, progress_current, progress_total, phase_detail)
		 VALUES ($1, $2, $3::jsonb, $4::jsonb, $5, $6, $7, $8) RETURNING id`,
		j.Status, j.Phase, snap, warn, j.DataVersion, j.ProgressCurrent, j.ProgressTotal, j.PhaseDetail,
	).Scan(&id)
	return id, err
}

func (p *PGRepository) UpdateFinOpsSyncJob(ctx context.Context, j FinOpsSyncJobRow) error {
	snap := jsonbOrEmpty(j.ConfigSnapshot, []byte("{}"))
	warn := jsonbOrEmpty(j.Warnings, []byte("[]"))
	_, err := p.db.ExecContext(ctx,
		`UPDATE finops_sync_job SET status=$1, phase=$2, config_snapshot=$3::jsonb, warnings=$4::jsonb,
		 error_message=NULLIF($5,''), started_at=$6, completed_at=$7, data_version=$8,
		 progress_current=$9, progress_total=$10, phase_detail=$11 WHERE id=$12`,
		j.Status, j.Phase, snap, warn, j.ErrorMessage, j.StartedAt, j.CompletedAt, j.DataVersion,
		j.ProgressCurrent, j.ProgressTotal, j.PhaseDetail, j.ID,
	)
	return err
}

func (p *PGRepository) GetFinOpsSyncJob(ctx context.Context, id int64) (*FinOpsSyncJobRow, error) {
	row := p.db.QueryRowContext(ctx,
		`SELECT id, status, phase,
		 COALESCE(config_snapshot::text,''), COALESCE(warnings::text,''),
		 COALESCE(error_message,''), created_at, started_at, completed_at, data_version,
		 progress_current, progress_total, COALESCE(phase_detail,'')
		 FROM finops_sync_job WHERE id=$1`, id)
	var j FinOpsSyncJobRow
	var st, comp sql.NullTime
	err := row.Scan(&j.ID, &j.Status, &j.Phase, &j.ConfigSnapshot, &j.Warnings, &j.ErrorMessage, &j.CreatedAt, &st, &comp, &j.DataVersion,
		&j.ProgressCurrent, &j.ProgressTotal, &j.PhaseDetail)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, sql.ErrNoRows
		}
		return nil, err
	}
	if st.Valid {
		t := st.Time
		j.StartedAt = &t
	}
	if comp.Valid {
		t := comp.Time
		j.CompletedAt = &t
	}
	return &j, nil
}

func (p *PGRepository) CountActiveFinOpsSyncJobs(ctx context.Context) (int64, error) {
	var n int64
	err := p.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM finops_sync_job WHERE status IN ('queued','running')`,
	).Scan(&n)
	return n, err
}

func (p *PGRepository) GetActiveFinOpsSyncJobID(ctx context.Context) (int64, error) {
	var id int64
	err := p.db.QueryRowContext(ctx,
		`SELECT id FROM finops_sync_job WHERE status IN ('queued','running') ORDER BY id DESC LIMIT 1`,
	).Scan(&id)
	if err != nil {
		return 0, err
	}
	return id, nil
}

// --- AggregateDailyNamespaceCosts (L1 回退用) ---

func (p *PGRepository) AggregateDailyNamespaceCosts(ctx context.Context, startDate, endDate time.Time) ([]DailyNamespaceCost, error) {
	rows, err := p.db.QueryContext(ctx,
		`SELECT day, namespace, billable_cost, usage_cost, waste_cost, efficiency, pod_count FROM cost_daily_namespace WHERE day >= $1::date AND day <= $2::date ORDER BY day, namespace`,
		startDate.Format("2006-01-02"), endDate.Format("2006-01-02"))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []DailyNamespaceCost
	for rows.Next() {
		var d time.Time
		var ns string
		var billable, usage, waste, eff sql.NullFloat64
		var podCount sql.NullInt64
		if err := rows.Scan(&d, &ns, &billable, &usage, &waste, &eff, &podCount); err != nil {
			return nil, err
		}
		out = append(out, DailyNamespaceCost{
			Namespace:       ns,
			Date:            d,
			BillableCost:    nullFloat(billable),
			UsageCost:       nullFloat(usage),
			WasteCost:       nullFloat(waste),
			EfficiencyScore: nullFloat(eff),
			PodCount:        int(nullInt64(podCount)),
			NodeCount:       0,
			WorkloadCount:   0,
			CreatedAt:       d,
		})
	}
	return out, rows.Err()
}

func nullFloat(n sql.NullFloat64) float64 {
	if n.Valid {
		return n.Float64
	}
	return 0
}
func nullInt64(n sql.NullInt64) int64 {
	if n.Valid {
		return n.Int64
	}
	return 0
}

// --- BeginTx ---

func (p *PGRepository) BeginTx(ctx context.Context) (Transaction, error) {
	tx, err := p.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	return &pgTransaction{tx: tx, repo: p}, nil
}

type pgTransaction struct {
	tx   *sql.Tx
	repo *PGRepository
}

func (t *pgTransaction) Commit() error   { return t.tx.Commit() }
func (t *pgTransaction) Rollback() error { return t.tx.Rollback() }
func (t *pgTransaction) Repository() Repository {
	return &pgTxRepository{tx: t.tx, parent: t.repo}
}

// pgTxRepository 在事务内执行时仅实现 01_ 与 CostService 需要的方法；其余委托 parent 但用 tx。
type pgTxRepository struct {
	tx    *sql.Tx
	parent *PGRepository
}

func (r *pgTxRepository) SaveCloudBillSummary(ctx context.Context, s CloudBillSummary) error {
	day := s.Day.Truncate(24 * time.Hour).Format("2006-01-02")
	js, err := json.Marshal(s.ProductBreakdown)
	if err != nil {
		return err
	}
	_, err = r.tx.ExecContext(ctx,
		`INSERT INTO cost_cloud_bill_summary (day, billing_cycle, total_amount, product_breakdown, created_at, updated_at)
		 VALUES ($1::date, $2, $3, $4, $5, $6)
		 ON CONFLICT (day, billing_cycle) DO UPDATE SET total_amount = EXCLUDED.total_amount, product_breakdown = EXCLUDED.product_breakdown, updated_at = EXCLUDED.updated_at`,
		day, s.BillingCycle, s.TotalAmount, js, s.CreatedAt, s.UpdatedAt)
	return err
}
func (r *pgTxRepository) GetCloudBillSummary(ctx context.Context, day time.Time, billingCycle string) (*CloudBillSummary, error) {
	return r.parent.GetCloudBillSummary(ctx, day, billingCycle)
}
func (r *pgTxRepository) GetLatestCloudBillSummary(ctx context.Context) (*CloudBillSummary, error) {
	return r.parent.GetLatestCloudBillSummary(ctx)
}
func (r *pgTxRepository) GetLatestCloudBillSummaryForBillingCycle(ctx context.Context, billingCycle string) (*CloudBillSummary, error) {
	return r.parent.GetLatestCloudBillSummaryForBillingCycle(ctx, billingCycle)
}
func (r *pgTxRepository) GetCloudBillSummariesForBillingCycles(ctx context.Context, billingCycles []string) ([]*CloudBillSummary, error) {
	return r.parent.GetCloudBillSummariesForBillingCycles(ctx, billingCycles)
}
func (r *pgTxRepository) SaveCloudBillDailyRaw(ctx context.Context, raw CloudBillDailyRaw) error {
	return r.parent.SaveCloudBillDailyRaw(ctx, raw)
}
func (r *pgTxRepository) GetCloudBillDailyRaw(ctx context.Context, billDate time.Time, accountID string) (*CloudBillDailyRaw, error) {
	return r.parent.GetCloudBillDailyRaw(ctx, billDate, accountID)
}
func (r *pgTxRepository) DeleteCloudBillDailyRawForDate(ctx context.Context, billDate time.Time, accountID string) error {
	return r.parent.DeleteCloudBillDailyRawForDate(ctx, billDate, accountID)
}
func (r *pgTxRepository) ListMissingCloudBillDailyDates(ctx context.Context, from, to time.Time, accountID string) ([]time.Time, error) {
	return r.parent.ListMissingCloudBillDailyDates(ctx, from, to, accountID)
}
func (r *pgTxRepository) SaveCloudBillMonthlyRaw(ctx context.Context, r2 CloudBillMonthlyRaw) error {
	return r.parent.SaveCloudBillMonthlyRaw(ctx, r2)
}
func (r *pgTxRepository) GetCloudBillMonthlyRaw(ctx context.Context, billingCycle, accountID string) (*CloudBillMonthlyRaw, error) {
	return r.parent.GetCloudBillMonthlyRaw(ctx, billingCycle, accountID)
}
func (r *pgTxRepository) ListCloudBillMonthlyRawByCycle(ctx context.Context, billingCycle string) ([]CloudBillMonthlyRaw, error) {
	return r.parent.ListCloudBillMonthlyRawByCycle(ctx, billingCycle)
}
func (r *pgTxRepository) DeleteCloudBillMonthlyRawOlderThan(ctx context.Context, cutoffBillingCycle string, accountID string) error {
	return r.parent.DeleteCloudBillMonthlyRawOlderThan(ctx, cutoffBillingCycle, accountID)
}
func (r *pgTxRepository) SaveCloudBillAggregate(ctx context.Context, a CloudBillAggregate) error {
	return r.parent.SaveCloudBillAggregate(ctx, a)
}
func (r *pgTxRepository) GetCloudBillAggregate(ctx context.Context, reportType, periodKey string) (*CloudBillAggregate, error) {
	return r.parent.GetCloudBillAggregate(ctx, reportType, periodKey)
}
func (r *pgTxRepository) DeleteCloudBillAggregateExcept(ctx context.Context, reportType string, keepPeriodKeys []string, accountID string) error {
	return r.parent.DeleteCloudBillAggregateExcept(ctx, reportType, keepPeriodKeys, accountID)
}
func (r *pgTxRepository) ListCloudBillDailyRawFromTo(ctx context.Context, from, to time.Time, accountID string) ([]CloudBillDailyRaw, error) {
	return r.parent.ListCloudBillDailyRawFromTo(ctx, from, to, accountID)
}
func (r *pgTxRepository) UpdateEnvAccountConfigAccountID(ctx context.Context, environment, aliyunAccountID string) error {
	return r.parent.UpdateEnvAccountConfigAccountID(ctx, environment, aliyunAccountID)
}
func (r *pgTxRepository) ListEnvAccountConfig(ctx context.Context) ([]EnvAccountConfig, error) {
	return r.parent.ListEnvAccountConfig(ctx)
}
func (r *pgTxRepository) GetProductCategory(ctx context.Context, productCode string) (string, bool) {
	return r.parent.GetProductCategory(ctx, productCode)
}
func (r *pgTxRepository) UpsertProductCategory(ctx context.Context, productCode, category string) error {
	return r.parent.UpsertProductCategory(ctx, productCode, category)
}
func (r *pgTxRepository) ListCloudBillAggregateForReportPeriod(ctx context.Context, reportType, periodKey, metricType string, accountIDs []string) ([]CloudBillAggregate, error) {
	return r.parent.ListCloudBillAggregateForReportPeriod(ctx, reportType, periodKey, metricType, accountIDs)
}
func (r *pgTxRepository) HealthCheck(ctx context.Context) error { return r.parent.HealthCheck(ctx) }
func (r *pgTxRepository) InsertFinOpsSyncJob(ctx context.Context, j FinOpsSyncJobRow) (int64, error) {
	return r.parent.InsertFinOpsSyncJob(ctx, j)
}
func (r *pgTxRepository) UpdateFinOpsSyncJob(ctx context.Context, j FinOpsSyncJobRow) error {
	return r.parent.UpdateFinOpsSyncJob(ctx, j)
}
func (r *pgTxRepository) GetFinOpsSyncJob(ctx context.Context, id int64) (*FinOpsSyncJobRow, error) {
	return r.parent.GetFinOpsSyncJob(ctx, id)
}
func (r *pgTxRepository) CountActiveFinOpsSyncJobs(ctx context.Context) (int64, error) {
	return r.parent.CountActiveFinOpsSyncJobs(ctx)
}
func (r *pgTxRepository) GetActiveFinOpsSyncJobID(ctx context.Context) (int64, error) {
	return r.parent.GetActiveFinOpsSyncJobID(ctx)
}
func (r *pgTxRepository) BeginTx(ctx context.Context) (Transaction, error) {
	return nil, errors.New("nested transaction not implemented")
}
func (r *pgTxRepository) AggregateDailyNamespaceCosts(ctx context.Context, startDate, endDate time.Time) ([]DailyNamespaceCost, error) {
	return r.parent.AggregateDailyNamespaceCosts(ctx, startDate, endDate)
}

// --- 未实现方法（01_ 与 CostService 未使用）：返回空或 no-op ---

var errNotImplemented = errors.New("not implemented in PGRepository")

func (p *PGRepository) SaveCostSnapshot(ctx context.Context, snapshot CostSnapshot) error              { return nil }
func (p *PGRepository) GetCostSnapshot(ctx context.Context, id string) (*CostSnapshot, error)         { return nil, nil }
func (p *PGRepository) ListCostSnapshots(ctx context.Context, filter CostSnapshotFilter) ([]CostSnapshot, error) {
	return nil, nil
}
func (p *PGRepository) DeleteCostSnapshot(ctx context.Context, id string) error                        { return nil }
func (p *PGRepository) SaveROIBaseline(ctx context.Context, baseline ROIBaseline) error               { return nil }
func (p *PGRepository) GetROIBaseline(ctx context.Context, id string) (*ROIBaseline, error)           { return nil, nil }
func (p *PGRepository) ListROIBaselines(ctx context.Context, filter ROIBaselineFilter) ([]ROIBaseline, error) {
	return nil, nil
}
func (p *PGRepository) DeleteROIBaseline(ctx context.Context, id string) error                        { return nil }
func (p *PGRepository) SaveDailyNamespaceCost(ctx context.Context, cost DailyNamespaceCost) error     { return nil }
func (p *PGRepository) GetDailyNamespaceCost(ctx context.Context, namespace string, date time.Time) (*DailyNamespaceCost, error) {
	return nil, nil
}
func (p *PGRepository) ListDailyNamespaceCosts(ctx context.Context, filter DailyNamespaceCostFilter) ([]DailyNamespaceCost, error) {
	return nil, nil
}
func (p *PGRepository) SaveHourlyWorkloadStat(ctx context.Context, stat HourlyWorkloadStat) error   { return nil }
func (p *PGRepository) GetHourlyWorkloadStat(ctx context.Context, namespace, workloadName string, timestamp time.Time) (*HourlyWorkloadStat, error) {
	return nil, nil
}
func (p *PGRepository) ListHourlyWorkloadStats(ctx context.Context, filter HourlyWorkloadStatFilter) ([]HourlyWorkloadStat, error) {
	return nil, nil
}
func (p *PGRepository) AggregateHourlyWorkloadStats(ctx context.Context, startTime, endTime time.Time) ([]HourlyWorkloadStat, error) {
	return nil, nil
}
func (p *PGRepository) SaveMetadata(ctx context.Context, metadata Metadata) error                   { return nil }
func (p *PGRepository) GetMetadata(ctx context.Context, key string) (*Metadata, error)               { return nil, nil }
func (p *PGRepository) ListMetadata(ctx context.Context, filter MetadataFilter) ([]Metadata, error)  { return nil, nil }
func (p *PGRepository) DeleteMetadata(ctx context.Context, key string) error                         { return nil }

// 以下为 transaction 内未实现、需满足接口的 stub（pgTxRepository 仅实现 CloudBillSummary + HealthCheck + AggregateDailyNamespaceCosts + BeginTx）
func (r *pgTxRepository) SaveCostSnapshot(ctx context.Context, snapshot CostSnapshot) error { return errNotImplemented }
func (r *pgTxRepository) GetCostSnapshot(ctx context.Context, id string) (*CostSnapshot, error) {
	return nil, nil
}
func (r *pgTxRepository) ListCostSnapshots(ctx context.Context, filter CostSnapshotFilter) ([]CostSnapshot, error) {
	return nil, nil
}
func (r *pgTxRepository) DeleteCostSnapshot(ctx context.Context, id string) error { return nil }
func (r *pgTxRepository) SaveROIBaseline(ctx context.Context, baseline ROIBaseline) error { return errNotImplemented }
func (r *pgTxRepository) GetROIBaseline(ctx context.Context, id string) (*ROIBaseline, error) {
	return nil, nil
}
func (r *pgTxRepository) ListROIBaselines(ctx context.Context, filter ROIBaselineFilter) ([]ROIBaseline, error) {
	return nil, nil
}
func (r *pgTxRepository) DeleteROIBaseline(ctx context.Context, id string) error { return nil }
func (r *pgTxRepository) SaveDailyNamespaceCost(ctx context.Context, cost DailyNamespaceCost) error { return errNotImplemented }
func (r *pgTxRepository) GetDailyNamespaceCost(ctx context.Context, namespace string, date time.Time) (*DailyNamespaceCost, error) {
	return nil, nil
}
func (r *pgTxRepository) ListDailyNamespaceCosts(ctx context.Context, filter DailyNamespaceCostFilter) ([]DailyNamespaceCost, error) {
	return nil, nil
}
func (r *pgTxRepository) SaveHourlyWorkloadStat(ctx context.Context, stat HourlyWorkloadStat) error { return errNotImplemented }
func (r *pgTxRepository) GetHourlyWorkloadStat(ctx context.Context, namespace, workloadName string, timestamp time.Time) (*HourlyWorkloadStat, error) {
	return nil, nil
}
func (r *pgTxRepository) ListHourlyWorkloadStats(ctx context.Context, filter HourlyWorkloadStatFilter) ([]HourlyWorkloadStat, error) {
	return nil, nil
}
func (r *pgTxRepository) AggregateHourlyWorkloadStats(ctx context.Context, startTime, endTime time.Time) ([]HourlyWorkloadStat, error) {
	return nil, nil
}
func (r *pgTxRepository) SaveMetadata(ctx context.Context, metadata Metadata) error { return errNotImplemented }
func (r *pgTxRepository) GetMetadata(ctx context.Context, key string) (*Metadata, error) {
	return nil, nil
}
func (r *pgTxRepository) ListMetadata(ctx context.Context, filter MetadataFilter) ([]Metadata, error) {
	return nil, nil
}
func (r *pgTxRepository) DeleteMetadata(ctx context.Context, key string) error { return nil }

// --- [Ref: 16_云账单动态对账与高可靠处理规范 §三] 行级流水 + 月度状态 ---

// UpsertCloudBillLineItem 幂等写入流水条目（ON CONFLICT record_id DO UPDATE）。
// CashAmount 含负数冲正，不得在调用层过滤。ingestion_channel 见 03_Phase6/01_FinOps。
func (p *PGRepository) UpsertCloudBillLineItem(ctx context.Context, item CloudBillLineItem) error {
	d := item.BillDate.Truncate(24 * time.Hour).Format("2006-01-02")
	now := time.Now()
	ch := item.IngestionChannel
	if ch == "" {
		ch = "api_query_account_bill"
	}
	_, err := p.db.ExecContext(ctx,
		`INSERT INTO cost_cloud_bill_line_items
		 (record_id, bill_date, billing_cycle, product_code, product_name, sub_order_id, instance_id,
		  billing_item, subscription_type, cash_amount, pretax_amount, pretax_gross_amount, currency,
		  is_reversal, account_id, ingestion_channel, region, synced_at, created_at, updated_at)
		 VALUES ($1, $2::date, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20)
		 ON CONFLICT (record_id) DO UPDATE SET
		   cash_amount         = EXCLUDED.cash_amount,
		   pretax_amount       = EXCLUDED.pretax_amount,
		   pretax_gross_amount = EXCLUDED.pretax_gross_amount,
		   is_reversal         = EXCLUDED.is_reversal,
		   ingestion_channel   = EXCLUDED.ingestion_channel,
		   synced_at           = EXCLUDED.synced_at,
		   updated_at          = EXCLUDED.updated_at`,
		item.RecordID, d, item.BillingCycle, item.ProductCode, item.ProductName,
		item.SubOrderID, item.InstanceID, item.BillingItem, item.SubscriptionType,
		item.CashAmount, item.PretaxAmount, item.PretaxGrossAmount,
		nullStr(item.Currency, "CNY"), item.IsReversal,
		nullStr(item.AccountID, ""), ch, nullableStr(item.Region),
		now, now, now)
	return err
}

func nullStr(s, def string) string {
	if s == "" {
		return def
	}
	return s
}
func nullableStr(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}

// ListCloudBillLineItemsByDate 返回指定日期+账号的所有流水条目（含负数冲正）。
func (p *PGRepository) ListCloudBillLineItemsByDate(ctx context.Context, billDate time.Time, accountID string) ([]CloudBillLineItem, error) {
	d := billDate.Truncate(24 * time.Hour).Format("2006-01-02")
	rows, err := p.db.QueryContext(ctx,
		`SELECT record_id, bill_date, billing_cycle, product_code, subscription_type,
		        cash_amount, pretax_amount, pretax_gross_amount, is_reversal, account_id, synced_at, created_at, updated_at
		 FROM cost_cloud_bill_line_items WHERE bill_date = $1::date AND account_id = $2 ORDER BY record_id`,
		d, accountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []CloudBillLineItem
	for rows.Next() {
		var it CloudBillLineItem
		var bdStr string
		var productCode, subType sql.NullString
		if err := rows.Scan(&it.RecordID, &bdStr, &it.BillingCycle, &productCode, &subType,
			&it.CashAmount, &it.PretaxAmount, &it.PretaxGrossAmount, &it.IsReversal,
			&it.AccountID, &it.SyncedAt, &it.CreatedAt, &it.UpdatedAt); err != nil {
			return nil, err
		}
		it.ProductCode = productCode.String
		it.SubscriptionType = subType.String
		it.BillDate, _ = time.Parse("2006-01-02", bdStr[:10])
		out = append(out, it)
	}
	return out, rows.Err()
}

// SumLineItemsPretaxCGByDateRange 按 bill_date 在 [from,to] 内汇总 C（pretax>0）、G（pretax<0）。[Ref: 03_Phase6/01_FinOps 采集与ETL_缺陷分析与最佳实践方案]
func (p *PGRepository) SumLineItemsPretaxCGByDateRange(ctx context.Context, from, to time.Time, accountIDs []string) (c, g float64, err error) {
	fromStr := from.Truncate(24 * time.Hour).Format("2006-01-02")
	toStr := to.Truncate(24 * time.Hour).Format("2006-01-02")
	query := `SELECT COALESCE(SUM(CASE WHEN pretax_amount > 0 THEN pretax_amount ELSE 0 END), 0),
	                 COALESCE(SUM(CASE WHEN pretax_amount < 0 THEN pretax_amount ELSE 0 END), 0)
	          FROM cost_cloud_bill_line_items WHERE bill_date >= $1::date AND bill_date <= $2::date`
	args := []interface{}{fromStr, toStr}
	if len(accountIDs) > 0 {
		placeholders := make([]string, 0, len(accountIDs))
		for i, id := range accountIDs {
			placeholders = append(placeholders, fmt.Sprintf("$%d", i+3))
			args = append(args, id)
		}
		query += fmt.Sprintf(" AND COALESCE(account_id,'') IN (%s)", strings.Join(placeholders, ","))
	}
	var cVal, gVal sql.NullFloat64
	err = p.db.QueryRowContext(ctx, query, args...).Scan(&cVal, &gVal)
	if err != nil {
		return 0, 0, err
	}
	if cVal.Valid {
		c = cVal.Float64
	}
	if gVal.Valid {
		g = gVal.Float64
	}
	return c, g, nil
}

func lineItemsDateAccountFilter(fromStr, toStr string, accountIDs []string) (suffix string, args []interface{}) {
	args = []interface{}{fromStr, toStr}
	if len(accountIDs) == 0 {
		return ` WHERE bill_date >= $1::date AND bill_date <= $2::date`, args
	}
	placeholders := make([]string, 0, len(accountIDs))
	for i, id := range accountIDs {
		placeholders = append(placeholders, fmt.Sprintf("$%d", i+3))
		args = append(args, id)
	}
	return fmt.Sprintf(` WHERE bill_date >= $1::date AND bill_date <= $2::date AND COALESCE(account_id,'') IN (%s)`, strings.Join(placeholders, ",")), args
}

// SumLineItemsPretaxCGByDateRangePreferOSS 若区间内存在 oss_detail 行则仅汇总该渠道，否则与 SumLineItemsPretaxCGByDateRange 相同。[Ref: 03_Phase6/01_FinOps]
func (p *PGRepository) SumLineItemsPretaxCGByDateRangePreferOSS(ctx context.Context, from, to time.Time, accountIDs []string) (c, g float64, err error) {
	fromStr := from.Truncate(24 * time.Hour).Format("2006-01-02")
	toStr := to.Truncate(24 * time.Hour).Format("2006-01-02")
	suf, args := lineItemsDateAccountFilter(fromStr, toStr, accountIDs)
	var n int
	err = p.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM cost_cloud_bill_line_items`+suf+` AND COALESCE(ingestion_channel,'')='oss_detail'`, args...).Scan(&n)
	if err != nil {
		return 0, 0, err
	}
	if n > 0 {
		return p.SumLineItemsPretaxCGByDateRangeWithChannel(ctx, from, to, accountIDs, "oss_detail")
	}
	return p.SumLineItemsPretaxCGByDateRange(ctx, from, to, accountIDs)
}

// SumLineItemsPretaxCGByDateRangeWithChannel 仅汇总指定 ingestion_channel 的 C/G。[Ref: 03_Phase6/01_FinOps FINOPS_CG_SOURCE]
func (p *PGRepository) SumLineItemsPretaxCGByDateRangeWithChannel(ctx context.Context, from, to time.Time, accountIDs []string, channel string) (c, g float64, err error) {
	fromStr := from.Truncate(24 * time.Hour).Format("2006-01-02")
	toStr := to.Truncate(24 * time.Hour).Format("2006-01-02")
	return p.sumLineItemsPretaxCGByDateRangeWithChannelStr(ctx, fromStr, toStr, accountIDs, channel)
}

func (p *PGRepository) sumLineItemsPretaxCGByDateRangeWithChannelStr(ctx context.Context, fromStr, toStr string, accountIDs []string, channel string) (c, g float64, err error) {
	query := `SELECT COALESCE(SUM(CASE WHEN pretax_amount > 0 THEN pretax_amount ELSE 0 END), 0),
	                 COALESCE(SUM(CASE WHEN pretax_amount < 0 THEN pretax_amount ELSE 0 END), 0)
	          FROM cost_cloud_bill_line_items WHERE bill_date >= $1::date AND bill_date <= $2::date AND COALESCE(ingestion_channel,'')=$3`
	args := []interface{}{fromStr, toStr, channel}
	if len(accountIDs) > 0 {
		placeholders := make([]string, 0, len(accountIDs))
		for i, id := range accountIDs {
			placeholders = append(placeholders, fmt.Sprintf("$%d", i+4))
			args = append(args, id)
		}
		query += fmt.Sprintf(" AND COALESCE(account_id,'') IN (%s)", strings.Join(placeholders, ","))
	}
	var cVal, gVal sql.NullFloat64
	err = p.db.QueryRowContext(ctx, query, args...).Scan(&cVal, &gVal)
	if err != nil {
		return 0, 0, err
	}
	if cVal.Valid {
		c = cVal.Float64
	}
	if gVal.Valid {
		g = gVal.Float64
	}
	return c, g, nil
}

// SumPretaxByChannelForDateRange 汇总 pretax_amount（带符号），用于 OSS/API 对账差额。[Ref: 03_Phase6/01_FinOps]
func (p *PGRepository) SumPretaxByChannelForDateRange(ctx context.Context, from, to time.Time, accountIDs []string, channel string) (pretaxSum float64, err error) {
	fromStr := from.Truncate(24 * time.Hour).Format("2006-01-02")
	toStr := to.Truncate(24 * time.Hour).Format("2006-01-02")
	query := `SELECT COALESCE(SUM(pretax_amount), 0) FROM cost_cloud_bill_line_items WHERE bill_date >= $1::date AND bill_date <= $2::date AND COALESCE(ingestion_channel,'')=$3`
	args := []interface{}{fromStr, toStr, channel}
	if len(accountIDs) > 0 {
		placeholders := make([]string, 0, len(accountIDs))
		for i, id := range accountIDs {
			placeholders = append(placeholders, fmt.Sprintf("$%d", i+4))
			args = append(args, id)
		}
		query += fmt.Sprintf(" AND COALESCE(account_id,'') IN (%s)", strings.Join(placeholders, ","))
	}
	var v sql.NullFloat64
	err = p.db.QueryRowContext(ctx, query, args...).Scan(&v)
	if err != nil {
		return 0, err
	}
	if v.Valid {
		pretaxSum = v.Float64
	}
	return pretaxSum, nil
}

// UpsertBSSTransaction 幂等写入 BSS 流水。[Ref: 03_Phase6/01_FinOps]
func (p *PGRepository) UpsertBSSTransaction(ctx context.Context, tx BSSTransactionRow) error {
	if tx.TransactionNumber == "" {
		return fmt.Errorf("bss transaction_number required")
	}
	tm := tx.TransactionTime.UTC().Format("2006-01-02 15:04:05")
	cur := tx.Currency
	if cur == "" {
		cur = "CNY"
	}
	_, err := p.db.ExecContext(ctx, `
		INSERT INTO cost_bss_transactions (transaction_number, account_id, transaction_time, amount, transaction_type, transaction_flow, record_id, billing_cycle, currency, synced_at)
		VALUES ($1,$2,$3::timestamp,$4,$5,$6,$7,$8,$9,NOW())
		ON CONFLICT (transaction_number) DO UPDATE SET
		  amount=EXCLUDED.amount, transaction_type=EXCLUDED.transaction_type, transaction_flow=EXCLUDED.transaction_flow,
		  record_id=EXCLUDED.record_id, billing_cycle=EXCLUDED.billing_cycle, synced_at=NOW()`,
		tx.TransactionNumber, nullStr(tx.AccountID, ""), tm, tx.Amount, nullStr(tx.TransactionType, ""), nullStr(tx.TransactionFlow, ""),
		nullStr(tx.RecordID, ""), nullStr(tx.BillingCycle, ""), cur)
	return err
}

// UpsertBSSBalanceSnapshot 写入账户余额快照（按日覆盖同键）。[Ref: 03_Phase6/01_FinOps]
func (p *PGRepository) UpsertBSSBalanceSnapshot(ctx context.Context, s BSSBalanceSnapshotRow) error {
	d := s.SnapshotDate.UTC().Format("2006-01-02")
	cur := s.Currency
	if cur == "" {
		cur = "CNY"
	}
	_, err := p.db.ExecContext(ctx, `
		INSERT INTO cost_bss_balance_snapshot (account_id, snapshot_date, available_amount, currency, synced_at)
		VALUES ($1,$2::date,$3,$4,NOW())
		ON CONFLICT (account_id, snapshot_date) DO UPDATE SET available_amount=EXCLUDED.available_amount, synced_at=NOW()`,
		nullStr(s.AccountID, ""), d, s.AvailableAmount, cur)
	return err
}

// UpsertBillOutstandingMonthly 账期维度应付/在途汇总（U）。[Ref: 03_Phase6/01_FinOps]
func (p *PGRepository) UpsertBillOutstandingMonthly(ctx context.Context, o BillOutstandingMonthlyRow) error {
	_, err := p.db.ExecContext(ctx, `
		INSERT INTO cost_bill_outstanding_monthly (billing_cycle, account_id, outstanding_amount, synced_at)
		VALUES ($1,$2,$3,NOW())
		ON CONFLICT (billing_cycle, account_id) DO UPDATE SET outstanding_amount=EXCLUDED.outstanding_amount, synced_at=NOW()`,
		o.BillingCycle, nullStr(o.AccountID, ""), o.OutstandingAmount)
	return err
}

// SumBSSPaymentExpenseByDateRange 实付 P：Payment + Expense 流水金额绝对值之和。[Ref: 03_Phase6/01_FinOps]
func (p *PGRepository) SumBSSPaymentExpenseByDateRange(ctx context.Context, from, to time.Time, accountIDs []string) (float64, error) {
	fromStr := from.UTC().Format("2006-01-02 15:04:05")
	toStr := to.UTC().Format("2006-01-02 15:04:05")
	q := `SELECT COALESCE(SUM(ABS(amount)), 0) FROM cost_bss_transactions
	      WHERE transaction_time >= $1::timestamp AND transaction_time <= $2::timestamp
	      AND LOWER(COALESCE(transaction_type,'')) = 'payment' AND LOWER(COALESCE(transaction_flow,'')) = 'expense'`
	args := []interface{}{fromStr, toStr}
	if len(accountIDs) > 0 {
		ph := make([]string, 0, len(accountIDs))
		for i, id := range accountIDs {
			ph = append(ph, fmt.Sprintf("$%d", i+3))
			args = append(args, id)
		}
		q += fmt.Sprintf(" AND COALESCE(account_id,'') IN (%s)", strings.Join(ph, ","))
	}
	var v sql.NullFloat64
	err := p.db.QueryRowContext(ctx, q, args...).Scan(&v)
	if err != nil {
		return 0, err
	}
	if v.Valid {
		return v.Float64, nil
	}
	return 0, nil
}

// LatestBSSBalanceSum 各 account 截至 asOf 的最近一条快照 available 之和。[Ref: 03_Phase6/01_FinOps]
// accountIDs 为空时汇总库内全部 account 的「最新一条」快照（与配置全量列表语义一致，避免未传 IN 时误返回 0）。
func (p *PGRepository) LatestBSSBalanceSum(ctx context.Context, accountIDs []string, asOf time.Time) (float64, error) {
	asOfD := asOf.UTC().Format("2006-01-02")
	if len(accountIDs) == 0 {
		q := `
		WITH latest AS (
		  SELECT DISTINCT ON (account_id) account_id, available_amount
		  FROM cost_bss_balance_snapshot
		  WHERE snapshot_date <= $1::date
		  ORDER BY account_id, snapshot_date DESC
		)
		SELECT COALESCE(SUM(available_amount), 0) FROM latest`
		var v sql.NullFloat64
		err := p.db.QueryRowContext(ctx, q, asOfD).Scan(&v)
		if err != nil {
			return 0, err
		}
		if v.Valid {
			return v.Float64, nil
		}
		return 0, nil
	}
	ph := make([]string, 0, len(accountIDs))
	args := make([]interface{}, 0, len(accountIDs)+1)
	args = append(args, asOfD)
	for i, id := range accountIDs {
		ph = append(ph, fmt.Sprintf("$%d", i+2))
		args = append(args, id)
	}
	q := fmt.Sprintf(`
		WITH latest AS (
		  SELECT DISTINCT ON (account_id) account_id, available_amount
		  FROM cost_bss_balance_snapshot
		  WHERE snapshot_date <= $1::date AND COALESCE(account_id,'') IN (%s)
		  ORDER BY account_id, snapshot_date DESC
		)
		SELECT COALESCE(SUM(available_amount), 0) FROM latest`, strings.Join(ph, ","))
	var v sql.NullFloat64
	err := p.db.QueryRowContext(ctx, q, args...).Scan(&v)
	if err != nil {
		return 0, err
	}
	if v.Valid {
		return v.Float64, nil
	}
	return 0, nil
}

// LatestBSSBalanceMap 各 account 截至 asOf 的最近一条快照 available（不按 account 求和，供按环境去重）。[Ref: 03_Phase6/01_FinOps]
func (p *PGRepository) LatestBSSBalanceMap(ctx context.Context, asOf time.Time) (map[string]float64, error) {
	asOfD := asOf.UTC().Format("2006-01-02")
	q := `
		WITH latest AS (
		  SELECT DISTINCT ON (account_id) account_id, available_amount
		  FROM cost_bss_balance_snapshot
		  WHERE snapshot_date <= $1::date
		  ORDER BY account_id, snapshot_date DESC
		)
		SELECT account_id, available_amount FROM latest`
	rows, err := p.db.QueryContext(ctx, q, asOfD)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make(map[string]float64)
	for rows.Next() {
		var aid string
		var amt sql.NullFloat64
		if err := rows.Scan(&aid, &amt); err != nil {
			return nil, err
		}
		if amt.Valid {
			out[aid] = amt.Float64
		}
	}
	return out, rows.Err()
}

// ListBillOutstandingInBillingCycles 返回账期列表内全部应付行。[Ref: 03_Phase6/01_FinOps]
func (p *PGRepository) ListBillOutstandingInBillingCycles(ctx context.Context, billingCycles []string) ([]BillOutstandingMonthlyRow, error) {
	if len(billingCycles) == 0 {
		return nil, nil
	}
	phC := make([]string, 0, len(billingCycles))
	args := make([]interface{}, 0, len(billingCycles))
	for i, c := range billingCycles {
		phC = append(phC, fmt.Sprintf("$%d", i+1))
		args = append(args, c)
	}
	q := `SELECT billing_cycle, account_id, outstanding_amount FROM cost_bill_outstanding_monthly WHERE billing_cycle IN (` + strings.Join(phC, ",") + `) ORDER BY billing_cycle, account_id`
	rows, err := p.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []BillOutstandingMonthlyRow
	for rows.Next() {
		var r BillOutstandingMonthlyRow
		if err := rows.Scan(&r.BillingCycle, &r.AccountID, &r.OutstandingAmount); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// SumOutstandingByBillingCycles U：多账期 outstanding 之和。[Ref: 03_Phase6/01_FinOps]
func (p *PGRepository) SumOutstandingByBillingCycles(ctx context.Context, billingCycles []string, accountIDs []string) (float64, error) {
	if len(billingCycles) == 0 {
		return 0, nil
	}
	phC := make([]string, 0, len(billingCycles))
	args := make([]interface{}, 0)
	for i, c := range billingCycles {
		phC = append(phC, fmt.Sprintf("$%d", i+1))
		args = append(args, c)
	}
	q := `SELECT COALESCE(SUM(outstanding_amount), 0) FROM cost_bill_outstanding_monthly WHERE billing_cycle IN (` + strings.Join(phC, ",") + `)`
	if len(accountIDs) > 0 {
		phA := make([]string, 0, len(accountIDs))
		base := len(args)
		for i, id := range accountIDs {
			phA = append(phA, fmt.Sprintf("$%d", base+i+1))
			args = append(args, id)
		}
		q += fmt.Sprintf(" AND COALESCE(account_id,'') IN (%s)", strings.Join(phA, ","))
	}
	var v sql.NullFloat64
	err := p.db.QueryRowContext(ctx, q, args...).Scan(&v)
	if err != nil {
		return 0, err
	}
	if v.Valid {
		return v.Float64, nil
	}
	return 0, nil
}

// SumMonthlyCashTotalForBillingCycles 汇总指定账期列表下 cash_total_amount（资金轨 P 降级）。[Ref: 03_Phase6/01_FinOps]
func (p *PGRepository) SumMonthlyCashTotalForBillingCycles(ctx context.Context, billingCycles []string, accountIDs []string) (float64, error) {
	if len(billingCycles) == 0 {
		return 0, nil
	}
	phC := make([]string, 0, len(billingCycles))
	args := make([]interface{}, 0)
	for i, c := range billingCycles {
		phC = append(phC, fmt.Sprintf("$%d", i+1))
		args = append(args, c)
	}
	q := `SELECT COALESCE(SUM(cash_total_amount), 0) FROM cost_cloud_bill_monthly_raw WHERE billing_cycle IN (` + strings.Join(phC, ",") + `)`
	if len(accountIDs) > 0 {
		phA := make([]string, 0, len(accountIDs))
		base := len(args)
		for i, id := range accountIDs {
			phA = append(phA, fmt.Sprintf("$%d", base+i+1))
			args = append(args, id)
		}
		q += fmt.Sprintf(" AND COALESCE(account_id,'') IN (%s)", strings.Join(phA, ","))
	}
	var v sql.NullFloat64
	err := p.db.QueryRowContext(ctx, q, args...).Scan(&v)
	if err != nil {
		return 0, err
	}
	if v.Valid {
		return v.Float64, nil
	}
	return 0, nil
}

// ListCloudBillLineItemsByBillingCycle 返回指定账期+账号的所有流水条目（用于按 billing_cycle 汇总月原始表，回退归属到被冲正账期）。[Ref: 16_ §四、§七]
func (p *PGRepository) ListCloudBillLineItemsByBillingCycle(ctx context.Context, billingCycle, accountID string) ([]CloudBillLineItem, error) {
	rows, err := p.db.QueryContext(ctx,
		`SELECT record_id, bill_date, billing_cycle, product_code, subscription_type,
		        cash_amount, pretax_amount, pretax_gross_amount, is_reversal, account_id, synced_at, created_at, updated_at
		 FROM cost_cloud_bill_line_items WHERE billing_cycle = $1 AND account_id = $2 ORDER BY record_id`,
		billingCycle, accountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []CloudBillLineItem
	for rows.Next() {
		var it CloudBillLineItem
		var bdStr string
		var productCode, subType sql.NullString
		if err := rows.Scan(&it.RecordID, &bdStr, &it.BillingCycle, &productCode, &subType,
			&it.CashAmount, &it.PretaxAmount, &it.PretaxGrossAmount, &it.IsReversal,
			&it.AccountID, &it.SyncedAt, &it.CreatedAt, &it.UpdatedAt); err != nil {
			return nil, err
		}
		it.ProductCode = productCode.String
		it.SubscriptionType = subType.String
		it.BillDate, _ = time.Parse("2006-01-02", bdStr[:10])
		out = append(out, it)
	}
	return out, rows.Err()
}

// ListDistinctBillingCyclesInDateRange 返回日期范围内有流水的所有账期（用于步骤⑧按窗口重算月表）。[Ref: 16_ §七 结合方案]
func (p *PGRepository) ListDistinctBillingCyclesInDateRange(ctx context.Context, from, to time.Time, accountID string) ([]string, error) {
	fromStr := from.Truncate(24 * time.Hour).Format("2006-01-02")
	toStr := to.Truncate(24 * time.Hour).Format("2006-01-02")
	rows, err := p.db.QueryContext(ctx,
		`SELECT DISTINCT billing_cycle FROM cost_cloud_bill_line_items
		 WHERE bill_date >= $1::date AND bill_date <= $2::date AND account_id = $3 ORDER BY billing_cycle`,
		fromStr, toStr, accountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var c string
		if err := rows.Scan(&c); err != nil {
			return nil, err
		}
		if c != "" {
			out = append(out, c)
		}
	}
	return out, rows.Err()
}

// SumLineItemsCashByBillingCycle 汇总指定账期的 CashAmount 代数和（含负数冲正条目）。
func (p *PGRepository) SumLineItemsCashByBillingCycle(ctx context.Context, billingCycle, accountID string) (float64, error) {
	var sum sql.NullFloat64
	err := p.db.QueryRowContext(ctx,
		`SELECT SUM(cash_amount) FROM cost_cloud_bill_line_items WHERE billing_cycle = $1 AND account_id = $2`,
		billingCycle, accountID).Scan(&sum)
	if err != nil {
		return 0, err
	}
	return sum.Float64, nil
}

// DeleteLineItemsOlderThan 删除 bill_date < before 的流水条目（配合 daily_raw 10 个月滑动清理）。
func (p *PGRepository) DeleteLineItemsOlderThan(ctx context.Context, before time.Time, accountID string) error {
	d := before.Truncate(24 * time.Hour).Format("2006-01-02")
	_, err := p.db.ExecContext(ctx,
		`DELETE FROM cost_cloud_bill_line_items WHERE bill_date < $1::date AND account_id = $2`, d, accountID)
	return err
}

// UpsertCloudBillMonthStatus 幂等写入月度对账状态。
func (p *PGRepository) UpsertCloudBillMonthStatus(ctx context.Context, s CloudBillMonthStatus) error {
	now := time.Now()
	_, err := p.db.ExecContext(ctx,
		`INSERT INTO cost_cloud_bill_month_status
		 (billing_cycle, account_id, data_status, line_items_sum, monthly_api_total, drift_amount,
		  last_reconciled_at, last_full_sync_at, finalized_at, notes, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		 ON CONFLICT (billing_cycle, account_id) DO UPDATE SET
		   data_status        = EXCLUDED.data_status,
		   line_items_sum     = EXCLUDED.line_items_sum,
		   monthly_api_total  = EXCLUDED.monthly_api_total,
		   drift_amount       = EXCLUDED.drift_amount,
		   last_reconciled_at = EXCLUDED.last_reconciled_at,
		   last_full_sync_at  = EXCLUDED.last_full_sync_at,
		   finalized_at       = EXCLUDED.finalized_at,
		   notes              = EXCLUDED.notes,
		   updated_at         = EXCLUDED.updated_at`,
		s.BillingCycle, nullStr(s.AccountID, ""), s.DataStatus,
		s.LineItemsSum, s.MonthlyAPITotal, s.DriftAmount,
		nullableTime(s.LastReconciledAt), nullableTime(s.LastFullSyncAt), nullableTime(s.FinalizedAt),
		s.Notes, now, now)
	return err
}

func nullableTime(t *time.Time) interface{} {
	if t == nil {
		return nil
	}
	return *t
}

// GetCloudBillMonthStatus 读取月度对账状态。
func (p *PGRepository) GetCloudBillMonthStatus(ctx context.Context, billingCycle, accountID string) (*CloudBillMonthStatus, error) {
	var s CloudBillMonthStatus
	var lineItemsSum, monthlyAPITotal, driftAmount sql.NullFloat64
	var lastReconciledAt, lastFullSyncAt, finalizedAt sql.NullTime
	var notes sql.NullString
	err := p.db.QueryRowContext(ctx,
		`SELECT billing_cycle, account_id, data_status, line_items_sum, monthly_api_total, drift_amount,
		        last_reconciled_at, last_full_sync_at, finalized_at, notes, created_at, updated_at
		 FROM cost_cloud_bill_month_status WHERE billing_cycle = $1 AND account_id = $2`,
		billingCycle, nullStr(accountID, "")).
		Scan(&s.BillingCycle, &s.AccountID, &s.DataStatus,
			&lineItemsSum, &monthlyAPITotal, &driftAmount,
			&lastReconciledAt, &lastFullSyncAt, &finalizedAt,
			&notes, &s.CreatedAt, &s.UpdatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	s.LineItemsSum = lineItemsSum.Float64
	s.MonthlyAPITotal = monthlyAPITotal.Float64
	s.DriftAmount = driftAmount.Float64
	if lastReconciledAt.Valid {
		s.LastReconciledAt = &lastReconciledAt.Time
	}
	if lastFullSyncAt.Valid {
		s.LastFullSyncAt = &lastFullSyncAt.Time
	}
	if finalizedAt.Valid {
		s.FinalizedAt = &finalizedAt.Time
	}
	s.Notes = notes.String
	return &s, nil
}

// pgTxRepository 代理新方法至父 repository
func (r *pgTxRepository) UpsertCloudBillLineItem(ctx context.Context, item CloudBillLineItem) error {
	return r.parent.UpsertCloudBillLineItem(ctx, item)
}
func (r *pgTxRepository) ListCloudBillLineItemsByDate(ctx context.Context, billDate time.Time, accountID string) ([]CloudBillLineItem, error) {
	return r.parent.ListCloudBillLineItemsByDate(ctx, billDate, accountID)
}
func (r *pgTxRepository) ListCloudBillLineItemsByBillingCycle(ctx context.Context, billingCycle, accountID string) ([]CloudBillLineItem, error) {
	return r.parent.ListCloudBillLineItemsByBillingCycle(ctx, billingCycle, accountID)
}
func (r *pgTxRepository) ListDistinctBillingCyclesInDateRange(ctx context.Context, from, to time.Time, accountID string) ([]string, error) {
	return r.parent.ListDistinctBillingCyclesInDateRange(ctx, from, to, accountID)
}
func (r *pgTxRepository) SumLineItemsCashByBillingCycle(ctx context.Context, billingCycle, accountID string) (float64, error) {
	return r.parent.SumLineItemsCashByBillingCycle(ctx, billingCycle, accountID)
}
func (r *pgTxRepository) SumLineItemsPretaxCGByDateRange(ctx context.Context, from, to time.Time, accountIDs []string) (c, g float64, err error) {
	return r.parent.SumLineItemsPretaxCGByDateRange(ctx, from, to, accountIDs)
}
func (r *pgTxRepository) SumLineItemsPretaxCGByDateRangePreferOSS(ctx context.Context, from, to time.Time, accountIDs []string) (c, g float64, err error) {
	return r.parent.SumLineItemsPretaxCGByDateRangePreferOSS(ctx, from, to, accountIDs)
}

func (r *pgTxRepository) SumLineItemsPretaxCGByDateRangeWithChannel(ctx context.Context, from, to time.Time, accountIDs []string, channel string) (c, g float64, err error) {
	return r.parent.SumLineItemsPretaxCGByDateRangeWithChannel(ctx, from, to, accountIDs, channel)
}
func (r *pgTxRepository) SumPretaxByChannelForDateRange(ctx context.Context, from, to time.Time, accountIDs []string, channel string) (float64, error) {
	return r.parent.SumPretaxByChannelForDateRange(ctx, from, to, accountIDs, channel)
}
func (r *pgTxRepository) UpsertBSSTransaction(ctx context.Context, tx BSSTransactionRow) error {
	return r.parent.UpsertBSSTransaction(ctx, tx)
}
func (r *pgTxRepository) UpsertBSSBalanceSnapshot(ctx context.Context, s BSSBalanceSnapshotRow) error {
	return r.parent.UpsertBSSBalanceSnapshot(ctx, s)
}
func (r *pgTxRepository) UpsertBillOutstandingMonthly(ctx context.Context, o BillOutstandingMonthlyRow) error {
	return r.parent.UpsertBillOutstandingMonthly(ctx, o)
}
func (r *pgTxRepository) SumBSSPaymentExpenseByDateRange(ctx context.Context, from, to time.Time, accountIDs []string) (float64, error) {
	return r.parent.SumBSSPaymentExpenseByDateRange(ctx, from, to, accountIDs)
}
func (r *pgTxRepository) LatestBSSBalanceSum(ctx context.Context, accountIDs []string, asOf time.Time) (float64, error) {
	return r.parent.LatestBSSBalanceSum(ctx, accountIDs, asOf)
}
func (r *pgTxRepository) LatestBSSBalanceMap(ctx context.Context, asOf time.Time) (map[string]float64, error) {
	return r.parent.LatestBSSBalanceMap(ctx, asOf)
}
func (r *pgTxRepository) ListBillOutstandingInBillingCycles(ctx context.Context, billingCycles []string) ([]BillOutstandingMonthlyRow, error) {
	return r.parent.ListBillOutstandingInBillingCycles(ctx, billingCycles)
}
func (r *pgTxRepository) SumOutstandingByBillingCycles(ctx context.Context, billingCycles []string, accountIDs []string) (float64, error) {
	return r.parent.SumOutstandingByBillingCycles(ctx, billingCycles, accountIDs)
}
func (r *pgTxRepository) SumMonthlyCashTotalForBillingCycles(ctx context.Context, billingCycles []string, accountIDs []string) (float64, error) {
	return r.parent.SumMonthlyCashTotalForBillingCycles(ctx, billingCycles, accountIDs)
}
func (r *pgTxRepository) DeleteLineItemsOlderThan(ctx context.Context, before time.Time, accountID string) error {
	return r.parent.DeleteLineItemsOlderThan(ctx, before, accountID)
}
func (r *pgTxRepository) UpsertCloudBillMonthStatus(ctx context.Context, s CloudBillMonthStatus) error {
	return r.parent.UpsertCloudBillMonthStatus(ctx, s)
}
func (r *pgTxRepository) GetCloudBillMonthStatus(ctx context.Context, billingCycle, accountID string) (*CloudBillMonthStatus, error) {
	return r.parent.GetCloudBillMonthStatus(ctx, billingCycle, accountID)
}

// 确保类型实现接口（编译期检查）
var _ Repository = (*PGRepository)(nil)
