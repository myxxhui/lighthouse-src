import React, { useEffect, useState } from 'react';
import { useNavigate, useSearchParams } from 'umi';
import {
  Card,
  Row,
  Col,
  Statistic,
  Switch,
  Space,
  Alert,
  Tooltip,
  Select,
  Modal,
  Descriptions,
  Table,
  Tabs,
  DatePicker,
} from 'antd';
import dayjs, { type Dayjs } from 'dayjs';
import { LoadingOutlined, QuestionCircleOutlined } from '@ant-design/icons';
import CostTable from '@/components/CostTable';
import EfficiencyChart from '@/components/EfficiencyChart';
import ROITrendSection from '@/components/ROITrendSection';
import { useAppStore } from '@/store';
import { LineChart, Line, XAxis, YAxis, CartesianGrid, Tooltip as RechartsTooltip, ResponsiveContainer, Legend } from 'recharts';
import type { CostTimeRange, CostCompareMode, CloudProductDrilldownItem } from '@/types';
import type { DomainBreakdown } from '@/types';
import { CURRENCY_SYMBOL } from '@/constants';

// [Ref: 01_实践 §3.1 DNA front_end_time_ranges] 昨天、这周、近七天、上周、这月、近30天、上月、这季度、近90天、上季度、今年、去年 + 自定义
const TIME_RANGE_OPTIONS: { label: string; value: CostTimeRange }[] = [
  { label: '昨天', value: '1d' },
  { label: '这周', value: 'this_week' },
  { label: '近七天', value: '7d_range' },
  { label: '上周', value: 'last_week' },
  { label: '这月', value: 'month' },
  { label: '近30天', value: '30d' },
  { label: '上月', value: 'last_month' },
  { label: '这季度', value: 'quarter' },
  { label: '近90天', value: '90d' },
  { label: '上季度', value: 'last_quarter' },
  { label: '今年', value: 'this_year' },
  { label: '去年', value: 'last_year' },
  { label: '自定义', value: 'custom' },
];

const COMPARE_OPTIONS: { label: string; value: CostCompareMode }[] = [
  { label: '不对比', value: 'none' },
  { label: '对比上一周期', value: 'previous' },
];

// [Ref: 01_设计 §成本分解大类与 API category 映射] 跳转云产品明细时带 category 参数
const DOMAIN_TO_CATEGORY: Record<string, string> = {
  '计算资源': 'compute',
  '存储': 'storage',
  '网络': 'network',
  '安全': 'security',
  '其他': 'other',
};

type DetailModalType = 'bill' | 'efficiency' | null;

