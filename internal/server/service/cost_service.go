// Package service provides business logic services for the HTTP API.
package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/myxxhui/lighthouse-src/internal/config"
	"github.com/myxxhui/lighthouse-src/internal/data/postgres"
	"github.com/myxxhui/lighthouse-src/internal/server/dto"
	"github.com/myxxhui/lighthouse-src/pkg/billingcalendar"
	"github.com/myxxhui/lighthouse-src/pkg/costmodel"
)

// finOpsLedgerSnapshotNoteZH 可选元数据说明；前端默认不展示。避免恒等式表述。[Ref: 03_Phase6/01_FinOps]
const finOpsLedgerSnapshotNoteZH = "五维为多数据源并列指标，各维时间口径可能不同，仅供对照。"

// CostService provides cost-related business logic using Mock data and costmodel.
type CostService struct {
	repo postgres.Repository
	// finOpsCGSource 默认 C/G 源（oss|api）；按环境覆盖见 finOpsCGSourceByEnv。[Ref: 03_Phase6/01_FinOps]
	finOpsCGSource string
	// finOpsCGSourceByEnv 环境名（大写）→ oss|api，来自 FINOPS_CG_SOURCE_<ENV> 或配置。[Ref: 03_Phase6/01_FinOps]
	finOpsCGSourceByEnv map[string]string

	// [Ref: D8-7] 日期选择叠加结果短时缓存，key=from:to，TTL 1h
	dateRangeCache    map[string]dateRangeCacheEntry
	dateRangeCacheMu  sync.RWMutex
	dateRangeCacheTTL time.Duration

	// finOpsAuxiliarySync 可选：ledger_refresh=1 时对各环境执行 BSS/应付拉取（由 main 注入 BillingWorker.SyncFinOpsAuxiliary）。[Ref: 03_Phase6/01_FinOps]
	finOpsAuxiliarySync func(context.Context) error

	// cloudDisplay 可选：YAML 云环境账户卡标题（项目-云-环境）；nil 时前端用旧格式。[Ref: 03_Phase6/03_前端全域成本透视/01_设计]
	cloudDisplay *config.CloudAccountDisplay
}

type dateRangeCacheEntry struct {
	resp *dto.GlobalCostResponse
	exp  time.Time
}

// NewCostService creates a new CostService with the given repository.
// finOpsCGSource：FINOPS_CG_SOURCE 默认；finOpsCGSourceByEnv：按环境覆盖（可为 nil）。[Ref: 03_Phase6/01_FinOps]
func NewCostService(repo postgres.Repository, finOpsCGSource string, finOpsCGSourceByEnv map[string]string) *CostService {
	return &CostService{
		repo:                repo,
		finOpsCGSource:      config.EffectiveFinOpsCGSource(finOpsCGSource),
		finOpsCGSourceByEnv: config.BuildFinOpsCGSourceByEnvMap(finOpsCGSourceByEnv),
		dateRangeCache:      make(map[string]dateRangeCacheEntry),
		dateRangeCacheTTL:   time.Hour,
	}
}

// SetFinOpsAuxiliarySync 注册在读库前触发的多环境 FinOps 辅助同步（可为 nil）。[Ref: 03_Phase6/01_FinOps]
func (s *CostService) SetFinOpsAuxiliarySync(fn func(context.Context) error) {
	s.finOpsAuxiliarySync = fn
}

// SetCloudAccountDisplay 注册云环境账户 YAML 展示（可为 nil）。[Ref: 03_Phase6/03_前端全域成本透视/01_设计]
func (s *CostService) SetCloudAccountDisplay(c *config.CloudAccountDisplay) {
	if s == nil {
		return
	}
	s.cloudDisplay = c
}

// finalizeGlobalCostResponse 写 cloud_account_label 等展示字段。[Ref: 03_Phase6/03_前端全域成本透视/01_设计]
func (s *CostService) finalizeGlobalCostResponse(ctx context.Context, resp *dto.GlobalCostResponse) {
	if s == nil || resp == nil {
		return
	}
	s.applyCloudAccountLabels(ctx, resp)
}

// applyCloudAccountLabels 按 cost_project + YAML 为 env_breakdown 填 cloud_account_label。[Ref: 03_Phase6/03_前端全域成本透视/01_设计]
func (s *CostService) applyCloudAccountLabels(ctx context.Context, resp *dto.GlobalCostResponse) {
	if s == nil || resp == nil || s.cloudDisplay == nil || len(resp.EnvBreakdown) == 0 {
		return
	}
	projects, err := s.repo.ListCostProjects(ctx)
	if err != nil || len(projects) == 0 {
		return
	}
	for i := range resp.EnvBreakdown {
		env := strings.TrimSpace(resp.EnvBreakdown[i].Environment)
		if env == "" {
			continue
		}
		pname := primaryProjectNameForEnvFromList(projects, env)
		if pname == "" {
			continue
		}
		label, site := lookupCloudDisplayLabel(s.cloudDisplay, pname, env)
		if label != "" {
			resp.EnvBreakdown[i].CloudAccountLabel = label
		}
		if site != "" {
			resp.EnvBreakdown[i].CloudAccountSiteNote = site
		}
	}
}

func primaryProjectNameForEnvFromList(projects []postgres.CostProject, env string) string {
	var hits []postgres.CostProject
	for _, p := range projects {
		for _, e := range p.Environments {
			if strings.EqualFold(strings.TrimSpace(e), env) {
				hits = append(hits, p)
				break
			}
		}
	}
	if len(hits) == 0 {
		return ""
	}
	sort.Slice(hits, func(i, j int) bool {
		if hits[i].SortOrder != hits[j].SortOrder {
			return hits[i].SortOrder < hits[j].SortOrder
		}
		return hits[i].ID < hits[j].ID
	})
	return hits[0].Name
}

func lookupCloudDisplayLabel(d *config.CloudAccountDisplay, projectName, env string) (label, siteNote string) {
	if d == nil {
		return "", ""
	}
	pn := strings.TrimSpace(projectName)
	en := strings.TrimSpace(env)
	for _, pr := range d.Projects {
		if !strings.EqualFold(strings.TrimSpace(pr.Name), pn) {
			continue
		}
		for _, acc := range pr.Accounts {
			if !strings.EqualFold(strings.TrimSpace(acc.Environment), en) {
				continue
			}
			third := strings.TrimSpace(acc.DisplayEnv)
			if third == "" {
				third = formatEnvTitleSegment(en)
			}
			cloud := strings.TrimSpace(acc.Cloud)
			if cloud == "" {
				cloud = "Aliyun"
			}
			label = fmt.Sprintf("%s-%s-%s", pn, cloud, third)
			return label, cloudAccountSiteNoteZH(acc.Site)
		}
	}
	return "", ""
}

func formatEnvTitleSegment(env string) string {
	e := strings.TrimSpace(env)
	if e == "" {
		return e
	}
	if len(e) == 1 {
		return strings.ToUpper(e)
	}
	return strings.ToUpper(e[:1]) + strings.ToLower(e[1:])
}

func cloudAccountSiteNoteZH(site string) string {
	switch strings.ToLower(strings.TrimSpace(site)) {
	case "domestic", "cn", "china":
		return "国内站"
	case "international", "intl", "overseas", "global":
		return "国际站"
	default:
		return ""
	}
}

func (s *CostService) runFinOpsAuxiliarySyncIfRequested(ctx context.Context) {
	if !LedgerRefreshRequested(ctx) || s.finOpsAuxiliarySync == nil {
		return
	}
	syncCtx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	if err := s.finOpsAuxiliarySync(syncCtx); err != nil {
		slog.Warn("FinOps auxiliary sync (ledger_refresh) failed", "error", err)
	}
}

// CostDiagnostic 成本数据源诊断，供 GET /api/v1/cost/diagnostic 返回；用于排查「暂无数据」根因。[Ref: 01_实践 §2.4]
type CostDiagnostic struct {
	DailyRawCountLast30   int     `json:"daily_raw_count_last_30d"`   // 近 30 天日原始表行数
	DailyRawCashSumLast30 float64 `json:"daily_raw_cash_sum_last_30d"` // 近 30 天日表 cash_total_amount 代数和
	MonthlyRawCountRecent int     `json:"monthly_raw_count_recent_3"` // 上月/前两月/前三月月原始表存在数（0～3）
	AggregateMonthRows    int     `json:"aggregate_month_rows"`       // 聚合表 report_type=month 当前月 period_key 行数
	AggregateMonthTotal   float64 `json:"aggregate_month_total"`      // 上述行 total_amount 合计
	Hint                  string `json:"hint"`                        // 简要排查建议
}

// GetCostDiagnostic 返回成本数据源健康情况，便于定位「无数据」是 ETL 未写、API 返回 0 还是读路径问题。
func (s *CostService) GetCostDiagnostic(ctx context.Context) (*CostDiagnostic, error) {
	now := time.Now().UTC()
	from30 := now.AddDate(0, 0, -30).Truncate(24 * time.Hour)
	out := &CostDiagnostic{}
	rows, err := s.repo.ListCloudBillDailyRawFromTo(ctx, from30, now, "")
	if err == nil {
		out.DailyRawCountLast30 = len(rows)
		for _, r := range rows {
			out.DailyRawCashSumLast30 += r.CashTotalAmount
		}
	}
	for i := 1; i <= 3; i++ {
		cycle := now.AddDate(0, -i, 0).Format("2006-01")
		list, _ := s.repo.ListCloudBillMonthlyRawByCycle(ctx, cycle)
		if len(list) > 0 {
			out.MonthlyRawCountRecent++
		}
	}
	month := now.Format("2006-01")
	list, _ := s.repo.ListCloudBillAggregateForReportPeriod(ctx, "month", month, "consumption", nil)
	out.AggregateMonthRows = len(list)
	for _, a := range list {
		out.AggregateMonthTotal += a.TotalAmount
	}
	// 简要提示（当前使用消耗口径 total_amount）[Ref: 用户确认 消耗口径]
	if out.DailyRawCountLast30 == 0 && out.MonthlyRawCountRecent == 0 {
		out.Hint = "日/月原始表均无数据：请确认 1) CLOUD_BILLING_PROVIDER=aliyun 且已配置 AK/SK；2) 后端启动后 ETL 已执行（查看日志 billing ETL pipeline）；3) 已执行 migrate-02/migrate-03 等迁移"
	} else if out.DailyRawCountLast30 > 0 && out.DailyRawCashSumLast30 == 0 {
		out.Hint = "当前使用消耗口径（total_amount）；日表 cash 为 0 不影响展示"
	} else if out.AggregateMonthRows == 0 && (out.DailyRawCountLast30 > 0 || out.MonthlyRawCountRecent > 0) {
		out.Hint = "日/月表有数据但聚合表无当月：RunPipeline 聚合步骤可能未执行或失败；查看日志 runAggregateStep"
	} else if out.AggregateMonthTotal > 0 {
		out.Hint = "数据源正常；若前端仍为 0 请检查 period 与 reportType/periodKey 映射及前端请求参数"
	} else {
		out.Hint = "聚合表有行但 total_amount=0；或仅历史月有数据、当月为 0（T+1 延迟或当月暂无消费）"
	}
	return out, nil
}

// toCostmodelDailyNamespaceCost converts postgres.DailyNamespaceCost to costmodel.DailyNamespaceCost.
func toCostmodelDailyNamespaceCost(p postgres.DailyNamespaceCost) costmodel.DailyNamespaceCost {
	return costmodel.DailyNamespaceCost{
		Namespace:     p.Namespace,
		Date:          p.Date,
		BillableCost:  p.BillableCost,
		UsageCost:     p.UsageCost,
		WasteCost:     p.WasteCost,
		PodCount:      p.PodCount,
		NodeCount:     p.NodeCount,
		WorkloadCount: p.WorkloadCount,
	}
}

// domainOrderForBreakdown 成本分解五大类，固定顺序 [Ref: 01_设计 §成本结构、成本分解；service_test 期望五类含「其他」]
var domainOrderForBreakdown = []string{"计算资源", "存储", "网络", "安全", "其他"}

// buildDomainBreakdownNormalized 从 merged（领域/领域:产品码）构建 domain_breakdown：五大类；「其它」归并入「其他」，按固定顺序返回。[Ref: 01_设计 §成本分解]
func buildDomainBreakdownNormalized(merged map[string]float64) []dto.DomainBreakdownItem {
	// 「其它」→「其他」（统一写法）
	if v, ok := merged["其它"]; ok {
		merged["其他"] += v
		delete(merged, "其它")
	}
	var toOther []string
	for k := range merged {
		if strings.HasPrefix(k, "其它:") {
			toOther = append(toOther, k)
		}
	}
	for _, k := range toOther {
		suffix := strings.TrimPrefix(k, "其它:")
		merged["其他:"+suffix] += merged[k]
		delete(merged, k)
	}
	if _, ok := merged["安全"]; !ok {
		merged["安全"] = 0
	}
	if _, ok := merged["其他"]; !ok {
		merged["其他"] = 0
	}
	out := make([]dto.DomainBreakdownItem, 0, len(domainOrderForBreakdown))
	for _, domain := range domainOrderForBreakdown {
		topLevelCost := merged[domain]
		var productSum float64
		prefix := domain + ":"
		for k, v := range merged {
			if strings.HasPrefix(k, prefix) {
				productSum += v
			}
		}
		cost := topLevelCost
		if math.Abs(productSum) > math.Abs(topLevelCost) {
			cost = productSum
		}
		topProducts := topProductsForDomain(&merged, domain, 4)
		out = append(out, dto.DomainBreakdownItem{
			Domain:           domain,
			Cost:             cost,
			OptimizableSpace: 0,
			Efficiency:       0,
			TopProducts:      topProducts,
		})
	}
	return out
}

// scaleMergedToTotal 将 merged 按 targetTotal 等比例缩放，使 sum(merged)=targetTotal，保证 total_cost 与 sum(products) 一致。[Ref: 用户确认 月表权威值+总成本=产品之和]
func scaleMergedToTotal(merged map[string]float64, targetTotal float64) {
	if len(merged) == 0 {
		return
	}
	var sum float64
	for _, v := range merged {
		sum += v
	}
	if math.Abs(sum) < 1e-9 {
		return
	}
	scale := targetTotal / sum
	for k := range merged {
		merged[k] *= scale
	}
}

// scaleDomainBreakdownToTotal 将 domain_breakdown 各 Cost 按 targetTotal 等比例缩放，使 sum(domain_breakdown[].Cost)=targetTotal。[Ref: 成本分解总和=总成本]
func scaleDomainBreakdownToTotal(domainBreakdown []dto.DomainBreakdownItem, targetTotal float64) {
	if len(domainBreakdown) == 0 {
		return
	}
	var sum float64
	for _, d := range domainBreakdown {
		sum += d.Cost
	}
	if math.Abs(sum) < 1e-9 {
		return
	}
	scale := targetTotal / sum
	for i := range domainBreakdown {
		domainBreakdown[i].Cost *= scale
	}
}

// scaleEnvBreakdownToTotal 将 env_breakdown 各 TotalCost 按 targetTotal 等比例缩放，使 sum(env_breakdown)=总环境成本。[Ref: 用户需求 环境成本与全环境总成本一致]
func scaleEnvBreakdownToTotal(envBreakdown []dto.EnvBreakdownItem, targetTotal float64) {
	if len(envBreakdown) == 0 {
		return
	}
	if targetTotal <= 0 {
		for i := range envBreakdown {
			envBreakdown[i].TotalCost = 0
		}
		return
	}
	var sum float64
	for i := range envBreakdown {
		if envBreakdown[i].TotalCost < 0 {
			envBreakdown[i].TotalCost = 0
		}
		sum += envBreakdown[i].TotalCost
	}
	if math.Abs(sum) < 1e-9 {
		// 聚合表无当期数据时 env 全 0，将总账归入第一项，保证 sum(env_breakdown)=总环境成本 [Ref: 用户需求]
		if targetTotal > 0 && len(envBreakdown) > 0 {
			envBreakdown[0].TotalCost = targetTotal
		}
		return
	}
	scale := targetTotal / sum
	for i := range envBreakdown {
		envBreakdown[i].TotalCost *= scale
	}
}

// filterEnvAccountConfigs 若 envs 非空则仅保留所选环境行，与 GetGlobalCost 筛选一致。[Ref: 03_Phase6/01_FinOps]
func filterEnvAccountConfigs(configs []postgres.EnvAccountConfig, envs []string) []postgres.EnvAccountConfig {
	if len(envs) == 0 {
		return configs
	}
	set := make(map[string]bool)
	for _, e := range envs {
		if e != "" {
			set[e] = true
		}
	}
	var out []postgres.EnvAccountConfig
	for i := range configs {
		if set[configs[i].Environment] {
			out = append(out, configs[i])
		}
	}
	return out
}

// aggregateHeroTotalsFromDedupedList 与 buildEnvBreakdownFromList 同键优先：每环境仅计一行，避免「占位 account_id + ETL 回写真实 ID」双行时 Hero/领域分解翻倍。[Ref: 03_Phase6/01_FinOps]
func aggregateHeroTotalsFromDedupedList(list []postgres.CloudBillAggregate, configs []postgres.EnvAccountConfig) (total float64, merged map[string]float64, err error) {
	merged = make(map[string]float64)
	if len(list) == 0 {
		return 0, merged, nil
	}
	curBy := make(map[string]float64)
	pbBy := make(map[string]map[string]float64)
	for _, a := range list {
		aid := strings.TrimSpace(a.AccountID)
		curBy[aid] += a.TotalAmount
		if pbBy[aid] == nil {
			pbBy[aid] = make(map[string]float64)
		}
		for k, v := range a.ProductBreakdown {
			pbBy[aid][k] += v
		}
	}
	if len(configs) == 0 {
		for _, a := range list {
			total += a.TotalAmount
			for k, v := range a.ProductBreakdown {
				merged[k] += v
			}
		}
		return total, merged, nil
	}
	for _, c := range configs {
		aid := strings.TrimSpace(c.AccountID)
		env := c.Environment
		var t float64
		var key string
		if v := curBy[aid]; v != 0 {
			t = v
			key = aid
		} else if v := curBy[env]; v != 0 {
			t = v
			key = env
		} else {
			t = curBy[aid]
			if t == 0 {
				t = curBy[env]
			}
			key = aid
			if key == "" {
				key = env
			}
		}
		total += t
		if m := pbBy[key]; m != nil {
			for k, v := range m {
				merged[k] += v
			}
		}
	}
	return total, merged, nil
}

