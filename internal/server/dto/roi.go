// Package dto defines Data Transfer Objects for HTTP API requests and responses.
package dto

import (
	"time"
)

// =============================================
// ROI Dashboard DTOs
// =============================================

// ROIDashboardRequest represents the request for ROI dashboard.
type ROIDashboardRequest struct {
	TimeRange   string `form:"time_range"`  // Optional time range (e.g., "30d", "90d")
	Granularity string `form:"granularity"` // Optional granularity (daily, weekly, monthly)
	Metric      string `form:"metric"`      // Optional specific metric filter
}

// ROIDashboardResponse represents the response for ROI dashboard.
type ROIDashboardResponse struct {
	Summary         ROISummary          `json:"summary"`
	Trends          []ROITrend          `json:"trends"`
	Breakdown       []ROIBreakdown      `json:"breakdown"`
	Recommendations []ROIRecommendation `json:"recommendations"`
	Timestamp       time.Time           `json:"timestamp"`
}

// ROISummary provides a high-level summary of ROI.
type ROISummary struct {
	ROIPercentage        float64 `json:"roi_percentage"`
	TotalInvestment      float64 `json:"total_investment"`
	TotalSavings         float64 `json:"total_savings"`
	TotalBenefits        float64 `json:"total_benefits"`
	PaybackPeriod        string  `json:"payback_period"` // e.g., "6 months"
	NetPresentValue      float64 `json:"net_present_value"`
	InternalRateOfReturn float64 `json:"internal_rate_of_return"`
	Status               string  `json:"status"` // excellent, good, moderate, poor
	Trend                string  `json:"trend"`  // improving, stable, declining
}

// ROITrend represents ROI trend over time.
type ROITrend struct {
	Period     string  `json:"period"` // e.g., "2024-01"
	ROI        float64 `json:"roi"`
	Investment float64 `json:"investment"`
	Savings    float64 `json:"savings"`
	Benefits   float64 `json:"benefits"`
}

// ROIBreakdown provides breakdown of ROI by category.
type ROIBreakdown struct {
	Category   string  `json:"category"`
	Investment float64 `json:"investment"`
	Savings    float64 `json:"savings"`
	Benefits   float64 `json:"benefits"`
	ROI        float64 `json:"roi"`
	Percentage float64 `json:"percentage"` // contribution to total
}

// ROIRecommendation represents a recommendation for improving ROI.
type ROIRecommendation struct {
	ID           string  `json:"id"`
	Title        string  `json:"title"`
	Description  string  `json:"description"`
	Impact       string  `json:"impact"` // high, medium, low
	Effort       string  `json:"effort"` // high, medium, low
	ROIPotential float64 `json:"roi_potential"`
	Category     string  `json:"category"`
	Status       string  `json:"status"` // pending, in-progress, completed
	Priority     int     `json:"priority"`
}

// =============================================
// ROI Details DTOs
// =============================================

// ROIDetailsRequest represents the request for detailed ROI analysis.
type ROIDetailsRequest struct {
	Category  string `form:"category"` // Optional category filter
	StartTime string `form:"start_time"`
	EndTime   string `form:"end_time"`
}

// ROIDetailsResponse represents the response for detailed ROI analysis.
type ROIDetailsResponse struct {
	Category    string             `json:"category"`
	Metrics     []ROIMetric        `json:"metrics"`
	Initiatives []ROIInitiative    `json:"initiatives"`
	Timeline    []ROITimelineEvent `json:"timeline"`
	Timestamp   time.Time          `json:"timestamp"`
}

// ROIMetric represents a specific ROI metric.
type ROIMetric struct {
	Name     string  `json:"name"`
	Value    float64 `json:"value"`
	Unit     string  `json:"unit"`
	Target   float64 `json:"target"`
	Variance float64 `json:"variance"`
	Status   string  `json:"status"`
}

// ROIInitiative represents an ROI improvement initiative.
type ROIInitiative struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	StartDate   time.Time `json:"start_date"`
	EndDate     time.Time `json:"end_date"`
	Budget      float64   `json:"budget"`
	ActualCost  float64   `json:"actual_cost"`
	Savings     float64   `json:"savings"`
	ROI         float64   `json:"roi"`
	Status      string    `json:"status"`
	Owner       string    `json:"owner"`
}

// ROITimelineEvent represents a timeline event in ROI tracking.
type ROITimelineEvent struct {
	Date        time.Time `json:"date"`
	Event       string    `json:"event"`
	Description string    `json:"description"`
	Impact      string    `json:"impact"`
}

// =============================================
// ROI Comparison DTOs
// =============================================

// ROIComparisonRequest represents the request for ROI comparison.
type ROIComparisonRequest struct {
	BaselinePeriod   string `form:"baseline_period"`   // e.g., "2023-Q4"
	ComparisonPeriod string `form:"comparison_period"` // e.g., "2024-Q1"
	Metric           string `form:"metric"`
}

// ROIComparisonResponse represents the response for ROI comparison.
type ROIComparisonResponse struct {
	Baseline   ROIPeriod `json:"baseline"`
	Comparison ROIPeriod `json:"comparison"`
	Delta      ROIDelta  `json:"delta"`
	Insights   []string  `json:"insights"`
	Timestamp  time.Time `json:"timestamp"`
}

// ROIPeriod represents ROI data for a specific period.
type ROIPeriod struct {
	Period     string  `json:"period"`
	ROI        float64 `json:"roi"`
	Investment float64 `json:"investment"`
	Savings    float64 `json:"savings"`
	Benefits   float64 `json:"benefits"`
}

// ROIDelta represents the change between two periods.
type ROIDelta struct {
	ROIChange        float64 `json:"roi_change"`
	InvestmentChange float64 `json:"investment_change"`
	SavingsChange    float64 `json:"savings_change"`
	BenefitsChange   float64 `json:"benefits_change"`
	PercentageChange float64 `json:"percentage_change"`
	Interpretation   string  `json:"interpretation"`
}
