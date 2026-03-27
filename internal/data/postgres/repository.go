// Package postgres provides repository implementations for PostgreSQL storage.
package postgres

import (
	"context"
	"time"

	"github.com/myxxhui/lighthouse-src/pkg/costmodel"
)

// Repository defines the interface for PostgreSQL data storage operations.
type Repository interface {
	// CostSnapshot operations
	SaveCostSnapshot(ctx context.Context, snapshot CostSnapshot) error
	GetCostSnapshot(ctx context.Context, id string) (*CostSnapshot, error)
	ListCostSnapshots(ctx context.Context, filter CostSnapshotFilter) ([]CostSnapshot, error)
	DeleteCostSnapshot(ctx context.Context, id string) error

	// ROIBaseline operations
	SaveROIBaseline(ctx context.Context, baseline ROIBaseline) error
	GetROIBaseline(ctx context.Context, id string) (*ROIBaseline, error)
	ListROIBaselines(ctx context.Context, filter ROIBaselineFilter) ([]ROIBaseline, error)
	DeleteROIBaseline(ctx context.Context, id string) error

	// DailyNamespaceCost operations
	SaveDailyNamespaceCost(ctx context.Context, cost DailyNamespaceCost) error
	GetDailyNamespaceCost(ctx context.Context, namespace string, date time.Time) (*DailyNamespaceCost, error)
	ListDailyNamespaceCosts(ctx context.Context, filter DailyNamespaceCostFilter) ([]DailyNamespaceCost, error)
	AggregateDailyNamespaceCosts(ctx context.Context, startDate, endDate time.Time) ([]DailyNamespaceCost, error)

	// HourlyWorkloadStat operations
	SaveHourlyWorkloadStat(ctx context.Context, stat HourlyWorkloadStat) error
	GetHourlyWorkloadStat(ctx context.Context, namespace, workloadName string, timestamp time.Time) (*HourlyWorkloadStat, error)
	ListHourlyWorkloadStats(ctx context.Context, filter HourlyWorkloadStatFilter) ([]HourlyWorkloadStat, error)
	AggregateHourlyWorkloadStats(ctx context.Context, startTime, endTime time.Time) ([]HourlyWorkloadStat, error)

	// Metadata operations
	SaveMetadata(ctx context.Context, metadata Metadata) error
	GetMetadata(ctx context.Context, key string) (*Metadata, error)
	ListMetadata(ctx context.Context, filter MetadataFilter) ([]Metadata, error)
	DeleteMetadata(ctx context.Context, key string) error

	// CloudBillSummary (cost_cloud_bill_summary, 06_ §2.1) — Phase4 01_ 成本透视真实数据
	SaveCloudBillSummary(ctx context.Context, s CloudBillSummary) error
	GetCloudBillSummary(ctx context.Context, day time.Time, billingCycle string) (*CloudBillSummary, error)
	GetLatestCloudBillSummary(ctx context.Context) (*CloudBillSummary, error)
	// GetLatestCloudBillSummaryForBillingCycle 返回指定账期最近一条汇总（用于按时间范围返回真实数据）
	GetLatestCloudBillSummaryForBillingCycle(ctx context.Context, billingCycle string) (*CloudBillSummary, error)
	// GetCloudBillSummariesForBillingCycles 返回多个账期各自最近一条汇总（用于本季度聚合）
	GetCloudBillSummariesForBillingCycles(ctx context.Context, billingCycles []string) ([]*CloudBillSummary, error)

	// [Ref: 06_ 成本云账单三表 D2] 日/月原始与聚合表（ETL 五步流水线）；月/日表主键含 account_id，多环境各写一行 [Ref: 01_多环境 UAT]
	SaveCloudBillDailyRaw(ctx context.Context, r CloudBillDailyRaw) error
	GetCloudBillDailyRaw(ctx context.Context, billDate time.Time, accountID string) (*CloudBillDailyRaw, error)
	DeleteCloudBillDailyRawForDate(ctx context.Context, billDate time.Time, accountID string) error
	ListMissingCloudBillDailyDates(ctx context.Context, from, to time.Time, accountID string) ([]time.Time, error)
	SaveCloudBillMonthlyRaw(ctx context.Context, r CloudBillMonthlyRaw) error
	GetCloudBillMonthlyRaw(ctx context.Context, billingCycle, accountID string) (*CloudBillMonthlyRaw, error)
	// ListCloudBillMonthlyRawByCycle 返回指定账期下所有 account 的月原始行，供 cost_service 多账户汇总 [Ref: 01_多环境 UAT]
	ListCloudBillMonthlyRawByCycle(ctx context.Context, billingCycle string) ([]CloudBillMonthlyRaw, error)
	// DeleteCloudBillMonthlyRawOlderThan 删除 billing_cycle < cutoff 的该 account 月原始数据。[Ref: 01_实践 月表保留由配置控制]
	DeleteCloudBillMonthlyRawOlderThan(ctx context.Context, cutoffBillingCycle string, accountID string) error
	SaveCloudBillAggregate(ctx context.Context, a CloudBillAggregate) error
	GetCloudBillAggregate(ctx context.Context, reportType, periodKey string) (*CloudBillAggregate, error)
	// DeleteCloudBillAggregateExcept 删除指定 report_type 下 period_key 不在 keepPeriodKeys 中的行；accountID 非空时仅删该账号，多环境互不覆盖 [Ref: 01_多环境 UAT]
	DeleteCloudBillAggregateExcept(ctx context.Context, reportType string, keepPeriodKeys []string, accountID string) error
	// ListCloudBillDailyRawFromTo accountID 为空时返回该日期范围内所有 account 的行；非空时仅该 account [Ref: 01_多环境 UAT]
	ListCloudBillDailyRawFromTo(ctx context.Context, from, to time.Time, accountID string) ([]CloudBillDailyRaw, error)

	// [Ref: 16_云账单动态对账与高可靠处理规范 §三] 行级流水 Upsert（ON CONFLICT record_id DO UPDATE）
	UpsertCloudBillLineItem(ctx context.Context, item CloudBillLineItem) error
	// ListCloudBillLineItemsByDate 返回指定日期（含）的所有流水条目（含负数冲正）
	ListCloudBillLineItemsByDate(ctx context.Context, billDate time.Time, accountID string) ([]CloudBillLineItem, error)
	// ListCloudBillLineItemsByBillingCycle 返回指定账期+账号的所有流水条目（用于按 billing_cycle 汇总月原始表，回退归属到被冲正账期）[Ref: 16_ §四、§七]
	ListCloudBillLineItemsByBillingCycle(ctx context.Context, billingCycle, accountID string) ([]CloudBillLineItem, error)
	// ListDistinctBillingCyclesInDateRange 返回日期范围内有流水的所有账期（用于步骤⑧按窗口重算月表，发现冲正即更新对应月）[Ref: 16_ §七 结合方案]
	ListDistinctBillingCyclesInDateRange(ctx context.Context, from, to time.Time, accountID string) ([]string, error)
	// SumLineItemsCashByBillingCycle 计算指定账期所有条目的 CashAmount 代数和（含负数）
	SumLineItemsCashByBillingCycle(ctx context.Context, billingCycle, accountID string) (float64, error)
	// SumLineItemsPretaxCGByDateRange 按 bill_date 在区间内汇总 C=SUM(pretax_amount) WHERE pretax_amount>0，G=SUM(pretax_amount) WHERE pretax_amount<0。[Ref: 03_Phase6/01_FinOps 采集与ETL]
	SumLineItemsPretaxCGByDateRange(ctx context.Context, from, to time.Time, accountIDs []string) (c, g float64, err error)
	// SumLineItemsPretaxCGByDateRangePreferOSS 若区间内存在 oss_detail 行则仅汇总该渠道，否则汇总全部（含 api_query_account_bill）。[Ref: 03_Phase6/01_FinOps]
	SumLineItemsPretaxCGByDateRangePreferOSS(ctx context.Context, from, to time.Time, accountIDs []string) (c, g float64, err error)
	// SumLineItemsPretaxCGByDateRangeWithChannel 仅汇总指定 ingestion_channel 的 C/G（FINOPS_CG_SOURCE 单变量语义）。[Ref: 03_Phase6/01_FinOps]
	SumLineItemsPretaxCGByDateRangeWithChannel(ctx context.Context, from, to time.Time, accountIDs []string, channel string) (c, g float64, err error)
	// SumPretaxByChannelForDateRange 按 ingestion_channel 过滤汇总 C/G（用于 OSS vs API 对账）。[Ref: 03_Phase6/01_FinOps]
	SumPretaxByChannelForDateRange(ctx context.Context, from, to time.Time, accountIDs []string, channel string) (pretaxSum float64, err error)

	// [Ref: 03_Phase6/01_FinOps] BSS 实付流水与余额、账期应付
	UpsertBSSTransaction(ctx context.Context, tx BSSTransactionRow) error
	UpsertBSSBalanceSnapshot(ctx context.Context, s BSSBalanceSnapshotRow) error
	UpsertBillOutstandingMonthly(ctx context.Context, o BillOutstandingMonthlyRow) error
	// SumBSSPaymentExpenseByDateRange 汇总区间内 Payment+Expense 类流水金额（实付 P，取绝对值之和）。[Ref: 03_Phase6/01_FinOps]
	SumBSSPaymentExpenseByDateRange(ctx context.Context, from, to time.Time, accountIDs []string) (p float64, err error)
	// LatestBSSBalanceSum 各 account 在 asOf 日前最近一条快照的 available 之和（B）。[Ref: 03_Phase6/01_FinOps]
	LatestBSSBalanceSum(ctx context.Context, accountIDs []string, asOf time.Time) (b float64, err error)
	// LatestBSSBalanceMap 各 account 在 asOf 日前最近一条快照的 available，供按环境去重汇总 B（与 Hero 同键）。[Ref: 03_Phase6/01_FinOps]
	LatestBSSBalanceMap(ctx context.Context, asOf time.Time) (map[string]float64, error)
	// SumOutstandingByBillingCycles 多账期 outstanding 之和（U）。[Ref: 03_Phase6/01_FinOps]
	SumOutstandingByBillingCycles(ctx context.Context, billingCycles []string, accountIDs []string) (u float64, err error)
	// ListBillOutstandingInBillingCycles 返回账期列表内全部应付行，供按环境去重汇总 U。[Ref: 03_Phase6/01_FinOps]
	ListBillOutstandingInBillingCycles(ctx context.Context, billingCycles []string) ([]BillOutstandingMonthlyRow, error)
	// SumMonthlyCashTotalForBillingCycles 月表现金合计（BSS 无流水时 P 的降级来源）。[Ref: 03_Phase6/01_FinOps]
	SumMonthlyCashTotalForBillingCycles(ctx context.Context, billingCycles []string, accountIDs []string) (float64, error)
	// [Ref: Phase6 finops_billing_fact OLAP] 阿里云账单 CSV → 事实表；C/G 聚合优先于 line_items
	DeleteFinOpsBillingFactsByBillingCycle(ctx context.Context, billingCycle, accountID string) error
	BulkInsertFinOpsBillingFacts(ctx context.Context, rows []FinOpsBillingFactRow) error
	// ReplaceFinOpsBillingCycleWithFacts 关账全量：单事务内 DELETE 该账期该账号后批量写入，消除滚动快照幽灵行。[Ref: 04_采集 §六]
	ReplaceFinOpsBillingCycleWithFacts(ctx context.Context, billingCycle, accountID string, rows []FinOpsBillingFactRow) error
	// GetFinOpsOSSSyncCheckpoint / SetFinOpsOSSSyncCheckpoint — OSS 增量同步水位（与 OSS_INCREMENTAL_SYNC）。[Ref: 04_采集 §七]
	GetFinOpsOSSSyncCheckpoint(ctx context.Context, accountID string) (maxObjectLastModified time.Time, found bool, err error)
	SetFinOpsOSSSyncCheckpoint(ctx context.Context, accountID string, maxObjectLastModified time.Time) error
	CountFinOpsBillingFactsInDateRange(ctx context.Context, from, to time.Time, accountIDs []string) (int64, error)
	SumFinOpsFactPretaxCGByDateRange(ctx context.Context, from, to time.Time, accountIDs []string) (c, g float64, err error)
	SumFinOpsFactPretaxTotalByDateRange(ctx context.Context, from, to time.Time, accountIDs []string) (float64, error)
	// DeleteLineItemsOlderThan 清理早于指定日期的流水条目（配合 daily_raw 10 个月滑动清理）
	DeleteLineItemsOlderThan(ctx context.Context, before time.Time, accountID string) error

	// [Ref: 16_ §三] 月度对账状态 CRUD
	UpsertCloudBillMonthStatus(ctx context.Context, s CloudBillMonthStatus) error
	GetCloudBillMonthStatus(ctx context.Context, billingCycle, accountID string) (*CloudBillMonthStatus, error)

	// [Ref: 01_设计 §环境与云账号配置 D9-3] 环境与产品配置
	ListEnvAccountConfig(ctx context.Context) ([]EnvAccountConfig, error)
	// UpdateEnvAccountConfigAccountID 将 BSS 解析的阿里云主账号 ID 写回 cost_env_account_config，与 ETL account_id 主键对齐。[Ref: 03_Phase6/01_FinOps]
	UpdateEnvAccountConfigAccountID(ctx context.Context, environment, aliyunAccountID string) error
	GetProductCategory(ctx context.Context, productCode string) (category string, ok bool)
	UpsertProductCategory(ctx context.Context, productCode, category string) error
	// ListCloudBillAggregateForReportPeriod 返回指定 report_type+period_key+metric_type 下 account 的聚合行。
	// accountIDs 非空时仅返回其内 account；nil 则返回所有 account。[Ref: 聚合表主路径 方案A]
	ListCloudBillAggregateForReportPeriod(ctx context.Context, reportType, periodKey, metricType string, accountIDs []string) ([]CloudBillAggregate, error)

	// HealthCheck checks if the database is reachable.
	HealthCheck(ctx context.Context) error

	// FinOpsSyncJob 主动同步 Job（与部署配置一致的异步拉取+流水线）。[Ref: 03_Phase6/01_FinOps 主动同步]
	InsertFinOpsSyncJob(ctx context.Context, j FinOpsSyncJobRow) (id int64, err error)
	UpdateFinOpsSyncJob(ctx context.Context, j FinOpsSyncJobRow) error
	GetFinOpsSyncJob(ctx context.Context, id int64) (*FinOpsSyncJobRow, error)
	CountActiveFinOpsSyncJobs(ctx context.Context) (int64, error)
	// GetActiveFinOpsSyncJobID 当前 queued/running 的 Job id（至多一个活跃；冲突 409 时供前端轮询）。[Ref: 03_Phase6/01_FinOps 主动同步]
	GetActiveFinOpsSyncJobID(ctx context.Context) (int64, error)

	// Transaction operations
	BeginTx(ctx context.Context) (Transaction, error)
}

