/** 成本透视时间范围 */
export type CostTimeRange = '7d' | '30d' | 'month' | 'quarter';

/** 成本对比模式 */
export type CostCompareMode = 'none' | 'previous';

/** 成本账单详情：按资源类型拆分的费用（与领域可对齐） */
export interface BillDetail {
  compute: number;
  storage: number;
  network: number;
  other: number;
}

/** 资源维度：算力 / 存储 / 网络，与 domain、billDetail 对齐 */
export type ResourceDimension = 'compute' | 'storage' | 'network';

/** 钻取节点成本分解（与后端 CostBreakdown 对齐） */
export interface DrilldownCostBreakdown {
  cpu: number;
  memory: number;
  storage: number;
  network: number;
}

export interface CostMetrics {
  totalBillableCost: number;
  totalOptimizableSpace: number; // 使用"可优化空间"代替"浪费"
  globalEfficiency: number;
  domainBreakdown: DomainBreakdown[];
  /** 可选：按资源类型的账单详情（基础计算、存储、网络、其它云产品） */
  billDetail?: BillDetail;
  /** 可选：对比上一周期时的基准值（用于展示环比） */
  previousPeriod?: {
    totalBillableCost: number;
    totalOptimizableSpace: number;
    globalEfficiency: number;
  };
}

export interface DomainBreakdown {
  domain: string;
  cost: number;
  optimizableSpace: number;
  efficiency: number;
}

export interface NamespaceCost {
  namespace: string;
  cost: number;
  optimizableSpace: number;
  efficiency: number;
  resourceUsage: ResourceUsage;
  recommendations: string[];
}

export interface ResourceUsage {
  cpu: number;
  memory: number;
  storage: number;
}

/** 算力钻取节点类型 */
export type ComputeDrilldownType = 'namespace' | 'node' | 'workload' | 'pod';
/** 存储钻取节点类型 */
export type StorageDrilldownType = 'namespace' | 'storage_class' | 'pvc' | 'volume';
/** 网络钻取节点类型 */
export type NetworkDrilldownType = 'namespace' | 'service' | 'ingress' | 'lb' | 'traffic_type';
/** 钻取节点类型联合（按维度使用） */
export type DrilldownNodeType =
  | ComputeDrilldownType
  | StorageDrilldownType
  | NetworkDrilldownType;

export interface DrilldownItem {
  id: string;
  name: string;
  type: DrilldownNodeType;
  cost: number;
  optimizableSpace: number;
  efficiency: number;
  /** 成本构成：按资源类型拆分，算力钻取每层返回；存储/网络可部分占位 */
  costBreakdown?: DrilldownCostBreakdown;
  children?: DrilldownItem[];
}

/** SLO 层级，与成本透视一致：全域 → 域 → 服务 → Pod */
export type SLOScope = 'global' | 'domain' | 'service' | 'pod';

export interface SLOStatus {
  serviceName: string;
  status: 'healthy' | 'warning' | 'critical';
  uptime: number;
  responseTime: number;
  errorRate: number;
  /** 层级：全域 / 域 / 服务 / Pod */
  scope?: SLOScope;
  /** 当前层级对象 ID（如 domain 名、service 名、pod 名） */
  scopeId?: string;
  /** 当前层级展示名称 */
  scopeName?: string;
}

export interface ROITrend {
  date: string;
  value: number;
  cost: number;
  efficiency: number;
}

export interface ApiError {
  message: string;
  code: string;
  timestamp: string;
}
