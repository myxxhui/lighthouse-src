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

export interface EnvDrilldownApiItem {
  product_code: string;
  product_name?: string;
  cost: number;
  category: string;
}

const COST_API_PREFIX = '/v1/cost';

export interface CostQueryParams {
  period?: CostTimeRange;
  compareMode?: CostCompareMode;
  /** D8-2/D8-5：日期选择，最多最近 6 个月内 */
  date_from?: string;
  date_to?: string;
}

export const costService = {
  /** D8-8：日期选择请求超时 25s，常规 period 请求用默认超时 */
  DATE_RANGE_REQUEST_TIMEOUT_MS: 25000,

  async getGlobalCostMetrics(params?: CostQueryParams): Promise<CostMetrics> {
    try {
      const query: Record<string, string | undefined> = {};
      if (params?.period != null) query.period = params.period;
      if (params?.compareMode != null) query.compareMode = params.compareMode;
      if (params?.date_from != null) query.date_from = params.date_from;
      if (params?.date_to != null) query.date_to = params.date_to;
      const config: { params: Record<string, string | undefined>; timeout?: number } = { params: query };
      if (params?.date_from != null && params?.date_to != null) {
        config.timeout = costService.DATE_RANGE_REQUEST_TIMEOUT_MS;
      }
      const response = await apiClient.get<GlobalCostApiResponse>(`${COST_API_PREFIX}/global`, config);
      return adaptGlobalCostToCostMetrics(response.data);
    } catch (error) {
      console.error('Failed to fetch global cost metrics:', error);
      throw error;
    }
  },

  // 获取Namespace级别成本数据（通过适配层转换为前端类型）
  // 真实账期数据时后端可能返回 null，避免 .map 报错 [Ref: 04_Phase4/01_成本透视真实数据]
  async getNamespaceCosts(params?: { period?: CostTimeRange }): Promise<NamespaceCost[]> {
    try {
      const query = params?.period ? { period: params.period } : {};
      const response = await apiClient.get<NamespaceCostSummaryApiItem[] | null>(
        `${COST_API_PREFIX}/namespaces`,
        { params: query },
      );
      const data = Array.isArray(response.data) ? response.data : [];
      return adaptNamespacesToNamespaceCosts(data);
    } catch (error) {
      console.error('Failed to fetch namespace costs:', error);
      throw error;
    }
  },

  /** 按环境云产品钻取 [Ref: 01_设计 D9-4] GET /api/v1/cost/drilldown/env/:envId */
  async getEnvDrilldown(
    envId: string,
    params?: { report_type?: string; period_key?: string; category?: string; sort?: string },
  ): Promise<EnvDrilldownApiItem[]> {
    const query: Record<string, string> = {};
    if (params?.report_type) query.report_type = params.report_type;
    if (params?.period_key) query.period_key = params.period_key;
    if (params?.category) query.category = params.category;
    if (params?.sort) query.sort = params.sort;
    const response = await apiClient.get<EnvDrilldownApiItem[]>(
      `${COST_API_PREFIX}/drilldown/env/${encodeURIComponent(envId)}`,
      { params: query },
    );
    return Array.isArray(response.data) ? response.data : [];
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
