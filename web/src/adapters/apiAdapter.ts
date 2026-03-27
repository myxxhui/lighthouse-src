import {
  CostMetrics,
  NamespaceCost,
  DomainBreakdown,
  DrilldownItem,
  DrilldownCostBreakdown,
  EnvBreakdownItem,
  type CostTrack,
  type FinOpsLedger,
  type FinOpsReconciliation,
} from '@/types';

/**
 * 后端 API 响应类型（与 backend dto 一致）
 */
export interface GlobalCostApiResponse {
  total_cost: number;
  total_optimizable?: number;
  global_efficiency?: number;
  domain_breakdown?: DomainBreakdownApiItem[];
  env_breakdown?: EnvBreakdownApiItem[];
  namespaces: NamespaceCostSummaryApiItem[];
  timestamp: string;
  metadata?: {
    last_updated_at?: string;
    data_status?: string;
    bill_data_status?: string;
    display_note?: string;
    /** 后端在合法 track 请求下返回 [Ref: 03_Phase6/01_FinOps双轨语义与全域成本契约_设计 §API、track 与 UX] */
    effective_track?: 'technical' | 'finance';
    /** 五维并列快照与守恒式说明 [Ref: 03_Phase6/01_FinOps双轨语义与全域成本契约_设计 §五维并列快照与 UX] */
    ledger_snapshot_note?: string;
    /** 与 FINOPS_CG_SOURCE 一致；多环境混用为 mixed [Ref: 03_Phase6/01_FinOps] */
    finops_cg_source?: 'oss' | 'api' | 'mixed';
    /** 按环境名实际 C/G 源（与 cost_env_account_config.environment 一致） [Ref: 03_Phase6/01_FinOps] */
    finops_cg_source_by_env?: Partial<Record<string, 'oss' | 'api'>>;
  };
  /** FinOps 五维 [Ref: 03_Phase6/01_FinOps双轨语义与全域成本契约_设计] */
  ledger?: Partial<Record<'C' | 'G' | 'P' | 'U' | 'B', number>>;
  reconciliation?: FinOpsReconciliation;
}

export interface EnvBreakdownApiItem {
  environment: string;
  account_id: string;
  account_display_name: string;
  total_cost: number;
  previous_period_cost?: number;
  change_pct?: number;
  ledger_g?: number;
  ledger_p?: number;
  ledger_u?: number;
  ledger_b?: number;
}

export interface DomainBreakdownApiItem {
  domain: string;
  cost: number;
  optimizable_space: number;
  efficiency: number;
  top_products?: { product: string; cost: number }[];
}

export interface NamespaceCostSummaryApiItem {
  name: string;
  cost: number;
  grade?: string;
  pod_count?: number;
  node_count?: number;
}

export interface AdaptGlobalCostOptions {
  /** 仅当 URL 含 track= 时传入 [Ref: 03_Phase6/01_FinOps双轨语义与全域成本契约_设计] */
  requestTrack?: CostTrack;
}

/**
 * GlobalCostApiResponse -> CostMetrics
 */