// sumPerEnvFromAccountMap 与 aggregateHeroTotalsFromDedupedList 同键：每环境配置一行仅取 AccountID 或 Environment 占位键之一，避免月原始/应付/余额双行叠加。[Ref: 03_Phase6/01_FinOps 五维 P/U/B]
func sumPerEnvFromAccountMap(curBy map[string]float64, configs []postgres.EnvAccountConfig) float64 {
	var total float64
	if len(configs) == 0 {
		for _, v := range curBy {
			total += v
		}
		return total
	}
	for _, c := range configs {
		aid := strings.TrimSpace(c.AccountID)
		env := c.Environment
		var t float64
		if v := curBy[aid]; v != 0 {
			t = v
		} else if v := curBy[env]; v != 0 {
			t = v
		} else {
			t = curBy[aid]
			if t == 0 {
				t = curBy[env]
			}
		}
		total += t
	}
	return total
}

// sumMonthlyCashTotalDedupedForCycles 按账期拉月原始表现金列，再按环境配置去重汇总（与 Hero 消耗去重一致）。[Ref: 03_Phase6/01_FinOps]
func (s *CostService) sumMonthlyCashTotalDedupedForCycles(ctx context.Context, cycles []string, configs []postgres.EnvAccountConfig) (float64, error) {
	var sum float64
	for _, c := range cycles {
		list, err := s.repo.ListCloudBillMonthlyRawByCycle(ctx, c)
		if err != nil {
			return 0, err
		}
		if len(list) == 0 {
			continue
		}
		curBy := make(map[string]float64)
		for i := range list {
			aid := strings.TrimSpace(list[i].AccountID)
			t, _ := pickMonthlyDataCash(&list[i])
			curBy[aid] += t
		}
		sum += sumPerEnvFromAccountMap(curBy, configs)
	}
	return sum, nil
}

// sumOutstandingDedupedForCycles 按账期拉应付行后按环境去重汇总。[Ref: 03_Phase6/01_FinOps]
func (s *CostService) sumOutstandingDedupedForCycles(ctx context.Context, cycles []string, configs []postgres.EnvAccountConfig) (float64, error) {
	rows, err := s.repo.ListBillOutstandingInBillingCycles(ctx, cycles)
	if err != nil {
		return 0, err
	}
	byCycle := make(map[string]map[string]float64)
	for i := range rows {
		c := rows[i].BillingCycle
		if byCycle[c] == nil {
			byCycle[c] = make(map[string]float64)
		}
		aid := strings.TrimSpace(rows[i].AccountID)
		byCycle[c][aid] += rows[i].OutstandingAmount
	}
	var u float64
	for _, c := range cycles {
		cur := byCycle[c]
		if cur == nil {
			cur = make(map[string]float64)
		}
		u += sumPerEnvFromAccountMap(cur, configs)
	}
	return u, nil
}

// scaleDrilldownListToTotal 将云产品明细列表按 targetTotal 等比例缩放，使 sum(items.Cost)=targetTotal，与总环境成本一致。[Ref: 用户需求 云产品成本明细叠加=总环境成本]
func scaleDrilldownListToTotal(items []dto.EnvDrilldownItem, targetTotal float64) {
	if len(items) == 0 {
		return
	}
	if targetTotal <= 0 {
		for i := range items {
			items[i].Cost = 0
		}
		return
	}
	var sum float64
	for i := range items {
		sum += items[i].Cost
	}
	if math.Abs(sum) < 1e-9 {
		return
	}
	scale := targetTotal / sum
	for i := range items {
		items[i].Cost *= scale
	}
}

// topProductsForDomain 从 productBreakdown 中取 key 为 "domain:ProductCode" 的项，按绝对值降序取前 n 个。
// 含冲正/退款月份可能全为负值，仍需展示以帮助用户了解成本构成。[Ref: 01_成本透视真实数据]
func topProductsForDomain(pb *map[string]float64, domain string, n int) []dto.ProductCostItem {
	if pb == nil || n <= 0 {
		return nil
	}
	prefix := domain + ":"
	var list []dto.ProductCostItem
	for k, cost := range *pb {
		if !strings.HasPrefix(k, prefix) || cost == 0 {
			continue
		}
		product := strings.TrimPrefix(k, prefix)
		list = append(list, dto.ProductCostItem{Product: product, Cost: cost})
	}
	if len(list) == 0 {
		return nil
	}
	sort.Slice(list, func(i, j int) bool {
		return math.Abs(list[i].Cost) > math.Abs(list[j].Cost)
	})
	if len(list) > n {
		list = list[:n]
	}
	return list
}

// filterDailyRowsByAccountIDs 当 accountIDs 非空时仅保留其内 account 的行；nil 返回原切片。[Ref: 20260323_POC_UAT_账期锚点]
func filterDailyRowsByAccountIDs(rows []postgres.CloudBillDailyRaw, accountIDs map[string]bool) []postgres.CloudBillDailyRaw {
	if accountIDs == nil || len(accountIDs) == 0 {
		return rows
	}
	out := rows[:0]
	for _, r := range rows {
		if accountIDs[r.AccountID] {
			out = append(out, r)
		}
	}
	return out
}

// avgDailyConsumptionLastMonth 上月天粒度消耗汇总平均数，用于回调日替代；accountIDs 非空时仅统计该集合内行。[Ref: 用户需求、20260323_POC_UAT_账期锚点]
func (s *CostService) avgDailyConsumptionLastMonth(ctx context.Context, now time.Time, accountIDs map[string]bool) float64 {
	ref := billingCalendarRef(now)
	prevKey := billingcalendar.PreviousMonthYYYYMM(ref)
	t, err := time.Parse("2006-01", prevKey)
	if err != nil {
		return 0
	}
	first := time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, time.UTC)
	lastDay := first.AddDate(0, 1, -1)
	rows, err := s.repo.ListCloudBillDailyRawFromTo(ctx, first, lastDay, "")
	if err != nil || len(rows) == 0 {
		return 0
	}
	rows = filterDailyRowsByAccountIDs(rows, accountIDs)
	if len(rows) == 0 {
		return 0
	}
	daysInMonth := lastDay.Day()
	if daysInMonth <= 0 {
		return 0
	}
	var sum float64
	for _, r := range rows {
		sum += r.TotalAmount
	}
	return sum / float64(daysInMonth)
}

// aggregateCloudBillByPeriodWithAccountTotals 按时间范围聚合云账单并返回各 account 的金额，供 env_breakdown 按环境隔离展示。[Ref: 16_ §十三 POC/UAT 数据绝对隔离]
func (s *CostService) aggregateCloudBillByPeriodWithAccountTotals(ctx context.Context, period string, accountIDs map[string]bool) (ok bool, total float64, productBreakdown *map[string]float64, accountTotals map[string]float64, prevAccountTotals map[string]float64) {
	now := time.Now().UTC()
	mergeAcc := func(dest map[string]float64, add map[string]float64) {
		for k, v := range add {
			dest[k] += v
		}
	}
	switch period {
	case "last_month":
		ref := billingCalendarRef(now)
		firstOfCur := time.Date(ref.Year(), ref.Month(), 1, 0, 0, 0, 0, ref.Location())
		prevCycle := firstOfCur.AddDate(0, -1, 0).Format("2006-01")
		prevPrevCycle := firstOfCur.AddDate(0, -2, 0).Format("2006-01")
		t, pb, accT := s.mergeMonthlyRawByCycle(ctx, prevCycle, accountIDs, false)
		_, _, prevT := s.mergeMonthlyRawByCycle(ctx, prevPrevCycle, accountIDs, false)
		if t == 0 && len(pb) == 0 {
			return false, 0, nil, nil, nil
		}
		return true, t, &pb, accT, prevT
	case "last_quarter":
		curMonth := int(now.Month())
		curYear := now.Year()
		var prevQStartMonth, prevQYear int
		switch {
		case curMonth <= 3:
			prevQStartMonth, prevQYear = 10, curYear-1
		case curMonth <= 6:
			prevQStartMonth, prevQYear = 1, curYear
		case curMonth <= 9:
			prevQStartMonth, prevQYear = 4, curYear
		default:
			prevQStartMonth, prevQYear = 7, curYear
		}
		prevCycles := []string{
			fmt.Sprintf("%04d-%02d", prevQYear, prevQStartMonth),
			fmt.Sprintf("%04d-%02d", prevQYear, prevQStartMonth+1),
			fmt.Sprintf("%04d-%02d", prevQYear, prevQStartMonth+2),
		}
		accCur := make(map[string]float64)
		accPrev := make(map[string]float64)
		var total float64
		merged := make(map[string]float64)
		for _, cycle := range prevCycles {
			t, pb, accT := s.mergeMonthlyRawByCycle(ctx, cycle, accountIDs, false)
			total += t
			mergeAcc(accCur, accT)
			for k, v := range pb {
				merged[k] += v
			}
		}
		if total == 0 && len(merged) == 0 {
			return false, 0, nil, nil, nil
		}
		// 上期：上上季度
		prevQStart := prevQStartMonth - 3
		prevQY := prevQYear
		if prevQStart <= 0 {
			prevQStart += 12
			prevQY--
		}
		for m := 0; m < 3; m++ {
			mm := prevQStart + m
			if mm > 12 {
				mm -= 12
			}
			_, _, pt := s.mergeMonthlyRawByCycle(ctx, fmt.Sprintf("%04d-%02d", prevQY, mm), accountIDs, false)
			mergeAcc(accPrev, pt)
		}
		return true, total, &merged, accCur, accPrev
	case "last_year":
		lastYear := now.Year() - 1
		accCur := make(map[string]float64)
		accPrev := make(map[string]float64)
		var total float64
		merged := make(map[string]float64)
		for m := 1; m <= 12; m++ {
			cycle := fmt.Sprintf("%04d-%02d", lastYear, m)
			t, pb, accT := s.mergeMonthlyRawByCycle(ctx, cycle, accountIDs, false)
			total += t
			mergeAcc(accCur, accT)
			for k, v := range pb {
				merged[k] += v
			}
		}
		if total == 0 && len(merged) == 0 {
			return false, 0, nil, nil, nil
		}
		for m := 1; m <= 12; m++ {
			_, _, pt := s.mergeMonthlyRawByCycle(ctx, fmt.Sprintf("%04d-%02d", lastYear-1, m), accountIDs, false)
			mergeAcc(accPrev, pt)
		}
		return true, total, &merged, accCur, accPrev
	case "this_year":
		thisYear := now.Year()
		currentMonth := int(now.Month())
		accCur := make(map[string]float64)
		accPrev := make(map[string]float64)
		var total float64
		merged := make(map[string]float64)
		for m := 1; m < currentMonth; m++ {
			cycle := fmt.Sprintf("%04d-%02d", thisYear, m)
			t, pb, accT := s.mergeMonthlyRawByCycle(ctx, cycle, accountIDs, false)
			total += t
			mergeAcc(accCur, accT)
			for k, v := range pb {
				merged[k] += v
			}
		}
		yesterday := now.AddDate(0, 0, -1).Truncate(24 * time.Hour)
		firstOfCurrentMonth := time.Date(thisYear, time.Month(currentMonth), 1, 0, 0, 0, 0, time.UTC)
		lastMonthAvg := s.avgDailyConsumptionLastMonth(ctx, now, accountIDs)
		rows, _ := s.repo.ListCloudBillDailyRawFromTo(ctx, firstOfCurrentMonth, yesterday, "")
		rows = filterDailyRowsByAccountIDs(rows, accountIDs)
		if len(rows) > 0 {
			curTotal, curMerged := mergeDailyRowsSafe(rows, lastMonthAvg)
			total += curTotal
			for k, v := range curMerged {
				merged[k] += v
			}
			for _, r := range rows {
				accCur[r.AccountID] += r.TotalAmount
			}
		}
		if total == 0 && len(merged) == 0 {
			return false, 0, nil, nil, nil
		}
		return true, total, &merged, accCur, accPrev
	case "quarter":
		curQ := (int(now.Month())-1)/3 + 1
		qStartMonth := (curQ-1)*3 + 1
		accCur := make(map[string]float64)
		accPrev := make(map[string]float64)
		lastMonthAvg := s.avgDailyConsumptionLastMonth(ctx, now, accountIDs)
		var total float64
		merged := make(map[string]float64)
		for m := 0; m < 3; m++ {
			cycle := fmt.Sprintf("%04d-%02d", now.Year(), qStartMonth+m)
			if cycle == now.Format("2006-01") {
				firstOfMonth := time.Date(now.Year(), time.Month(qStartMonth+m), 1, 0, 0, 0, 0, time.UTC)
				yesterday := now.AddDate(0, 0, -1).Truncate(24 * time.Hour)
				rows, _ := s.repo.ListCloudBillDailyRawFromTo(ctx, firstOfMonth, yesterday, "")
				rows = filterDailyRowsByAccountIDs(rows, accountIDs)
				if len(rows) > 0 {
					t, pb := mergeDailyRowsWithCallbackReplacement(rows, lastMonthAvg)
					total += t
					for k, v := range pb {
						merged[k] += v
					}
					for _, r := range rows {
						accCur[r.AccountID] += r.TotalAmount
					}
				}
			} else if t, pb, accT := s.mergeMonthlyRawByCycle(ctx, cycle, accountIDs, false); t != 0 || len(pb) > 0 {
				total += t
				mergeAcc(accCur, accT)
				for k, v := range pb {
					merged[k] += v
				}
			}
		}
		if total != 0 || len(merged) > 0 {
			return true, total, &merged, accCur, accPrev
		}
		return false, 0, nil, nil, nil
	case "month", "":
		firstOfMonth := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
		yesterday := now.AddDate(0, 0, -1).Truncate(24 * time.Hour)
		lastMonthAvg := s.avgDailyConsumptionLastMonth(ctx, now, accountIDs)
		rows, err := s.repo.ListCloudBillDailyRawFromTo(ctx, firstOfMonth, yesterday, "")
		if err != nil || len(rows) == 0 {
			return false, 0, nil, nil, nil
		}
		rows = filterDailyRowsByAccountIDs(rows, accountIDs)
		accCur := make(map[string]float64)
		for _, r := range rows {
			accCur[r.AccountID] += r.TotalAmount
		}
		total, merged := mergeDailyRowsSafe(rows, lastMonthAvg)
		if total != 0 || len(merged) > 0 {
			return true, total, &merged, accCur, nil
		}
		return false, 0, nil, nil, nil
	default:
		return false, 0, nil, nil, nil
	}
}

// aggregateCloudBillByPeriod 按时间范围聚合云账单；accountIDs 非空时仅汇总该集合内 account（POC/UAT 隔离）。[Ref: 用户需求、20260323_POC_UAT_账期锚点]
// 返回 (有云账单, 总金额, 领域分项)；无数据时 (false, 0, nil)。
func (s *CostService) aggregateCloudBillByPeriod(ctx context.Context, period string, accountIDs map[string]bool) (bool, float64, *map[string]float64) {
	now := time.Now().UTC()
	switch period {
	case "quarter":
		// [Ref: 用户需求] 整月用 API 月汇总（现金）；当月未结用天消耗+回调日用上月日均替代
		curQ := (int(now.Month())-1)/3 + 1
		qStartMonth := (curQ-1)*3 + 1
		lastMonthAvg := s.avgDailyConsumptionLastMonth(ctx, now, accountIDs)
		var total float64
		merged := make(map[string]float64)
		for m := 0; m < 3; m++ {
			cycle := fmt.Sprintf("%04d-%02d", now.Year(), qStartMonth+m)
			if cycle == now.Format("2006-01") {
				firstOfMonth := time.Date(now.Year(), time.Month(qStartMonth+m), 1, 0, 0, 0, 0, time.UTC)
				yesterday := now.AddDate(0, 0, -1).Truncate(24 * time.Hour)
				rows, _ := s.repo.ListCloudBillDailyRawFromTo(ctx, firstOfMonth, yesterday, "")
				rows = filterDailyRowsByAccountIDs(rows, accountIDs)
				if len(rows) > 0 {
					t, pb := mergeDailyRowsWithCallbackReplacement(rows, lastMonthAvg)
					total += t
					for k, v := range pb {
						merged[k] += v
					}
				}
			} else if t, pb, _ := s.mergeMonthlyRawByCycle(ctx, cycle, accountIDs, false); t != 0 || len(pb) > 0 {
				total += t
				for k, v := range pb {
					merged[k] += v
				}
			}
		}
		if total != 0 || len(merged) > 0 {
			return true, total, &merged
		}
		return false, 0, nil
	case "last_month":
		// [Ref: 用户需求] 上月=整月，只用月粒度现金/已付；多账户汇总 [Ref: 01_多环境 UAT]
		prevCycle := billingcalendar.PreviousMonthYYYYMM(billingCalendarRef(now))
		t, pb, _ := s.mergeMonthlyRawByCycle(ctx, prevCycle, accountIDs, false)
		if t == 0 && len(pb) == 0 {
			return false, 0, nil
		}
		return true, t, &pb
	case "last_quarter":
		// [Ref: 用户需求] 上季度=整月叠加，只用月粒度现金/已付，绝对准确
		curMonth := int(now.Month())
		curYear := now.Year()
		var prevQStartMonth, prevQYear int
		switch {
		case curMonth <= 3:
			prevQStartMonth, prevQYear = 10, curYear-1
		case curMonth <= 6:
			prevQStartMonth, prevQYear = 1, curYear
		case curMonth <= 9:
			prevQStartMonth, prevQYear = 4, curYear
		default:
			prevQStartMonth, prevQYear = 7, curYear
		}
		prevCycles := []string{
			fmt.Sprintf("%04d-%02d", prevQYear, prevQStartMonth),
			fmt.Sprintf("%04d-%02d", prevQYear, prevQStartMonth+1),
			fmt.Sprintf("%04d-%02d", prevQYear, prevQStartMonth+2),
		}
		var total float64
		merged := make(map[string]float64)
		for _, cycle := range prevCycles {
			t, pb, _ := s.mergeMonthlyRawByCycle(ctx, cycle, accountIDs, false)
			total += t
			for k, v := range pb {
				merged[k] += v
			}
		}
		if total != 0 || len(merged) > 0 {
			return true, total, &merged
		}
		return false, 0, nil
	case "this_year":
		// [Ref: 用户需求] 已完成月用月粒度现金/已付；当月用天叠+消耗+回调日替代（支出参考）
		thisYear := now.Year()
		currentMonth := int(now.Month())
		var total float64
		merged := make(map[string]float64)
		for m := 1; m < currentMonth; m++ {
			cycle := fmt.Sprintf("%04d-%02d", thisYear, m)
			t, pb, _ := s.mergeMonthlyRawByCycle(ctx, cycle, accountIDs, false)
			total += t
			for k, v := range pb {
				merged[k] += v
			}
		}
		yesterday := now.AddDate(0, 0, -1).Truncate(24 * time.Hour)
		firstOfCurrentMonth := time.Date(thisYear, time.Month(currentMonth), 1, 0, 0, 0, 0, time.UTC)
		lastMonthAvg := s.avgDailyConsumptionLastMonth(ctx, now, accountIDs)
		rows, _ := s.repo.ListCloudBillDailyRawFromTo(ctx, firstOfCurrentMonth, yesterday, "")
		rows = filterDailyRowsByAccountIDs(rows, accountIDs)
		if len(rows) > 0 {
			curTotal, curMerged := mergeDailyRowsSafe(rows, lastMonthAvg)
			total += curTotal
			for k, v := range curMerged {
				merged[k] += v
			}
		}
		if total == 0 && len(merged) == 0 {
			return false, 0, nil
		}
		return true, total, &merged
	case "last_year":
		// [Ref: 用户需求] 去年=整月叠加，只用月粒度现金/已付，绝对准确
		lastYear := now.Year() - 1
		var total float64
		merged := make(map[string]float64)
		for m := 1; m <= 12; m++ {
			cycle := fmt.Sprintf("%04d-%02d", lastYear, m)
			t, pb, _ := s.mergeMonthlyRawByCycle(ctx, cycle, accountIDs, false)
			total += t
			for k, v := range pb {
				merged[k] += v
			}
		}
		if total == 0 && len(merged) == 0 {
			return false, 0, nil
		}
		return true, total, &merged
	case "month", "":
		// [Ref: 用户需求] 本月=仅天叠，天消耗+回调日用上月天粒度消耗日均替代
		yesterday := now.AddDate(0, 0, -1).Truncate(24 * time.Hour)
		firstOfMonth := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
		lastMonthAvg := s.avgDailyConsumptionLastMonth(ctx, now, accountIDs)
		if rows, err := s.repo.ListCloudBillDailyRawFromTo(ctx, firstOfMonth, yesterday, ""); err == nil && len(rows) > 0 {
			rows = filterDailyRowsByAccountIDs(rows, accountIDs)
			total, merged := mergeDailyRowsSafe(rows, lastMonthAvg)
			if total != 0 || len(merged) > 0 {
				return true, total, &merged
			}
		}
		return false, 0, nil
	default:
		return false, 0, nil
	}
}