const CostOverviewPage: React.FC = () => {
  const navigate = useNavigate();
  const [searchParams, setSearchParams] = useSearchParams();
  const tabFromUrl = searchParams.get('tab');
  const activeTab = tabFromUrl === 'roi' ? 'roi' : 'cost';
  const [detailModal, setDetailModal] = useState<DetailModalType>(null);
  const [domainDetail, setDomainDetail] = useState<DomainBreakdown | null>(null);
  const [highlightCloudProduct, setHighlightCloudProduct] = useState(false);
  const {
    globalCostMetrics,
    namespaceCosts,
    drilldownGlobalProducts,
    drilldownGlobalProductsPrev,
    loadingGlobalMetrics,
    loadingNamespaceCosts,
    loadingDrilldownGlobal,
    errorGlobalMetrics,
    errorNamespaceCosts,
    errorDrilldownGlobal,
    useMockData,
    costTimeRange,
    costCompareMode,
    costCustomDateRange,
    selectedDimension,
    fetchGlobalCostMetrics,
    fetchNamespaceCosts,
    fetchDrilldownGlobal,
    fetchCostTrend,
    costTrendData,
    costTrendDataPrev,
    loadingCostTrend,
    errorCostTrend,
    setUseMockData,
    setCostTimeRange,
    setCostCompareMode,
    setCostCustomDateRange,
    theme,
    setTheme,
  } = useAppStore();

  // [Ref: 01_实践 D9-15、D4] 时间范围、对比模式、自定义日期从 URL 恢复，便于分享与新开标签一致
  useEffect(() => {
    const period = searchParams.get('period');
    const compare = searchParams.get('compare');
    const date_from = searchParams.get('date_from');
    const date_to = searchParams.get('date_to');
    const validPeriods = new Set(TIME_RANGE_OPTIONS.map(o => o.value));
    const validCompares = new Set(COMPARE_OPTIONS.map(o => o.value));
    if (period && validPeriods.has(period as CostTimeRange)) {
      setCostTimeRange(period as CostTimeRange);
    }
    if (compare && validCompares.has(compare as CostCompareMode)) {
      setCostCompareMode(compare as CostCompareMode);
    }
    if ((period === 'custom' || costTimeRange === 'custom') && date_from && date_to) {
      setCostCustomDateRange([date_from, date_to]);
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  const drilldownEnv = searchParams.get('env') || 'all';
  const drilldownCategory = searchParams.get('category') || undefined;
  const drilldownSort = searchParams.get('sort') || 'cost_desc';
  const drilldownCompare = searchParams.get('compare_trend') === '1';
  const showTrendChart = searchParams.get('show_trend') === '1'; // [Ref: 01_设计 §成本趋势 API 趋势总图默认关闭且收起]
  // [Ref: 01_设计 §云产品成本明细索引 索引区可独立选择时间范围] 仅作用于云产品明细与该区趋势/环比
  const indexPeriod = searchParams.get('index_period') as CostTimeRange | null;
  const indexDateFrom = searchParams.get('index_date_from') ?? null;
  const indexDateTo = searchParams.get('index_date_to') ?? null;
  const effectiveDrilldownPeriod: CostTimeRange = indexPeriod ?? costTimeRange;
  const effectiveDrilldownDateRange: [string, string] | null =
    (indexPeriod === 'custom' && indexDateFrom && indexDateTo ? [indexDateFrom, indexDateTo] : null) ?? costCustomDateRange;

  useEffect(() => {
    fetchGlobalCostMetrics();
    fetchNamespaceCosts();
    if (!useMockData) {
      fetchDrilldownGlobal(drilldownEnv, drilldownCategory, drilldownSort, { period: effectiveDrilldownPeriod, dateRange: effectiveDrilldownDateRange }, drilldownCompare);
    }
  }, [fetchGlobalCostMetrics, fetchNamespaceCosts, fetchDrilldownGlobal, useMockData, costTimeRange, costCompareMode, costCustomDateRange, drilldownEnv, drilldownCategory, drilldownSort, effectiveDrilldownPeriod, effectiveDrilldownDateRange, drilldownCompare]);
  // [Ref: 01_设计 D9-16] 云产品明细索引区趋势图：按索引区时间范围拉取，可选环比
  useEffect(() => {
    if (useMockData) return;
    const period = effectiveDrilldownPeriod === '7d_range' ? '7d' : effectiveDrilldownPeriod === 'custom' ? undefined : effectiveDrilldownPeriod;
    const dateFrom = effectiveDrilldownDateRange?.[0];
    const dateTo = effectiveDrilldownDateRange?.[1];
    if (effectiveDrilldownPeriod === 'custom' && dateFrom && dateTo) {
      fetchCostTrend({ date_from: dateFrom, date_to: dateTo }, drilldownCompare);
    } else if (period) {
      fetchCostTrend({ period }, drilldownCompare);
    }
  }, [useMockData, effectiveDrilldownPeriod, effectiveDrilldownDateRange, drilldownCompare, fetchCostTrend]);

  const handleRowClick = (record: any) => {
    navigate(
      `/DrilldownPage?dimension=${selectedDimension}&type=namespace&id=${encodeURIComponent(record.namespace)}`,
    );
  };

  const renderGlobalMetrics = () => {
    if (loadingGlobalMetrics) {
      const isDateRange = costCustomDateRange != null && costCustomDateRange[0] && costCustomDateRange[1];
      const daysHint =
        isDateRange && costCustomDateRange
          ? Math.max(0, Math.ceil((new Date(costCustomDateRange[1]).getTime() - new Date(costCustomDateRange[0]).getTime()) / 86400000) + 1)
          : 0;
      return (
        <Card loading={true}>
          <div style={{ display: 'flex', flexDirection: 'column', alignItems: 'center', gap: 8 }}>
            <LoadingOutlined spin style={{ fontSize: 24 }} />
            <span>{isDateRange ? '正在汇总所选日期成本…' : '加载中...'}</span>
            {isDateRange && daysHint > 0 && (
              <span style={{ fontSize: 12, color: '#666' }}>
                已选 {daysHint} 天，可能需要 10～30 秒
              </span>
            )}
          </div>
        </Card>
      );
    }

    if (errorGlobalMetrics && !globalCostMetrics) {
      return (
        <Alert message="加载全局指标失败" description={errorGlobalMetrics} type="error" showIcon />
      );
    }

    if (!globalCostMetrics) {
      return <Card>暂无数据</Card>;
    }

    const prev = globalCostMetrics.previousPeriod;
    // [Ref: 01_实践 D9-13] 云账单模式下成本优化空间、全局效率分仅占位，展示「—」
    const isPlaceholderOptimEff =
      !useMockData &&
      globalCostMetrics.totalOptimizableSpace === 0 &&
      globalCostMetrics.globalEfficiency === 0;
    // 周期对比算法：环比 = (本期 - 上期) / 上期 * 100；效率为百分点差
    const costChange =
      prev && prev.totalBillableCost > 0
        ? ((globalCostMetrics.totalBillableCost - prev.totalBillableCost) / prev.totalBillableCost) * 100
        : null;
    const optimChange =
      prev && prev.totalOptimizableSpace > 0
        ? ((globalCostMetrics.totalOptimizableSpace - prev.totalOptimizableSpace) /
            prev.totalOptimizableSpace) *
          100
        : null;
    const effChange = prev ? globalCostMetrics.globalEfficiency - prev.globalEfficiency : null;

    return (
      <>
        {errorGlobalMetrics && (
          <Alert
            message="数据暂未更新"
            description={errorGlobalMetrics}
            type="warning"
            showIcon
            style={{ marginBottom: 12 }}
          />
        )}
        <Row gutter={[16, 16]}>
        <Col xs={24} sm={12} md={6}>
          <Card hoverable onClick={() => setDetailModal('bill')} style={{ cursor: 'pointer' }}>
            <Statistic
              title={
                <Space>
                  总账单成本
                  <Tooltip title="各资源类型（计算、存储、网络、安全、其他）计费成本汇总。明细见下方成本分解与云产品成本明细。">
                    <QuestionCircleOutlined style={{ color: '#999', cursor: 'help' }} />
                  </Tooltip>
                </Space>
              }
              value={globalCostMetrics.totalBillableCost}
              prefix={CURRENCY_SYMBOL}
              formatter={value => Number(value).toLocaleString()}
              suffix={
                costCompareMode === 'previous' && costChange != null ? (
                  <span
                    style={{
                      fontSize: 12,
                      marginLeft: 4,
                      color: costChange >= 0 ? '#ff4d4f' : '#52c41a',
                    }}
                  >
                    {costChange >= 0 ? '+' : ''}
                    {costChange.toFixed(1)}% 较上期
                  </span>
                ) : null
              }
            />
          </Card>
        </Col>
        <Col xs={24} sm={12} md={6}>
          <Card
            hoverable={!isPlaceholderOptimEff}
            onClick={!isPlaceholderOptimEff ? () => navigate(`/DrilldownPage?dimension=${selectedDimension}&focus=optimizable`) : undefined}
            style={{ cursor: isPlaceholderOptimEff ? 'default' : 'pointer' }}
          >
            <Statistic
              title={
                <Space>
                  可优化空间
                  <Tooltip title={isPlaceholderOptimEff ? '当前仅云账单模式，无优化空间计算；后续开放。' : '各资源类型可优化空间汇总（账单成本减使用成本）。点击可下钻至命名空间、服务组、Pod 查看明细。'}>
                    <QuestionCircleOutlined style={{ color: '#999', cursor: 'help' }} />
                  </Tooltip>
                </Space>
              }
              value={isPlaceholderOptimEff ? '—' : globalCostMetrics.totalOptimizableSpace}
              prefix={!isPlaceholderOptimEff ? CURRENCY_SYMBOL : undefined}
              formatter={isPlaceholderOptimEff ? undefined : (value: number) => Number(value).toLocaleString()}
              suffix={
                costCompareMode === 'previous' && !isPlaceholderOptimEff && optimChange != null ? (
                  <span
                    style={{
                      fontSize: 12,
                      marginLeft: 4,
                      color: optimChange >= 0 ? '#ff4d4f' : '#52c41a',
                    }}
                  >
                    {optimChange >= 0 ? '+' : ''}
                    {optimChange.toFixed(1)}% 较上期
                  </span>
                ) : null
              }
            />
          </Card>
        </Col>
        <Col xs={24} sm={12} md={6}>
          <Card hoverable={!isPlaceholderOptimEff} onClick={!isPlaceholderOptimEff ? () => setDetailModal('efficiency') : undefined} style={{ cursor: isPlaceholderOptimEff ? 'default' : 'pointer' }}>
            <Statistic
              title={
                <Space>
                  全局效率分
                  <Tooltip title={isPlaceholderOptimEff ? '当前仅云账单模式，无效率分计算；后续开放。' : '汇总使用成本/汇总账单成本×100%。各层级效率构成见领域与命名空间明细。'}>
                    <QuestionCircleOutlined style={{ color: '#999', cursor: 'help' }} />
                  </Tooltip>
                </Space>
              }
              value={isPlaceholderOptimEff ? '—' : globalCostMetrics.globalEfficiency}
              suffix={
                !isPlaceholderOptimEff ? (
                  <>
                    %
                    {costCompareMode === 'previous' && effChange != null ? (
                      <span
                        style={{
                          fontSize: 12,
                          marginLeft: 4,
                          color: effChange >= 0 ? '#52c41a' : '#ff4d4f',
                        }}
                      >
                        {effChange >= 0 ? '+' : ''}
                        {effChange.toFixed(1)}% 较上期
                      </span>
                    ) : null}
                  </>
                ) : null}
            />
          </Card>
        </Col>
        <Col xs={24} sm={12} md={6}>
          <Card>
            <div
              style={{
                display: 'flex',
                justifyContent: 'center',
                alignItems: 'center',
                height: '100%',
              }}
            >
              <EfficiencyChart efficiency={globalCostMetrics.globalEfficiency} size={80} />
            </div>
          </Card>
        </Col>
      </Row>
      </>
    );
  };

  const renderDomainBreakdown = () => {
    if (!globalCostMetrics?.domainBreakdown?.length) {
      return null;
    }

    // [Ref: 01_设计 §成本结构、成本分解类、云产品成本明细] 四大类展示；其它→其他；点击展示 Top4+查看更多→云产品成本明细
    const domainOrder = ['计算资源', '存储', '网络', '安全', '其他'];
    const ordered = domainOrder.reduce<(typeof globalCostMetrics.domainBreakdown)[0][]>((acc, name) => {
      const found = globalCostMetrics.domainBreakdown.find(d => d.domain === name || (d.domain === '其它' && name === '其他'));
      if (found) acc.push({ ...found, domain: found.domain === '其它' ? '其他' : found.domain });
      else if (name === '安全' || name === '其他') acc.push({ domain: name, cost: 0, optimizableSpace: 0, efficiency: 0, topProducts: [] });
      return acc;
    }, []);
    const displayList = ordered.length > 0 ? ordered : globalCostMetrics.domainBreakdown.map(d => ({ ...d, domain: d.domain === '其它' ? '其他' : d.domain }));

    return (
      <Card title="成本分解" style={{ marginTop: 16 }}>
        <Row gutter={[16, 16]}>
          {displayList.map((domain, index) => {
            const isStorageOrNetwork = domain.domain === '存储' || domain.domain === '网络';
            const card = (
              <Card
                size="small"
                hoverable
                onClick={() => {
                  // [Ref: 01_设计 D9-17 成本分解某大类点击 → 进入索引，默认 category=该大类、env=全环境、时间=当前]
                  const category = DOMAIN_TO_CATEGORY[domain.domain] || 'other';
                  setSearchParams((prev: URLSearchParams) => {
                    const next = new URLSearchParams(prev);
                    next.set('env', 'all');
                    next.set('period', costTimeRange);
                    if (costTimeRange === 'custom' && costCustomDateRange?.[0] && costCustomDateRange?.[1]) {
                      next.set('date_from', costCustomDateRange[0]);
                      next.set('date_to', costCustomDateRange[1]);
                    }
                    next.set('category', category);
                    return next;
                  });
                  setHighlightCloudProduct(true);
                  setTimeout(() => setHighlightCloudProduct(false), 2500);
                  setTimeout(() => document.getElementById('cloud-product-detail')?.scrollIntoView({ behavior: 'smooth' }), 0);
                  setDomainDetail(domain);
                }}
                style={{ cursor: 'pointer' }}
              >
                <Statistic
                  title={
                    isStorageOrNetwork ? (
                      <Tooltip title="当前仅展示成本金额，深度分析后续开放">
                        <span>{domain.domain} <QuestionCircleOutlined style={{ fontSize: 12, color: '#999' }} /></span>
                      </Tooltip>
                    ) : (
                      domain.domain
                    )
                  }
                  value={domain.cost}
                  prefix={CURRENCY_SYMBOL}
                  formatter={value => Number(value).toLocaleString()}
                  suffix={
                    <div style={{ marginTop: 8 }}>
                      <small>效率: {domain.efficiency}%</small>
                    </div>
                  }
                />
              </Card>
            );
            return (
              <Col key={domain.domain + index} xs={24} sm={12} md={8} lg={6}>
                {card}
              </Col>
            );
          })}
        </Row>
      </Card>
    );
  };

  return (
    <div>
      <div
        style={{
          display: 'flex',
          justifyContent: 'space-between',
          alignItems: 'center',
          marginBottom: 16,
        }}
      >
        <h2>全域成本透视</h2>
        <Space>
          <span style={{ color: '#666', fontSize: 12 }}>
            {useMockData ? '数据来源：Mock' : (globalCostMetrics && globalCostMetrics.totalBillableCost > 0 ? '数据来源：云账单' : '数据来源：—')}
            {globalCostMetrics?.lastUpdatedAt && (
              <> · 数据更新至 {new Date(globalCostMetrics.lastUpdatedAt).toLocaleString('zh-CN', { dateStyle: 'short', timeStyle: 'short' })}</>
            )}
          </span>
          <span>使用Mock数据</span>
          <Switch
            checked={useMockData}
            onChange={setUseMockData}
            checkedChildren="是"
            unCheckedChildren="否"
          />
          <span style={{ marginLeft: 12 }}>深色主题</span>
          <Switch
            checked={theme === 'dark'}
            onChange={on => setTheme(on ? 'dark' : 'light')}
            checkedChildren="开"
            unCheckedChildren="关"
          />
        </Space>
      </div>

      {!useMockData &&
        globalCostMetrics &&
        globalCostMetrics.totalBillableCost === 0 &&
        (!namespaceCosts || namespaceCosts.length === 0) && (
          <Alert
            message="暂无真实数据"
            description="当前未接入真实数据源。请完成：1) 01_ 成本透视真实数据：配置阿里云 AK/SK 并执行 ETL，将云账单写入 cost_cloud_bill_summary；2) 02_ 真实数据源：接入 Prometheus/K8s 获取集群内计算成本。完成后关闭 Mock 即可看到真实成本与领域占比。"
            type="warning"
            showIcon
            style={{ marginBottom: 16 }}
          />
        )}

      {/* [Ref: 01_实践 D9-14] 顶部时间区：仅时间范围 Select + 对比；不包含「自定义日期」区块，自定义日期仅保留下方索引区一处入口。时间范围必须用 Select 下拉，禁止用 Segmented，否则窄屏会截断「近7天」「近30天」「今年」「去年」「自定义」等文案。 */}
      <Card style={{ marginBottom: 16 }}>
        <Space wrap size="middle" style={{ alignItems: 'center' }}>
          <span>时间范围：</span>
          <Select
            value={costCustomDateRange != null && costCustomDateRange[0] && costCustomDateRange[1] ? 'custom' : costTimeRange}
            onChange={v => {
              const val = v as CostTimeRange;
              if (val === 'custom') {
                setCostTimeRange('custom');
                setCostCustomDateRange(null);
              } else {
                setCostCustomDateRange(null);
                setCostTimeRange(val);
              }
              setSearchParams((prev: URLSearchParams) => {
                const next = new URLSearchParams(prev);
                next.set('period', val);
                if (val !== 'custom') next.delete('date_from'), next.delete('date_to'); // 非自定义时清除日期参数
                return next;
              });
            }}
            options={TIME_RANGE_OPTIONS}
            style={{ minWidth: 160 }}
            getPopupContainer={trigger => trigger.parentElement ?? document.body}
          />
          <span style={{ marginLeft: 8 }}>对比：</span>
          <Select
            value={costCompareMode}
            onChange={v => {
              const mode = v as CostCompareMode;
              setCostCompareMode(mode);
              setSearchParams((prev: URLSearchParams) => {
                const next = new URLSearchParams(prev);
                next.set('compare', mode);
                return next;
              });
            }}
            options={COMPARE_OPTIONS}
            style={{ width: 140 }}
          />
        </Space>
        {!useMockData && costTimeRange === '1d' && (
          <div style={{ marginTop: 8, color: 'var(--ant-color-text-secondary)', fontSize: 12 }}>
            若当前仅有一日原始数据，昨日与本月可能显示相同金额，属正常；累积多日后会区分。
          </div>
        )}
        {/* D9-14: 顶部不渲染「自定义日期」区块或 RangePicker；仅此一行说明，日期选择仅在下方案云产品成本明细索引区 */}
        <div style={{ marginTop: 8, fontSize: 12, color: 'rgba(0,0,0,0.45)' }}>
          <Tooltip title={costTimeRange === 'custom' ? '请在下方「云产品成本明细」索引区选择日期范围（最多 6 个月）' : '数据为 T+1，不含当日'}>
            <span>{costTimeRange === 'custom' ? '自定义日期请在下方「云产品成本明细」索引区选择日期范围' : '数据截止昨日'}</span>
          </Tooltip>
        </div>
      </Card>

      {/* [Ref: 01_设计 §按环境展示、D2 方案 B] 全环境总成本：有 envBreakdown 则 sum(env)；自定义日期无 env 时用 totalBillableCost 展示并注明口径 */}
      {(globalCostMetrics?.envBreakdown?.length ||
        (globalCostMetrics != null && Number(globalCostMetrics.totalBillableCost) > 0)) ? (
        <Card
          size="small"
          onClick={() => {
            setSearchParams((prev: URLSearchParams) => {
              const next = new URLSearchParams(prev);
              next.set('env', 'all');
              next.set('period', costTimeRange);
              if (costTimeRange === 'custom' && costCustomDateRange?.[0] && costCustomDateRange?.[1]) {
                next.set('date_from', costCustomDateRange[0]);
                next.set('date_to', costCustomDateRange[1]);
              }
              return next;
            });
            setHighlightCloudProduct(true);
            setTimeout(() => setHighlightCloudProduct(false), 2500);
            setTimeout(() => document.getElementById('cloud-product-detail')?.scrollIntoView({ behavior: 'smooth' }), 0);
          }}
          style={{
            marginBottom: 16,
            background: 'linear-gradient(135deg, #e6f4ff 0%, #f0f5ff 50%, #fff 100%)',
            borderLeft: '5px solid #1677ff',
            borderRadius: 8,
            boxShadow: '0 1px 4px rgba(22,119,255,0.12)',
            cursor: 'pointer',
          }}
        >
          <div style={{ marginBottom: 4, fontSize: 12, color: 'rgba(0,0,0,0.45)', fontWeight: 500 }}>总览</div>
          <Statistic
            title={
              <Tooltip title={!globalCostMetrics?.envBreakdown?.length ? '全环境汇总（所选日期）' : undefined}>
                <span style={{ fontWeight: 600, color: 'rgba(0,0,0,0.85)', fontSize: 15 }}>
                  全环境总成本
                  {!globalCostMetrics?.envBreakdown?.length && (
                    <span style={{ marginLeft: 6, fontSize: 12, fontWeight: 400, color: '#666' }}>（所选日期）</span>
                  )}
                </span>
              </Tooltip>
            }
            value={
              globalCostMetrics?.envBreakdown?.length
                ? (globalCostMetrics.envBreakdown as { total_cost?: number }[]).reduce(
                    (s, e) => s + (e.total_cost ?? 0),
                    0,
                  )
                : Number(globalCostMetrics?.totalBillableCost ?? 0)
            }
            prefix={CURRENCY_SYMBOL}
            formatter={value => Number(value).toLocaleString()}
            valueStyle={{ fontSize: 28, fontWeight: 700 }}
            suffix={
              costCompareMode === 'previous' &&
              globalCostMetrics?.envBreakdown?.length &&
              (() => {
                const prevTotal = (globalCostMetrics!.envBreakdown as { previous_period_cost?: number }[]).reduce(
                  (s, e) => s + (e.previous_period_cost ?? 0),
                  0,
                );
                const curTotal = (globalCostMetrics!.envBreakdown as { total_cost?: number }[]).reduce(
                  (s, e) => s + (e.total_cost ?? 0),
                  0,
                );
                if (prevTotal <= 0) return null;
                const pct = ((curTotal - prevTotal) / prevTotal) * 100;
                return (
                  <span style={{ fontSize: 14, marginLeft: 8, fontWeight: 500, color: pct >= 0 ? '#ff4d4f' : '#52c41a' }}>
                    {pct >= 0 ? '+' : ''}
                    {pct.toFixed(1)}% 较上期
                  </span>
                );
              })()
            }
          />
        </Card>
      ) : null}

      {/* 四环境卡片 POC/FAT/UAT/PROD [Ref: 01_设计 §按环境展示 D9-4、D11 色条区分] */}
      <Card title="按环境总账" style={{ marginBottom: 16 }}>
        <Row gutter={[16, 16]}>
          {(['POC', 'FAT', 'UAT', 'PROD'] as const).map(env => {
            const item = globalCostMetrics?.envBreakdown?.find(e => e.environment === env);
            const isConfigured = item && (item.account_id || item.total_cost > 0 || item.account_display_name !== '未配置');
            const displayName = item?.account_display_name ?? '未配置';
            const totalCost = item?.total_cost ?? 0;
            const changePct = item?.change_pct;
            const envColors: Record<string, string> = { POC: '#1677ff', FAT: '#52c41a', UAT: '#faad14', PROD: '#ff4d4f' };
            const card = (
              <Col key={env} xs={24} sm={12} md={6}>
                <Card
                  size="small"
                  title={<span style={{ borderLeft: `4px solid ${envColors[env] || '#999'}`, paddingLeft: 8 }}>{env}</span>}
                  hoverable={!!isConfigured}
                  onClick={
                    isConfigured
                      ? () => {
                          setSearchParams((prev: URLSearchParams) => {
                            const next = new URLSearchParams(prev);
                            next.set('env', env);
                            next.set('period', costTimeRange);
                            if (costTimeRange === 'custom' && costCustomDateRange?.[0] && costCustomDateRange?.[1]) {
                              next.set('date_from', costCustomDateRange[0]);
                              next.set('date_to', costCustomDateRange[1]);
                            }
                            return next;
                          });
                          setTimeout(() => document.getElementById('cloud-product-detail')?.scrollIntoView({ behavior: 'smooth' }), 0);
                        }
                      : undefined
                  }
                  style={{ cursor: isConfigured ? 'pointer' : 'default' }}
                >
                  <Statistic
                    title={displayName}
                    value={totalCost}
                    prefix={CURRENCY_SYMBOL}
                    formatter={value => Number(value).toLocaleString()}
                    suffix={
                      costCompareMode === 'previous' && changePct != null && !Number.isNaN(changePct) ? (
                        <span style={{ fontSize: 12, color: changePct >= 0 ? '#ff4d4f' : '#52c41a' }}>
                          {changePct >= 0 ? '+' : ''}{changePct.toFixed(1)}% 较上期
                        </span>
                      ) : null
                    }
                  />
                </Card>
              </Col>
            );
            return card;
          })}
        </Row>
      </Card>

      <Tabs
        activeKey={activeTab}
        onChange={key => {
          setSearchParams(key === 'roi' ? { tab: 'roi' } : {});
        }}
        items={[
          {
            key: 'cost',
            label: '成本结构',
            children: (
              <>
                {renderGlobalMetrics()}
                {renderDomainBreakdown()}
                {/* [Ref: 01_设计 §云产品成本明细索引、D9-16] 索引模块：筛选维度独立于全页时间，仅驱动本区表格与趋势/环比 */}
                <Card title="云产品成本明细 · 筛选（索引）" size="small" style={{ marginTop: 16, marginBottom: 8 }}>
                <div style={{ display: 'flex', flexWrap: 'wrap', gap: 12, alignItems: 'center' }}>
                  <span style={{ fontWeight: 500 }}>时间范围：</span>
                  <Select
                    value={indexPeriod ?? (costTimeRange === 'custom' ? 'custom' : 'page')}
                    style={{ minWidth: 120 }}
                    options={[
                      { label: '同上方', value: 'page' },
                      ...TIME_RANGE_OPTIONS.map(o => ({ label: o.label, value: o.value })),
                    ]}
                    onChange={v => {
                      setSearchParams((prev: URLSearchParams) => {
                        const n = new URLSearchParams(prev);
                        if (v === 'page') {
                          n.delete('index_period');
                          n.delete('index_date_from');
                          n.delete('index_date_to');
                        } else {
                          n.set('index_period', v);
                          if (v !== 'custom') n.delete('index_date_from'), n.delete('index_date_to');
                        }
                        return n;
                      });
                    }}
                  />
                  {effectiveDrilldownPeriod === 'custom' && (
                    <>
                      <DatePicker.RangePicker
                        value={effectiveDrilldownDateRange?.[0] && effectiveDrilldownDateRange?.[1] ? [dayjs(effectiveDrilldownDateRange[0]), dayjs(effectiveDrilldownDateRange[1])] : null}
                        onChange={(dates: [Dayjs | null, Dayjs | null] | null) => {
                          if (dates?.[0] && dates?.[1] && dates[1].diff(dates[0], 'day') <= 180) {
                            const from = dates[0].format('YYYY-MM-DD');
                            const to = dates[1].format('YYYY-MM-DD');
                            setSearchParams((prev: URLSearchParams) => {
                              const n = new URLSearchParams(prev);
                              n.set('index_period', 'custom');
                              n.set('index_date_from', from);
                              n.set('index_date_to', to);
                              return n;
                            });
                          }
                        }}
                      />
                      <span style={{ fontSize: 12, color: '#666' }}>最多 6 个月</span>
                    </>
                  )}
                  <span style={{ fontWeight: 500, marginLeft: 8 }}>环境：</span>
                  <Select
                    value={drilldownEnv}
                    style={{ width: 120 }}
                    options={[
                      { label: '全环境', value: 'all' },
                      { label: 'POC', value: 'POC' },
                      { label: 'FAT', value: 'FAT' },
                      { label: 'UAT', value: 'UAT' },
                      { label: 'PROD', value: 'PROD' },
                    ]}
                    onChange={v => setSearchParams((prev: URLSearchParams) => { const n = new URLSearchParams(prev); n.set('env', v); return n; })}
                  />
                  <span style={{ fontWeight: 500, marginLeft: 8 }}>大类：</span>
                  <Select
                    value={drilldownCategory || 'all'}
                    style={{ width: 120 }}
                    options={[
                      { label: '全部', value: 'all' },
                      { label: '计算资源', value: 'compute' },
                      { label: '存储', value: 'storage' },
                      { label: '网络', value: 'network' },
                      { label: '安全', value: 'security' },
                      { label: '其他', value: 'other' },
                    ]}
                    onChange={v => setSearchParams((prev: URLSearchParams) => { const n = new URLSearchParams(prev); if (v === 'all') n.delete('category'); else n.set('category', v); return n; })}
                  />
                  <span style={{ fontWeight: 500, marginLeft: 8 }}>排序：</span>
                  <Select
                    value={searchParams.get('sort') || 'cost_desc'}
                    style={{ width: 100 }}
                    options={[
                      { label: '成本降序', value: 'cost_desc' },
                      { label: '成本升序', value: 'cost_asc' },
                    ]}
                    onChange={v => setSearchParams((prev: URLSearchParams) => { const n = new URLSearchParams(prev); n.set('sort', v); return n; })}
                  />
                  <span style={{ marginLeft: 16, fontWeight: 500 }}>环比：</span>
                  <Switch
                    checked={drilldownCompare}
                    onChange={on => setSearchParams((prev: URLSearchParams) => {
                      const n = new URLSearchParams(prev);
                      if (on) n.set('compare_trend', '1'); else n.delete('compare_trend');
                      return n;
                    })}
                    checkedChildren="对比上期"
                    unCheckedChildren="关"
                  />
                  <span style={{ marginLeft: 16, fontWeight: 500 }}>展示趋势：</span>
                  <Switch
                    checked={showTrendChart}
                    onChange={on => setSearchParams((prev: URLSearchParams) => {
                      const n = new URLSearchParams(prev);
                      if (on) n.set('show_trend', '1'); else n.delete('show_trend');
                      return n;
                    })}
                    checkedChildren="开"
                    unCheckedChildren="关"
                  />
                </div>
                </Card>
                {/* [Ref: 01_设计 D9-16] 趋势总图默认关闭且收起，由「展示趋势」开关控制 */}
                {!useMockData && showTrendChart && (
                  <Card size="small" title="成本趋势" style={{ marginBottom: 16 }}>
                    {errorCostTrend && (
                      <Alert message={errorCostTrend} type="warning" showIcon style={{ marginBottom: 8 }} />
                    )}
                    {loadingCostTrend ? (
                      <div style={{ height: 220, display: 'flex', alignItems: 'center', justifyContent: 'center' }}>
                        <LoadingOutlined spin /> 加载趋势中...
                      </div>
                    ) : (costTrendData?.length ?? 0) > 0 ? (
                      <div>
                        {drilldownCompare && (!costTrendDataPrev || costTrendDataPrev.length === 0) && (
                          <div style={{ marginBottom: 8, fontSize: 12, color: '#faad14' }}>环比已开，暂无上期数据（展示「—」）</div>
                        )}
                      <div style={{ height: 220 }}>
                        <ResponsiveContainer width="100%" height="100%">
                          <LineChart
                            data={(() => {
                              const cur = costTrendData ?? [];
                              const prev = costTrendDataPrev ?? [];
                              const dateSet = new Set<string>([...cur.map(d => d.date), ...prev.map(d => d.date)]);
                              const sorted = Array.from(dateSet).sort();
                              return sorted.map(date => ({
                                date,
                                本期: cur.find(d => d.date === date)?.total_cost ?? null,
                                上期: drilldownCompare && prev.length ? (prev.find(d => d.date === date)?.total_cost ?? null) : undefined,
                              }));
                            })()}
                            margin={{ top: 5, right: 20, left: 0, bottom: 5 }}
                          >
                            <CartesianGrid strokeDasharray="3 3" />
                            <XAxis dataKey="date" tick={{ fontSize: 11 }} tickFormatter={d => d.slice(5)} />
                            <YAxis tick={{ fontSize: 11 }} tickFormatter={v => (v >= 1000 ? `${(v / 1000).toFixed(1)}k` : String(v))} />
                            <RechartsTooltip
                              formatter={(v: number) => (v != null ? `${CURRENCY_SYMBOL}${Number(v).toLocaleString()}` : '-')}
                              labelFormatter={l => l}
                            />
                            <Line type="monotone" dataKey="本期" stroke="#1677ff" dot={false} name="本期" />
                            {drilldownCompare && costTrendDataPrev?.length ? (
                              <Line type="monotone" dataKey="上期" stroke="#52c41a" dot={false} name="上期" />
                            ) : null}
                            <Legend />
                          </LineChart>
                        </ResponsiveContainer>
                      </div>
                      </div>
                    ) : (
                      <div style={{ height: 100, display: 'flex', alignItems: 'center', justifyContent: 'center', color: '#999' }}>
                        暂无趋势数据
                      </div>
                    )}
                  </Card>
                )}
                <Card
                  id="cloud-product-detail"
                  title="云产品成本明细"
                  style={{
                    marginTop: 0,
                    transition: 'box-shadow 0.3s',
                    boxShadow: highlightCloudProduct ? '0 0 0 2px #1677ff' : undefined,
                  }}
                >
                  {useMockData ? (
                    loadingNamespaceCosts ? (
                      <div style={{ textAlign: 'center', padding: '20px' }}>
                        <LoadingOutlined spin /> 加载中...
                      </div>
                    ) : errorNamespaceCosts ? (
                      <Alert
                        message="加载云产品明细失败"
                        description={errorNamespaceCosts}
                        type="error"
                        showIcon
                      />
                    ) : namespaceCosts?.length ? (
                      <CostTable data={namespaceCosts} onRowClick={handleRowClick} />
                    ) : (
                      <div style={{ textAlign: 'center', padding: '20px' }}>暂无云产品明细数据</div>
                    )
                  ) : (
                    loadingDrilldownGlobal ? (
                      <div style={{ textAlign: 'center', padding: '20px' }}>
                        <LoadingOutlined spin /> 加载中...
                      </div>
                    ) : errorDrilldownGlobal ? (
                      <Alert
                        message="加载云产品明细失败"
                        description={errorDrilldownGlobal}
                        type="error"
                        showIcon
                      />
                    ) : drilldownGlobalProducts?.length ? (
                      <Table<CloudProductDrilldownItem>
                        size="small"
                        rowKey={(r) => r.product_code + (r.category ?? '')}
                        dataSource={drilldownGlobalProducts}
                        columns={[
                          { title: '产品', dataIndex: 'product_name', key: 'product', render: (_: unknown, r: CloudProductDrilldownItem) => r.product_name || r.product_code },
                          { title: `成本 (${CURRENCY_SYMBOL})`, dataIndex: 'cost', key: 'cost', render: (v: number) => CURRENCY_SYMBOL + (v ?? 0).toLocaleString() },
                          { title: '分类', dataIndex: 'category', key: 'category' },
                          ...(drilldownCompare ? [{
                            title: '环比',
                            key: 'change_pct',
                            render: (_: unknown, r: CloudProductDrilldownItem) => {
                              const prev = drilldownGlobalProductsPrev?.find(p => p.product_code === r.product_code);
                              const prevCost = prev?.cost ?? 0;
                              if (prevCost <= 0) return '—';
                              const pct = ((r.cost - prevCost) / prevCost) * 100;
                              return `${pct >= 0 ? '+' : ''}${pct.toFixed(1)}%`;
                            },
                          }] : []),
                          ...(showTrendChart && costTrendData?.length ? [{
                            title: '趋势',
                            key: 'trend',
                            width: 100,
                            render: () => (
                              <ResponsiveContainer width={90} height={28}>
                                <LineChart data={costTrendData ?? []} margin={{ top: 2, right: 2, left: 0, bottom: 2 }}>
                                  <XAxis dataKey="date" hide />
                                  <YAxis hide domain={['auto', 'auto']} />
                                  <Line type="monotone" dataKey="total_cost" stroke="#1677ff" dot={false} isAnimationActive={false} strokeWidth={1.5} />
                                </LineChart>
                              </ResponsiveContainer>
                            ),
                          }] : []),
                        ]}
                        pagination={{ pageSize: 20, showSizeChanger: true, showTotal: (t) => `共 ${t} 条` }}
                      />
                    ) : (
                      <div style={{ textAlign: 'center', padding: '20px' }}>暂无云产品明细数据</div>
                    )
                  )}
                </Card>
              </>
            ),
          },
          {
            key: 'roi',
            label: '成本结构趋势',
            children: <ROITrendSection />,
          },
        ]}
      />

      <Modal
        title={domainDetail ? `${domainDetail.domain} 成本前 4 产品` : undefined}
        open={domainDetail !== null}
        onCancel={() => setDomainDetail(null)}
        footer={
          domainDetail ? (
            <div>
              <a
                href="#cloud-product-detail"
                onClick={(e) => {
                  e.preventDefault();
                  const category = domainDetail ? DOMAIN_TO_CATEGORY[domainDetail.domain] || 'other' : undefined;
                  setSearchParams((prev: URLSearchParams) => {
                    const next = new URLSearchParams(prev);
                    next.set('env', 'all');
                    next.set('period', costTimeRange);
                    if (costTimeRange === 'custom' && costCustomDateRange?.[0] && costCustomDateRange?.[1]) {
                      next.set('date_from', costCustomDateRange[0]);
                      next.set('date_to', costCustomDateRange[1]);
                    }
                    if (category) next.set('category', category);
                    return next;
                  });
                  setDomainDetail(null);
                  setTimeout(() => document.getElementById('cloud-product-detail')?.scrollIntoView({ behavior: 'smooth' }), 0);
                }}
              >
                查看更多 → 云产品成本明细
              </a>
            </div>
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
            <div style={{ marginBottom: 8, fontSize: 12, color: '#666' }}>该大类下成本最高的 4 个云产品（该时间段总成本）：</div>
            {domainDetail.topProducts && domainDetail.topProducts.length > 0 ? (
              <ul style={{ margin: 0, paddingLeft: 20 }}>
                {domainDetail.topProducts.slice(0, 4).map((p, j) => (
                  <li key={j} style={{ marginBottom: 4 }}>{p.product}：{CURRENCY_SYMBOL}{p.cost.toLocaleString()}</li>
                ))}
              </ul>
            ) : (
              <div style={{ color: '#999', fontSize: 12 }}>暂无产品明细</div>
            )}
          </>
        )}
      </Modal>

      <Modal
        title={detailModal === 'bill' ? '成本账单详情' : '效率构成'}
        open={detailModal !== null}
        onCancel={() => setDetailModal(null)}
        footer={null}
        width={640}
      >
        {detailModal === 'bill' && globalCostMetrics && (
          <div>
            <p style={{ color: '#666', marginBottom: 16 }}>
              总账单由计算资源、存储、网络、其它四类领域汇总，分项之和=总账单。可优化空间与效率在无数据支撑时显示「—」。下方按领域列出该账期成本最高的产品（最多 4 个），金额为该领域下该产品的总成本（非单价）。点击「查看详情」可跳转钻取页（账期汇总数据下无命名空间级下钻，仅 Mock 时可下钻）。
            </p>
            {globalCostMetrics.billDetail && (
              <Descriptions column={1} bordered size="small" style={{ marginBottom: 16 }}>
                <Descriptions.Item label="基础计算资源">{CURRENCY_SYMBOL}{globalCostMetrics.billDetail.compute.toLocaleString()}</Descriptions.Item>
                <Descriptions.Item label="存储">{CURRENCY_SYMBOL}{globalCostMetrics.billDetail.storage.toLocaleString()}</Descriptions.Item>
                <Descriptions.Item label="网络">{CURRENCY_SYMBOL}{globalCostMetrics.billDetail.network.toLocaleString()}</Descriptions.Item>
                <Descriptions.Item label="其它云产品">{CURRENCY_SYMBOL}{globalCostMetrics.billDetail.other.toLocaleString()}</Descriptions.Item>
              </Descriptions>
            )}
            <Descriptions column={1} bordered size="small">
              {(globalCostMetrics.domainBreakdown ?? []).map((d, i) => {
                const dimensionAnchor = (d.domain === '计算资源' && 'compute') || (d.domain === '存储' && 'storage') || (d.domain === '网络' && 'network') || null;
                return (
                  <Descriptions.Item key={i} label={d.domain}>
                    <div>
                      <div>
                        {CURRENCY_SYMBOL}{d.cost.toLocaleString()}
                        （可优化空间 {d.optimizableSpace > 0 ? `${CURRENCY_SYMBOL}${d.optimizableSpace.toLocaleString()}` : '—'}，效率 {d.efficiency > 0 ? `${d.efficiency}%` : '—'}）
                      </div>
                      {d.topProducts && d.topProducts.length > 0 && (
                        <div style={{ marginTop: 8 }}>
                          <div style={{ marginBottom: 4, fontSize: 12, color: '#666' }}>成本前 4 产品（该领域该账期总成本，非单价）：</div>
                          <ul style={{ margin: 0, paddingLeft: 18 }}>
                            {(d.topProducts || []).map((p, j) => (
                              <li key={j}>{p.product}：{CURRENCY_SYMBOL}{p.cost.toLocaleString()}</li>
                            ))}
                          </ul>
                          {dimensionAnchor && (
                            <a
                              href={`#/DrilldownPage?dimension=${dimensionAnchor}`}
                              onClick={(e) => { e.preventDefault(); navigate(`/DrilldownPage?dimension=${dimensionAnchor}`); setDetailModal(null); }}
                              style={{ marginTop: 6, display: 'inline-block' }}
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
            <p style={{ color: '#666', marginBottom: 16 }}>
              全局效率 = 汇总使用成本/汇总账单成本×100%。各领域与命名空间效率如下。
            </p>
            <Table
              size="small"
              dataSource={[
                ...(globalCostMetrics.domainBreakdown ?? []).map(d => ({
                  key: `domain-${d.domain}`,
                  name: d.domain,
                  efficiency: d.efficiency,
                  type: '领域',
                })),
                ...(namespaceCosts || []).map(n => ({
                  key: `ns-${n.namespace}`,
                  name: n.namespace,
                  efficiency: n.efficiency,
                  type: '命名空间',
                })),
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
