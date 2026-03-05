import { create } from 'zustand';
import { persist } from 'zustand/middleware';
import {
  CostMetrics,
  NamespaceCost,
  DrilldownItem,
  SLOStatus,
  SLOScope,
  CostTimeRange,
  CostCompareMode,
  ResourceDimension,
  CloudProductDrilldownItem,
} from '@/types';
import { costService } from '@/services/costService';
import { mockApi } from '@/services/mockApi';

interface AppState {
  // 全局成本指标
  globalCostMetrics: CostMetrics | null;
  loadingGlobalMetrics: boolean;
  errorGlobalMetrics: string | null;

  // Namespace成本数据（Mock 或 L1 钻取）
  namespaceCosts: NamespaceCost[] | null;
  loadingNamespaceCosts: boolean;
  errorNamespaceCosts: string | null;

  // 全环境云产品明细 [Ref: 01_设计 D9-8 GET /api/v1/cost/drilldown/global]
  drilldownGlobalProducts: CloudProductDrilldownItem[] | null;
  /** 环比：上期云产品明细，用于表格环比列 */
  drilldownGlobalProductsPrev: CloudProductDrilldownItem[] | null;
  loadingDrilldownGlobal: boolean;
  errorDrilldownGlobal: string | null;

  // 钻取数据
  currentDrilldownItem: DrilldownItem | null;
  drilldownPath: string[];
  loadingDrilldown: boolean;
  errorDrilldown: string | null;

  // SLO状态
  sloStatus: SLOStatus[] | null;
  loadingSLO: boolean;
  errorSLO: string | null;

  /** [Ref: 01_设计 D9-16] 成本结构趋势（云产品明细索引区趋势图） */
  costTrendData: Array<{ date: string; total_cost: number }> | null;
  costTrendDataPrev: Array<{ date: string; total_cost: number }> | null; // 环比：上一周期
  loadingCostTrend: boolean;
  errorCostTrend: string | null;

  // 成本透视时间与对比
  costTimeRange: CostTimeRange;
  costCompareMode: CostCompareMode;
  /** D8-5：自定义日期范围 [date_from, date_to] YYYY-MM-DD，最多 6 个月内；非空时请求使用 date_from/date_to */
  costCustomDateRange: [string, string] | null;

  // 应用状态
  useMockData: boolean;
  /** 当前钻取资源维度：算力 / 存储 / 网络 */
  selectedDimension: ResourceDimension;
  selectedNamespace: string | null;
  selectedNode: string | null;
  selectedWorkload: string | null;
  selectedPod: string | null;

  /** [Ref: 01_实践 深色主题必选] 主题：light | dark */
  theme: 'light' | 'dark';

  // Actions
  setTheme: (theme: 'light' | 'dark') => void;
  fetchGlobalCostMetrics: () => Promise<void>;
  fetchNamespaceCosts: () => Promise<void>;
  /** [Ref: 01_设计 §云产品成本明细索引 索引区时间范围] override 为索引区独立时间范围；withCompare 为 true 时再拉上期明细填 drilldownGlobalProductsPrev */
  fetchDrilldownGlobal: (env?: string, category?: string, sort?: string, override?: { period: CostTimeRange; dateRange: [string, string] | null }, withCompare?: boolean) => Promise<void>;
  fetchDrilldownData: (
    type: string,
    id: string,
    dimension?: ResourceDimension,
  ) => Promise<void>;
  setSelectedDimension: (dimension: ResourceDimension) => void;
  fetchSLOStatus: (scope?: SLOScope) => Promise<void>;
  fetchCostTrend: (params?: { period?: string; date_from?: string; date_to?: string; env?: string }, withCompare?: boolean) => Promise<void>;
  setCostTimeRange: (range: CostTimeRange) => void;
  setCostCompareMode: (mode: CostCompareMode) => void;
  setCostCustomDateRange: (range: [string, string] | null) => void;
  setUseMockData: (useMock: boolean) => void;
  setSelectedNamespace: (namespace: string | null) => void;
  setSelectedNode: (node: string | null) => void;
  setSelectedWorkload: (workload: string | null) => void;
  setSelectedPod: (pod: string | null) => void;
  clearDrilldownPath: () => void;
  resetErrors: () => void;
}

