// Package costmodel defines the core types for Lighthouse's dual-cost model.
// These types are shared across different modules and should be kept stable.
package costmodel

import (
	"encoding/json"
	"time"
)

// ResourceMetric represents the resource usage and request metrics for a container/pod.
// This is the fundamental data structure for cost calculation.
type ResourceMetric struct {
	// CPU metrics
	CPURequest  float64 `json:"cpu_request"`
	CPUUsageP95 float64 `json:"cpu_usage_p95"`

	// Memory metrics (in bytes)
	MemRequest  float64 `json:"mem_request"`
	MemUsageP95 float64 `json:"mem_usage_p95"`

	// Timestamp of the metric
	Timestamp time.Time `json:"timestamp"`
}

// CostResult represents the result of dual-cost calculation for a resource.
// This includes billable cost, usage value, waste, and efficiency score.
type CostResult struct {
	// BillableCost is the cost based on K8s Request configuration.
	// Formula: Σ(Request_Core × Price_Node_Core) + Σ(Request_Mem × Price_Mem)
	BillableCost float64 `json:"billable_cost"`

	// UsageCost is the cost based on actual usage (P95).
	// Formula: Σ(Usage_P95_Core × Price_Node_Core) + Σ(Usage_P95_Mem × Price_Mem)
	UsageCost float64 `json:"usage_cost"`

	// WasteCost is the difference between billable and usage cost.
	// Formula: Cost_Billable - Cost_Usage
	WasteCost float64 `json:"waste_cost"`

	// EfficiencyScore is the resource utilization efficiency percentage.
	// Formula: (Usage_P95 / Request) × 100%
	// Range: 0.0 - 100.0
	EfficiencyScore float64 `json:"efficiency_score"`

	// Grade represents the efficiency grade based on the score.
	// Valid values: Zombie, OverProvisioned, Healthy, Risk
	Grade EfficiencyGrade `json:"grade"`

	// Resource type (CPU or Memory)
	ResourceType string `json:"resource_type"`
}

// EfficiencyGrade represents the efficiency classification based on efficiency score.
type EfficiencyGrade string

const (
	// Zombie represents extremely wasteful resources (< 10% efficiency).
	// Recommendation: Consider shutting down.
	Zombie EfficiencyGrade = "Zombie"

	// OverProvisioned represents over-provisioned resources (10% - 40% efficiency).
	// Recommendation: Consider downgrading configuration.
	OverProvisioned EfficiencyGrade = "OverProvisioned"

	// Healthy represents healthy resources (40% - 70% efficiency).
	// This is the optimal buffer range.
	Healthy EfficiencyGrade = "Healthy"

	// Risk represents risky resources (> 90% efficiency).
	// Warning: Risk of OOM/Throttling due to insufficient resources.
	Risk EfficiencyGrade = "Risk"
)

// ZombieMetrics represents the metrics used for zombie asset detection.
// A service is considered zombie if it meets all the criteria below.
type ZombieMetrics struct {
	// CPU usage average over 7 days (in cores)
	CPUUsageAvg7d float64 `json:"cpu_usage_avg_7d"`

	// CPU usage standard deviation over 7 days
	CPUUsageStdDev7d float64 `json:"cpu_usage_stddev_7d"`

	// Memory usage average over 7 days (in bytes)
	MemUsageAvg7d float64 `json:"mem_usage_avg_7d"`

	// Network I/O average over 7 days (in bytes per second)
	NetworkIOAvg7d float64 `json:"network_io_avg_7d"`

	// Timestamp range for the metrics
	StartTime time.Time `json:"start_time"`
	EndTime   time.Time `json:"end_time"`
}

// IsZombie determines if the metrics indicate a zombie asset.
// Criteria:
// - CPU: avg(usage_7d) < 0.1 Core AND stddev(usage_7d) ≈ 0 (dead line)
// - Memory: avg(mem_usage_7d) < 0.1 GiB AND no fluctuation
// - Network: avg(network_io_7d) < 1 KB/s
func (zm *ZombieMetrics) IsZombie() bool {
	const (
		cpuThreshold     = 0.1       // 0.1 Core
		memThreshold     = 0.1 * 1e9 // 0.1 GiB in bytes
		networkThreshold = 1024.0    // 1 KB/s in bytes
		stdDevThreshold  = 0.01      // Very low fluctuation
	)

	return zm.CPUUsageAvg7d < cpuThreshold &&
		zm.CPUUsageStdDev7d < stdDevThreshold &&
		zm.MemUsageAvg7d < memThreshold &&
		zm.NetworkIOAvg7d < networkThreshold
}

// AggregationLevel represents the four-level drill-down hierarchy.
type AggregationLevel string

const (
	LevelNamespace AggregationLevel = "namespace" // L1: Namespace (Domain) view
	LevelNode      AggregationLevel = "node"      // L2: Node view
	LevelWorkload  AggregationLevel = "workload"  // L3: Workload (Deployment) view
	LevelPod       AggregationLevel = "pod"       // L4: Pod (Instance) view
)

// AggregationResult represents the result of aggregation at a specific level.
type AggregationResult struct {
	Level AggregationLevel `json:"level"`

	// Total costs
	TotalBillableCost float64 `json:"total_billable_cost"`
	TotalUsageCost    float64 `json:"total_usage_cost"`
	TotalWasteCost    float64 `json:"total_waste_cost"`

	// Average efficiency score
	AverageEfficiencyScore float64 `json:"average_efficiency_score"`

	// Count of resources
	ResourceCount int `json:"resource_count"`

	// Identifier for the aggregation (e.g., namespace name, node name, etc.)
	Identifier string `json:"identifier"`

	// Timestamp of the aggregation
	Timestamp time.Time `json:"timestamp"`
}

// MarshalJSON implements custom JSON marshaling for CostResult.
func (cr *CostResult) MarshalJSON() ([]byte, error) {
	type Alias CostResult
	return json.Marshal(&struct {
		*Alias
	}{
		Alias: (*Alias)(cr),
	})
}

// UnmarshalJSON implements custom JSON unmarshaling for CostResult.
func (cr *CostResult) UnmarshalJSON(data []byte) error {
	type Alias CostResult
	aux := &struct {
		*Alias
	}{
		Alias: (*Alias)(cr),
	}
	return json.Unmarshal(data, &aux)
}