// [Ref: 03_Phase6/01_FinOps双轨语义与全域成本契约_设计 §数据流与契约（前端依赖）]
export function adaptGlobalCostToCostMetrics(
  res: GlobalCostApiResponse,
  opts?: AdaptGlobalCostOptions,
): CostMetrics {
  // 真实数据时后端可能返回 0 表示无可优化/效率数据，不臆造
  const totalOptimizable =
    res.total_optimizable !== undefined && res.total_optimizable !== null
      ? res.total_optimizable
      : res.total_cost != null
        ? res.total_cost * 0.3
        : 0;
  const globalEfficiency =
    res.global_efficiency !== undefined && res.global_efficiency !== null
      ? res.global_efficiency
      : 70;
  const totalCost = res.total_cost || 1;
  const domainBreakdown: DomainBreakdown[] =
    res.domain_breakdown?.map((d) => ({
      domain: d.domain,
      cost: d.cost,
      optimizableSpace: d.optimizable_space,
      efficiency: d.efficiency,
      topProducts: d.top_products,
    })) ??
    (res.namespaces ?? []).map((n) => ({
      domain: n.name,
      cost: n.cost,
      optimizableSpace: totalOptimizable * (n.cost / totalCost) || 0,
      efficiency: gradeToEfficiency(n.grade ?? 'Healthy'),
    }));

  const lastUpdatedAt =
    res.metadata?.last_updated_at != null ? res.metadata.last_updated_at : undefined;
  const billDataStatus = res.metadata?.bill_data_status ?? res.metadata?.data_status;
  const displayNote = res.metadata?.display_note ?? undefined;
  const ledgerSnapshotNote = res.metadata?.ledger_snapshot_note ?? undefined;
  const envBreakdown: EnvBreakdownItem[] | undefined = res.env_breakdown?.map((e) => ({
    environment: e.environment,
    account_id: e.account_id,
    account_display_name: e.account_display_name,
    total_cost: e.total_cost,
    previous_period_cost: e.previous_period_cost,
    change_pct: e.change_pct,
    ledger_g: e.ledger_g,
    ledger_p: e.ledger_p,
    ledger_u: e.ledger_u,
    ledger_b: e.ledger_b,
  }));
  // [Ref: 用户需求 仅四大分类] 从 domain_breakdown 构建 billDetail（计算资源、存储、网络、安全）
  const billDetail =
    domainBreakdown.length > 0
      ? {
          compute: domainBreakdown.find((d) => d.domain === '计算资源')?.cost ?? 0,
          storage: domainBreakdown.find((d) => d.domain === '存储')?.cost ?? 0,
          network: domainBreakdown.find((d) => d.domain === '网络')?.cost ?? 0,
          security: domainBreakdown.find((d) => d.domain === '安全')?.cost ?? 0,
        }
      : undefined;

  const ledgerRaw = res.ledger;
  const ledger: FinOpsLedger | undefined =
    ledgerRaw && typeof ledgerRaw === 'object'
      ? {
          C: ledgerRaw.C,
          G: ledgerRaw.G,
          P: ledgerRaw.P,
          U: ledgerRaw.U,
          B: ledgerRaw.B,
        }
      : undefined;

  const requestTrack = opts?.requestTrack ?? 'finance';
  const reconciliation = res.reconciliation;

  return {
    totalBillableCost: res.total_cost,
    totalOptimizableSpace: totalOptimizable,
    globalEfficiency,
    domainBreakdown,
    envBreakdown,
    billDetail,
    lastUpdatedAt,
    billDataStatus,
    displayNote,
    effectiveRequestTrack: requestTrack,
    ledger,
    reconciliation,
    ledgerSnapshotNote,
  };
}

/**
 * NamespaceCostSummaryApiItem[] -> NamespaceCost[]
 */
export function adaptNamespacesToNamespaceCosts(
  items: NamespaceCostSummaryApiItem[] | null | undefined,
): NamespaceCost[] {
  const list = items ?? [];
  return list.map((n) => {
    const efficiency = gradeToEfficiency(n.grade ?? 'Healthy');
    const optimizableRatio = 1 - efficiency / 100;
    return {
      namespace: n.name,
      cost: n.cost,
      optimizableSpace: n.cost * optimizableRatio || 0,
      efficiency,
      resourceUsage: { cpu: 0, memory: 0, storage: 0 },
      recommendations: [],
    };
  });
}

/** 钻取 API 响应（可能带 snake_case cost_breakdown） */
export interface DrilldownApiResponse {
  id: string;
  name: string;
  type: string;
  cost: number;
  optimizableSpace: number;
  efficiency: number;
  cost_breakdown?: DrilldownCostBreakdown;
  costBreakdown?: DrilldownCostBreakdown;
  children?: DrilldownApiResponse[];
}

/**
 * 将 API 钻取响应规范为前端 DrilldownItem（统一 costBreakdown、递归 children）
 */
export function adaptDrilldownResponse(res: DrilldownApiResponse): DrilldownItem {
  const costBreakdown =
    res.costBreakdown ?? res.cost_breakdown;
  const children = res.children?.map((c) => adaptDrilldownResponse(c));
  const item: DrilldownItem = {
    id: res.id,
    name: res.name,
    type: res.type as DrilldownItem['type'],
    cost: res.cost,
    optimizableSpace: res.optimizableSpace,
    efficiency: res.efficiency,
    ...(costBreakdown && { costBreakdown }),
    ...(children && children.length > 0 && { children }),
  };
  return item;
}

function gradeToEfficiency(grade: string): number {
  switch (grade) {
    case 'Zombie':
      return 10;
    case 'OverProvisioned':
      return 40;
    case 'Healthy':
      return 70;
    case 'Risk':
      return 90;
    default:
      return 70;
  }
}
