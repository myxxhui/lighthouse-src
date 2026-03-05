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

	// [Ref: 06_ 成本云账单三表 D2] 日/月原始与聚合表（ETL 五步流水线）
	SaveCloudBillDailyRaw(ctx context.Context, r CloudBillDailyRaw) error
	GetCloudBillDailyRaw(ctx context.Context, billDate time.Time) (*CloudBillDailyRaw, error)
	DeleteCloudBillDailyRawForDate(ctx context.Context, billDate time.Time) error
	ListMissingCloudBillDailyDates(ctx context.Context, from, to time.Time) ([]time.Time, error)
	SaveCloudBillMonthlyRaw(ctx context.Context, r CloudBillMonthlyRaw) error
	GetCloudBillMonthlyRaw(ctx context.Context, billingCycle string) (*CloudBillMonthlyRaw, error)
	SaveCloudBillAggregate(ctx context.Context, a CloudBillAggregate) error
	GetCloudBillAggregate(ctx context.Context, reportType, periodKey string) (*CloudBillAggregate, error)
	// DeleteCloudBillAggregateExcept 删除指定 report_type + metric_type 下 period_key 不在 keepPeriodKeys 中的行
	DeleteCloudBillAggregateExcept(ctx context.Context, reportType string, keepPeriodKeys []string) error
	ListCloudBillDailyRawFromTo(ctx context.Context, from, to time.Time) ([]CloudBillDailyRaw, error)

	// [Ref: 16_云账单动态对账与高可靠处理规范 §三] 行级流水 Upsert（ON CONFLICT record_id DO UPDATE）
	UpsertCloudBillLineItem(ctx context.Context, item CloudBillLineItem) error
	// ListCloudBillLineItemsByDate 返回指定日期（含）的所有流水条目（含负数冲正）
	ListCloudBillLineItemsByDate(ctx context.Context, billDate time.Time, accountID string) ([]CloudBillLineItem, error)
	// ListCloudBillLineItemsByBillingCycle 返回指定账期+账号的所有流水条目（用于按 billing_cycle 汇总月原始表，回退归属到被冲正账期）[Ref: 16_ §四、§七]
	ListCloudBillLineItemsByBillingCycle(ctx context.Context, billingCycle, accountID string) ([]CloudBillLineItem, error)
	// SumLineItemsCashByBillingCycle 计算指定账期所有条目的 CashAmount 代数和（含负数）
	SumLineItemsCashByBillingCycle(ctx context.Context, billingCycle, accountID string) (float64, error)
	// DeleteLineItemsOlderThan 清理早于指定日期的流水条目（配合 daily_raw 10 个月滑动清理）
	DeleteLineItemsOlderThan(ctx context.Context, before time.Time, accountID string) error

	// [Ref: 16_ §三] 月度对账状态 CRUD
	UpsertCloudBillMonthStatus(ctx context.Context, s CloudBillMonthStatus) error
	GetCloudBillMonthStatus(ctx context.Context, billingCycle, accountID string) (*CloudBillMonthStatus, error)

	// [Ref: 01_设计 §环境与云账号配置 D9-3] 环境与产品配置
	ListEnvAccountConfig(ctx context.Context) ([]EnvAccountConfig, error)
	GetProductCategory(ctx context.Context, productCode string) (category string, ok bool)
	// ListCloudBillAggregateForReportPeriod 返回指定 report_type+period_key+metric_type 下所有 account 的聚合行（多账号时多行）
	// metricType 为 "" 时默认 "payment"（仅保留实际付款聚合表）[Ref: 16_ §四]
	ListCloudBillAggregateForReportPeriod(ctx context.Context, reportType, periodKey, metricType string) ([]CloudBillAggregate, error)

	// HealthCheck checks if the database is reachable.
	HealthCheck(ctx context.Context) error

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
	Region            string    `json:"region,omitempty"`
	SyncedAt          time.Time `json:"synced_at"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
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
