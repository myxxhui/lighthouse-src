// Package aliyun 实现阿里云 BssOpenApi 账单拉取（15_ 规范）。凭证仅从环境变量或 Secret 读取。
// 不依赖 cloudbilling 包以避免循环依赖；由 cloudbilling 工厂做适配。
package aliyun

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/alibabacloud-go/bssopenapi-20171214/v4/client"
	openapi "github.com/alibabacloud-go/darabonba-openapi/v2/client"
	"github.com/alibabacloud-go/tea-utils/v2/service"
	"github.com/alibabacloud-go/tea/tea"
)

const (
	envAccessKeyID         = "ALIBABA_CLOUD_ACCESS_KEY_ID"
	envAccessKeySecret     = "ALIBABA_CLOUD_ACCESS_KEY_SECRET"
	envBillingEndpoint     = "ALIBABA_CLOUD_BILLING_ENDPOINT" // 可选；国际站填 business.ap-southeast-1.aliyuncs.com
	envBillingEndpointAlt  = "CLOUD_BILLING_ENDPOINT"         // 与设计/实践文档一致
	defaultBillingEndpoint = "business.aliyuncs.com"          // 中国站
	// DefaultIntlBillingEndpoint 国际站 BSS OpenAPI 常用域名（与阿里云文档「国际站费用」一致；临时 CLI 可在未配 env 时选用）。[Ref: 03_Phase6/01_FinOps]
	DefaultIntlBillingEndpoint = "business.ap-southeast-1.aliyuncs.com"
	maxRetries                 = 3
	baseBackoff            = time.Second
	circuitFailThreshold   = 5
	circuitOpenDuration    = 60 * time.Second
)

// productCodeToDomain 将阿里云产品码显式映射为四大类（计算资源/存储/网络/安全），无「其它」分类。
// 所有已知产品码均在下表中归入四类之一；仅当账单项产品码为空（API 未返回或空串）时兜底归入计算资源。
// 新上市产品码需补充进下表，保证全量产品均显式映射。[Ref: 用户需求 仅四大分类、所有产品映射到四类]
var productCodeToDomain = map[string]string{
	// 计算资源：ECS、容器、函数、数据库、大数据、消息等
	"ecs": "计算资源", "ack": "计算资源", "cs": "计算资源", "ecs_workflow": "计算资源", "sfm": "计算资源",
	"fc": "计算资源", "sae": "计算资源", "batchcompute": "计算资源", "emr": "计算资源", "openanalytics": "计算资源",
	"rds": "计算资源", "polardb": "计算资源", "cddc": "计算资源", "redis": "计算资源", "memcache": "计算资源",
	"mongodb": "计算资源", "hbase": "计算资源", "lindorm": "计算资源", "drds": "计算资源", "adb": "计算资源",
	"dts": "计算资源", "hbr": "计算资源", "oos": "计算资源", "cr": "计算资源", "acr": "计算资源",
	"arms": "计算资源", "gts": "计算资源", "mq": "计算资源", "amqp": "计算资源", "kafka": "计算资源",
	"rocketmq": "计算资源", "eventbridge": "计算资源", "fnf": "计算资源", "serverless": "计算资源",
	"cassandra": "计算资源", "clickhouse": "计算资源", "elasticsearch": "计算资源", "graphcompute": "计算资源",
	"hdr": "计算资源", "swas": "计算资源", "ens": "计算资源",
	// 存储：对象存储、文件、块存储、表格存储等
	"oss": "存储", "nas": "存储", "disk": "存储", "ots": "存储", "pds": "存储",
	// 网络：CDN、负载均衡、VPC、EIP、流量包、直播点播流量等
	"cdn": "网络", "dcdn": "网络", "slb": "网络", "vpc": "网络", "eip": "网络", "cdt": "网络",
	"flowbag": "网络", "ossbag": "网络", "cdnflowbag": "网络", "live": "网络", "vod": "网络",
	"privatelink": "网络", "expressconnect": "网络", "smartag": "网络", "cbn": "网络", "nat": "网络",
	// 安全：WAF、安全防护、内容安全等
	"waf": "安全", "sas": "安全", "yundun": "安全", "ddos": "安全", "cfw": "安全", "cdi": "安全",
}

// BillItemResult 单产品金额，用于产品级明细与 top-N 展示。
type BillItemResult struct {
	ProductCode string
	Amount      float64
	Domain      string
}

// BillOverviewResult 账期总账与按产品金额（供工厂转换为 FetchAccountSummaryResponse）。
// TotalAmount     = PretaxAmount 代数和（资源消耗价值）
// CashTotalAmount = CashAmount  代数和（资源支付价值）
type BillOverviewResult struct {
	BillingCycle   string
	TotalAmount    float64            // 消耗价值（PretaxAmount）
	CashTotalAmount float64           // 支付价值（CashAmount）
	ByCategory     map[string]float64 // 按分类 PretaxAmount
	CashByCategory map[string]float64 // 按分类 CashAmount
	Items          []BillItemResult
	Currency       string
}

