import React, { useEffect, useState, useRef, useCallback } from 'react';
import { useNavigate, useSearchParams } from 'umi';
import {
  Button, Statistic, Switch, Alert, Tooltip,
  Select, Segmented, Radio, Modal, Descriptions, Table, DatePicker, Tag, message,
} from 'antd';
import dayjs, { type Dayjs } from 'dayjs';
import {
  LoadingOutlined, QuestionCircleOutlined, InfoCircleOutlined,
  CloudServerOutlined, DatabaseOutlined, GlobalOutlined, SafetyCertificateOutlined,
  ArrowUpOutlined, ArrowDownOutlined, SyncOutlined,
} from '@ant-design/icons';
import { useAppStore } from '@/store';
import {
  AreaChart, Area, LineChart, Line,
  XAxis, YAxis, CartesianGrid, Tooltip as RTooltip,
  ResponsiveContainer, Legend,
} from 'recharts';
import type { ApiError, CostTimeRange, CostCompareMode, CloudProductDrilldownItem, EnvBreakdownItem, ProjectBreakdownItem } from '@/types';
import type { DomainBreakdown } from '@/types';
import { CURRENCY_SYMBOL } from '@/constants';
import { costService, type CostProjectApiItem } from '@/services/costService';

/* ─── Static Config ──────────────────────────────────────────────────────── */
// [Ref: 01_实践 §3.1 DNA front_end_time_ranges]
const TIME_RANGE_OPTIONS: { label: string; value: CostTimeRange }[] = [
  { label: '本月',   value: 'month' },
  { label: '上月',   value: 'last_month' },
  { label: '这季度', value: 'quarter' },
  { label: '上季度', value: 'last_quarter' },
  { label: '今年',   value: 'this_year' },
  { label: '去年',   value: 'last_year' },
  { label: '自定义', value: 'custom' },
];
const TOP_TIME_RANGE_OPTIONS = TIME_RANGE_OPTIONS.filter(o => o.value !== 'custom');
const COMPARE_OPTIONS: { label: string; value: CostCompareMode }[] = [
  { label: '不对比', value: 'none' },
  { label: '对比上一周期', value: 'previous' },
];
// [Ref: 01_设计 §成本分解大类与 API category 映射]
const DOMAIN_TO_CATEGORY: Record<string, string> = {
  '计算资源': 'compute', '存储': 'storage', '网络': 'network', '安全': 'security',
};
const CATEGORY_TO_LABEL: Record<string, string> = {
  compute: '计算资源', storage: '存储', network: '网络', security: '安全',
};
// 五维格 Tooltip：极简提示，不展示恒等式或长说明 [Ref: 03_Phase6/01_FinOps]
const FIVE_DIM_CELL_TIPS: Record<'C' | 'G' | 'P' | 'U' | 'B', string> = {
  C: '与 finops_billing_fact 正额行叠加、后端 consumption_cost 同源（临时程序「应付 Y」）',
  G: '与 finops_billing_fact 负额行叠加（临时程序「回帐 H」）',
  P: 'BSS 已还款 / 月表现金，与临时程序「实付已还款」同源',
  U: '所选时间范围覆盖账期下当月应付在途（outstanding）合计',
  B: '本月：各账户 BSS 可用余额快照。非本月：展示余额相对关系 = 当前快照余额 − 账期内(应付消耗+G 回血)；多月视图后端已按区间汇总 C/G，等价于逐月净额累加后与快照对照（展示用，非云侧实时余额）。[Ref: 03_Phase6/01_FinOps]',
};
// Domain visual metadata
const DOMAIN_META: Record<string, { icon: React.ReactNode; color: string; gradStart: string; gradEnd: string }> = {
  '计算资源': { icon: <CloudServerOutlined />,      color: '#3b82f6', gradStart: 'rgba(59,130,246,0.18)',  gradEnd: 'rgba(59,130,246,0.04)'  },
  '存储':     { icon: <DatabaseOutlined />,          color: '#8b5cf6', gradStart: 'rgba(139,92,246,0.18)',  gradEnd: 'rgba(139,92,246,0.04)'  },
  '网络':     { icon: <GlobalOutlined />,            color: '#06b6d4', gradStart: 'rgba(6,182,212,0.18)',   gradEnd: 'rgba(6,182,212,0.04)'   },
  '安全':     { icon: <SafetyCertificateOutlined />, color: '#f59e0b', gradStart: 'rgba(245,158,11,0.18)',  gradEnd: 'rgba(245,158,11,0.04)'  },
};
/** 标准 5 域 cron「分 时 * * *」单行日触发：UTC 钟点与北京时间（UTC+8，无夏令时）[Ref: 03_Phase4/01_成本] */
function etlDailyTriggerHintUTC(cronExpr: string): string {
  const parts = cronExpr.trim().split(/\s+/);
  if (parts.length < 5) return '';
  const minute = parts[0];
  const hourField = parts[1];
  if (hourField === '*' || /[/,\-]/.test(hourField)) return '';
  const h = parseInt(hourField, 10);
  if (Number.isNaN(h)) return '';
  const mm = minute === '*' ? 0 : parseInt(minute, 10);
  const m = Number.isNaN(mm) ? 0 : mm;
  const utcStr = `${String(h).padStart(2, '0')}:${String(m).padStart(2, '0')}`;
  const bjH = (h + 8) % 24;
  const bjStr = `${String(bjH).padStart(2, '0')}:${String(m).padStart(2, '0')}`;
  return `每日 UTC ${utcStr} 自动触发（约北京时间 ${bjStr}）。`;
}

/** 与 GET /finops/effective-config 的 etl_schedule_cron 一致，避免写死「9 点」与部署 18 点等配置不符 [Ref: 03_Phase6/01_FinOps] */
function buildBillDynamicSyncTooltip(etlScheduleCron?: string): string {
  const cron = (etlScheduleCron && etlScheduleCron.trim()) || '0 1 * * *';
  const hint = etlDailyTriggerHintUTC(cron);
  return (
    '当前视图含未关账或滚动更新的账单区间，展示金额可能随云侧出账变化。' +
    (hint || `当前生效定时表达式（UTC）：${cron}。`) +
    ' 实际以进程环境变量 ETL_SCHEDULE_CRON 与 effective-config 为准；亦可点「同步数据」立即拉取。'
  );
}
/** 「同步数据」按钮：手动触发异步任务，与定时 ETL 互补 [Ref: 03_Phase6/01_FinOps 主动同步] */
const FINOPS_SYNC_MANUAL_TOOLTIP =
  '手动触发 FinOps 辅助数据与云账单流水线异步任务，用于立即拉取最新账单并对齐聚合；与每日定时自动拉取互补。进行中会显示步骤进度，完成后本页数据将自动刷新。';

/** 应付消耗：全环境 / 环境卡 / 项目卡同源，优先 consumption_cost，否则 total_cost [Ref: 03_Phase6/01_FinOps] */
function payableConsumptionAmount(row: { total_cost?: number; consumption_cost?: number }): number {
  if (row.consumption_cost != null && !Number.isNaN(Number(row.consumption_cost))) {
    return Number(row.consumption_cost);
  }
  return Number(row.total_cost ?? 0);
}

/** 实付 P：仅认 ledger_p；无有效值返回 null（与「应付」分列，禁止与 total_cost 混用）[Ref: 03_Phase6/03_前端全域成本透视] */
function cardActualPaidP(row: { ledger_p?: number }): number | null {
  if (row.ledger_p != null && !Number.isNaN(Number(row.ledger_p))) {
    return Number(row.ledger_p);
  }
  return null;
}

/** 有 project_breakdown 时 Hero/环比用：未选 URL 项目 id 视为「全选」叠加 [Ref: 03_Phase6/03_前端全域成本透视/01_设计] */
function projectRowsForSelection(
  proj: ProjectBreakdownItem[] | undefined,
  selectedIds: number[],
): ProjectBreakdownItem[] {
  if (!proj?.length) return [];
  if (!selectedIds.length) return proj;
  return proj.filter((p) => selectedIds.includes(p.project_id));
}

function fmtDimOrDash(v: number | null, fmtDim: (n: number) => string): string {
  if (v == null) return '—';
  return fmtDim(v);
}

/** 单卡净额 N ≈ 应付消耗 + 回血(G≤0)，与临时程序 Y+H / OSS SUM(amount) 一致 [Ref: 03_Phase6/01_FinOps] */
function finopsNetAmountForCard(row: { total_cost?: number; consumption_cost?: number; ledger_g?: number }): number {
  return payableConsumptionAmount(row) + Number(row.ledger_g ?? 0);
}

/** 页顶选「本月」：余额维仅用 BSS 快照，不叠净额 [Ref: 03_Phase6/01_FinOps UX] */
function isCurrentMonthCostView(costTimeRange: CostTimeRange): boolean {
  return costTimeRange === 'month';
}

/**
 * 环境/项目卡 B「余额」：本月 = ledger_b 快照；历史/自定义月 = 快照 − 账期内净消耗(应付+G)，多月时行内 C/G 已为区间汇总，与逐月叠加净额一致。[Ref: 03_Phase6/01_FinOps]
 */
function displayedBalanceForCard(
  ledgerB: number | undefined,
  row: { total_cost?: number; consumption_cost?: number; ledger_g?: number },
  costTimeRange: CostTimeRange,
): number {
  const b = Number(ledgerB ?? 0);
  if (isCurrentMonthCostView(costTimeRange)) return b;
  const net = finopsNetAmountForCard(row);
  return b - net;
}

const ENV_COLORS: Record<string, string> = { POC: '#3b82f6', FAT: '#10b981', UAT: '#f59e0b', PROD: '#ef4444' };
const ENV_SELECTED_BG: Record<string, [string, string]> = {
  POC:  ['rgba(59,130,246,0.15)',  'rgba(59,130,246,0.08)'],
  FAT:  ['rgba(16,185,129,0.15)',  'rgba(16,185,129,0.08)'],
  UAT:  ['rgba(245,158,11,0.15)', 'rgba(245,158,11,0.08)'],
  PROD: ['rgba(239,68,68,0.15)',  'rgba(239,68,68,0.08)'],
};

/** 未知环境名：稳定 HSL 色，与预置四环境并存 [Ref: 03_Phase6/01_FinOps 多环境] */
function envHue(env: string): number {
  let h = 0;
  for (let i = 0; i < env.length; i++) h = (h * 31 + env.charCodeAt(i)) % 360;
  return h;
}
function getEnvColor(env: string): string {
  const c = ENV_COLORS[env];
  if (c) return c;
  return `hsl(${envHue(env)}, 58%, 52%)`;
}
function getEnvSelectedBg(env: string, isDark: boolean): string {
  const pair = ENV_SELECTED_BG[env];
  if (pair) return pair[isDark ? 0 : 1] ?? '';
  const h = envHue(env);
  return isDark ? `hsla(${h}, 55%, 50%, 0.18)` : `hsla(${h}, 55%, 50%, 0.08)`;
}

type DetailModalType = 'bill' | 'efficiency' | null;