// Transaction represents a database transaction.
type Transaction interface {
	Commit() error
	Rollback() error
	Repository() Repository
}

// CostSnapshot represents a saved cost calculation result.
type CostSnapshot struct {
	ID                     string                                                       `json:"id"`
	CalculationID          string                                                       `json:"calculation_id"`
	Timestamp              time.Time                                                    `json:"timestamp"`
	TimeRangeStart         time.Time                                                    `json:"time_range_start"`
	TimeRangeEnd           time.Time                                                    `json:"time_range_end"`
	ResourceResults        []costmodel.CostResult                                       `json:"resource_results"`
	AggregatedResults      map[costmodel.AggregationLevel][]costmodel.AggregationResult `json:"aggregated_results"`
	TotalBillableCost      float64                                                      `json:"total_billable_cost"`
	TotalUsageCost         float64                                                      `json:"total_usage_cost"`
	TotalWasteCost         float64                                                      `json:"total_waste_cost"`
	OverallEfficiencyScore float64                                                      `json:"overall_efficiency_score"`
	ZombieCount            int                                                          `json:"zombie_count"`
	OverProvisionedCount   int                                                          `json:"over_provisioned_count"`
	HealthyCount           int                                                          `json:"healthy_count"`
	RiskCount              int                                                          `json:"risk_count"`
	Metadata               map[string]interface{}                                       `json:"metadata"`
	CreatedAt              time.Time                                                    `json:"created_at"`
	UpdatedAt              time.Time                                                    `json:"updated_at"`
}

