// Package ossfinops 从阿里云 OSS 流式读取账单 CSV 并写入 finops_billing_fact。[Ref: Phase6 OLAP 落地]
package ossfinops

import (
	"archive/zip"
	"bufio"
	"bytes"
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

// 抹零列去重：控制台「抹零金额」多为整月一笔（如 0.101269 USD），部分 CSV 在**每一行**重复同一数值；
// 若对列简单求和会得到 N×抹零，追加 −roundSum 后总额严重偏离应付。[Ref: 04_采集 §5.4 抹零]
const (
	finopsRoundingDupRatio  = 10.0  // sum(列) 远大于单列 max 时视为「整月抹零被逐行重复」
	finopsRoundingBillFloor = 0.01  // 低于此的单列值视为行级分摊，不触发 dedup（避免误伤按比例拆分）
)

// normalizeRoundingSum 在检测到「sum ≫ max」且 max 足够大时，将账单级抹零取为 max（单笔），否则保持 sum。
func normalizeRoundingSum(roundSum, maxCell float64) float64 {
	if maxCell < finopsRoundingBillFloor || roundSum <= 0 {
		return roundSum
	}
	if roundSum > finopsRoundingDupRatio*maxCell {
		slog.Info("ossfinops: rounding dedup (monthly rounding duplicated on many rows)", "round_sum_raw", roundSum, "max_cell", maxCell, "round_sum_use", maxCell)
		return maxCell
	}
	return roundSum
}

// normalizeBillLevelDupSum 与 normalizeRoundingSum 相同算法，用于优惠券抵扣等「整月一笔可能多行重复」列。[Ref: 04_采集 §5.4]
func normalizeBillLevelDupSum(colSum, maxCell float64) float64 {
	return normalizeRoundingSum(colSum, maxCell)
}

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

// LoadBillingCSVsFromOSS 列举 Prefix 下账单 CSV（含无后缀的 BillingItemDetail 订阅对象），按 LastModified **升序**处理；**文件名含 `_YYYY-MM.csv` 关账全量**时单事务 DELETE 账期+写入以消除幽灵行。[Ref: 04_采集 §六 §七 R9]
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

	all, err := ListOSSBillingObjects(bucket, prefix)
	if err != nil {
		return time.Time{}, err
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
	var ingestErrs int
	for _, obj := range work {
		key := obj.Key
		cycle := GuessBillingCycleFromKey(key)
		if cfg.SyncMode == "current_month" && cycle != "" && cycle != curCycle {
			slog.Info("ossfinops: skip non-current month object", "key", key, "cycle", cycle, "want", curCycle)
			continue
		}
		closed := isClosedMonthCSVFilename(baseName(key))
		if err := ingestOneObject(ctx, db, bucket, key, cfg.AccountID, cycle, closed); err != nil {
			ingestErrs++
			slog.Error("ossfinops: ingest object failed", "key", key, "err", err)
			continue
		}
	}
	if ingestErrs > 0 {
		return maxLM, fmt.Errorf("ossfinops: %d object(s) failed ingest (see logs); checkpoint will not advance on error return", ingestErrs)
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
var reExportStampBeforeInstanceConsumeDay = regexp.MustCompile(`(?i)-(\d{14})_instanceconsumeday`)

// exportTimestampFromConsumeDetailName 从 consumeDetail / instanceconsumeday 文件名提取导出时间戳（14 位），缺失时返回空串（排序退化为 key）。[Ref: 04_采集 §七]
func exportTimestampFromConsumeDetailName(base string) string {
	if m := reExportStampBeforeConsumeDetail.FindStringSubmatch(base); len(m) == 2 {
		return m[1]
	}
	if m := reExportStampBeforeInstanceConsumeDay.FindStringSubmatch(base); len(m) == 2 {
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

// GuessBillingCycleFromKey 从 OSS 对象 key 推断账期 YYYY-MM（文件名 _YYYY-MM.csv 优先；否则目录或简单文件名）。[Ref: 04_采集 §5.4]
func GuessBillingCycleFromKey(key string) string {
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
	lk := strings.ToLower(objectKey)
	if strings.HasSuffix(lk, ".zip") {
		if !strings.Contains(strings.ToLower(baseName(objectKey)), "billingitemdetail") {
			return fmt.Errorf("ossfinops: unsupported billing zip key %s", objectKey)
		}
		return ingestBillingItemDetailZip(ctx, db, bucket, objectKey, accountID, cycleHint, closedMonthFromFilename)
	}
	rc, err := getObjectWithRetry(ctx, bucket, objectKey)
	if err != nil {
		return err
	}
	defer rc.Close()
	return ingestFromParsedFacts(ctx, db, rc, objectKey, accountID, cycleHint, closedMonthFromFilename)
}

// ingestBillingItemDetailZip BSS 订阅在子目录下常为 zip，内嵌同名 .csv；须解压后再 parseCSVToFacts。[Ref: 04_采集 §七 R13]
func ingestBillingItemDetailZip(ctx context.Context, db FactWriter, bucket *oss.Bucket, objectKey, accountID, cycleHint string, closedOuter bool) error {
	rc, err := getObjectWithRetry(ctx, bucket, objectKey)
	if err != nil {
		return err
	}
	body, err := io.ReadAll(rc)
	_ = rc.Close()
	if err != nil {
		return fmt.Errorf("read zip body: %w", err)
	}
	if len(body) < 22 {
		return fmt.Errorf("zip too small: %s", objectKey)
	}
	zr, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
	if err != nil {
		return fmt.Errorf("zip open %s: %w", objectKey, err)
	}
	var n int
	for _, f := range zr.File {
		if f.FileInfo().IsDir() {
			continue
		}
		if !strings.HasSuffix(strings.ToLower(f.Name), ".csv") {
			continue
		}
		fr, err := f.Open()
		if err != nil {
			return fmt.Errorf("zip entry open %s: %w", f.Name, err)
		}
		innerSource := objectKey + "#" + f.Name
		innerClosed := closedOuter || isClosedMonthCSVFilename(baseName(f.Name))
		err = ingestFromParsedFacts(ctx, db, fr, innerSource, accountID, cycleHint, innerClosed)
		_ = fr.Close()
		if err != nil {
			return fmt.Errorf("zip inner %s: %w", f.Name, err)
		}
		n++
	}
	if n == 0 {
		return fmt.Errorf("no .csv inside billing zip %s", objectKey)
	}
	slog.Info("ossfinops: ingested billing zip", "key", objectKey, "inner_csv_count", n, "account_id", accountID)
	return nil
}

func ingestFromParsedFacts(ctx context.Context, db FactWriter, r io.Reader, sourceObject, accountID, cycleHint string, closedMonthFromFilename bool) error {
	rows, billingCycle, err := parseCSVToFacts(r, sourceObject, accountID, cycleHint)
	if err != nil {
		return err
	}
	if billingCycle == "" {
		return fmt.Errorf("cannot determine billing_cycle for %s", sourceObject)
	}
	if len(rows) == 0 {
		slog.Warn("ossfinops: no data rows", "key", sourceObject, "billing_cycle", billingCycle)
		return nil
	}
	if closedMonthFromFilename {
		if err := db.ReplaceFinOpsBillingCycleWithFacts(ctx, billingCycle, accountID, rows); err != nil {
			return fmt.Errorf("replace billing cycle: %w", err)
		}
		slog.Info("ossfinops: closed-month replace", "key", sourceObject, "billing_cycle", billingCycle, "rows", len(rows), "account_id", accountID)
		return nil
	}
	if err := db.BulkInsertFinOpsBillingFacts(ctx, rows); err != nil {
		return fmt.Errorf("bulk upsert: %w", err)
	}
	slog.Info("ossfinops: ingested csv", "key", sourceObject, "billing_cycle", billingCycle, "rows", len(rows), "account_id", accountID)
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
	iBillDate := findCol(idx, []string{"账单日期", "billingdate", "billing date", "账务日期", "扣费时间"})
	iConsume := findCol(idx, []string{"消费时间", "consumptiontime", "usage time"})
	iSvcStart := findCol(idx, []string{"服务开始时间", "servicestarttime", "service start"})
	iCycle := findCol(idx, []string{"账单月份", "账期", "billingcycle", "billing cycle", "账单周期"})
	iRecord := findCol(idx, []string{"主键", "recordid", "账单记录id", "record id", "记录id", "billid"})
	// 国际站/折扣类导出常见「优惠后金额」为行级应付；优先于「应付金额」。[Ref: k8s-finops-billing-poc CSV]
	iPretax := findCol(idx, []string{
		"优惠后金额", "折扣后金额", "应付金额", "pretaxamount", "pretax amount", "折后应付", "官网价", "应付金额(元)", "应付金额（含税）",
		"官网价含税金额", "应付信息/应付金额", "payableamount", "listprice", "pretaxgrossamount",
	})
	iProduct := findCol(idx, []string{"产品code", "产品代码", "productcode", "product code", "产品名称", "productname"})
	iInst := findCol(idx, []string{"资产id", "实例id", "instanceid", "instance id", "实例信息", "实例", "实例id（出账粒度）"})
	iItem := findCol(idx, []string{"计费项", "billingitem", "bill item", "计费项代码", "billingitemcode", "商品名称", "计费项名称"})
	iTags := findCol(idx, []string{"标签", "tag", "tags", "用户标签", "usertag", "资源标签"})
	iAlias := findCol(idx, []string{"账号昵称", "accountname", "account name", "账号"})
	iCurrency := findCol(idx, []string{"定价币种", "币种", "currency"})
	// 月账单级「抹零」：控制台「应付 = 行明细口径 − 抹零(正)」；列可能仅个别行非零，按列求和后补一行使 SUM 对齐。[Ref: 04_采集 §5.4 抹零]
	// 国际站/英文表头常见 Round off / Rounding amount；括号与币种后缀经 normalizeHeader 去空格后仍可能带 (usd) 等。
	iRounding := findCol(idx, []string{
		"抹零金额", "抹零", "账单抹零金额", "抹零调整金额", "抹零金额(元)",
		"roundoffamount", "roundoff", "roundoffamount(usd)", "roundoffamount(cny)",
		"roundingamount", "rounding amount", "roundingamount(usd)", "roundingamount(cny)",
		"roundofffee", "roundingfee", "roundoff adjustment", "roundingadjustment",
	})
	// 控制台应付 = 目录总价 − 优惠 − **优惠券抵扣** − 抹零；行级「优惠后」常为目录−优惠，需单独扣券。[Ref: 04_采集 §5.4]
	iCoupon := findCol(idx, []string{
		"优惠券抵扣金额", "优惠券抵扣", "代金券抵扣", "券抵扣",
		"coupondeduction", "coupondeductionamount", "coupon deduction", "deductedbycoupons",
		"couponamount", "cashcoupon",
	})

	hasAnyDateCol := iBillDate >= 0 || iConsume >= 0 || iSvcStart >= 0
	if iPretax < 0 {
		return nil, "", fmt.Errorf("required columns missing: need pretax/应付类金额列, got header=%v", header)
	}
	if !hasAnyDateCol && iCycle < 0 {
		return nil, "", fmt.Errorf("required columns missing: need (账单日期 or 消费时间 or 服务开始时间 or 账单月份/账期), got header=%v", header)
	}
	if iRounding < 0 {
		slog.Warn("ossfinops: CSV has no recognizable rounding column; SUM(amount) may differ from console payable by monthly rounding (抹零). Prefer consumeDetail/monthly bill export with 抹零金额, not instance day-only CSV.",
			"source", sourceObject)
	}
	if iCoupon < 0 {
		slog.Warn("ossfinops: CSV has no recognizable coupon column; SUM(amount) may differ from console payable by coupon deduction (优惠券抵扣).",
			"source", sourceObject)
	}

	var out []postgres.FinOpsBillingFactRow
	var roundSum float64
	var roundMax float64 // 抹零列非零单元格的最大绝对值（用于识别「整月一笔被多行重复」）
	var couponSum float64
	var couponMax float64
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
		var ud time.Time
		var derr error
		if strings.TrimSpace(dateStr) != "" {
			ud, derr = parseDateFlexible(dateStr)
		} else if iCycle >= 0 {
			c := strings.TrimSpace(get(rec, iCycle))
			if c != "" {
				nc := normalizeBillingCycle(c)
				if len(nc) == 7 && strings.Count(nc, "-") == 1 {
					ud, derr = time.ParseInLocation("2006-01-02", nc+"-01", time.UTC)
				} else {
					derr = fmt.Errorf("bad billing cycle %q", c)
				}
			} else {
				derr = fmt.Errorf("empty billing cycle cell")
			}
		} else {
			derr = fmt.Errorf("empty date")
		}
		if derr != nil {
			slog.Debug("ossfinops: skip bad date", "line", lineIdx, "val", dateStr, "err", derr)
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
		cur := "CNY"
		if iCurrency >= 0 {
			if c := strings.TrimSpace(get(rec, iCurrency)); c != "" {
				cur = strings.ToUpper(c)
			}
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
			Currency:     cur,
			TagsJSON:     tagsJSON,
			SourceObject: sourceObject,
			DedupKey:     dedup,
		})
		if iRounding >= 0 {
			if rs := strings.TrimSpace(get(rec, iRounding)); rs != "" {
				if rv, err := parseAmount(rs); err == nil {
					roundSum += rv
					av := rv
					if av < 0 {
						av = -av
					}
					if av > roundMax {
						roundMax = av
					}
				}
			}
		}
		if iCoupon >= 0 {
			if cs := strings.TrimSpace(get(rec, iCoupon)); cs != "" {
				if cv, err := parseAmount(cs); err == nil {
					couponSum += cv
					av := cv
					if av < 0 {
						av = -av
					}
					if av > couponMax {
						couponMax = av
					}
				}
			}
		}
	}

	roundSum = normalizeRoundingSum(roundSum, roundMax)
	couponSum = normalizeBillLevelDupSum(couponSum, couponMax)

	// 按天实例消耗（instance consume day）导出：列名可能含「券/抹零」但语义为行级分摊或口径与月账单不一致，按列求和再追加账单级 FINOPS_* 会与控制台应付冲突；仅保留行级 amount。费用明细 consumedetail 仍追加汇总行。[Ref: 04_采集 §5.4、OSS_BILLING_CONFIG 方案 A]
	if isInstanceConsumeDayCSVSource(sourceObject) {
		if couponSum != 0 || roundSum != 0 {
			slog.Warn("ossfinops: skip bill-level coupon/rounding adjustment rows for instance consume day export (not bill-level 券/抹零); use consumeDetail CSV or API backfill",
				"source", sourceObject, "coupon_sum_skipped", couponSum, "round_sum_skipped", roundSum)
		}
		couponSum = 0
		roundSum = 0
	}

	// 优惠券抵扣汇总行：控制台应付已扣券；列求和为券扣减正数时，补一行 amount = −couponSum。[Ref: 04_采集 §5.4]
	if couponSum != 0 {
		bc := cycleHint
		if len(out) > 0 {
			bc = out[0].BillingCycle
		}
		if strings.TrimSpace(bc) == "" {
			bc = time.Now().UTC().Format("2006-01")
		}
		cur := "CNY"
		if len(out) > 0 {
			cur = out[0].Currency
		}
		ud := lastDayOfBillingCycleYM(bc)
		recC := "COUPON|" + sourceObject
		dedupC := stableFinOpsDedupKey(accountID, recC, bc, ud.Format("2006-01-02"), "", "FINOPS_BILLING_COUPON_DEDUCTION", "COUPON")
		adjC := -couponSum
		slog.Info("ossfinops: coupon deduction adjustment row", "source", sourceObject, "billing_cycle", bc, "coupon_sum", couponSum, "amount", adjC, "account_id", accountID)
		out = append(out, postgres.FinOpsBillingFactRow{
			BillingCycle: bc,
			UsageDate:    ud,
			AccountAlias: "",
			AccountID:    accountID,
			Env:          "UNTAGGED",
			ProductCode:  "COUPON",
			InstanceID:   "",
			ItemCode:     "FINOPS_BILLING_COUPON_DEDUCTION",
			Amount:       adjC,
			Currency:     cur,
			TagsJSON:     nil,
			SourceObject: sourceObject,
			DedupKey:     dedupC,
		})
	}

	// 抹零汇总行：控制台应付 = 明细优惠后之和 − 抹零(正数表示扣减)；补一行 amount = −roundSum，使 SUM(amount) 与月账单一致。
	if roundSum != 0 {
		bc := cycleHint
		if len(out) > 0 {
			bc = out[0].BillingCycle
		}
		if strings.TrimSpace(bc) == "" {
			bc = time.Now().UTC().Format("2006-01")
		}
		cur := "CNY"
		if len(out) > 0 {
			cur = out[0].Currency
		}
		ud := lastDayOfBillingCycleYM(bc)
		recRound := "ROUNDING|" + sourceObject
		dedupR := stableFinOpsDedupKey(accountID, recRound, bc, ud.Format("2006-01-02"), "", "FINOPS_BILLING_ROUNDING", "ROUNDING")
		adj := -roundSum
		slog.Info("ossfinops: rounding adjustment row", "source", sourceObject, "billing_cycle", bc, "round_sum", roundSum, "amount", adj, "account_id", accountID)
		out = append(out, postgres.FinOpsBillingFactRow{
			BillingCycle: bc,
			UsageDate:    ud,
			AccountAlias: "",
			AccountID:    accountID,
			Env:          "UNTAGGED",
			ProductCode:  "ROUNDING",
			InstanceID:   "",
			ItemCode:     "FINOPS_BILLING_ROUNDING",
			Amount:       adj,
			Currency:     cur,
			TagsJSON:     nil,
			SourceObject: sourceObject,
			DedupKey:     dedupR,
		})
	}

	if len(out) == 0 {
		return nil, cycleHint, nil
	}
	return out, out[0].BillingCycle, nil
}

// lastDayOfBillingCycleYM 解析 YYYY-MM，返回该月最后一日 UTC 00:00（用于抹零汇总行 usage_date）。
func lastDayOfBillingCycleYM(cycle string) time.Time {
	t, err := time.ParseInLocation("2006-01", strings.TrimSpace(cycle), time.UTC)
	if err != nil {
		return time.Now().UTC().Truncate(24 * time.Hour)
	}
	return t.AddDate(0, 1, -1)
}

func isInstanceConsumeDayCSVSource(sourceObject string) bool {
	return strings.Contains(strings.ToLower(sourceObject), "instanceconsumeday")
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

// findCol 先精确匹配 normalize 后的表头；再按「表头包含候选子串」匹配（费用明细类长表头）。
// 禁止 strings.Contains(候选, 表头)：否则 "couponamount" 会误命中短表头 "amount"，把应付列当券抵扣累加。[Ref: 04_采集 §5.4 优惠券]
func findCol(idx map[string]int, candidates []string) int {
	for _, c := range candidates {
		c = strings.ToLower(strings.ReplaceAll(c, " ", ""))
		if i, ok := idx[c]; ok {
			return i
		}
	}
	for _, c := range candidates {
		cc := strings.ToLower(strings.ReplaceAll(c, " ", ""))
		if len(cc) < 3 {
			continue
		}
		for h, i := range idx {
			if strings.Contains(h, cc) {
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

// EnvForFinOps 从环境变量解析 OSS 凭证；accountEnv 与 BillingWorker.EnvKey 一致（如 C66_UAT），对应 ALIBABA_CLOUD_ACCESS_KEY_ID_C66_UAT。
func EnvForFinOps(accountEnv string) (ak, sk string) {
	suf := strings.TrimSpace(accountEnv)
	if suf == "" {
		return os.Getenv("ALIBABA_CLOUD_ACCESS_KEY_ID"), os.Getenv("ALIBABA_CLOUD_ACCESS_KEY_SECRET")
	}
	ak = os.Getenv("ALIBABA_CLOUD_ACCESS_KEY_ID_" + suf)
	sk = os.Getenv("ALIBABA_CLOUD_ACCESS_KEY_SECRET_" + suf)
	return ak, sk
}
