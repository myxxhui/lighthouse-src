// Package cloudbilling 定义云账单拉取接口与类型，供 ETL/业务层依赖。
// 具体实现（如 aliyun/aws）在子包中；AKSK 仅通过环境变量或 Secret 注入，禁止配置明文。
package cloudbilling

import (
	"context"
	"time"
)

// FetchAccountSummaryRequest 拉取账户总账单汇总的请求。
// BillingCycle: 账期，如 "2025-01"（月）或 "2025-01-01"（日）
// PeriodType: "day" | "month"
// CategoryFilter: 可选，如 ["compute","storage","network"]
type FetchAccountSummaryRequest struct {
	BillingCycle   string   `json:"billing_cycle"`
	PeriodType     string   `json:"period_type"` // "day" | "month"
	CategoryFilter []string `json:"category_filter,omitempty"`
}

// FetchAccountSummaryResponse 账户总账单汇总响应（dual-metric v3）。
// TotalAmount / ByCategory     = PretaxAmount（消耗价值）
// CashTotalAmount / CashByCategory = CashAmount（支付价值）
type FetchAccountSummaryResponse struct {
	BillingCycle    string             `json:"billing_cycle"`
	TotalAmount     float64            `json:"total_amount"`      // 消耗价值（PretaxAmount）
	CashTotalAmount float64            `json:"cash_total_amount"` // 支付价值（CashAmount）
	Currency        string             `json:"currency"`
	ByCategory      map[string]float64 `json:"by_category"`
	CashByCategory  map[string]float64 `json:"cash_by_category,omitempty"`
	Items        []BillItem         `json:"items,omitempty"` // 可选，产品/计费项明细
}

// BillItem 账单明细项（可选，用于对账或下钻）
type BillItem struct {
	ProductCode string  `json:"product_code"`
	ItemCode    string  `json:"item_code"`
	Amount      float64 `json:"amount"`
	Category    string  `json:"category"`
}

// BillLineItem 行级流水条目，对应阿里云 QueryAccountBill 每一行原始记录。
// [Ref: 16_云账单动态对账与高可靠处理规范 §三]
// RecordID: 阿里云 RecordID；若 API 未返回则由业务键构造
// CashAmount: 现金实付（含负数冲正，禁止过滤）；PretaxAmount: 折后应付（辅助）
type BillLineItem struct {
	RecordID          string  `json:"record_id"`
	BillingDate       string  `json:"billing_date"`   // YYYY-MM-DD
	BillingCycle      string  `json:"billing_cycle"`  // YYYY-MM
	ProductCode       string  `json:"product_code"`
	ProductName       string  `json:"product_name"`
	SubOrderID        string  `json:"sub_order_id"`
	InstanceID        string  `json:"instance_id"`
	BillingItem       string  `json:"billing_item"`
	SubscriptionType  string  `json:"subscription_type"`
	CashAmount        float64 `json:"cash_amount"`           // 现金支付（含负数）
	PretaxAmount      float64 `json:"pretax_amount"`         // 折后应付（辅助）
	PretaxGrossAmount float64 `json:"pretax_gross_amount"`   // 官网原价（辅助）
	Category          string  `json:"category"`              // compute/storage/network/security/other
}

// FetchLineItemsRequest 拉取行级流水的请求。
// BillingDate: YYYY-MM-DD；BillingCycle 由 BillingDate 自动推导
type FetchLineItemsRequest struct {
	BillingDate string `json:"billing_date"` // YYYY-MM-DD
}

// FetchLineItemsResponse 行级流水响应。
type FetchLineItemsResponse struct {
	BillingDate string         `json:"billing_date"`
	BillingCycle string        `json:"billing_cycle"`
	Items        []BillLineItem `json:"items"`
}

// BSSTransactionItem QueryAccountTransactions 单条流水，供 ETL 映射至 cost_bss_transactions。[Ref: 03_Phase6/01_FinOps]
type BSSTransactionItem struct {
	TransactionNumber string
	TransactionTime   time.Time
	Amount            float64
	TransactionType   string
	TransactionFlow   string
	RecordID          string
	BillingCycle      string
	Currency          string
}

// CloudBillingFetcher 云账单拉取接口。业务/ETL 仅依赖此接口与工厂获取实现。
// Phase3 为占位；Phase4 接入真实云厂商（如 aliyun BSS/费用中心）。
type CloudBillingFetcher interface {
	FetchAccountSummary(ctx context.Context, req FetchAccountSummaryRequest) (*FetchAccountSummaryResponse, error)
	// FetchLineItems 拉取指定日期的行级流水（IsGroupByProduct=false），含 RecordID 与 CashAmount。
	// [Ref: 16_云账单动态对账与高可靠处理规范 §四] 用于幂等 Upsert 流水表与窗口回溯
	FetchLineItems(ctx context.Context, req FetchLineItemsRequest) (*FetchLineItemsResponse, error)
	// FetchBSSTransactions 分页拉取 [start,end] 内账户流水（CreateTime ISO8601）。[Ref: 03_Phase6/01_FinOps]
	FetchBSSTransactions(ctx context.Context, start, end time.Time) ([]BSSTransactionItem, error)
	// FetchAccountBalanceSnapshot QueryAccountBalance → 可用余额与币种。[Ref: 03_Phase6/01_FinOps]
	FetchAccountBalanceSnapshot(ctx context.Context) (available float64, currency string, err error)
	// FetchOutstandingMonthly QueryAccountBill MONTHLY+IsGroupByProduct 汇总 OutstandingAmount。[Ref: 03_Phase6/01_FinOps]
	FetchOutstandingMonthly(ctx context.Context, billingCycle string) (float64, error)
	// FetchCallingAccountID QueryAccountBill 返回 Data.AccountID（阿里云主账号 ID），与流水/BSS 落库主键对齐。[Ref: 03_Phase6/01_FinOps]
	FetchCallingAccountID(ctx context.Context, billingCycle string) (string, error)
}
