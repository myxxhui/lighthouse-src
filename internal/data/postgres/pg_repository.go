// Package postgres: PGRepository 实现基于 PostgreSQL 的 Repository（Phase4 01_ 成本透视真实数据）。
// [Ref: 03_06_存储架构与ETL规范] [Ref: 04_Phase4/01_成本透视真实数据]
package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
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
	_, err = p.db.ExecContext(ctx,
		`INSERT INTO cost_cloud_bill_daily_raw (bill_date, total_amount, product_breakdown, snapshot_at, created_at)
		 VALUES ($1::date, $2, $3, $4, $5)
		 ON CONFLICT (bill_date) DO UPDATE SET total_amount = EXCLUDED.total_amount, product_breakdown = EXCLUDED.product_breakdown, snapshot_at = EXCLUDED.snapshot_at`,
		d, r.TotalAmount, js, r.SnapshotAt, r.CreatedAt)
	return err
}

func (p *PGRepository) GetCloudBillDailyRaw(ctx context.Context, billDate time.Time) (*CloudBillDailyRaw, error) {
	d := billDate.Truncate(24 * time.Hour).Format("2006-01-02")
	var totalAmount float64
	var breakdown []byte
	var snapshotAt, createdAt time.Time
	err := p.db.QueryRowContext(ctx,
		`SELECT total_amount, product_breakdown, snapshot_at, created_at FROM cost_cloud_bill_daily_raw WHERE bill_date = $1::date`, d).
		Scan(&totalAmount, &breakdown, &snapshotAt, &createdAt)
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
	t, _ := time.Parse("2006-01-02", d)
	return &CloudBillDailyRaw{BillDate: t, TotalAmount: totalAmount, ProductBreakdown: m, SnapshotAt: snapshotAt, CreatedAt: createdAt}, nil
}

func (p *PGRepository) DeleteCloudBillDailyRawForDate(ctx context.Context, billDate time.Time) error {
	d := billDate.Truncate(24 * time.Hour).Format("2006-01-02")
	_, err := p.db.ExecContext(ctx, `DELETE FROM cost_cloud_bill_daily_raw WHERE bill_date = $1::date`, d)
	return err
}

func (p *PGRepository) ListMissingCloudBillDailyDates(ctx context.Context, from, to time.Time) ([]time.Time, error) {
	rows, err := p.db.QueryContext(ctx,
		`SELECT d::date FROM generate_series($1::date, $2::date, '1 day'::interval) d
		 LEFT JOIN cost_cloud_bill_daily_raw r ON r.bill_date = d::date WHERE r.bill_date IS NULL ORDER BY 1`,
		from.Format("2006-01-02"), to.Format("2006-01-02"))
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
	_, err = p.db.ExecContext(ctx,
		`INSERT INTO cost_cloud_bill_monthly_raw (billing_cycle, total_amount, product_breakdown, snapshot_at, created_at)
		 VALUES ($1, $2, $3, $4, $5)
		 ON CONFLICT (billing_cycle) DO UPDATE SET total_amount = EXCLUDED.total_amount, product_breakdown = EXCLUDED.product_breakdown, snapshot_at = EXCLUDED.snapshot_at`,
		r.BillingCycle, r.TotalAmount, js, r.SnapshotAt, r.CreatedAt)
	return err
}

func (p *PGRepository) GetCloudBillMonthlyRaw(ctx context.Context, billingCycle string) (*CloudBillMonthlyRaw, error) {
	var totalAmount float64
	var breakdown []byte
	var snapshotAt, createdAt time.Time
	err := p.db.QueryRowContext(ctx,
		`SELECT total_amount, product_breakdown, snapshot_at, created_at FROM cost_cloud_bill_monthly_raw WHERE billing_cycle = $1`, billingCycle).
		Scan(&totalAmount, &breakdown, &snapshotAt, &createdAt)
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
	return &CloudBillMonthlyRaw{BillingCycle: billingCycle, TotalAmount: totalAmount, ProductBreakdown: m, SnapshotAt: snapshotAt, CreatedAt: createdAt}, nil
}

