// Package dto defines Data Transfer Objects for HTTP API requests and responses.
package dto

import (
	"time"

	"github.com/myxxhui/lighthouse-src/pkg/costmodel"
)

// =============================================
// Global Cost DTOs
// =============================================

// GlobalCostResponse represents the response for global cost overview.
type GlobalCostResponse struct {
	TotalCost  float64                `json:"total_cost"`
	Namespaces []NamespaceCostSummary `json:"namespaces"`
	Timestamp  time.Time              `json:"timestamp"`
}

// NamespaceCostSummary represents a summary of cost for a namespace.
type NamespaceCostSummary struct {
	Name      string  `json:"name"`
	Cost      float64 `json:"cost"`
	Grade     string  `json:"grade"`
	PodCount  int     `json:"pod_count"`
	NodeCount int     `json:"node_count"`
}

// =============================================
// Namespace Cost DTOs
// =============================================

// NamespaceCostRequest represents the request for namespace cost details.
type NamespaceCostRequest struct {
	Namespace string `uri:"namespace" binding:"required"`
	StartTime string `form:"start_time"` // Optional timestamp in RFC3339
	EndTime   string `form:"end_time"`   // Optional timestamp in RFC3339
}

// NamespaceCostResponse represents the response for namespace cost details.
type NamespaceCostResponse struct {
	Namespace string            `json:"namespace"`
	Cost      CostBreakdown     `json:"cost"`
	Workloads []WorkloadCost    `json:"workloads"`
	Nodes     []NodeCostSummary `json:"nodes"`
	Timestamp time.Time         `json:"timestamp"`
}

// CostBreakdown provides detailed cost breakdown.
type CostBreakdown struct {
	Total      float64 `json:"total"`
	CPU        float64 `json:"cpu"`
	Memory     float64 `json:"memory"`
	Storage    float64 `json:"storage"`
	Network    float64 `json:"network"`
	Billable   float64 `json:"billable"`
	Usage      float64 `json:"usage"`
	Waste      float64 `json:"waste"`
	Efficiency float64 `json:"efficiency"`
}

// WorkloadCost represents cost for a specific workload.
type WorkloadCost struct {
	Name      string  `json:"name"`
	Type      string  `json:"type"` // Deployment, StatefulSet, etc.
	Cost      float64 `json:"cost"`
	PodCount  int     `json:"pod_count"`
	Grade     string  `json:"grade"`
	Namespace string  `json:"namespace"`
}

// NodeCostSummary represents cost summary for a node.
type NodeCostSummary struct {
	Name           string  `json:"name"`
	TotalCost      float64 `json:"total_cost"`
	UtilizationCPU float64 `json:"utilization_cpu"`
	UtilizationMem float64 `json:"utilization_mem"`
	PodCount       int     `json:"pod_count"`
}

// =============================================
// Drilldown DTOs
// =============================================

// DrilldownRequest represents the request for cost drilldown.
type DrilldownRequest struct {
	Level      string `uri:"level" binding:"required,oneof=L0 L1 L2 L3"`
	Identifier string `uri:"identifier" binding:"required"`
	Dimension  string `form:"dimension"` // Optional dimension filter
	StartTime  string `form:"start_time"`
	EndTime    string `form:"end_time"`
}

// DrilldownResponse represents the response for cost drilldown.
type DrilldownResponse struct {
	Level       string                  `json:"level"`
	Identifier  string                  `json:"identifier"`
	Cost        CostBreakdown           `json:"cost"`
	Children    []DrilldownChild        `json:"children"`
	Granularity []GranularCostDataPoint `json:"granularity"`
	Timestamp   time.Time               `json:"timestamp"`
}

// DrilldownChild represents a child item in drilldown.
type DrilldownChild struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	Cost        float64 `json:"cost"`
	Grade       string  `json:"grade"`
	Resource    string  `json:"resource"`
	Utilization float64 `json:"utilization"`
}

// GranularCostDataPoint represents a time-series data point for cost.
type GranularCostDataPoint struct {
	Timestamp time.Time `json:"timestamp"`
	Cost      float64   `json:"cost"`
	Usage     float64   `json:"usage"`
	Waste     float64   `json:"waste"`
}

// =============================================
// Error Response DTO
// =============================================

// ErrorResponse represents a standard error response.
type ErrorResponse struct {
	Error     string `json:"error"`
	Code      string `json:"code"`
	Message   string `json:"message"`
	RequestID string `json:"request_id,omitempty"`
}

// =============================================
// Helper Functions
// =============================================

// ToCostBreakdown converts business model to DTO.
func ToCostBreakdown(result costmodel.CostResult) CostBreakdown {
	return CostBreakdown{
		Total:      result.TotalBillableCost + result.TotalUsageCost + result.TotalWasteCost,
		CPU:        result.CPUBillableCost + result.CPUUsageCost + result.CPUWasteCost,
		Memory:     result.MemBillableCost + result.MemUsageCost + result.MemWasteCost,
		Storage:    0, // Not available in CostResult
		Network:    0, // Not available in CostResult
		Billable:   result.TotalBillableCost,
		Usage:      result.TotalUsageCost,
		Waste:      result.TotalWasteCost,
		Efficiency: result.OverallEfficiencyScore,
	}
}

// ToNamespaceCostSummary converts business model to DTO.
func ToNamespaceCostSummary(nsCost costmodel.DailyNamespaceCost) NamespaceCostSummary {
	return NamespaceCostSummary{
		Name:      nsCost.Namespace,
		Cost:      nsCost.BillableCost + nsCost.UsageCost + nsCost.WasteCost,
		Grade:     "", // Grade not available in DailyNamespaceCost
		PodCount:  nsCost.PodCount,
		NodeCount: nsCost.NodeCount,
	}
}
