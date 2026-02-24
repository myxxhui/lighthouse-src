// Package aliyun 实现阿里云 BssOpenApi 账单拉取（15_ 规范）。凭证仅从环境变量或 Secret 读取。
// 不依赖 cloudbilling 包以避免循环依赖；由 cloudbilling 工厂做适配。
package aliyun

import (
	"context"
	"log/slog"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/alibabacloud-go/bssopenapi-20171214/v4/client"
	openapi "github.com/alibabacloud-go/darabonba-openapi/v2/client"
	"github.com/alibabacloud-go/tea/tea"
	"github.com/alibabacloud-go/tea-utils/v2/service"
)

const (
	envAccessKeyID         = "ALIBABA_CLOUD_ACCESS_KEY_ID"
	envAccessKeySecret     = "ALIBABA_CLOUD_ACCESS_KEY_SECRET"
	envBillingEndpoint     = "ALIBABA_CLOUD_BILLING_ENDPOINT" // 可选；国际站填 business.ap-southeast-1.aliyuncs.com
	defaultBillingEndpoint = "business.aliyuncs.com"          // 中国站
	maxRetries             = 3
	baseBackoff            = time.Second
	circuitFailThreshold   = 5
	circuitOpenDuration    = 60 * time.Second
)

// productCodeToDomain 将阿里云产品码映射为领域名（15_ 规范：算力/存储/网络/其它）。未匹配的归为「其它」，保证领域汇总之和=总账。
var productCodeToDomain = map[string]string{
	"ecs": "计算资源", "ack": "计算资源", "cs": "计算资源", "ecs_workflow": "计算资源",
	"oss": "存储", "nas": "存储", "disk": "存储",
	"cdn": "网络", "slb": "网络", "vpc": "网络", "eip": "网络",
	"cdt": "其它", "sfm": "其它", // 未归入计算/存储/网络的产品统一归为其它，避免与总账不一致
}

// BillItemResult 单产品金额，用于产品级明细与 top-N 展示。
type BillItemResult struct {
	ProductCode string
	Amount      float64
	Domain      string
}

// BillOverviewResult 账期总账与按产品金额（供工厂转换为 FetchAccountSummaryResponse）。
type BillOverviewResult struct {
	BillingCycle string
	TotalAmount  float64
	ByCategory   map[string]float64
	Items        []BillItemResult
	Currency     string
}

// Fetcher 阿里云 BssOpenApi 拉取，带退避重试与熔断。
type Fetcher struct {
	bssClient          *client.Client
	consecutiveFailures int
	circuitOpenUntil    time.Time
	mu                  sync.Mutex
}

// NewFetcher 从环境变量读取 AccessKeyId/AccessKeySecret 创建 Fetcher；endpoint 可选（空则用环境变量或中国站默认）。
// 国际站账号须指定 endpoint，如 business.ap-southeast-1.aliyuncs.com，否则会报 NotApplicable（caller site 与 regionId 不匹配）。
func NewFetcher(endpoint string) (*Fetcher, bool) {
	ak := os.Getenv(envAccessKeyID)
	sk := os.Getenv(envAccessKeySecret)
	if ak == "" || sk == "" {
		return nil, false
	}
	if endpoint == "" {
		endpoint = os.Getenv(envBillingEndpoint)
	}
	if endpoint == "" {
		endpoint = defaultBillingEndpoint
	}
	cfg := &openapi.Config{
		AccessKeyId:     tea.String(ak),
		AccessKeySecret: tea.String(sk),
		Endpoint:        tea.String(endpoint),
	}
	c, err := client.NewClient(cfg)
	if err != nil {
		slog.Warn("aliyun billing: NewClient failed", "error", err)
		return nil, false
	}
	return &Fetcher{bssClient: c}, true
}

