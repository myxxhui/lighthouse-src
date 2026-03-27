// Package ossfinops 从阿里云 OSS 流式读取账单 CSV 并写入 finops_billing_fact。[Ref: Phase6 OLAP 落地]
package ossfinops

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/aliyun/aliyun-oss-go-sdk/oss"
	"github.com/myxxhui/lighthouse-src/internal/data/postgres"
)

// Config OSS 拉取与同步策略。
type Config struct {
	Endpoint  string // 如 https://oss-ap-southeast-1.aliyuncs.com
	AccessKey string
	SecretKey string
	Bucket    string
	Prefix    string // 如 billing-data/
	AccountID string // Lighthouse 环境账号键，如 UAT
	SyncMode  string // all | current_month（仅处理文件名含当月账期）
	Now       time.Time
	// IncrementalSince 非零时仅处理 LastModified 晚于该时间的对象（配合 DB 水位与 OSS_INCREMENTAL_SYNC）。[Ref: 04_采集 §七]
	IncrementalSince time.Time
}

// FactWriter 由 postgres.Repository 实现。
type FactWriter interface {
	DeleteFinOpsBillingFactsByBillingCycle(ctx context.Context, billingCycle, accountID string) error
	BulkInsertFinOpsBillingFacts(ctx context.Context, rows []postgres.FinOpsBillingFactRow) error
	ReplaceFinOpsBillingCycleWithFacts(ctx context.Context, billingCycle, accountID string, rows []postgres.FinOpsBillingFactRow) error
}