// GetGlobalCostByDateRange 按月份范围从月原始表叠加，支持最多 5 年（60 个月）。[Ref: 01_实践 月源数据保留近5年]
// [Ref: D8-7] 相同日期范围结果短时缓存 1h，重复请求直接命中。
// track：FinOps 双轨，与 global 一致。[Ref: 03_Phase6/01_FinOps双轨与全域成本重构/01_设计 §API、track 与 UX]
// project_ids 非空时展开为项目成员环境并忽略 URL envs（与 GetGlobalCost 一致）。[Ref: 03_Phase6/03_前端全域成本透视/01_设计]
// envs 非空时 Hero/域分解/ledger 与聚合表路径一致按环境过滤；环境卡与环比分母为全量账户（与 GetGlobalCost 聚合分支一致）。[Ref: 03_Phase6/03_前端全域成本透视/01_设计]
func (s *CostService) GetGlobalCostByDateRange(ctx context.Context, from, to time.Time, track string, projectIDs []int, envs []string) (*dto.GlobalCostResponse, error) {
	s.runFinOpsAuxiliarySyncIfRequested(ctx)
	const maxMonths = 60
	if to.Before(from) {
		from, to = to, from
	}
	envsEff, err := s.effectiveEnvsForGlobalCost(ctx, envs, projectIDs)
	if err != nil {
		return nil, err
	}
	fromCycle := from.Format("2006-01")
	toCycle := to.Format("2006-01")
	key := fromCycle + ":" + toCycle + ":" + track + ":" + projectIDsCacheKey(projectIDs) + ":" + envsCacheKey(envsEff)
	now := time.Now().UTC()
	s.dateRangeCacheMu.RLock()
	if e, ok := s.dateRangeCache[key]; ok && e.exp.After(now) {
		s.dateRangeCacheMu.RUnlock()
		return e.resp, nil
	}
	s.dateRangeCacheMu.RUnlock()

	accountIDsAll := s.accountIDsFromAllConfig(ctx)
	var accountIDsHero map[string]bool
	if len(envsEff) > 0 {
		// 宽松 map 会把同一环境的 account_id 行与 environment 占位行同时纳入 Hero，与 env 卡「每环境一行」不一致。[Ref: accountIDsForEnvsStrict]
		accountIDsHero = s.accountIDsForEnvsStrict(ctx, envsEff)
	}

	var total float64
	merged := make(map[string]float64)
	curAccountTotals := make(map[string]float64)
	var hasAnyMonthlyRow bool

	cur := time.Date(from.Year(), from.Month(), 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(to.Year(), to.Month(), 1, 0, 0, 0, 0, time.UTC)
	count := 0
	useConsumption := track == "technical"
	for !cur.After(end) && count < maxMonths {
		cycle := cur.Format("2006-01")
		tFull, _, accFull := s.mergeMonthlyRawByCycle(ctx, cycle, accountIDsAll, useConsumption)
		if tFull != 0 || len(accFull) > 0 {
			hasAnyMonthlyRow = true
		}
		for acc, amt := range accFull {
			curAccountTotals[acc] += amt
		}
		var tHero float64
		var pbHero map[string]float64
		if len(envsEff) > 0 {
			tHero, pbHero, _ = s.mergeMonthlyRawByCycle(ctx, cycle, accountIDsHero, useConsumption)
		} else {
			tHero, pbHero, _ = s.mergeMonthlyRawByCycle(ctx, cycle, accountIDsAll, useConsumption)
		}
		total += tHero
		for k, v := range pbHero {
			merged[k] += v
		}
		cur = cur.AddDate(0, 1, 0)
		count++
	}
	if !hasAnyMonthlyRow {
		return nil, nil
	}
	scaleMergedToTotal(merged, total)
	domainBreakdown := buildDomainBreakdownNormalized(merged)
	scaleDomainBreakdownToTotal(domainBreakdown, total)
	// [Ref: 01_实践 自定义月] 上一周期用于环比；单月则上一周期=上月，多月则上一周期=等长前一段
	prevAccountTotals := make(map[string]float64)
	prevStart := from.AddDate(0, -1*count, 0)
	prevEnd := from.AddDate(0, -1, 0)
	pcur := time.Date(prevStart.Year(), prevStart.Month(), 1, 0, 0, 0, 0, time.UTC)
	pend := time.Date(prevEnd.Year(), prevEnd.Month(), 1, 0, 0, 0, 0, time.UTC)
	pcnt := 0
	for !pcur.After(pend) && pcnt < maxMonths {
		cycle := pcur.Format("2006-01")
		_, _, accTotals := s.mergeMonthlyRawByCycle(ctx, cycle, accountIDsAll, useConsumption)
		for acc, amt := range accTotals {
			prevAccountTotals[acc] += amt
		}
		pcur = pcur.AddDate(0, 1, 0)
		pcnt++
	}
	envBreakdown := s.buildEnvBreakdownFromAccountTotals(ctx, curAccountTotals, prevAccountTotals)
	// 有环境/项目筛选时环境卡保持全量、仅 Hero 为筛选后合计（与聚合表路径一致）。[Ref: 用户需求 环境卡片与筛选解耦]
	if len(envsEff) == 0 {
		scaleEnvBreakdownToTotal(envBreakdown, total)
	}
	if track == "finance" && len(envBreakdown) > 0 {
		s.enrichEnvBreakdownConsumptionFromMonthlyRawRange(ctx, envBreakdown, from, to, accountIDsAll)
	}
	// [Ref: 用户需求] 自定义月范围现金合计为负时展示为 0 并注明净退款已抵减
	displayTotal := total
	periodKeyDR := fromCycle + ":" + toCycle
	var meta *dto.GlobalCostMetadata
	if total < 0 {
		displayTotal = 0
		for i := range domainBreakdown {
			domainBreakdown[i].Cost = 0
		}
		for i := range envBreakdown {
			if envBreakdown[i].TotalCost < 0 {
				envBreakdown[i].TotalCost = 0
			}
		}
		meta = &dto.GlobalCostMetadata{DataStatus: "fallback", DisplayNote: "该周期净退款已抵减", ReportType: "date_range", PeriodKey: periodKeyDR}
	} else {
		meta = &dto.GlobalCostMetadata{DataStatus: "month_raw_range", ReportType: "date_range", PeriodKey: periodKeyDR}
	}
	resp := &dto.GlobalCostResponse{
		TotalCost:        displayTotal,
		TotalOptimizable: 0,
		GlobalEfficiency: 0,
		DomainBreakdown:  domainBreakdown,
		EnvBreakdown:     envBreakdown,
		Namespaces:       nil,
		Timestamp:        now,
		Metadata:         meta,
	}
	dto.ApplyFinOpsGlobalMetadata(resp, track)
	projects, _ := s.repo.ListCostProjects(ctx)
	configsPB, _ := s.repo.ListEnvAccountConfig(ctx)
	if len(projects) > 0 && len(configsPB) > 0 {
		resp.ProjectBreakdown = buildProjectBreakdownFromAccountTotalsMap(projects, configsPB, curAccountTotals)
	}
	idsLedger := accountIDsToSlice(len(envsEff) > 0, s.accountIDsForEnvsStrict(ctx, envsEff), s.accountIDsFromAllConfig(ctx))
	s.enrichFinOpsLedger(ctx, resp, track, "date_range", periodKeyDR, now, idsLedger, envsEff)
	// 资金轨 + 环境筛选：Hero 合计已为 displayTotal，但 enrichFinOpsLedger 后 ledger.P 可能仍为 0；前端 finance 以 ledger.P 为主数字须与 total_cost 一致。[Ref: 03_Phase6/01_FinOps]
	if track == "finance" && len(envsEff) > 0 && resp.Ledger != nil && math.Abs(displayTotal) > 1e-9 {
		p := displayTotal
		if resp.Ledger.P == nil || (resp.Ledger.P != nil && math.Abs(*resp.Ledger.P) < 1e-9) {
			resp.Ledger.P = &p
		}
	}
	s.finalizeGlobalCostResponse(ctx, resp)
	s.dateRangeCacheMu.Lock()
	s.dateRangeCache[key] = dateRangeCacheEntry{resp: resp, exp: now.Add(s.dateRangeCacheTTL)}
	s.dateRangeCacheMu.Unlock()
	return resp, nil
}

// resolveBillDataStatus 根据 period 类型和时间推断账单对账状态，用于前端三段式状态标识。
// [Ref: 16_云账单动态对账与高可靠处理规范 §三段式聚合策略]
//   - 历史月（periodKey 为上月或更早的 YYYY-MM）→ 从 month_status 表读真实状态
//   - 当前月 / 1d / this_week → "PRELIMINARY"（前端展示「动态同步」）
func (s *CostService) resolveBillDataStatus(ctx context.Context, reportType, periodKey string, now time.Time) string {
	currentMonth := billingCalendarRef(now).Format("2006-01")
	switch reportType {
	case "month":
		if periodKey == currentMonth {
			return "PRELIMINARY"
		}
		// 历史月：查 month_status 表
		if ms, err := s.repo.GetCloudBillMonthStatus(ctx, periodKey, ""); err == nil && ms != nil {
			return ms.DataStatus
		}
		return "FINALIZED" // 无 month_status 记录时，历史月默认视为已结算
	default:
		// quarter/this_year 等跨月周期：任一月仍为 PRELIMINARY 则整体 PRELIMINARY
		return "PRELIMINARY"
	}
}

// mergeDailyRowsWithCallbackReplacement 聚合日原始行（消耗口径）；某日 total_amount < 0 视为回调日。
// 若 lastMonthAvg > 0 则用上月天粒度消耗汇总平均数替代回调日，否则用当前周期内正常日日均替代。[Ref: 用户需求 回调导致天汇总<0则按上月天粒度消耗汇总平均数代替]
func mergeDailyRowsWithCallbackReplacement(rows []postgres.CloudBillDailyRaw, lastMonthAvg float64) (float64, map[string]float64) {
	if len(rows) == 0 {
		return 0, nil
	}
	var normalRows []postgres.CloudBillDailyRaw
	for _, r := range rows {
		if r.TotalAmount >= 0 {
			normalRows = append(normalRows, r)
		}
	}
	if len(normalRows) == 0 {
		return 0, make(map[string]float64)
	}
	var sumTotal float64
	avgPB := make(map[string]float64)
	for _, r := range normalRows {
		sumTotal += r.TotalAmount
		for k, v := range r.ProductBreakdown {
			avgPB[k] += v
		}
	}
	nNormal := float64(len(normalRows))
	avgTotal := sumTotal / nNormal
	for k := range avgPB {
		avgPB[k] /= nNormal
	}
	var sumPB float64
	for _, v := range avgPB {
		sumPB += v
	}
	if sumPB != 0 && math.Abs(avgTotal) > 1e-9 {
		scale := avgTotal / sumPB
		for k := range avgPB {
			avgPB[k] *= scale
		}
	}
	nCallback := len(rows) - len(normalRows)
	// 回调日替代：优先用上月天粒度消耗日均；否则用当前正常日日均
	replaceAvg := avgTotal
	if lastMonthAvg > 0 {
		replaceAvg = lastMonthAvg
	}
	total := sumTotal + replaceAvg*float64(nCallback)
	merged := make(map[string]float64)
	for _, r := range normalRows {
		for k, v := range r.ProductBreakdown {
			merged[k] += v
		}
	}
	// 回调日贡献按当前正常日分类比例分配
	if replaceAvg > 0 && math.Abs(avgTotal) > 1e-9 {
		ratio := replaceAvg / avgTotal
		for k, v := range avgPB {
			merged[k] += v * ratio * float64(nCallback)
		}
	} else {
		for k, v := range avgPB {
			merged[k] += v * float64(nCallback)
		}
	}
	return total, merged
}

// mergeDailyRowsSafe 聚合日原始行；已统一为消耗口径，含回调日替代（无上月日均时用当前正常日日均）。[Ref: 16_ §三；用户确认 消耗口径]
func mergeDailyRowsSafe(rows []postgres.CloudBillDailyRaw, lastMonthAvg float64) (float64, map[string]float64) {
	return mergeDailyRowsWithCallbackReplacement(rows, lastMonthAvg)
}

// pickMonthlyData 月表消耗口径（TotalAmount/ProductBreakdown），仅用于天叠部分的参考。[Ref: 天粒度可用消耗]
func pickMonthlyData(mon *postgres.CloudBillMonthlyRaw) (total float64, pb map[string]float64) {
	if mon == nil {
		return 0, nil
	}
	return mon.TotalAmount, mon.ProductBreakdown
}

// pickMonthlyDataCash 月表现金/已付口径（CashTotalAmount/CashProductBreakdown），用于整月及整月叠加的绝对准确数据。[Ref: 用户需求 月粒度只算现金支付或已付，不被回调恶心]
// [Ref: 用户反馈 2025-10/11 控制台有正数但 Lighthouse 显示 $0] 当 cash=0 且 total 有值时，fallback 到 PretaxAmount，避免 API 未返回 PaymentAmount 时完全空白。
func pickMonthlyDataCash(mon *postgres.CloudBillMonthlyRaw) (total float64, pb map[string]float64) {
	if mon == nil {
		return 0, nil
	}
	total = mon.CashTotalAmount
	pb = mon.CashProductBreakdown
	if pb == nil {
		pb = make(map[string]float64)
	}
	if total == 0 && len(pb) == 0 && (mon.TotalAmount != 0 || len(mon.ProductBreakdown) > 0) {
		total = mon.TotalAmount
		pb = mon.ProductBreakdown
		if pb == nil {
			pb = make(map[string]float64)
		}
	}
	return total, pb
}

// mergeMonthlyRawByCycle 汇总指定账期下月原始行；accountIDs 非空时仅汇总该集合内 account，保证 POC/UAT 绝对隔离。[Ref: 01_多环境 UAT、20260323_POC_UAT_账期锚点]
// useConsumption 为 true 时用 TotalAmount/ProductBreakdown（技术消耗）；否则用现金/已付口径，与历史默认一致。[Ref: 03_Phase6/01_FinOps]
func (s *CostService) mergeMonthlyRawByCycle(ctx context.Context, billingCycle string, accountIDs map[string]bool, useConsumption bool) (total float64, breakdown map[string]float64, accountTotals map[string]float64) {
	list, err := s.repo.ListCloudBillMonthlyRawByCycle(ctx, billingCycle)
	if err != nil || len(list) == 0 {
		return 0, nil, nil
	}
	breakdown = make(map[string]float64)
	accountTotals = make(map[string]float64)
	for i := range list {
		acc := list[i].AccountID
		if accountIDs != nil {
			// 严格过滤：仅保留 accountIDs 中的 account；空串为单账号占位，无 accountIDs[""] 则排除
			if !accountIDs[acc] {
				continue
			}
		}
		var t float64
		var pb map[string]float64
		if useConsumption {
			t, pb = pickMonthlyData(&list[i])
		} else {
			t, pb = pickMonthlyDataCash(&list[i])
		}
		total += t
		accountTotals[acc] += t
		for k, v := range pb {
			breakdown[k] += v
		}
	}
	return total, breakdown, accountTotals
}

// ErrFallbackTimeout D1-4：降级从原始表聚合时查询超时，调用方应返回 503。
var ErrFallbackTimeout = errors.New("cost fallback query timeout")

// FallbackQueryTimeout D1-4：降级路径下从日原始表聚合的查询超时时间。
const FallbackQueryTimeout = 5 * time.Second

// billingLocation 账单「自然月」锚点：与 pkg/billingcalendar.Location 一致（无 zoneinfo 时 UTC+8）。[Ref: 用户需求]
func billingLocation() *time.Location {
	return billingcalendar.Location()
}

// billingCalendarRef 将任意时刻映射为业务日历下的本地时间，用于本月/上月/本季等 YYYY-MM。[Ref: 用户需求 北京时间]
func billingCalendarRef(now time.Time) time.Time {
	return now.In(billingLocation())
}

// tryGetGlobalCostByMonth 单月 GetGlobalCostByDateRange，供 last_month 月原始回退。[Ref: 03_Phase6/01_FinOps]
func (s *CostService) tryGetGlobalCostByMonth(ctx context.Context, yyyyMM string, track string, projectIDs []int, envs []string) (*dto.GlobalCostResponse, error) {
	t, err := time.Parse("2006-01", yyyyMM)
	if err != nil {
		return nil, nil
	}
	from := time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, time.UTC)
	return s.GetGlobalCostByDateRange(ctx, from, from, track, projectIDs, envs)
}

