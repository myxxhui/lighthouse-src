// Package dto defines Data Transfer Objects for HTTP API requests and responses.
package dto

import (
	"time"
)

// =============================================
// SLO Health DTOs
// =============================================

// SLOHealthRequest represents the request for SLO health status.
type SLOHealthRequest struct {
	Namespace string `form:"namespace"`  // Optional namespace filter
	Service   string `form:"service"`    // Optional service filter
	TimeRange string `form:"time_range"` // Optional time range (e.g., "24h", "7d")
}

// SLOHealthResponse represents the response for SLO health status.
type SLOHealthResponse struct {
	Status     string         `json:"status"` // healthy, degraded, critical
	Metrics    []SLOMetric    `json:"metrics"`
	Violations []SLOViolation `json:"violations"`
	Summary    SLOSummary     `json:"summary"`
	Timestamp  time.Time      `json:"timestamp"`
}

// SLOMetric represents a single SLO metric.
type SLOMetric struct {
	Name        string    `json:"name"`
	Value       float64   `json:"value"`
	Threshold   float64   `json:"threshold"`
	Unit        string    `json:"unit"`
	Status      string    `json:"status"` // met, warning, breached
	Trend       string    `json:"trend"`  // improving, stable, degrading
	LastUpdated time.Time `json:"last_updated"`
}

// SLOViolation represents an SLO violation event.
type SLOViolation struct {
	ID          string    `json:"id"`
	Service     string    `json:"service"`
	Metric      string    `json:"metric"`
	ViolatedAt  time.Time `json:"violated_at"`
	Duration    string    `json:"duration"` // e.g., "2h30m"
	Severity    string    `json:"severity"` // low, medium, high
	Description string    `json:"description"`
	Evidence    []string  `json:"evidence"`
}

// SLOSummary provides a summary of SLO health.
type SLOSummary struct {
	TotalServices    int     `json:"total_services"`
	HealthyServices  int     `json:"healthy_services"`
	AtRiskServices   int     `json:"at_risk_services"`
	BreachedServices int     `json:"breached_services"`
	OverallHealth    float64 `json:"overall_health"` // percentage
	MTTR             string  `json:"mttr"`           // mean time to resolution
	Availability     float64 `json:"availability"`
	LatencyP95       int     `json:"latency_p95"`
	ErrorRate        float64 `json:"error_rate"`
}

// =============================================
// SLO History DTOs
// =============================================

// SLOHistoryRequest represents the request for SLO history.
type SLOHistoryRequest struct {
	Service     string `form:"service" binding:"required"`
	Metric      string `form:"metric"`
	StartTime   string `form:"start_time"`
	EndTime     string `form:"end_time"`
	Granularity string `form:"granularity"` // hour, day, week
}

// SLOHistoryResponse represents the response for SLO history.
type SLOHistoryResponse struct {
	Service    string                `json:"service"`
	Metric     string                `json:"metric"`
	DataPoints []SLOHistoryDataPoint `json:"data_points"`
	Trend      string                `json:"trend"`
	Timestamp  time.Time             `json:"timestamp"`
}

// SLOHistoryDataPoint represents a historical data point.
type SLOHistoryDataPoint struct {
	Timestamp time.Time `json:"timestamp"`
	Value     float64   `json:"value"`
	Threshold float64   `json:"threshold"`
	Status    string    `json:"status"`
	Violation bool      `json:"violation"`
}

// =============================================
// SLO Burn Rate DTOs
// =============================================

// SLOBurnRateRequest represents the request for SLO burn rate.
type SLOBurnRateRequest struct {
	Service string `form:"service" binding:"required"`
	Window  string `form:"window"` // e.g., "1h", "6h", "24h"
}

// SLOBurnRateResponse represents the response for SLO burn rate.
type SLOBurnRateResponse struct {
	Service             string    `json:"service"`
	Window              string    `json:"window"`
	BurnRate            float64   `json:"burn_rate"`
	RemainingBudget     float64   `json:"remaining_budget"`
	BudgetConsumed      float64   `json:"budget_consumed"`
	Status              string    `json:"status"`
	ProjectedExhaustion time.Time `json:"projected_exhaustion"`
	Timestamp           time.Time `json:"timestamp"`
}
