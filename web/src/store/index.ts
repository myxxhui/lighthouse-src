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
      useMockData: true,
      selectedDimension: 'compute',
      selectedNamespace: null,
      selectedNode: null,
      selectedWorkload: null,
      selectedPod: null,

      // Actions
      fetchGlobalCostMetrics: async () => {
        const { useMockData, costTimeRange, costCompareMode } = get();
        set({ loadingGlobalMetrics: true, errorGlobalMetrics: null });

        try {
          const data = useMockData
            ? await mockApi.getGlobalCostMetrics({ period: costTimeRange, compareMode: costCompareMode })
            : await costService.getGlobalCostMetrics({ period: costTimeRange, compareMode: costCompareMode });
          set({ globalCostMetrics: data, loadingGlobalMetrics: false });
        } catch (error) {
          const errorMessage = error instanceof Error ? error.message : '获取全局成本指标失败';
          set({ errorGlobalMetrics: errorMessage, loadingGlobalMetrics: false });
        }
      },

      fetchNamespaceCosts: async () => {
        const { useMockData, costTimeRange } = get();
        set({ loadingNamespaceCosts: true, errorNamespaceCosts: null });

        try {
          const data = useMockData
            ? await mockApi.getNamespaceCosts({ period: costTimeRange })
            : await costService.getNamespaceCosts({ period: costTimeRange });
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
