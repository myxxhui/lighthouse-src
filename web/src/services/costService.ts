import { apiClient } from '@/services/api';
import {
  CostMetrics,
  NamespaceCost,
  DrilldownItem,
  SLOStatus,
  SLOScope,
  CostTimeRange,
  CostCompareMode,
  ResourceDimension,
  type CostTrack,
} from '@/types';
import type { ApiError } from '@/types';
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
const FINOPS_API_PREFIX = '/v1/finops';

/** GET /api/v1/finops/sync-jobs/:id [Ref: 03_Phase6/01_FinOps 主动同步] */
export interface FinOpsSyncJobStatus {
  job_id: number;
  status: string;
  phase: string;
  warnings?: string[];
  error_message?: string;
  created_at?: string;
  started_at?: string | null;
  completed_at?: string | null;
  data_version?: number;
  config_snapshot?: string;
  /** 步骤进度（非耗时）；与 GET 契约一致 [Ref: 03_Phase6/01_FinOps 主动同步] */
  progress_current?: number;
  progress_total?: number;
  phase_detail?: string;
}

export interface CostQueryParams {
  period?: CostTimeRange;
  compareMode?: CostCompareMode;
  /** D8-2/D8-5：日期选择，最多最近 6 个月内 */
  date_from?: string;
  date_to?: string;
  /** 环境多选，逗号分隔，如 POC,FAT；不传或 all 表示全环境 [Ref: 用户需求 环境多选] */
  envs?: string;
  /** 默认 finance；请求始终带 track [Ref: 03_Phase6/01_FinOps] */
  track?: CostTrack;
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
      if (params?.envs != null && params.envs !== '') query.envs = params.envs;
      query.track = params?.track ?? 'finance';
      const config: { params: Record<string, string | undefined>; timeout?: number } = { params: query };
      if (params?.date_from != null && params?.date_to != null) {
        config.timeout = costService.DATE_RANGE_REQUEST_TIMEOUT_MS;
      }
      const response = await apiClient.get<GlobalCostApiResponse>(`${COST_API_PREFIX}/global`, config);
      return adaptGlobalCostToCostMetrics(response.data, { requestTrack: params?.track ?? 'finance' });
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

  /** 全环境云产品明细 [Ref: 01_设计 D9-8、D6 索引 env] GET /api/v1/cost/drilldown/global */
  async getDrilldownGlobal(params?: {
    report_type?: string;
    period_key?: string;
    category?: string;
    sort?: string;
    env?: string;
    date_from?: string;
    date_to?: string;
    /** technical=消耗明细；finance=现金/实付明细 [Ref: 03_Phase6/01_FinOps] */
    track?: string;
  }): Promise<EnvDrilldownApiItem[]> {
    const query: Record<string, string> = {};
    if (params?.report_type) query.report_type = params.report_type;
    if (params?.period_key) query.period_key = params.period_key;
    if (params?.category) query.category = params.category;
    if (params?.sort) query.sort = params.sort ?? 'cost_desc';
    if (params?.env) query.env = params.env;
    if (params?.date_from) query.date_from = params.date_from;
    if (params?.date_to) query.date_to = params.date_to;
    query.track = params?.track ?? 'finance';
    const response = await apiClient.get<EnvDrilldownApiItem[]>(
      `${COST_API_PREFIX}/drilldown/global`,
      { params: query },
    );
    return Array.isArray(response.data) ? response.data : [];
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

  /** [Ref: 01_设计 D9-16] 成本结构趋势 GET /api/v1/cost/trend，支持 env 与 track（与全域/钻取双轨一致） */
  async getCostTrend(params?: {
    period?: string;
    date_from?: string;
    date_to?: string;
    /** 按环境过滤趋势数据，'all' 或空值表示全环境 */
    env?: string;
    track?: CostTrack;
  }): Promise<{ data: Array<{ date: string; total_cost: number; by_domain?: Record<string, number>; by_product?: Record<string, number> }> }> {
    const query: Record<string, string> = {};
    if (params?.period) query.period = params.period;
    if (params?.date_from) query.date_from = params.date_from;
    if (params?.date_to) query.date_to = params.date_to;
    if (params?.env && params.env !== 'all') query.env = params.env;
    query.track = params?.track ?? 'finance';
    const response = await apiClient.get<{ data: Array<{ date: string; total_cost?: number; by_domain?: Record<string, number>; by_product?: Record<string, number> }> }>(
      `${COST_API_PREFIX}/trend`,
      { params: query, timeout: 15000 },
    );
    const data = response.data?.data ?? [];
    return {
      data: data.map((d: { date: string; total_cost?: number; by_domain?: Record<string, number>; by_product?: Record<string, number> }) => ({
        date: d.date,
        total_cost: d.total_cost ?? 0,
        by_domain: d.by_domain,
        by_product: d.by_product,
      })),
    };
  },

  /** POST /api/v1/finops/sync-jobs — 异步拉取 BSS/OSS 辅助数据 + 账单 ETL 流水线 [Ref: 03_Phase6/01_FinOps 主动同步] */
  async createFinOpsSyncJob(): Promise<{ job_id: number }> {
    const headers: Record<string, string> = {};
    const k =
      typeof process !== 'undefined' && process.env.FINOPS_SYNC_JOB_KEY
        ? String(process.env.FINOPS_SYNC_JOB_KEY).trim()
        : '';
    if (k) {
      headers['X-FinOps-Sync-Key'] = k;
    }
    const response = await apiClient.post<{ job_id: number }>(
      `${FINOPS_API_PREFIX}/sync-jobs`,
      {},
      { timeout: 120000, headers },
    );
    return response.data;
  },

  /** GET /api/v1/finops/sync-jobs/:id */
  async getFinOpsSyncJob(jobId: number): Promise<FinOpsSyncJobStatus> {
    const response = await apiClient.get<FinOpsSyncJobStatus>(`${FINOPS_API_PREFIX}/sync-jobs/${jobId}`, {
      timeout: 15000,
    });
    return response.data;
  },

  /** GET /api/v1/finops/sync-jobs/active — 无活跃任务时返回 null（刷新页恢复进度） [Ref: 03_Phase6/01_FinOps 主动同步] */
  async getFinOpsSyncJobActive(): Promise<FinOpsSyncJobStatus | null> {
    try {
      const response = await apiClient.get<FinOpsSyncJobStatus>(`${FINOPS_API_PREFIX}/sync-jobs/active`, {
        timeout: 15000,
      });
      return response.data;
    } catch (e: unknown) {
      const ae = e as ApiError;
      if (ae.code === 'FINOPS_SYNC_NO_ACTIVE' || ae.code === '404') {
        return null;
      }
      throw e;
    }
  },
};
