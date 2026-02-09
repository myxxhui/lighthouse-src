// Package roi defines the business domain types for ROI (Return on Investment) tracking and value measurement.
// These types support baseline comparison, financial savings calculation, and efficiency gains tracking.
package roi

import (
	"time"
)

// BaselineSnapshot represents the Day 0 baseline snapshot for ROI tracking.
type BaselineSnapshot struct {
	// Snapshot metadata
	SnapshotID string `json:"snapshot_id"`

	// Resource utilization metrics
	CPUUtilization float64 `json:"cpu_utilization"`
	MemUtilization float64 `json:"mem_utilization"`

	// Cost metrics
	TotalWasteAmount  float64 `json:"total_waste_amount"`
	TotalBillableCost float64 `json:"total_billable_cost"`

	// Node metrics
	NodeCount int `json:"node_count"`

	// Zombie asset count
	ZombieAssetCount int `json:"zombie_asset_count"`

	// Snapshot timestamp
	Timestamp time.Time `json:"timestamp"`
}

// DailyComparison represents the daily comparison against the baseline.
type DailyComparison struct {
	// Comparison metadata
	Date time.Time `json:"date"`

	// Baseline reference
	BaselineID string `json:"baseline_id"`

	// Current metrics
	CurrentCPUUtilization    float64 `json:"current_cpu_utilization"`
	CurrentMemUtilization    float64 `json:"current_mem_utilization"`
	CurrentTotalWasteAmount  float64 `json:"current_total_waste_amount"`
	CurrentTotalBillableCost float64 `json:"current_total_billable_cost"`
	CurrentNodeCount         int     `json:"current_node_count"`
	CurrentZombieAssetCount  int     `json:"current_zombie_asset_count"`

	// Comparison results
	CPUUtilizationImprovement float64 `json:"cpu_utilization_improvement"` // percentage points
	MemUtilizationImprovement float64 `json:"mem_utilization_improvement"` // percentage points
	WasteReductionAmount      float64 `json:"waste_reduction_amount"`
	CostSavingsAmount         float64 `json:"cost_savings_amount"`
	NodeReductionCount        int     `json:"node_reduction_count"`
	ZombieCleanupCount        int     `json:"zombie_cleanup_count"`

	// Efficiency gains
	ResourceRecoveryRate float64 `json:"resource_recovery_rate"` // percentage improvement
}

// FinancialSavings represents the financial savings from optimization activities.
type FinancialSavings struct {
	// Savings breakdown
	ZombieCleanupSavings float64 `json:"zombie_cleanup_savings"`
	OptimizationSavings  float64 `json:"optimization_savings"`
	NodeReductionSavings float64 `json:"node_reduction_savings"`

	// Total savings
	TotalSavings float64 `json:"total_savings"`

	// Time period
	StartDate time.Time `json:"start_date"`
	EndDate   time.Time `json:"end_date"`

	// Currency
	Currency string `json:"currency"` // e.g., "CNY", "USD"
}

// EfficiencyGains represents the efficiency gains from Lighthouse implementation.
type EfficiencyGains struct {
	// Resource utilization improvements
	CPUUtilizationGain float64 `json:"cpu_utilization_gain"` // e.g., 15% -> 25% = 10 percentage points
	MemUtilizationGain float64 `json:"mem_utilization_gain"`

	// Resource recovery rate
	ResourceRecoveryRate float64 `json:"resource_recovery_rate"` // percentage

	// Node reduction metrics
	NodeReductionCount      int     `json:"node_reduction_count"`
	NodeReductionPercentage float64 `json:"node_reduction_percentage"`

	// Efficiency score improvements
	AverageEfficiencyScoreBefore float64 `json:"average_efficiency_score_before"`
	AverageEfficiencyScoreAfter  float64 `json:"average_efficiency_score_after"`
	EfficiencyScoreImprovement   float64 `json:"efficiency_score_improvement"`

	// Time period
	PeriodStart time.Time `json:"period_start"`
	PeriodEnd   time.Time `json:"period_end"`
}

// ROIDashboardData represents the data for the ROI dashboard.
type ROIDashboardData struct {
	// Baseline snapshot
	Baseline BaselineSnapshot `json:"baseline"`

	// Daily comparisons (last 30 days)
	DailyComparisons []DailyComparison `json:"daily_comparisons"`

	// Financial savings summary
	FinancialSavings FinancialSavings `json:"financial_savings"`

	// Efficiency gains summary
	EfficiencyGains EfficiencyGains `json:"efficiency_gains"`

	// Key performance indicators
	KPIs map[string]float64 `json:"kpis"`

	// Last updated timestamp
	LastUpdated time.Time `json:"last_updated"`
}

// OptimizationActivity represents a specific optimization activity tracked by ROI.
type OptimizationActivity struct {
	// Activity ID
	ActivityID string `json:"activity_id"`

	// Activity type
	ActivityType string `json:"activity_type"` // zombie_cleanup, resource_optimization, node_reduction

	// Target resources
	TargetResources []string `json:"target_resources"`

	// Savings achieved
	SavingsAmount float64 `json:"savings_amount"`

	// Resources released
	ResourcesReleased map[string]float64 `json:"resources_released"` // e.g., {"cpu": 8.0, "memory": 16.0}

	// Equivalent nodes
	EquivalentNodes float64 `json:"equivalent_nodes"`

	// Activity timestamp
	CompletedAt time.Time `json:"completed_at"`
}

// ROITrend represents ROI trends over time.
type ROITrend struct {
	// Time series data
	TimeSeries []ROITimePoint `json:"time_series"`

	// Trend analysis
	TrendAnalysis ROITrendAnalysis `json:"trend_analysis"`
}

// ROITimePoint represents a single point in the ROI time series.
type ROITimePoint struct {
	// Timestamp
	Timestamp time.Time `json:"timestamp"`

	// Cumulative savings
	CumulativeSavings float64 `json:"cumulative_savings"`

	// Cumulative efficiency gains
	CumulativeEfficiencyGains float64 `json:"cumulative_efficiency_gains"`

	// Current metrics
	CurrentMetrics map[string]float64 `json:"current_metrics"`
}

// ROITrendAnalysis provides analysis of ROI trends.
type ROITrendAnalysis struct {
	// Growth rate
	SavingsGrowthRate    float64 `json:"savings_growth_rate"`
	EfficiencyGrowthRate float64 `json:"efficiency_growth_rate"`

	// Projection
	ProjectedAnnualSavings         float64 `json:"projected_annual_savings"`
	ProjectedAnnualEfficiencyGains float64 `json:"projected_annual_efficiency_gains"`

	// Milestones
	Milestones []ROIMilestone `json:"milestones"`
}

// ROIMilestone represents a significant milestone in ROI tracking.
type ROIMilestone struct {
	// Milestone name
	Name string `json:"name"`

	// Description
	Description string `json:"description"`

	// Achievement date
	AchievedAt time.Time `json:"achieved_at"`

	// Metrics at milestone
	Metrics map[string]float64 `json:"metrics"`
}
