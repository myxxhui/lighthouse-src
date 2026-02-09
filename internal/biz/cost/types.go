// Package cost defines the business domain types for cost calculation and resource analysis.
// These types are specific to the cost module and may evolve independently.
package cost

import (
	"time"

	"github.com/myxxhui/lighthouse-src/pkg/costmodel"
)

// CostCalculationInput represents the input required for cost calculation.
// This includes resource metrics and pricing information.
type CostCalculationInput struct {
	// Resource metrics (CPU and Memory)
	Metrics []costmodel.ResourceMetric `json:"metrics"`

	// Pricing information
	NodeCorePrice float64 `json:"node_core_price"`
	NodeMemPrice  float64 `json:"node_mem_price"`

	// Time range for the calculation
	StartTime time.Time `json:"start_time"`
	EndTime   time.Time `json:"end_time"`
}

// CostCalculationResult represents the complete result of cost calculation.
// This includes detailed breakdowns for different resources and aggregation levels.
type CostCalculationResult struct {
	// Individual resource results
	ResourceResults []costmodel.CostResult `json:"resource_results"`

	// Aggregated results by level
	AggregatedResults map[costmodel.AggregationLevel][]costmodel.AggregationResult `json:"aggregated_results"`

	// Summary statistics
	Summary CostSummary `json:"summary"`

	// Calculation timestamp
	CalculatedAt time.Time `json:"calculated_at"`
}

// CostSummary provides a high-level summary of the cost calculation.
type CostSummary struct {
	// Total costs across all resources
	TotalBillableCost float64 `json:"total_billable_cost"`
	TotalUsageCost    float64 `json:"total_usage_cost"`
	TotalWasteCost    float64 `json:"total_waste_cost"`

	// Overall efficiency score
	OverallEfficiencyScore float64 `json:"overall_efficiency_score"`

	// Resource counts by grade
	ZombieCount          int `json:"zombie_count"`
	OverProvisionedCount int `json:"over_provisioned_count"`
	HealthyCount         int `json:"healthy_count"`
	RiskCount            int `json:"risk_count"`

	// Potential savings
	PotentialSavings float64 `json:"potential_savings"`
}

// NamespaceCost represents the cost breakdown for a specific namespace.
// This is used for L1 (Namespace) level aggregation.
type NamespaceCost struct {
	Namespace string `json:"namespace"`

	// Cost breakdown
	BillableCost float64 `json:"billable_cost"`
	UsageCost    float64 `json:"usage_cost"`
	WasteCost    float64 `json:"waste_cost"`

	// Efficiency metrics
	EfficiencyScore float64                   `json:"efficiency_score"`
	Grade           costmodel.EfficiencyGrade `json:"grade"`

	// Resource counts
	PodCount      int `json:"pod_count"`
	NodeCount     int `json:"node_count"`
	WorkloadCount int `json:"workload_count"`

	// Timestamp
	Timestamp time.Time `json:"timestamp"`
}

// NodeCost represents the cost breakdown for a specific node.
// This is used for L2 (Node) level aggregation.
type NodeCost struct {
	NodeName string `json:"node_name"`

	// Cost breakdown
	TotalBillableCost float64 `json:"total_billable_cost"`
	AllocatedCost     float64 `json:"allocated_cost"`
	UnallocatedWaste  float64 `json:"unallocated_waste"`

	// Resource utilization
	CPUUtilization float64 `json:"cpu_utilization"`
	MemUtilization float64 `json:"mem_utilization"`

	// Pod count on this node
	PodCount int `json:"pod_count"`

	// Timestamp
	Timestamp time.Time `json:"timestamp"`
}