// CostSnapshotFilter defines filtering options for cost snapshots.
type CostSnapshotFilter struct {
	CalculationID string    `json:"calculation_id"`
	StartTime     time.Time `json:"start_time"`
	EndTime       time.Time `json:"end_time"`
	MinTotalCost  float64   `json:"min_total_cost"`
	MaxTotalCost  float64   `json:"max_total_cost"`
	Limit         int       `json:"limit"`
	Offset        int       `json:"offset"`
}

// ROIBaseline represents a Return on Investment baseline for comparison.
type ROIBaseline struct {
	ID              string                 `json:"id"`
	Name            string                 `json:"name"`
	Description     string                 `json:"description"`
	BaselineType    string                 `json:"baseline_type"` // "historical", "target", "industry"
	TimePeriodStart time.Time              `json:"time_period_start"`
	TimePeriodEnd   time.Time              `json:"time_period_end"`
	Metrics         map[string]float64     `json:"metrics"` // e.g., "efficiency_score": 0.85, "waste_percentage": 0.15
	ReferenceData   map[string]interface{} `json:"reference_data"`
	CreatedBy       string                 `json:"created_by"`
	CreatedAt       time.Time              `json:"created_at"`
	UpdatedAt       time.Time              `json:"updated_at"`
}

// ROIBaselineFilter defines filtering options for ROI baselines.
type ROIBaselineFilter struct {
	Name         string    `json:"name"`
	BaselineType string    `json:"baseline_type"`
	StartDate    time.Time `json:"start_date"`
	EndDate      time.Time `json:"end_date"`
	Limit        int       `json:"limit"`
	Offset       int       `json:"offset"`
}