func (p *PGRepository) SaveCloudBillAggregate(ctx context.Context, a CloudBillAggregate) error {
	js, _ := json.Marshal(a.ProductBreakdown)
	now := time.Now()
	_, err := p.db.ExecContext(ctx,
		`INSERT INTO cost_cloud_bill_aggregate (report_type, period_key, total_amount, product_breakdown, last_success_at, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)
		 ON CONFLICT (report_type, period_key) DO UPDATE SET total_amount = EXCLUDED.total_amount, product_breakdown = EXCLUDED.product_breakdown, last_success_at = EXCLUDED.last_success_at, updated_at = EXCLUDED.updated_at`,
		a.ReportType, a.PeriodKey, a.TotalAmount, js, a.LastSuccessAt, now, now)
	return err
}

func (p *PGRepository) GetCloudBillAggregate(ctx context.Context, reportType, periodKey string) (*CloudBillAggregate, error) {
	list, err := p.ListCloudBillAggregateForReportPeriod(ctx, reportType, periodKey)
	if err != nil || len(list) == 0 {
		return nil, err
	}
	// 单账号时返回唯一一行；多账号时调用方应使用 List 按 env 汇总，此处兼容返回第一行
	return &list[0], nil
}

// ListCloudBillAggregateForReportPeriod 返回指定 report_type+period_key 下所有 account 的聚合行。[Ref: 01_设计 §后端数据聚合与存储方案]
func (p *PGRepository) ListCloudBillAggregateForReportPeriod(ctx context.Context, reportType, periodKey string) ([]CloudBillAggregate, error) {
	rows, err := p.db.QueryContext(ctx,
		`SELECT total_amount, product_breakdown, last_success_at, created_at, updated_at, COALESCE(account_id,'') FROM cost_cloud_bill_aggregate WHERE report_type = $1 AND period_key = $2`,
		reportType, periodKey)
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
		var accountID string
		if err := rows.Scan(&totalAmount, &breakdown, &lastSuccessAt, &createdAt, &updatedAt, &accountID); err != nil {
			return nil, err
		}
		var m map[string]float64
		if len(breakdown) > 0 {
			_ = json.Unmarshal(breakdown, &m)
		}
		if m == nil {
			m = make(map[string]float64)
		}
		var last *time.Time
		if lastSuccessAt.Valid {
			last = &lastSuccessAt.Time
		}
		out = append(out, CloudBillAggregate{
			ReportType:       reportType,
			PeriodKey:        periodKey,
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

func (p *PGRepository) DeleteCloudBillAggregateExcept(ctx context.Context, reportType string, keepPeriodKeys []string) error {
	if len(keepPeriodKeys) == 0 {
		_, err := p.db.ExecContext(ctx, `DELETE FROM cost_cloud_bill_aggregate WHERE report_type = $1`, reportType)
		return err
	}
	// keepPeriodKeys 占位构建 IN 子句
	args := []interface{}{reportType}
	for _, k := range keepPeriodKeys {
		args = append(args, k)
	}
	placeholders := ""
	for i := 0; i < len(keepPeriodKeys); i++ {
		if i > 0 {
			placeholders += ","
		}
		placeholders += fmt.Sprintf("$%d", i+2)
	}
	_, err := p.db.ExecContext(ctx, `DELETE FROM cost_cloud_bill_aggregate WHERE report_type = $1 AND period_key NOT IN (`+placeholders+`)`, args...)
	return err
}

// ListCloudBillDailyRawFromTo 按日期范围查询日原始表。[Ref: D8-6] 严格按 bill_date 索引（主键）只取 [from,to] 所选日期，单次查询完成，无全表扫描。
func (p *PGRepository) ListCloudBillDailyRawFromTo(ctx context.Context, from, to time.Time) ([]CloudBillDailyRaw, error) {
	rows, err := p.db.QueryContext(ctx,
		`SELECT bill_date, total_amount, product_breakdown, snapshot_at, created_at FROM cost_cloud_bill_daily_raw WHERE bill_date >= $1::date AND bill_date <= $2::date ORDER BY bill_date`,
		from.Format("2006-01-02"), to.Format("2006-01-02"))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []CloudBillDailyRaw
	for rows.Next() {
		var billDate time.Time
		var totalAmount float64
		var breakdown []byte
		var snapshotAt, createdAt time.Time
		if err := rows.Scan(&billDate, &totalAmount, &breakdown, &snapshotAt, &createdAt); err != nil {
			return nil, err
		}
		var m map[string]float64
		if len(breakdown) > 0 {
			_ = json.Unmarshal(breakdown, &m)
		}
		if m == nil {
			m = make(map[string]float64)
		}
		out = append(out, CloudBillDailyRaw{BillDate: billDate, TotalAmount: totalAmount, ProductBreakdown: m, SnapshotAt: snapshotAt, CreatedAt: createdAt})
	}
	return out, rows.Err()
}

// --- HealthCheck ---

func (p *PGRepository) HealthCheck(ctx context.Context) error {
	return p.db.PingContext(ctx)
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
func (r *pgTxRepository) GetCloudBillDailyRaw(ctx context.Context, billDate time.Time) (*CloudBillDailyRaw, error) {
	return r.parent.GetCloudBillDailyRaw(ctx, billDate)
}
func (r *pgTxRepository) DeleteCloudBillDailyRawForDate(ctx context.Context, billDate time.Time) error {
	return r.parent.DeleteCloudBillDailyRawForDate(ctx, billDate)
}
func (r *pgTxRepository) ListMissingCloudBillDailyDates(ctx context.Context, from, to time.Time) ([]time.Time, error) {
	return r.parent.ListMissingCloudBillDailyDates(ctx, from, to)
}
func (r *pgTxRepository) SaveCloudBillMonthlyRaw(ctx context.Context, r2 CloudBillMonthlyRaw) error {
	return r.parent.SaveCloudBillMonthlyRaw(ctx, r2)
}
func (r *pgTxRepository) GetCloudBillMonthlyRaw(ctx context.Context, billingCycle string) (*CloudBillMonthlyRaw, error) {
	return r.parent.GetCloudBillMonthlyRaw(ctx, billingCycle)
}
func (r *pgTxRepository) SaveCloudBillAggregate(ctx context.Context, a CloudBillAggregate) error {
	return r.parent.SaveCloudBillAggregate(ctx, a)
}
func (r *pgTxRepository) GetCloudBillAggregate(ctx context.Context, reportType, periodKey string) (*CloudBillAggregate, error) {
	return r.parent.GetCloudBillAggregate(ctx, reportType, periodKey)
}
func (r *pgTxRepository) DeleteCloudBillAggregateExcept(ctx context.Context, reportType string, keepPeriodKeys []string) error {
	return r.parent.DeleteCloudBillAggregateExcept(ctx, reportType, keepPeriodKeys)
}
func (r *pgTxRepository) ListCloudBillDailyRawFromTo(ctx context.Context, from, to time.Time) ([]CloudBillDailyRaw, error) {
	return r.parent.ListCloudBillDailyRawFromTo(ctx, from, to)
}
func (r *pgTxRepository) ListEnvAccountConfig(ctx context.Context) ([]EnvAccountConfig, error) {
	return r.parent.ListEnvAccountConfig(ctx)
}
func (r *pgTxRepository) GetProductCategory(ctx context.Context, productCode string) (string, bool) {
	return r.parent.GetProductCategory(ctx, productCode)
}
func (r *pgTxRepository) ListCloudBillAggregateForReportPeriod(ctx context.Context, reportType, periodKey string) ([]CloudBillAggregate, error) {
	return r.parent.ListCloudBillAggregateForReportPeriod(ctx, reportType, periodKey)
}
func (r *pgTxRepository) HealthCheck(ctx context.Context) error { return r.parent.HealthCheck(ctx) }
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

// 确保类型实现接口（编译期检查）
var _ Repository = (*PGRepository)(nil)
