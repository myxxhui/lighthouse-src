// [Ref: Phase6 finops_billing_fact OLAP] 财务事实表读写
package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// sqlExec 支持 *sql.DB 与 *sql.Tx 上执行 ExecContext。[Ref: 04_采集 §六]
type sqlExec interface {
	ExecContext(ctx context.Context, query string, args ...interface{}) (sql.Result, error)
}

// DeleteFinOpsBillingFactsByBillingCycle 显式清空某账期（如关账全量重灌前）；日常 OSS 同步用 BulkInsertFinOpsBillingFacts 行级 UPSERT。[Ref: 04_采集 §5.6]
func (p *PGRepository) DeleteFinOpsBillingFactsByBillingCycle(ctx context.Context, billingCycle, accountID string) error {
	_, err := p.db.ExecContext(ctx,
		`DELETE FROM finops_billing_fact WHERE billing_cycle = $1 AND COALESCE(account_id,'') = $2`,
		billingCycle, accountID)
	return err
}

// BulkInsertFinOpsBillingFacts 批量 UPSERT（每批最多 1000 行），ON CONFLICT (account_id, dedup_key)。[Ref: 04_采集 §5.6]
func (p *PGRepository) BulkInsertFinOpsBillingFacts(ctx context.Context, rows []FinOpsBillingFactRow) error {
	const batchSize = 1000
	for i := 0; i < len(rows); i += batchSize {
		j := i + batchSize
		if j > len(rows) {
			j = len(rows)
		}
		if err := p.insertFinOpsBatch(ctx, p.db, rows[i:j]); err != nil {
			return err
		}
	}
	return nil
}