// DailyNamespaceCost represents daily aggregated cost data for a namespace.
type DailyNamespaceCost struct {
	Namespace       string    `json:"namespace"`
	Date            time.Time `json:"date"`
	BillableCost    float64   `json:"billable_cost"`
	UsageCost       float64   `json:"usage_cost"`
	WasteCost       float64   `json:"waste_cost"`
	PodCount        int       `json:"pod_count"`
	NodeCount       int       `json:"node_count"`
	WorkloadCount   int       `json:"workload_count"`
	EfficiencyScore float64   `json:"efficiency_score"`
	CreatedAt       time.Time `json:"created_at"`
}

// DailyNamespaceCostFilter defines filtering options for daily namespace costs.
type DailyNamespaceCostFilter struct {
	Namespace     string    `json:"namespace"`
	StartDate     time.Time `json:"start_date"`
	EndDate       time.Time `json:"end_date"`
	MinEfficiency float64   `json:"min_efficiency"`
	MaxEfficiency float64   `json:"max_efficiency"`
	Limit         int       `json:"limit"`
	Offset        int       `json:"offset"`
}

// HourlyWorkloadStat represents hourly statistics for a workload.
type HourlyWorkloadStat struct {
	Namespace         string    `json:"namespace"`
	WorkloadName      string    `json:"workload_name"`
	WorkloadType      string    `json:"workload_type"`
	NodeName          string    `json:"node_name"`
	PodName           string    `json:"pod_name"`
	Timestamp         time.Time `json:"timestamp"`
	CPURequest        float64   `json:"cpu_request"`
	CPUUsageP95       float64   `json:"cpu_usage_p95"`
	MemRequest        int64     `json:"mem_request"`
	MemUsageP95       int64     `json:"mem_usage_p95"`
	CPUBillableCost   float64   `json:"cpu_billable_cost"`
	CPUUsageCost      float64   `json:"cpu_usage_cost"`
	CPUWasteCost      float64   `json:"cpu_waste_cost"`
	MemBillableCost   float64   `json:"mem_billable_cost"`
	MemUsageCost      float64   `json:"mem_usage_cost"`
	MemWasteCost      int64     `json:"mem_waste_cost"`
	TotalBillableCost float64   `json:"total_billable_cost"`
	TotalUsageCost    float64   `json:"total_usage_cost"`
	TotalWasteCost    float64   `json:"total_waste_cost"`
}

