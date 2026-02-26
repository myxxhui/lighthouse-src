// Package cloudbilling 工厂：根据 Config.CloudBilling.Provider 返回 CloudBillingFetcher 实现。
// 凭证仅从环境变量（ALIBABA_CLOUD_ACCESS_KEY_ID/SECRET）或 K8s Secret 注入，不在配置明文。
package cloudbilling

import (
	"context"

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
		BillingCycle: res.BillingCycle,
		TotalAmount:  res.TotalAmount,
		Currency:     res.Currency,
		ByCategory:   res.ByCategory,
		Items:        items,
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

// NewFetcherForEnv 按环境名（POC/FAT/UAT/PROD）从环境变量 ALIBABA_CLOUD_ACCESS_KEY_ID_<env>/SECRET_<env> 创建 Fetcher。[Ref: 01_实践 §3.3(3a) 多账号]
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
