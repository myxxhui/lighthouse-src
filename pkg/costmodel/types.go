// Package costmodel provides the core types and algorithms for calculating
// dual costs (billable vs usage) in Kubernetes resource analysis.
package costmodel

import "time"

// ResourceMetric represents the resource usage metrics for a Kubernetes resource.
// This includes both requested resources (configuration) and actual usage (P95).
type ResourceMetric struct {
	// CPU requested in cores (e.g., 2.5 cores)
	CPURequest float64 `json:"cpu_request"`

	// CPU usage at P95 percentile in cores
	CPUUsageP95 float64 `json:"cpu_usage_p95"`

	// Memory requested in bytes
	MemRequest int64 `json:"mem_request"`

	// Memory usage at P95 percentile in bytes
	MemUsageP95 int64 `json:"mem_usage_p95"`

	// Timestamp of the measurement
	Timestamp time.Time `json:"timestamp"`
}

// CostResult represents the calculated cost results for a single resource.
type CostResult struct {
	// CPU costs
	CPUBillableCost    float64 `json:"cpu_billable_cost"`
	CPUUsageCost       float64 `json:"cpu_usage_cost"`
	CPUWasteCost       float64 `json:"cpu_waste_cost"`
	CPUEfficiencyScore float64 `json:"cpu_efficiency_score"`

	// Memory costs
	MemBillableCost    float64 `json:"mem_billable_cost"`
	MemUsageCost       float64 `json:"mem_usage_cost"`
	MemWasteCost       float64 `json:"mem_waste_cost"`
	MemEfficiencyScore float64 `json:"mem_efficiency_score"`

	// Total costs
	TotalBillableCost      float64 `json:"total_billable_cost"`
	TotalUsageCost         float64 `json:"total_usage_cost"`
	TotalWasteCost         float64 `json:"total_waste_cost"`
	OverallEfficiencyScore float64 `json:"overall_efficiency_score"`

	// Efficiency grade
	OverallGrade EfficiencyGrade `json:"overall_grade"`
}

// DualCostResult is an alias for CostResult for backward compatibility.
type DualCostResult = CostResult

// EfficiencyGrade represents the efficiency rating of a resource.
type EfficiencyGrade string

const (
	// GradeZombie indicates extremely wasteful resources (<10% utilization)
	GradeZombie EfficiencyGrade = "Zombie"

	// GradeOverProvisioned indicates over-provisioned resources (10%-40% utilization)
	GradeOverProvisioned EfficiencyGrade = "OverProvisioned"

	// GradeHealthy indicates healthy resources with reasonable buffer (40%-70% utilization)
	GradeHealthy EfficiencyGrade = "Healthy"

	// GradeRisk indicates under-provisioned resources at risk (>90% utilization)
	GradeRisk EfficiencyGrade = "Risk"

	// GradeUnknown indicates unknown or unclassified efficiency
	GradeUnknown EfficiencyGrade = "Unknown"
)

// AggregationLevel represents the level at which costs are aggregated.
type AggregationLevel int

const (
	// LevelPod represents pod-level aggregation
	LevelPod AggregationLevel = iota + 1

	// LevelWorkload represents workload-level aggregation
	LevelWorkload

	// LevelNamespace represents namespace-level aggregation
	LevelNamespace

	// LevelNode represents node-level aggregation
	LevelNode

	// LevelCluster represents cluster-level aggregation
	LevelCluster
)

// AggregationResult represents the result of aggregating costs at a specific level.
type AggregationResult struct {
	Level         AggregationLevel `json:"level"`
	Identifier    string           `json:"identifier"`
	TotalCost     CostResult       `json:"total_cost"`
	ResourceCount int              `json:"resource_count"`
	Timestamp     time.Time        `json:"timestamp"`
}

// Aggregator interface defines the contract for cost aggregators.
type Aggregator interface {
	Aggregate(results []DualCostResult) (*AggregationResult, error)
}