// HourlyWorkloadStatFilter defines filtering options for hourly workload stats.
type HourlyWorkloadStatFilter struct {
	Namespace    string    `json:"namespace"`
	WorkloadName string    `json:"workload_name"`
	NodeName     string    `json:"node_name"`
	StartTime    time.Time `json:"start_time"`
	EndTime      time.Time `json:"end_time"`
	Limit        int       `json:"limit"`
	Offset       int       `json:"offset"`
}

// Metadata represents generic key-value metadata storage.
type Metadata struct {
	Key         string                 `json:"key"`
	Value       map[string]interface{} `json:"value"`
	Description string                 `json:"description"`
	CreatedBy   string                 `json:"created_by"`
	CreatedAt   time.Time              `json:"created_at"`
	UpdatedAt   time.Time              `json:"updated_at"`
}

// MetadataFilter defines filtering options for metadata.
type MetadataFilter struct {
	KeyPrefix string `json:"key_prefix"`
	CreatedBy string `json:"created_by"`
	Limit     int    `json:"limit"`
	Offset    int    `json:"offset"`
}

// CloudBillSummary 云账单汇总（表 cost_cloud_bill_summary，06_ §2.1）。Phase4 01_ 落库目标。
type CloudBillSummary struct {
	Day              time.Time       `json:"day"`
	BillingCycle     string          `json:"billing_cycle"`
	TotalAmount      float64         `json:"total_amount"`
	ProductBreakdown map[string]float64 `json:"product_breakdown"` // domain/category -> amount，用于 domain_breakdown
	CreatedAt        time.Time       `json:"created_at"`
	UpdatedAt        time.Time       `json:"updated_at"`
}

