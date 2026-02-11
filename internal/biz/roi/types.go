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

// =============================================
// Core ROI and Financial Tracking Types
// =============================================

// CostSavingsBreakdown provides detailed breakdown of cost savings.
// This type enables granular tracking of savings sources.
type CostSavingsBreakdown struct {
	// Time period
	PeriodStart time.Time `json:"period_start"`
	PeriodEnd   time.Time `json:"period_end"`

	// Savings by category
	ZombieCleanupSavings        float64 `json:"zombie_cleanup_savings"`        // From decommissioning zombie assets
	ResourceOptimizationSavings float64 `json:"resource_optimization_savings"` // From right-sizing resources
	NodeConsolidationSavings    float64 `json:"node_consolidation_savings"`    // From reducing node count
	StorageOptimizationSavings  float64 `json:"storage_optimization_savings"`  // From storage optimization
	LicenseOptimizationSavings  float64 `json:"license_optimization_savings"`  // From license optimization

	// Recurring vs one-time savings
	RecurringSavingsMonthly float64 `json:"recurring_savings_monthly"` // Monthly recurring savings
	OneTimeSavings          float64 `json:"one_time_savings"`          // One-time savings (e.g., hardware)

	// Annualized savings
	AnnualizedSavings float64 `json:"annualized_savings"`

	// Savings validation
	ValidatedBy      string    `json:"validated_by,omitempty"`      // Who validated the savings
	ValidationDate   time.Time `json:"validation_date,omitempty"`   // When validation occurred
	ValidationMethod string    `json:"validation_method,omitempty"` // How savings were validated
}

// ResourceRecoveryMetrics tracks the recovery of resources from optimization.
type ResourceRecoveryMetrics struct {
	// Recovered CPU (in cores)
	RecoveredCPU float64 `json:"recovered_cpu"`

	// Recovered Memory (in GB)
	RecoveredMemory float64 `json:"recovered_memory"`

	// Recovered Storage (in GB)
	RecoveredStorage float64 `json:"recovered_storage,omitempty"`

	// Equivalent node count recovered
	EquivalentNodesRecovered float64 `json:"equivalent_nodes_recovered"`

	// Resource utilization improvement
	CPUUtilizationImprovement    float64 `json:"cpu_utilization_improvement"`    // Percentage points
	MemoryUtilizationImprovement float64 `json:"memory_utilization_improvement"` // Percentage points

	// Redeployment potential
	CanHostNewWorkloads  bool    `json:"can_host_new_workloads"` // Whether recovered resources can host new workloads
	NewWorkloadCapacity  float64 `json:"new_workload_capacity"`  // Estimated capacity for new workloads
	ResourceRecoveryRate float64 `json:"resource_recovery_rate"` // Percentage of total resources recovered
}

// ROIValidationEvidence provides evidence for ROI calculations.
// This ensures financial claims are backed by data.
type ROIValidationEvidence struct {
	// Evidence identifier
	EvidenceID string `json:"evidence_id"`

	// ROI claim being validated
	ROIClaimID string `json:"roi_claim_id"`

	// Evidence type
	EvidenceType string `json:"evidence_type"` // "cost_report", "resource_metrics", "invoice", "configuration_snapshot"

	// Evidence source
	SourceSystem string `json:"source_system"` // e.g., "AWS_Cost_Explorer", "GCP_Billing", "Azure_Cost_Management"

	// Evidence data
	Data map[string]interface{} `json:"data"`

	// Confidence level (0.0-1.0)
	ConfidenceLevel float64 `json:"confidence_level"`

	// Validation timestamp
	ValidatedAt time.Time `json:"validated_at"`

	// Validator information
	ValidatorName string `json:"validator_name,omitempty"`
	ValidatorRole string `json:"validator_role,omitempty"`
	ValidatorTeam string `json:"validator_team,omitempty"`
}

// FinancialImpactAnalysis provides comprehensive financial impact analysis.
type FinancialImpactAnalysis struct {
	// Analysis period
	AnalysisPeriodStart time.Time `json:"analysis_period_start"`
	AnalysisPeriodEnd   time.Time `json:"analysis_period_end"`

	// Cost avoidance (prevented future costs)
	CostAvoidance float64 `json:"cost_avoidance"`

	// Cost reduction (actual reduction in current costs)
	CostReduction float64 `json:"cost_reduction"`

	// Efficiency gains (improved resource utilization)
	EfficiencyGainsValue float64 `json:"efficiency_gains_value"` // Monetary value of efficiency gains

	// Total financial impact
	TotalFinancialImpact float64 `json:"total_financial_impact"`

	// Return on Investment (ROI) metrics
	ROIPercentage        float64 `json:"roi_percentage"`          // ROI percentage
	PaybackPeriodMonths  float64 `json:"payback_period_months"`   // Months to recoup investment
	NetPresentValue      float64 `json:"net_present_value"`       // NPV of savings
	InternalRateOfReturn float64 `json:"internal_rate_of_return"` // IRR percentage

	// Investment costs
	ImplementationCosts float64 `json:"implementation_costs"` // Cost to implement Lighthouse
	OngoingCosts        float64 `json:"ongoing_costs"`        // Ongoing operational costs
	TotalInvestment     float64 `json:"total_investment"`     // Total investment

	// Risk assessment
	RiskAdjustedROI      float64  `json:"risk_adjusted_roi"`               // ROI adjusted for risk
	RiskFactors          []string `json:"risk_factors,omitempty"`          // Identified risk factors
	MitigationStrategies []string `json:"mitigation_strategies,omitempty"` // Risk mitigation strategies
}

// OptimizationTrackingRecord tracks individual optimization actions.
type OptimizationTrackingRecord struct {
	// Record identifier
	RecordID string `json:"record_id"`

	// Optimization type
	OptimizationType string `json:"optimization_type"` // "zombie_cleanup", "resource_rightsizing", "node_consolidation", "storage_optimization"

	// Target resource
	TargetResourceID   string `json:"target_resource_id"`
	TargetResourceType string `json:"target_resource_type"` // "pod", "namespace", "node", "storage_class"

	// Before and after state
	BeforeState map[string]interface{} `json:"before_state"`
	AfterState  map[string]interface{} `json:"after_state"`

	// Savings achieved
	ImmediateSavings   float64            `json:"immediate_savings"`   // Immediate cost savings
	ProjectedSavings   float64            `json:"projected_savings"`   // Projected annual savings
	ResourcesRecovered map[string]float64 `json:"resources_recovered"` // Resources recovered (CPU, Memory, etc.)

	// Implementation details
	ImplementationDate   time.Time `json:"implementation_date"`
	ImplementedBy        string    `json:"implemented_by,omitempty"`
	ImplementationEffort string    `json:"implementation_effort,omitempty"` // "low", "medium", "high"

	// Verification
	Verified         bool      `json:"verified"` // Whether savings were verified
	VerificationDate time.Time `json:"verification_date,omitempty"`
	VerifiedBy       string    `json:"verified_by,omitempty"`

	// Impact assessment
	RiskLevel         string `json:"risk_level,omitempty"`         // "low", "medium", "high"
	BusinessImpact    string `json:"business_impact,omitempty"`    // "positive", "neutral", "negative"
	PerformanceImpact string `json:"performance_impact,omitempty"` // "improved", "neutral", "degraded"
}
