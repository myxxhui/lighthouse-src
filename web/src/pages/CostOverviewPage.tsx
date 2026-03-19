import React, { useEffect, useState } from 'react';
import { useNavigate, useSearchParams } from 'umi';
import {
  Button, Statistic, Switch, Alert, Tooltip,
  Select, Segmented, Radio, Modal, Descriptions, Table, DatePicker, Tag,
} from 'antd';
import dayjs, { type Dayjs } from 'dayjs';
import {
  LoadingOutlined, QuestionCircleOutlined,
  CloudServerOutlined, DatabaseOutlined, GlobalOutlined, SafetyCertificateOutlined,
  ArrowUpOutlined, ArrowDownOutlined,
} from '@ant-design/icons';
import EfficiencyChart from '@/components/EfficiencyChart';
import { useAppStore } from '@/store';
import {
  AreaChart, Area, LineChart, Line,
  XAxis, YAxis, CartesianGrid, Tooltip as RTooltip,
  ResponsiveContainer, Legend,
} from 'recharts';
import type { CostTimeRange, CostCompareMode, CloudProductDrilldownItem } from '@/types';
import type { DomainBreakdown } from '@/types';
import { CURRENCY_SYMBOL } from '@/constants';

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
// Domain visual metadata
const DOMAIN_META: Record<string, { icon: React.ReactNode; color: string; gradStart: string; gradEnd: string }> = {
  '计算资源': { icon: <CloudServerOutlined />,      color: '#3b82f6', gradStart: 'rgba(59,130,246,0.18)',  gradEnd: 'rgba(59,130,246,0.04)'  },
  '存储':     { icon: <DatabaseOutlined />,          color: '#8b5cf6', gradStart: 'rgba(139,92,246,0.18)',  gradEnd: 'rgba(139,92,246,0.04)'  },
  '网络':     { icon: <GlobalOutlined />,            color: '#06b6d4', gradStart: 'rgba(6,182,212,0.18)',   gradEnd: 'rgba(6,182,212,0.04)'   },
  '安全':     { icon: <SafetyCertificateOutlined />, color: '#f59e0b', gradStart: 'rgba(245,158,11,0.18)',  gradEnd: 'rgba(245,158,11,0.04)'  },
};
const ENV_COLORS: Record<string, string> = { POC: '#3b82f6', FAT: '#10b981', UAT: '#f59e0b', PROD: '#ef4444' };
const ENV_SELECTED_BG: Record<string, [string, string]> = {
  POC:  ['rgba(59,130,246,0.15)',  'rgba(59,130,246,0.08)'],
  FAT:  ['rgba(16,185,129,0.15)',  'rgba(16,185,129,0.08)'],
  UAT:  ['rgba(245,158,11,0.15)', 'rgba(245,158,11,0.08)'],
  PROD: ['rgba(239,68,68,0.15)',  'rgba(239,68,68,0.08)'],
};

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
  // [Ref: 用户需求] 分页 state：受控 pageSize + currentPage，确保 showSizeChanger 选择后自动跳回第1页
  const [tablePageSize, setTablePageSize] = useState(20);
  const [tablePage, setTablePage] = useState(1);

  const {
    globalCostMetrics, namespaceCosts,
    drilldownGlobalProducts, drilldownGlobalProductsPrev,
    loadingGlobalMetrics, loadingDrilldownGlobal,
    errorGlobalMetrics, errorDrilldownGlobal,
    costTimeRange, costCompareMode, costCustomDateRange, selectedDimension,
    fetchGlobalCostMetrics, fetchNamespaceCosts, fetchDrilldownGlobal,
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
  const selectedEnvs: string[] = envsFromUrl && envsFromUrl !== 'all'
    ? envsFromUrl.split(',').map(s => s.trim()).filter(Boolean)
    : [];
  const drilldownEnv = selectedEnvs.length ? selectedEnvs.join(',') : 'all';
  const drilldownCategory = searchParams.get('category') || undefined;
  const drilldownSort = searchParams.get('sort') || 'cost_desc';
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

  useEffect(() => {
    fetchGlobalCostMetrics(selectedEnvs.length ? selectedEnvs : undefined);
    fetchNamespaceCosts();
    fetchDrilldownGlobal(drilldownEnv, drilldownCategory, drilldownSort, { period: effectiveDrilldownPeriod, dateRange: effectiveDrilldownDateRange }, drilldownCompare);
  }, [fetchGlobalCostMetrics, fetchNamespaceCosts, fetchDrilldownGlobal, costTimeRange, costCompareMode, costCustomDateRange, drilldownEnv, drilldownCategory, drilldownSort, effectiveDrilldownPeriod, effectiveDrilldownDateRange, drilldownCompare]);

  useEffect(() => {
    const period = effectiveDrilldownPeriod === '7d_range' ? '7d' : effectiveDrilldownPeriod === 'custom' ? undefined : effectiveDrilldownPeriod;
    const dateFrom = effectiveDrilldownDateRange?.[0];
    const dateTo = effectiveDrilldownDateRange?.[1];
    const envParam = drilldownEnv !== 'all' ? drilldownEnv : undefined;
    if (effectiveDrilldownPeriod === 'custom' && dateFrom && dateTo) {
      fetchCostTrend({ date_from: dateFrom, date_to: dateTo, env: envParam }, drilldownCompare);
    } else if (period) {
      fetchCostTrend({ period, env: envParam }, drilldownCompare);
    }
  }, [effectiveDrilldownPeriod, effectiveDrilldownDateRange, drilldownCompare, drilldownEnv, fetchCostTrend]);

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
      PRELIMINARY:  { label: '动态同步中', color: '#f59e0b', bg: 'rgba(245,158,11,0.12)',  dot: '#f59e0b' },
      RECONCILING:  { label: '对账中',    color: '#3b82f6', bg: 'rgba(59,130,246,0.12)',  dot: '#3b82f6' },
      DIRTY:        { label: '数据偏差',  color: '#ef4444', bg: 'rgba(239,68,68,0.12)',   dot: '#ef4444' },
    };
    const c = cfg[status] ?? { label: '动态同步中', color: '#f59e0b', bg: 'rgba(245,158,11,0.12)', dot: '#f59e0b' };
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
          boxShadow: status === 'PRELIMINARY' || status === 'RECONCILING' ? `0 0 4px ${c.dot}` : 'none',
          animation: status === 'PRELIMINARY' ? 'pulse 2s infinite' : 'none',
          flexShrink: 0,
        }} />
        {c.label}
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

  /* ─── Hero 全环境总成本：以 API total_cost 为唯一来源，与后端聚合一致 [Ref: 去掉总账单功能，与全环境总成本统一] ────────────────────────────────────────────────────── */
  const totalCost = Number(globalCostMetrics?.totalBillableCost ?? 0);
  const prevTotalCost = globalCostMetrics?.previousPeriod?.totalBillableCost ?? null;

  const heroChangePct =
    costCompareMode === 'previous' && prevTotalCost && prevTotalCost > 0
      ? ((totalCost - prevTotalCost) / prevTotalCost) * 100
      : null;

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
                updateParams(n => { n.set('period', val); n.delete('date_from'); n.delete('date_to'); });
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
                  updateParams(n => { n.set('period', 'month'); n.delete('date_from'); n.delete('date_to'); });
                  return;
                }
                const from = dates[0].format('YYYY-MM');
                const to   = dates[1].format('YYYY-MM');
                setCostCustomDateRange([from, to]);
                setCostTimeRange('custom');
                updateParams(n => { n.set('period', 'custom'); n.set('date_from', from); n.set('date_to', to); });
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
        {/* Data note */}
        <div style={{ marginLeft: 'auto', display: 'flex', alignItems: 'flex-end', paddingBottom: 2, gap: 8 }}>
          {globalCostMetrics?.billDataStatus && (
            <BillDataStatusBadge status={globalCostMetrics.billDataStatus} isDark={isDark} />
          )}
          <div style={{ fontSize: 11, color: txt2, display: 'flex', alignItems: 'center', gap: 6 }}>
            <span className="live-dot" />
            {globalCostMetrics?.lastUpdatedAt
              ? `更新至 ${new Date(globalCostMetrics.lastUpdatedAt).toLocaleString('zh-CN', { dateStyle: 'short', timeStyle: 'short' })}`
              : '数据来源：云账单'}
          </div>
        </div>
      </div>
      {costTimeRange === '1d' && (
        <div style={{ marginTop: 8, fontSize: 11, color: txt2 }}>
          昨日与本月在仅有一日数据时可能相同，累积多日后自动区分。
        </div>
      )}
    </div>
  );

  /* ─── Render: Hero + Env Bento ───────────────────────────────────────────── */
  // [Ref: 01_实践] 全环境总成本与环境卡片框架始终渲染；无数据/报错时展示占位（— 或 0.00），不因数据缺失隐藏框架
  const renderHeroBento = () => (
      <div style={{ display: 'grid', gridTemplateColumns: '2fr 1fr 1fr', gridTemplateRows: 'auto auto', gap: 12, marginBottom: 14 }}>
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
            gridRow: '1 / span 2',
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
              <div style={{ fontSize: 11, color: isDark ? 'rgba(255,255,255,0.45)' : 'rgba(0,0,0,0.42)', fontWeight: 500, letterSpacing: '0.06em', textTransform: 'uppercase', marginBottom: 8 }}>
                全环境总成本
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
                      {totalCost.toLocaleString(undefined, { minimumFractionDigits: 2, maximumFractionDigits: 2 })}</>
                    ) : (
                      <span style={{ fontSize: 24 }}>—</span>
                    )}
                  </div>
                  {heroChangePct !== null && (
                    <div style={{ marginTop: 8 }}>
                      <ChangeBadge pct={heroChangePct} inverted />
                      <span style={{ fontSize: 11, color: txt2, marginLeft: 4 }}>较上期</span>
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
            <div style={{ display: 'flex', flexDirection: 'column', alignItems: 'flex-end', gap: 6 }}>
              <Tag style={{
                background: isDark ? 'rgba(59,130,246,0.18)' : 'rgba(59,130,246,0.1)',
                border: `1px solid ${isDark ? 'rgba(59,130,246,0.35)' : 'rgba(59,130,246,0.25)'}`,
                color: '#3b82f6', borderRadius: 20, fontSize: 11, fontWeight: 600, padding: '2px 10px',
              }}>
                云账单
              </Tag>
              {/* 数据状态标识 [Ref: 16_云账单动态对账与高可靠处理规范 §三段式] */}
              {globalCostMetrics?.billDataStatus && (
                <BillDataStatusBadge status={globalCostMetrics.billDataStatus} isDark={isDark} />
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

        {/* 4 Env cards - click to toggle single/multi env filter; 成本分解与云产品明细随选择展示 [Ref: 用户需求 环境多选] */}
        {(['POC', 'FAT', 'UAT', 'PROD'] as const).map(env => {
          const item = globalCostMetrics?.envBreakdown?.find(e => e.environment === env);
          const isConfigured = item && (item.account_id || item.total_cost > 0 || item.account_display_name !== '未配置');
          const isSelected = selectedEnvs.includes(env);
          const totalCostEnv = item?.total_cost ?? 0;
          const changePct = item?.change_pct;
          const color = ENV_COLORS[env];
          return (
            <div
              key={env}
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
                padding: '16px 18px',
                borderLeft: `3px solid ${color}`,
                background: isSelected
                  ? (ENV_SELECTED_BG[env]?.[isDark ? 0 : 1] ?? (isDark ? 'rgba(255,255,255,0.08)' : 'rgba(0,0,0,0.04)'))
                  : (isDark ? `rgba(255,255,255,0.04)` : `rgba(255,255,255,0.78)`),
                outline: isSelected ? `1.5px solid ${color}` : 'none',
                cursor: 'pointer',
                transition: 'background 0.2s, outline 0.2s',
              }}
            >
              <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 6 }}>
                <span style={{ fontSize: 11, fontWeight: 700, letterSpacing: '0.08em', color }}>
                  {env}
                </span>
                <div style={{ display: 'flex', alignItems: 'center', gap: 4 }}>
                  {isSelected && (
                    <span style={{ fontSize: 9, fontWeight: 600, color, background: isDark ? 'rgba(255,255,255,0.1)' : 'rgba(0,0,0,0.06)', padding: '1px 5px', borderRadius: 3, letterSpacing: '0.04em' }}>
                      {selectedEnvs.length > 1 ? '已选' : '已筛选'}
                    </span>
                  )}
                  {loadingGlobalMetrics && <LoadingOutlined style={{ fontSize: 12, color: txt2 }} />}
                </div>
              </div>
              <div style={{ fontSize: 11, color: isConfigured ? txt2 : (isDark ? 'rgba(255,255,255,0.3)' : 'rgba(0,0,0,0.3)'), marginBottom: 4, height: 16, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
                {item?.account_display_name ?? '未配置'}
              </div>
              <div style={{ fontSize: 20, fontWeight: 700, color: isConfigured ? txt1 : (isDark ? 'rgba(255,255,255,0.25)' : 'rgba(0,0,0,0.2)') }}>
                {CURRENCY_SYMBOL}{totalCostEnv.toLocaleString(undefined, { minimumFractionDigits: 2, maximumFractionDigits: 2 })}
              </div>
              {costCompareMode === 'previous' && changePct != null && !Number.isNaN(changePct) ? (
                <div style={{ marginTop: 4, display: 'flex', alignItems: 'center', gap: 4 }}>
                  <ChangeBadge pct={changePct} inverted />
                  <span style={{ fontSize: 10, color: txt2 }}>较上期</span>
                </div>
              ) : (
                <div style={{ marginTop: 4, height: 18 }}>
                  {!isConfigured && (
                    <span style={{ fontSize: 10, color: isDark ? 'rgba(255,255,255,0.25)' : 'rgba(0,0,0,0.25)' }}>点击查看明细</span>
                  )}
                </div>
              )}
            </div>
          );
        })}
      </div>
  );

  /* ─── Render: KPI Cards (placeholder when no data) ─────────────────────────── */
  const renderKpiCardsPlaceholder = () => {
    const labels = [
      { label: '可优化空间', tip: '云账单模式下暂不计算优化空间。', accentColor: '#8b5cf6' },
      { label: '全局效率分', tip: '云账单模式下暂不计算效率分。', accentColor: '#10b981' },
    ];
    return (
      <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr auto', gap: 12, marginBottom: 14, alignItems: 'start' }}>
        {labels.map((k, i) => (
          <div key={i} className="bento-card" style={{ ...gc, padding: '20px 22px', borderTop: `3px solid ${k.accentColor}` }}>
            <div style={{ display: 'flex', alignItems: 'center', gap: 6, marginBottom: 12, fontSize: 12, color: txt2, fontWeight: 500 }}>
              {k.label}
              <Tooltip title={k.tip}><QuestionCircleOutlined style={{ fontSize: 11, cursor: 'help' }} /></Tooltip>
            </div>
            <div style={{ fontSize: 28, fontWeight: 800, color: txt2, letterSpacing: '-0.02em' }}>—</div>
          </div>
        ))}
        <div className="bento-card" style={{ ...gc, padding: '20px', display: 'flex', alignItems: 'center', justifyContent: 'center' }}>
          <EfficiencyChart efficiency={0} size={88} />
        </div>
      </div>
    );
  };

  /* ─── Render: KPI Cards ──────────────────────────────────────────────────── */
  // [Ref: 01_实践] 可优化空间/全局效率分框架始终渲染；无数据时展示占位（—），错误时先展示 Alert 再展示占位卡片
  const renderKpiCards = () => {
    if (loadingGlobalMetrics && !globalCostMetrics) {
      return (
        <div style={{ ...gc, padding: 28, marginBottom: 14, display: 'flex', alignItems: 'center', justifyContent: 'center', gap: 12 }}>
          <LoadingOutlined spin style={{ fontSize: 20, color: '#3b82f6' }} />
          <span style={{ color: txt2 }}>加载指标中…</span>
        </div>
      );
    }
    const showErrorBanner = errorGlobalMetrics && !globalCostMetrics;
    if (showErrorBanner) {
      return (
        <>
          <Alert
            message="数据暂未就绪"
            description={errorGlobalMetrics}
            type="warning"
            showIcon
            action={<Button size="small" onClick={() => { resetErrors(); fetchGlobalCostMetrics(); fetchNamespaceCosts(); }}>重试</Button>}
            style={{ marginBottom: 14 }}
          />
          {renderKpiCardsPlaceholder()}
        </>
      );
    }
    if (!globalCostMetrics) return renderKpiCardsPlaceholder();

    const prev = globalCostMetrics.previousPeriod;
    const isPlaceholder = globalCostMetrics.totalOptimizableSpace === 0 && globalCostMetrics.globalEfficiency === 0;
    const optimChange = prev && prev.totalOptimizableSpace > 0 ? ((globalCostMetrics.totalOptimizableSpace - prev.totalOptimizableSpace) / prev.totalOptimizableSpace) * 100 : null;
    const effChange = prev ? globalCostMetrics.globalEfficiency - prev.globalEfficiency : null;

    const kpiData = [
      {
        label: '可优化空间',
        tip: isPlaceholder ? '云账单模式下暂不计算优化空间。' : '账单成本减使用成本，点击下钻查看明细。',
        value: isPlaceholder ? null : globalCostMetrics.totalOptimizableSpace,
        prefix: CURRENCY_SYMBOL,
        change: !isPlaceholder && costCompareMode === 'previous' ? optimChange : null,
        inverted: true,
        onClick: !isPlaceholder ? () => navigate(`/DrilldownPage?dimension=${selectedDimension}&focus=optimizable`) : undefined,
        accentColor: '#8b5cf6',
      },
      {
        label: '全局效率分',
        tip: isPlaceholder ? '云账单模式下暂不计算效率分。' : '使用成本/账单成本×100%。',
        value: isPlaceholder ? null : globalCostMetrics.globalEfficiency,
        suffix: isPlaceholder ? undefined : '%',
        change: !isPlaceholder && costCompareMode === 'previous' ? effChange : null,
        inverted: false,
        onClick: !isPlaceholder ? () => setDetailModal('efficiency') : undefined,
        accentColor: '#10b981',
      },
    ];

    return (
      <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr auto', gap: 12, marginBottom: 14, alignItems: 'start' }}>
        {kpiData.map((k, i) => (
          <div
            key={i}
            className={`bento-card${k.onClick ? ' bento-card-clickable' : ''}`}
            onClick={k.onClick}
            style={{
              ...gc,
              padding: '20px 22px',
              cursor: k.onClick ? 'pointer' : 'default',
              borderTop: `3px solid ${k.accentColor}`,
            }}
          >
            <div style={{ display: 'flex', alignItems: 'center', gap: 6, marginBottom: 12, fontSize: 12, color: txt2, fontWeight: 500 }}>
              {k.label}
              <Tooltip title={k.tip}>
                <QuestionCircleOutlined style={{ fontSize: 11, cursor: 'help' }} />
              </Tooltip>
            </div>
            <div className="kpi-value-appear" style={{ fontSize: 28, fontWeight: 800, color: txt1, letterSpacing: '-0.02em', lineHeight: 1 }}>
              {k.value === null ? (
                <span style={{ fontSize: 20, color: txt2 }}>—</span>
              ) : (
                <>
                  {k.prefix && <span style={{ fontSize: 16, fontWeight: 600, marginRight: 1, opacity: 0.65 }}>{k.prefix}</span>}
                  {Number(k.value).toLocaleString(undefined, { minimumFractionDigits: 2, maximumFractionDigits: 2 })}
                  {k.suffix && <span style={{ fontSize: 16, marginLeft: 2 }}>{k.suffix}</span>}
                </>
              )}
            </div>
            {k.change !== null && k.value !== null && (
              <div style={{ marginTop: 8 }}>
                <ChangeBadge pct={k.change} inverted={k.inverted} />
                <span style={{ fontSize: 11, color: txt2, marginLeft: 4 }}>较上期</span>
              </div>
            )}
          </div>
        ))}
        {/* Efficiency Donut */}
        <div className="bento-card" style={{ ...gc, padding: '20px', display: 'flex', alignItems: 'center', justifyContent: 'center' }}>
          <EfficiencyChart efficiency={globalCostMetrics.globalEfficiency} size={88} />
        </div>
      </div>
    );
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
                <div style={{ height: 4, background: isDark ? 'rgba(255,255,255,0.08)' : 'rgba(0,0,0,0.08)', borderRadius: 2, overflow: 'hidden', marginBottom: 10 }}>
                  <div
                    className="domain-bar-fill"
                    style={{ height: '100%', width: `${pct}%`, background: `linear-gradient(90deg, ${meta.color}, ${meta.color}88)`, borderRadius: 2 }}
                  />
                </div>
                {/* Efficiency */}
                <div style={{ fontSize: 11, color: txt2 }}>
                  效率: <span style={{ color: domain.efficiency > 60 ? '#10b981' : domain.efficiency > 30 ? '#f59e0b' : txt2, fontWeight: 600 }}>
                    {domain.efficiency > 0 ? `${domain.efficiency}%` : '—'}
                  </span>
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
        <Select value={drilldownEnv} style={{ width: 110 }} options={[{ label: '全环境', value: 'all' }, { label: 'POC', value: 'POC' }, { label: 'FAT', value: 'FAT' }, { label: 'UAT', value: 'UAT' }, { label: 'PROD', value: 'PROD' }]}
          onChange={v => updateParams(n => { n.set('env', v); })}
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

    const topProducts = React.useMemo(() => {
      const sums: Record<string, number> = {};
      for (const pt of cur) {
        for (const [code, cost] of Object.entries(pt.by_product ?? {})) {
          sums[code] = (sums[code] ?? 0) + Math.abs(cost);
        }
      }
      return Object.entries(sums).sort((a, b) => b[1] - a[1]).slice(0, 8).map(e => e[0]);
    }, [cur]);

    const productChartData = React.useMemo(() => {
      return cur.map(pt => {
        const row: Record<string, number | string | null> = { date: pt.date };
        for (const code of topProducts) {
          row[code] = pt.by_product?.[code] ?? 0;
        }
        return row;
      });
    }, [cur, topProducts]);

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
                const label = CATEGORY_TO_LABEL[v] ?? v ?? '计算资源';
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
            {globalCostMetrics && (globalCostMetrics.totalBillableCost ?? 0) > 0 ? '数据来源：云账单' : '数据来源：—'}
          </span>
        </div>
        <div style={{ display: 'flex', alignItems: 'center', gap: 6, fontSize: 11, color: txt2 }}>
          <span className="live-dot" />
          {globalCostMetrics?.lastUpdatedAt
            ? `数据更新至 ${new Date(globalCostMetrics.lastUpdatedAt).toLocaleString('zh-CN', { dateStyle: 'short', timeStyle: 'short' })}`
            : errorGlobalMetrics ? '暂未就绪' : '—'}
        </div>
      </div>

      {/* ── Alert: no data ── */}
      {globalCostMetrics && (globalCostMetrics.totalBillableCost ?? 0) === 0 && !errorGlobalMetrics && (
        <Alert
          message="暂无真实数据"
          description="请完成：1) 配置阿里云 AK/SK 并执行 ETL，将云账单写入日/月原始表与聚合表（cost_cloud_bill_daily_raw、cost_cloud_bill_aggregate 等）；2) 接入 Prometheus/K8s 获取集群内计算成本。部署后首次可触发全量回填或等待定时 ETL。"
          type="warning" showIcon style={{ marginBottom: 14, borderRadius: 12 }}
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

        {/* KPI 4 cards */}
        {renderKpiCards()}

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
                ...(namespaceCosts || []).map(n => ({ key: `ns-${n.namespace}`, name: n.namespace, efficiency: n.efficiency, type: '命名空间' })),
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