// Fetcher 阿里云 BssOpenApi 拉取，带退避重试与熔断。
type Fetcher struct {
	bssClient           *client.Client
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

// NewFetcherWithCredentials 显式 AK/SK 与 BSS Endpoint，供 YAML 项目级配置注入（不经环境变量）。[Ref: 03_Phase6 项目云账号]
func NewFetcherWithCredentials(ak, sk, endpoint string) (*Fetcher, bool) {
	ak = strings.TrimSpace(ak)
	sk = strings.TrimSpace(sk)
	if ak == "" || sk == "" {
		return nil, false
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
		slog.Warn("aliyun billing: NewFetcherWithCredentials failed", "error", err)
		return nil, false
	}
	return &Fetcher{bssClient: c}, true
}

// NewFetcherForEnv 按后缀从环境变量读取 AK/SK 创建 Fetcher。[Ref: 01_实践 §3.3(3a)]
// 后缀可为 POC/FAT/UAT/PROD，或与项目 YAML 一致的 environment_key（如 C66_UAT），避免多项目共用短名冲突。
// 变量名：ALIBABA_CLOUD_ACCESS_KEY_ID_<suffix>、ALIBABA_CLOUD_ACCESS_KEY_SECRET_<suffix>；可选 CLOUD_BILLING_ENDPOINT_<suffix>。
func NewFetcherForEnv(environment string) (*Fetcher, bool) {
	if environment == "" {
		return nil, false
	}
	ak := os.Getenv(envAccessKeyID + "_" + environment)
	sk := os.Getenv(envAccessKeySecret + "_" + environment)
	if ak == "" || sk == "" {
		return nil, false
	}
	endpoint := os.Getenv(envBillingEndpointAlt + "_" + environment)
	if endpoint == "" {
		endpoint = os.Getenv(envBillingEndpoint + "_" + environment)
	}
	if endpoint == "" {
		endpoint = os.Getenv(envBillingEndpointAlt)
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
		slog.Warn("aliyun billing: NewFetcherForEnv failed", "environment", environment, "error", err)
		return nil, false
	}
	return &Fetcher{bssClient: c}, true
}

// NewFetcherForEnvWithEndpoint 若 endpointOverride 非空则固定使用该 BSS 域名；否则与 NewFetcherForEnv 相同（读 CLOUD_BILLING_ENDPOINT_<suffix> 与中国站默认）。[Ref: 03_Phase6/01_FinOps 国际站 QueryAccountTransactions]
func NewFetcherForEnvWithEndpoint(environment string, endpointOverride string) (*Fetcher, bool) {
	environment = strings.TrimSpace(environment)
	if environment == "" {
		return nil, false
	}
	if strings.TrimSpace(endpointOverride) != "" {
		ak := os.Getenv(envAccessKeyID + "_" + environment)
		sk := os.Getenv(envAccessKeySecret + "_" + environment)
		if ak == "" || sk == "" {
			return nil, false
		}
		return NewFetcherWithCredentials(ak, sk, strings.TrimSpace(endpointOverride))
	}
	return NewFetcherForEnv(environment)
}

// ResolveBillingEndpointForEnv 返回与 NewFetcherForEnv 相同的 BSS Endpoint（仅用于本地诊断，不含密钥）。[Ref: 03_Phase6/01_FinOps]
func ResolveBillingEndpointForEnv(environment string) string {
	if environment == "" {
		return defaultBillingEndpoint
	}
	endpoint := os.Getenv(envBillingEndpointAlt + "_" + environment)
	if endpoint == "" {
		endpoint = os.Getenv(envBillingEndpoint + "_" + environment)
	}
	if endpoint == "" {
		endpoint = os.Getenv(envBillingEndpointAlt)
	}
	if endpoint == "" {
		endpoint = os.Getenv(envBillingEndpoint)
	}
	if endpoint == "" {
		endpoint = defaultBillingEndpoint
	}
	return endpoint
}

// BalanceDiagnostics QueryAccountBalance 原始字段与解析结果，供本地验证（无 AK）。[Ref: 03_Phase6/01_FinOps]
type BalanceDiagnostics struct {
	Success              *bool
	APIMessage           string
	Currency             string
	AvailableAmountRaw   string
	AvailableCashRaw     string
	CreditAmountRaw      string
	MybankCreditAmountRaw string
	ParsedByCode         float64
}

func strOrNil(p *string) string {
	if p == nil {
		return "<nil>"
	}
	return *p
}

// DiagnosticsQueryAccountBalance 同一次 QueryAccountBalance，返回 Body 关键字段（不经过熔断早退，便于对照控制台）。[Ref: 03_Phase6/01_FinOps]
func (f *Fetcher) DiagnosticsQueryAccountBalance(ctx context.Context) (*BalanceDiagnostics, error) {
	resp, err := f.bssClient.QueryAccountBalanceWithOptions(&service.RuntimeOptions{})
	if err != nil {
		return nil, err
	}
	out := &BalanceDiagnostics{}
	if resp == nil || resp.Body == nil {
		return out, fmt.Errorf("QueryAccountBalance: empty body")
	}
	out.Success = resp.Body.Success
	if resp.Body.Message != nil {
		out.APIMessage = *resp.Body.Message
	}
	if resp.Body.Data == nil {
		return out, nil
	}
	d := resp.Body.Data
	if d.Currency != nil && *d.Currency != "" {
		out.Currency = *d.Currency
	} else {
		out.Currency = "CNY"
	}
	out.AvailableAmountRaw = strOrNil(d.AvailableAmount)
	out.AvailableCashRaw = strOrNil(d.AvailableCashAmount)
	out.CreditAmountRaw = strOrNil(d.CreditAmount)
	out.MybankCreditAmountRaw = strOrNil(d.MybankCreditAmount)
	out.ParsedByCode = balanceFromQueryAccountBalanceData(d)
	return out, nil
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
		resp, err := f.queryBillOverviewMerged(ctx, billingCycle)
		if err == nil {
			f.mu.Lock()
			f.consecutiveFailures = 0
			f.mu.Unlock()
			slog.Info("aliyun billing: fetch ok", "billing_cycle", billingCycle, "duration_ms", time.Since(start).Milliseconds(), "total", resp.TotalAmount)
			// 当月无数据时尝试上月账期，便于新环境或当月未出账时仍有展示
			if resp.TotalAmount == 0 && len(resp.ByCategory) == 0 {
				prevCycle := prevMonthBillingCycle(billingCycle)
				if prevCycle != billingCycle {
					prev, err2 := f.queryBillOverviewMerged(ctx, prevCycle)
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

// queryBillOverviewSingle 拉取指定账期、可选订阅类型的单次 API 调用。[Ref: 阿里云 QueryBillOverview SubscriptionType]
func (f *Fetcher) queryBillOverviewSingle(ctx context.Context, billingCycle string, subscriptionType string) (*BillOverviewResult, error) {
	req := &client.QueryBillOverviewRequest{
		BillingCycle: tea.String(billingCycle),
	}
	if subscriptionType != "" {
		req.SubscriptionType = tea.String(subscriptionType)
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
	cashTotal := 0.0
	byCategory := make(map[string]float64)
	cashByCategory := make(map[string]float64)
	var items []BillItemResult
	if data.Items != nil && len(data.Items.Item) > 0 {
		for _, it := range data.Items.Item {
			// [Ref: 16_云账单动态对账与高可靠处理规范] 消耗统计口径：使用 PretaxAmount（税前应付金额）。
			// PretaxAmount：真实资源消耗价值（代金券/积分/现金支付均有值）。
			// CashAmount：实际现金支出（代金券账号为0，月度信用结算条目为大额负数），并列保存供支付视图使用。
			domain := "计算资源"
			codeStr := "OTHER"
			if it.ProductCode != nil && *it.ProductCode != "" {
				codeStr = strings.ToUpper(strings.TrimSpace(*it.ProductCode))
				if d, ok := productCodeToDomain[strings.ToLower(*it.ProductCode)]; ok {
					domain = d
				}
			} else if it.PipCode != nil && *it.PipCode != "" {
				codeStr = strings.ToUpper(strings.TrimSpace(*it.PipCode))
				if d, ok := productCodeToDomain[strings.ToLower(*it.PipCode)]; ok {
					domain = d
				}
			}
			// consumption: PretaxAmount（零值不参与汇总）
			if it.PretaxAmount != nil {
				amount := float64(*it.PretaxAmount)
				if amount != 0 {
					total += amount
					byCategory[domain] += amount
					items = append(items, BillItemResult{ProductCode: codeStr, Amount: amount, Domain: domain})
					if amount < 0 {
						itemType := ""
						if it.Item != nil {
							itemType = *it.Item
						}
						slog.Info("aliyun billing: negative PretaxAmount item (chargeback)",
							"product_code", codeStr, "pretax_amount", amount, "item_type", itemType, "billing_cycle", billingCycle)
					}
				}
			}
			// 现金支付：优先 PaymentAmount（控制台「现金支付」列），无则用 CashAmount，确保月/日源数据落库 [Ref: 16_ §3.3]
			cashAmt := 0.0
			if it.PaymentAmount != nil {
				cashAmt = float64(*it.PaymentAmount)
			}
			if cashAmt == 0 && it.CashAmount != nil {
				cashAmt = float64(*it.CashAmount)
			}
			if cashAmt != 0 {
				cashTotal += cashAmt
				cashByCategory[domain] += cashAmt
			}
		}
	}
	cycle := billingCycle
	if data.BillingCycle != nil && *data.BillingCycle != "" {
		cycle = *data.BillingCycle
	}
	return &BillOverviewResult{
		BillingCycle:    cycle,
		TotalAmount:     total,
		CashTotalAmount: cashTotal,
		ByCategory:      byCategory,
		CashByCategory:  cashByCategory,
		Items:           items,
		Currency:        "CNY",
	}, nil
}

// queryBillOverviewMerged 分别拉取预付费(Subscription)与后付费(PayAsYouGo)并合并，确保控制台「支付明细」与 Lighthouse 数据一致。[Ref: 用户反馈 2025-10/11 控制台有正数、Lighthouse 显示 $0；QueryBillOverview 不传 SubscriptionType 时部分场景 PaymentAmount 未返回]
func (f *Fetcher) queryBillOverviewMerged(ctx context.Context, billingCycle string) (*BillOverviewResult, error) {
	sub, err := f.queryBillOverviewSingle(ctx, billingCycle, "Subscription")
	if err != nil {
		return nil, err
	}
	payg, err := f.queryBillOverviewSingle(ctx, billingCycle, "PayAsYouGo")
	if err != nil {
		return nil, err
	}
	merged := &BillOverviewResult{
		BillingCycle:    billingCycle,
		TotalAmount:     sub.TotalAmount + payg.TotalAmount,
		CashTotalAmount: sub.CashTotalAmount + payg.CashTotalAmount,
		ByCategory:     make(map[string]float64),
		CashByCategory:  make(map[string]float64),
		Items:           append([]BillItemResult{}, sub.Items...),
		Currency:        "CNY",
	}
	for k, v := range sub.ByCategory {
		merged.ByCategory[k] += v
	}
	for k, v := range payg.ByCategory {
		merged.ByCategory[k] += v
	}
	for k, v := range sub.CashByCategory {
		merged.CashByCategory[k] += v
	}
	for k, v := range payg.CashByCategory {
		merged.CashByCategory[k] += v
	}
	for _, it := range payg.Items {
		merged.Items = append(merged.Items, it)
	}
	return merged, nil
}

// FetchBillOverviewByDay 按自然日拉取账单汇总（QueryAccountBill Granularity=DAILY，分页拉全）。[Ref: 01_设计 §拉取粒度与落表、15_]
// billingDate 格式 YYYY-MM-DD。返回与 FetchBillOverview 同结构的 BillOverviewResult，BillingCycle 字段设为 billingDate。
func (f *Fetcher) FetchBillOverviewByDay(ctx context.Context, billingDate string) (*BillOverviewResult, error) {
	f.mu.Lock()
	if time.Now().Before(f.circuitOpenUntil) {
		f.mu.Unlock()
		return nil, ErrCircuitOpen
	}
	f.mu.Unlock()

	// 阿里云 API：BillingCycle 为必填；日粒度时需同时传 BillingDate(YYYY-MM-DD) 与 BillingCycle(YYYY-MM)
	billingCycle := billingDate
	if len(billingDate) >= 7 {
		billingCycle = billingDate[:7]
	}
	pageSize := int32(300)
	pageNum := int32(1)
	var totalAmount float64
	byCategory := make(map[string]float64)
	var cashTotalAmount float64
	cashByCategory := make(map[string]float64)
	var allItems []BillItemResult
	for {
		req := &client.QueryAccountBillRequest{
			BillingCycle:     tea.String(billingCycle),
			BillingDate:      tea.String(billingDate),
			Granularity:      tea.String("DAILY"),
			IsGroupByProduct: tea.Bool(true),
			PageNum:          tea.Int32(pageNum),
			PageSize:         tea.Int32(pageSize),
		}
		resp, err := f.bssClient.QueryAccountBillWithOptions(req, &service.RuntimeOptions{})
		if err != nil {
			slog.Warn("aliyun billing: QueryAccountBill failed", "billing_date", billingDate, "page", pageNum, "error", err)
			return nil, err
		}
		if resp == nil || resp.Body == nil || resp.Body.Data == nil {
			break
		}
		data := resp.Body.Data
		if data.Items == nil || len(data.Items.Item) == 0 {
			break
		}
		for _, it := range data.Items.Item {
			// [Ref: 16_云账单动态对账与高可靠处理规范] 消耗统计口径：使用 PretaxAmount（税前应付金额 = 真实资源消耗价值）。
			// 代金券账号 CashAmount=0（导致日粒度全部为零），月度信用结算条目 CashAmount 为大额负数。
			// PretaxAmount 反映实际消耗，代金券支付时仍有值；零消耗条目自然排除。
			// 冲正/退款条目 PretaxAmount 为负，代数参与汇总（净消耗语义正确）。
			if it.PretaxAmount == nil {
				continue
			}
			amount := float64(*it.PretaxAmount)
			if amount == 0 {
				continue // 零消耗（纯信用结算或免费额度）
			}
			if amount < 0 {
				pipCode := ""
				if it.PipCode != nil {
					pipCode = *it.PipCode
				}
				bizType := ""
				if it.BizType != nil {
					bizType = *it.BizType
				}
				slog.Info("aliyun billing: negative PretaxAmount item included (chargeback/adjustment)",
					"billing_date", billingDate,
					"pip_code", pipCode,
					"biz_type", bizType,
					"pretax_amount", amount,
				)
			}
			totalAmount += amount
			codeStr := "OTHER"
			domain := "计算资源"
			if it.PipCode != nil && *it.PipCode != "" {
				codeStr = strings.ToUpper(strings.TrimSpace(*it.PipCode))
				if d, ok := productCodeToDomain[strings.ToLower(*it.PipCode)]; ok {
					domain = d
				}
			}
			byCategory[domain] += amount
			allItems = append(allItems, BillItemResult{ProductCode: codeStr, Amount: amount, Domain: domain})
			// [Ref: 16_ §3.3] 日粒度现金支付：优先 PaymentAmount（控制台「现金支付」列），无则用 CashAmount，确保日表落库
			cashAmt := 0.0
			if it.PaymentAmount != nil {
				cashAmt = float64(*it.PaymentAmount)
			}
			if cashAmt == 0 && it.CashAmount != nil {
				cashAmt = float64(*it.CashAmount)
			}
			if cashAmt != 0 {
				cashTotalAmount += cashAmt
				cashByCategory[domain] += cashAmt
			}
		}
		total := int32(0)
		if data.TotalCount != nil {
			total = *data.TotalCount
		}
		if int(pageNum)*int(pageSize) >= int(total) {
			break
		}
		pageNum++
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}
	}
	return &BillOverviewResult{
		BillingCycle:    billingDate,
		TotalAmount:    totalAmount,
		CashTotalAmount: cashTotalAmount,
		ByCategory:     byCategory,
		CashByCategory: cashByCategory,
		Items:          allItems,
		Currency:       "CNY",
	}, nil
}

// effectivePretaxPayableFromItem 对齐控制台「应付」：QueryAccountBill / QueryBillOverview 的 Data.Items.Item 中
//   目录总价 PretaxGrossAmount；优惠 InvoiceDiscount；优惠券抵扣 DeductedByCoupons（接口多为正数，须从目录链路中减去；另见 DeductedByCashCoupons）；
//   应付(税前) PretaxAmount 常与 PretaxGrossAmount − InvoiceDiscount − DeductedByCoupons − 抹零调账 一致。
// 本函数在能拆分时用 Gross−Discount−Coupons；与 PretaxAmount 近似相等则采用 PretaxAmount（避免与控制台单行重复）。[Ref: 04_采集 §5.4]
func effectivePretaxPayableFromItem(it *client.QueryAccountBillResponseBodyDataItemsItem) float64 {
	if it == nil {
		return 0
	}
	var pretax float64
	if it.PretaxAmount != nil {
		pretax = float64(*it.PretaxAmount)
	}
	if it.PretaxGrossAmount == nil || *it.PretaxGrossAmount == 0 {
		return pretax
	}
	hasDiscount := it.InvoiceDiscount != nil && *it.InvoiceDiscount != 0
	hasCoupon := (it.DeductedByCoupons != nil && *it.DeductedByCoupons != 0) ||
		(it.DeductedByCashCoupons != nil && *it.DeductedByCashCoupons != 0)
	if !hasDiscount && !hasCoupon {
		return pretax
	}
	gross := float64(*it.PretaxGrossAmount)
	deduct := 0.0
	if it.InvoiceDiscount != nil {
		deduct += float64(*it.InvoiceDiscount)
	}
	if it.DeductedByCoupons != nil {
		deduct += float64(*it.DeductedByCoupons)
	}
	if it.DeductedByCashCoupons != nil {
		deduct += float64(*it.DeductedByCashCoupons)
	}
	derived := gross - deduct
	if math.Abs(derived-pretax) <= 1e-4 {
		return pretax
	}
	return derived
}

// BillLineItemResult 行级流水条目（QueryAccountBill IsGroupByProduct=false）。
// [Ref: 16_云账单动态对账与高可靠处理规范 §四]
type BillLineItemResult struct {
	RecordID          string  // 阿里云 RecordID（若 API 返回）；否则为业务构造键
	BillingDate       string  // YYYY-MM-DD
	BillingCycle      string  // YYYY-MM
	ProductCode       string
	ProductName       string
	SubOrderID        string
	InstanceID        string
	BillingItem       string
	SubscriptionType  string
	CashAmount        float64 // 现金支付（含负数冲正）
	PretaxAmount      float64
	PretaxGrossAmount float64
	Category          string  // 四大类映射后的分类
}

// FetchLineItemsByDay 拉取指定日期的行级流水明细（QueryAccountBill IsGroupByProduct=false）。
// 含负数 CashAmount 冲正条目，调用方禁止过滤。
// [Ref: 16_云账单动态对账与高可靠处理规范 §四]
func (f *Fetcher) FetchLineItemsByDay(ctx context.Context, billingDate string) ([]BillLineItemResult, error) {
	f.mu.Lock()
	if time.Now().Before(f.circuitOpenUntil) {
		f.mu.Unlock()
		return nil, ErrCircuitOpen
	}
	f.mu.Unlock()

	billingCycle := billingDate
	if len(billingDate) >= 7 {
		billingCycle = billingDate[:7]
	}

	pageSize := int32(300)
	pageNum := int32(1)
	var allItems []BillLineItemResult
	seqMap := make(map[string]int) // 同键计数，用于合成唯一 RecordID

	for {
		req := &client.QueryAccountBillRequest{
			BillingCycle:     tea.String(billingCycle),
			BillingDate:      tea.String(billingDate),
			Granularity:      tea.String("DAILY"),
			IsGroupByProduct: tea.Bool(false), // 行级明细
			PageNum:          tea.Int32(pageNum),
			PageSize:         tea.Int32(pageSize),
		}
		resp, err := f.bssClient.QueryAccountBillWithOptions(req, &service.RuntimeOptions{})
		if err != nil {
			slog.Warn("aliyun billing: FetchLineItemsByDay failed", "billing_date", billingDate, "page", pageNum, "error", err)
			f.mu.Lock()
			f.consecutiveFailures++
			if f.consecutiveFailures >= circuitFailThreshold {
				f.circuitOpenUntil = time.Now().Add(circuitOpenDuration)
				f.mu.Unlock()
				return nil, err
			}
			f.mu.Unlock()
			return nil, err
		}
		f.mu.Lock()
		f.consecutiveFailures = 0
		f.mu.Unlock()

		if resp == nil || resp.Body == nil || resp.Body.Data == nil {
			break
		}
		data := resp.Body.Data
		if data.Items == nil || len(data.Items.Item) == 0 {
			break
		}
		for _, it := range data.Items.Item {
			// 现金支付：优先 PaymentAmount（控制台「现金支付」），无则用 CashAmount，与按日/月汇总一致
			cashAmount := 0.0
			if it.PaymentAmount != nil {
				cashAmount = float64(*it.PaymentAmount)
			}
			if cashAmount == 0 && it.CashAmount != nil {
				cashAmount = float64(*it.CashAmount)
			}
			pretaxAmount := effectivePretaxPayableFromItem(it)
			pretaxGrossAmount := 0.0
			if it.PretaxGrossAmount != nil {
				pretaxGrossAmount = float64(*it.PretaxGrossAmount)
			}
			// PipCode 日粒度产品码（IsGroupByProduct=false 时有效）
			productCode := ""
			if it.PipCode != nil {
				productCode = strings.ToLower(strings.TrimSpace(*it.PipCode))
			}
			// ProductName/SubscriptionType 仅在 IsGroupByProduct=true 时返回；false 时通常为空
			productName := ""
			if it.ProductName != nil {
				productName = *it.ProductName
			}
			subscriptionType := ""
			if it.SubscriptionType != nil {
				subscriptionType = *it.SubscriptionType
			}
			bizType := ""
			if it.BizType != nil {
				bizType = *it.BizType
			}
			// QueryAccountBill API 不返回 RecordID/SubOrderId/InstanceID；构造业务唯一键
			// 包含 billingDate + productCode + bizType + pageNum + 序号 确保唯一
			base := billingDate + "|" + billingCycle + "|" + productCode + "|" + bizType + "|" + subscriptionType
			seqMap[base]++
			recordID := fmt.Sprintf("syn_%x_%d", simpleHash(base), seqMap[base])

			category := "其他"
			if d, ok := productCodeToDomain[productCode]; ok {
				category = d
			} else if productCode != "" {
				category = "计算资源"
			}

			allItems = append(allItems, BillLineItemResult{
				RecordID:          recordID,
				BillingDate:       billingDate,
				BillingCycle:      billingCycle,
				ProductCode:       productCode,
				ProductName:       productName,
				BillingItem:       bizType,
				SubscriptionType:  subscriptionType,
				CashAmount:        cashAmount,
				PretaxAmount:      pretaxAmount,
				PretaxGrossAmount: pretaxGrossAmount,
				Category:          category,
			})
		}

		total := int32(0)
		if data.TotalCount != nil {
			total = *data.TotalCount
		}
		if int(pageNum)*int(pageSize) >= int(total) {
			break
		}
		pageNum++
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}
	}

	slog.Info("aliyun billing: FetchLineItemsByDay done", "billing_date", billingDate, "total_items", len(allItems))
	return allItems, nil
}

// FetchBSSTransactions 分页拉取 CreateTime 在 [start,end]（含边界，UTC）内的账户流水。[Ref: 03_Phase6/01_FinOps]
func (f *Fetcher) FetchBSSTransactions(ctx context.Context, start, end time.Time) ([]BSSAccountTransactionItem, error) {
	f.mu.Lock()
	if time.Now().Before(f.circuitOpenUntil) {
		f.mu.Unlock()
		return nil, ErrCircuitOpen
	}
	f.mu.Unlock()

	startS := start.UTC().Format("2006-01-02T15:04:05Z")
	endS := end.UTC().Format("2006-01-02T15:04:05Z")
	pageSize := int32(300)
	pageNum := int32(1)
	var out []BSSAccountTransactionItem
	for {
		req := &client.QueryAccountTransactionsRequest{
			CreateTimeStart: tea.String(startS),
			CreateTimeEnd:   tea.String(endS),
			PageNum:         tea.Int32(pageNum),
			PageSize:        tea.Int32(pageSize),
		}
		resp, err := f.bssClient.QueryAccountTransactionsWithOptions(req, &service.RuntimeOptions{})
		if err != nil {
			slog.Warn("aliyun billing: QueryAccountTransactions failed", "page", pageNum, "error", err)
			return nil, err
		}
		if resp == nil || resp.Body == nil {
			break
		}
		if resp.Body.Success != nil && !*resp.Body.Success {
			msg := ""
			if resp.Body.Message != nil {
				msg = *resp.Body.Message
			}
			return nil, fmt.Errorf("QueryAccountTransactions: %s", msg)
		}
		data := resp.Body.Data
		if data == nil || data.AccountTransactionsList == nil || len(data.AccountTransactionsList.AccountTransactionsList) == 0 {
			break
		}
		items := data.AccountTransactionsList.AccountTransactionsList
		for _, it := range items {
			txNum := derefStr(it.TransactionNumber)
			if txNum == "" {
				txNum = fmt.Sprintf("bss_syn_%x", simpleHash(derefStr(it.TransactionTime)+derefStr(it.Amount)+derefStr(it.RecordID)))
			}
			tm, err := parseAliyunTransactionTime(derefStr(it.TransactionTime))
			if err != nil {
				slog.Warn("aliyun billing: skip transaction with bad time", "tx", txNum, "error", err)
				continue
			}
			out = append(out, BSSAccountTransactionItem{
				TransactionNumber: txNum,
				TransactionTime:   tm,
				Amount:            parseAliyunFloat(derefStr(it.Amount)),
				TransactionType:   derefStr(it.TransactionType),
				TransactionFlow:   derefStr(it.TransactionFlow),
				RecordID:          derefStr(it.RecordID),
				BillingCycle:      derefStr(it.BillingCycle),
				Currency:          "CNY",
				TransactionChannel: derefStr(it.TransactionChannel),
				FundType:           derefStr(it.FundType),
				Remarks:            derefStr(it.Remarks),
			})
		}
		total := int32(0)
		if data.TotalCount != nil {
			total = *data.TotalCount
		}
		if int(pageNum)*int(pageSize) >= int(total) {
			break
		}
		pageNum++
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}
	}
	slog.Info("aliyun billing: FetchBSSTransactions done", "count", len(out), "start", startS, "end", endS)
	return out, nil
}

// BSSAccountTransactionItem 与 cloudbilling.BSSTransactionItem 字段对齐，避免 aliyun 依赖 cloudbilling。
type BSSAccountTransactionItem struct {
	TransactionNumber string
	TransactionTime   time.Time
	Amount            float64
	TransactionType   string
	TransactionFlow   string
	RecordID          string
	BillingCycle      string
	Currency          string
	TransactionChannel string
	FundType           string
	Remarks            string
}

func derefStr(p *string) string {
	if p == nil {
		return ""
	}
	return strings.TrimSpace(*p)
}

func parseAliyunFloat(s string) float64 {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	// 国际站 BSS 部分字段带千分位逗号（如 "7,245.65"），ParseFloat 会失败。[Ref: 03_Phase6/01_FinOps 本地验证]
	s = strings.ReplaceAll(s, ",", "")
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0
	}
	return v
}

func parseAliyunTransactionTime(s string) (time.Time, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, fmt.Errorf("empty transaction time")
	}
	layouts := []string{
		time.RFC3339,
		"2006-01-02T15:04:05Z",
		"2006-01-02T15:04:05",
		"2006-01-02 15:04:05",
		"2006-01-02",
	}
	for _, l := range layouts {
		if t, err := time.Parse(l, s); err == nil {
			return t.UTC(), nil
		}
	}
	return time.Time{}, fmt.Errorf("parse transaction time: %q", s)
}

// FetchCallingAccountID 通过 QueryAccountBill（MONTHLY、首页）读取 Data.AccountID，与凭证对应主账号一致。[Ref: 03_Phase6/01_FinOps]
func (f *Fetcher) FetchCallingAccountID(ctx context.Context, billingCycle string) (string, error) {
	f.mu.Lock()
	if time.Now().Before(f.circuitOpenUntil) {
		f.mu.Unlock()
		return "", ErrCircuitOpen
	}
	f.mu.Unlock()
	if strings.TrimSpace(billingCycle) == "" {
		billingCycle = time.Now().UTC().Format("2006-01")
	}
	req := &client.QueryAccountBillRequest{
		BillingCycle:     tea.String(billingCycle),
		Granularity:      tea.String("MONTHLY"),
		IsGroupByProduct: tea.Bool(true),
		PageNum:          tea.Int32(1),
		PageSize:         tea.Int32(1),
	}
	resp, err := f.bssClient.QueryAccountBillWithOptions(req, &service.RuntimeOptions{})
	if err != nil {
		return "", err
	}
	if resp == nil || resp.Body == nil || resp.Body.Data == nil {
		return "", nil
	}
	if resp.Body.Success != nil && !*resp.Body.Success {
		msg := ""
		if resp.Body.Message != nil {
			msg = *resp.Body.Message
		}
		return "", fmt.Errorf("QueryAccountBill: %s", msg)
	}
	d := resp.Body.Data
	if d.AccountID != nil {
		return strings.TrimSpace(*d.AccountID), nil
	}
	return "", nil
}

// FetchAccountBalanceSnapshot 拉取 QueryAccountBalance 可用余额。[Ref: 03_Phase6/01_FinOps]
func (f *Fetcher) FetchAccountBalanceSnapshot(ctx context.Context) (available float64, currency string, err error) {
	f.mu.Lock()
	if time.Now().Before(f.circuitOpenUntil) {
		f.mu.Unlock()
		return 0, "", ErrCircuitOpen
	}
	f.mu.Unlock()

	resp, err := f.bssClient.QueryAccountBalanceWithOptions(&service.RuntimeOptions{})
	if err != nil {
		return 0, "", err
	}
	if resp == nil || resp.Body == nil {
		return 0, "", fmt.Errorf("QueryAccountBalance: empty body")
	}
	if resp.Body.Success != nil && !*resp.Body.Success {
		msg := ""
		if resp.Body.Message != nil {
			msg = *resp.Body.Message
		}
		return 0, "", fmt.Errorf("QueryAccountBalance: %s", msg)
	}
	if resp.Body.Data == nil {
		return 0, "", nil
	}
	d := resp.Body.Data
	cur := "CNY"
	if d.Currency != nil && *d.Currency != "" {
		cur = *d.Currency
	}
	avail := balanceFromQueryAccountBalanceData(d)
	return avail, cur, nil
}

// balanceFromQueryAccountBalanceData 可用余额：优先 AvailableAmount；若为 0 再叠加现金/信控（国际站等场景常见仅 Credit/现金列有值）。[Ref: 03_Phase6/01_FinOps]
func balanceFromQueryAccountBalanceData(d *client.QueryAccountBalanceResponseBodyData) float64 {
	if d == nil {
		return 0
	}
	var avail float64
	if d.AvailableAmount != nil {
		avail = parseAliyunFloat(*d.AvailableAmount)
	}
	if avail > 1e-9 {
		return avail
	}
	if d.AvailableCashAmount != nil {
		avail += parseAliyunFloat(*d.AvailableCashAmount)
	}
	if d.CreditAmount != nil {
		avail += parseAliyunFloat(*d.CreditAmount)
	}
	if d.MybankCreditAmount != nil {
		avail += parseAliyunFloat(*d.MybankCreditAmount)
	}
	return avail
}

// sumOutstandingQueryAccountBillMonthly 分页汇总 QueryAccountBill MONTHLY 的 OutstandingAmount。[Ref: 03_Phase6/01_FinOps]
func (f *Fetcher) sumOutstandingQueryAccountBillMonthly(ctx context.Context, billingCycle string, byProduct bool) (float64, error) {
	pageNum := int32(1)
	pageSize := int32(300)
	var sum float64
	for {
		req := &client.QueryAccountBillRequest{
			BillingCycle:     tea.String(billingCycle),
			Granularity:      tea.String("MONTHLY"),
			IsGroupByProduct: tea.Bool(byProduct),
			PageNum:          tea.Int32(pageNum),
			PageSize:         tea.Int32(pageSize),
		}
		resp, err := f.bssClient.QueryAccountBillWithOptions(req, &service.RuntimeOptions{})
		if err != nil {
			return 0, err
		}
		if resp == nil || resp.Body == nil || resp.Body.Data == nil {
			break
		}
		data := resp.Body.Data
		if data.Items == nil || len(data.Items.Item) == 0 {
			break
		}
		for _, it := range data.Items.Item {
			if it.OutstandingAmount != nil {
				sum += float64(*it.OutstandingAmount)
			}
		}
		total := int32(0)
		if data.TotalCount != nil {
			total = *data.TotalCount
		}
		if int(pageNum)*int(pageSize) >= int(total) {
			break
		}
		pageNum++
		select {
		case <-ctx.Done():
			return 0, ctx.Err()
		default:
		}
	}
	return sum, nil
}

// sumOutstandingFromBillOverview 从 QueryBillOverview 各订阅类型行汇总 OutstandingAmount（AccountBill 无应付字段时的兜底）。[Ref: 03_Phase6/01_FinOps]
func (f *Fetcher) sumOutstandingFromBillOverview(ctx context.Context, billingCycle string) (float64, error) {
	var total float64
	for _, st := range []string{"Subscription", "PayAsYouGo"} {
		req := &client.QueryBillOverviewRequest{
			BillingCycle:     tea.String(billingCycle),
			SubscriptionType: tea.String(st),
		}
		resp, err := f.bssClient.QueryBillOverviewWithOptions(req, &service.RuntimeOptions{})
		if err != nil {
			return 0, err
		}
		if resp == nil || resp.Body == nil || resp.Body.Data == nil || resp.Body.Data.Items == nil {
			continue
		}
		for _, it := range resp.Body.Data.Items.Item {
			if it.OutstandingAmount != nil {
				total += float64(*it.OutstandingAmount)
			}
		}
	}
	return total, nil
}

// SumOutstandingMonthly 先按产品汇总 QueryAccountBill；若为 0 再试不按产品分组；仍为 0 则 QueryBillOverview 行级应付兜底。[Ref: 03_Phase6/01_FinOps]
func (f *Fetcher) SumOutstandingMonthly(ctx context.Context, billingCycle string) (float64, error) {
	f.mu.Lock()
	if time.Now().Before(f.circuitOpenUntil) {
		f.mu.Unlock()
		return 0, ErrCircuitOpen
	}
	f.mu.Unlock()

	if strings.TrimSpace(billingCycle) == "" {
		return 0, nil
	}
	grouped, errG := f.sumOutstandingQueryAccountBillMonthly(ctx, billingCycle, true)
	if errG == nil && grouped > 1e-9 {
		return grouped, nil
	}
	ungrouped, errU := f.sumOutstandingQueryAccountBillMonthly(ctx, billingCycle, false)
	if errU == nil && ungrouped > 1e-9 {
		return ungrouped, nil
	}
	ov, errO := f.sumOutstandingFromBillOverview(ctx, billingCycle)
	if errO == nil && ov > 1e-9 {
		return ov, nil
	}
	if errU == nil {
		return ungrouped, nil
	}
	if errG == nil {
		return grouped, nil
	}
	if errO == nil {
		return ov, nil
	}
	if errG != nil {
		return 0, errG
	}
	if errU != nil {
		return 0, errU
	}
	return 0, errO
}

// FetchQueryAccountBillMonthlyItems 分页拉取 QueryAccountBill **MONTHLY** 全部明细行，供抹零/调账与控制台核对。[Ref: QueryAccountBill 临时验证]
func (f *Fetcher) FetchQueryAccountBillMonthlyItems(ctx context.Context, billingCycle string, byProduct bool) ([]*client.QueryAccountBillResponseBodyDataItemsItem, error) {
	f.mu.Lock()
	if time.Now().Before(f.circuitOpenUntil) {
		f.mu.Unlock()
		return nil, ErrCircuitOpen
	}
	f.mu.Unlock()
	if strings.TrimSpace(billingCycle) == "" {
		return nil, fmt.Errorf("billingCycle required")
	}
	var out []*client.QueryAccountBillResponseBodyDataItemsItem
	pageNum := int32(1)
	pageSize := int32(300)
	for {
		req := &client.QueryAccountBillRequest{
			BillingCycle:     tea.String(billingCycle),
			Granularity:      tea.String("MONTHLY"),
			IsGroupByProduct: tea.Bool(byProduct),
			PageNum:          tea.Int32(pageNum),
			PageSize:         tea.Int32(pageSize),
		}
		resp, err := f.bssClient.QueryAccountBillWithOptions(req, &service.RuntimeOptions{})
		if err != nil {
			return nil, err
		}
		if resp == nil || resp.Body == nil {
			break
		}
		if resp.Body.Success != nil && !*resp.Body.Success {
			msg := ""
			if resp.Body.Message != nil {
				msg = *resp.Body.Message
			}
			return nil, fmt.Errorf("QueryAccountBill: %s", msg)
		}
		if resp.Body.Data == nil {
			break
		}
		data := resp.Body.Data
		if data.Items == nil || len(data.Items.Item) == 0 {
			break
		}
		out = append(out, data.Items.Item...)
		total := int32(0)
		if data.TotalCount != nil {
			total = *data.TotalCount
		}
		if int(pageNum)*int(pageSize) >= int(total) {
			break
		}
		pageNum++
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}
	}
	return out, nil
}

