import { create } from 'zustand';
import { persist } from 'zustand/middleware';
import { CostMetrics, NamespaceCost, DrilldownItem, SLOStatus, ROITrend } from '@/types';
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

  // 应用状态
  useMockData: boolean;
  selectedNamespace: string | null;
  selectedNode: string | null;
  selectedWorkload: string | null;
  selectedPod: string | null;

  // Actions
  fetchGlobalCostMetrics: () => Promise<void>;
  fetchNamespaceCosts: () => Promise<void>;
  fetchDrilldownData: (
    type: 'namespace' | 'node' | 'workload' | 'pod',
    id: string,
  ) => Promise<void>;
  fetchSLOStatus: () => Promise<void>;
  fetchROITrends: () => Promise<void>;
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

      useMockData: true,
      selectedNamespace: null,
      selectedNode: null,
      selectedWorkload: null,
      selectedPod: null,

      // Actions
      fetchGlobalCostMetrics: async () => {
        const { useMockData } = get();
        set({ loadingGlobalMetrics: true, errorGlobalMetrics: null });

        try {
          const data = useMockData
            ? await mockApi.getGlobalCostMetrics()
            : await costService.getGlobalCostMetrics();
          set({ globalCostMetrics: data, loadingGlobalMetrics: false });
        } catch (error) {
          const errorMessage = error instanceof Error ? error.message : '获取全局成本指标失败';
          set({ errorGlobalMetrics: errorMessage, loadingGlobalMetrics: false });
        }
      },

      fetchNamespaceCosts: async () => {
        const { useMockData } = get();
        set({ loadingNamespaceCosts: true, errorNamespaceCosts: null });

        try {
          const data = useMockData
            ? await mockApi.getNamespaceCosts()
            : await costService.getNamespaceCosts();
          set({ namespaceCosts: data, loadingNamespaceCosts: false });
        } catch (error) {
          const errorMessage = error instanceof Error ? error.message : '获取命名空间成本失败';
          set({ errorNamespaceCosts: errorMessage, loadingNamespaceCosts: false });
        }
      },

      fetchDrilldownData: async (type, id) => {
        const { useMockData, drilldownPath } = get();
        set({ loadingDrilldown: true, errorDrilldown: null });

        try {
          const data = useMockData
            ? await mockApi.getDrilldownData(type, id)
            : await costService.getDrilldownData(type, id);

          const newPath = [...drilldownPath, `${type}:${id}`];
          set({
            currentDrilldownItem: data,
            drilldownPath: newPath,
            loadingDrilldown: false,
          });

          // 更新选中状态
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

      fetchSLOStatus: async () => {
        const { useMockData } = get();
        set({ loadingSLO: true, errorSLO: null });

        try {
          const data = useMockData
            ? await mockApi.getSLOStatus()
            : await costService.getSLOStatus();
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
        useMockData: state.useMockData,
        selectedNamespace: state.selectedNamespace,
        selectedNode: state.selectedNode,
        selectedWorkload: state.selectedWorkload,
        selectedPod: state.selectedPod,
      }),
    },
  ),
);