// resolveLastMonthFromMonthlyRaw period=last_month 时走月原始表 Cash；「上月」账期为 **北京时间** 自然月上一月，与 reportTypeAndPeriodKey 一致。[Ref: 用户需求]
func (s *CostService) resolveLastMonthFromMonthlyRaw(ctx context.Context, now time.Time, track string, projectIDs []int, envs []string) (*dto.GlobalCostResponse, error) {
	prev := billingcalendar.PreviousMonthYYYYMM(billingCalendarRef(now))
	return s.tryGetGlobalCostByMonth(ctx, prev, track, projectIDs, envs)
}

// reportTypeAndPeriodKey 返回常规 period 对应的聚合表 (report_type, period_key)。[Ref: 04_01_成本透视真实数据 展示与延迟说明、01_设计 report_type 与 period_key]
// 自然月/季/年 YYYY-MM 锚点统一为 **Asia/Shanghai（北京时间）**；now 可为 UTC 瞬时，先经 billingCalendarRef 再算日历。[Ref: 用户需求]
func reportTypeAndPeriodKey(period string, now time.Time) (reportType, periodKey string) {
	ref := billingCalendarRef(now)
	month := ref.Format("2006-01")
	prevMonth := billingcalendar.PreviousMonthYYYYMM(ref)
	q := (int(ref.Month())-1)/3 + 1
	quarter := fmt.Sprintf("%s-Q%d", ref.Format("2006"), q)
	prevQ := q - 1
	prevY := ref.Year()
	if prevQ <= 0 {
		prevQ = 4
		prevY--
	}
	prevQuarter := fmt.Sprintf("%d-Q%d", prevY, prevQ)
	switch period {
	case "month", "":
		return "month", month
	case "last_month":
		return "last_month", prevMonth
	case "quarter":
		return "quarter", quarter
	case "last_quarter":
		return "last_quarter", prevQuarter
	case "this_year":
		// 今年：当年1月1日至昨日（按日累加，T+1 精度）
		return "this_year", ref.Format("2006")
	case "last_year":
		return "last_year", ref.AddDate(-1, 0, 0).Format("2006")
	default:
		return "", ""
	}
}

// periodKeyToDateRange 将 (reportType, periodKey) 转为 bill_date 区间，供 line_items 聚合 C/G。[Ref: 03_Phase6/01_FinOps ledger 填充]
func periodKeyToDateRange(reportType, periodKey string, now time.Time) (from, to time.Time) {
	yesterday := now.AddDate(0, 0, -1).Truncate(24 * time.Hour)
	switch reportType {
	case "month":
		t, _ := time.Parse("2006-01", periodKey)
		from = time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, time.UTC)
		endOfMonth := from.AddDate(0, 1, -1)
		if endOfMonth.After(yesterday) {
			to = yesterday
		} else {
			to = endOfMonth
		}
	case "last_month":
		t, _ := time.Parse("2006-01", periodKey)
		from = time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, time.UTC)
		to = from.AddDate(0, 1, -1)
	case "quarter", "last_quarter":
		var y, q int
		_, _ = fmt.Sscanf(periodKey, "%d-Q%d", &y, &q)
		from = time.Date(y, time.Month((q-1)*3+1), 1, 0, 0, 0, 0, time.UTC)
		to = from.AddDate(0, 3, -1)
		if to.After(yesterday) {
			to = yesterday
		}
	case "this_year":
		var y int
		fmt.Sscanf(periodKey, "%d", &y)
		from = time.Date(y, 1, 1, 0, 0, 0, 0, time.UTC)
		to = yesterday
		if from.After(to) {
			to = from
		}
	case "last_year":
		var y int
		fmt.Sscanf(periodKey, "%d", &y)
		from = time.Date(y, 1, 1, 0, 0, 0, 0, time.UTC)
		to = time.Date(y, 12, 31, 0, 0, 0, 0, time.UTC)
	case "date_range":
		// periodKey "2025-01:2025-03" 与 GetGlobalCostByDateRange 一致，供 enrichFinOpsLedger / P U B 区间 [Ref: 03_Phase6/01_FinOps]
		parts := strings.Split(periodKey, ":")
		if len(parts) != 2 {
			return time.Time{}, time.Time{}
		}
		t0, e0 := time.Parse("2006-01", parts[0])
		t1, e1 := time.Parse("2006-01", parts[1])
		if e0 != nil || e1 != nil {
			return time.Time{}, time.Time{}
		}
		from = time.Date(t0.Year(), t0.Month(), 1, 0, 0, 0, 0, time.UTC)
		endMonth := time.Date(t1.Year(), t1.Month(), 1, 0, 0, 0, 0, time.UTC)
		endOfRange := endMonth.AddDate(0, 1, -1)
		if endOfRange.After(yesterday) {
			to = yesterday
		} else {
			to = endOfRange
		}
	default:
		return time.Time{}, time.Time{}
	}
	return from, to
}

// ledgerCanonicalAccountIDs 按 ListEnvAccountConfig 顺序返回「账号线」（AccountID 优先，否则 Environment），与 env_breakdown 一致，避免 accountIDsSlice 中重复键导致双计。[Ref: 03_Phase6/01_FinOps]
func (s *CostService) ledgerCanonicalAccountIDs(ctx context.Context, envFilter []string) []string {
	configs, err := s.repo.ListEnvAccountConfig(ctx)
	if err != nil || len(configs) == 0 {
		return nil
	}
	envSet := make(map[string]bool)
	if len(envFilter) == 0 {
		for i := range configs {
			envSet[configs[i].Environment] = true
		}
	} else {
		for _, e := range envFilter {
			if e != "" {
				envSet[e] = true
			}
		}
	}
	var out []string
	for i := range configs {
		c := configs[i]
		if !envSet[c.Environment] {
			continue
		}
		id := c.AccountID
		if id == "" {
			id = c.Environment
		}
		if id != "" {
			out = append(out, id)
		}
	}
	return out
}

// buildAccountToEnvMap account_id（账号线）→ environment。[Ref: 03_Phase6/01_FinOps]
func (s *CostService) buildAccountToEnvMap(ctx context.Context) map[string]string {
	configs, err := s.repo.ListEnvAccountConfig(ctx)
	if err != nil || len(configs) == 0 {
		return nil
	}
	m := make(map[string]string)
	for i := range configs {
		c := configs[i]
		id := c.AccountID
		if id == "" {
			id = c.Environment
		}
		if id != "" {
			m[id] = c.Environment
		}
	}
	return m
}

// finOpsCGSourceForEnv 返回某环境的 C/G 源（oss|api）。[Ref: 03_Phase6/01_FinOps]
func (s *CostService) finOpsCGSourceForEnv(env string) string {
	e := strings.ToUpper(strings.TrimSpace(env))
	if s.finOpsCGSourceByEnv != nil {
		if v, ok := s.finOpsCGSourceByEnv[e]; ok && v != "" {
			return v
		}
	}
	return s.finOpsCGSource
}

// finOpsCGSourceForAccount 按 cost_env_account_config 将 account_id 映射到环境再解析覆盖。[Ref: 03_Phase6/01_FinOps]
func (s *CostService) finOpsCGSourceForAccount(accountID string, accountToEnv map[string]string) string {
	if accountToEnv != nil {
		if env, ok := accountToEnv[accountID]; ok && env != "" {
			return s.finOpsCGSourceForEnv(env)
		}
	}
	return s.finOpsCGSource
}

// finOpsCGPretaxForAccount 单账户 C/G。FINOPS_CG_SOURCE=api 时仅汇总 api_query_account_bill（行级 Pretax 已按 QueryAccountBill 目录−优惠−券口径）；否则 finops_billing_fact 优先，无则 oss_detail。[Ref: 03_Phase6/01_FinOps、04_采集 §5.4]
func (s *CostService) finOpsCGPretaxForAccount(ctx context.Context, from, to time.Time, accountID string, accountToEnv map[string]string) (c, g float64, err error) {
	src := strings.ToLower(strings.TrimSpace(s.finOpsCGSourceForAccount(accountID, accountToEnv)))
	if src == "" {
		src = "oss"
	}
	if src == "api" {
		return s.repo.SumLineItemsPretaxCGByDateRangeWithChannel(ctx, from, to, []string{accountID}, "api_query_account_bill")
	}
	nAcc, err := s.repo.CountFinOpsBillingFactsInDateRange(ctx, from, to, []string{accountID})
	if err != nil {
		return 0, 0, err
	}
	if nAcc > 0 {
		return s.repo.SumFinOpsFactPretaxCGByDateRange(ctx, from, to, []string{accountID})
	}
	return s.repo.SumLineItemsPretaxCGByDateRangeWithChannel(ctx, from, to, []string{accountID}, "oss_detail")
}