// SumCouponDeductionForBillingCycle 汇总账期内「优惠券抵扣」合计（DeductedByCoupons+DeductedByCashCoupons）。[Ref: 04_采集 §5.4 优惠券]
func (f *Fetcher) SumCouponDeductionForBillingCycle(ctx context.Context, billingCycle string) (float64, error) {
	c, cc, err := f.SumCouponDeductionPartsForBillingCycle(ctx, billingCycle)
	if err != nil {
		return 0, err
	}
	return c + cc, nil
}

// SumCouponDeductionPartsForBillingCycle 分项汇总：先 MONTHLY；合计近似 0 时再按日 DAILY 累加。[Ref: 04_采集 §5.4 优惠券]
func (f *Fetcher) SumCouponDeductionPartsForBillingCycle(ctx context.Context, billingCycle string) (coupon float64, cashCoupon float64, err error) {
	f.mu.Lock()
	if time.Now().Before(f.circuitOpenUntil) {
		f.mu.Unlock()
		return 0, 0, ErrCircuitOpen
	}
	f.mu.Unlock()
	items, err := f.FetchQueryAccountBillMonthlyItems(ctx, billingCycle, false)
	if err != nil {
		return 0, 0, err
	}
	c, cc := sumCouponFieldsOnAccountBillItemsSplit(items)
	if c+cc > 1e-6 {
		slog.Info("aliyun billing: coupon deduction from MONTHLY QueryAccountBill", "billing_cycle", billingCycle, "deducted_by_coupons", c, "deducted_by_cash_coupons", cc)
		return c, cc, nil
	}
	dc, dcc, err := f.sumCouponDeductionDailyWalkSplit(ctx, billingCycle)
	if err != nil {
		return 0, 0, err
	}
	slog.Info("aliyun billing: coupon deduction from DAILY QueryAccountBill (fallback)", "billing_cycle", billingCycle, "deducted_by_coupons", dc, "deducted_by_cash_coupons", dcc)
	return dc, dcc, nil
}