// [Ref: 06_ 成本云账单三表] 日原始表行
// [Ref: 06_ 成本云账单三表] 日原始表行（dual-metric v3）
// TotalAmount/ProductBreakdown        = PretaxAmount（资源消耗价值）
// CashTotalAmount/CashProductBreakdown = CashAmount（资源支付价值）
type CloudBillDailyRaw struct {
	BillDate               time.Time         `json:"bill_date"`
	TotalAmount            float64           `json:"total_amount"`
	ProductBreakdown       map[string]float64 `json:"product_breakdown"`
	CashTotalAmount        float64           `json:"cash_total_amount"`
	CashProductBreakdown   map[string]float64 `json:"cash_product_breakdown"`
	SnapshotAt             time.Time         `json:"snapshot_at"`
	CreatedAt              time.Time         `json:"created_at"`
	AccountID              string            `json:"account_id,omitempty"`
}

// [Ref: 06_ 成本云账单三表] 月原始表行（dual-metric v3）
// TotalAmount/ProductBreakdown        = PretaxAmount（资源消耗价值）
// CashTotalAmount/CashProductBreakdown = CashAmount（资源支付价值）
type CloudBillMonthlyRaw struct {
	BillingCycle           string            `json:"billing_cycle"`
	TotalAmount            float64           `json:"total_amount"`
	ProductBreakdown       map[string]float64 `json:"product_breakdown"`
	CashTotalAmount        float64           `json:"cash_total_amount"`
	CashProductBreakdown   map[string]float64 `json:"cash_product_breakdown"`
	SnapshotAt             time.Time         `json:"snapshot_at"`
	CreatedAt              time.Time         `json:"created_at"`
	AccountID              string            `json:"account_id,omitempty"`
}