export const useAppStore = create<AppState>()(
  persist(
    (set, get) => ({
      // 初始状态
      globalCostMetrics: null,
      loadingGlobalMetrics: false,
      errorGlobalMetrics: null,

      namespaceCosts: null,
      loadingNamespaceCosts: false,
      errorNamespaceCosts: null,

      drilldownGlobalProducts: null,
      drilldownGlobalProductsPrev: null,
      loadingDrilldownGlobal: false,
      errorDrilldownGlobal: null,

      currentDrilldownItem: null,
      drilldownPath: [],
      loadingDrilldown: false,
      errorDrilldown: null,

      sloStatus: null,
      loadingSLO: false,
      errorSLO: null,

      costTrendData: null,
      costTrendDataPrev: null,
      loadingCostTrend: false,
      errorCostTrend: null,

      costTimeRange: 'month',
      costCompareMode: 'none',
      costCustomDateRange: null,
      useMockData: false,
      selectedDimension: 'compute',
      selectedNamespace: null,
      selectedNode: null,
      selectedWorkload: null,
      selectedPod: null,

      theme: 'dark',

      // Actions
      setTheme: (theme: 'light' | 'dark') => set({ theme }),

      fetchGlobalCostMetrics: async () => {
        const { useMockData, costTimeRange, costCompareMode, costCustomDateRange } = get();
        set({ loadingGlobalMetrics: true, errorGlobalMetrics: null });

        try {
          let data: CostMetrics;
          const effectivePeriod = costTimeRange === 'custom' ? 'month' : costTimeRange;
          // [Ref: 16_ §三] 统计口径已移除，后端固定返回实际付款（payment）
          if (useMockData) {
            data = await mockApi.getGlobalCostMetrics({ period: effectivePeriod, compareMode: costCompareMode });
          } else if (costTimeRange === 'custom' && costCustomDateRange != null && costCustomDateRange[0] && costCustomDateRange[1]) {
            data = await costService.getGlobalCostMetrics({
              date_from: costCustomDateRange[0],
              date_to: costCustomDateRange[1],
            });
          } else {
            data = await costService.getGlobalCostMetrics({ period: effectivePeriod, compareMode: costCompareMode });
          }
          set({ globalCostMetrics: data, loadingGlobalMetrics: false });
        } catch (error) {
          const err = error as { message?: string; code?: string };
          const isTimeout =
            err?.code === 'ECONNABORTED' ||
            (typeof err?.message === 'string' && err.message.toLowerCase().includes('timeout'));
          const is502 = err?.code === '502';
          const is503 = err?.code === '503';
          // 502/503/超时：视为「数据加载中/后端可能正忙」，用友好提示
          const likelyBackendBusy = is502 || is503 || isTimeout;
          let errorMessage: string;
          if (likelyBackendBusy) {
            errorMessage = '数据正在加载或聚合中，请稍候 1～3 分钟再试。';
          } else {
            // 其余视为后端真实错误，保留错误信息
            const raw = err?.message || (error instanceof Error ? error.message : null) || '未知错误';
            errorMessage = raw.includes('后端') ? raw : `后端错误：${raw}`;
          }
          set({ errorGlobalMetrics: errorMessage, loadingGlobalMetrics: false });
        }
      },

      fetchNamespaceCosts: async () => {
        const { useMockData, costTimeRange } = get();
        set({ loadingNamespaceCosts: true, errorNamespaceCosts: null });
        const effectivePeriod = costTimeRange === 'custom' ? 'month' : costTimeRange;

        try {
          const data = useMockData
            ? await mockApi.getNamespaceCosts({ period: effectivePeriod })
            : await costService.getNamespaceCosts({ period: effectivePeriod });
          set({ namespaceCosts: data, loadingNamespaceCosts: false });
        } catch (error) {
          const errorMessage = error instanceof Error ? error.message : '获取命名空间成本失败';
          set({ errorNamespaceCosts: errorMessage, loadingNamespaceCosts: false });
        }
      },

      fetchDrilldownGlobal: async (env?: string, category?: string, sort?: string, override?: { period: CostTimeRange; dateRange: [string, string] | null }, withCompare?: boolean) => {
        const { useMockData, costTimeRange, costCustomDateRange } = get();
        const period = override?.period ?? costTimeRange;
        const dateRange = override?.dateRange ?? costCustomDateRange;
        const envFilter = env ?? 'all';
        const sortOrder = sort ?? 'cost_desc';
        set({ loadingDrilldownGlobal: true, errorDrilldownGlobal: null, drilldownGlobalProductsPrev: null });
        try {
          if (useMockData) {
            set({ drilldownGlobalProducts: [], loadingDrilldownGlobal: false });
            return;
          }
          // 与后端 ETL (time.Now().UTC()) 对齐，统一用 UTC 计算 period_key
          const nowUTC = new Date();
          const utcYear = nowUTC.getUTCFullYear();
          const utcMonth = nowUTC.getUTCMonth(); // 0-based
          const utcDate = nowUTC.getUTCDate();
          const now = new Date(Date.UTC(utcYear, utcMonth, utcDate));
          const yesterday = new Date(Date.UTC(utcYear, utcMonth, utcDate - 1));
          const yesterdayStr = yesterday.toISOString().slice(0, 10);
          const monthStr = `${utcYear}-${String(utcMonth + 1).padStart(2, '0')}`;
          const prevMonth = new Date(Date.UTC(utcYear, utcMonth - 1, 1));
          const prevMonthStr = `${prevMonth.getUTCFullYear()}-${String(prevMonth.getUTCMonth() + 1).padStart(2, '0')}`;
          const q = Math.floor(utcMonth / 3) + 1;
          const quarterKey = `${utcYear}-Q${q}`;
          const prevQ = q <= 1 ? 4 : q - 1;
          const prevY = q <= 1 ? utcYear - 1 : utcYear;
          const prevQuarterKey = `${prevY}-Q${prevQ}`;
          // [Ref: 01_设计 report_type 与 period_key] this_week 后端期望 period_key=YYYY-Www（ISO 周），last_week 为上周日日期
          const getISOWeekKey = (d: Date) => {
            const dt = new Date(Date.UTC(d.getFullYear(), d.getMonth(), d.getDate()));
            const dayNum = dt.getUTCDay() || 7;
            dt.setUTCDate(dt.getUTCDate() + 4 - dayNum);
            const year = dt.getUTCFullYear();
            const start = new Date(Date.UTC(year, 0, 1));
            const week = Math.ceil((((dt.getTime() - start.getTime()) / 86400000) + 1) / 7);
            return `${year}-W${String(week).padStart(2, '0')}`;
          };
          const getLastWeekEndKey = (d: Date) => {
            const wd = d.getUTCDay();
            const sunOffset = wd === 0 ? 7 : wd;
            const lastSunday = new Date(d.getTime());
            lastSunday.setUTCDate(d.getUTCDate() - sunOffset);
            return lastSunday.toISOString().slice(0, 10);
          };
          const reportTypeMap: Record<string, string> = {
            '1d': '1d', this_week: 'this_week', last_week: 'last_week',
            month: 'month', last_month: 'last_month',
            quarter: 'quarter', last_quarter: 'last_quarter',
            this_year: 'this_year', last_year: 'last_year',
          };
          const periodKeyMap: Record<string, string> = {
            '1d': yesterdayStr,
            this_week: getISOWeekKey(now), last_week: getLastWeekEndKey(now),
            month: monthStr, last_month: prevMonthStr,
            quarter: quarterKey, last_quarter: prevQuarterKey,
            this_year: String(now.getFullYear()), last_year: String(now.getFullYear() - 1),
          };
          if (period === 'custom' && dateRange?.[0] && dateRange?.[1]) {
            const data = await costService.getDrilldownGlobal({
              date_from: dateRange[0],
              date_to: dateRange[1],
              env: envFilter,
              category: category || undefined,
              sort: sortOrder,
            });
            set({ drilldownGlobalProducts: data, loadingDrilldownGlobal: false });
            if (withCompare) {
              const from = new Date(dateRange[0]);
              const to = new Date(dateRange[1]);
              const days = Math.round((to.getTime() - from.getTime()) / 86400000) + 1;
              const prevEnd = new Date(from);
              prevEnd.setDate(prevEnd.getDate() - 1);
              const prevStart = new Date(prevEnd);
              prevStart.setDate(prevStart.getDate() - days + 1);
              const prevFrom = prevStart.toISOString().slice(0, 10);
              const prevTo = prevEnd.toISOString().slice(0, 10);
              const prevData = await costService.getDrilldownGlobal({ date_from: prevFrom, date_to: prevTo, env: envFilter, category: category || undefined, sort: sortOrder });
              set(state => ({ ...state, drilldownGlobalProductsPrev: prevData }));
            }
            return;
          }
          const report_type = reportTypeMap[period] ?? 'month';
          const period_key = periodKeyMap[period] ?? monthStr;
          const data = await costService.getDrilldownGlobal({ report_type, period_key, env: envFilter, category: category || undefined, sort: sortOrder });
          set({ drilldownGlobalProducts: data, loadingDrilldownGlobal: false });
          // [Ref: 用户需求 各时间范围均有对应环比] 上期与本期同源（report_type+period_key），保证产品维度一致、环比可算
          if (withCompare) {
            let prevReportType = report_type;
            let prevPeriodKey: string;
            if (period === 'this_week') {
              const prevWeekKey = getISOWeekKey(new Date(now.getTime() - 7 * 86400000));
              prevPeriodKey = prevWeekKey;
            } else if (period === 'last_week') {
              const prevLastWeekEnd = new Date(getLastWeekEndKey(now));
              prevLastWeekEnd.setDate(prevLastWeekEnd.getDate() - 7);
              prevPeriodKey = prevLastWeekEnd.toISOString().slice(0, 10);
            } else if (period === '1d') {
              const dayBefore = new Date(yesterdayStr);
              dayBefore.setDate(dayBefore.getDate() - 1);
              prevPeriodKey = dayBefore.toISOString().slice(0, 10);
            } else if (period === 'month') {
              prevPeriodKey = prevMonthStr;
            } else if (period === 'last_month') {
              const prevPrevMonth = new Date(prevMonth.getFullYear(), prevMonth.getMonth() - 1, 1);
              prevPeriodKey = `${prevPrevMonth.getFullYear()}-${String(prevPrevMonth.getMonth() + 1).padStart(2, '0')}`;
            } else if (period === 'quarter') {
              // 上季度在 ETL 中以 report_type='last_quarter' 存储，不保留历史 'quarter' 记录
              prevReportType = 'last_quarter';
              prevPeriodKey = prevQuarterKey;
            } else if (period === 'last_quarter') {
              const [pqY, pqN] = prevQuarterKey.split('-Q').map((s, i) => (i === 0 ? parseInt(s, 10) : parseInt(s, 10)));
              const prevPrevQ = pqN <= 1 ? 4 : pqN - 1;
              const prevPrevY = pqN <= 1 ? pqY - 1 : pqY;
              prevPeriodKey = `${prevPrevY}-Q${prevPrevQ}`;
            } else if (period === 'this_year') {
              prevReportType = 'last_year';
              prevPeriodKey = String(now.getFullYear() - 1);
            } else if (period === 'last_year') {
              prevPeriodKey = String(now.getFullYear() - 2);
            } else {
              prevPeriodKey = prevMonthStr;
            }
            const prevData = await costService.getDrilldownGlobal({
              report_type: prevReportType,
              period_key: prevPeriodKey,
              env: envFilter,
              category: category || undefined,
              sort: sortOrder,
            });
            set(state => ({ ...state, drilldownGlobalProductsPrev: prevData }));
          }
        } catch (error) {
          const msg = error instanceof Error ? error.message : '获取云产品明细失败';
          set({ errorDrilldownGlobal: msg, loadingDrilldownGlobal: false });
        }
      },

      fetchDrilldownData: async (type, id, dimension) => {
        const { useMockData, drilldownPath, selectedDimension } = get();
        const dim = dimension ?? selectedDimension;
        set({ loadingDrilldown: true, errorDrilldown: null });

        try {
          const data = useMockData
            ? await mockApi.getDrilldownData(type, id, dim)
            : await costService.getDrilldownData(type, id, dim);

          const newPath = [...drilldownPath, `${type}:${id}`];
          set({
            currentDrilldownItem: data,
            drilldownPath: newPath,
            loadingDrilldown: false,
          });

          // 更新选中状态（算力维度）
          if (type === 'namespace') {
            set({
              selectedNamespace: id,
              selectedNode: null,
              selectedWorkload: null,
              selectedPod: null,
            });
          } else if (type === 'node') {
            set({ selectedNode: id, selectedWorkload: null, selectedPod: null });
          } else if (type === 'workload') {
            set({ selectedWorkload: id, selectedPod: null });
          } else if (type === 'pod') {
            set({ selectedPod: id });
          }
        } catch (error) {
          const errorMessage = error instanceof Error ? error.message : '获取钻取数据失败';
          set({ errorDrilldown: errorMessage, loadingDrilldown: false });
        }
      },

      setSelectedDimension: dimension => {
        set({ selectedDimension: dimension });
      },

      fetchSLOStatus: async (scope?: SLOScope) => {
        const { useMockData } = get();
        set({ loadingSLO: true, errorSLO: null });

        try {
          const data = useMockData
            ? await mockApi.getSLOStatus(scope)
            : await costService.getSLOStatus(scope);
          set({ sloStatus: data, loadingSLO: false });
        } catch (error) {
          const errorMessage = error instanceof Error ? error.message : '获取SLO状态失败';
          set({ errorSLO: errorMessage, loadingSLO: false });
        }
      },

      fetchCostTrend: async (params?: { period?: string; date_from?: string; date_to?: string; env?: string }, withCompare?: boolean) => {
        set({ loadingCostTrend: true, errorCostTrend: null, costTrendDataPrev: null });
        try {
          const res = await costService.getCostTrend(params);
          const list = (res?.data ?? []).map(d => ({ date: d.date, total_cost: d.total_cost }));
          set(state => ({ ...state, costTrendData: list, loadingCostTrend: false }));
          if (withCompare && (params?.period || (params?.date_from && params?.date_to))) {
            let prevFrom: string | undefined;
            let prevTo: string | undefined;
            if (params?.date_from && params?.date_to) {
              const from = new Date(params.date_from);
              const to = new Date(params.date_to);
              const days = Math.round((to.getTime() - from.getTime()) / 86400000) + 1;
              const prevEnd = new Date(from);
              prevEnd.setDate(prevEnd.getDate() - 1);
              const prevStart = new Date(prevEnd);
              prevStart.setDate(prevStart.getDate() - days + 1);
              prevFrom = prevStart.toISOString().slice(0, 10);
              prevTo = prevEnd.toISOString().slice(0, 10);
            } else {
              const period = params?.period ?? '30d';
              const len = period === '7d' || period === '7d_range' ? 7 : period === '90d' ? 90 : 30;
              const end = new Date();
              end.setDate(end.getDate() - 1);
              const startPrev = new Date(end);
              startPrev.setDate(startPrev.getDate() - len * 2 + 1);
              prevFrom = startPrev.toISOString().slice(0, 10);
              prevTo = new Date(end.getTime() - len * 86400000).toISOString().slice(0, 10);
            }
            const resPrev = await costService.getCostTrend(prevFrom && prevTo ? { date_from: prevFrom, date_to: prevTo, env: params?.env } : { period: params?.period, env: params?.env });
            const listPrev = (resPrev?.data ?? []).map(d => ({ date: d.date, total_cost: d.total_cost }));
            set(state => ({ ...state, costTrendDataPrev: listPrev }));
          }
        } catch (error) {
          const msg = error instanceof Error ? error.message : '获取成本趋势失败';
          set({ errorCostTrend: msg, loadingCostTrend: false });
        }
      },

      setCostTimeRange: range => {
        set({ costTimeRange: range });
      },

      setCostCompareMode: mode => {
        set({ costCompareMode: mode });
      },

      setCostCustomDateRange: range => {
        set({ costCustomDateRange: range });
      },

      setUseMockData: useMock => {
        set({ useMockData: useMock });
      },

      setSelectedNamespace: namespace => {
        set({ selectedNamespace: namespace });
      },

      setSelectedNode: node => {
        set({ selectedNode: node });
      },

      setSelectedWorkload: workload => {
        set({ selectedWorkload: workload });
      },

      setSelectedPod: pod => {
        set({ selectedPod: pod });
      },

      clearDrilldownPath: () => {
        set({
          drilldownPath: [],
          currentDrilldownItem: null,
          selectedNamespace: null,
          selectedNode: null,
          selectedWorkload: null,
          selectedPod: null,
        });
      },

      resetErrors: () => {
        set({
          errorGlobalMetrics: null,
          errorNamespaceCosts: null,
          errorDrilldownGlobal: null,
          errorDrilldown: null,
          errorSLO: null,
          errorCostTrend: null,
        });
      },
    }),
    {
      name: 'lighthouse-storage',
      partialize: state => ({
        costTimeRange: state.costTimeRange,
        costCompareMode: state.costCompareMode,
        useMockData: state.useMockData,
        selectedDimension: state.selectedDimension,
        selectedNamespace: state.selectedNamespace,
        selectedNode: state.selectedNode,
        selectedWorkload: state.selectedWorkload,
        selectedPod: state.selectedPod,
        theme: state.theme,
      }),
    },
  ),
);
