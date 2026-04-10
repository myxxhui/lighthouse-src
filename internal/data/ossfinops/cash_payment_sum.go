// 从账单 CSV 累加「现金支付金额 / Cash Payment」列，供临时对账程序使用。[Ref: 03_Phase6/01_FinOps]
package ossfinops

import (
	"bufio"
	"context"
	"encoding/csv"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/aliyun/aliyun-oss-go-sdk/oss"
)

var cashPaymentColCandidates = []string{
	"现金支付金额", "现金支付(元)", "现金支付", "现金账户支付金额",
	"cashpayment", "cash payment", "cashpaymentamount",
	"cash_amount", "cashamount",
}

// findCashPaymentCol 匹配「现金支付」列；不用 coupon 相关别名，避免与券列混淆。
func findCashPaymentCol(idx map[string]int) int {
	return findCol(idx, cashPaymentColCandidates)
}

// SumCashPaymentColumnInCSV 累加 CSV 中现金支付列；billingCycleFilter 非空时仅统计该行归属账期（与 parseCSVToFacts 一致）。
func SumCashPaymentColumnInCSV(r io.Reader, sourceObject, billingCycleFilter, cycleHint string) (sum float64, colFound bool, err error) {
	br := bufio.NewReader(r)
	b, err := br.Peek(3)
	if err != nil && err != io.EOF {
		return 0, false, err
	}
	if len(b) >= 3 && b[0] == 0xEF && b[1] == 0xBB && b[2] == 0xBF {
		_, _ = br.Discard(3)
	}
	cr := csv.NewReader(br)
	cr.LazyQuotes = true
	cr.FieldsPerRecord = -1

	var header []string
	for {
		h, err := cr.Read()
		if err != nil {
			return 0, false, fmt.Errorf("csv header: %w", err)
		}
		if !rowEmpty(h) {
			header = h
			break
		}
	}
	norm := normalizeHeader(header)
	idx := map[string]int{}
	for i, h := range norm {
		idx[h] = i
	}
	iBillDate := findCol(idx, []string{"账单日期", "billingdate", "billing date"})
	iConsume := findCol(idx, []string{"消费时间", "consumptiontime", "usage time"})
	iSvcStart := findCol(idx, []string{"服务开始时间", "servicestarttime", "service start"})
	iCycle := findCol(idx, []string{"账单月份", "账期", "billingcycle", "billing cycle"})
	iCash := findCashPaymentCol(idx)
	if iCash < 0 {
		return 0, false, nil
	}
	colFound = true

	for {
		rec, err := cr.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return sum, colFound, err
		}
		if len(rec) == 0 || rowEmpty(rec) {
			continue
		}
		dateStr := firstNonEmptyField(rec, iBillDate, iConsume, iSvcStart)
		ud, err := parseDateFlexible(dateStr)
		if err != nil {
			continue
		}
		rowCycle := cycleHint
		if iCycle >= 0 {
			if c := strings.TrimSpace(get(rec, iCycle)); c != "" {
				rowCycle = normalizeBillingCycle(c)
			}
		}
		if rowCycle == "" {
			rowCycle = ud.Format("2006-01")
		}
		if len(rowCycle) > 7 {
			rowCycle = rowCycle[:7]
		}
		if billingCycleFilter != "" && rowCycle != billingCycleFilter {
			continue
		}
		cs := strings.TrimSpace(get(rec, iCash))
		if cs == "" {
			continue
		}
		cv, err := parseAmount(cs)
		if err != nil {
			slog.Debug("ossfinops: skip bad cash payment cell", "source", sourceObject, "val", cs, "err", err)
			continue
		}
		sum += cv
	}
	return sum, colFound, nil
}