// billingMonthsInRange 将 [from,to] 覆盖的公历月份（YYYY-MM）列出，供 U（账期 outstanding）汇总。[Ref: 03_Phase6/01_FinOps]
func billingMonthsInRange(from, to time.Time) []string {
	from = from.UTC().Truncate(24 * time.Hour)
	to = to.UTC().Truncate(24 * time.Hour)
	if to.Before(from) {
		return nil
	}
	start := time.Date(from.Year(), from.Month(), 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(to.Year(), to.Month(), 1, 0, 0, 0, 0, time.UTC)
	var out []string
	for d := start; !d.After(end); d = d.AddDate(0, 1, 0) {
		out = append(out, d.Format("2006-01"))
	}
	return out
}

// financeAccountIDsForPUBFromConfigs 返回 BSS/实付/应付/余额 查询用的 account_id 列表。
// 若所选环境在配置中均为「account_id 与 environment 同名」占位，则返回 nil：仓库层不按 IN 过滤，以匹配真实阿里云账号主键。[Ref: 03_Phase6/01_FinOps 五维 P/U/B]
// 若混用占位与真实主账号，亦返回 nil，退化为全库汇总，避免漏计。
func financeAccountIDsForPUBFromConfigs(configs []postgres.EnvAccountConfig, envs []string) []string {
	if len(configs) == 0 {
		return nil
	}
	envSet := make(map[string]bool)
	if len(envs) == 0 {
		for _, c := range configs {
			envSet[c.Environment] = true
		}
	} else {
		for _, e := range envs {
			if e != "" {
				envSet[e] = true
			}
		}
	}
	var ids []string
	hasPlaceholder := false
	hasReal := false
	for _, c := range configs {
		if !envSet[c.Environment] {
			continue
		}
		aid := strings.TrimSpace(c.AccountID)
		if aid == "" || strings.EqualFold(aid, c.Environment) {
			hasPlaceholder = true
			continue
		}
		hasReal = true
		ids = append(ids, aid)
	}
	if hasPlaceholder && !hasReal {
		return nil
	}
	if hasPlaceholder && hasReal {
		return nil
	}
	if hasReal {
		return ids
	}
	return nil
}

func (s *CostService) financeAccountIDsForPUB(ctx context.Context, envs []string) []string {
	cfg, err := s.repo.ListEnvAccountConfig(ctx)
	if err != nil || len(cfg) == 0 {
		return nil
	}
	return financeAccountIDsForPUBFromConfigs(cfg, envs)
}

// applyLedgerOmitPCurrentMonth 本月时间范围下不展示 P（实付为账期现金流，当月未闭合）；与双轨设计一致。[Ref: 03_Phase6/01_FinOps]
func applyLedgerOmitPCurrentMonth(resp *dto.GlobalCostResponse, reportType, periodKey string, now time.Time) {
	if resp == nil || resp.Ledger == nil || resp.Ledger.P == nil {
		return
	}
	if reportType != "month" {
		return
	}
	t, err := time.Parse("2006-01", periodKey)
	if err != nil {
		return
	}
	curMonth := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	if !t.Equal(curMonth) {
		return
	}
	resp.Ledger.P = nil
	for i := range resp.EnvBreakdown {
		resp.EnvBreakdown[i].LedgerP = nil
	}
}

// envAccountIsPlaceholder account_id 与 environment 同名或为空时视为占位，不按真实主账号查 FinOps/BSS。[Ref: 03_Phase6/01_FinOps]
func envAccountIsPlaceholder(cfg *postgres.EnvAccountConfig) bool {
	if cfg == nil {
		return true
	}
	aid := strings.TrimSpace(cfg.AccountID)
	if aid == "" {
		return true
	}
	return strings.EqualFold(aid, strings.TrimSpace(cfg.Environment))
}

// paidPForAccount 单账号实付：BSS Payment+Expense 区间和，不足则月表现金列（与 mergeFinanceLedgerPUB 同键）。[Ref: 03_Phase6/01_FinOps]
func (s *CostService) paidPForAccount(ctx context.Context, from, to time.Time, cycles []string, accountID string) float64 {
	aid := strings.TrimSpace(accountID)
	if aid == "" {
		return 0
	}
	ids := []string{aid}
	p, errP := s.repo.SumBSSPaymentExpenseByDateRange(ctx, from, to, ids)
	pCash, errCash := s.repo.SumMonthlyCashTotalForBillingCycles(ctx, cycles, ids)
	if errP != nil && errCash == nil {
		return pCash
	}
	if errP == nil && math.Abs(p) < 1e-9 && errCash == nil && math.Abs(pCash) > 1e-9 {
		return pCash
	}
	if errP != nil {
		return 0
	}
	return p
}

// enrichEnvBreakdownLedgerCGPPerAccount 环境卡：按每条 env_breakdown 对应 account 填 consumption_cost、ledger_g、ledger_p。
// envs 仅用于 filterEnvAccountConfigs：须传 nil/空切片，使 **响应内全部环境行** 均参与匹配；若传非空（如 project_ids 展开后的 envsEff），未列入的环境行将缺 ledger，进而 enrichProjectBreakdownFromEnvBreakdown 把未选项目卡 G/P 置零或误分摊。[Ref: 03_Phase6/01_FinOps、03_Phase6/03_前端全域成本透视 R15]
func (s *CostService) enrichEnvBreakdownLedgerCGPPerAccount(ctx context.Context, resp *dto.GlobalCostResponse, track, reportType, periodKey string, now time.Time, envs []string) {
	if track != "technical" && track != "finance" {
		return
	}
	if resp == nil || len(resp.EnvBreakdown) == 0 {
		return
	}
	cfgAll, err := s.repo.ListEnvAccountConfig(ctx)
	if err != nil || len(cfgAll) == 0 {
		return
	}
	cfgF := filterEnvAccountConfigs(cfgAll, envs)
	if len(cfgF) == 0 {
		return
	}
	from, to := periodKeyToDateRange(reportType, periodKey, now)
	if from.IsZero() {
		return
	}
	cycles := billingMonthsInRange(from, to)
	accountToEnv := s.buildAccountToEnvMap(ctx)
	for i := range resp.EnvBreakdown {
		cfg := matchEnvAccountConfig(cfgF, resp.EnvBreakdown[i])
		if cfg == nil || envAccountIsPlaceholder(cfg) {
			continue
		}
		aid := strings.TrimSpace(cfg.AccountID)
		pp := s.paidPForAccount(ctx, from, to, cycles, aid)
		if c, g, err := s.finOpsCGPretaxForAccount(ctx, from, to, aid, accountToEnv); err == nil {
			cc, gg := c, g
			// OSS/fact 主路径 C≈0 时，用 api_query_account_bill 补全应付/回血（国内站等常见：BSS 已付但 OSS 正额未入账）[Ref: 03_Phase6/01_FinOps]
			srcCG := strings.ToLower(strings.TrimSpace(s.finOpsCGSourceForAccount(aid, accountToEnv)))
			if srcCG != "api" && math.Abs(cc) < 1e-6 {
				if c2, g2, err2 := s.repo.SumLineItemsPretaxCGByDateRangeWithChannel(ctx, from, to, []string{aid}, "api_query_account_bill"); err2 == nil {
					if math.Abs(c2) > 1e-6 {
						cc, gg = c2, g2
					} else if math.Abs(g2) > 1e-6 {
						gg = g2
					}
				}
			}
			// OLAP/API 仍无正额应付但 BSS 已有实付：用实付作应付展示代理（与 cash 口径对齐）[Ref: 03_Phase6/01_FinOps]
			if math.Abs(cc) < 1e-6 && pp > 1e-6 {
				cc = pp
			}
			resp.EnvBreakdown[i].ConsumptionCost = &cc
			resp.EnvBreakdown[i].LedgerG = &gg
		}
		resp.EnvBreakdown[i].LedgerP = &pp
	}
}

// matchEnvAccountConfig 将 env_breakdown 行与 filter 后的 cost_env_account_config 对齐（同 Environment，多行时按 AccountID）。[Ref: 03_Phase6/01_FinOps 五维 P/U/B]
func matchEnvAccountConfig(cfgF []postgres.EnvAccountConfig, ei dto.EnvBreakdownItem) *postgres.EnvAccountConfig {
	aid := strings.TrimSpace(ei.AccountID)
	var sameEnv []*postgres.EnvAccountConfig
	for i := range cfgF {
		if cfgF[i].Environment != ei.Environment {
			continue
		}
		sameEnv = append(sameEnv, &cfgF[i])
	}
	if len(sameEnv) == 0 {
		return nil
	}
	if len(sameEnv) == 1 {
		c := sameEnv[0]
		ca := strings.TrimSpace(c.AccountID)
		if aid != "" && ca != "" && aid != ca {
			return nil
		}
		return c
	}
	if aid != "" {
		for _, c := range sameEnv {
			if strings.TrimSpace(c.AccountID) == aid {
				return c
			}
		}
		return nil
	}
	return sameEnv[0]
}

// enrichEnvBreakdownLedgerPerEnvBUP 将各环境卡片的 B、U 与各账号 BSS 余额快照 / 应付事实对齐（与 mergeFinanceLedgerPUB 同键），替代按 TotalCost 占比的错误分摊。
// envs 须 nil/空，理由同 enrichEnvBreakdownLedgerCGPPerAccount。[Ref: 03_Phase6/01_FinOps 环境卡五维、03_Phase6/03_前端全域成本透视 R15]
func (s *CostService) enrichEnvBreakdownLedgerPerEnvBUP(ctx context.Context, resp *dto.GlobalCostResponse, reportType, periodKey string, now time.Time, envs []string) {
	if resp == nil || len(resp.EnvBreakdown) == 0 || resp.Ledger == nil {
		return
	}
	cfgAll, err := s.repo.ListEnvAccountConfig(ctx)
	if err != nil || len(cfgAll) == 0 {
		return
	}
	cfgF := filterEnvAccountConfigs(cfgAll, envs)
	if len(cfgF) == 0 {
		return
	}
	from, to := periodKeyToDateRange(reportType, periodKey, now)
	if from.IsZero() {
		return
	}
	cycles := billingMonthsInRange(from, to)
	bAsOf := now.UTC().Truncate(24 * time.Hour)

	balMap, errB := s.repo.LatestBSSBalanceMap(ctx, bAsOf)
	rows, errU := s.repo.ListBillOutstandingInBillingCycles(ctx, cycles)
	byCycle := make(map[string]map[string]float64)
	if errU == nil {
		for i := range rows {
			c := rows[i].BillingCycle
			if byCycle[c] == nil {
				byCycle[c] = make(map[string]float64)
			}
			aid := strings.TrimSpace(rows[i].AccountID)
			byCycle[c][aid] += rows[i].OutstandingAmount
		}
	}

	for i := range resp.EnvBreakdown {
		cfg := matchEnvAccountConfig(cfgF, resp.EnvBreakdown[i])
		if cfg == nil {
			continue
		}
		one := []postgres.EnvAccountConfig{*cfg}
		if errB == nil {
			b := sumPerEnvFromAccountMap(balMap, one)
			resp.EnvBreakdown[i].LedgerB = &b
		}
		if errU == nil {
			var u float64
			for _, cy := range cycles {
				cur := byCycle[cy]
				if cur == nil {
					cur = make(map[string]float64)
				}
				u += sumPerEnvFromAccountMap(cur, one)
			}
			resp.EnvBreakdown[i].LedgerU = &u
		}
	}
}

// fillTechnicalLedgerCG 填充 C/G；默认 FINOPS_CG_SOURCE + 按环境 FINOPS_CG_SOURCE_<ENV> 覆盖（metadata.finops_cg_source_by_env）。[Ref: 03_Phase6/01_FinOps]
func (s *CostService) fillTechnicalLedgerCG(ctx context.Context, resp *dto.GlobalCostResponse, reportType, periodKey string, now time.Time, accountIDs []string, envs []string) {
	from, to := periodKeyToDateRange(reportType, periodKey, now)
	if from.IsZero() {
		return
	}
	idList := s.ledgerCanonicalAccountIDs(ctx, envs)
	if len(idList) == 0 {
		idList = accountIDs
	}
	if len(idList) == 0 {
		return
	}
	accountToEnv := s.buildAccountToEnvMap(ctx)
	var cSum, gSum float64
	any := false
	for _, id := range idList {
		c, g, err := s.finOpsCGPretaxForAccount(ctx, from, to, id, accountToEnv)
		if err != nil {
			continue
		}
		cSum += c
		gSum += g
		any = true
	}
	if any {
		if resp.Ledger == nil {
			resp.Ledger = &dto.FinOpsLedger{}
		}
		resp.Ledger.C = &cSum
		resp.Ledger.G = &gSum
	}
}

// mergeFinanceLedgerPUB 合并 P/U/B（不覆盖已有 C/G）；双轨下五维尽可能同时有值。[Ref: 03_Phase6/01_FinOps]
// 当 financeAccountIDsForPUB 返回 nil（占位或多键混用）时，月原始现金/应付/余额按 cost_env_account_config 每环境只计一行，避免与 Hero 相同的双行叠加。[Ref: 03_Phase6/01_FinOps]
func (s *CostService) mergeFinanceLedgerPUB(ctx context.Context, resp *dto.GlobalCostResponse, reportType, periodKey string, now time.Time, accountIDs []string, envs []string) {
	from, to := periodKeyToDateRange(reportType, periodKey, now)
	if from.IsZero() {
		return
	}
	p, errP := s.repo.SumBSSPaymentExpenseByDateRange(ctx, from, to, accountIDs)
	if errP != nil {
		p = 0
	}
	// B 为「当前账户可用余额」最新快照：统一按「今天」取最近一条，避免 last_month 等区间 to=上月末日时 snapshot_date 晚于 to 导致筛不到行、余额恒为 0。[Ref: 03_Phase6/01_FinOps]
	bAsOf := now.UTC().Truncate(24 * time.Hour)
	cycles := billingMonthsInRange(from, to)

	cfgAll, _ := s.repo.ListEnvAccountConfig(ctx)
	cfgF := filterEnvAccountConfigs(cfgAll, envs)

	var b float64
	var errB error
	if len(accountIDs) == 0 && len(cfgF) > 0 {
		balMap, err := s.repo.LatestBSSBalanceMap(ctx, bAsOf)
		if err != nil {
			errB = err
		} else {
			b = sumPerEnvFromAccountMap(balMap, cfgF)
		}
	} else {
		b, errB = s.repo.LatestBSSBalanceSum(ctx, accountIDs, bAsOf)
	}

	var u float64
	var errU error
	if len(accountIDs) == 0 && len(cfgF) > 0 {
		u, errU = s.sumOutstandingDedupedForCycles(ctx, cycles, cfgF)
	} else {
		u, errU = s.repo.SumOutstandingByBillingCycles(ctx, cycles, accountIDs)
	}

	var pCash float64
	var errCash error
	if len(accountIDs) == 0 && len(cfgF) > 0 {
		pCash, errCash = s.sumMonthlyCashTotalDedupedForCycles(ctx, cycles, cfgF)
	} else {
		pCash, errCash = s.repo.SumMonthlyCashTotalForBillingCycles(ctx, cycles, accountIDs)
	}

	if errP != nil && errCash == nil {
		p = pCash
		errP = nil
	} else if errP == nil && math.Abs(p) < 1e-9 && errCash == nil && math.Abs(pCash) > 1e-9 {
		p = pCash
		errP = nil
	}
	if resp.Ledger == nil {
		resp.Ledger = &dto.FinOpsLedger{}
	}
	// 实付 P：与控制台「已还款」一致，优先 BSS Payment+Expense，不足则月表现金；不因 C+G 用 max(0,C+G) 覆盖。[Ref: 03_Phase6/01_FinOps]
	var pp float64
	hasP := false
	if errP == nil {
		pp = p
		hasP = true
	} else if errCash == nil && math.Abs(pCash) > 1e-9 {
		pp = pCash
		hasP = true
	}
	if hasP {
		resp.Ledger.P = &pp
	}
	if errB == nil {
		bb := b
		resp.Ledger.B = &bb
	}
	if errU == nil {
		uu := u
		resp.Ledger.U = &uu
	}
}

// alignLedgerPWithAggregatePaymentFallback 当 BSS/月表现金未给出有效 P 时，用聚合表 payment 回填全局 P（与钻取 payment 口径对齐）。[Ref: 03_Phase6/01_FinOps]
func (s *CostService) alignLedgerPWithAggregatePaymentFallback(ctx context.Context, resp *dto.GlobalCostResponse, reportType, periodKey string, accountIDs []string) {
	if resp == nil {
		return
	}
	var cur float64
	hasP := false
	if resp.Ledger != nil && resp.Ledger.P != nil {
		cur = *resp.Ledger.P
		hasP = true
	}
	if hasP && math.Abs(cur) > 1e-6 {
		return
	}
	list, err := s.repo.ListCloudBillAggregateForReportPeriod(ctx, reportType, periodKey, "payment", accountIDs)
	if err != nil || len(list) == 0 {
		return
	}
	var sum float64
	for i := range list {
		sum += list[i].TotalAmount
	}
	if math.Abs(sum) < 1e-9 {
		return
	}
	if resp.Ledger == nil {
		resp.Ledger = &dto.FinOpsLedger{}
	}
	pp := sum
	resp.Ledger.P = &pp
}

// enrichProjectBreakdownFromEnvBreakdown U/B/consumption 按成员环境加总；G/P 优先取成员环境 env 行加总（与全量分摊后的 env 卡一致）。
// 不再用「已筛选的 Hero ledger × 权重」分摊，避免 project_ids/envs 筛选时项目卡与未筛选不一致。[Ref: 03_Phase6/03_前端全域成本透视/01_设计]
func enrichProjectBreakdownFromEnvBreakdown(resp *dto.GlobalCostResponse, projects []postgres.CostProject, track string) {
	if resp == nil || len(resp.ProjectBreakdown) == 0 || len(resp.EnvBreakdown) == 0 {
		return
	}
	envByName := make(map[string]dto.EnvBreakdownItem, len(resp.EnvBreakdown))
	for i := range resp.EnvBreakdown {
		e := resp.EnvBreakdown[i]
		envByName[e.Environment] = e
	}
	pidToEnvs := make(map[int][]string, len(projects))
	for i := range projects {
		pidToEnvs[projects[i].ID] = projects[i].Environments
	}
	envWeight := func(e dto.EnvBreakdownItem) float64 {
		if e.TotalCost > 0 {
			return e.TotalCost
		}
		if e.ConsumptionCost != nil && *e.ConsumptionCost > 0 {
			return *e.ConsumptionCost
		}
		if e.LedgerP != nil && *e.LedgerP > 0 {
			return *e.LedgerP
		}
		return 0
	}
	projWeight := make(map[int]float64, len(resp.ProjectBreakdown))
	var totalPos float64
	for i := range resp.ProjectBreakdown {
		pid := resp.ProjectBreakdown[i].ProjectID
		envs := pidToEnvs[pid]
		var w float64
		for _, env := range envs {
			if e, ok := envByName[env]; ok {
				w += envWeight(e)
			}
		}
		projWeight[pid] = w
		totalPos += w
	}
	for i := range resp.ProjectBreakdown {
		p := &resp.ProjectBreakdown[i]
		envs := pidToEnvs[p.ProjectID]
		if len(envs) == 0 {
			continue
		}
		w := projWeight[p.ProjectID]
		var gSum, pSum, uSum, bSum, consSum float64
		var hasCons bool
		for _, env := range envs {
			e, ok := envByName[env]
			if !ok {
				continue
			}
			if e.LedgerG != nil {
				gSum += *e.LedgerG
			}
			if e.LedgerP != nil {
				pSum += *e.LedgerP
			}
			if e.LedgerU != nil {
				uSum += *e.LedgerU
			}
			if e.LedgerB != nil {
				bSum += *e.LedgerB
			}
			if e.ConsumptionCost != nil {
				consSum += *e.ConsumptionCost
				hasCons = true
			}
		}
		L := resp.Ledger
		if L != nil {
			if L.G != nil {
				v := gSum
				if math.Abs(v) < 1e-12 && totalPos > 1e-12 && w > 0 && L.G != nil {
					v = *L.G * (w / totalPos)
				} else if totalPos > 1e-12 && w <= 0 {
					v = 0 // 成员环境无正额权重时不分摊全局 G（占位环境不与 POC/UAT 同额）
				}
				p.LedgerG = &v
			}
			if L.P != nil || (track == "finance" && math.Abs(p.TotalCost) > 1e-12) {
				v := pSum
				if math.Abs(v) < 1e-12 && totalPos > 1e-12 && w > 0 && L.P != nil {
					v = *L.P * (w / totalPos)
				} else if totalPos > 1e-12 && w <= 0 {
					v = 0
				} else if math.Abs(v) < 1e-12 && track == "finance" {
					v = p.TotalCost // 本月未闭合等场景下 Hero 可能省略 P，项目实付与聚合 total 对齐
				}
				p.LedgerP = &v
			}
			if L.U != nil {
				v := uSum
				p.LedgerU = &v
			}
			if L.B != nil {
				v := bSum
				p.LedgerB = &v
			}
		}
		if hasCons {
			v := consSum
			p.ConsumptionCost = &v
		}
	}
}

// enrichFinOpsLedger 填充 ledger / reconciliation（track=technical|finance）；C/G 与 P/U/B 合并填充以利五维快照。
// accountIDs/envs 仅用于 **顶层** resp.Ledger（Hero 筛选）与 fillTechnicalLedgerCG / mergeFinanceLedgerPUB；**每条** env_breakdown 的 C/G/P 与 B/U 必须按全量 cost_env_account_config 逐环境填写（传 nil 给 enrichEnv*），否则 project_ids 筛选时未选项目成员环境缺事实、项目卡畸变。[Ref: 03_Phase6/01_FinOps、03_Phase6/03_前端全域成本透视 R15]
func (s *CostService) enrichFinOpsLedger(ctx context.Context, resp *dto.GlobalCostResponse, track, reportType, periodKey string, now time.Time, accountIDs []string, envs []string) {
	if track != "technical" && track != "finance" {
		return
	}
	from, to := periodKeyToDateRange(reportType, periodKey, now)
	if from.IsZero() {
		return
	}
	nFact, _ := s.repo.CountFinOpsBillingFactsInDateRange(ctx, from, to, accountIDs)
	idList := s.ledgerCanonicalAccountIDs(ctx, envs)
	if len(idList) == 0 {
		idList = accountIDs
	}
	s.fillTechnicalLedgerCG(ctx, resp, reportType, periodKey, now, accountIDs, envs)
	pubIDs := s.financeAccountIDsForPUB(ctx, envs)
	s.mergeFinanceLedgerPUB(ctx, resp, reportType, periodKey, now, pubIDs, envs)
	s.alignLedgerPWithAggregatePaymentFallback(ctx, resp, reportType, periodKey, pubIDs)
	s.enrichEnvBreakdownLedgerCGPPerAccount(ctx, resp, track, reportType, periodKey, now, nil)
	applyLedgerOmitPCurrentMonth(resp, reportType, periodKey, now)
	s.enrichEnvBreakdownLedgerPerEnvBUP(ctx, resp, reportType, periodKey, now, nil)
	if projs, perr := s.repo.ListCostProjects(ctx); perr == nil && len(projs) > 0 {
		enrichProjectBreakdownFromEnvBreakdown(resp, projs, track)
	}
	var ossSum float64
	if nFact > 0 {
		if len(idList) > 0 {
			for _, id := range idList {
				nAcc, _ := s.repo.CountFinOpsBillingFactsInDateRange(ctx, from, to, []string{id})
				if nAcc > 0 {
					v, _ := s.repo.SumFinOpsFactPretaxTotalByDateRange(ctx, from, to, []string{id})
					ossSum += v
				} else {
					v, _ := s.repo.SumPretaxByChannelForDateRange(ctx, from, to, []string{id}, "oss_detail")
					ossSum += v
				}
			}
		} else {
			ossSum, _ = s.repo.SumFinOpsFactPretaxTotalByDateRange(ctx, from, to, accountIDs)
		}
	} else {
		ossSum, _ = s.repo.SumPretaxByChannelForDateRange(ctx, from, to, accountIDs, "oss_detail")
	}
	apiSum, _ := s.repo.SumPretaxByChannelForDateRange(ctx, from, to, accountIDs, "api_query_account_bill")
	if math.Abs(ossSum) > 1e-9 && math.Abs(apiSum) > 1e-9 {
		res := apiSum - ossSum
		resp.Reconciliation = &dto.FinOpsReconciliation{
			Residual: &res,
			Explain:  "api_query_account_bill pretax sum minus OSS OLAP (finops_billing_fact) or oss_detail line_items (same date range)",
		}
	}
	if resp.Metadata == nil {
		resp.Metadata = &dto.GlobalCostMetadata{}
	}
	s.applyFinOpsCGMetadata(ctx, resp.Metadata, envs)
	resp.Metadata.LedgerSnapshotNote = finOpsLedgerSnapshotNoteZH
}

// applyFinOpsCGMetadata 写入 finops_cg_source / finops_cg_source_by_env（多环境混用为 mixed）。[Ref: 03_Phase6/01_FinOps]
func (s *CostService) applyFinOpsCGMetadata(ctx context.Context, meta *dto.GlobalCostMetadata, envFilter []string) {
	if meta == nil {
		return
	}
	configs, err := s.repo.ListEnvAccountConfig(ctx)
	if err != nil || len(configs) == 0 {
		meta.FinOpsCGSource = "oss"
		return
	}
	envSet := make(map[string]bool)
	if len(envFilter) == 0 {
		for i := range configs {
			envSet[configs[i].Environment] = true
		}
	} else {
		for _, e := range envFilter {
			if e != "" {
				envSet[e] = true
			}
		}
	}
	byEnv := make(map[string]string)
	for i := range configs {
		c := configs[i]
		if !envSet[c.Environment] {
			continue
		}
		byEnv[c.Environment] = "oss"
	}
	meta.FinOpsCGSourceByEnv = byEnv
	meta.FinOpsCGSource = "oss"
}

// previousPeriodKey 返回上一周期的 period_key（用于 compare_mode=previous）。[Ref: 01_设计 report_type 与 period_key]
func previousPeriodKey(reportType, periodKey string, now time.Time) string {
	switch reportType {
	case "month", "last_month":
		t, _ := time.Parse("2006-01", periodKey)
		return t.AddDate(0, -1, 0).Format("2006-01")
	case "quarter", "last_quarter":
		var y int
		var q int
		_, _ = fmt.Sscanf(periodKey, "%d-Q%d", &y, &q)
		if q <= 1 {
			return fmt.Sprintf("%d-Q4", y-1)
		}
		return fmt.Sprintf("%d-Q%d", y, q-1)
	case "this_year", "last_year":
		var y int
		if _, err := fmt.Sscanf(periodKey, "%d", &y); err == nil {
			return fmt.Sprintf("%d", y-1)
		}
		return ""
	default:
		return ""
	}
}

// buildEnvBreakdownFromAccountTotals 从 cost_env_account_config 与按 account 隔离的金额构建 env_breakdown，保证各环境仅展示各自账户数据。[Ref: 16_ §十三 POC/UAT 数据绝对隔离]
func (s *CostService) buildEnvBreakdownFromAccountTotals(ctx context.Context, curAccountTotals, prevAccountTotals map[string]float64) []dto.EnvBreakdownItem {
	configs, err := s.repo.ListEnvAccountConfig(ctx)
	if err != nil {
		return nil
	}
	if curAccountTotals == nil {
		curAccountTotals = make(map[string]float64)
	}
	if prevAccountTotals == nil {
		prevAccountTotals = make(map[string]float64)
	}
	out := make([]dto.EnvBreakdownItem, 0, len(configs))
	for i := range configs {
		c := &configs[i]
		env := c.Environment
		// [Ref: 16_ §十三] 各环境严格仅展示其 account_id 对应金额；禁止将空 account（未分配）摊到其它环境
		total := curAccountTotals[c.AccountID]
		if total == 0 {
			total = curAccountTotals[c.Environment]
		}
		prev := prevAccountTotals[c.AccountID]
		if prev == 0 {
			prev = prevAccountTotals[c.Environment]
		}
		changePct := 0.0
		if prev > 0 {
			changePct = ((total - prev) / prev) * 100
		}
		displayName := c.DisplayName
		if displayName == "" {
			displayName = c.AccountID
		}
		out = append(out, dto.EnvBreakdownItem{
			Environment:        env,
			AccountID:          c.AccountID,
			AccountDisplayName: displayName,
			TotalCost:          total,
			PreviousPeriodCost: prev,
			ChangePct:          changePct,
		})
	}
	return out
}

// buildEnvBreakdown 从 cost_env_account_config 与聚合表按环境汇总 env_breakdown。[Ref: 01_设计 §按环境展示、§后端数据聚合与存储方案]
func (s *CostService) buildEnvBreakdown(ctx context.Context, reportType, periodKey, prevPeriodKey string) []dto.EnvBreakdownItem {
	configs, err := s.repo.ListEnvAccountConfig(ctx)
	if err != nil {
		return nil
	}
	curList, _ := s.repo.ListCloudBillAggregateForReportPeriod(ctx, reportType, periodKey, "consumption", nil)
	prevList, _ := s.repo.ListCloudBillAggregateForReportPeriod(ctx, reportType, prevPeriodKey, "consumption", nil)
	curByAccount := make(map[string]float64)
	for _, a := range curList {
		k := a.AccountID
		curByAccount[k] += a.TotalAmount
	}
	prevByAccount := make(map[string]float64)
	for _, a := range prevList {
		prevByAccount[a.AccountID] += a.TotalAmount
	}
	out := make([]dto.EnvBreakdownItem, 0, len(configs))
	for i := range configs {
		c := &configs[i]
		env := c.Environment
		total := curByAccount[c.AccountID]
		if total == 0 {
			total = curByAccount[c.Environment] // 单账号时 ETL 可能写入 environment 名（如 POC），与 config.account_id 不一致时用环境名回退 [Ref: 01_设计 §按环境展示]
		}
		if total == 0 && curByAccount[""] > 0 {
			total = curByAccount[""] // 单账号占位符 '' 时，将总账归入该已配置环境 [Ref: 01_设计 §按环境展示、D1 排查]
		}
		prev := prevByAccount[c.AccountID]
		if prev == 0 {
			prev = prevByAccount[c.Environment]
		}
		if prev == 0 && prevByAccount[""] > 0 {
			prev = prevByAccount[""]
		}
		changePct := 0.0
		if prev > 0 {
			changePct = ((total - prev) / prev) * 100
		}
		displayName := c.DisplayName
		if displayName == "" {
			displayName = c.AccountID
		}
		out = append(out, dto.EnvBreakdownItem{
			Environment:        env,
			AccountID:          c.AccountID,
			AccountDisplayName: displayName,
			TotalCost:          total,
			PreviousPeriodCost: prev,
			ChangePct:          changePct,
		})
	}
	return out
}

// accountIDsForEnvs 返回所选环境对应的 account_id 集合（用于按环境过滤）。[Ref: 用户需求 环境多选]
// envs 为空时返回 nil；需要「全环境但排除空 account」时用 accountIDsFromAllConfig。
func (s *CostService) accountIDsForEnvs(ctx context.Context, envs []string) map[string]bool {
	if len(envs) == 0 {
		return nil
	}
	configs, err := s.repo.ListEnvAccountConfig(ctx)
	if err != nil {
		return nil
	}
	envSet := make(map[string]bool)
	for _, e := range envs {
		if e != "" {
			envSet[e] = true
		}
	}
	ids := make(map[string]bool)
	for _, c := range configs {
		if envSet[c.Environment] {
			ids[c.AccountID] = true
			ids[c.Environment] = true // 单账号时 ETL 可能写 environment 名
		}
	}
	return ids
}

// accountIDsForEnvsStrict 与 accountIDsForEnvs 类似，但每行配置仅加入「主键」：有 account_id 则只加 account_id，否则才加 environment 名。
// 月原始表若同时存在数字 account_id 行与同名 environment 占位行，宽松 map 会导致 Hero 双计而 env 卡仍按单环境一行展示。[Ref: 03_Phase6/03_前端全域成本透视/01_设计 §环境筛选]
func (s *CostService) accountIDsForEnvsStrict(ctx context.Context, envs []string) map[string]bool {
	if len(envs) == 0 {
		return nil
	}
	configs, err := s.repo.ListEnvAccountConfig(ctx)
	if err != nil {
		return nil
	}
	envSet := make(map[string]bool)
	for _, e := range envs {
		if e != "" {
			envSet[e] = true
		}
	}
	ids := make(map[string]bool)
	for _, c := range configs {
		if !envSet[c.Environment] {
			continue
		}
		if strings.TrimSpace(c.AccountID) != "" {
			ids[c.AccountID] = true
		} else {
			ids[c.Environment] = true
		}
	}
	return ids
}

// accountIDsFromAllConfig 返回 cost_env_account_config 中所有 account_id，用于聚合时排除空 account 行（避免 ETL 写入 account_id=空 导致重复摊给多环境）。[Ref: 16_ §十三]
func (s *CostService) accountIDsFromAllConfig(ctx context.Context) map[string]bool {
	configs, err := s.repo.ListEnvAccountConfig(ctx)
	if err != nil || len(configs) == 0 {
		return nil
	}
	ids := make(map[string]bool)
	for _, c := range configs {
		if c.AccountID != "" {
			ids[c.AccountID] = true
		}
		ids[c.Environment] = true
	}
	return ids
}

// projectIDsCacheKey 将 project_ids 序列化进日期范围缓存键。[Ref: 03_Phase6/03_前端全域成本透视/01_设计]
func projectIDsCacheKey(ids []int) string {
	if len(ids) == 0 {
		return ""
	}
	sort.Ints(ids)
	var b strings.Builder
	for i, id := range ids {
		if i > 0 {
			b.WriteByte(',')
		}
		fmt.Fprintf(&b, "%d", id)
	}
	return b.String()
}

// envsCacheKey 将 effective env 筛选序列化进日期范围缓存键（与聚合表路径一致）。[Ref: 03_Phase6/03_前端全域成本透视/01_设计]
func envsCacheKey(envs []string) string {
	if len(envs) == 0 {
		return ""
	}
	cp := append([]string(nil), envs...)
	sort.Strings(cp)
	return strings.Join(cp, ",")
}

// effectiveEnvsForGlobalCost project_ids 优先：非空时展开为项目成员环境并忽略 URL envs。[Ref: 03_Phase6/03_前端全域成本透视/01_设计]
func (s *CostService) effectiveEnvsForGlobalCost(ctx context.Context, envs []string, projectIDs []int) ([]string, error) {
	if len(projectIDs) == 0 {
		return envs, nil
	}
	return s.repo.EnvironmentsByProjectIDs(ctx, projectIDs)
}

// buildProjectBreakdownFromAggregateList 按聚合表全量 list 与各项目成员环境汇总。[Ref: 03_Phase6/03_前端全域成本透视/01_设计]
func buildProjectBreakdownFromAggregateList(list []postgres.CloudBillAggregate, projects []postgres.CostProject, configs []postgres.EnvAccountConfig) []dto.ProjectBreakdownItem {
	curByAccount := make(map[string]float64)
	for i := range list {
		a := &list[i]
		curByAccount[a.AccountID] += a.TotalAmount
	}
	envToAccount := make(map[string]string)
	for i := range configs {
		c := &configs[i]
		envToAccount[c.Environment] = c.AccountID
	}
	out := make([]dto.ProjectBreakdownItem, 0, len(projects))
	for i := range projects {
		p := &projects[i]
		var sum float64
		for _, env := range p.Environments {
			acc := envToAccount[env]
			if acc == "" {
				continue
			}
			v := curByAccount[acc]
			if v == 0 {
				v = curByAccount[env]
			}
			sum += v
		}
		out = append(out, dto.ProjectBreakdownItem{
			ProjectID: p.ID,
			Code:        p.Code,
			Name:        p.Name,
			SortOrder:   p.SortOrder,
			TotalCost:   sum,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].SortOrder != out[j].SortOrder {
			return out[i].SortOrder < out[j].SortOrder
		}
		return out[i].ProjectID < out[j].ProjectID
	})
	return out
}

// buildProjectBreakdownFromAccountTotalsMap 自定义月区间按 account 累计映射到项目。[Ref: 03_Phase6/03_前端全域成本透视/01_设计]
func buildProjectBreakdownFromAccountTotalsMap(projects []postgres.CostProject, configs []postgres.EnvAccountConfig, totals map[string]float64) []dto.ProjectBreakdownItem {
	if len(totals) == 0 {
		totals = map[string]float64{}
	}
	envToAccount := make(map[string]string)
	for i := range configs {
		c := &configs[i]
		envToAccount[c.Environment] = c.AccountID
	}
	out := make([]dto.ProjectBreakdownItem, 0, len(projects))
	for i := range projects {
		p := &projects[i]
		var sum float64
		for _, env := range p.Environments {
			acc := envToAccount[env]
			if acc == "" {
				continue
			}
			v := totals[acc]
			if v == 0 {
				v = totals[env]
			}
			sum += v
		}
		out = append(out, dto.ProjectBreakdownItem{
			ProjectID: p.ID,
			Code:        p.Code,
			Name:        p.Name,
			SortOrder:   p.SortOrder,
			TotalCost:   sum,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].SortOrder != out[j].SortOrder {
			return out[i].SortOrder < out[j].SortOrder
		}
		return out[i].ProjectID < out[j].ProjectID
	})
	return out
}

// normalizePeriod 规范化前端传入的 period，避免因大小写或别名导致 reportType 为空、env_breakdown 全 0。[Ref: 01_成本透视 环境卡片无数据]
func normalizePeriod(p string) string {
	p = strings.TrimSpace(p)
	switch strings.ToLower(p) {
	case "lastmonth", "last_month":
		return "last_month"
	case "lastquarter", "last_quarter":
		return "last_quarter"
	case "lastyear", "last_year":
		return "last_year"
	case "thisyear", "this_year":
		return "this_year"
	default:
		return p
	}
}

// GetGlobalCost returns L0 cost. [Ref: 04_01_成本透视真实数据、16_ §三]
// [Ref: 用户需求] 上月/上季度/去年/自定义月=月粒度现金/已付（绝对准确），不读聚合表；本月/这季度/今年=天叠可用消耗+回调日替代（支出参考），可读聚合表或降级。
// envs 非空时仅汇总所选环境的成本（仅聚合表路径支持）；project_ids 非空时展开为成员环境并忽略 envs。[Ref: 03_Phase6/03_前端全域成本透视/01_设计]
// metricType 已废弃。
// track：FinOps 双轨 technical|finance；空或非法则与现网一致不写 effective_track。[Ref: 03_Phase6/01_FinOps双轨与全域成本重构/01_设计 §API、track 与 UX]
func (s *CostService) GetGlobalCost(ctx context.Context, period string, metricType string, envs []string, projectIDs []int, track string) (*dto.GlobalCostResponse, error) {
	s.runFinOpsAuxiliarySyncIfRequested(ctx)
	period = normalizePeriod(period)
	now := time.Now().UTC()
	envsEff, err := s.effectiveEnvsForGlobalCost(ctx, envs, projectIDs)
	if err != nil {
		return nil, err
	}
	reportType, periodKey := reportTypeAndPeriodKey(period, now)
	// [Ref: 用户需求、03_Phase6/01_FinOps] 上月与自定义单月同口径：优先走月原始表（与 GET .../global?date_from&date_to 一致）。
	// 聚合表 last_month+payment 若未落库或全零，资金轨 Hero 会显示实付 0 而 ledger 仍有消耗，与自定义选同月矛盾。
	// 账期按北京时间自然月（Asia/Shanghai）；last_month 月原始回退见 resolveLastMonthFromMonthlyRaw。[Ref: 用户需求]
	if period == "last_month" {
		if respDR, errDR := s.resolveLastMonthFromMonthlyRaw(ctx, now, track, projectIDs, envs); errDR != nil {
			return nil, errDR
		} else if respDR != nil {
			return respDR, nil
		}
	}
	// [Ref: 聚合表主路径 方案A] 仅读聚合表，无降级；所有 period 映射到 report_type+period_key。
	if reportType != "" && periodKey != "" {
		metricTypeSel := globalMetricTypeForTrack(track, reportType)
		accountIDsSlice := accountIDsToSlice(len(envsEff) > 0, s.accountIDsForEnvs(ctx, envsEff), s.accountIDsFromAllConfig(ctx))
		list, _ := s.repo.ListCloudBillAggregateForReportPeriod(ctx, reportType, periodKey, metricTypeSel, accountIDsSlice)
		// 环境卡片：始终展示各环境全量金额；筛选仅影响 Hero/分解/ledger 汇总 [Ref: 用户需求 环境卡片与筛选解耦]
		listFull, _ := s.repo.ListCloudBillAggregateForReportPeriod(ctx, reportType, periodKey, metricTypeSel, nil)
		if len(list) > 0 {
			configs, cfgErr := s.repo.ListEnvAccountConfig(ctx)
			configsF := filterEnvAccountConfigs(configs, envsEff)
			var totalAmount float64
			merged := make(map[string]float64)
			if cfgErr == nil && len(configsF) > 0 {
				totalAmount, merged, _ = aggregateHeroTotalsFromDedupedList(list, configsF)
			} else {
				for _, a := range list {
					totalAmount += a.TotalAmount
					for k, v := range a.ProductBreakdown {
						merged[k] += v
					}
				}
			}
			var lastSuccessAt *time.Time
			for _, a := range list {
				if a.LastSuccessAt != nil && (lastSuccessAt == nil || a.LastSuccessAt.After(*lastSuccessAt)) {
					lastSuccessAt = a.LastSuccessAt
				}
			}
			if totalAmount != 0 || len(merged) > 0 {
				scaleMergedToTotal(merged, totalAmount)
				domainBreakdown := buildDomainBreakdownNormalized(merged)
				scaleDomainBreakdownToTotal(domainBreakdown, totalAmount)
				prevKey := previousPeriodKey(reportType, periodKey, now)
				prevFull, _ := s.repo.ListCloudBillAggregateForReportPeriod(ctx, reportType, prevKey, metricTypeSel, nil)
				envBreakdown := s.buildEnvBreakdownFromList(ctx, listFull, prevFull)
				// [Ref: 用户需求] 聚合表返回为负时同样展示为 0 并注明净退款已抵减（如今年、这季度）
				displayTotal := totalAmount
				displayNote := ""
				if totalAmount < 0 {
					displayTotal = 0
					displayNote = "该周期净退款已抵减"
					for i := range domainBreakdown {
						domainBreakdown[i].Cost = 0
					}
					for i := range envBreakdown {
						if envBreakdown[i].TotalCost < 0 {
							envBreakdown[i].TotalCost = 0
						}
					}
				}
				// 仅「全环境」时缩放到 Hero；有 env 筛选时卡片保持全量、Hero 为筛选后合计 [Ref: 用户需求]
				if len(envsEff) == 0 {
					scaleEnvBreakdownToTotal(envBreakdown, displayTotal)
				}
				if track == "finance" && len(envBreakdown) > 0 {
					s.enrichEnvBreakdownConsumptionFromAggregate(ctx, envBreakdown, reportType, periodKey)
				}
				meta := &dto.GlobalCostMetadata{DataStatus: "aggregate", ReportType: reportType, PeriodKey: periodKey, DisplayNote: displayNote}
				if lastSuccessAt != nil {
					meta.LastUpdatedAt = lastSuccessAt
				}
				// [Ref: 16_云账单动态对账与高可靠处理规范 §三段式] 注入账单对账状态供前端区分"已财务核算"/"动态同步"
				meta.BillDataStatus = s.resolveBillDataStatus(ctx, reportType, periodKey, now)
				resp := &dto.GlobalCostResponse{
					TotalCost:        displayTotal,
					TotalOptimizable: 0,
					GlobalEfficiency: 0,
					DomainBreakdown:  domainBreakdown,
					EnvBreakdown:     envBreakdown,
					Namespaces:       nil,
					Timestamp:        now,
					Metadata:         meta,
				}
				dto.ApplyFinOpsGlobalMetadata(resp, track)
				if projects, perr := s.repo.ListCostProjects(ctx); perr == nil && len(projects) > 0 && len(configs) > 0 {
					resp.ProjectBreakdown = buildProjectBreakdownFromAggregateList(listFull, projects, configs)
				}
				s.enrichFinOpsLedger(ctx, resp, track, reportType, periodKey, now, accountIDsSlice, envsEff)
				s.finalizeGlobalCostResponse(ctx, resp)
				return resp, nil
			}
		}
		// 聚合表无数据或全零：返回空结构；传 track 时仍填充 ledger。[Ref: 聚合表主路径、03_Phase6/01_FinOps]
		configs, _ := s.repo.ListEnvAccountConfig(ctx)
		envBreakdown := buildEnvBreakdownEmpty(configs)
		resp := &dto.GlobalCostResponse{
			TotalCost:        0,
			TotalOptimizable: 0,
			GlobalEfficiency: 0,
			DomainBreakdown:  []dto.DomainBreakdownItem{},
			EnvBreakdown:     envBreakdown,
			Namespaces:       nil,
			Timestamp:        now,
			Metadata:         &dto.GlobalCostMetadata{DataStatus: "aggregate", ReportType: reportType, PeriodKey: periodKey},
		}
		dto.ApplyFinOpsGlobalMetadata(resp, track)
		if projects, perr := s.repo.ListCostProjects(ctx); perr == nil && len(projects) > 0 && len(configs) > 0 {
			resp.ProjectBreakdown = buildProjectBreakdownFromAggregateList(nil, projects, configs)
		}
		s.enrichFinOpsLedger(ctx, resp, track, reportType, periodKey, now, accountIDsSlice, envsEff)
		s.finalizeGlobalCostResponse(ctx, resp)
		return resp, nil
	}

	// 未知 period（reportType 或 periodKey 为空）：返回空结构 [Ref: 聚合表主路径]
	configs, _ := s.repo.ListEnvAccountConfig(ctx)
	envBreakdownEmpty := buildEnvBreakdownEmpty(configs)
	resp := &dto.GlobalCostResponse{
		TotalCost:        0,
		TotalOptimizable: 0,
		GlobalEfficiency: 0,
		DomainBreakdown:  []dto.DomainBreakdownItem{},
		EnvBreakdown:     envBreakdownEmpty,
		Namespaces:       nil,
		Timestamp:        now,
		Metadata:         &dto.GlobalCostMetadata{DataStatus: "aggregate"},
	}
	dto.ApplyFinOpsGlobalMetadata(resp, track)
	if projects, perr := s.repo.ListCostProjects(ctx); perr == nil && len(projects) > 0 && len(configs) > 0 {
		resp.ProjectBreakdown = buildProjectBreakdownFromAggregateList(nil, projects, configs)
	}
	s.finalizeGlobalCostResponse(ctx, resp)
	return resp, nil
}

// enrichEnvBreakdownConsumptionFromAggregate 资金轨下写入各环境聚合表 consumption 口径金额，与 total_cost（payment）分离，避免环境卡 C 与 P 因同一权重分摊全局 ledger 而恒等。[Ref: 03_Phase6/01_FinOps]
func (s *CostService) enrichEnvBreakdownConsumptionFromAggregate(ctx context.Context, env []dto.EnvBreakdownItem, reportType, periodKey string) {
	if s == nil || len(env) == 0 {
		return
	}
	consList, err := s.repo.ListCloudBillAggregateForReportPeriod(ctx, reportType, periodKey, "consumption", nil)
	if err != nil || len(consList) == 0 {
		return
	}
	curByAccount := make(map[string]float64)
	for i := range consList {
		acc := consList[i].AccountID
		curByAccount[acc] += consList[i].TotalAmount
	}
	for i := range env {
		aid := strings.TrimSpace(env[i].AccountID)
		v := curByAccount[aid]
		if v == 0 {
			v = curByAccount[env[i].Environment]
		}
		vv := v
		env[i].ConsumptionCost = &vv
	}
}

// enrichEnvBreakdownConsumptionFromMonthlyRawRange 自定义月范围资金轨：从月原始表按消耗口径叠加各 account，写入 consumption_cost。[Ref: 03_Phase6/01_FinOps]
func (s *CostService) enrichEnvBreakdownConsumptionFromMonthlyRawRange(ctx context.Context, env []dto.EnvBreakdownItem, from, to time.Time, accountIDs map[string]bool) {
	if s == nil || len(env) == 0 {
		return
	}
	cur := time.Date(from.Year(), from.Month(), 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(to.Year(), to.Month(), 1, 0, 0, 0, 0, time.UTC)
	const maxMonths = 60
	count := 0
	consTotals := make(map[string]float64)
	for !cur.After(end) && count < maxMonths {
		cycle := cur.Format("2006-01")
		_, _, accTotals := s.mergeMonthlyRawByCycle(ctx, cycle, accountIDs, true)
		for acc, amt := range accTotals {
			consTotals[acc] += amt
		}
		cur = cur.AddDate(0, 1, 0)
		count++
	}
	if len(consTotals) == 0 {
		return
	}
	for i := range env {
		aid := strings.TrimSpace(env[i].AccountID)
		v := consTotals[aid]
		if v == 0 {
			v = consTotals[env[i].Environment]
		}
		vv := v
		env[i].ConsumptionCost = &vv
	}
}

// buildEnvBreakdownFromList 从聚合表行按 account 构建 env_breakdown。[Ref: 聚合表主路径]
func (s *CostService) buildEnvBreakdownFromList(ctx context.Context, curList, prevList []postgres.CloudBillAggregate) []dto.EnvBreakdownItem {
	configs, err := s.repo.ListEnvAccountConfig(ctx)
	if err != nil {
		return nil
	}
	curByAccount := make(map[string]float64)
	for _, a := range curList {
		curByAccount[a.AccountID] += a.TotalAmount
	}
	prevByAccount := make(map[string]float64)
	for _, a := range prevList {
		prevByAccount[a.AccountID] += a.TotalAmount
	}
	if len(configs) == 0 {
		return nil
	}
	out := make([]dto.EnvBreakdownItem, 0, len(configs))
	for i := range configs {
		c := &configs[i]
		env := c.Environment
		total := curByAccount[c.AccountID]
		if total == 0 {
			total = curByAccount[c.Environment] // 兼容 ETL 写 environment 名作 account_id
		}
		prev := prevByAccount[c.AccountID]
		if prev == 0 {
			prev = prevByAccount[c.Environment]
		}
		changePct := 0.0
		if prev > 0 {
			changePct = ((total - prev) / prev) * 100
		}
		dn := c.DisplayName
		if dn == "" {
			dn = c.AccountID
		}
		out = append(out, dto.EnvBreakdownItem{
			Environment:        env,
			AccountID:          c.AccountID,
			AccountDisplayName: dn,
			TotalCost:          total,
			PreviousPeriodCost: prev,
			ChangePct:          changePct,
		})
	}
	return out
}

// metricTypeForPeriod last_month/last_quarter/last_year/this_year/quarter 用 payment（与今年同口径，YTD=当季时一致）。[Ref: 聚合表主路径、用户需求 今年=这季度]
func metricTypeForPeriod(reportType string) string {
	switch reportType {
	case "last_month", "last_quarter", "last_year", "this_year", "quarter":
		return "payment"
	default:
		return "consumption"
	}
}

// drilldownMetricType 云产品明细与双轨对齐：finance 用与聚合表一致的 metricType；technical 用消耗口径。[Ref: 03_Phase6/01_FinOps]
func drilldownMetricType(track, reportType string) string {
	if track == "finance" {
		return metricTypeForPeriod(reportType)
	}
	return "consumption"
}

// globalMetricTypeForTrack 全域 Hero/成本分解 与聚合表读数：finance 同 metricTypeForPeriod；technical 用消耗；空 track 保持旧客户端（仅 metricTypeForPeriod）。[Ref: 03_Phase6/01_FinOps]
func globalMetricTypeForTrack(track, reportType string) string {
	switch track {
	case "finance":
		return metricTypeForPeriod(reportType)
	case "technical":
		return "consumption"
	default:
		return metricTypeForPeriod(reportType)
	}
}

// accountIDsToSlice 从 map 转为 []string；nil 表示不过滤。[Ref: 聚合表主路径]
func accountIDsToSlice(useEnvs bool, idsMap, allMap map[string]bool) []string {
	m := allMap
	if useEnvs && idsMap != nil {
		m = idsMap
	}
	if m == nil || len(m) == 0 {
		return nil
	}
	var out []string
	for k := range m {
		if k != "" {
			out = append(out, k)
		}
	}
	return out
}

// buildEnvBreakdownEmpty 返回与 cost_env_account_config 顺序一致的全 0 环境行。[Ref: 聚合表主路径 无数据]
func buildEnvBreakdownEmpty(configs []postgres.EnvAccountConfig) []dto.EnvBreakdownItem {
	if len(configs) == 0 {
		return nil
	}
	out := make([]dto.EnvBreakdownItem, 0, len(configs))
	for i := range configs {
		c := &configs[i]
		env := c.Environment
		dn := c.DisplayName
		if dn == "" {
			dn = c.AccountID
		}
		out = append(out, dto.EnvBreakdownItem{
			Environment: env, AccountID: c.AccountID, AccountDisplayName: dn,
			TotalCost: 0, PreviousPeriodCost: 0, ChangePct: 0,
		})
	}
	return out
}

// MixedQueryTimeSeries 混合查询：历史 cost_hourly_workload + 当日 Prometheus 合并的时间序列（占位）。
// 供趋势/全域视图使用；Phase4 实现历史表与当日实时数据合并。
func (s *CostService) MixedQueryTimeSeries(ctx context.Context, start, end time.Time, namespace string) ([]dto.GranularCostDataPoint, error) {
	// Phase3 占位：返回空切片；实现时合并 repo.AggregateHourlyWorkloadStats(start,end) 与当日 Prometheus 数据
	return nil, nil
}

// ListCostProjects 返回成本项目及成员环境，供前端项目卡。[Ref: 03_Phase6/03_前端全域成本透视/01_设计]
func (s *CostService) ListCostProjects(ctx context.Context) ([]dto.CostProjectItem, error) {
	rows, err := s.repo.ListCostProjects(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]dto.CostProjectItem, len(rows))
	for i := range rows {
		out[i] = dto.CostProjectItem{
			ID:           rows[i].ID,
			Code:         rows[i].Code,
			Name:         rows[i].Name,
			SortOrder:    rows[i].SortOrder,
			Environments: rows[i].Environments,
		}
	}
	return out, nil
}

// ListNamespaces returns all namespaces with cost summary for the frontend cost table.
func (s *CostService) ListNamespaces(ctx context.Context, period string) ([]dto.NamespaceCostSummary, error) {
	resp, err := s.GetGlobalCost(ctx, period, "payment", nil, nil, "")
	if err != nil {
		return nil, err
	}
	return resp.Namespaces, nil
}

// GetNamespaceCost returns L1 cost for a namespace.
func (s *CostService) GetNamespaceCost(ctx context.Context, namespace string) (*dto.NamespaceCostResponse, error) {
	now := time.Now().UTC()
	start := now.AddDate(0, 0, -7)

	costs, err := s.repo.AggregateDailyNamespaceCosts(ctx, start, now)
	if err != nil {
		return nil, err
	}

	var totalBillable, totalUsage, totalWaste float64
	for _, c := range costs {
		if c.Namespace == namespace {
			totalBillable += c.BillableCost
			totalUsage += c.UsageCost
			totalWaste += c.WasteCost
		}
	}

	efficiency := 0.0
	if totalBillable > 0 {
		efficiency = (totalUsage / totalBillable) * 100
	}

	return &dto.NamespaceCostResponse{
		Namespace: namespace,
		Cost: dto.CostBreakdown{
			Total:      totalBillable + totalUsage + totalWaste,
			Billable:   totalBillable,
			Usage:      totalUsage,
			Waste:      totalWaste,
			Efficiency: efficiency,
		},
		Timestamp: time.Now().UTC(),
	}, nil
}

// GetEnvDrilldown 按环境云产品钻取：返回该环境对应 account 的云产品成本列表。[Ref: 01_设计 §产品分类与按环境钻取、12_API]
// envId: POC|FAT|UAT|PROD；category 可选 compute|network|storage|security；sort 默认 cost_desc。
func (s *CostService) GetEnvDrilldown(ctx context.Context, envId, reportType, periodKey, category, sortOrder string) ([]dto.EnvDrilldownItem, error) {
	configs, err := s.repo.ListEnvAccountConfig(ctx)
	if err != nil {
		return nil, err
	}
	var accountID string
	for _, c := range configs {
		if c.Environment == envId {
			accountID = c.AccountID
			break
		}
	}
	if accountID == "" && envId != "" {
		// 未配置该环境，返回空
		return []dto.EnvDrilldownItem{}, nil
	}
	list, err := s.repo.ListCloudBillAggregateForReportPeriod(ctx, reportType, periodKey, "consumption", nil)
	if err != nil || len(list) == 0 {
		return []dto.EnvDrilldownItem{}, nil
	}
	var pb map[string]float64
	for _, a := range list {
		if a.AccountID == accountID {
			pb = a.ProductBreakdown
			break
		}
	}
	if pb == nil {
		return []dto.EnvDrilldownItem{}, nil
	}
	// 从 product_breakdown 按产品汇总：key 形如 "domain:ProductCode"；方案 B 键前缀映射 category [Ref: 01_设计 §产品分类 方案 B]
	productCosts := make(map[string]float64)
	categoryFromPrefix := make(map[string]string)
	for k, cost := range pb {
		if idx := strings.Index(k, ":"); idx >= 0 && idx < len(k)-1 {
			code := strings.ToUpper(strings.TrimSpace(k[idx+1:]))
			if invalidDrilldownProductCode(code) {
				continue
			}
			productCosts[code] += cost
			prefix := k[:idx]
			if cat := domainPrefixToCategory(prefix); cat != "" {
				categoryFromPrefix[code] = upgradeDrilldownCategory(categoryFromPrefix[code], cat)
			}
		}
	}
	var out []dto.EnvDrilldownItem
	for code, cost := range productCosts {
		if math.Abs(cost) < 0.005 {
			continue
		}
		cat := s.resolveDrilldownCategory(ctx, code, categoryFromPrefix[code])
		if category != "" && cat != category {
			continue
		}
		out = append(out, dto.EnvDrilldownItem{
			ProductCode: code,
			ProductName: FormatAliyunProductDisplayName(code),
			Cost:        cost,
			Category:    cat,
		})
	}
	if sortOrder != "cost_asc" {
		sort.Slice(out, func(i, j int) bool { return math.Abs(out[i].Cost) > math.Abs(out[j].Cost) })
	} else {
		sort.Slice(out, func(i, j int) bool { return math.Abs(out[i].Cost) < math.Abs(out[j].Cost) })
	}
	return out, nil
}

// domainPrefixToCategory 方案 B：product_breakdown 键前缀（中文领域）映射为 API category；仅四大类，其它/其他已合并入计算资源。[Ref: 01_设计 §产品分类；用户需求 仅四大分类]
func domainPrefixToCategory(prefix string) string {
	switch prefix {
	case "计算资源":
		return "compute"
	case "存储":
		return "storage"
	case "网络":
		return "network"
	case "安全":
		return "security"
	case "其它", "其他":
		return "compute"
	default:
		return ""
	}
}

// resolveDrilldownCategory 方案 B：前缀优先，否则查 product_category_mapping；未命中时归入计算资源（仅四大类）。[Ref: 01_设计 §产品分类；用户需求 仅四大分类]
func (s *CostService) resolveDrilldownCategory(ctx context.Context, code string, categoryFromPrefix string) string {
	if categoryFromPrefix != "" {
		return categoryFromPrefix
	}
	cat, _ := s.repo.GetProductCategory(ctx, code)
	if cat != "" && (cat == "compute" || cat == "storage" || cat == "network" || cat == "security") {
		return cat
	}
	return "compute"
}

// drilldownPeriodToDateRange 将 report_type+period_key 转为日期范围，用于聚合表无数据时从日表降级。[Ref: 01_实践 展示与延迟说明]
func drilldownPeriodToDateRange(reportType, periodKey string, now time.Time) (from, to time.Time, ok bool) {
	yesterday := now.AddDate(0, 0, -1).Truncate(24 * time.Hour)
	switch reportType {
	case "1d":
		t, err := time.Parse("2006-01-02", periodKey)
		if err != nil {
			return from, to, false
		}
		t = t.Truncate(24 * time.Hour)
		return t, t, true
	case "this_week":
		var y, w int
		if _, err := fmt.Sscanf(periodKey, "%04d-W%02d", &y, &w); err != nil {
			return from, to, false
		}
		// ISO 周：W01 为含 1 月 4 日的周，其周一为 from
		jan4 := time.Date(y, 1, 4, 0, 0, 0, 0, time.UTC)
		wd := int(jan4.Weekday())
		if wd == 0 {
			wd = 7
		}
		mondayW01 := jan4.AddDate(0, 0, -(wd - 1))
		monday := mondayW01.AddDate(0, 0, (w-1)*7)
		sunday := monday.AddDate(0, 0, 6)
		if yesterday.Before(monday) {
			return from, to, false
		}
		if yesterday.Before(sunday) {
			return monday, yesterday, true
		}
		return monday, sunday, true
	case "7d", "7d_range":
		t, err := time.Parse("2006-01-02", periodKey)
		if err != nil {
			return from, to, false
		}
		to = t.Truncate(24 * time.Hour)
		from = to.AddDate(0, 0, -6)
		return from, to, true
	case "30d":
		t, err := time.Parse("2006-01-02", periodKey)
		if err != nil {
			return from, to, false
		}
		to = t.Truncate(24 * time.Hour)
		from = to.AddDate(0, 0, -29)
		return from, to, true
	case "90d":
		t, err := time.Parse("2006-01-02", periodKey)
		if err != nil {
			return from, to, false
		}
		to = t.Truncate(24 * time.Hour)
		from = to.AddDate(0, 0, -89)
		return from, to, true
	case "last_week":
		t, err := time.Parse("2006-01-02", periodKey)
		if err != nil {
			return from, to, false
		}
		to = t.Truncate(24 * time.Hour)
		from = to.AddDate(0, 0, -6)
		return from, to, true
	// [Ref: 01_实践] 本月/上月钻取降级：period_key 为 YYYY-MM，从月原始表按单月取数
	case "month", "last_month":
		t, err := time.Parse("2006-01", periodKey)
		if err != nil {
			return from, to, false
		}
		from = time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, time.UTC)
		lastDay := time.Date(t.Year(), t.Month()+1, 0, 0, 0, 0, 0, time.UTC)
		if lastDay.After(yesterday) {
			to = yesterday
		} else {
			to = lastDay
		}
		if from.After(to) {
			return from, to, false
		}
		return from, to, true
	// [Ref: 04_01_成本透视真实数据] 季度与年度环比降级：聚合表无历史记录时从月原始表计算
	case "quarter", "last_quarter":
		var y, q int
		if _, err := fmt.Sscanf(periodKey, "%d-Q%d", &y, &q); err != nil || q < 1 || q > 4 {
			return from, to, false
		}
		from = time.Date(y, time.Month((q-1)*3+1), 1, 0, 0, 0, 0, time.UTC)
		// 季度末：Q1=3月末, Q2=6月末, Q3=9月末, Q4=12月末
		lastMonth := time.Month(q * 3)
		to = time.Date(y, lastMonth+1, 0, 0, 0, 0, 0, time.UTC) // 下月第0天 = 本月最后一天
		if to.After(yesterday) {
			to = yesterday
		}
		if from.After(to) {
			return from, to, false
		}
		return from, to, true
	case "this_year", "last_year":
		var y int
		if _, err := fmt.Sscanf(periodKey, "%d", &y); err != nil {
			return from, to, false
		}
		from = time.Date(y, 1, 1, 0, 0, 0, 0, time.UTC)
		to = time.Date(y, 12, 31, 0, 0, 0, 0, time.UTC)
		if to.After(yesterday) {
			to = yesterday
		}
		if from.After(to) {
			return from, to, false
		}
		return from, to, true
	default:
		return from, to, false
	}
}

