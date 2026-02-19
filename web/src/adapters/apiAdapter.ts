import {
  CostMetrics,
  NamespaceCost,
  DomainBreakdown,
  DrilldownItem,
  DrilldownCostBreakdown,
} from '@/types';

/**
 * 后端 API 响应类型（与 backend dto 一致）
 */
export interface GlobalCostApiResponse {
  total_cost: number;
  total_optimizable?: number;
  global_efficiency?: number;
  domain_breakdown?: DomainBreakdownApiItem[];
  namespaces: NamespaceCostSummaryApiItem[];
  timestamp: string;
}

export interface DomainBreakdownApiItem {
  domain: string;
  cost: number;
  optimizable_space: number;
  efficiency: number;
}

export interface NamespaceCostSummaryApiItem {
  name: string;
  cost: number;
  grade?: string;
  pod_count?: number;
  node_count?: number;
}

/**
 * GlobalCostApiResponse -> CostMetrics
 */
export function adaptGlobalCostToCostMetrics(res: GlobalCostApiResponse): CostMetrics {
  const totalOptimizable = res.total_optimizable ?? res.total_cost * 0.3;
  const globalEfficiency = res.global_efficiency ?? 70;
  const totalCost = res.total_cost || 1;
  const domainBreakdown: DomainBreakdown[] =
    res.domain_breakdown?.map((d) => ({
      domain: d.domain,
      cost: d.cost,
      optimizableSpace: d.optimizable_space,
      efficiency: d.efficiency,
    })) ??
    res.namespaces.map((n) => ({
      domain: n.name,
      cost: n.cost,
      optimizableSpace: totalOptimizable * (n.cost / totalCost) || 0,
      efficiency: gradeToEfficiency(n.grade ?? 'Healthy'),
    }));

  return {
    totalBillableCost: res.total_cost,
    totalOptimizableSpace: totalOptimizable,
    globalEfficiency,
    domainBreakdown,
  };
}

/**
 * NamespaceCostSummaryApiItem[] -> NamespaceCost[]
 */
export function adaptNamespacesToNamespaceCosts(
  items: NamespaceCostSummaryApiItem[],
): NamespaceCost[] {
  return items.map((n) => {
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
