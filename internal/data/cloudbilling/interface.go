// Package cloudbilling 定义云账单拉取接口与类型，供 ETL/业务层依赖。
// 具体实现（如 aliyun/aws）在子包中；AKSK 仅通过环境变量或 Secret 注入，禁止配置明文。
package cloudbilling

import "context"

// FetchAccountSummaryRequest 拉取账户总账单汇总的请求。
// BillingCycle: 账期，如 "2025-01"（月）或 "2025-01-01"（日）
// PeriodType: "day" | "month"
// CategoryFilter: 可选，如 ["compute","storage","network"]
type FetchAccountSummaryRequest struct {
	BillingCycle   string   `json:"billing_cycle"`
	PeriodType     string   `json:"period_type"` // "day" | "month"
	CategoryFilter []string `json:"category_filter,omitempty"`
}

// FetchAccountSummaryResponse 账户总账单汇总响应。
// ByCategory: compute/storage/network/other/unassigned 对应金额（元）
type FetchAccountSummaryResponse struct {
	BillingCycle string             `json:"billing_cycle"`
	TotalAmount  float64            `json:"total_amount"`
	Currency     string             `json:"currency"`
	ByCategory   map[string]float64 `json:"by_category"`
	Items        []BillItem         `json:"items,omitempty"` // 可选，产品/计费项明细
}

// BillItem 账单明细项（可选，用于对账或下钻）
type BillItem struct {
	ProductCode string  `json:"product_code"`
	ItemCode    string  `json:"item_code"`
	Amount      float64 `json:"amount"`
	Category    string  `json:"category"`
}

// CloudBillingFetcher 云账单拉取接口。业务/ETL 仅依赖此接口与工厂获取实现。
// Phase3 为占位；Phase4 接入真实云厂商（如 aliyun BSS/费用中心）。
type CloudBillingFetcher interface {
	FetchAccountSummary(ctx context.Context, req FetchAccountSummaryRequest) (*FetchAccountSummaryResponse, error)
}
