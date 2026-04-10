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
//   - PRELIMINARY : 当前月动态同步（可信，未结算）→ 前端展示「动态同步」+ 自动调度说明
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
	// EffectiveTrack 仅当请求含合法 track（technical|finance）时设置，与请求一致。[Ref: 03_Phase6/01_FinOps双轨与全域成本重构/01_设计 §API、track 与 UX]
	EffectiveTrack string `json:"effective_track,omitempty"`
	// FinOpsCGSource 与默认 FINOPS_CG_SOURCE 或 uniform 时一致；多环境混用为 "mixed"。[Ref: 03_Phase6/01_FinOps]
	FinOpsCGSource string `json:"finops_cg_source,omitempty"`
	// FinOpsCGSourceByEnv 当前筛选下各环境实际 C/G 源（oss|api）。[Ref: 03_Phase6/01_FinOps]
	FinOpsCGSourceByEnv map[string]string `json:"finops_cg_source_by_env,omitempty"`
	// LedgerSnapshotNote 五维并列快照与守恒式说明（固定文案）；与 ledger 同传。[Ref: 03_Phase6/01_FinOps双轨与全域成本重构/01_设计 §五维并列快照与 UX]
	LedgerSnapshotNote string `json:"ledger_snapshot_note,omitempty"`
}

// FinOpsLedger 五维并列快照；整块未就绪时省略或 null。单维：查询成功则含数值（可为 0）；查询失败则 omit。[Ref: 03_Phase6/01_FinOps双轨与全域成本重构/01_设计 §ledger 单维语义]
type FinOpsLedger struct {
	C        *float64              `json:"C,omitempty"`
	G        *float64              `json:"G,omitempty"`
	P        *float64              `json:"P,omitempty"`
	U        *float64              `json:"U,omitempty"`
	B        *float64              `json:"B,omitempty"`
	Previous *FinOpsLedgerPrevious `json:"previous,omitempty"`
}

// FinOpsLedgerPrevious 环比上期（可选 C/P）。
type FinOpsLedgerPrevious struct {
	C *float64 `json:"C,omitempty"`
	P *float64 `json:"P,omitempty"`
}

// FinOpsReconciliation 守恒式闭合残差与说明。
type FinOpsReconciliation struct {
	Residual *float64 `json:"residual,omitempty"`
	Explain  string   `json:"explain,omitempty"`
}

// ProjectBreakdownItem 按成本项目汇总（成员环境对应账户金额之和）；ledger_* / consumption_cost 由成员环境 env_breakdown 行加总，与项目卡五维展示一致。[Ref: 03_Phase6/03_前端全域成本透视/01_设计]
type ProjectBreakdownItem struct {
	ProjectID int     `json:"project_id"`
	Code      string  `json:"code"`
	Name      string  `json:"name"`
	SortOrder int     `json:"sort_order"`
	TotalCost float64 `json:"total_cost"`
	// ConsumptionCost 资金轨下为成员环境 consumption 之和；与 EnvBreakdownItem 语义一致。
	ConsumptionCost *float64 `json:"consumption_cost,omitempty"`
	LedgerG           *float64 `json:"ledger_g,omitempty"`
	LedgerP           *float64 `json:"ledger_p,omitempty"`
	LedgerU           *float64 `json:"ledger_u,omitempty"`
	LedgerB           *float64 `json:"ledger_b,omitempty"`
}

// CostProjectItem GET /api/v1/projects 列表项。[Ref: 03_Phase6/03_前端全域成本透视/01_设计]
type CostProjectItem struct {
	ID           int      `json:"id"`
	Code         string   `json:"code"`
	Name         string   `json:"name"`
	SortOrder    int      `json:"sort_order"`
	Environments []string `json:"environments"`
}

// EnvBreakdownItem 按环境的总账与对比；ledger_g/ledger_p 与各账号 FinOps 事实一致（正额/负额汇总与 BSS 已还款）；ledger_b/ledger_u 与各环境 canonical account 的 BSS/应付事实一致。[Ref: 01_设计 §按环境展示、12_API GlobalCostResponse]
// ConsumptionCost 环境卡「应付消耗」：优先为 finops/line_items 正额之和（与 ledger.C 同源拆分）；资金轨聚合路径可先写入再由 FinOps 覆盖。[Ref: 03_Phase6/01_FinOps]
type EnvBreakdownItem struct {
	Environment         string   `json:"environment"`
	AccountID           string   `json:"account_id"`
	AccountDisplayName  string   `json:"account_display_name"`
	TotalCost           float64  `json:"total_cost"`
	ConsumptionCost     *float64 `json:"consumption_cost,omitempty"`
	PreviousPeriodCost  float64  `json:"previous_period_cost,omitempty"`
	ChangePct           float64  `json:"change_pct,omitempty"`
	LedgerG             *float64 `json:"ledger_g,omitempty"`
	LedgerP             *float64 `json:"ledger_p,omitempty"`
	LedgerU             *float64 `json:"ledger_u,omitempty"`
	LedgerB             *float64 `json:"ledger_b,omitempty"`
	// CloudAccountLabel 云环境账户卡主标题，如 C66-Aliyun-Uat；来自 CLOUD_ACCOUNT_DISPLAY_YAML。[Ref: 03_Phase6/03_前端全域成本透视/01_设计]
	CloudAccountLabel string `json:"cloud_account_label,omitempty"`
	// CloudAccountSiteNote 站点说明（国内站/国际站），与 YAML site 对应。[Ref: 03_Phase6/03_前端全域成本透视/01_设计]
	CloudAccountSiteNote string `json:"cloud_account_site_note,omitempty"`
}

// GlobalCostResponse represents the response for global cost overview.
type GlobalCostResponse struct {
	TotalCost        float64                 `json:"total_cost"`
	TotalOptimizable float64                 `json:"total_optimizable"`
	GlobalEfficiency float64                 `json:"global_efficiency"`
	DomainBreakdown  []DomainBreakdownItem   `json:"domain_breakdown"`
	EnvBreakdown     []EnvBreakdownItem       `json:"env_breakdown,omitempty"` // [Ref: 01_设计 D9-4]
	ProjectBreakdown []ProjectBreakdownItem   `json:"project_breakdown,omitempty"` // [Ref: 03_Phase6/03_前端全域成本透视/01_设计]
	Namespaces       []NamespaceCostSummary  `json:"namespaces"`
	Timestamp        time.Time               `json:"timestamp"`
	Metadata         *GlobalCostMetadata     `json:"metadata,omitempty"`
	Ledger           *FinOpsLedger           `json:"ledger,omitempty"`
	Reconciliation   *FinOpsReconciliation   `json:"reconciliation,omitempty"`
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

// ApplyFinOpsGlobalMetadata 在请求含合法 track 时写入 metadata.effective_track。[Ref: 03_Phase6/01_FinOps双轨与全域成本重构/01_设计 §API、track 与 UX]
func ApplyFinOpsGlobalMetadata(resp *GlobalCostResponse, track string) {
	if track != "technical" && track != "finance" {
		return
	}
	if resp == nil {
		return
	}
	if resp.Metadata == nil {
		resp.Metadata = &GlobalCostMetadata{}
	}
	resp.Metadata.EffectiveTrack = track
}

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
