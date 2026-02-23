// Package aliyun 实现阿里云 BssOpenApi 账单拉取（15_ 规范）。凭证仅从环境变量或 Secret 读取。
// 不依赖 cloudbilling 包以避免循环依赖；由 cloudbilling 工厂做适配。
package aliyun

import (
	"context"
	"log/slog"
	"os"
	"sync"
	"time"

	"github.com/alibabacloud-go/bssopenapi-20171214/v4/client"
	openapi "github.com/alibabacloud-go/darabonba-openapi/v2/client"
	"github.com/alibabacloud-go/tea/tea"
)

const (
	envAccessKeyID     = "ALIBABA_CLOUD_ACCESS_KEY_ID"
	envAccessKeySecret = "ALIBABA_CLOUD_ACCESS_KEY_SECRET"
	maxRetries         = 3
	baseBackoff        = time.Second
	circuitFailThreshold = 5
	circuitOpenDuration  = 60 * time.Second
)

// BillOverviewResult 账期总账与按产品金额（供工厂转换为 FetchAccountSummaryResponse）。
type BillOverviewResult struct {
	BillingCycle string
	TotalAmount  float64
	ByCategory   map[string]float64
	Currency     string
}

// Fetcher 阿里云 BssOpenApi 拉取，带退避重试与熔断。
type Fetcher struct {
	bssClient          *client.Client
	consecutiveFailures int
	circuitOpenUntil    time.Time
	mu                  sync.Mutex
}

// NewFetcher 从环境变量读取 AccessKeyId/AccessKeySecret 创建 Fetcher。未设置凭证时返回 nil, false。
func NewFetcher() (*Fetcher, bool) {
	ak := os.Getenv(envAccessKeyID)
	sk := os.Getenv(envAccessKeySecret)
	if ak == "" || sk == "" {
		return nil, false
	}
	cfg := &openapi.Config{
		AccessKeyId:     tea.String(ak),
		AccessKeySecret: tea.String(sk),
		Endpoint:        tea.String("business.aliyuncs.com"),
	}
	c, err := client.NewClient(cfg)
	if err != nil {
		slog.Warn("aliyun billing: NewClient failed", "error", err)
		return nil, false
	}
	return &Fetcher{bssClient: c}, true
}

// FetchBillOverview 拉取账期总账单与按产品占比。退避重试；熔断时返回错误。
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
			// 可观测：成功调用与延迟（15_、文档 4.1 第5条）
			slog.Info("aliyun billing: fetch ok", "billing_cycle", billingCycle, "duration_ms", time.Since(start).Milliseconds(), "total", resp.TotalAmount)
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

func (f *Fetcher) queryBillOverview(ctx context.Context, billingCycle string) (*BillOverviewResult, error) {
	req := &client.QueryBillOverviewRequest{
		BillingCycle: tea.String(billingCycle),
	}
	resp, err := f.bssClient.QueryBillOverviewWithOptions(req, nil)
	if err != nil {
		return nil, err
	}
	if resp == nil || resp.Body == nil || resp.Body.Data == nil {
		return &BillOverviewResult{
			BillingCycle: billingCycle,
			TotalAmount:  0,
			ByCategory:   make(map[string]float64),
			Currency:     "CNY",
		}, nil
	}
	data := resp.Body.Data
	total := 0.0
	byCategory := make(map[string]float64)
	if data.Items != nil && len(data.Items.Item) > 0 {
		for _, it := range data.Items.Item {
			amount := 0.0
			if it.PretaxAmount != nil {
				amount = float64(*it.PretaxAmount)
			}
			total += amount
			productCode := "other"
			if it.ProductCode != nil && *it.ProductCode != "" {
				productCode = *it.ProductCode
			}
			byCategory[productCode] += amount
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
		Currency:     "CNY",
	}, nil
}

// ErrCircuitOpen 熔断打开时返回。
var ErrCircuitOpen = errCircuitOpen{}

type errCircuitOpen struct{}

func (errCircuitOpen) Error() string { return "cloud billing circuit open" }
