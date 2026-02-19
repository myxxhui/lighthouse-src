import { apiClient } from '@/services/api';
import { CostMetrics, NamespaceCost, DrilldownItem, SLOStatus, ROITrend } from '@/types';
import {
  adaptGlobalCostToCostMetrics,
  adaptNamespacesToNamespaceCosts,
  type GlobalCostApiResponse,
  type NamespaceCostSummaryApiItem,
} from '@/adapters/apiAdapter';

const COST_API_PREFIX = '/v1/cost';

export const costService = {
  // 获取全域成本透视数据（通过适配层转换为前端类型）
  async getGlobalCostMetrics(): Promise<CostMetrics> {
    try {
      const response = await apiClient.get<GlobalCostApiResponse>(`${COST_API_PREFIX}/global`);
      return adaptGlobalCostToCostMetrics(response.data);
    } catch (error) {
      console.error('Failed to fetch global cost metrics:', error);
      throw error;
    }
  },

  // 获取Namespace级别成本数据（通过适配层转换为前端类型）
  async getNamespaceCosts(): Promise<NamespaceCost[]> {
    try {
      const response = await apiClient.get<NamespaceCostSummaryApiItem[]>(
        `${COST_API_PREFIX}/namespaces`,
      );
      return adaptNamespacesToNamespaceCosts(response.data);
    } catch (error) {
      console.error('Failed to fetch namespace costs:', error);
      throw error;
    }
  },

  // 获取钻取数据
  async getDrilldownData(
    type: 'namespace' | 'node' | 'workload' | 'pod',
    id: string,
  ): Promise<DrilldownItem> {
    try {
      const response = await apiClient.get<DrilldownItem>(
        `${COST_API_PREFIX}/drilldown/${type}/${id}`,
      );
      return response.data;
    } catch (error) {
      console.error(`Failed to fetch drilldown data for ${type} ${id}:`, error);
      throw error;
    }
  },

  // 获取SLO状态 (GET /api/v1/slo/health)
  async getSLOStatus(): Promise<SLOStatus[]> {
    try {
      const response = await apiClient.get<{ items?: SLOStatus[] } | SLOStatus[]>('/v1/slo/health');
      const data = response.data;
      return Array.isArray(data) ? data : (data?.items ?? []);
    } catch (error) {
      console.error('Failed to fetch SLO status:', error);
      throw error;
    }
  },

  // 获取ROI趋势数据 (GET /api/v1/roi/dashboard)
  async getROITrends(): Promise<ROITrend[]> {
    try {
      const response = await apiClient.get<{ trends?: ROITrend[] } | ROITrend[]>('/v1/roi/dashboard');
      const data = response.data;
      return Array.isArray(data) ? data : (data?.trends ?? []);
    } catch (error) {
      console.error('Failed to fetch ROI trends:', error);
      throw error;
    }
  },
};