// LoadBillingCSVsFromOSS 列举 Prefix 下 .csv，按 LastModified **升序**处理（滚动快照后者覆盖前者）；**文件名含 `_YYYY-MM.csv` 关账全量**时单事务 DELETE 账期+写入以消除幽灵行。[Ref: 04_采集 §六]
// 返回值 maxObjectLastModified 为本轮参与排序的工作集中对象 LastModified 的最大值（用于 OSS 增量水位）；无对象时为零值。[Ref: 04_采集 §七]
func LoadBillingCSVsFromOSS(ctx context.Context, db FactWriter, cfg Config) (maxObjectLastModified time.Time, err error) {
	if cfg.Bucket == "" || cfg.AccessKey == "" || cfg.SecretKey == "" {
		return time.Time{}, fmt.Errorf("ossfinops: bucket/access/secret required")
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

	cli, err := oss.New(endpoint, cfg.AccessKey, cfg.SecretKey)
	if err != nil {
		return time.Time{}, fmt.Errorf("oss.New: %w", err)
	}
	bucket, err := cli.Bucket(cfg.Bucket)
	if err != nil {
		return time.Time{}, fmt.Errorf("Bucket: %w", err)
	}

	var all []oss.ObjectProperties
	marker := ""
	for {
		lsRes, err := bucket.ListObjects(oss.Prefix(prefix), oss.Marker(marker), oss.MaxKeys(200))
		if err != nil {
			return time.Time{}, fmt.Errorf("ListObjects: %w", err)
		}
		for _, obj := range lsRes.Objects {
			key := obj.Key
			if !strings.HasSuffix(strings.ToLower(key), ".csv") {
				continue
			}
			if obj.Size == 0 {
				slog.Warn("ossfinops: skip zero-byte csv object", "key", key)
				continue
			}
			all = append(all, obj)
		}
		if !lsRes.IsTruncated {
			break
		}
		marker = lsRes.NextMarker
		if marker == "" {
			break
		}
	}
	work := all
	if !cfg.IncrementalSince.IsZero() {
		work = make([]oss.ObjectProperties, 0, len(all))
		for _, obj := range all {
			if obj.LastModified.After(cfg.IncrementalSince) {
				work = append(work, obj)
			}
		}
		if len(work) < len(all) {
			slog.Info("ossfinops: incremental filter", "total", len(all), "after_since", len(work), "since", cfg.IncrementalSince.UTC().Format(time.RFC3339))
		}
	}
	sortObjectsForIngestion(work)
	var maxLM time.Time
	for _, obj := range work {
		if obj.LastModified.After(maxLM) {
			maxLM = obj.LastModified
		}
	}
	for _, obj := range work {
		key := obj.Key
		cycle := guessBillingCycleFromKey(key)
		if cfg.SyncMode == "current_month" && cycle != "" && cycle != curCycle {
			slog.Info("ossfinops: skip non-current month object", "key", key, "cycle", cycle, "want", curCycle)
			continue
		}
		closed := isClosedMonthCSVFilename(baseName(key))
		if err := ingestOneObject(ctx, db, bucket, key, cfg.AccountID, cycle, closed); err != nil {
			return time.Time{}, fmt.Errorf("ingest %s: %w", key, err)
		}
	}
	return maxLM, nil
}

// sortObjectsForIngestion 先按 LastModified 升序；同毫秒时按文件名内导出时间戳（如 -20260326112001_）升序，再按 key，保证多文件当月时较新导出后处理。[Ref: 04_采集 §七]
func sortObjectsForIngestion(all []oss.ObjectProperties) {
	sort.Slice(all, func(i, j int) bool {
		ai, aj := all[i], all[j]
		if !ai.LastModified.Equal(aj.LastModified) {
			return ai.LastModified.Before(aj.LastModified)
		}
		ti := exportTimestampFromConsumeDetailName(baseName(ai.Key))
		tj := exportTimestampFromConsumeDetailName(baseName(aj.Key))
		if ti != tj {
			return ti < tj
		}
		return ai.Key < aj.Key
	})
}

var reExportStampBeforeConsumeDetail = regexp.MustCompile(`(?i)-(\d{14})_consumedetail`)

// exportTimestampFromConsumeDetailName 从 consumeDetail 文件名提取导出时间戳（14 位），缺失时返回空串（排序退化为 key）。[Ref: 04_采集 §七]
func exportTimestampFromConsumeDetailName(base string) string {
	m := reExportStampBeforeConsumeDetail.FindStringSubmatch(base)
	if len(m) == 2 {
		return m[1]
	}
	return ""
}

func baseName(key string) string {
	if i := strings.LastIndex(key, "/"); i >= 0 {
		return key[i+1:]
	}
	return key
}

// isClosedMonthCSVFilename 文件名末尾 `_YYYY-MM.csv`（已结账自然月全量导出）。[Ref: 04_采集 §六]
func isClosedMonthCSVFilename(base string) bool {
	return reCycleSuffix.MatchString(base)
}

func getObjectWithRetry(ctx context.Context, bucket *oss.Bucket, key string) (io.ReadCloser, error) {
	var last error
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(time.Duration(100*attempt) * time.Millisecond):
			}
			slog.Warn("ossfinops: GetObject retry", "key", key, "attempt", attempt+1)
		}
		rc, err := bucket.GetObject(key)
		if err == nil {
			return rc, nil
		}
		last = err
	}
	return nil, fmt.Errorf("GetObject after retries: %w", last)
}

var reCycleInKey = regexp.MustCompile(`(20\d{2})[-_]?(\d{2})`)

// 文件名末尾账期优先于路径中的导出时间戳（如 ...-20260325130300_... 会误匹配为 2026-03）。[Ref: Phase6 consumeDetailBillV2]
var reCycleSuffix = regexp.MustCompile(`(?i)[_-](20\d{2})[-_](\d{2})\.csv`)

// 无 _YYYY-MM 后缀的 consumeDetail 滚动导出：文件名中的长数字为导出时间，不作为账期提示。[Ref: 04_采集 §5.4]
var reRollingConsumeDetail = regexp.MustCompile(`(?i)consumedetail`)

func guessBillingCycleFromKey(key string) string {
	base := key
	if i := strings.LastIndex(key, "/"); i >= 0 {
		base = key[i+1:]
	}
	if m := reCycleSuffix.FindStringSubmatch(base); len(m) == 3 {
		return m[1] + "-" + m[2]
	}
	if reRollingConsumeDetail.MatchString(base) {
		return ""
	}
	dir := key
	if i := strings.LastIndex(key, "/"); i >= 0 {
		dir = key[:i]
	}
	if dir != "" {
		if m := reCycleInKey.FindStringSubmatch(dir); len(m) == 3 {
			return m[1] + "-" + m[2]
		}
	}
	// 简单文件名如 export_2024-01.csv（无 consumedetail）
	if m := reCycleInKey.FindStringSubmatch(base); len(m) == 3 {
		return m[1] + "-" + m[2]
	}
	return ""
}