func sumCouponFieldsOnAccountBillItemsSplit(items []*client.QueryAccountBillResponseBodyDataItemsItem) (c float64, cc float64) {
	for _, it := range items {
		if it == nil {
			continue
		}
		if it.DeductedByCoupons != nil {
			c += float64(*it.DeductedByCoupons)
		}
		if it.DeductedByCashCoupons != nil {
			cc += float64(*it.DeductedByCashCoupons)
		}
	}
	return c, cc
}

func sumCouponFieldsOnAccountBillItems(items []*client.QueryAccountBillResponseBodyDataItemsItem) float64 {
	c, cc := sumCouponFieldsOnAccountBillItemsSplit(items)
	return c + cc
}

func (f *Fetcher) sumCouponDeductionDailyWalkSplit(ctx context.Context, billingCycle string) (float64, float64, error) {
	billingCycle = strings.TrimSpace(billingCycle)
	start, err := time.ParseInLocation("2006-01", billingCycle, time.UTC)
	if err != nil {
		return 0, 0, fmt.Errorf("billingCycle: %w", err)
	}
	end := start.AddDate(0, 1, -1)
	var totC, totCC float64
	for d := start; !d.After(end); d = d.AddDate(0, 0, 1) {
		ds := d.Format("2006-01-02")
		partC, partCC, err := f.sumCouponDeductionDailyOneDateSplit(ctx, ds, billingCycle)
		if err != nil {
			return 0, 0, err
		}
		totC += partC
		totCC += partCC
		select {
		case <-ctx.Done():
			return 0, 0, ctx.Err()
		default:
		}
	}
	return totC, totCC, nil
}

