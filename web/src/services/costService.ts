import { apiClient } from '@/services/api';
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
import {
  adaptGlobalCostToCostMetrics,
  adaptNamespacesToNamespaceCosts,
  adaptDrilldownResponse,
  type GlobalCostApiResponse,
  type NamespaceCostSummaryApiItem,
  type DrilldownApiResponse,
} from '@/adapters/apiAdapter';

const COST_API_PREFIX = '/v1/cost';

export interface CostQueryParams {
  period?: CostTimeRange;
  compareMode?: CostCompareMode;
}

export const costService = {
  // 获取全域成本透视数据（通过适配层转换为前端类型）
  async getGlobalCostMetrics(params?: CostQueryParams): Promise<CostMetrics> {
    try {
      const query = params
        ? { period: params.period, compareMode: params.compareMode }
        : {};
      const response = await apiClient.get<GlobalCostApiResponse>(`${COST_API_PREFIX}/global`, {
        params: query,
      });
      return adaptGlobalCostToCostMetrics(response.data);
    } catch (error) {
      console.error('Failed to fetch global cost metrics:', error);
      throw error;
    }
  },

  // 获取Namespace级别成本数据（通过适配层转换为前端类型）
  async getNamespaceCosts(params?: { period?: CostTimeRange }): Promise<NamespaceCost[]> {
    try {
      const query = params?.period ? { period: params.period } : {};
      const response = await apiClient.get<NamespaceCostSummaryApiItem[]>(
        `${COST_API_PREFIX}/namespaces`,
        { params: query },
      );
      return adaptNamespacesToNamespaceCosts(response.data);
    } catch (error) {
      console.error('Failed to fetch namespace costs:', error);
      throw error;
    }
  },

  // 获取钻取数据（dimension=compute|storage|network，未传默认 compute）
  async getDrilldownData(
    type: string,
    id: string,
    dimension: ResourceDimension = 'compute',
  ): Promise<DrilldownItem> {
    try {
      const response = await apiClient.get<DrilldownApiResponse>(
        `${COST_API_PREFIX}/drilldown/${type}/${encodeURIComponent(id)}`,
        { params: { dimension } },
      );
      return adaptDrilldownResponse(response.data);
    } catch (error) {
      console.error(`Failed to fetch drilldown data for ${type} ${id}:`, error);
      throw error;
    }
  },

  // 获取SLO状态 (GET /api/v1/slo/health?scope=global|domain|service|pod)
  async getSLOStatus(scope?: SLOScope): Promise<SLOStatus[]> {
    try {
      const params = scope ? { scope } : {};
      const response = await apiClient.get<{ items?: SLOStatus[] } | SLOStatus[]>('/v1/slo/health', {
        params,
      });
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