// stableFinOpsDedupKey 行级幂等键：优先阿里云 RecordID；否则 SHA256(account|账期|日|实例|计费项|产品)。不含行号/文件名，多文件同键覆盖。[Ref: 04_采集 §5.6]
func stableFinOpsDedupKey(accountID, recordID, billingCycle, usageDateYMD, inst, item, prod string) string {
	recordID = strings.TrimSpace(recordID)
	if recordID != "" {
		h := sha256.Sum256([]byte(strings.TrimSpace(accountID) + "|rid|" + recordID))
		return hex.EncodeToString(h[:])
	}
	h := sha256.Sum256([]byte(strings.Join([]string{
		strings.TrimSpace(accountID),
		strings.TrimSpace(billingCycle),
		strings.TrimSpace(usageDateYMD),
		strings.TrimSpace(inst),
		strings.TrimSpace(item),
		strings.TrimSpace(prod),
	}, "|")))
	return hex.EncodeToString(h[:])
}

func ingestOneObject(ctx context.Context, db FactWriter, bucket *oss.Bucket, objectKey, accountID, cycleHint string, closedMonthFromFilename bool) error {
	rc, err := getObjectWithRetry(ctx, bucket, objectKey)
	if err != nil {
		return err
	}
	defer rc.Close()

	rows, billingCycle, err := parseCSVToFacts(rc, objectKey, accountID, cycleHint)
	if err != nil {
		return err
	}
	if billingCycle == "" {
		return fmt.Errorf("cannot determine billing_cycle for %s", objectKey)
	}
	if len(rows) == 0 {
		slog.Warn("ossfinops: no data rows", "key", objectKey, "billing_cycle", billingCycle)
		return nil
	}
	if closedMonthFromFilename {
		if err := db.ReplaceFinOpsBillingCycleWithFacts(ctx, billingCycle, accountID, rows); err != nil {
			return fmt.Errorf("replace billing cycle: %w", err)
		}
		slog.Info("ossfinops: closed-month replace", "key", objectKey, "billing_cycle", billingCycle, "rows", len(rows), "account_id", accountID)
		return nil
	}
	if err := db.BulkInsertFinOpsBillingFacts(ctx, rows); err != nil {
		return fmt.Errorf("bulk upsert: %w", err)
	}
	slog.Info("ossfinops: ingested csv", "key", objectKey, "billing_cycle", billingCycle, "rows", len(rows), "account_id", accountID)
	return nil
}