// SumCashPaymentFromOSS 列举 Prefix 下 CSV（与 LoadBillingCSVsFromOSS 同序），按账期过滤行后累加现金支付列。[Ref: 04_采集 §七]
func SumCashPaymentFromOSS(ctx context.Context, cfg Config, billingCycle string) (float64, error) {
	if cfg.Bucket == "" || cfg.AccessKey == "" || cfg.SecretKey == "" {
		return 0, fmt.Errorf("ossfinops: bucket/access/secret required for cash sum")
	}
	endpoint := cfg.Endpoint
	if endpoint == "" {
		endpoint = "https://oss-ap-southeast-1.aliyuncs.com"
	}
	if !strings.HasPrefix(endpoint, "http") {
		endpoint = "https://" + endpoint
	}
	prefix := cfg.Prefix
	if prefix != "" && !strings.HasSuffix(prefix, "/") {
		prefix += "/"
	}
	now := cfg.Now
	if now.IsZero() {
		now = time.Now().UTC()
	}
	curCycle := now.Format("2006-01")
	billingCycle = strings.TrimSpace(billingCycle)

	cli, err := oss.New(endpoint, cfg.AccessKey, cfg.SecretKey)
	if err != nil {
		return 0, fmt.Errorf("oss.New: %w", err)
	}
	bucket, err := cli.Bucket(cfg.Bucket)
	if err != nil {
		return 0, fmt.Errorf("Bucket: %w", err)
	}

	all, err := ListOSSBillingObjects(bucket, prefix)
	if err != nil {
		return 0, err
	}
	work := all
	if !cfg.IncrementalSince.IsZero() {
		work = make([]oss.ObjectProperties, 0, len(all))
		for _, obj := range all {
			if obj.LastModified.After(cfg.IncrementalSince) {
				work = append(work, obj)
			}
		}
	}
	sortObjectsForIngestion(work)

	var total float64
	for _, obj := range work {
		key := obj.Key
		if strings.HasSuffix(strings.ToLower(key), ".zip") {
			continue
		}
		cycleHint := GuessBillingCycleFromKey(key)
		if cfg.SyncMode == "current_month" && cycleHint != "" && cycleHint != curCycle {
			continue
		}
		rc, err := getObjectWithRetry(ctx, bucket, key)
		if err != nil {
			return total, fmt.Errorf("GetObject %s: %w", key, err)
		}
		sum, found, err := SumCashPaymentColumnInCSV(rc, key, billingCycle, cycleHint)
		_ = rc.Close()
		if err != nil {
			return total, fmt.Errorf("sum cash %s: %w", key, err)
		}
		if found {
			total += sum
			slog.Debug("ossfinops: cash payment column partial sum", "key", key, "billing_cycle", billingCycle, "partial", sum)
		}
	}
	return total, nil
}

// ConfigFromEnv 从环境变量组装 OSS 配置（与 ETL 全局 OSS 变量一致）；无桶或未配置 AK 时 ok=false。
// credentialEnv 如 C66_POC，用于 EnvForFinOps 与 OSS_BILLING_BUCKET_<ENV> 等后缀变量。
func ConfigFromEnv(credentialEnv string) (cfg Config, ok bool) {
	bucket := strings.TrimSpace(os.Getenv("OSS_BILLING_BUCKET"))
	if bucket == "" && credentialEnv != "" {
		bucket = strings.TrimSpace(os.Getenv("OSS_BILLING_BUCKET_" + strings.TrimSpace(credentialEnv)))
	}
	if bucket == "" {
		return Config{}, false
	}
	prefix := strings.TrimSpace(os.Getenv("OSS_BILLING_PREFIX"))
	if prefix == "" && credentialEnv != "" {
		prefix = strings.TrimSpace(os.Getenv("OSS_BILLING_PREFIX_" + strings.TrimSpace(credentialEnv)))
	}
	endpoint := strings.TrimSpace(os.Getenv("OSS_ENDPOINT"))
	if endpoint == "" && credentialEnv != "" {
		endpoint = strings.TrimSpace(os.Getenv("OSS_ENDPOINT_" + strings.TrimSpace(credentialEnv)))
	}
	ce := strings.TrimSpace(credentialEnv)
	if ce == "" {
		ce = strings.TrimSpace(os.Getenv("OSS_BILLING_CREDENTIAL_ENV"))
	}
	ak, sk := EnvForFinOps(ce)
	if ak == "" || sk == "" {
		return Config{}, false
	}
	syncMode := strings.TrimSpace(os.Getenv("OSS_SYNC_MODE"))
	if syncMode == "" {
		syncMode = "all"
	}
	return Config{
		Endpoint:  endpoint,
		AccessKey: ak,
		SecretKey: sk,
		Bucket:    bucket,
		Prefix:    prefix,
		SyncMode:  syncMode,
		Now:       time.Now().UTC(),
	}, true
}
