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