func parseCSVToFacts(r io.Reader, sourceObject, accountID, cycleHint string) ([]postgres.FinOpsBillingFactRow, string, error) {
	br := bufio.NewReader(r)
	b, err := br.Peek(3)
	if err != nil && err != io.EOF {
		return nil, "", err
	}
	if len(b) >= 3 && b[0] == 0xEF && b[1] == 0xBB && b[2] == 0xBF {
		_, _ = br.Discard(3)
	}
	cr := csv.NewReader(br)
	cr.LazyQuotes = true
	cr.FieldsPerRecord = -1

	// 部分导出文件首行为空或仅 BOM 换行，跳过直至非空表头行。[Ref: Phase6 consumeDetailBillV2 CSV]
	var header []string
	for {
		h, err := cr.Read()
		if err != nil {
			return nil, "", fmt.Errorf("csv header: %w", err)
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
	iRecord := findCol(idx, []string{"recordid", "账单记录id", "record id", "记录id"})
	iPretax := findCol(idx, []string{"应付金额", "pretaxamount", "pretax amount", "折后应付", "官网价", "应付金额(元)", "应付金额（含税）"})
	iProduct := findCol(idx, []string{"产品code", "产品代码", "productcode", "product code", "产品名称", "productname"})
	iInst := findCol(idx, []string{"实例id", "instanceid", "instance id", "实例信息", "实例", "实例id（出账粒度）"})
	iItem := findCol(idx, []string{"计费项", "billingitem", "bill item", "计费项代码", "billingitemcode", "商品名称", "计费项名称"})
	iTags := findCol(idx, []string{"标签", "tag", "tags", "用户标签", "usertag", "资源标签"})
	iAlias := findCol(idx, []string{"账号昵称", "accountname", "account name", "账号"})

	if (iBillDate < 0 && iConsume < 0 && iSvcStart < 0) || iPretax < 0 {
		return nil, "", fmt.Errorf("required columns missing: need (账单日期 or 消费时间 or 服务开始时间) + 应付金额, got header=%v", header)
	}

	var out []postgres.FinOpsBillingFactRow
	lineIdx := 0
	for {
		rec, err := cr.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, "", err
		}
		lineIdx++
		if len(rec) == 0 || rowEmpty(rec) {
			continue
		}
		dateStr := firstNonEmptyField(rec, iBillDate, iConsume, iSvcStart)
		amtStr := get(rec, iPretax)
		ud, err := parseDateFlexible(dateStr)
		if err != nil {
			slog.Debug("ossfinops: skip bad date", "line", lineIdx, "val", dateStr, "err", err)
			continue
		}
		amt, err := parseAmount(amtStr)
		if err != nil {
			slog.Debug("ossfinops: skip bad amount", "line", lineIdx, "val", amtStr)
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
		prod := ""
		if iProduct >= 0 {
			prod = strings.TrimSpace(get(rec, iProduct))
		}
		inst := ""
		if iInst >= 0 {
			inst = strings.TrimSpace(get(rec, iInst))
		}
		item := ""
		if iItem >= 0 {
			item = strings.TrimSpace(get(rec, iItem))
		}
		if item == "" {
			item = prod + "|" + inst
		}
		tagStr := ""
		if iTags >= 0 {
			tagStr = strings.TrimSpace(get(rec, iTags))
		}
		env := envFromTags(tagStr)
		alias := ""
		if iAlias >= 0 {
			alias = strings.TrimSpace(get(rec, iAlias))
		}
		var tagsJSON []byte
		if tagStr != "" {
			tagsJSON, _ = json.Marshal(tagStr)
		}
		recID := ""
		if iRecord >= 0 {
			recID = strings.TrimSpace(get(rec, iRecord))
		}
		dedup := stableFinOpsDedupKey(accountID, recID, rowCycle, ud.Format("2006-01-02"), inst, item, prod)
		out = append(out, postgres.FinOpsBillingFactRow{
			BillingCycle: rowCycle,
			UsageDate:    ud,
			AccountAlias: alias,
			AccountID:    accountID,
			Env:          env,
			ProductCode:  prod,
			InstanceID:   inst,
			ItemCode:     item,
			Amount:       amt,
			Currency:     "CNY",
			TagsJSON:     tagsJSON,
			SourceObject: sourceObject,
			DedupKey:     dedup,
		})
	}
	if len(out) == 0 {
		return nil, cycleHint, nil
	}
	return out, out[0].BillingCycle, nil
}

func normalizeHeader(h []string) []string {
	out := make([]string, len(h))
	for i, s := range h {
		s = strings.TrimSpace(s)
		s = strings.TrimPrefix(s, "\ufeff")
		out[i] = strings.ToLower(strings.TrimSpace(strings.ReplaceAll(s, " ", "")))
	}
	return out
}

func findCol(idx map[string]int, candidates []string) int {
	for _, c := range candidates {
		c = strings.ToLower(strings.ReplaceAll(c, " ", ""))
		if i, ok := idx[c]; ok {
			return i
		}
	}
	for _, c := range candidates {
		cc := strings.ToLower(strings.ReplaceAll(c, " ", ""))
		for h, i := range idx {
			if strings.Contains(h, cc) || strings.Contains(cc, h) {
				return i
			}
		}
	}
	return -1
}

func firstNonEmptyField(rec []string, cols ...int) string {
	for _, c := range cols {
		if c < 0 || c >= len(rec) {
			continue
		}
		if v := strings.TrimSpace(rec[c]); v != "" {
			return v
		}
	}
	return ""
}

func get(rec []string, i int) string {
	if i < 0 || i >= len(rec) {
		return ""
	}
	return rec[i]
}

func rowEmpty(rec []string) bool {
	for _, s := range rec {
		if strings.TrimSpace(s) != "" {
			return false
		}
	}
	return true
}

func parseAmount(s string) (float64, error) {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, ",", "")
	s = strings.ReplaceAll(s, "，", "")
	return strconv.ParseFloat(s, 64)
}

func parseDateFlexible(s string) (time.Time, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, fmt.Errorf("empty date")
	}
	// 阿里云部分导出「账单日期」为纯数字 YYYYMMDD（与「账单月份」列区分）。[Ref: Phase6 consumeDetailBillV2]
	if len(s) == 8 && isAllDigits(s) {
		if t, err := time.ParseInLocation("20060102", s, time.UTC); err == nil {
			return t.UTC().Truncate(24 * time.Hour), nil
		}
	}
	layouts := []string{
		"2006-01-02 15:04:05",
		"2006-01-02",
		"2006/1/2 15:04:05",
		"2006/01/02 15:04:05",
		"2006/1/2",
		"2006/01/02",
		"2006-1-2",
		time.RFC3339,
	}
	for _, l := range layouts {
		if t, err := time.ParseInLocation(l, s, time.UTC); err == nil {
			return t.UTC().Truncate(24 * time.Hour), nil
		}
	}
	// 仅日期前缀（整行带未识别后缀时）
	if i := strings.IndexByte(s, ' '); i > 0 {
		prefix := strings.TrimSpace(s[:i])
		for _, l := range layouts {
			if t, err := time.ParseInLocation(l, prefix, time.UTC); err == nil {
				return t.UTC().Truncate(24 * time.Hour), nil
			}
		}
	}
	return time.Time{}, fmt.Errorf("bad date %q", s)
}

