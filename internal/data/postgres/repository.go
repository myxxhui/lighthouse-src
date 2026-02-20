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
