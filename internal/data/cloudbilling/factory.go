// Package cloudbilling 工厂：根据 Config.CloudBilling.Provider 返回 CloudBillingFetcher 实现。
// 凭证仅从环境变量（ALIBABA_CLOUD_ACCESS_KEY_ID/SECRET）或 K8s Secret 注入，不在配置明文。
package cloudbilling

import (
	"context"
	"time"

	"github.com/myxxhui/lighthouse-src/internal/data/cloudbilling/aliyun"
)

// CloudBillingConfig 云账单配置（与 config.CloudBillingConfig 对齐）。Provider 决定工厂返回的实现。
// AccessKeyID / AccessKeySecret 仅由环境变量或 Secret 填充，不落配置文件。
type CloudBillingConfig struct {
	Provider     string `json:"provider"`      // "aliyun" | "aws" | "tencent" | ""
	Endpoint     string `json:"endpoint"`      // 可选
	PeriodType   string `json:"period_type"`   // "day" | "month"
	BillingCycle string `json:"billing_cycle"` // 账期，如 2025-01
}

// aliCloudFetcher 适配 aliyun.Fetcher 为 CloudBillingFetcher（避免 aliyun 依赖 cloudbilling 造成循环引用）。
type aliCloudFetcher struct{ inner *aliyun.Fetcher }

// FetchLineItems 拉取行级流水（IsGroupByProduct=false），含 RecordID 与 CashAmount（含负数冲正）。
// [Ref: 16_云账单动态对账与高可靠处理规范 §四]
func (a *aliCloudFetcher) FetchLineItems(ctx context.Context, req FetchLineItemsRequest) (*FetchLineItemsResponse, error) {
	items, err := a.inner.FetchLineItemsByDay(ctx, req.BillingDate)
	if err != nil {
		return nil, err
	}
	billingCycle := req.BillingDate
	if len(req.BillingDate) >= 7 {
		billingCycle = req.BillingDate[:7]
	}
	out := make([]BillLineItem, 0, len(items))
	for _, it := range items {
		out = append(out, BillLineItem{
			RecordID:          it.RecordID,
			BillingDate:       it.BillingDate,
			BillingCycle:      it.BillingCycle,
			ProductCode:       it.ProductCode,
			ProductName:       it.ProductName,
			SubOrderID:        it.SubOrderID,
			InstanceID:        it.InstanceID,
			BillingItem:       it.BillingItem,
			SubscriptionType:  it.SubscriptionType,
			CashAmount:        it.CashAmount,
			PretaxAmount:      it.PretaxAmount,
			PretaxGrossAmount: it.PretaxGrossAmount,
			Category:          it.Category,
		})
	}
	return &FetchLineItemsResponse{BillingDate: req.BillingDate, BillingCycle: billingCycle, Items: out}, nil
}

func (a *aliCloudFetcher) FetchBSSTransactions(ctx context.Context, start, end time.Time) ([]BSSTransactionItem, error) {
	items, err := a.inner.FetchBSSTransactions(ctx, start, end)
	if err != nil {
		return nil, err
	}
	out := make([]BSSTransactionItem, 0, len(items))
	for _, it := range items {
		out = append(out, BSSTransactionItem{
			TransactionNumber: it.TransactionNumber,
			TransactionTime:   it.TransactionTime,
			Amount:            it.Amount,
			TransactionType:   it.TransactionType,
			TransactionFlow:   it.TransactionFlow,
			RecordID:          it.RecordID,
			BillingCycle:      it.BillingCycle,
			Currency:          it.Currency,
			TransactionChannel: it.TransactionChannel,
			FundType:           it.FundType,
			Remarks:            it.Remarks,
		})
	}
	return out, nil
}

func (a *aliCloudFetcher) FetchAccountBalanceSnapshot(ctx context.Context) (float64, string, error) {
	return a.inner.FetchAccountBalanceSnapshot(ctx)
}

func (a *aliCloudFetcher) FetchOutstandingMonthly(ctx context.Context, billingCycle string) (float64, error) {
	return a.inner.SumOutstandingMonthly(ctx, billingCycle)
}

func (a *aliCloudFetcher) FetchCallingAccountID(ctx context.Context, billingCycle string) (string, error) {
	return a.inner.FetchCallingAccountID(ctx, billingCycle)
}

func (a *aliCloudFetcher) FetchCouponDeductionMonthly(ctx context.Context, billingCycle string) (float64, float64, error) {
	return a.inner.SumCouponDeductionPartsForBillingCycle(ctx, billingCycle)
}

func (a *aliCloudFetcher) FetchAccountSummary(ctx context.Context, req FetchAccountSummaryRequest) (*FetchAccountSummaryResponse, error) {
	// [Ref: 01_设计 §拉取粒度与落表] day → QueryAccountBill DAILY(BillingDate)；month → QueryBillOverview(BillingCycle)
	var res *aliyun.BillOverviewResult
	var err error
	if req.PeriodType == "day" && req.BillingCycle != "" {
		res, err = a.inner.FetchBillOverviewByDay(ctx, req.BillingCycle)
	} else {
		res, err = a.inner.FetchBillOverview(ctx, req.BillingCycle)
	}
	if err != nil {
		return nil, err
	}
	items := make([]BillItem, 0, len(res.Items))
	for _, it := range res.Items {
		items = append(items, BillItem{
			ProductCode: it.ProductCode,
			ItemCode:    "",
			Amount:      it.Amount,
			Category:    it.Domain,
		})
	}
	return &FetchAccountSummaryResponse{
		BillingCycle:    res.BillingCycle,
		TotalAmount:     res.TotalAmount,
		CashTotalAmount: res.CashTotalAmount,
		Currency:        res.Currency,
		ByCategory:      res.ByCategory,
		CashByCategory:  res.CashByCategory,
		Items:           items,
	}, nil
}

// NewFetcher 根据配置返回 CloudBillingFetcher 实现。aliyun 时从环境变量读取凭证（无后缀）；无凭证或未实现时返回 nil。
func NewFetcher(cfg CloudBillingConfig) CloudBillingFetcher {
	switch cfg.Provider {
	case "aliyun":
		f, ok := aliyun.NewFetcher(cfg.Endpoint)
		if !ok {
			return nil
		}
		return &aliCloudFetcher{inner: f}
	default:
		return nil
	}
}

// NewFetcherForEnv 按后缀从环境变量 ALIBABA_CLOUD_ACCESS_KEY_ID_<suffix>/SECRET_<suffix> 创建 Fetcher；suffix 可为 POC/UAT 或与项目 YAML 一致的 environment_key（如 C66_UAT）。[Ref: 01_实践 §3.3(3a) 多账号]
func NewFetcherForEnv(environment string) CloudBillingFetcher {
	if environment == "" {
		return nil
	}
	f, ok := aliyun.NewFetcherForEnv(environment)
	if !ok {
		return nil
	}
	return &aliCloudFetcher{inner: f}
}

// NewAliyunFetcherFromCredentials 显式凭证 + BSS Endpoint（YAML 项目环境）；endpoint 空则中国站默认。[Ref: 03_Phase6 项目云账号]
func NewAliyunFetcherFromCredentials(accessKeyID, secretAccessKey, bssEndpoint string) CloudBillingFetcher {
	f, ok := aliyun.NewFetcherWithCredentials(accessKeyID, secretAccessKey, bssEndpoint)
	if !ok {
		return nil
	}
	return &aliCloudFetcher{inner: f}
}