func isAllDigits(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return s != ""
}

func normalizeBillingCycle(s string) string {
	s = strings.TrimSpace(s)
	if strings.Count(s, "-") == 1 && len(s) == 7 {
		return s
	}
	// 202509 -> 2025-09
	if len(s) == 6 && s[0] == '2' {
		return s[:4] + "-" + s[4:]
	}
	return s
}

var reEnvTag = regexp.MustCompile(`(?i)env\s*[:=]\s*([A-Za-z0-9_\-]+)`)

func envFromTags(tagStr string) string {
	if tagStr == "" {
		return "UNTAGGED"
	}
	if m := reEnvTag.FindStringSubmatch(tagStr); len(m) == 2 {
		return strings.ToUpper(strings.TrimSpace(m[1]))
	}
	// 简单 JSON: [{"key":"env","value":"UAT"}]
	if strings.Contains(tagStr, "env") && (strings.Contains(tagStr, "[") || strings.Contains(tagStr, "{")) {
		var arr []struct {
			Key   string `json:"key"`
			Value string `json:"value"`
		}
		if json.Unmarshal([]byte(tagStr), &arr) == nil {
			for _, t := range arr {
				if strings.EqualFold(t.Key, "env") && t.Value != "" {
					return strings.ToUpper(strings.TrimSpace(t.Value))
				}
			}
		}
	}
	return "UNTAGGED"
}

// EnvForFinOps 从环境变量解析 OSS 凭证（与 BillingWorker.EnvKey 对齐的后缀）。
func EnvForFinOps(accountEnv string) (ak, sk string) {
	suf := strings.TrimSpace(accountEnv)
	if suf == "" {
		return os.Getenv("ALIBABA_CLOUD_ACCESS_KEY_ID"), os.Getenv("ALIBABA_CLOUD_ACCESS_KEY_SECRET")
	}
	ak = os.Getenv("ALIBABA_CLOUD_ACCESS_KEY_ID_" + suf)
	sk = os.Getenv("ALIBABA_CLOUD_ACCESS_KEY_SECRET_" + suf)
	return ak, sk
}