// GetGlobalDrilldown 全环境云产品明细：合并所有 account 的 product_breakdown 按产品汇总并打 category；env 非 all 时仅汇总该环境对应 account_id。[Ref: 01_设计 D9-8、D6 云产品成本明细索引、12_API；方案 B 键前缀映射]
// 上月/上季度/去年/今年/这季度与 GetGlobalCost 同源（月表现金+天叠），使云产品明细叠加=总环境成本。[Ref: 用户需求 云产品成本明细与总环境成本一致]
// track=finance 时走月现金快路径；track=technical 时走消耗聚合，与 Hero(C) 口径一致。[Ref: 03_Phase6/01_FinOps]
func (s *CostService) GetGlobalDrilldown(ctx context.Context, reportType, periodKey, category, sortOrder, env, track string) ([]dto.EnvDrilldownItem, error) {
	if track == "" {
		track = "technical"
	}
	useMonthCashReportTypes := map[string]bool{"last_month": true, "last_quarter": true, "last_year": true, "this_year": true, "quarter": true}
	if useMonthCashReportTypes[reportType] && (env == "" || env == "all") && track == "finance" {
		cloud, total, pb := s.aggregateCloudBillByPeriod(ctx, reportType, nil)
		if cloud && pb != nil && total > 0 {
			raw := *pb
			productCosts := make(map[string]float64)
			categoryFromPrefix := make(map[string]string)
			for k, cost := range raw {
				if idx := strings.Index(k, ":"); idx >= 0 && idx < len(k)-1 {
					code := strings.ToUpper(strings.TrimSpace(k[idx+1:]))
					if invalidDrilldownProductCode(code) {
						continue
					}
					productCosts[code] += cost
					prefix := k[:idx]
					if cat := domainPrefixToCategory(prefix); cat != "" {
						categoryFromPrefix[code] = upgradeDrilldownCategory(categoryFromPrefix[code], cat)
					}
				}
			}
			var out []dto.EnvDrilldownItem
			for code, cost := range productCosts {
				if math.Abs(cost) < 0.005 {
					continue
				}
				cat := s.resolveDrilldownCategory(ctx, code, categoryFromPrefix[code])
				if category != "" && cat != category {
					continue
				}
				out = append(out, dto.EnvDrilldownItem{
					ProductCode: code,
					ProductName: FormatAliyunProductDisplayName(code),
					Cost:        cost,
					Category:    cat,
				})
			}
			// 月原始现金路径有总额但 product_breakdown 键无法解析为「领域:产品码」时 out 为空；须回退聚合表 payment，避免 finance 明细恒为空而 technical 有数据 [Ref: 03_Phase6/01_FinOps 云产品明细]
			if len(out) > 0 {
				scaleDrilldownListToTotal(out, total)
				if sortOrder != "cost_asc" {
					sort.Slice(out, func(i, j int) bool { return math.Abs(out[i].Cost) > math.Abs(out[j].Cost) })
				} else {
					sort.Slice(out, func(i, j int) bool { return math.Abs(out[i].Cost) < math.Abs(out[j].Cost) })
				}
				return out, nil
			}
		}
	}
	metric := drilldownMetricType(track, reportType)
	list, err := s.repo.ListCloudBillAggregateForReportPeriod(ctx, reportType, periodKey, metric, nil)
	if err != nil {
		return nil, err
	}
	if len(list) == 0 {
		from, to, ok := drilldownPeriodToDateRange(reportType, periodKey, time.Now().UTC())
		if ok {
			ctxFallback, cancel := context.WithTimeout(ctx, FallbackQueryTimeout)
			defer cancel()
			out, _ := s.GetGlobalDrilldownByDateRange(ctxFallback, from, to, category, sortOrder, env, track)
			return out, nil
		}
		return []dto.EnvDrilldownItem{}, nil
	}
	var accountIDs map[string]bool // env 为单个或逗号分隔（如 POC,FAT）时仅包含这些环境映射的 account_id [Ref: 用户需求 环境多选]
	if env != "" && env != "all" {
		envNames := strings.Split(env, ",")
		for i, e := range envNames {
			envNames[i] = strings.TrimSpace(e)
		}
		accountIDs = s.accountIDsForEnvs(ctx, envNames)
		if len(accountIDs) == 0 {
			return []dto.EnvDrilldownItem{}, nil
		}
	}
	var periodTotal float64
	productCosts := make(map[string]float64)
	categoryFromPrefix := make(map[string]string)
	for _, a := range list {
		if accountIDs != nil && !accountIDs[a.AccountID] {
			continue
		}
		periodTotal += a.TotalAmount
		for k, cost := range a.ProductBreakdown {
			if idx := strings.Index(k, ":"); idx >= 0 && idx < len(k)-1 {
				code := strings.ToUpper(strings.TrimSpace(k[idx+1:]))
				if invalidDrilldownProductCode(code) {
					continue
				}
				productCosts[code] += cost
				prefix := k[:idx]
				if cat := domainPrefixToCategory(prefix); cat != "" {
					categoryFromPrefix[code] = upgradeDrilldownCategory(categoryFromPrefix[code], cat)
				}
			}
		}
	}
	var out []dto.EnvDrilldownItem
	for code, cost := range productCosts {
		if math.Abs(cost) < 0.005 {
			continue
		}
		cat := s.resolveDrilldownCategory(ctx, code, categoryFromPrefix[code])
		if category != "" && cat != category {
			continue
		}
		out = append(out, dto.EnvDrilldownItem{
			ProductCode: code,
			ProductName: FormatAliyunProductDisplayName(code),
			Cost:        cost,
			Category:    cat,
		})
	}
	// 聚合行有金额但 product_breakdown 无「领域:产品码」键时 out 为空；降级月原始表按环境汇总 [Ref: 云产品明细与 Hero 一致]
	if len(out) == 0 {
		from, to, ok := drilldownPeriodToDateRange(reportType, periodKey, time.Now().UTC())
		if ok {
			ctxFallback, cancel := context.WithTimeout(ctx, FallbackQueryTimeout)
			defer cancel()
			alt, errAlt := s.GetGlobalDrilldownByDateRange(ctxFallback, from, to, category, sortOrder, env, track)
			if errAlt == nil && len(alt) > 0 {
				return alt, nil
			}
		}
	}
	scaleDrilldownListToTotal(out, periodTotal)
	if sortOrder != "cost_asc" {
		sort.Slice(out, func(i, j int) bool { return math.Abs(out[i].Cost) > math.Abs(out[j].Cost) })
	} else {
		sort.Slice(out, func(i, j int) bool { return math.Abs(out[i].Cost) < math.Abs(out[j].Cost) })
	}
	return out, nil
}