/* ─── Component ──────────────────────────────────────────────────────────── */
const CostOverviewPage: React.FC = () => {
  const navigate = useNavigate();
  const [searchParams, setSearchParams] = useSearchParams();

  // Umi 4 兼容：避免 setSearchParams 回调模式不触发重渲染，统一用直接赋值
  const updateParams = (updater: (p: URLSearchParams) => void, opts?: { replace?: boolean }) => {
    const next = new URLSearchParams(searchParams);
    updater(next);
    setSearchParams(next, opts);
  };

  const [detailModal, setDetailModal] = useState<DetailModalType>(null);
  const [domainDetail, setDomainDetail] = useState<DomainBreakdown | null>(null);
  const [highlightCloudProduct, setHighlightCloudProduct] = useState(false);
  const [trendModalOpen, setTrendModalOpen] = useState(false);
  /** 从云产品明细行点击趋势图打开大图时，展示该产品趋势；否则展示总成本趋势 [Ref: 单产品趋势大图] */
  const [trendModalProductCode, setTrendModalProductCode] = useState<string | null>(null);
  /** FinOps 主动同步 Job（POST + 轮询 GET + 步骤进度填充条） [Ref: 03_Phase6/01_FinOps 主动同步] */
  const [finopsEtlCron, setFinopsEtlCron] = useState<string | undefined>(undefined);
  const [finopsSyncLoading, setFinopsSyncLoading] = useState(false);
  const [finopsSyncPoll, setFinopsSyncPoll] = useState<{
    pct: number;
    phaseDetail: string;
    phase: string;
  } | null>(null);
  /** 防止重复轮询（刷新恢复与点击并发） [Ref: 03_Phase6/01_FinOps 主动同步] */
  const finopsPollLockRef = useRef(false);
  const pollFinopsJobUntilDoneRef = useRef<(jobId: number) => Promise<void>>(async () => {});
  /** 项目卡：单击延迟切换多选，双击取消定时并跳转索引区 [Ref: 03_Phase6/03_前端全域成本透视/01_设计] */
  const projectCardClickTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  // [Ref: 用户需求] 分页 state：受控 pageSize + currentPage，确保 showSizeChanger 选择后自动跳回第1页
  const [tablePageSize, setTablePageSize] = useState(20);
  const [tablePage, setTablePage] = useState(1);

  const {
    globalCostMetrics,
    drilldownGlobalProducts, drilldownGlobalProductsPrev,
    loadingGlobalMetrics, loadingDrilldownGlobal,
    errorGlobalMetrics, errorDrilldownGlobal,
    costTimeRange, costCompareMode, costCustomDateRange, selectedDimension,
    fetchGlobalCostMetrics, fetchDrilldownGlobal,
    resetErrors, fetchCostTrend,
    costTrendData, costTrendDataPrev, loadingCostTrend, errorCostTrend,
    setCostTimeRange, setCostCompareMode, setCostCustomDateRange,
    theme,
  } = useAppStore();

  // [Ref: 01_实践 D9-15、D4] 从 URL 恢复时间范围与对比
  useEffect(() => {
    const period = searchParams.get('period');
    const compare = searchParams.get('compare');
    const date_from = searchParams.get('date_from');
    const date_to = searchParams.get('date_to');
    const validPeriods = new Set(TIME_RANGE_OPTIONS.map(o => o.value));
    const validCompares = new Set(COMPARE_OPTIONS.map(o => o.value));
    if (period && validPeriods.has(period as CostTimeRange)) setCostTimeRange(period as CostTimeRange);
    if (compare && validCompares.has(compare as CostCompareMode)) setCostCompareMode(compare as CostCompareMode);
    if ((period === 'custom' || costTimeRange === 'custom') && date_from && date_to) setCostCustomDateRange([date_from, date_to]);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  // 环境多选：URL envs=POC,FAT 或兼容单选 env=POC [Ref: 用户需求 环境多选]
  const envsFromUrl = searchParams.get('envs') || searchParams.get('env') || '';
  /** 须稳定引用：否则每次 render 新建 [] 会触发下方 useEffect 依赖「变化」→ 无限 fetch → React #185 白屏 */
  const selectedEnvs: string[] = React.useMemo(() => {
    if (!envsFromUrl || envsFromUrl === 'all') return [];
    return envsFromUrl.split(',').map(s => s.trim()).filter(Boolean);
  }, [envsFromUrl]);

  /** 成本项目多选，与 GET /api/v1/cost/global?project_ids= 一致 [Ref: 03_Phase6/03_前端全域成本透视/01_设计] */
  const projectIdsFromUrl = React.useMemo(() => {
    const raw = searchParams.get('project_ids');
    if (!raw?.trim()) return [] as number[];
    return raw.split(',').map(s => parseInt(s.trim(), 10)).filter(n => !Number.isNaN(n));
  }, [searchParams]);

  const [costProjects, setCostProjects] = useState<CostProjectApiItem[]>([]);
  useEffect(() => {
    void costService.listCostProjects().then(setCostProjects).catch(() => setCostProjects([]));
  }, []);

  useEffect(() => {
    void costService.getFinOpsEffectiveConfig().then(c => setFinopsEtlCron(c.etl_schedule_cron)).catch(() => {});
  }, []);

  useEffect(() => () => {
    if (projectCardClickTimerRef.current) clearTimeout(projectCardClickTimerRef.current);
  }, []);

  /** URL 已选成本项目时，云环境账户仅展示这些项目所包含的环境 [Ref: 03_Phase6/03_前端全域成本透视/01_设计] */
  const envsForSelectedProjects = React.useMemo(() => {
    if (projectIdsFromUrl.length === 0) return null as string[] | null;
    const set = new Set<string>();
    for (const pid of projectIdsFromUrl) {
      const pr = costProjects.find(p => p.id === pid);
      pr?.environments?.forEach(e => set.add(e));
    }
    return [...set];
  }, [costProjects, projectIdsFromUrl]);

  const primaryProjectNameForEnv = React.useCallback(
    (env: string) => {
      const hits = costProjects.filter(p => p.environments?.includes(env));
      if (hits.length === 0) return null;
      return [...hits].sort((a, b) => (a.sort_order ?? 0) - (b.sort_order ?? 0))[0].name;
    },
    [costProjects],
  );

  const formatCloudEnvCardTitle = (env: string) => {
    const pn = primaryProjectNameForEnv(env);
    return pn ? `${pn}-云-${env}` : env;
  };

  /** 环境卡：以 API env_breakdown 顺序为准；按项目筛选收窄；URL 已选但尚未出现在 breakdown 中的环境追加在末尾 [Ref: 03_Phase6/01_FinOps 多环境] */
  const envCardEnvironments = React.useMemo(() => {
    const br = globalCostMetrics?.envBreakdown;
    let ordered = br && br.length > 0 ? br.map(e => e.environment) : ['POC', 'FAT', 'UAT', 'PROD'];
    if (envsForSelectedProjects != null && envsForSelectedProjects.length > 0) {
      ordered = ordered.filter(e => envsForSelectedProjects.includes(e));
    }
    const seen = new Set(ordered);
    const extra = selectedEnvs.filter(e => e && !seen.has(e));
    return [...ordered, ...extra];
  }, [globalCostMetrics?.envBreakdown, selectedEnvs, envsForSelectedProjects]);

  const envFilterOptions = React.useMemo(() => {
    const br = globalCostMetrics?.envBreakdown;
    if (br && br.length > 0) {
      return br.map(e => ({
        label: e.account_display_name ? `${e.environment} (${e.account_display_name})` : e.environment,
        value: e.environment,
      }));
    }
    return ['POC', 'FAT', 'UAT', 'PROD'].map(e => ({ label: e, value: e }));
  }, [globalCostMetrics?.envBreakdown]);

  const drilldownEnv = selectedEnvs.length ? selectedEnvs.join(',') : 'all';
  const drilldownCategory = searchParams.get('category') || undefined;
  const drilldownSort = searchParams.get('sort') || 'cost_desc';
  /** 固定消耗/应付口径（原 technical），无双视角 [Ref: 03_Phase6/01_FinOps] */
  const API_TRACK = 'technical' as const;
  // [Ref: 用户需求] 默认开启环比与趋势图；通过 ?compare_trend=0 / ?show_trend=0 可显式关闭
  const drilldownCompare = searchParams.get('compare_trend') !== '0';
  const showTrendChart = searchParams.get('show_trend') !== '0';
  const [trendMode, setTrendMode] = React.useState<'total' | 'product'>('total');
  const indexPeriod = searchParams.get('index_period') as CostTimeRange | null;
  const indexDateFrom = searchParams.get('index_date_from') ?? null;
  const indexDateTo = searchParams.get('index_date_to') ?? null;
  const effectiveDrilldownPeriod: CostTimeRange = indexPeriod ?? costTimeRange;
  const effectiveDrilldownDateRange: [string, string] | null =
    (indexPeriod === 'custom' && indexDateFrom && indexDateTo ? [indexDateFrom, indexDateTo] : null) ?? costCustomDateRange;

  const hasPreviousPeriodData = (m?: { envBreakdown?: { previous_period_cost?: number }[] } | null) =>
    (m?.envBreakdown?.some(e => (e.previous_period_cost ?? 0) > 0)) ?? false;

  useEffect(() => {
    fetchGlobalCostMetrics(
      selectedEnvs.length ? selectedEnvs : undefined,
      API_TRACK,
      projectIdsFromUrl.length ? projectIdsFromUrl : undefined,
    );
    fetchDrilldownGlobal(drilldownEnv, drilldownCategory, drilldownSort, { period: effectiveDrilldownPeriod, dateRange: effectiveDrilldownDateRange }, drilldownCompare, API_TRACK);
  }, [fetchGlobalCostMetrics, fetchDrilldownGlobal, costTimeRange, costCompareMode, costCustomDateRange, selectedEnvs, drilldownEnv, drilldownCategory, drilldownSort, effectiveDrilldownPeriod, effectiveDrilldownDateRange, drilldownCompare, projectIdsFromUrl]);

  useEffect(() => {
    const period = effectiveDrilldownPeriod === '7d_range' ? '7d' : effectiveDrilldownPeriod === 'custom' ? undefined : effectiveDrilldownPeriod;
    const dateFrom = effectiveDrilldownDateRange?.[0];
    const dateTo = effectiveDrilldownDateRange?.[1];
    const envParam = drilldownEnv !== 'all' ? drilldownEnv : undefined;
    if (effectiveDrilldownPeriod === 'custom' && dateFrom && dateTo) {
      fetchCostTrend({ date_from: dateFrom, date_to: dateTo, env: envParam, track: API_TRACK }, drilldownCompare);
    } else if (period) {
      fetchCostTrend({ period, env: envParam, track: API_TRACK }, drilldownCompare);
    }
  }, [effectiveDrilldownPeriod, effectiveDrilldownDateRange, drilldownCompare, drilldownEnv, fetchCostTrend]);

  /** 移除 URL 中的 track 参数，避免旧书签切到已删除的「资金经营」视角 */
  useEffect(() => {
    if (!searchParams.get('track')) return;
    updateParams(n => { n.delete('track'); });
  }, []);

  const pollFinopsJobUntilDone = useCallback(
    async (jobId: number) => {
      if (finopsPollLockRef.current) {
        return;
      }
      finopsPollLockRef.current = true;
      setFinopsSyncLoading(true);
      try {
        let terminal: string | null = null;
        /** 最长约 4h；步骤进度来自 progress_current/total [Ref: 03_Phase6/01_FinOps 主动同步] */
        const maxPolls = 7200;
        const pollMs = 2000;
        for (let i = 0; i < maxPolls; i++) {
          if (i > 0) {
            await new Promise<void>(r => setTimeout(r, pollMs));
          }
          const st = await costService.getFinOpsSyncJob(jobId);
          const t = st.progress_total ?? 0;
          const c = st.progress_current ?? 0;
          const pct = t > 0 ? Math.min(100, Math.round((c / t) * 100)) : 0;
          setFinopsSyncPoll({
            pct,
            phaseDetail: (st.phase_detail && String(st.phase_detail).trim()) ? String(st.phase_detail) : (st.phase || '—'),
            phase: st.phase || '',
          });
          if (st.status === 'succeeded' || st.status === 'succeeded_with_warnings' || st.status === 'failed') {
            terminal = st.status;
            if (st.status === 'failed') {
              message.error(st.error_message || '同步失败');
            } else if (st.status === 'succeeded_with_warnings') {
              message.warning(`同步完成（${(st.warnings ?? []).length} 条警告）`);
            } else {
              message.success('同步完成');
            }
            break;
          }
        }
        if (!terminal) {
          message.warning('同步超时，请稍后手动刷新页面');
        }
        await fetchGlobalCostMetrics(
          selectedEnvs.length ? selectedEnvs : undefined,
          API_TRACK,
          projectIdsFromUrl.length ? projectIdsFromUrl : undefined,
        );
        await fetchDrilldownGlobal(
          drilldownEnv,
          drilldownCategory,
          drilldownSort,
          { period: effectiveDrilldownPeriod, dateRange: effectiveDrilldownDateRange },
          drilldownCompare,
          API_TRACK,
        );
      } finally {
        finopsPollLockRef.current = false;
        setFinopsSyncPoll(null);
        setFinopsSyncLoading(false);
      }
    },
    [
      fetchGlobalCostMetrics,
      fetchDrilldownGlobal,
      selectedEnvs,
      projectIdsFromUrl,
      drilldownEnv,
      drilldownCategory,
      drilldownSort,
      effectiveDrilldownPeriod,
      effectiveDrilldownDateRange,
      drilldownCompare,
    ],
  );

  pollFinopsJobUntilDoneRef.current = pollFinopsJobUntilDone;

  /** 刷新后恢复：有 queued/running Job 则自动轮询，无需再点「同步数据」 [Ref: 03_Phase6/01_FinOps 主动同步] */
  useEffect(() => {
    let cancelled = false;
    void (async () => {
      try {
        const st = await costService.getFinOpsSyncJobActive();
        if (cancelled || !st) {
          return;
        }
        if (st.status !== 'queued' && st.status !== 'running') {
          return;
        }
        if (cancelled) {
          return;
        }
        await pollFinopsJobUntilDoneRef.current(st.job_id);
      } catch {
        /* 忽略 active 接口不可用，不打断大盘 */
      }
    })();
    return () => {
      cancelled = true;
    };
  }, []);

  const runFinopsSync = useCallback(async () => {
    if (finopsPollLockRef.current) {
      message.info('同步任务已在进行中');
      return;
    }
    setFinopsSyncLoading(true);
    setFinopsSyncPoll({ pct: 0, phaseDetail: '提交中…', phase: '' });
    let jobId: number;
    try {
      try {
        const r = await costService.createFinOpsSyncJob();
        jobId = r.job_id;
      } catch (e: unknown) {
        const ae = e as ApiError;
        if (ae.code === 'FINOPS_SYNC_ACTIVE' && typeof ae.active_job_id === 'number' && ae.active_job_id > 0) {
          jobId = ae.active_job_id;
          message.info('已有同步任务进行中，正在显示进度…');
        } else if (ae.code === 'FINOPS_SYNC_ACTIVE') {
          message.warning('已有同步任务进行中，请稍后重试');
          setFinopsSyncPoll(null);
          setFinopsSyncLoading(false);
          return;
        } else {
          throw e;
        }
      }
      await pollFinopsJobUntilDone(jobId);
    } catch (e: unknown) {
      const ae = e as ApiError;
      const msg = ae.message || '同步请求失败';
      if (ae.code === 'FINOPS_SYNC_AUTH_REQUIRED' || ae.code === '401') {
        message.error('未授权：后端已启用同步密钥，请配置构建环境变量 FINOPS_SYNC_JOB_KEY 与 FINOPS_SYNC_JOB_API_KEY 一致，或由网关注入请求头');
      } else {
        message.error(msg);
      }
      setFinopsSyncPoll(null);
      setFinopsSyncLoading(false);
    }
  }, [pollFinopsJobUntilDone]);

  /* ─── Theme-aware Color Palette ─────────────────────────────────────────── */
  const isDark = theme === 'dark';

  const gc: React.CSSProperties = {
    background: isDark ? 'rgba(255,255,255,0.04)' : 'rgba(255,255,255,0.76)',
    backdropFilter: 'blur(16px)',
    WebkitBackdropFilter: 'blur(16px)',
    border: `1px solid ${isDark ? 'rgba(255,255,255,0.08)' : 'rgba(255,255,255,0.85)'}`,
    borderRadius: 16,
    boxShadow: isDark ? '0 4px 28px rgba(0,0,0,0.4)' : '0 2px 20px rgba(0,0,0,0.07)',
    overflow: 'hidden',
  };

  const txt1 = isDark ? 'rgba(255,255,255,0.90)' : 'rgba(0,0,0,0.85)';
  const txt2 = isDark ? 'rgba(255,255,255,0.45)' : 'rgba(0,0,0,0.45)';
  const divider = isDark ? 'rgba(255,255,255,0.07)' : 'rgba(0,0,0,0.06)';

  /* ─── Chart Tooltip ─────────────────────────────────────────────────────── */
  const ChartTooltip = ({ active, payload, label }: any) => {
    if (!active || !payload?.length) return null;
    return (
      <div style={{
        background: isDark ? 'rgba(8,18,38,0.96)' : 'rgba(255,255,255,0.97)',
        border: `1px solid ${isDark ? 'rgba(255,255,255,0.1)' : 'rgba(0,0,0,0.08)'}`,
        borderRadius: 10,
        padding: '10px 14px',
        boxShadow: '0 8px 24px rgba(0,0,0,0.18)',
        backdropFilter: 'blur(8px)',
      }}>
        <div style={{ fontSize: 11, color: txt2, marginBottom: 6 }}>{label}</div>
        {payload.map((p: any, i: number) => (
          <div key={i} style={{ fontSize: 13, fontWeight: 600, color: p.stroke || p.color }}>
            {p.name}: {CURRENCY_SYMBOL}{Number(p.value ?? 0).toLocaleString()}
          </div>
        ))}
      </div>
    );
  };

  /* ─── Bill Data Status Badge ─────────────────────────────────────────────── */
  // [Ref: 16_云账单动态对账与高可靠处理规范 §三段式] 展示账单对账状态
  const BillDataStatusBadge = ({ status, isDark: dark }: { status: string; isDark: boolean }) => {
    const cfg: Record<string, { label: string; color: string; bg: string; dot: string }> = {
      FINALIZED:    { label: '已财务核算', color: '#10b981', bg: 'rgba(16,185,129,0.12)', dot: '#10b981' },
      PRELIMINARY:  { label: '动态同步', color: '#f59e0b', bg: 'rgba(245,158,11,0.12)',  dot: '#f59e0b' },
      RECONCILING:  { label: '对账中',    color: '#3b82f6', bg: 'rgba(59,130,246,0.12)',  dot: '#3b82f6' },
      DIRTY:        { label: '数据偏差',  color: '#ef4444', bg: 'rgba(239,68,68,0.12)',   dot: '#ef4444' },
    };
    const c = cfg[status] ?? { label: '动态同步', color: '#f59e0b', bg: 'rgba(245,158,11,0.12)', dot: '#f59e0b' };
    const isPreliminaryLike = status === 'PRELIMINARY' || !(status in cfg);
    const showDynamicHelp = c.label === '动态同步';
    return (
      <span style={{
        display: 'inline-flex', alignItems: 'center', gap: 5,
        fontSize: 10, fontWeight: 600, color: c.color,
        background: dark ? c.bg.replace('0.12', '0.20') : c.bg,
        borderRadius: 20, padding: '2px 8px',
        border: `1px solid ${c.color}30`,
      }}>
        <span style={{
          width: 5, height: 5, borderRadius: '50%', background: c.dot,
          boxShadow: isPreliminaryLike || status === 'RECONCILING' ? `0 0 4px ${c.dot}` : 'none',
          animation: isPreliminaryLike ? 'pulse 2s infinite' : 'none',
          flexShrink: 0,
        }} />
        {c.label}
        {showDynamicHelp && (
          <Tooltip title={buildBillDynamicSyncTooltip(finopsEtlCron)} placement="bottom" overlayStyle={{ maxWidth: 360 }}>
            <InfoCircleOutlined
              role="img"
              aria-label="动态同步说明"
              style={{ fontSize: 11, color: c.color, opacity: 0.9, cursor: 'help' }}
              onClick={e => e.stopPropagation()}
            />
          </Tooltip>
        )}
      </span>
    );
  };

  /* ─── Change Badge ───────────────────────────────────────────────────────── */
  const ChangeBadge = ({ pct, inverted = false }: { pct: number | null; inverted?: boolean }) => {
    if (pct === null) return null;
    const isPositive = pct >= 0;
    const isBad = inverted ? !isPositive : isPositive;
    const color = isBad ? '#ef4444' : '#10b981';
    const bg = isBad ? 'rgba(239,68,68,0.1)' : 'rgba(16,185,129,0.1)';
    return (
      <span style={{
        display: 'inline-flex', alignItems: 'center', gap: 2,
        fontSize: 11, fontWeight: 600, color,
        background: bg, borderRadius: 20, padding: '2px 7px', marginLeft: 6,
      }}>
        {isPositive ? <ArrowUpOutlined style={{ fontSize: 9 }} /> : <ArrowDownOutlined style={{ fontSize: 9 }} />}
        {Math.abs(pct).toFixed(1)}%
      </span>
    );
  };

  /** Hero 主数字：实付 P = 所选项目各行 ledger_p 之和；若项目行均未带 P 则与同响应 env_breakdown 行叠加，再回退 ledger.P [Ref: 03_Phase6/03_前端全域成本透视] */
  const heroPaidSum = React.useMemo(() => {
    const m = globalCostMetrics;
    if (!m) return 0;
    const proj = m.projectBreakdown;
    if (proj?.length) {
      const rows = projectRowsForSelection(proj, projectIdsFromUrl);
      const fromProj = rows.reduce((s, p) => s + (cardActualPaidP(p) ?? 0), 0);
      const anyProjP = rows.some(p => cardActualPaidP(p) != null);
      if (anyProjP) return fromProj;
      const br = m.envBreakdown;
      if (br?.length) {
        const fromEnv = br.reduce((s, e) => s + (cardActualPaidP(e) ?? 0), 0);
        const anyEnvP = br.some(e => cardActualPaidP(e) != null);
        if (anyEnvP) return fromEnv;
      }
      const pL = m.ledger?.P;
      if (pL != null && !Number.isNaN(Number(pL))) return Number(pL);
      return fromProj;
    }
    const br = m.envBreakdown;
    if (br?.length) {
      return br.reduce((s, e) => s + (cardActualPaidP(e) ?? 0), 0);
    }
    const pL = m.ledger?.P;
    if (pL != null && !Number.isNaN(Number(pL))) return Number(pL);
    return 0;
  }, [globalCostMetrics, projectIdsFromUrl]);

  /** Hero 副数字：应付消耗 = 与主数字同一批行上 payableConsumptionAmount 之和 [Ref: 03_Phase6/03_前端全域成本透视] */
  const heroPayableSum = React.useMemo(() => {
    const m = globalCostMetrics;
    if (!m) return 0;
    const proj = m.projectBreakdown;
    if (proj?.length) {
      const rows = projectRowsForSelection(proj, projectIdsFromUrl);
      return rows.reduce((s, p) => s + payableConsumptionAmount(p), 0);
    }
    const br = m.envBreakdown;
    if (br?.length) {
      return br.reduce((s, e) => s + payableConsumptionAmount(e), 0);
    }
    const api = m.totalBillableCost;
    if (api != null && !Number.isNaN(Number(api))) return Number(api);
    return 0;
  }, [globalCostMetrics, projectIdsFromUrl]);

  /** 与 Hero/项目卡展示一致：有应付、实付或任一带量五维时不提示「本期无消费」[Ref: 03_Phase6/03_前端全域成本透视] */
  const NO_SPEND_EPS = 0.005;
  const hasMeaningfulDisplayedSpend = React.useMemo(() => {
    const m = globalCostMetrics;
    if (!m) return false;
    if (Math.abs(heroPayableSum) >= NO_SPEND_EPS || Math.abs(heroPaidSum) >= NO_SPEND_EPS) return true;
    const L = m.ledger;
    if (L) {
      for (const k of ['C', 'G', 'P', 'U', 'B'] as const) {
        const v = L[k];
        if (v != null && !Number.isNaN(Number(v)) && Math.abs(Number(v)) >= NO_SPEND_EPS) return true;
      }
    }
    if (m.domainBreakdown?.some(d => (d.cost ?? 0) >= NO_SPEND_EPS)) return true;
    return false;
  }, [globalCostMetrics, heroPayableSum, heroPaidSum]);

  /** 全环境账期净额 N≈C+G：与所选项目/环境行 finopsNet 加总一致，供历史视图 Hero B（余额）= 快照 − N [Ref: 03_Phase6/01_FinOps] */
  const heroNetForBalanceDisplay = React.useMemo(() => {
    const m = globalCostMetrics;
    if (!m) return 0;
    const proj = m.projectBreakdown;
    if (proj?.length) {
      const rows = projectRowsForSelection(proj, projectIdsFromUrl);
      if (rows.length) {
        return rows.reduce((s, p) => s + finopsNetAmountForCard(p), 0);
      }
    }
    const cRaw = m.ledger?.C;
    const c =
      cRaw != null && !Number.isNaN(Number(cRaw))
        ? Number(cRaw)
        : heroPayableSum;
    const g = m.ledger?.G != null && !Number.isNaN(Number(m.ledger.G)) ? Number(m.ledger.G) : 0;
    return c + g;
  }, [globalCostMetrics, projectIdsFromUrl, heroPayableSum]);

  const heroDisplayedBalanceB = React.useMemo(() => {
    const m = globalCostMetrics;
    const b = Number(m?.ledger?.B ?? 0);
    if (isCurrentMonthCostView(costTimeRange)) return b;
    return b - heroNetForBalanceDisplay;
  }, [globalCostMetrics, costTimeRange, heroNetForBalanceDisplay]);

  const prevTotalCost = globalCostMetrics?.previousPeriod?.totalBillableCost ?? null;

  /** 环比：仅全量视图下用「应付」与上期 totalBillableCost」比；带 project_ids 时上期未按项目剖分，避免误导读数 [Ref: 03_Phase6/03_前端全域成本透视] */
  const heroChangePct =
    costCompareMode === 'previous' &&
    projectIdsFromUrl.length === 0 &&
    prevTotalCost &&
    prevTotalCost > 0
      ? ((heroPayableSum - prevTotalCost) / prevTotalCost) * 100
      : null;

  const heroTitleLabel = '全环境 · 实付 (P)';

  const envCardDimC = (item: EnvBreakdownItem) => payableConsumptionAmount(item);
  const envCardDimG = (item: EnvBreakdownItem) => Number(item.ledger_g ?? 0);
  const envCardDimB = (item: EnvBreakdownItem) => displayedBalanceForCard(item.ledger_b, item, costTimeRange);

  const projectCardDimC = (item: ProjectBreakdownItem) => payableConsumptionAmount(item);
  const projectCardDimG = (item: ProjectBreakdownItem) => Number(item.ledger_g ?? 0);
  const projectCardDimB = (item: ProjectBreakdownItem) => displayedBalanceForCard(item.ledger_b, item, costTimeRange);

  const fmtDim = (v: number) =>
    `${CURRENCY_SYMBOL}${Number(v).toLocaleString(undefined, { minimumFractionDigits: 2, maximumFractionDigits: 2 })}`;

  /* ─── Render: Filter Bar ─────────────────────────────────────────────────── */
  const renderFilterBar = () => (
    <div className="bento-card" style={{ ...gc, padding: '14px 20px', marginBottom: 14 }}>
      <div style={{ display: 'flex', flexWrap: 'wrap', gap: 16, alignItems: 'flex-start' }}>
        {/* Time range */}
        <div>
          <div style={{ fontSize: 11, color: txt2, marginBottom: 6, letterSpacing: '0.04em', textTransform: 'uppercase', fontWeight: 500 }}>
            时间范围
          </div>
          <div style={{ display: 'flex', flexWrap: 'wrap', gap: 6, alignItems: 'center' }}>
            <Radio.Group
              value={costTimeRange === 'custom' ? undefined : costTimeRange}
              onChange={e => {
                const val = e.target.value as CostTimeRange;
                setCostCustomDateRange(null);
                setCostTimeRange(val);
                updateParams(n => {
                  n.set('period', val);
                  n.delete('date_from');
                  n.delete('date_to');
                  n.delete('index_period');
                  n.delete('index_date_from');
                  n.delete('index_date_to');
                });
              }}
              optionType="button"
              buttonStyle="solid"
              size="small"
              options={TOP_TIME_RANGE_OPTIONS}
              style={{ display: 'flex', flexWrap: 'wrap', gap: 3 }}
            />
            {/* [Ref: 01_实践 §时间范围] 自定义月份范围入口：与预设按钮互斥，最多 3 年 */}
            <DatePicker.RangePicker
              picker="month"
              size="small"
              allowClear
              placeholder={['开始月份', '结束月份']}
              value={
                costTimeRange === 'custom' && costCustomDateRange
                  ? [dayjs(costCustomDateRange[0]), dayjs(costCustomDateRange[1])]
                  : null
              }
              disabledDate={current =>
                !current ||
                current.isAfter(dayjs().startOf('month')) ||
                current.isBefore(dayjs().subtract(3, 'year').startOf('month'))
              }
              onChange={(dates) => {
                if (!dates || !dates[0] || !dates[1]) {
                  setCostTimeRange('month');
                  setCostCustomDateRange(null);
                  updateParams(n => {
                    n.set('period', 'month');
                    n.delete('date_from');
                    n.delete('date_to');
                    n.delete('index_period');
                    n.delete('index_date_from');
                    n.delete('index_date_to');
                  });
                  return;
                }
                const from = dates[0].format('YYYY-MM');
                const to   = dates[1].format('YYYY-MM');
                setCostCustomDateRange([from, to]);
                setCostTimeRange('custom');
                updateParams(n => {
                  n.set('period', 'custom');
                  n.set('date_from', from);
                  n.set('date_to', to);
                  n.delete('index_period');
                  n.delete('index_date_from');
                  n.delete('index_date_to');
                });
              }}
            />
          </div>
        </div>
        {/* Compare */}
        <div>
          <div style={{ fontSize: 11, color: txt2, marginBottom: 6, letterSpacing: '0.04em', textTransform: 'uppercase', fontWeight: 500 }}>
            对比模式
          </div>
          <Segmented
            value={costCompareMode}
            onChange={v => {
              setCostCompareMode(v as CostCompareMode);
              updateParams(n => { n.set('compare', v as string); });
            }}
            options={COMPARE_OPTIONS}
            size="small"
          />
        </div>
      </div>
      {costTimeRange === '1d' && (
        <div style={{ marginTop: 8, fontSize: 11, color: txt2 }}>
          昨日与本月在仅有一日数据时可能相同，累积多日后自动区分。
        </div>
      )}
    </div>
  );

  /* ─── Render: Hero + Bento：主区项目卡五维优先；其下为云环境账户紧凑卡（C/P）[Ref: 03_Phase6/03_前端全域成本透视/01_设计] ─ */
  const hasProjectBento = (globalCostMetrics?.projectBreakdown?.length ?? 0) > 0;
  const renderHeroBento = () => (
    <>
      <div
        style={{
          display: 'grid',
          gridTemplateColumns: hasProjectBento ? '2fr 1fr 1fr' : '1fr',
          gridTemplateRows: hasProjectBento ? 'auto auto' : 'auto',
          gap: 12,
          marginBottom: 14,
        }}
      >
        {/* Hero card */}
        <div
          className="bento-card bento-card-clickable bento-hero-glow"
          onClick={() => {
            updateParams(n => { n.set('env', 'all'); n.set('period', costTimeRange); if (costTimeRange === 'custom' && costCustomDateRange?.[0] && costCustomDateRange?.[1]) { n.set('date_from', costCustomDateRange[0]); n.set('date_to', costCustomDateRange[1]); } });
            setHighlightCloudProduct(true);
            setTimeout(() => setHighlightCloudProduct(false), 2500);
            setTimeout(() => document.getElementById('cloud-product-detail')?.scrollIntoView({ behavior: 'smooth' }), 0);
          }}
          style={{
            ...gc,
            gridRow: hasProjectBento ? '1 / span 2' : undefined,
            position: 'relative',
            background: isDark
              ? 'linear-gradient(135deg, rgba(30,60,120,0.7) 0%, rgba(20,40,90,0.8) 60%, rgba(10,25,60,0.9) 100%)'
              : 'linear-gradient(135deg, rgba(59,130,246,0.18) 0%, rgba(99,102,241,0.10) 50%, rgba(139,92,246,0.08) 100%)',
            border: `1px solid ${isDark ? 'rgba(99,130,255,0.2)' : 'rgba(99,102,241,0.2)'}`,
            padding: '28px 28px 16px',
            display: 'flex',
            flexDirection: 'column',
            justifyContent: 'space-between',
            minHeight: 190,
          }}
        >
          {/* Top label */}
          <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start' }}>
            <div>
              <div style={{ fontSize: 11, color: isDark ? 'rgba(255,255,255,0.45)' : 'rgba(0,0,0,0.42)', fontWeight: 500, letterSpacing: '0.06em', textTransform: 'uppercase', marginBottom: 8, display: 'flex', alignItems: 'center', gap: 6 }}>
                {heroTitleLabel}
                <Tooltip
                  title="主数字为所选成本项目（URL 未选 project_ids 视为全选）各行实付 P（ledger_p）之和；副区为同一批行应付消耗之和，与项目卡同源。卡片金额与是否选中无关；选中仅影响请求参数与下方云环境账户列表。G/U/B 仍以后端 ledger 为准。"
                >
                  <QuestionCircleOutlined style={{ fontSize: 12, opacity: 0.55, cursor: 'help' }} />
                </Tooltip>
              </div>
              {loadingGlobalMetrics ? (
                <div style={{ display: 'flex', alignItems: 'center', gap: 10, height: 60 }}>
                  <LoadingOutlined spin style={{ fontSize: 20, color: '#3b82f6' }} />
                  <span style={{ color: txt2, fontSize: 14 }}>汇总中…</span>
                </div>
              ) : (
                <>
                  <div className="kpi-value-appear" style={{ fontSize: 42, fontWeight: 800, letterSpacing: '-0.02em', color: globalCostMetrics ? (isDark ? '#fff' : '#1e3a5f') : txt2, lineHeight: 1.1 }}>
                    {globalCostMetrics ? (
                      <><span style={{ fontSize: 22, fontWeight: 600, marginRight: 3, opacity: 0.7 }}>{CURRENCY_SYMBOL}</span>
                      {heroPaidSum.toLocaleString(undefined, { minimumFractionDigits: 2, maximumFractionDigits: 2 })}</>
                    ) : (
                      <span style={{ fontSize: 24 }}>—</span>
                    )}
                  </div>
                  {globalCostMetrics && (
                    <div
                      style={{
                        marginTop: 12,
                        paddingTop: 12,
                        borderTop: `1px solid ${isDark ? 'rgba(255,255,255,0.12)' : 'rgba(0,0,0,0.08)'}`,
                      }}
                    >
                      <div style={{ fontSize: 11, color: isDark ? 'rgba(255,255,255,0.5)' : 'rgba(0,0,0,0.45)', fontWeight: 600, letterSpacing: '0.06em', marginBottom: 4 }}>
                        应付消耗
                      </div>
                      <div style={{ fontSize: 24, fontWeight: 700, letterSpacing: '-0.02em', color: isDark ? 'rgba(255,255,255,0.92)' : '#1e3a5f' }}>
                        <><span style={{ fontSize: 16, fontWeight: 600, marginRight: 3, opacity: 0.75 }}>{CURRENCY_SYMBOL}</span>
                        {heroPayableSum.toLocaleString(undefined, { minimumFractionDigits: 2, maximumFractionDigits: 2 })}</>
                      </div>
                    </div>
                  )}
                  {heroChangePct !== null && (
                    <div style={{ marginTop: 8 }}>
                      <ChangeBadge pct={heroChangePct} inverted />
                      <span style={{ fontSize: 11, color: txt2, marginLeft: 4 }}>应付较上期</span>
                    </div>
                  )}
                  {globalCostMetrics?.displayNote && (
                    <div style={{ marginTop: 6, fontSize: 12, color: isDark ? 'rgba(255,255,255,0.6)' : 'rgba(0,0,0,0.55)' }}>
                      {globalCostMetrics.displayNote}
                    </div>
                  )}
                </>
              )}
            </div>
          </div>
          {/* Mini sparkline */}
          {(costTrendData?.length ?? 0) > 1 && (
            <div style={{ marginTop: 'auto', paddingTop: 12, height: 56, opacity: 0.8 }}>
              <ResponsiveContainer width="100%" height="100%">
                <AreaChart data={costTrendData ?? []} margin={{ top: 2, right: 0, left: 0, bottom: 0 }}>
                  <defs>
                    <linearGradient id="heroSparkGrad" x1="0" y1="0" x2="0" y2="1">
                      <stop offset="0%" stopColor={isDark ? '#60a5fa' : '#3b82f6'} stopOpacity={0.35} />
                      <stop offset="100%" stopColor={isDark ? '#60a5fa' : '#3b82f6'} stopOpacity={0} />
                    </linearGradient>
                  </defs>
                  <XAxis dataKey="date" hide />
                  <YAxis hide domain={['auto', 'auto']} />
                  <Area type="monotone" dataKey="total_cost" stroke={isDark ? '#93c5fd' : '#3b82f6'} strokeWidth={2} fill="url(#heroSparkGrad)" dot={false} isAnimationActive={false} />
                </AreaChart>
              </ResponsiveContainer>
            </div>
          )}
        </div>

        {/* 成本项目卡（五维）：单击多选 project_ids；双击跳转云产品索引区 [Ref: 03_Phase6/03_前端全域成本透视/01_设计] */}
        {hasProjectBento &&
          globalCostMetrics!.projectBreakdown!.map(p => {
            const isSel = projectIdsFromUrl.includes(p.project_id);
            return (
              <div
                key={p.project_id}
                className="bento-card bento-card-clickable"
                onClick={() => {
                  if (projectCardClickTimerRef.current) clearTimeout(projectCardClickTimerRef.current);
                  const pid = p.project_id;
                  projectCardClickTimerRef.current = window.setTimeout(() => {
                    projectCardClickTimerRef.current = null;
                    const q = new URLSearchParams(window.location.search);
                    const raw = q.get('project_ids');
                    const cur = raw?.trim()
                      ? raw.split(',').map(s => parseInt(s.trim(), 10)).filter(n => !Number.isNaN(n))
                      : [];
                    const sel = cur.includes(pid);
                    const next = sel ? cur.filter(id => id !== pid) : [...cur, pid];
                    updateParams(n => {
                      if (next.length === 0) n.delete('project_ids');
                      else n.set('project_ids', next.join(','));
                    });
                  }, 280);
                }}
                onDoubleClick={e => {
                  e.preventDefault();
                  if (projectCardClickTimerRef.current) {
                    clearTimeout(projectCardClickTimerRef.current);
                    projectCardClickTimerRef.current = null;
                  }
                  document.getElementById('cloud-product-detail')?.scrollIntoView({ behavior: 'smooth' });
                }}
                style={{
                  ...gc,
                  padding: '18px 20px',
                  minHeight: 210,
                  borderLeft: `3px solid ${isSel ? '#6366f1' : (isDark ? 'rgba(255,255,255,0.15)' : 'rgba(0,0,0,0.12)')}`,
                  background: isSel ? (isDark ? 'rgba(99,102,241,0.2)' : 'rgba(99,102,241,0.08)') : (isDark ? `rgba(255,255,255,0.04)` : `rgba(255,255,255,0.78)`),
                  outline: isSel ? '1.5px solid #6366f1' : 'none',
                  cursor: 'pointer',
                  transition: 'background 0.2s, outline 0.2s',
                }}
              >
                <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start', marginBottom: 8, gap: 8 }}>
                  <div style={{ fontSize: 17, fontWeight: 800, color: txt1, letterSpacing: '-0.02em', lineHeight: 1.25, flex: 1, minWidth: 0 }}>
                    {p.name}
                  </div>
                  <div style={{ display: 'flex', alignItems: 'center', gap: 4, flexShrink: 0 }}>
                    {isSel && (
                      <span style={{ fontSize: 9, fontWeight: 600, color: '#6366f1', background: isDark ? 'rgba(255,255,255,0.1)' : 'rgba(0,0,0,0.06)', padding: '1px 5px', borderRadius: 3, letterSpacing: '0.04em' }}>
                        {projectIdsFromUrl.length > 1 ? '已选' : '已筛选'}
                      </span>
                    )}
                    {loadingGlobalMetrics && <LoadingOutlined style={{ fontSize: 12, color: txt2 }} />}
                  </div>
                </div>
                <div style={{ marginBottom: 4 }}>
                  <div style={{ fontSize: 10, fontWeight: 700, color: txt2, letterSpacing: '0.05em', marginBottom: 4 }}>实付 (P)</div>
                  <div style={{ fontSize: 22, fontWeight: 800, color: txt1, lineHeight: 1.15, letterSpacing: '-0.02em' }}>
                    {fmtDimOrDash(cardActualPaidP(p), fmtDim)}
                  </div>
                  <div style={{ fontSize: 10, fontWeight: 700, color: txt2, letterSpacing: '0.05em', marginTop: 10, marginBottom: 4 }}>应付消耗</div>
                  <div style={{ fontSize: 17, fontWeight: 700, color: txt1, lineHeight: 1.15, letterSpacing: '-0.02em', opacity: 0.92 }}>
                    {fmtDim(projectCardDimC(p))}
                  </div>
                </div>
                <div
                  style={{
                    marginTop: 10,
                    display: 'grid',
                    gridTemplateColumns: 'repeat(3, minmax(0, 1fr))',
                    gap: 6,
                    paddingTop: 10,
                    borderTop: `1px solid ${isDark ? 'rgba(255,255,255,0.08)' : 'rgba(0,0,0,0.06)'}`,
                  }}
                >
                  {([
                    ['G', projectCardDimG(p), '回血（负额叠加，同临时程序 H）'],
                    ['U', p.ledger_u ?? 0, '在途'],
                    ['B', projectCardDimB(p), isCurrentMonthCostView(costTimeRange) ? 'BSS 余额快照' : '余额≈快照−(应付+G)'],
                  ] as const).map(([k, val, hint]) => (
                    <div key={k} style={{ textAlign: 'center', minWidth: 0 }}>
                      <div style={{ fontSize: 10, fontWeight: 700, color: txt2, letterSpacing: '0.06em', marginBottom: 3 }}>{k}</div>
                      <Tooltip title={`${k} · ${hint}`}>
                        <div style={{ fontSize: 12, fontWeight: 700, color: txt1, lineHeight: 1.2, wordBreak: 'break-all' }}>{fmtDim(val)}</div>
                      </Tooltip>
                    </div>
                  ))}
                </div>
              </div>
            );
          })}
      </div>
      <div style={{ marginBottom: 12 }}>
        <div style={{ fontSize: 11, color: txt2, marginBottom: 8, letterSpacing: '0.04em', textTransform: 'uppercase', fontWeight: 500 }}>
          云环境账户
        </div>
        <div style={{ display: 'flex', flexWrap: 'wrap', gap: 8 }}>
          {envCardEnvironments.map(env => {
            const item = globalCostMetrics?.envBreakdown?.find(e => e.environment === env);
            const isConfigured = item && (item.account_id || item.total_cost > 0 || item.account_display_name !== '未配置');
            const isSelected = selectedEnvs.includes(env);
            const color = getEnvColor(env);
            return (
              <div
                key={`env-strip-${env}`}
                className="bento-card bento-card-clickable"
                onClick={() => {
                  const next = isSelected ? selectedEnvs.filter(e => e !== env) : [...selectedEnvs, env];
                  updateParams(n => {
                    if (next.length === 0) {
                      n.delete('envs');
                      n.set('env', 'all');
                    } else {
                      n.set('envs', next.join(','));
                      n.delete('env');
                    }
                    n.set('period', costTimeRange);
                    if (costTimeRange === 'custom' && costCustomDateRange?.[0] && costCustomDateRange?.[1]) {
                      n.set('date_from', costCustomDateRange[0]);
                      n.set('date_to', costCustomDateRange[1]);
                    }
                  });
                  if (!isSelected) {
                    setTimeout(() => document.getElementById('cloud-product-detail')?.scrollIntoView({ behavior: 'smooth' }), 100);
                  }
                }}
                style={{
                  ...gc,
                  minWidth: 168,
                  maxWidth: 300,
                  padding: '12px 14px 14px',
                  borderLeft: `3px solid ${color}`,
                  background: isSelected
                    ? (getEnvSelectedBg(env, isDark) || (isDark ? 'rgba(255,255,255,0.08)' : 'rgba(0,0,0,0.04)'))
                    : (isDark ? `rgba(255,255,255,0.04)` : `rgba(255,255,255,0.78)`),
                  outline: isSelected ? `1.5px solid ${color}` : 'none',
                  cursor: 'pointer',
                  transition: 'background 0.2s, outline 0.2s',
                }}
              >
                <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start', marginBottom: 6, gap: 8 }}>
                  <span style={{
                    fontSize: 15,
                    fontWeight: 800,
                    letterSpacing: '0.02em',
                    color,
                    lineHeight: 1.25,
                    wordBreak: 'break-word',
                  }}
                  >
                    {(item?.cloud_account_label?.trim()) || formatCloudEnvCardTitle(env)}
                  </span>
                  {isSelected && (
                    <span style={{ fontSize: 9, fontWeight: 600, color, opacity: 0.9, flexShrink: 0 }}>✓</span>
                  )}
                </div>
                <div style={{ fontSize: 11, color: isConfigured ? txt2 : (isDark ? 'rgba(255,255,255,0.3)' : 'rgba(0,0,0,0.3)'), marginBottom: 10, lineHeight: 1.35 }}>
                  <span style={{ overflow: 'hidden', textOverflow: 'ellipsis', display: '-webkit-box', WebkitLineClamp: 2, WebkitBoxOrient: 'vertical' as const }}>
                    {item?.account_display_name ?? '未配置'}
                    {item?.cloud_account_site_note ? ` · ${item.cloud_account_site_note}` : ''}
                  </span>
                </div>
                <div>
                  <div style={{ color: txt2, fontWeight: 700, marginBottom: 4, fontSize: 11, letterSpacing: '0.04em' }}>实付 (P)</div>
                  <div style={{
                    fontSize: 22,
                    fontWeight: 800,
                    letterSpacing: '-0.02em',
                    color: isConfigured ? txt1 : (isDark ? 'rgba(255,255,255,0.25)' : 'rgba(0,0,0,0.2)'),
                    lineHeight: 1.15,
                  }}
                  >
                    {item ? fmtDimOrDash(cardActualPaidP(item), fmtDim) : '—'}
                  </div>
                  <div style={{ color: txt2, fontWeight: 700, marginTop: 8, marginBottom: 4, fontSize: 11, letterSpacing: '0.04em' }}>
                    应付消耗
                  </div>
                  <div style={{
                    fontSize: 17,
                    fontWeight: 700,
                    letterSpacing: '-0.02em',
                    color: isConfigured ? txt1 : (isDark ? 'rgba(255,255,255,0.25)' : 'rgba(0,0,0,0.2)'),
                    lineHeight: 1.15,
                    opacity: 0.92,
                  }}
                  >
                    {item ? fmtDim(envCardDimC(item)) : '—'}
                  </div>
                </div>
                <div
                  style={{
                    marginTop: 10,
                    display: 'grid',
                    gridTemplateColumns: 'repeat(3, minmax(0, 1fr))',
                    gap: 6,
                    paddingTop: 10,
                    borderTop: `1px solid ${isDark ? 'rgba(255,255,255,0.08)' : 'rgba(0,0,0,0.06)'}`,
                  }}
                >
                  {item &&
                    ([
                      ['G', envCardDimG(item), '回血（负额叠加，同临时程序 H）'],
                      ['U', item.ledger_u ?? 0, '在途'],
                      ['B', envCardDimB(item), isCurrentMonthCostView(costTimeRange) ? 'BSS 余额快照' : '余额≈快照−(应付+G)'],
                    ] as const).map(([k, val, hint]) => (
                      <div key={k} style={{ textAlign: 'center', minWidth: 0 }}>
                        <div style={{ fontSize: 9, fontWeight: 700, color: txt2, letterSpacing: '0.06em', marginBottom: 2 }}>{k}</div>
                        <Tooltip title={`${k} · ${hint}`}>
                          <div style={{ fontSize: 11, fontWeight: 700, color: txt1, lineHeight: 1.2, wordBreak: 'break-all' }}>{fmtDim(val)}</div>
                        </Tooltip>
                      </div>
                    ))}
                </div>
                {loadingGlobalMetrics && (
                  <div style={{ marginTop: 4 }}>
                    <LoadingOutlined style={{ fontSize: 11, color: txt2 }} />
                  </div>
                )}
              </div>
            );
          })}
        </div>
      </div>
    </>
  );

  /* ─── 五维快照 [Ref: 03_Phase6/01_FinOps] 不展示恒等式文案，避免强定义 ───────────────────────── */
  const renderFiveDimSnapshot = () => {
    const ledger = globalCostMetrics?.ledger;
    const rec = globalCostMetrics?.reconciliation;
    const cells: { key: 'C' | 'G' | 'P' | 'U' | 'B'; label: string; color: string }[] = [
      { key: 'C', label: 'C 应付消耗', color: '#3b82f6' },
      { key: 'G', label: 'G 回血', color: '#8b5cf6' },
      { key: 'P', label: 'P 实付', color: '#10b981' },
      { key: 'U', label: 'U 当月应付 在途', color: '#f59e0b' },
      {
        key: 'B',
        label: isCurrentMonthCostView(costTimeRange) ? 'B 当前账户余额' : 'B 余额',
        color: '#06b6d4',
      },
    ];
    const fmt = (key: 'C' | 'G' | 'P' | 'U' | 'B', v: number | undefined) => {
      let n: number;
      if (key === 'C') {
        n = heroPayableSum;
      } else if (key === 'P') {
        n = heroPaidSum;
      } else if (key === 'B') {
        n = heroDisplayedBalanceB;
      } else if (v == null || Number.isNaN(Number(v))) {
        n = 0;
      } else {
        n = Number(v);
      }
      return `${CURRENCY_SYMBOL}${n.toLocaleString(undefined, { minimumFractionDigits: 2, maximumFractionDigits: 2 })}`;
    };
    return (
      <div style={{ marginBottom: 20 }}>
        <div style={{ fontSize: 13, color: txt1, marginBottom: 12, letterSpacing: '0.06em', fontWeight: 700 }}>
          五维快照
        </div>
        <div style={{ display: 'grid', gridTemplateColumns: 'repeat(5, minmax(0, 1fr))', gap: 12 }}>
          {cells.map(({ key, label, color }) => (
            <div
              key={key}
              className="bento-card"
              style={{
                ...gc,
                padding: '18px 12px',
                minHeight: 108,
                borderTop: `3px solid ${color}`,
                boxShadow: isDark ? `0 4px 20px rgba(0,0,0,0.35)` : `0 4px 18px rgba(0,0,0,0.06)`,
              }}
            >
              <div style={{ fontSize: 11, color: txt2, marginBottom: 10, fontWeight: 600, display: 'flex', alignItems: 'center', gap: 4 }}>
                {label}
                <Tooltip title={FIVE_DIM_CELL_TIPS[key]}>
                  <QuestionCircleOutlined style={{ fontSize: 11, opacity: 0.65, cursor: 'help' }} />
                </Tooltip>
              </div>
              <div style={{ fontSize: 19, fontWeight: 800, color: txt1, letterSpacing: '-0.02em', wordBreak: 'break-all', lineHeight: 1.25 }}>
                {fmt(key, ledger?.[key])}
              </div>
            </div>
          ))}
        </div>
        {rec != null && (rec.residual != null || rec.explain) && (
          <div style={{ marginTop: 8, display: 'inline-flex', alignItems: 'center', gap: 6 }}>
            <Tag color="warning">对账差异</Tag>
            {rec.explain ? (
              <Tooltip title={rec.explain}>
                <span style={{ fontSize: 11, color: txt2, cursor: 'help' }}>说明</span>
              </Tooltip>
            ) : null}
          </div>
        )}
      </div>
    );
  };

  /* ─── 全域指标加载/致命错误（已移除可优化/效率占位行）[Ref: 03_Phase6/01_FinOps UX] ─── */
  const renderGlobalFetchStatus = () => {
    if (loadingGlobalMetrics && !globalCostMetrics) {
      return (
        <div style={{ ...gc, padding: 28, marginBottom: 14, display: 'flex', alignItems: 'center', justifyContent: 'center', gap: 12 }}>
          <LoadingOutlined spin style={{ fontSize: 20, color: '#3b82f6' }} />
          <span style={{ color: txt2 }}>加载指标中…</span>
        </div>
      );
    }
    if (errorGlobalMetrics && !globalCostMetrics) {
      return (
        <Alert
          message="数据暂未就绪"
          description={errorGlobalMetrics}
          type="warning"
          showIcon
          action={<Button size="small" onClick={() => { resetErrors(); fetchGlobalCostMetrics(selectedEnvs.length ? selectedEnvs : undefined, API_TRACK, projectIdsFromUrl.length ? projectIdsFromUrl : undefined); }}>重试</Button>}
          style={{ marginBottom: 14 }}
        />
      );
    }
    return null;
  };

  /* ─── Render: Domain Breakdown ───────────────────────────────────────────── */
  // [Ref: 01_实践] 成本分解四类框架始终渲染；无数据时展示 0 占位
  const renderDomainBreakdown = () => {
    const domainOrder = ['计算资源', '存储', '网络', '安全'];
    const ordered = (globalCostMetrics?.domainBreakdown?.length)
      ? domainOrder.reduce<NonNullable<typeof globalCostMetrics.domainBreakdown>>((acc, name) => {
          const found = globalCostMetrics!.domainBreakdown!.find(d => d.domain === name);
          if (found) acc.push({ ...found });
          else acc.push({ domain: name, cost: 0, optimizableSpace: 0, efficiency: 0, topProducts: [] });
          return acc;
        }, [])
      : domainOrder.map(name => ({ domain: name, cost: 0, optimizableSpace: 0, efficiency: 0, topProducts: [] }));
    const totalDomainCost = ordered.reduce((s, d) => s + d.cost, 0);

    return (
      <div style={{ marginBottom: 14 }}>
        <div
          role="button"
          tabIndex={0}
          onClick={() => setDetailModal('bill')}
          onKeyDown={e => e.key === 'Enter' && setDetailModal('bill')}
          style={{ fontSize: 11, color: txt2, fontWeight: 500, letterSpacing: '0.06em', textTransform: 'uppercase', marginBottom: 10, cursor: 'pointer' }}
        >
          成本分解
        </div>
        <div style={{ display: 'grid', gridTemplateColumns: 'repeat(4, 1fr)', gap: 12 }}>
          {ordered.map((domain) => {
            const meta = DOMAIN_META[domain.domain] ?? DOMAIN_META['计算资源'];
            const pct = totalDomainCost > 0 ? (domain.cost / totalDomainCost) * 100 : 0;
            return (
              <div
                key={domain.domain}
                className="bento-card bento-card-clickable"
                onClick={() => {
                  const category = DOMAIN_TO_CATEGORY[domain.domain] || 'compute';
                  updateParams(n => { n.set('env', 'all'); n.set('period', costTimeRange); if (costTimeRange === 'custom' && costCustomDateRange?.[0] && costCustomDateRange?.[1]) { n.set('date_from', costCustomDateRange[0]); n.set('date_to', costCustomDateRange[1]); } n.set('category', category); });
                  setHighlightCloudProduct(true);
                  setTimeout(() => setHighlightCloudProduct(false), 2500);
                  setTimeout(() => document.getElementById('cloud-product-detail')?.scrollIntoView({ behavior: 'smooth' }), 0);
                  setDomainDetail(domain);
                }}
                style={{
                  ...gc,
                  padding: '18px 20px',
                  background: isDark
                    ? `linear-gradient(135deg, ${meta.gradStart} 0%, rgba(255,255,255,0.03) 100%)`
                    : `linear-gradient(135deg, ${meta.gradStart} 0%, rgba(255,255,255,0.8) 100%)`,
                }}
              >
                {/* Icon + Name */}
                <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginBottom: 14 }}>
                  <div style={{
                    width: 32, height: 32, borderRadius: 10, display: 'flex', alignItems: 'center', justifyContent: 'center',
                    background: isDark ? `rgba(255,255,255,0.08)` : `rgba(255,255,255,0.9)`,
                    boxShadow: `0 2px 8px ${meta.gradStart}`,
                    fontSize: 16, color: meta.color,
                  }}>
                    {meta.icon}
                  </div>
                  <div>
                    <div style={{ fontSize: 13, fontWeight: 600, color: txt1 }}>{domain.domain}</div>
                    <div style={{ fontSize: 10, color: txt2 }}>
                      {pct.toFixed(1)}% 占比
                      {(domain.domain === '存储' || domain.domain === '网络') && (
                        <Tooltip title="当前仅展示成本金额，深度分析后续开放">
                          <QuestionCircleOutlined style={{ marginLeft: 3, fontSize: 10 }} />
                        </Tooltip>
                      )}
                    </div>
                  </div>
                </div>
                {/* Cost */}
                <div style={{ fontSize: 22, fontWeight: 800, color: meta.color, marginBottom: 12, letterSpacing: '-0.01em' }}>
                  {CURRENCY_SYMBOL}{domain.cost.toLocaleString(undefined, { minimumFractionDigits: 2, maximumFractionDigits: 2 })}
                </div>
                {/* Proportion bar */}
                <div style={{ height: 4, background: isDark ? 'rgba(255,255,255,0.08)' : 'rgba(0,0,0,0.08)', borderRadius: 2, overflow: 'hidden' }}>
                  <div
                    className="domain-bar-fill"
                    style={{ height: '100%', width: `${pct}%`, background: `linear-gradient(90deg, ${meta.color}, ${meta.color}88)`, borderRadius: 2 }}
                  />
                </div>
              </div>
            );
          })}
        </div>
      </div>
    );
  };

  /* ─── Render: Index/Filter Section ──────────────────────────────────────── */
  const renderIndexFilter = () => (
    <div style={{
      background: isDark ? 'rgba(255,255,255,0.07)' : 'rgba(245,248,255,0.95)',
      border: `1px solid ${isDark ? 'rgba(255,255,255,0.12)' : 'rgba(59,130,246,0.15)'}`,
      borderRadius: 12,
      boxShadow: isDark ? '0 2px 16px rgba(0,0,0,0.3)' : '0 1px 10px rgba(59,130,246,0.08)',
      padding: '18px 24px 16px',
      marginBottom: 14,
    }}>
      <div style={{
        fontSize: 14,
        color: isDark ? 'rgba(255,255,255,0.85)' : 'rgba(0,0,0,0.78)',
        fontWeight: 700,
        letterSpacing: '-0.01em',
        marginBottom: 14,
        display: 'flex',
        alignItems: 'center',
        gap: 8,
      }}>
        云产品成本明细 · 筛选
        <span style={{
          fontSize: 10,
          fontWeight: 500,
          color: isDark ? 'rgba(255,255,255,0.35)' : 'rgba(0,0,0,0.35)',
          background: isDark ? 'rgba(255,255,255,0.06)' : 'rgba(0,0,0,0.04)',
          padding: '2px 8px',
          borderRadius: 4,
          letterSpacing: '0.04em',
          textTransform: 'uppercase',
        }}>索引</span>
      </div>
      <div style={{ display: 'flex', flexWrap: 'wrap', gap: 12, alignItems: 'center' }}>
        <span style={{ fontSize: 12, color: txt2, fontWeight: 500 }}>时间范围</span>
        <Select value={indexPeriod ?? costTimeRange} style={{ minWidth: 180 }} dropdownStyle={{ minWidth: 240 }} listHeight={400}
          options={TIME_RANGE_OPTIONS.map(o => ({ label: o.label, value: o.value }))}
          onChange={v => updateParams(n => { n.set('index_period', v); if (v !== 'custom') { n.delete('index_date_from'); n.delete('index_date_to'); } })}
        />
        {effectiveDrilldownPeriod === 'custom' && (
          <>
            <DatePicker.RangePicker
              value={effectiveDrilldownDateRange?.[0] && effectiveDrilldownDateRange?.[1] ? [dayjs(effectiveDrilldownDateRange[0]), dayjs(effectiveDrilldownDateRange[1])] : null}
              onChange={(dates: [Dayjs | null, Dayjs | null] | null) => {
                if (dates?.[0] && dates?.[1] && dates[1].diff(dates[0], 'day') <= 180) {
                  const from = dates[0].format('YYYY-MM-DD'); const to = dates[1].format('YYYY-MM-DD');
                  updateParams(n => { n.set('index_period', 'custom'); n.set('index_date_from', from); n.set('index_date_to', to); n.set('period', 'custom'); n.set('date_from', from); n.set('date_to', to); });
                }
              }}
            />
            <span style={{ fontSize: 11, color: txt2 }}>最多6个月</span>
          </>
        )}
        <span style={{ fontSize: 12, color: txt2, fontWeight: 500, marginLeft: 4 }}>环境</span>
        <Select
          mode="multiple"
          allowClear
          placeholder="全环境"
          style={{ minWidth: 160, maxWidth: 280 }}
          maxTagCount="responsive"
          value={selectedEnvs.length > 0 ? selectedEnvs : undefined}
          options={envFilterOptions}
          onChange={vals => {
            updateParams(n => {
              if (!vals || vals.length === 0) {
                n.delete('envs');
                n.set('env', 'all');
              } else {
                n.set('envs', vals.join(','));
                n.delete('env');
              }
            });
          }}
        />
        <span style={{ fontSize: 12, color: txt2, fontWeight: 500 }}>大类</span>
        <Select value={drilldownCategory || 'all'} style={{ width: 120 }} options={[{ label: '全部', value: 'all' }, { label: '计算资源', value: 'compute' }, { label: '存储', value: 'storage' }, { label: '网络', value: 'network' }, { label: '安全', value: 'security' }]}
          onChange={v => updateParams(n => { if (v === 'all') n.delete('category'); else n.set('category', v); })}
        />
        <span style={{ fontSize: 12, color: txt2, fontWeight: 500 }}>排序</span>
        <Select value={searchParams.get('sort') || 'cost_desc'} style={{ width: 110 }} options={[{ label: '成本降序', value: 'cost_desc' }, { label: '成本升序', value: 'cost_asc' }]}
          onChange={v => updateParams(n => { n.set('sort', v); })}
        />
        <span style={{ fontSize: 12, color: txt2, fontWeight: 500, marginLeft: 4 }}>环比</span>
        <Switch
          checked={drilldownCompare}
          onChange={on => { const n = new URLSearchParams(searchParams); n.set('compare_trend', on ? '1' : '0'); setSearchParams(n, { replace: true }); }}
          checkedChildren="对比上期" unCheckedChildren="关" size="small"
        />
        <span style={{ fontSize: 12, color: txt2, fontWeight: 500 }}>趋势图</span>
        <Switch
          checked={showTrendChart}
          onChange={on => { const n = new URLSearchParams(searchParams); n.set('show_trend', on ? '1' : '0'); setSearchParams(n, { replace: true }); }}
          checkedChildren="开" unCheckedChildren="关" size="small"
        />
      </div>
    </div>
  );

  /* ─── Render: Trend Chart ────────────────────────────────────────────────── */
  const PRODUCT_COLORS = ['#3b82f6', '#ef4444', '#f59e0b', '#10b981', '#8b5cf6', '#ec4899', '#06b6d4', '#f97316', '#6366f1', '#14b8a6'];

  const renderTrendChart = () => {
    const cur = costTrendData ?? [];
    const prev = costTrendDataPrev ?? [];

    const trendChartData = (() => {
      const dateSet = new Set<string>([...cur.map(d => d.date), ...prev.map(d => d.date)]);
      return Array.from(dateSet).sort().map(date => ({
        date,
        本期: cur.find(d => d.date === date)?.total_cost ?? null,
        上期: drilldownCompare && prev.length ? (prev.find(d => d.date === date)?.total_cost ?? null) : undefined,
      }));
    })();

    // 禁止在 renderTrendChart 内使用 useMemo：非组件函数内调用 Hook 会违反 Rules of Hooks 并导致整页白屏 [Ref: React hooks rules]
    const topProducts = (() => {
      const sums: Record<string, number> = {};
      for (const pt of cur) {
        for (const [code, cost] of Object.entries(pt.by_product ?? {})) {
          sums[code] = (sums[code] ?? 0) + Math.abs(cost);
        }
      }
      return Object.entries(sums).sort((a, b) => b[1] - a[1]).slice(0, 8).map(e => e[0]);
    })();

    const productChartData = cur.map(pt => {
      const row: Record<string, number | string | null> = { date: pt.date };
      for (const code of topProducts) {
        row[code] = pt.by_product?.[code] ?? 0;
      }
      return row;
    });

    return (
      <div className="bento-card" style={{ ...gc, padding: '18px 20px', marginBottom: 12 }}>
        <div style={{ fontSize: 11, color: txt2, fontWeight: 600, letterSpacing: '0.06em', textTransform: 'uppercase', marginBottom: 12, display: 'flex', alignItems: 'center', gap: 8 }}>
          成本趋势
          {drilldownEnv && drilldownEnv !== 'all' && (
            <span style={{ fontSize: 10, fontWeight: 600, color: ENV_COLORS[drilldownEnv as keyof typeof ENV_COLORS] ?? '#3b82f6', background: isDark ? 'rgba(255,255,255,0.08)' : 'rgba(0,0,0,0.05)', padding: '1px 7px', borderRadius: 4, letterSpacing: '0.04em', textTransform: 'none' }}>
              {drilldownEnv} 环境
            </span>
          )}
          <span style={{ marginLeft: 'auto', display: 'flex', gap: 4 }}>
            {(['total', 'product'] as const).map(mode => (
              <span
                key={mode}
                role="button"
                tabIndex={0}
                onClick={() => setTrendMode(mode)}
                onKeyDown={e => e.key === 'Enter' && setTrendMode(mode)}
                style={{
                  fontSize: 10, fontWeight: 600, padding: '2px 8px', borderRadius: 4, cursor: 'pointer',
                  background: trendMode === mode ? (isDark ? 'rgba(59,130,246,0.25)' : 'rgba(59,130,246,0.12)') : 'transparent',
                  color: trendMode === mode ? '#3b82f6' : txt2,
                }}
              >
                {mode === 'total' ? '总成本' : '按产品'}
              </span>
            ))}
          </span>
        </div>
        {!showTrendChart ? (
          <div style={{ padding: '16px 0', color: txt2, fontSize: 12 }}>
            趋势图已关闭 · 打开上方「趋势图」开关即可查看按日成本趋势
          </div>
        ) : loadingCostTrend ? (
          <div style={{ height: 200, display: 'flex', alignItems: 'center', justifyContent: 'center', gap: 10 }}>
            <LoadingOutlined spin style={{ color: '#3b82f6' }} /> <span style={{ color: txt2 }}>加载趋势中…</span>
          </div>
        ) : errorCostTrend ? (
          <Alert message={errorCostTrend} type="warning" showIcon style={{ marginBottom: 0 }} />
        ) : trendChartData.length > 0 ? (
          <>
            {drilldownCompare && trendMode === 'total' && (!costTrendDataPrev || costTrendDataPrev.length === 0) && (
              <div style={{ marginBottom: 8, fontSize: 11, color: '#f59e0b' }}>环比已开，暂无上期数据</div>
            )}
            <div style={{ height: 220 }}>
              <ResponsiveContainer width="100%" height="100%">
                {trendMode === 'product' && topProducts.length > 0 ? (
                  <LineChart data={productChartData} margin={{ top: 5, right: 16, left: 0, bottom: 5 }}>
                    <CartesianGrid strokeDasharray="3 3" stroke={divider} />
                    <XAxis dataKey="date" tick={{ fontSize: 11, fill: txt2 }} tickLine={false} axisLine={{ stroke: divider }} tickFormatter={d => d.slice(5)} />
                    <YAxis tick={{ fontSize: 11, fill: txt2 }} tickLine={false} axisLine={false} tickFormatter={v => v >= 1000 ? `${(v / 1000).toFixed(1)}k` : String(Math.round(v))} />
                    <RTooltip content={<ChartTooltip />} />
                    {topProducts.map((code, i) => (
                      <Line key={code} type="monotone" dataKey={code} stroke={PRODUCT_COLORS[i % PRODUCT_COLORS.length]} strokeWidth={1.5} dot={false} name={code} />
                    ))}
                    <Legend wrapperStyle={{ fontSize: 10, paddingTop: 8 }} />
                  </LineChart>
                ) : (
                  <AreaChart data={trendChartData} margin={{ top: 5, right: 16, left: 0, bottom: 5 }}>
                    <defs>
                      <linearGradient id="trendGradCur" x1="0" y1="0" x2="0" y2="1">
                        <stop offset="0%" stopColor={isDark ? '#60a5fa' : '#3b82f6'} stopOpacity={0.25} />
                        <stop offset="100%" stopColor={isDark ? '#60a5fa' : '#3b82f6'} stopOpacity={0} />
                      </linearGradient>
                      <linearGradient id="trendGradPrev" x1="0" y1="0" x2="0" y2="1">
                        <stop offset="0%" stopColor={isDark ? '#34d399' : '#10b981'} stopOpacity={0.2} />
                        <stop offset="100%" stopColor={isDark ? '#34d399' : '#10b981'} stopOpacity={0} />
                      </linearGradient>
                    </defs>
                    <CartesianGrid strokeDasharray="3 3" stroke={divider} />
                    <XAxis dataKey="date" tick={{ fontSize: 11, fill: txt2 }} tickLine={false} axisLine={{ stroke: divider }} tickFormatter={d => d.slice(5)} />
                    <YAxis tick={{ fontSize: 11, fill: txt2 }} tickLine={false} axisLine={false} tickFormatter={v => v >= 1000 ? `${(v / 1000).toFixed(1)}k` : String(v)} />
                    <RTooltip content={<ChartTooltip />} />
                    <Area type="monotone" dataKey="本期" stroke={isDark ? '#60a5fa' : '#3b82f6'} strokeWidth={2} fill="url(#trendGradCur)" dot={false} name="本期" />
                    {drilldownCompare && costTrendDataPrev?.length ? (
                      <Area type="monotone" dataKey="上期" stroke={isDark ? '#34d399' : '#10b981'} strokeWidth={2} strokeDasharray="5 3" fill="url(#trendGradPrev)" dot={false} name="上期" />
                    ) : null}
                    <Legend wrapperStyle={{ fontSize: 12, paddingTop: 8 }} />
                  </AreaChart>
                )}
              </ResponsiveContainer>
            </div>
          </>
        ) : (
          <div style={{ height: 100, display: 'flex', alignItems: 'center', justifyContent: 'center', color: txt2, fontSize: 12 }}>
            暂无趋势数据（请确认 GET /api/v1/cost/trend 已启用且有日原始表数据）
          </div>
        )}
      </div>
    );
  };

  /* ─── Render: Cloud Product Table ────────────────────────────────────────── */
  const renderCloudProductTable = () => (
    <div
      id="cloud-product-detail"
      className="bento-card"
      style={{
        background: isDark ? 'rgba(255,255,255,0.04)' : 'rgba(248,249,252,0.95)',
        border: `1px solid ${isDark ? 'rgba(255,255,255,0.10)' : 'rgba(0,0,0,0.07)'}`,
        borderRadius: 14,
        boxShadow: highlightCloudProduct
          ? `0 0 0 2px #3b82f6, 0 4px 24px rgba(0,0,0,0.10)`
          : isDark ? '0 4px 24px rgba(0,0,0,0.35)' : '0 4px 24px rgba(0,0,0,0.06)',
        padding: '18px 20px 10px',
        transition: 'box-shadow 0.3s',
        overflow: 'hidden',
      }}
    >
      <div style={{ fontSize: 11, color: txt2, fontWeight: 600, letterSpacing: '0.06em', textTransform: 'uppercase', marginBottom: 14 }}>
        云产品成本明细
      </div>
      {loadingDrilldownGlobal ? (
        <div style={{ textAlign: 'center', padding: '24px', color: txt2 }}>
          <LoadingOutlined spin style={{ marginRight: 8 }} /> 加载中…
        </div>
      ) : errorDrilldownGlobal ? (
        <Alert message="云产品明细暂未就绪" description={errorDrilldownGlobal} type="warning" showIcon />
      ) : drilldownGlobalProducts?.length ? (
        /* [Ref: 用户需求] 重构列布局：产品列固定宽度使数据列左移，成本字号放大，分页受控 */
        <Table<CloudProductDrilldownItem>
          size="small"
          bordered
          rowKey={r => r.product_code + (r.category ?? '')}
          dataSource={drilldownGlobalProducts}
          columns={[
            {
              title: '产品',
              dataIndex: 'product_name',
              key: 'product',
              width: 220,
              ellipsis: true,
              render: (_: unknown, r: CloudProductDrilldownItem) => (
                <span style={{ fontWeight: 600, fontSize: 13 }}>{r.product_name || r.product_code}</span>
              ),
            },
            {
              title: `成本 (${CURRENCY_SYMBOL})`,
              dataIndex: 'cost',
              key: 'cost',
              width: 148,
              align: 'right' as const,
              render: (v: number) => (
                /* 等宽字体右对齐，字号放大至15px凸显核心数据 */
                <span style={{
                  fontFamily: '"SF Mono", "Fira Code", "Consolas", monospace',
                  fontWeight: 700,
                  fontSize: 15,
                  color: isDark ? '#93c5fd' : '#1d4ed8',
                  letterSpacing: '-0.02em',
                }}>
                  {CURRENCY_SYMBOL}{(v ?? 0).toLocaleString(undefined, { minimumFractionDigits: 2, maximumFractionDigits: 2 })}
                </span>
              ),
            },
            {
              title: '分类',
              dataIndex: 'category',
              key: 'category',
              width: 120,
              align: 'center' as const,
              render: (v: string) => {
                const catKey = (typeof v === 'string' ? v.toLowerCase() : '') as keyof typeof CATEGORY_TO_LABEL;
                const label = CATEGORY_TO_LABEL[catKey] ?? v ?? '—';
                const domainMeta = DOMAIN_META[label];
                return domainMeta ? (
                  <span style={{
                    display: 'inline-flex', alignItems: 'center', gap: 4,
                    color: domainMeta.color, fontWeight: 500, fontSize: 12,
                  }}>
                    {domainMeta.icon}
                    <span>{label}</span>
                  </span>
                ) : <span style={{ fontSize: 12 }}>{label}</span>;
              },
            },
            {
              title: '环比',
              key: 'change_pct',
              width: 96,
              align: 'center' as const,
              render: (_: unknown, r: CloudProductDrilldownItem) => {
                if (!drilldownCompare) return <span style={{ color: txt2, fontSize: 12 }}>—</span>;
                const prev = drilldownGlobalProductsPrev?.find(p => p.product_code === r.product_code);
                const prevCost = prev?.cost ?? 0;
                if (Math.abs(prevCost) < 0.01) return (
                  <span style={{ fontSize: 11, color: '#f59e0b', fontWeight: 600,
                    background: 'rgba(245,158,11,0.1)', borderRadius: 20, padding: '2px 7px' }}>
                    新增
                  </span>
                );
                const pct = ((r.cost - prevCost) / Math.abs(prevCost)) * 100;
                return <ChangeBadge pct={pct} inverted />;
              },
            },
            {
              title: '趋势',
              key: 'trend',
              width: 110,
              align: 'center' as const,
              render: (_: unknown, r: CloudProductDrilldownItem) => {
                const code = r.product_code;
                const productSeries = (costTrendData ?? []).map(pt => ({
                  date: pt.date,
                  cost: pt.by_product?.[code] ?? 0,
                }));
                const hasData = productSeries.some(p => p.cost !== 0);
                return (
                  <div
                    role="button"
                    tabIndex={0}
                    onClick={() => { setTrendModalProductCode(code); setTrendModalOpen(true); }}
                    onKeyDown={e => { if (e.key === 'Enter') { setTrendModalProductCode(code); setTrendModalOpen(true); } }}
                    style={{
                      cursor: hasData ? 'pointer' : 'default',
                      display: 'flex',
                      alignItems: 'center',
                      justifyContent: 'center',
                      height: 32,
                    }}
                    title={hasData ? `${code} 趋势 · 点击查看大图` : undefined}
                  >
                    {hasData ? (
                      <ResponsiveContainer width={96} height={28}>
                        <AreaChart data={productSeries} margin={{ top: 2, right: 2, left: 0, bottom: 2 }}>
                          <defs>
                            <linearGradient id={`miniGrad_${code}`} x1="0" y1="0" x2="0" y2="1">
                              <stop offset="0%" stopColor="#3b82f6" stopOpacity={0.35} />
                              <stop offset="100%" stopColor="#3b82f6" stopOpacity={0} />
                            </linearGradient>
                          </defs>
                          <XAxis dataKey="date" hide />
                          <YAxis hide domain={['auto', 'auto']} />
                          <Area
                            type="monotone"
                            dataKey="cost"
                            stroke="#3b82f6"
                            strokeWidth={1.5}
                            fill={`url(#miniGrad_${code})`}
                            dot={false}
                            isAnimationActive={false}
                          />
                        </AreaChart>
                      </ResponsiveContainer>
                    ) : (
                      <span style={{ color: txt2, fontSize: 12 }}>—</span>
                    )}
                  </div>
                );
              },
            },
          ]}
          // [Ref: 用户需求] 分页 Bug 修复：current + pageSize 双受控，pageSize 变更时强制跳回第1页
          pagination={{
            current: tablePage,
            pageSize: tablePageSize,
            pageSizeOptions: [10, 20, 50, 100],
            showSizeChanger: true,
            showTotal: t => `共 ${t} 条`,
            onChange: (page, size) => {
              setTablePage(page);
              if (size !== tablePageSize) {
                setTablePageSize(size);
                setTablePage(1);
              }
            },
          }}
        />
      ) : (
        <div style={{ textAlign: 'center', padding: '24px', color: txt2 }}>暂无云产品明细数据</div>
      )}
    </div>
  );

  /* ─── Main Render ────────────────────────────────────────────────────────── */
  return (
    <div style={{
      margin: -24,
      padding: '20px 24px 32px',
      minHeight: 'calc(100% + 48px)',
      background: isDark
        ? 'linear-gradient(145deg, #060d1c 0%, #0b1729 55%, #081320 100%)'
        : 'linear-gradient(145deg, #d9e7f7 0%, #e3edf9 50%, #edf2fc 100%)',
    }}>

      {/* ── Header ── */}
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 16 }}>
        <div style={{ display: 'flex', alignItems: 'baseline', gap: 10 }}>
          <h2 style={{ margin: 0, fontSize: 22, fontWeight: 800, color: txt1, letterSpacing: '-0.01em' }}>
            全域成本透视
          </h2>
          <span style={{ fontSize: 12, color: txt2 }}>
            {globalCostMetrics && hasMeaningfulDisplayedSpend ? '数据来源：云账单' : '数据来源：—'}
          </span>
        </div>
        <div style={{ display: 'flex', alignItems: 'center', gap: 10, flexWrap: 'wrap', justifyContent: 'flex-end' }}>
          <Tooltip
            title={
              finopsSyncLoading && finopsSyncPoll
                ? `${finopsSyncPoll.phaseDetail} · ${finopsSyncPoll.pct}%（步骤进度，非耗时）`
                : FINOPS_SYNC_MANUAL_TOOLTIP
            }
          >
            <div
              role="button"
              tabIndex={0}
              onClick={() => {
                if (!finopsSyncLoading) void runFinopsSync();
              }}
              onKeyDown={e => {
                if (!finopsSyncLoading && (e.key === 'Enter' || e.key === ' ')) {
                  e.preventDefault();
                  void runFinopsSync();
                }
              }}
              style={{
                position: 'relative',
                display: 'inline-flex',
                alignItems: 'center',
                justifyContent: 'center',
                gap: 6,
                minWidth: 148,
                height: 32,
                padding: '0 12px',
                borderRadius: 8,
                border: `1px solid ${isDark ? 'rgba(59,130,246,0.4)' : 'rgba(59,130,246,0.5)'}`,
                overflow: 'hidden',
                cursor: finopsSyncLoading ? 'wait' : 'pointer',
                opacity: finopsSyncLoading ? 0.92 : 1,
                background: isDark ? 'rgba(59,130,246,0.08)' : 'rgba(59,130,246,0.06)',
                userSelect: 'none',
              }}
            >
              <div
                aria-hidden
                style={{
                  position: 'absolute',
                  left: 0,
                  top: 0,
                  bottom: 0,
                  width: `${finopsSyncLoading && finopsSyncPoll ? finopsSyncPoll.pct : 0}%`,
                  background: 'linear-gradient(90deg, rgba(59,130,246,0.42), rgba(99,102,241,0.32))',
                  transition: 'width 0.35s ease',
                }}
              />
              <SyncOutlined style={{ position: 'relative', zIndex: 1, fontSize: 14, color: isDark ? '#93c5fd' : '#2563eb' }} />
              <span style={{
                position: 'relative',
                zIndex: 1,
                fontSize: 13,
                fontWeight: 600,
                color: isDark ? 'rgba(255,255,255,0.92)' : '#1e3a5f',
                display: 'inline-flex',
                alignItems: 'center',
                gap: 4,
              }}
              >
                {finopsSyncLoading ? (
                  `同步中 ${finopsSyncPoll ? finopsSyncPoll.pct : 0}%`
                ) : (
                  <>
                    同步数据
                    <InfoCircleOutlined
                      role="img"
                      aria-label="同步数据说明"
                      style={{ fontSize: 12, opacity: 0.75 }}
                    />
                  </>
                )}
              </span>
            </div>
          </Tooltip>
          {globalCostMetrics?.billDataStatus && (
            <BillDataStatusBadge status={globalCostMetrics.billDataStatus} isDark={isDark} />
          )}
          <div style={{ display: 'flex', alignItems: 'center', gap: 6, fontSize: 11, color: txt2 }}>
            <span className="live-dot" />
            {globalCostMetrics?.lastUpdatedAt
              ? `数据更新至 ${new Date(globalCostMetrics.lastUpdatedAt).toLocaleString('zh-CN', { timeZone: 'Asia/Shanghai', dateStyle: 'short', timeStyle: 'short' })}`
              : errorGlobalMetrics ? '暂未就绪' : '—'}
          </div>
        </div>
      </div>

      {/* ── Alert: no data ── */}
      {globalCostMetrics && !hasMeaningfulDisplayedSpend && !errorGlobalMetrics && (
        <Alert
          message={hasPreviousPeriodData(globalCostMetrics) ? '本期暂无消费数据' : '暂无真实数据'}
          description={
            hasPreviousPeriodData(globalCostMetrics)
              ? '本期（如本月）成本为 0，可能为 T+1 延迟或当月暂无消费；上期有数据说明 ETL 正常。'
              : '请完成：1) 配置阿里云 AK/SK 并执行 ETL，将云账单写入日/月原始表与聚合表（cost_cloud_bill_daily_raw、cost_cloud_bill_aggregate 等）；2) 接入 Prometheus/K8s 获取集群内计算成本。部署后首次可触发全量回填或等待定时 ETL。'
          }
          type="warning"
          showIcon
          style={{ marginBottom: 14, borderRadius: 12 }}
        />
      )}

      {/* ── Filter Bar ── */}
      {renderFilterBar()}

      {/* ── Hero + Env Bento ── */}
      {renderHeroBento()}

      {/* ── 成本结构主体（已移除“成本结构趋势”模块） ── */}
      <div className="bento-card" style={{ ...gc, padding: '12px 20px 20px' }}>
        {/* Error banner when data partial */}
        {errorGlobalMetrics && globalCostMetrics && (
          <Alert message="数据暂未更新" description={errorGlobalMetrics} type="warning" showIcon style={{ marginBottom: 14, borderRadius: 10 }} />
        )}

        {renderGlobalFetchStatus()}
        {/* 五维快照 [Ref: 03_Phase6/01_FinOps] */}
        {renderFiveDimSnapshot()}

        {/* Domain 4 cards */}
        {renderDomainBreakdown()}

        {/* Index filter */}
        {renderIndexFilter()}

        {/* Trend chart */}
        {renderTrendChart()}

        {/* Cloud product table */}
        {renderCloudProductTable()}
      </div>

      {/* ── Trend Modal ── [Ref: 单产品趋势大图：从明细行打开时展示该产品趋势] */}
      <Modal
        title={trendModalProductCode ? `${trendModalProductCode} 成本趋势` : '成本趋势大图'}
        open={trendModalOpen}
        onCancel={() => { setTrendModalOpen(false); setTrendModalProductCode(null); }}
        footer={null}
        width={680}
        destroyOnClose
      >
        {showTrendChart && (costTrendData?.length ?? 0) > 0 ? (
          <div style={{ height: 360 }}>
            <ResponsiveContainer width="100%" height="100%">
              <AreaChart
                data={(() => {
                  const cur = costTrendData ?? []; const prev = costTrendDataPrev ?? [];
                  const code = trendModalProductCode;
                  const dateSet = new Set<string>([...cur.map(d => d.date), ...prev.map(d => d.date)]);
                  return Array.from(dateSet).sort().map(date => {
                    if (code) {
                      const curVal = cur.find(d => d.date === date)?.by_product?.[code] ?? null;
                      const prevVal = drilldownCompare && prev.length ? (prev.find(d => d.date === date)?.by_product?.[code] ?? null) : undefined;
                      return { date, 本期: curVal, 上期: prevVal };
                    }
                    return {
                      date,
                      本期: cur.find(d => d.date === date)?.total_cost ?? null,
                      上期: drilldownCompare && prev.length ? (prev.find(d => d.date === date)?.total_cost ?? null) : undefined,
                    };
                  });
                })()}
                margin={{ top: 10, right: 24, left: 0, bottom: 10 }}
              >
                <defs>
                  <linearGradient id="modalGradCur" x1="0" y1="0" x2="0" y2="1">
                    <stop offset="0%" stopColor="#3b82f6" stopOpacity={0.25} /><stop offset="100%" stopColor="#3b82f6" stopOpacity={0} />
                  </linearGradient>
                </defs>
                <CartesianGrid strokeDasharray="3 3" />
                <XAxis dataKey="date" tick={{ fontSize: 12 }} tickFormatter={d => d.slice(5)} />
                <YAxis tick={{ fontSize: 12 }} tickFormatter={v => v >= 1000 ? `${(v / 1000).toFixed(1)}k` : String(v)} />
                <RTooltip content={<ChartTooltip />} />
                <Area type="monotone" dataKey="本期" stroke="#3b82f6" strokeWidth={2} fill="url(#modalGradCur)" dot={false} name="本期" />
                {drilldownCompare && costTrendDataPrev?.length ? (
                  <Area type="monotone" dataKey="上期" stroke="#10b981" strokeWidth={2} strokeDasharray="5 3" fill="none" dot={false} name="上期" />
                ) : null}
                <Legend />
              </AreaChart>
            </ResponsiveContainer>
          </div>
        ) : (
          <div style={{ padding: 24, textAlign: 'center', color: txt2 }}>
            请先打开「趋势图」开关并确保有趋势数据后再查看大图。
          </div>
        )}
      </Modal>

      {/* ── Domain Detail Modal ── */}
      <Modal
        title={domainDetail ? `${domainDetail.domain} — 成本前 4 产品` : undefined}
        open={domainDetail !== null}
        onCancel={() => setDomainDetail(null)}
        footer={
          domainDetail ? (
            <Button type="link" onClick={() => {
              const category = domainDetail ? DOMAIN_TO_CATEGORY[domainDetail.domain] || 'compute' : undefined;
              updateParams(n => { n.set('env', 'all'); n.set('period', costTimeRange); if (costTimeRange === 'custom' && costCustomDateRange?.[0] && costCustomDateRange?.[1]) { n.set('date_from', costCustomDateRange[0]); n.set('date_to', costCustomDateRange[1]); } if (category) n.set('category', category); });
              setDomainDetail(null);
              setTimeout(() => document.getElementById('cloud-product-detail')?.scrollIntoView({ behavior: 'smooth' }), 0);
            }}>查看更多 → 云产品成本明细</Button>
          ) : null
        }
        width={520}
      >
        {domainDetail && (
          <>
            <Descriptions column={1} bordered size="small" style={{ marginBottom: 12 }}>
              <Descriptions.Item label="大类">{domainDetail.domain}</Descriptions.Item>
              <Descriptions.Item label={`总成本 (${CURRENCY_SYMBOL})`}>{CURRENCY_SYMBOL}{domainDetail.cost.toLocaleString()}</Descriptions.Item>
            </Descriptions>
            <div style={{ marginBottom: 8, fontSize: 12, color: '#666' }}>该大类下成本最高的 4 个云产品：</div>
            {domainDetail.topProducts?.length ? (
              <ul style={{ margin: 0, paddingLeft: 20 }}>
                {domainDetail.topProducts.slice(0, 4).map((p, j) => (
                  <li key={j} style={{ marginBottom: 4 }}>{p.product}：{CURRENCY_SYMBOL}{p.cost.toLocaleString()}</li>
                ))}
              </ul>
            ) : <div style={{ color: '#999', fontSize: 12 }}>暂无产品明细</div>}
          </>
        )}
      </Modal>

      {/* ── Bill/Efficiency Detail Modal ── */}
      <Modal
        title={detailModal === 'bill' ? '成本分解（按领域）' : '效率构成'}
        open={detailModal !== null}
        onCancel={() => setDetailModal(null)}
        footer={null}
        width={640}
      >
        {detailModal === 'bill' && globalCostMetrics && (
          <div>
            <p style={{ color: '#666', marginBottom: 16 }}>
              全环境总成本由计算资源、存储、网络、安全四类领域汇总，分项之和=全环境总成本。
            </p>
            {globalCostMetrics.billDetail && (
              <Descriptions column={1} bordered size="small" style={{ marginBottom: 16 }}>
                <Descriptions.Item label="计算资源">{CURRENCY_SYMBOL}{globalCostMetrics.billDetail.compute.toLocaleString()}</Descriptions.Item>
                <Descriptions.Item label="存储">{CURRENCY_SYMBOL}{globalCostMetrics.billDetail.storage.toLocaleString()}</Descriptions.Item>
                <Descriptions.Item label="网络">{CURRENCY_SYMBOL}{globalCostMetrics.billDetail.network.toLocaleString()}</Descriptions.Item>
                <Descriptions.Item label="安全">{CURRENCY_SYMBOL}{(globalCostMetrics.billDetail.security ?? 0).toLocaleString()}</Descriptions.Item>
              </Descriptions>
            )}
            <Descriptions column={1} bordered size="small">
              {(globalCostMetrics.domainBreakdown ?? []).map((d, i) => {
                const categoryKey = DOMAIN_TO_CATEGORY[d.domain];
                return (
                  <Descriptions.Item key={i} label={d.domain}>
                    <div>
                      <div>{CURRENCY_SYMBOL}{d.cost.toLocaleString()}（可优化 {d.optimizableSpace > 0 ? `${CURRENCY_SYMBOL}${d.optimizableSpace.toLocaleString()}` : '—'}，效率 {d.efficiency > 0 ? `${d.efficiency}%` : '—'}）</div>
                      {d.topProducts?.length > 0 && (
                        <div style={{ marginTop: 8 }}>
                          <div style={{ fontSize: 12, color: '#666', marginBottom: 4 }}>成本前 4 产品：</div>
                          <ul style={{ margin: 0, paddingLeft: 18 }}>
                            {d.topProducts.map((p, j) => <li key={j}>{p.product}：{CURRENCY_SYMBOL}{p.cost.toLocaleString()}</li>)}
                          </ul>
                          {categoryKey && (
                            <a
                              onClick={e => {
                                e.preventDefault();
                                setDetailModal(null);
                                updateParams(n => { n.set('category', categoryKey); });
                                setTimeout(() => document.getElementById('cloud-product-detail')?.scrollIntoView({ behavior: 'smooth' }), 100);
                              }}
                              style={{ marginTop: 6, display: 'inline-block', cursor: 'pointer' }}
                            >
                              查看详情 →
                            </a>
                          )}
                        </div>
                      )}
                    </div>
                  </Descriptions.Item>
                );
              })}
            </Descriptions>
          </div>
        )}
        {detailModal === 'efficiency' && globalCostMetrics && (
          <div>
            <p style={{ color: '#666', marginBottom: 16 }}>全局效率 = 汇总使用成本/全环境总成本×100%。</p>
            <Table size="small"
              dataSource={[
                ...(globalCostMetrics.domainBreakdown ?? []).map(d => ({ key: `domain-${d.domain}`, name: d.domain, efficiency: d.efficiency, type: '领域' })),
              ]}
              columns={[
                { title: '类型', dataIndex: 'type', width: 80 },
                { title: '名称', dataIndex: 'name' },
                { title: '效率 (%)', dataIndex: 'efficiency', render: (v: number) => (v != null && v > 0 ? `${v}%` : '—') },
              ]}
              pagination={false}
            />
          </div>
        )}
      </Modal>
    </div>
  );
};

export default CostOverviewPage;
