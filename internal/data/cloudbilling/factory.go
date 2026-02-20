// Package cloudbilling 工厂：根据 Config.CloudBilling.Provider 返回 CloudBillingFetcher 实现。
// AKSK 仅从环境变量（如 CLOUD_BILL_AK、CLOUD_BILL_SK）或 Secret 注入，不在配置明文。
package cloudbilling

// CloudBillingConfig 云账单配置（占位）。Provider 决定工厂返回的实现。
// AccessKeyID / AccessKeySecret 仅由环境变量或 Secret 填充，不落配置文件。
type CloudBillingConfig struct {
	Provider   string `json:"provider"`   // "aliyun" | "aws" | "tencent" | ""
	Endpoint   string `json:"endpoint"`   // 可选
	PeriodType string `json:"period_type"`
}

// NewFetcher 根据配置返回 CloudBillingFetcher 实现。Phase3 占位：无 Provider 或未实现时返回 nil。
// 调用方需判断 nil，Phase4 接入 aliyun/ 等实现。
func NewFetcher(cfg CloudBillingConfig) CloudBillingFetcher {
	switch cfg.Provider {
	case "aliyun", "aws", "tencent":
		// Phase4: return aliyun.NewFetcher(cfg) 等
		return nil
	default:
		return nil
	}
}
