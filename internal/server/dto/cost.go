// Package dto defines Data Transfer Objects for HTTP API requests and responses.
package dto

import (
	"time"

	"github.com/myxxhui/lighthouse-src/pkg/costmodel"
)

// =============================================
// Global Cost DTOs
// =============================================

// ProductCostItem 产品级成本（用于账单详情 top-N 与详情跳转）。
type ProductCostItem struct {
	Product string  `json:"product"`
	Cost    float64 `json:"cost"`
}

// DomainBreakdownItem represents a domain in the cost breakdown pie chart.
type DomainBreakdownItem struct {
	Domain           string             `json:"domain"`
	Cost             float64            `json:"cost"`
	OptimizableSpace float64            `json:"optimizable_space"`
	Efficiency       float64            `json:"efficiency"`
	TopProducts      []ProductCostItem  `json:"top_products,omitempty"` // 成本最高的最多 4 个产品，用于详情与跳转
}

// GlobalCostMetadata D1-3：数据更新至、来源（聚合表/原始表降级）；含 report_type/period_key 便于校验时间范围对应关系。
// [Ref: 16_云账单动态对账与高可靠处理规范 §三段式聚合] BillDataStatus 字段语义：
//   - FINALIZED   : 已对账结算（历史月，权威级别）→ 前端展示「已财务核算」
//   - PRELIMINARY : 当前月动态同步中（可信，未结算）→ 前端展示「动态同步中」
//   - RECONCILING : 对账工作线运行中（修复中）→ 前端展示「对账中」
//   - DIRTY       : 发现偏差，待修复              → 前端展示「数据偏差」
//   - aggregate   : 来自聚合缓存（兼容旧语义）
//   - fallback    : 来自原始表降级聚合
type GlobalCostMetadata struct {
	LastUpdatedAt  *time.Time `json:"last_updated_at,omitempty"`  // 聚合完成时间，前端展示「数据更新至」
	DataStatus     string     `json:"data_status,omitempty"`      // "aggregate"|"fallback"|"FINALIZED"|"PRELIMINARY"|"RECONCILING"|"DIRTY"
	BillDataStatus string     `json:"bill_data_status,omitempty"` // 账单对账状态（来自 month_status 表）
	ReportType     string     `json:"report_type,omitempty"`      // 1d|7d|30d|month|quarter 等，对应聚合表 report_type
	PeriodKey      string     `json:"period_key,omitempty"`       // 对应聚合表 period_key，用于校验当前时间范围
	// DisplayNote 展示说明：月粒度周期内净退款导致现金合计为负时，后端将金额展示为 0 并设置本字段，前端可展示「该周期净退款已抵减」
	DisplayNote string `json:"display_note,omitempty"`
}

// EnvBreakdownItem 按环境（POC/FAT/UAT/PROD）的总账与对比。[Ref: 01_设计 §按环境展示、12_API GlobalCostResponse]
type EnvBreakdownItem struct {
	Environment         string   `json:"environment"`
	AccountID           string   `json:"account_id"`
	AccountDisplayName  string   `json:"account_display_name"`
	TotalCost           float64  `json:"total_cost"`
	PreviousPeriodCost  float64  `json:"previous_period_cost,omitempty"`
	ChangePct           float64  `json:"change_pct,omitempty"`
}

// GlobalCostResponse represents the response for global cost overview.
type GlobalCostResponse struct {
	TotalCost        float64                 `json:"total_cost"`
	TotalOptimizable float64                 `json:"total_optimizable"`
	GlobalEfficiency float64                 `json:"global_efficiency"`
	DomainBreakdown  []DomainBreakdownItem   `json:"domain_breakdown"`
	EnvBreakdown     []EnvBreakdownItem       `json:"env_breakdown,omitempty"` // [Ref: 01_设计 D9-4]
	Namespaces       []NamespaceCostSummary  `json:"namespaces"`
	Timestamp        time.Time               `json:"timestamp"`
	Metadata         *GlobalCostMetadata     `json:"metadata,omitempty"`
}

// EnvDrilldownItem 按环境钻取：云产品维度成本。[Ref: 01_设计 §产品分类与按环境钻取、12_API]
type EnvDrilldownItem struct {
	ProductCode string  `json:"product_code"`
	ProductName string  `json:"product_name,omitempty"`
	Cost        float64 `json:"cost"`
	Category    string  `json:"category"` // compute|network|storage|security|other
}

// CostTrendDataPoint 成本趋势按日/按月数据点。[Ref: 12_API GET /api/v1/cost/trend]
type CostTrendDataPoint struct {
	Date      string             `json:"date"`
	TotalCost float64            `json:"total_cost"`
	ByDomain  map[string]float64 `json:"by_domain,omitempty"`
	ByProduct map[string]float64 `json:"by_product,omitempty"`
}

// CostTrendResponse 成本结构趋势响应；最大 90 天、超时 10s。[Ref: 01_设计 §成本趋势 API、12_API]
type CostTrendResponse struct {
	Data []CostTrendDataPoint `json:"data"`
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