// ReplaceFinOpsBillingCycleWithFacts 关账全量替换：DELETE + INSERT（UPSERT 批），单事务。[Ref: 04_采集 §六]
func (p *PGRepository) ReplaceFinOpsBillingCycleWithFacts(ctx context.Context, billingCycle, accountID string, rows []FinOpsBillingFactRow) error {
	tx, err := p.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	_, err = tx.ExecContext(ctx,
		`DELETE FROM finops_billing_fact WHERE billing_cycle = $1 AND COALESCE(account_id,'') = $2`,
		billingCycle, accountID)
	if err != nil {
		return err
	}
	const batchSize = 1000
	for i := 0; i < len(rows); i += batchSize {
		j := i + batchSize
		if j > len(rows) {
			j = len(rows)
		}
		if err := p.insertFinOpsBatch(ctx, tx, rows[i:j]); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (p *PGRepository) insertFinOpsBatch(ctx context.Context, execer sqlExec, rows []FinOpsBillingFactRow) error {
	if len(rows) == 0 {
		return nil
	}
	var sb strings.Builder
	sb.WriteString(`INSERT INTO finops_billing_fact (billing_cycle, usage_date, account_alias, account_id, env, product_code, instance_id, item_code, amount, currency, tags_json, source_object, dedup_key) VALUES `)
	args := make([]interface{}, 0, len(rows)*13)
	base := 1
	for i, r := range rows {
		if i > 0 {
			sb.WriteByte(',')
		}
		fmt.Fprintf(&sb, "($%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d)",
			base, base+1, base+2, base+3, base+4, base+5, base+6, base+7, base+8, base+9, base+10, base+11, base+12)
		base += 13
		ud := r.UsageDate.UTC()
		var tags interface{}
		if len(r.TagsJSON) > 0 {
			tags = r.TagsJSON
		}
		args = append(args,
			r.BillingCycle, ud,
			nullStr(r.AccountAlias, ""), nullStr(r.AccountID, ""), nullStr(r.Env, "UNTAGGED"),
			nullStr(r.ProductCode, ""), nullStr(r.InstanceID, ""), nullStr(r.ItemCode, ""),
			r.Amount, nullStr(r.Currency, "CNY"),
			tags, nullStr(r.SourceObject, ""), r.DedupKey,
		)
	}
	sb.WriteString(` ON CONFLICT (account_id, dedup_key) DO UPDATE SET
		billing_cycle = EXCLUDED.billing_cycle,
		usage_date = EXCLUDED.usage_date,
		account_alias = EXCLUDED.account_alias,
		env = EXCLUDED.env,
		product_code = EXCLUDED.product_code,
		instance_id = EXCLUDED.instance_id,
		item_code = EXCLUDED.item_code,
		amount = EXCLUDED.amount,
		currency = EXCLUDED.currency,
		tags_json = EXCLUDED.tags_json,
		source_object = EXCLUDED.source_object,
		ingested_at = NOW()`)
	_, err := execer.ExecContext(ctx, sb.String(), args...)
	return err
}

// CountFinOpsBillingFactsInDateRange 区间内事实行数（用于判断是否启用 OLAP 聚合）。[Ref: Phase6]
func (p *PGRepository) CountFinOpsBillingFactsInDateRange(ctx context.Context, from, to time.Time, accountIDs []string) (int64, error) {
	fromStr := from.UTC().Truncate(24 * time.Hour).Format("2006-01-02")
	toStr := to.UTC().Truncate(24 * time.Hour).Format("2006-01-02")
	q := `SELECT COUNT(*) FROM finops_billing_fact WHERE usage_date >= $1::date AND usage_date <= $2::date`
	args := []interface{}{fromStr, toStr}
	if len(accountIDs) > 0 {
		ph := make([]string, 0, len(accountIDs))
		for i, id := range accountIDs {
			ph = append(ph, fmt.Sprintf("$%d", i+3))
			args = append(args, id)
		}
		q += fmt.Sprintf(" AND COALESCE(account_id,'') IN (%s)", strings.Join(ph, ","))
	}
	var n int64
	err := p.db.QueryRowContext(ctx, q, args...).Scan(&n)
	if err != nil {
		return 0, err
	}
	return n, nil
}

// SumFinOpsFactPretaxCGByDateRange 与 line_items 语义一致：C=正数 amount 和，G=负数 amount 和。[Ref: Phase6 P+G+U=C+B]
func (p *PGRepository) SumFinOpsFactPretaxCGByDateRange(ctx context.Context, from, to time.Time, accountIDs []string) (c, g float64, err error) {
	fromStr := from.UTC().Truncate(24 * time.Hour).Format("2006-01-02")
	toStr := to.UTC().Truncate(24 * time.Hour).Format("2006-01-02")
	q := `SELECT COALESCE(SUM(CASE WHEN amount > 0 THEN amount ELSE 0 END), 0),
	             COALESCE(SUM(CASE WHEN amount < 0 THEN amount ELSE 0 END), 0)
	      FROM finops_billing_fact WHERE usage_date >= $1::date AND usage_date <= $2::date`
	args := []interface{}{fromStr, toStr}
	if len(accountIDs) > 0 {
		ph := make([]string, 0, len(accountIDs))
		for i, id := range accountIDs {
			ph = append(ph, fmt.Sprintf("$%d", i+3))
			args = append(args, id)
		}
		q += fmt.Sprintf(" AND COALESCE(account_id,'') IN (%s)", strings.Join(ph, ","))
	}
	var cVal, gVal sql.NullFloat64
	err = p.db.QueryRowContext(ctx, q, args...).Scan(&cVal, &gVal)
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

// SumFinOpsFactPretaxTotalByDateRange amount 代数和（对账用）。[Ref: Phase6 reconciliation]
func (p *PGRepository) SumFinOpsFactPretaxTotalByDateRange(ctx context.Context, from, to time.Time, accountIDs []string) (float64, error) {
	fromStr := from.UTC().Truncate(24 * time.Hour).Format("2006-01-02")
	toStr := to.UTC().Truncate(24 * time.Hour).Format("2006-01-02")
	q := `SELECT COALESCE(SUM(amount), 0) FROM finops_billing_fact WHERE usage_date >= $1::date AND usage_date <= $2::date`
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

// GetFinOpsOSSSyncCheckpoint 读取 OSS 列举增量水位；无行时 found=false。[Ref: 04_采集 §七]
func (p *PGRepository) GetFinOpsOSSSyncCheckpoint(ctx context.Context, accountID string) (time.Time, bool, error) {
	var t time.Time
	err := p.db.QueryRowContext(ctx,
		`SELECT max_object_last_modified FROM finops_oss_sync_checkpoint WHERE account_id = $1`,
		accountID).Scan(&t)
	if errors.Is(err, sql.ErrNoRows) {
		return time.Time{}, false, nil
	}
	if err != nil {
		return time.Time{}, false, err
	}
	return t.UTC(), true, nil
}

// SetFinOpsOSSSyncCheckpoint 写入 OSS 列举增量水位（成功处理一批对象后更新）。[Ref: 04_采集 §七]
func (p *PGRepository) SetFinOpsOSSSyncCheckpoint(ctx context.Context, accountID string, maxObjectLastModified time.Time) error {
	_, err := p.db.ExecContext(ctx,
		`INSERT INTO finops_oss_sync_checkpoint (account_id, max_object_last_modified, updated_at)
		 VALUES ($1, $2, NOW())
		 ON CONFLICT (account_id) DO UPDATE SET
		   max_object_last_modified = EXCLUDED.max_object_last_modified,
		   updated_at = NOW()`,
		accountID, maxObjectLastModified.UTC())
	return err
}

func (r *pgTxRepository) GetFinOpsOSSSyncCheckpoint(ctx context.Context, accountID string) (time.Time, bool, error) {
	return r.parent.GetFinOpsOSSSyncCheckpoint(ctx, accountID)
}
func (r *pgTxRepository) SetFinOpsOSSSyncCheckpoint(ctx context.Context, accountID string, maxObjectLastModified time.Time) error {
	return r.parent.SetFinOpsOSSSyncCheckpoint(ctx, accountID, maxObjectLastModified)
}

func (r *pgTxRepository) DeleteFinOpsBillingFactsByBillingCycle(ctx context.Context, billingCycle, accountID string) error {
	return r.parent.DeleteFinOpsBillingFactsByBillingCycle(ctx, billingCycle, accountID)
}
func (r *pgTxRepository) BulkInsertFinOpsBillingFacts(ctx context.Context, rows []FinOpsBillingFactRow) error {
	return r.parent.BulkInsertFinOpsBillingFacts(ctx, rows)
}
func (r *pgTxRepository) ReplaceFinOpsBillingCycleWithFacts(ctx context.Context, billingCycle, accountID string, rows []FinOpsBillingFactRow) error {
	return r.parent.ReplaceFinOpsBillingCycleWithFacts(ctx, billingCycle, accountID, rows)
}
func (r *pgTxRepository) CountFinOpsBillingFactsInDateRange(ctx context.Context, from, to time.Time, accountIDs []string) (int64, error) {
	return r.parent.CountFinOpsBillingFactsInDateRange(ctx, from, to, accountIDs)
}
func (r *pgTxRepository) SumFinOpsFactPretaxCGByDateRange(ctx context.Context, from, to time.Time, accountIDs []string) (c, g float64, err error) {
	return r.parent.SumFinOpsFactPretaxCGByDateRange(ctx, from, to, accountIDs)
}
func (r *pgTxRepository) SumFinOpsFactPretaxTotalByDateRange(ctx context.Context, from, to time.Time, accountIDs []string) (float64, error) {
	return r.parent.SumFinOpsFactPretaxTotalByDateRange(ctx, from, to, accountIDs)
}