func (f *Fetcher) sumCouponDeductionDailyOneDateSplit(ctx context.Context, billingDate, billingCycle string) (float64, float64, error) {
	pageNum := int32(1)
	pageSize := int32(300)
	var sumC, sumCC float64
	for {
		req := &client.QueryAccountBillRequest{
			BillingCycle:     tea.String(billingCycle),
			BillingDate:      tea.String(billingDate),
			Granularity:      tea.String("DAILY"),
			IsGroupByProduct: tea.Bool(false),
			PageNum:          tea.Int32(pageNum),
			PageSize:         tea.Int32(pageSize),
		}
		resp, err := f.bssClient.QueryAccountBillWithOptions(req, &service.RuntimeOptions{})
		if err != nil {
			return 0, 0, err
		}
		if resp == nil || resp.Body == nil {
			break
		}
		if resp.Body.Success != nil && !*resp.Body.Success {
			msg := ""
			if resp.Body.Message != nil {
				msg = *resp.Body.Message
			}
			return 0, 0, fmt.Errorf("QueryAccountBill DAILY: %s", msg)
		}
		if resp.Body.Data == nil {
			break
		}
		data := resp.Body.Data
		if data.Items == nil || len(data.Items.Item) == 0 {
			break
		}
		for _, it := range data.Items.Item {
			if it.DeductedByCoupons != nil {
				sumC += float64(*it.DeductedByCoupons)
			}
			if it.DeductedByCashCoupons != nil {
				sumCC += float64(*it.DeductedByCashCoupons)
			}
		}
		total := int32(0)
		if data.TotalCount != nil {
			total = *data.TotalCount
		}
		if int(pageNum)*int(pageSize) >= int(total) {
			break
		}
		pageNum++
		select {
		case <-ctx.Done():
			return 0, 0, ctx.Err()
		default:
		}
	}
	return sumC, sumCC, nil
}

// simpleHash 用于合成 RecordID 的简单哈希（非安全，仅用于唯一性辅助）。
func simpleHash(s string) uint32 {
	h := uint32(2166136261)
	for i := 0; i < len(s); i++ {
		h ^= uint32(s[i])
		h *= 16777619
	}
	return h
}

// ErrCircuitOpen 熔断打开时返回。
var ErrCircuitOpen = errCircuitOpen{}

type errCircuitOpen struct{}

func (errCircuitOpen) Error() string { return "cloud billing circuit open" }
