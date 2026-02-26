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
} from '@/types';
import { costService } from '@/services/costService';
import { mockApi } from '@/services/mockApi';

interface AppState {
  // 全局成本指标
  globalCostMetrics: CostMetrics | null;
  loadingGlobalMetrics: boolean;
  errorGlobalMetrics: string | null;

  // Namespace成本数据
  namespaceCosts: NamespaceCost[] | null;
  loadingNamespaceCosts: boolean;
  errorNamespaceCosts: string | null;

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

  // Actions
  fetchGlobalCostMetrics: () => Promise<void>;
  fetchNamespaceCosts: () => Promise<void>;
  fetchDrilldownData: (
    type: string,
    id: string,
    dimension?: ResourceDimension,
  ) => Promise<void>;
  setSelectedDimension: (dimension: ResourceDimension) => void;
  fetchSLOStatus: (scope?: SLOScope) => Promise<void>;
  fetchROITrends: () => Promise<void>;
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

      costTimeRange: '30d',
      costCompareMode: 'none',
      costCustomDateRange: null,
      useMockData: false,
      selectedDimension: 'compute',
      selectedNamespace: null,
      selectedNode: null,
      selectedWorkload: null,
      selectedPod: null,

      // Actions
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
      }),
    },
  ),
);