// WorkloadCost represents the cost breakdown for a specific workload (Deployment/StatefulSet).
// This is used for L3 (Workload) level aggregation.
type WorkloadCost struct {
	WorkloadName string `json:"workload_name"`
	Namespace    string `json:"namespace"`
	WorkloadType string `json:"workload_type"` // Deployment, StatefulSet, etc.

	// Cost breakdown
	TotalBillableCost float64 `json:"total_billable_cost"`
	TotalUsageCost    float64 `json:"total_usage_cost"`
	TotalWasteCost    float64 `json:"total_waste_cost"`

	// Replica count
	ReplicaCount int `json:"replica_count"`

	// Average waste per replica
	AverageWastePerReplica float64 `json:"average_waste_per_replica"`

	// Efficiency metrics
	EfficiencyScore float64                   `json:"efficiency_score"`
	Grade           costmodel.EfficiencyGrade `json:"grade"`

	// Timestamp
	Timestamp time.Time `json:"timestamp"`
}

// PodCostDetail represents detailed cost information for a specific pod.
// This is used for L4 (Pod) level detail view.
type PodCostDetail struct {
	PodName      string `json:"pod_name"`
	Namespace    string `json:"namespace"`
	WorkloadName string `json:"workload_name"`
	NodeName     string `json:"node_name"`

	// CPU cost details
	CPUBillableCost float64 `json:"cpu_billable_cost"`
	CPUUsageCost    float64 `json:"cpu_usage_cost"`
	CPUWasteCost    float64 `json:"cpu_waste_cost"`

	// Memory cost details
	MemBillableCost float64 `json:"mem_billable_cost"`
	MemUsageCost    float64 `json:"mem_usage_cost"`
	MemWasteCost    float64 `json:"mem_waste_cost"`

	// Efficiency scores
	CPUEfficiencyScore float64                   `json:"cpu_efficiency_score"`
	MemEfficiencyScore float64                   `json:"mem_efficiency_score"`
	OverallGrade       costmodel.EfficiencyGrade `json:"overall_grade"`

	// Resource requests and usage
	CPURequest  float64 `json:"cpu_request"`
	CPUUsageP95 float64 `json:"cpu_usage_p95"`
	MemRequest  float64 `json:"mem_request"`
	MemUsageP95 float64 `json:"mem_usage_p95"`

	// Timestamp
	Timestamp time.Time `json:"timestamp"`
}

// ZombieAsset represents a detected zombie asset with detailed information.
type ZombieAsset struct {
	// Resource identification
	PodName      string `json:"pod_name"`
	Namespace    string `json:"namespace"`
	WorkloadName string `json:"workload_name"`

	// Zombie metrics
	ZombieMetrics costmodel.ZombieMetrics `json:"zombie_metrics"`

	// Cost impact
	WasteBillableCost float64 `json:"waste_billable_cost"`

	// Resource release potential
	ReleasableCPU float64 `json:"releasable_cpu"`
	ReleasableMem float64 `json:"releasable_mem"`

	// Equivalent node count (e.g., can release 3 x 8C16G machines)
	EquivalentNodeCount float64 `json:"equivalent_node_count"`

	// Detection timestamp
	DetectedAt time.Time `json:"detected_at"`
}

// CostSimulationInput represents input for cost optimization simulation.
type CostSimulationInput struct {
	Namespace        string  `json:"namespace"`
	TargetEfficiency float64 `json:"target_efficiency"` // e.g., 50.0 for 50%
}

// CostSimulationResult represents the result of cost optimization simulation.
type CostSimulationResult struct {
	Namespace         string  `json:"namespace"`
	CurrentEfficiency float64 `json:"current_efficiency"`
	TargetEfficiency  float64 `json:"target_efficiency"`

	// Current costs
	CurrentBillableCost float64 `json:"current_billable_cost"`
	CurrentUsageCost    float64 `json:"current_usage_cost"`
	CurrentWasteCost    float64 `json:"current_waste_cost"`

	// Projected costs after optimization
	ProjectedBillableCost float64 `json:"projected_billable_cost"`
	ProjectedUsageCost    float64 `json:"projected_usage_cost"`
	ProjectedWasteCost    float64 `json:"projected_waste_cost"`

	// Savings
	AnnualSavings float64 `json:"annual_savings"`

	// Simulation timestamp
	SimulatedAt time.Time `json:"simulated_at"`
}