// DailyNamespaceCost represents the daily aggregated cost data for a namespace.
// This is the source data for L0 (global view) aggregation from daily_namespace_costs table.
type DailyNamespaceCost struct {
	Namespace     string    `json:"namespace"`
	Date          time.Time `json:"date"`
	BillableCost  float64   `json:"billable_cost"`
	UsageCost     float64   `json:"usage_cost"`
	WasteCost     float64   `json:"waste_cost"`
	PodCount      int       `json:"pod_count"`
	NodeCount     int       `json:"node_count"`
	WorkloadCount int       `json:"workload_count"`
}

// HourlyWorkloadStat represents hourly statistics for a workload.
// This is the source data for L1-L4 aggregation from hourly_workload_stats table.
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
	MemWasteCost      float64   `json:"mem_waste_cost"`
	TotalBillableCost float64   `json:"total_billable_cost"`
	TotalUsageCost    float64   `json:"total_usage_cost"`
	TotalWasteCost    float64   `json:"total_waste_cost"`
}

// GlobalAggregatedResult represents the result of L0 global aggregation.
type GlobalAggregatedResult struct {
	TotalBillableCost float64   `json:"total_billable_cost"`
	TotalWaste        float64   `json:"total_waste"`
	GlobalEfficiency  float64   `json:"global_efficiency"`
	Timestamp         time.Time `json:"timestamp"`
}

// DomainBreakdownItem represents a single namespace/domain in the domain breakdown pie chart.
type DomainBreakdownItem struct {
	DomainName     string  `json:"domain_name"`
	CostPercentage float64 `json:"cost_percentage"`
	BillableCost   float64 `json:"billable_cost"`
	UsageCost      float64 `json:"usage_cost"`
	WasteCost      float64 `json:"waste_cost"`
	PodCount       int     `json:"pod_count"`
}

// AggregatedResult is a generic result type for L1-L4 aggregation.
type AggregatedResult struct {
	Identifier        string    `json:"identifier"`
	TotalBillableCost float64   `json:"total_billable_cost"`
	TotalUsageCost    float64   `json:"total_usage_cost"`
	TotalWasteCost    float64   `json:"total_waste_cost"`
	EfficiencyScore   float64   `json:"efficiency_score"`
	ResourceCount     int       `json:"resource_count"`
	Timestamp         time.Time `json:"timestamp"`
}

// PrecisionConfig holds configuration for decimal precision in financial calculations.
type PrecisionConfig struct {
	DecimalPlaces int     `json:"decimal_places"`
	RoundingMode  string  `json:"rounding_mode"`
	Epsilon       float64 `json:"epsilon"` // for floating point comparisons
}

// DefaultPrecisionConfig returns the default precision configuration.
func DefaultPrecisionConfig() PrecisionConfig {
	return PrecisionConfig{
		DecimalPlaces: 2, // financial precision to cents
		RoundingMode:  "half_up",
		Epsilon:       1e-9,
	}
}

// ZombieMetrics represents metrics for detecting zombie resources.
// Includes 7-day usage statistics for CPU, memory, and network.
type ZombieMetrics struct {
	CPUUtilization float64   `json:"cpu_utilization"`
	MemUtilization float64   `json:"mem_utilization"`
	InactiveDays   int       `json:"inactive_days"`
	LastAccessTime time.Time `json:"last_access_time"`
	// 7-day statistics
	CPUAvg        float64 `json:"cpu_avg"`         // CPU 7-day average usage (cores)
	CPUStdDev     float64 `json:"cpu_std_dev"`     // CPU 7-day standard deviation
	MemAvg        float64 `json:"mem_avg"`         // Memory 7-day average usage (GiB)
	MemStdDev     float64 `json:"mem_std_dev"`     // Memory 7-day standard deviation
	NetworkAvg    float64 `json:"network_avg"`     // Network 7-day average IO (KB/s)
	NetworkStdDev float64 `json:"network_std_dev"` // Network 7-day standard deviation
}