// mergeMonthlyRawProductBreakdown 按账期汇总月原始表：financial=true 为现金口径；false 为消耗/账单 total_amount 口径。[Ref: 01_实践 自定义月钻取需领域:产品码] [Ref: 03_Phase6/01_FinOps drilldown track]
func (s *CostService) mergeMonthlyRawProductBreakdown(ctx context.Context, billingCycle string, accountIDs map[string]bool, financial bool) (periodTotal float64, productBreakdown map[string]float64) {
	list, err := s.repo.ListCloudBillMonthlyRawByCycle(ctx, billingCycle)
	if err != nil || len(list) == 0 {
		return 0, nil
	}
	productBreakdown = make(map[string]float64)
	for i := range list {
		acc := list[i].AccountID
		if accountIDs != nil && !accountIDs[acc] {
			continue
		}
		mon := &list[i]
		if financial {
			periodTotal += mon.CashTotalAmount
			pb := mon.CashProductBreakdown
			hasProductKeys := false
			if pb != nil {
				for k := range pb {
					if strings.Contains(k, ":") && strings.Index(k, ":") < len(k)-1 {
						hasProductKeys = true
						break
					}
				}
			}
			if !hasProductKeys && mon.ProductBreakdown != nil {
				pb = mon.ProductBreakdown
			}
			if pb != nil {
				for k, v := range pb {
					if idx := strings.Index(k, ":"); idx >= 0 && idx < len(k)-1 {
						productBreakdown[k] += v
					}
				}
			}
		} else {
			periodTotal += mon.TotalAmount
			pb := mon.ProductBreakdown
			hasProductKeys := false
			if pb != nil {
				for k := range pb {
					if strings.Contains(k, ":") && strings.Index(k, ":") < len(k)-1 {
						hasProductKeys = true
						break
					}
				}
			}
			if !hasProductKeys && mon.CashProductBreakdown != nil {
				pb = mon.CashProductBreakdown
			}
			if pb != nil {
				for k, v := range pb {
					if idx := strings.Index(k, ":"); idx >= 0 && idx < len(k)-1 {
						productBreakdown[k] += v
					}
				}
			}
		}
	}
	return periodTotal, productBreakdown
}

