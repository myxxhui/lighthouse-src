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
  type CostTrack,
} from '@/types';
import { costService } from '@/services/costService';
import { mockApi } from '@/services/mockApi';
import { billingCalendarPartsFromNow } from '@/utils/billingCalendar';

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
  costTrendData: Array<{ date: string; total_cost: number; by_product?: Record<string, number> }> | null;
  costTrendDataPrev: Array<{ date: string; total_cost: number; by_product?: Record<string, number> }> | null;
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
  /** envs 非空时仅请求所选环境的成本；projectIds 非空时传 project_ids，后端筛选 Hero/ledger；环境卡与项目卡 ledger 由后端对全量 env 行填充（R15）。[Ref: 03_Phase6/03_前端全域成本透视/01_设计] */
  fetchGlobalCostMetrics: (envs?: string[], track?: CostTrack, projectIds?: number[]) => Promise<void>;
  fetchNamespaceCosts: () => Promise<void>;
  /** [Ref: 01_设计 §云产品成本明细索引 索引区时间范围] override 为索引区独立时间范围；withCompare 为 true 时再拉上期明细填 drilldownGlobalProductsPrev；track 与顶部视角一致 [Ref: 03_Phase6/01_FinOps] */
  fetchDrilldownGlobal: (env?: string, category?: string, sort?: string, override?: { period: CostTimeRange; dateRange: [string, string] | null }, withCompare?: boolean, track?: CostTrack) => Promise<void>;
  fetchDrilldownData: (
    type: string,
    id: string,
    dimension?: ResourceDimension,
  ) => Promise<void>;
  setSelectedDimension: (dimension: ResourceDimension) => void;
  fetchSLOStatus: (scope?: SLOScope) => Promise<void>;
  fetchCostTrend: (params?: { period?: string; date_from?: string; date_to?: string; env?: string; track?: CostTrack }, withCompare?: boolean) => Promise<void>;
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

      fetchGlobalCostMetrics: async (envs?: string[], track?: CostTrack, projectIds?: number[]) => {
        const { useMockData, costTimeRange, costCompareMode, costCustomDateRange } = get();
        // 避免 project_ids/时间切换时用上一帧 global 与新区间混显（项目卡数字短暂不一致）[Ref: 03_Phase6/03_前端全域成本透视]
        set({ loadingGlobalMetrics: true, errorGlobalMetrics: null, globalCostMetrics: undefined });
        const envsParam = envs?.length ? envs.join(',') : undefined;
        const projectIdsParam = projectIds?.length ? projectIds.join(',') : undefined;
        const trackParam = track;
        try {
          let data: CostMetrics;
          const effectivePeriod = costTimeRange === 'custom' ? 'month' : costTimeRange;
          // [Ref: 16_ §三] 统计口径已移除，后端固定返回实际付款（payment）
          if (useMockData) {
            data = await mockApi.getGlobalCostMetrics({
              period: effectivePeriod,
              compareMode: costCompareMode,
              track: trackParam ?? 'technical',
            });
          } else if (costTimeRange === 'custom' && costCustomDateRange != null && costCustomDateRange[0] && costCustomDateRange[1]) {
            data = await costService.getGlobalCostMetrics({
              date_from: costCustomDateRange[0],
              date_to: costCustomDateRange[1],
              envs: envsParam,
              project_ids: projectIdsParam,
              track: trackParam ?? 'technical',
            });
          } else {
            data = await costService.getGlobalCostMetrics({
              period: effectivePeriod,
              compareMode: costCompareMode,
              envs: envsParam,
              project_ids: projectIdsParam,
              track: trackParam ?? 'technical',
            });
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

      fetchDrilldownGlobal: async (env?: string, category?: string, sort?: string, override?: { period: CostTimeRange; dateRange: [string, string] | null }, withCompare?: boolean, track?: CostTrack) => {
        const { useMockData, costTimeRange, costCustomDateRange } = get();
        const trackParam = track ?? 'technical';
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
          const { monthStr, prevMonthStr, quarterKey, prevQuarterKey, yearStr, prevYearStr } = billingCalendarPartsFromNow();
          const reportTypeMap: Record<string, string> = {
            month: 'month', last_month: 'last_month',
            quarter: 'quarter', last_quarter: 'last_quarter',
            this_year: 'this_year', last_year: 'last_year',
          };
          const periodKeyMap: Record<string, string> = {
            month: monthStr, last_month: prevMonthStr,
            quarter: quarterKey, last_quarter: prevQuarterKey,
            this_year: yearStr, last_year: prevYearStr,
          };
          if (period === 'custom' && dateRange?.[0] && dateRange?.[1]) {
            const data = await costService.getDrilldownGlobal({
              date_from: dateRange[0],
              date_to: dateRange[1],
              env: envFilter,
              category: category || undefined,
              sort: sortOrder,
              track: trackParam,
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
              const prevData = await costService.getDrilldownGlobal({ date_from: prevFrom, date_to: prevTo, env: envFilter, category: category || undefined, sort: sortOrder, track: trackParam });
              set(state => ({ ...state, drilldownGlobalProductsPrev: prevData }));
            }
            return;
          }
          const report_type = reportTypeMap[period] ?? 'month';
          const period_key = periodKeyMap[period] ?? monthStr;
          const data = await costService.getDrilldownGlobal({ report_type, period_key, env: envFilter, category: category || undefined, sort: sortOrder, track: trackParam });
          set({ drilldownGlobalProducts: data, loadingDrilldownGlobal: false });
          // [Ref: 用户需求 各时间范围均有对应环比] 上期与本期同源（report_type+period_key），保证产品维度一致、环比可算
          if (withCompare) {
            let prevReportType = report_type;
            let prevPeriodKey: string;
            if (period === 'month') {
              prevPeriodKey = prevMonthStr;
            } else if (period === 'last_month') {
              const [py, pm] = prevMonthStr.split('-').map(Number);
              prevPeriodKey = pm === 1 ? `${py - 1}-12` : `${py}-${String(pm - 1).padStart(2, '0')}`;
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
              prevPeriodKey = prevYearStr;
            } else if (period === 'last_year') {
              prevPeriodKey = String(Number(yearStr) - 2);
            } else {
              prevPeriodKey = prevMonthStr;
            }
            const prevData = await costService.getDrilldownGlobal({
              report_type: prevReportType,
              period_key: prevPeriodKey,
              env: envFilter,
              category: category || undefined,
              sort: sortOrder,
              track: trackParam,
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

      fetchCostTrend: async (params?: { period?: string; date_from?: string; date_to?: string; env?: string; track?: CostTrack }, withCompare?: boolean) => {
        set({ loadingCostTrend: true, errorCostTrend: null, costTrendDataPrev: null });
        const trackParam = params?.track ?? 'technical';
        try {
          const res = await costService.getCostTrend({ ...params, track: trackParam });
          const list = (res?.data ?? []).map(d => ({ date: d.date, total_cost: d.total_cost, by_product: d.by_product }));
          set(state => ({ ...state, costTrendData: list, loadingCostTrend: false }));
          if (withCompare && (params?.period || (params?.date_from && params?.date_to))) {
            let prevFrom: string | undefined;
            let prevTo: string | undefined;
            if (params?.date_from && params?.date_to) {
              // 自定义月范围：前移相同月数作为对比期
              const fromParts = params.date_from.split('-').map(Number);
              const toParts = params.date_to.split('-').map(Number);
              const fromD = new Date(fromParts[0], fromParts[1] - 1, 1);
              const toD = new Date(toParts[0], toParts[1] - 1, 1);
              const months = (toD.getFullYear() - fromD.getFullYear()) * 12 + (toD.getMonth() - fromD.getMonth()) + 1;
              const prevEndD = new Date(fromD); prevEndD.setMonth(prevEndD.getMonth() - 1);
              const prevStartD = new Date(prevEndD); prevStartD.setMonth(prevStartD.getMonth() - months + 1);
              prevFrom = `${prevStartD.getFullYear()}-${String(prevStartD.getMonth()+1).padStart(2,'0')}`;
              prevTo = `${prevEndD.getFullYear()}-${String(prevEndD.getMonth()+1).padStart(2,'0')}`;
            } else {
              const period = params?.period ?? 'month';
              if (period === 'last_month') {
                const d = new Date(); d.setDate(1); d.setMonth(d.getMonth() - 2);
                prevFrom = `${d.getFullYear()}-${String(d.getMonth()+1).padStart(2,'0')}`;
                prevTo = prevFrom;
              } else if (period === 'month' || period === '') {
                const d = new Date(); d.setDate(1); d.setMonth(d.getMonth() - 1);
                prevFrom = `${d.getFullYear()}-${String(d.getMonth()+1).padStart(2,'0')}`;
                prevTo = prevFrom;
              } else if (period === 'last_quarter') {
                const now2 = new Date();
                const cm = now2.getMonth(); const cy = now2.getFullYear();
                let sq: number, sy: number;
                if (cm < 3) { sq = 7; sy = cy - 1; } else if (cm < 6) { sq = 10; sy = cy - 1; }
                else if (cm < 9) { sq = 1; sy = cy; } else { sq = 4; sy = cy; }
                prevFrom = `${sy}-${String(sq).padStart(2,'0')}`;
                const em = sq + 2; const ey2 = em > 12 ? sy + 1 : sy; const em2 = em > 12 ? em - 12 : em;
                prevTo = `${ey2}-${String(em2).padStart(2,'0')}`;
              } else if (period === 'quarter') {
                const now2 = new Date();
                const cq = Math.floor(now2.getMonth() / 3);
                let sq: number, sy: number;
                if (cq === 0) { sq = 10; sy = now2.getFullYear() - 1; }
                else { sq = (cq - 1) * 3 + 1; sy = now2.getFullYear(); }
                prevFrom = `${sy}-${String(sq).padStart(2,'0')}`;
                const em = sq + 2; const ey2 = em > 12 ? sy + 1 : sy; const em2 = em > 12 ? em - 12 : em;
                prevTo = `${ey2}-${String(em2).padStart(2,'0')}`;
              } else if (period === 'last_year') {
                const y = new Date().getFullYear() - 2;
                prevFrom = `${y}-01`; prevTo = `${y}-12`;
              } else if (period === 'this_year') {
                const y = new Date().getFullYear() - 1;
                prevFrom = `${y}-01`; prevTo = `${y}-12`;
              }
            }
            const resPrev = await costService.getCostTrend(
              prevFrom && prevTo
                ? { date_from: prevFrom, date_to: prevTo, env: params?.env, track: trackParam }
                : { period: params?.period, env: params?.env, track: trackParam },
            );
            const listPrev = (resPrev?.data ?? []).map(d => ({ date: d.date, total_cost: d.total_cost, by_product: d.by_product }));
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
