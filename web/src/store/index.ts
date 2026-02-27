import { create } from 'zustand';
import { persist } from 'zustand/middleware';
import {
  CostMetrics,
  NamespaceCost,
  DrilldownItem,
  SLOStatus,
  ROITrend,
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

  // ROI趋势
  roiTrends: ROITrend[] | null;
  loadingROI: boolean;
  errorROI: string | null;

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
  fetchROITrends: () => Promise<void>;
  fetchCostTrend: (params?: { period?: string; date_from?: string; date_to?: string }, withCompare?: boolean) => Promise<void>;
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

      roiTrends: null,
      loadingROI: false,
      errorROI: null,

      costTrendData: null,
      costTrendDataPrev: null,
      loadingCostTrend: false,
      errorCostTrend: null,

      costTimeRange: '30d',
      costCompareMode: 'none',
      costCustomDateRange: null,
      useMockData: false,
      selectedDimension: 'compute',
      selectedNamespace: null,
      selectedNode: null,
      selectedWorkload: null,
      selectedPod: null,

      theme: 'light',

      // Actions
      setTheme: (theme: 'light' | 'dark') => set({ theme }),

      fetchGlobalCostMetrics: async () => {
        const { useMockData, costTimeRange, costCompareMode, costCustomDateRange } = get();
        set({ loadingGlobalMetrics: true, errorGlobalMetrics: null });

        try {
          let data: CostMetrics;
          const effectivePeriod = costTimeRange === 'custom'
            ? '30d'
            : costTimeRange === '7d_range'
              ? '7d'
              : costTimeRange;
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
          // #region agent log
          const pocCost = data.envBreakdown?.find(e => e.environment === 'POC')?.total_cost;
          fetch('http://localhost:7370/ingest/822a34d3-40fb-48c1-8a89-5d12dd62b79d', { method: 'POST', headers: { 'Content-Type': 'application/json', 'X-Debug-Session-Id': 'c39b07' }, body: JSON.stringify({ sessionId: 'c39b07', hypothesisId: 'H2_H4', location: 'store:fetchGlobalCostMetrics', message: 'global cost received', data: { useMockData, hasLastUpdatedAt: !!data.lastUpdatedAt, envBreakdownLen: data.envBreakdown?.length ?? 0, pocTotalCost: pocCost ?? 0 }, timestamp: Date.now() }) }).catch(() => {});
          // #endregion
        } catch (error) {
          // D8-8：日期选择请求超时提示重试或缩小范围
          const isTimeout =
            (error as { code?: string })?.code === 'ECONNABORTED' ||
            (error instanceof Error && error.message?.toLowerCase().includes('timeout'));
          const errorMessage = isTimeout
            ? '请求超时，请重试或缩小日期范围'
            : error instanceof Error
              ? error.message
              : '获取全局成本指标失败';
          set({ errorGlobalMetrics: errorMessage, loadingGlobalMetrics: false });
        }
      },

      fetchNamespaceCosts: async () => {
        const { useMockData, costTimeRange } = get();
        set({ loadingNamespaceCosts: true, errorNamespaceCosts: null });
        const effectivePeriod = costTimeRange === 'custom'
            ? '30d'
            : costTimeRange === '7d_range'
              ? '7d'
              : costTimeRange;

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
          const yesterday = new Date();
          yesterday.setDate(yesterday.getDate() - 1);
          const yesterdayStr = yesterday.toISOString().slice(0, 10);
          const now = new Date();
          const monthStr = `${now.getFullYear()}-${String(now.getMonth() + 1).padStart(2, '0')}`;
          const prevMonth = new Date(now.getFullYear(), now.getMonth() - 1, 1);
          const prevMonthStr = `${prevMonth.getFullYear()}-${String(prevMonth.getMonth() + 1).padStart(2, '0')}`;
          const q = Math.floor(now.getMonth() / 3) + 1;
          const quarterKey = `${now.getFullYear()}-Q${q}`;
          const prevQ = q <= 1 ? 4 : q - 1;
          const prevY = q <= 1 ? now.getFullYear() - 1 : now.getFullYear();
          const prevQuarterKey = `${prevY}-Q${prevQ}`;
          const reportTypeMap: Record<string, string> = {
            '1d': '1d', this_week: 'this_week', '7d_range': '7d', '7d': '7d', last_week: 'last_week',
            '30d': '30d', month: 'month', quarter: 'quarter', '90d': '90d',
            last_month: 'last_month', last_quarter: 'last_quarter', this_year: 'this_year', last_year: 'last_year',
          };
          const periodKeyMap: Record<string, string> = {
            '1d': yesterdayStr, '7d': yesterdayStr, '7d_range': yesterdayStr, '30d': yesterdayStr, '90d': yesterdayStr,
            this_week: yesterdayStr, last_week: yesterdayStr,
            month: monthStr, last_month: prevMonthStr, quarter: quarterKey, last_quarter: prevQuarterKey,
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
          const report_type = reportTypeMap[period] ?? '30d';
          const period_key = periodKeyMap[period] ?? yesterdayStr;
          const data = await costService.getDrilldownGlobal({ report_type, period_key, env: envFilter, category: category || undefined, sort: sortOrder });
          set({ drilldownGlobalProducts: data, loadingDrilldownGlobal: false });
          if (withCompare) {
            const periodLen = period === '7d' || period === '7d_range' ? 7 : period === '90d' ? 90 : 30;
            const end = new Date(yesterdayStr);
            const startPrev = new Date(end);
            startPrev.setDate(startPrev.getDate() - periodLen * 2 + 1);
            const prevFrom = startPrev.toISOString().slice(0, 10);
            const prevTo = new Date(end.getTime() - periodLen * 86400000).toISOString().slice(0, 10);
            const prevData = await costService.getDrilldownGlobal({ date_from: prevFrom, date_to: prevTo, env: envFilter, category: category || undefined, sort: sortOrder });
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

      fetchROITrends: async () => {
        const { useMockData } = get();
        set({ loadingROI: true, errorROI: null });

        try {
          const data = useMockData
            ? await mockApi.getROITrends()
            : await costService.getROITrends();
          set({ roiTrends: data, loadingROI: false });
        } catch (error) {
          const errorMessage = error instanceof Error ? error.message : '获取ROI趋势失败';
          set({ errorROI: errorMessage, loadingROI: false });
        }
      },

      fetchCostTrend: async (params?: { period?: string; date_from?: string; date_to?: string }, withCompare?: boolean) => {
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
            const resPrev = await costService.getCostTrend(prevFrom && prevTo ? { date_from: prevFrom, date_to: prevTo } : { period: params?.period });
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
          errorROI: null,
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