// GetGlobalDrilldownByDateRange 全环境云产品明细（自定义日期）：从月原始表按 [from,to] 逐月聚合并打 category；支持最多 5 年（60 个月）。[Ref: 01_实践 月源数据保留近5年] 明细和缩放至同期总环境成本。
func (s *CostService) GetGlobalDrilldownByDateRange(ctx context.Context, from, to time.Time, category, sortOrder, env, track string) ([]dto.EnvDrilldownItem, error) {
	if track == "" {
		track = "technical"
	}
	financial := track == "finance"
	const maxMonths = 60
	if to.Before(from) {
		from, to = to, from
	}
	var accountIDs map[string]bool
	if env != "" && env != "all" {
		envNames := strings.Split(env, ",")
		for i, e := range envNames {
			envNames[i] = strings.TrimSpace(e)
		}
		accountIDs = s.accountIDsForEnvs(ctx, envNames)
		if len(accountIDs) == 0 {
			return []dto.EnvDrilldownItem{}, nil
		}
	}

	var periodTotal float64
	productCosts := make(map[string]float64)
	categoryFromPrefix := make(map[string]string)
	cur := time.Date(from.Year(), from.Month(), 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(to.Year(), to.Month(), 1, 0, 0, 0, 0, time.UTC)
	count := 0
	drillAccountIDs := s.accountIDsFromAllConfig(ctx)
	if len(drillAccountIDs) == 0 {
		drillAccountIDs = nil
	}
	if accountIDs != nil {
		drillAccountIDs = accountIDs
	}
	for !cur.After(end) && count < maxMonths {
		cycle := cur.Format("2006-01")
		ct, pb := s.mergeMonthlyRawProductBreakdown(ctx, cycle, drillAccountIDs, financial)
		periodTotal += ct
		for k, cost := range pb {
			if idx := strings.Index(k, ":"); idx >= 0 && idx < len(k)-1 {
				code := strings.ToUpper(strings.TrimSpace(k[idx+1:]))
				if invalidDrilldownProductCode(code) {
					continue
				}
				productCosts[code] += cost
				prefix := k[:idx]
				if cat := domainPrefixToCategory(prefix); cat != "" {
					categoryFromPrefix[code] = upgradeDrilldownCategory(categoryFromPrefix[code], cat)
				}
			}
		}
		cur = cur.AddDate(0, 1, 0)
		count++
	}
	var out []dto.EnvDrilldownItem
	for code, cost := range productCosts {
		if math.Abs(cost) < 0.005 {
			continue
		}
		cat := s.resolveDrilldownCategory(ctx, code, categoryFromPrefix[code])
		if category != "" && cat != category {
			continue
		}
		out = append(out, dto.EnvDrilldownItem{
			ProductCode: code,
			ProductName: FormatAliyunProductDisplayName(code),
			Cost:        cost,
			Category:    cat,
		})
	}
	scaleDrilldownListToTotal(out, periodTotal)
	if sortOrder != "cost_asc" {
		sort.Slice(out, func(i, j int) bool { return math.Abs(out[i].Cost) > math.Abs(out[j].Cost) })
	} else {
		sort.Slice(out, func(i, j int) bool { return math.Abs(out[i].Cost) < math.Abs(out[j].Cost) })
	}
	return out, nil
}

// CostTrendMaxDays 趋势 API 最大天数 [Ref: 12_API GET /api/v1/cost/trend]
const CostTrendMaxDays = 90

// CostTrendTimeout 趋势查询超时 [Ref: 01_设计 §成本趋势 API]
const CostTrendTimeout = 10 * time.Second

// dailyRowAmountAndPB 日原始表双轨：technical=消耗（Pretax）；finance=现金（Cash）。与 mergeMonthlyRawByCycle 语义一致。[Ref: 06_ 云账单三表、03_Phase6/01_FinOps]
func dailyRowAmountAndPB(r postgres.CloudBillDailyRaw, useConsumption bool) (float64, map[string]float64) {
	if useConsumption {
		pb := r.ProductBreakdown
		if pb == nil {
			pb = make(map[string]float64)
		}
		return r.TotalAmount, pb
	}
	pb := r.CashProductBreakdown
	if pb == nil {
		pb = make(map[string]float64)
	}
	return r.CashTotalAmount, pb
}

// GetCostTrend 成本结构趋势：按日/按月返回序列。[Ref: 01_设计 D9-9、12_API GET /api/v1/cost/trend]
// 月基时间范围（last_month/last_quarter/last_year/quarter/this_year）→ 按月数据点从 monthly_raw 读取。
// 日基时间范围（7d/30d/90d/custom）→ 按日数据点从 daily_raw 读取，最大 90 天、超时 10s。
// envFilter 非空且非"all"时按环境 account_id 过滤。
// track：与全域/钻取一致；technical=消耗口径；finance 或空=现金口径（与默认聚合一致）。[Ref: 03_Phase6/01_FinOps双轨]
func (s *CostService) GetCostTrend(ctx context.Context, period string, dateFrom, dateTo *time.Time, envFilter string, track string) (*dto.CostTrendResponse, error) {
	useConsumption := track == "technical"
	// 自定义月份范围优先 → 月粒度趋势
	if dateFrom != nil && dateTo != nil {
		fromY, fromM := dateFrom.Year(), int(dateFrom.Month())
		toY, toM := dateTo.Year(), int(dateTo.Month())
		return s.monthlyTrend(ctx, fromY, fromM, toY, toM, useConsumption)
	}
	// [Ref: 16_ §七] 单月趋势用日粒度（趋势图需多数据点），多月趋势用月粒度
	switch period {
	case "last_month":
		prev := time.Now().UTC().AddDate(0, -1, 0)
		from := time.Date(prev.Year(), prev.Month(), 1, 0, 0, 0, 0, time.UTC)
		to := from.AddDate(0, 1, -1)
		return s.dailyTrend(ctx, from, to, "", useConsumption)
	case "month", "":
		now := time.Now().UTC()
		from := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
		to := now.AddDate(0, 0, -1).Truncate(24 * time.Hour)
		if to.Before(from) {
			to = from
		}
		return s.dailyTrend(ctx, from, to, "", useConsumption)
	case "quarter":
		now := time.Now().UTC()
		q := (int(now.Month())-1)/3 + 1
		sm := (q-1)*3 + 1
		return s.monthlyTrend(ctx, now.Year(), sm, now.Year(), sm+2, useConsumption)
	case "last_quarter":
		now := time.Now().UTC()
		curMonth := int(now.Month())
		curYear := now.Year()
		var sm, sy int
		switch {
		case curMonth <= 3:
			sm, sy = 10, curYear-1
		case curMonth <= 6:
			sm, sy = 1, curYear
		case curMonth <= 9:
			sm, sy = 4, curYear
		default:
			sm, sy = 7, curYear
		}
		return s.monthlyTrend(ctx, sy, sm, sy, sm+2, useConsumption)
	case "this_year":
		y := time.Now().UTC().Year()
		return s.monthlyTrend(ctx, y, 1, y, int(time.Now().UTC().Month()), useConsumption)
	case "last_year":
		y := time.Now().UTC().Year() - 1
		return s.monthlyTrend(ctx, y, 1, y, 12, useConsumption)
	}

	// 其他情况默认本月日趋势
	now2 := time.Now().UTC()
	from := time.Date(now2.Year(), now2.Month(), 1, 0, 0, 0, 0, time.UTC)
	to := now2.AddDate(0, 0, -1).Truncate(24 * time.Hour)
	if to.Before(from) {
		to = from
	}
	return s.dailyTrend(ctx, from, to, envFilter, useConsumption)
}

// dailyTrend 按日数据点构建趋势，用于单月时间范围（last_month/month）。[Ref: 16_ §七]
func (s *CostService) dailyTrend(ctx context.Context, from, to time.Time, envFilter string, useConsumption bool) (*dto.CostTrendResponse, error) {
	var filterAccountID string
	if envFilter != "" && envFilter != "all" {
		if configs, err := s.repo.ListEnvAccountConfig(ctx); err == nil {
			for _, cfg := range configs {
				if strings.EqualFold(cfg.Environment, envFilter) {
					filterAccountID = cfg.AccountID
					break
				}
			}
		}
		if filterAccountID == "" {
			var empty []dto.CostTrendDataPoint
			for t := from; !t.After(to); t = t.AddDate(0, 0, 1) {
				empty = append(empty, dto.CostTrendDataPoint{Date: t.Format("2006-01-02")})
			}
			return &dto.CostTrendResponse{Data: empty}, nil
		}
	}
	ctxTrend, cancel := context.WithTimeout(ctx, CostTrendTimeout)
	defer cancel()
	rows, err := s.repo.ListCloudBillDailyRawFromTo(ctxTrend, from, to, "")
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return nil, ErrFallbackTimeout
		}
		return nil, err
	}
	byDate := make(map[string]*dto.CostTrendDataPoint)
	for _, r := range rows {
		if filterAccountID != "" && r.AccountID != filterAccountID {
			continue
		}
		d := r.BillDate.Format("2006-01-02")
		if byDate[d] == nil {
			byDate[d] = &dto.CostTrendDataPoint{Date: d, ByDomain: make(map[string]float64), ByProduct: make(map[string]float64)}
		}
		amt, pbMap := dailyRowAmountAndPB(r, useConsumption)
		byDate[d].TotalCost += amt
		for k, cost := range pbMap {
			if cost == 0 {
				continue
			}
			if strings.Contains(k, ":") {
				code := k[strings.Index(k, ":")+1:]
				byDate[d].ByProduct[code] += cost
			} else {
				byDate[d].ByDomain[k] += cost
			}
		}
	}
	var data []dto.CostTrendDataPoint
	for t := from; !t.After(to); t = t.AddDate(0, 0, 1) {
		d := t.Format("2006-01-02")
		if p, ok := byDate[d]; ok {
			data = append(data, *p)
		} else {
			data = append(data, dto.CostTrendDataPoint{Date: d})
		}
	}
	return &dto.CostTrendResponse{Data: data}, nil
}

// monthlyTrend 按月数据点构建趋势。[Ref: 16_ §七 步骤⑨]
func (s *CostService) monthlyTrend(ctx context.Context, startYear, startMonth, endYear, endMonth int, useConsumption bool) (*dto.CostTrendResponse, error) {
	var data []dto.CostTrendDataPoint
	y, m := startYear, startMonth
	for {
		cycle := fmt.Sprintf("%04d-%02d", y, m)
		pt := dto.CostTrendDataPoint{Date: cycle, ByDomain: make(map[string]float64), ByProduct: make(map[string]float64)}
		t, pb, _ := s.mergeMonthlyRawByCycle(ctx, cycle, nil, useConsumption)
		pt.TotalCost = t
		for k, v := range pb {
			if v == 0 {
				continue
			}
			if strings.Contains(k, ":") {
				code := k[strings.Index(k, ":")+1:]
				pt.ByProduct[code] += v
			} else {
				pt.ByDomain[k] = v
			}
		}
		data = append(data, pt)
		if y > endYear || (y == endYear && m >= endMonth) {
			break
		}
		m++
		if m > 12 {
			m = 1
			y++
		}
	}
	return &dto.CostTrendResponse{Data: data}, nil
}