// [Ref: 06_ 成本云账单三表] 聚合表行（dual-metric v3）
// MetricType: "consumption"（资源消耗价值，PretaxAmount）| "payment"（资源支付价值，CashAmount）
type CloudBillAggregate struct {
	ReportType       string            `json:"report_type"`
	PeriodKey        string            `json:"period_key"`
	MetricType       string            `json:"metric_type"` // "consumption" | "payment"
	TotalAmount      float64           `json:"total_amount"`
	ProductBreakdown map[string]float64 `json:"product_breakdown"`
	DataStatus       string            `json:"data_status,omitempty"`
	LastSuccessAt    *time.Time        `json:"last_success_at"`
	CreatedAt        time.Time         `json:"created_at"`
	UpdatedAt        time.Time         `json:"updated_at"`
	AccountID        string            `json:"account_id,omitempty"`
}

// [Ref: 16_云账单动态对账与高可靠处理规范 §三] 行级流水明细表行
// RecordID: 阿里云 RecordID 或业务唯一键（SHA256(date|cycle|product|item|suborder|instance)）
// CashAmount: 现金支付金额，含负数冲正条目；ETL 层禁止过滤
type CloudBillLineItem struct {
	RecordID          string    `json:"record_id"`
	BillDate          time.Time `json:"bill_date"`
	BillingCycle      string    `json:"billing_cycle"`
	ProductCode       string    `json:"product_code,omitempty"`
	ProductName       string    `json:"product_name,omitempty"`
	SubOrderID        string    `json:"sub_order_id,omitempty"`
	InstanceID        string    `json:"instance_id,omitempty"`
	BillingItem       string    `json:"billing_item,omitempty"`
	SubscriptionType  string    `json:"subscription_type,omitempty"`
	CashAmount        float64   `json:"cash_amount"`          // 实付现金；含负数
	PretaxAmount      float64   `json:"pretax_amount"`        // 折后应付（辅助）
	PretaxGrossAmount float64   `json:"pretax_gross_amount"`  // 官网原价（辅助）
	Currency          string    `json:"currency,omitempty"`
	IsReversal        bool      `json:"is_reversal"`          // cash_amount < 0 时为 true
	AccountID         string    `json:"account_id,omitempty"`
	IngestionChannel  string    `json:"ingestion_channel,omitempty"` // oss_detail | api_query_account_bill [Ref: 03_Phase6/01_FinOps]
	Region            string    `json:"region,omitempty"`
	SyncedAt          time.Time `json:"synced_at"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}

// BSSTransactionRow [Ref: 03_Phase6/01_FinOps] QueryAccountTransactions 落库
type BSSTransactionRow struct {
	TransactionNumber string
	AccountID         string
	TransactionTime   time.Time
	Amount            float64
	TransactionType   string
	TransactionFlow   string
	RecordID          string
	BillingCycle      string
	Currency          string
}

// BSSBalanceSnapshotRow [Ref: 03_Phase6/01_FinOps] QueryAccountBalance 快照
type BSSBalanceSnapshotRow struct {
	AccountID        string
	SnapshotDate     time.Time
	AvailableAmount  float64
	Currency         string
}

// BillOutstandingMonthlyRow [Ref: 03_Phase6/01_FinOps] QueryAccountBill MONTHLY OutstandingAmount 汇总落库
type BillOutstandingMonthlyRow struct {
	BillingCycle       string
	AccountID          string
	OutstandingAmount  float64
}

// FinOpsSyncJobRow 主动同步 Job 持久化（多实例以 DB 为准）。[Ref: 03_Phase6/01_FinOps 主动同步]
type FinOpsSyncJobRow struct {
	ID             int64      `json:"id"`
	Status         string     `json:"status"` // queued|running|succeeded|succeeded_with_warnings|failed
	Phase          string     `json:"phase"`
	ConfigSnapshot string     `json:"config_snapshot,omitempty"` // JSON
	Warnings       string     `json:"warnings,omitempty"`        // JSON array
	ErrorMessage   string     `json:"error_message,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
	StartedAt      *time.Time `json:"started_at,omitempty"`
	CompletedAt    *time.Time `json:"completed_at,omitempty"`
	DataVersion    int64      `json:"data_version"`
	// ProgressCurrent / ProgressTotal：步骤进度（1 步辅助同步 + 每环境 1 步流水线），非时间占比。[Ref: 03_Phase6/01_FinOps 主动同步]
	ProgressCurrent int    `json:"progress_current"`
	ProgressTotal   int    `json:"progress_total"`
	PhaseDetail     string `json:"phase_detail"`
}