// FetchBillOverview 拉取账期总账单与按产品占比。退避重试；熔断时返回错误。
// FetchBillOverview 拉取账期总账单与按产品占比。当月无数据时自动尝试上月账期（01_ 修复：避免 total_cost/domain_breakdown 始终为空）。
func (f *Fetcher) FetchBillOverview(ctx context.Context, billingCycle string) (*BillOverviewResult, error) {
	f.mu.Lock()
	if time.Now().Before(f.circuitOpenUntil) {
		f.mu.Unlock()
		return nil, ErrCircuitOpen
	}
	f.mu.Unlock()

	if billingCycle == "" {
		billingCycle = time.Now().Format("2006-01")
	}

	var lastErr error
	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			backoff := baseBackoff * time.Duration(1<<uint(attempt-1))
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(backoff):
			}
		}
		start := time.Now()
		resp, err := f.queryBillOverview(ctx, billingCycle)
		if err == nil {
			f.mu.Lock()
			f.consecutiveFailures = 0
			f.mu.Unlock()
			slog.Info("aliyun billing: fetch ok", "billing_cycle", billingCycle, "duration_ms", time.Since(start).Milliseconds(), "total", resp.TotalAmount)
			// 当月无数据时尝试上月账期，便于新环境或当月未出账时仍有展示
			if resp.TotalAmount == 0 && len(resp.ByCategory) == 0 {
				prevCycle := prevMonthBillingCycle(billingCycle)
				if prevCycle != billingCycle {
					prev, err2 := f.queryBillOverview(ctx, prevCycle)
					if err2 == nil && (prev.TotalAmount > 0 || len(prev.ByCategory) > 0) {
						slog.Info("aliyun billing: using previous cycle", "current", billingCycle, "previous", prevCycle, "total", prev.TotalAmount)
						return prev, nil
					}
				}
			}
			return resp, nil
		}
		lastErr = err
		slog.Warn("aliyun billing: fetch failed", "billing_cycle", billingCycle, "attempt", attempt+1, "error", err)
		f.mu.Lock()
		f.consecutiveFailures++
		if f.consecutiveFailures >= circuitFailThreshold {
			f.circuitOpenUntil = time.Now().Add(circuitOpenDuration)
			f.mu.Unlock()
			slog.Warn("aliyun billing: circuit open", "duration_sec", int64(circuitOpenDuration.Seconds()))
			return nil, lastErr
		}
		f.mu.Unlock()
	}
	return nil, lastErr
}

func prevMonthBillingCycle(cycle string) string {
	if len(cycle) != 7 || cycle[4] != '-' {
		return cycle
	}
	y, _ := time.Parse("2006-01", cycle)
	prev := y.AddDate(0, -1, 0)
	return prev.Format("2006-01")
}

func (f *Fetcher) queryBillOverview(ctx context.Context, billingCycle string) (*BillOverviewResult, error) {
	req := &client.QueryBillOverviewRequest{
		BillingCycle: tea.String(billingCycle),
	}
	resp, err := f.bssClient.QueryBillOverviewWithOptions(req, &service.RuntimeOptions{})
	if err != nil {
		return nil, err
	}
	if resp == nil || resp.Body == nil || resp.Body.Data == nil {
		return &BillOverviewResult{
			BillingCycle: billingCycle,
			TotalAmount:  0,
			ByCategory:   make(map[string]float64),
			Items:        nil,
			Currency:     "CNY",
		}, nil
	}
	data := resp.Body.Data
	total := 0.0
	byCategory := make(map[string]float64)
	var items []BillItemResult
	if data.Items != nil && len(data.Items.Item) > 0 {
		for _, it := range data.Items.Item {
			amount := 0.0
			if it.PretaxAmount != nil {
				amount = float64(*it.PretaxAmount)
			}
			total += amount
			domain := "其它"
			codeStr := "OTHER"
			if it.ProductCode != nil && *it.ProductCode != "" {
				codeStr = strings.ToUpper(strings.TrimSpace(*it.ProductCode))
				if d, ok := productCodeToDomain[strings.ToLower(*it.ProductCode)]; ok {
					domain = d
				}
			}
			byCategory[domain] += amount
			items = append(items, BillItemResult{ProductCode: codeStr, Amount: amount, Domain: domain})
		}
	}
	cycle := billingCycle
	if data.BillingCycle != nil && *data.BillingCycle != "" {
		cycle = *data.BillingCycle
	}
	return &BillOverviewResult{
		BillingCycle: cycle,
		TotalAmount:  total,
		ByCategory:   byCategory,
		Items:        items,
		Currency:     "CNY",
	}, nil
}

// ErrCircuitOpen 熔断打开时返回。
var ErrCircuitOpen = errCircuitOpen{}

type errCircuitOpen struct{}

func (errCircuitOpen) Error() string { return "cloud billing circuit open" }
