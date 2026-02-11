import { apiClient } from '@/services/api';
import { CostMetrics, NamespaceCost, DrilldownItem, SLOStatus, ROITrend } from '@/types';

const COST_API_PREFIX = '/v1/cost';

export const costService = {
  // 获取全域成本透视数据
  async getGlobalCostMetrics(): Promise<CostMetrics> {
    try {
      const response = await apiClient.get<CostMetrics>(`${COST_API_PREFIX}/global`);
      return response.data;
    } catch (error) {
      console.error('Failed to fetch global cost metrics:', error);
      throw error;
    }
  },

  // 获取Namespace级别成本数据
  async getNamespaceCosts(): Promise<NamespaceCost[]> {
    try {
      const response = await apiClient.get<NamespaceCost[]>(`${COST_API_PREFIX}/namespaces`);
      return response.data;
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

  // 获取SLO状态
  async getSLOStatus(): Promise<SLOStatus[]> {
    try {
      const response = await apiClient.get<SLOStatus[]>(`${COST_API_PREFIX}/slo`);
      return response.data;
    } catch (error) {
      console.error('Failed to fetch SLO status:', error);
      throw error;
    }
  },

  // 获取ROI趋势数据
  async getROITrends(): Promise<ROITrend[]> {
    try {
      const response = await apiClient.get<ROITrend[]>(`${COST_API_PREFIX}/roi/trends`);
      return response.data;
    } catch (error) {
      console.error('Failed to fetch ROI trends:', error);
      throw error;
    }
  },
};