// FinOpsBillingFactRow [Ref: Phase6 OLAP] 账单明细事实行；dedup_key 为稳定业务幂等键（RecordID 或自然键哈希），与 (account_id) 组成 UNIQUE。[Ref: 04_采集 §5.6]
type FinOpsBillingFactRow struct {
	BillingCycle  string
	UsageDate     time.Time
	AccountAlias  string
	AccountID     string
	Env           string
	ProductCode   string
	InstanceID    string
	ItemCode      string
	Amount        float64
	Currency      string
	TagsJSON      []byte // nullable JSON
	SourceObject  string
	DedupKey      string
}

// [Ref: 16_云账单动态对账与高可靠处理规范 §三] 月度对账状态
type CloudBillMonthStatus struct {
	BillingCycle     string     `json:"billing_cycle"`
	AccountID        string     `json:"account_id,omitempty"`
	DataStatus       string     `json:"data_status"` // PRELIMINARY|FINALIZED|DIRTY|RECONCILING
	LineItemsSum     float64    `json:"line_items_sum"`
	MonthlyAPITotal  float64    `json:"monthly_api_total"`
	DriftAmount      float64    `json:"drift_amount"`
	LastReconciledAt *time.Time `json:"last_reconciled_at,omitempty"`
	LastFullSyncAt   *time.Time `json:"last_full_sync_at,omitempty"`
	FinalizedAt      *time.Time `json:"finalized_at,omitempty"`
	Notes            string     `json:"notes,omitempty"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
}

// EnvAccountConfig 环境与云账号映射（cost_env_account_config）。[Ref: 01_设计 §环境与云账号配置]
type EnvAccountConfig struct {
	Environment  string    `json:"environment"`  // POC|FAT|UAT|PROD
	AccountID    string    `json:"account_id"`
	DisplayName  string    `json:"display_name"`
	SortOrder    int       `json:"sort_order"`
	CreatedAt    time.Time `json:"created_at"`
}

// ProductCategoryMapping 云产品与成本分类（product_category_mapping）。[Ref: 01_设计 §产品分类与按环境钻取]
type ProductCategoryMapping struct {
	ProductCode string    `json:"product_code"`
	Category    string    `json:"category"` // compute|network|storage|security
	CreatedAt   time.Time `json:"created_at"`
}

// BillAccountSummary 云账户总账单汇总（表 cost_bill_account_summary）。Phase3 Mock 占位。
type BillAccountSummary struct {
	AccountID   string             `json:"account_id"`
	PeriodType  string             `json:"period_type"`
	PeriodStart time.Time         `json:"period_start"`
	PeriodEnd   time.Time         `json:"period_end"`
	TotalAmount float64          `json:"total_amount"`
	Currency    string            `json:"currency"`
	ByCategory  map[string]float64 `json:"by_category"`
	CreatedAt   time.Time         `json:"created_at"`
}

// DailyStorageCost 存储维度日成本（表 cost_daily_storage）。Phase3 Mock 占位。
type DailyStorageCost struct {
	Day           time.Time `json:"day"`
	Namespace     string    `json:"namespace"`
	StorageClass  string    `json:"storage_class"`
	PVCName       string    `json:"pvc_name"`
	Cost          float64   `json:"cost"`
	CreatedAt     time.Time `json:"created_at"`
}

// DailyNetworkCost 网络维度日成本（表 cost_daily_network）。Phase3 Mock 占位。
type DailyNetworkCost struct {
	Day          time.Time `json:"day"`
	Namespace    string    `json:"namespace"`
	ResourceType string    `json:"resource_type"`
	ResourceID   string    `json:"resource_id"`
	Cost         float64   `json:"cost"`
	CreatedAt    time.Time `json:"created_at"`
}
