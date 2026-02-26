/** 成本透视时间范围 [Ref: 01_实践 front_end_time_ranges]；'custom' 表示使用自定义日期范围（与 costCustomDateRange 二选一）；这周=this_week 与 近七天=7d 必须区分 */
export type CostTimeRange =
  | '1d'
  | 'this_week'  // 这周：ISO 周周一至昨日
  | '7d'
  | '7d_range'   // 近七天，API 同 7d
  | '30d'
  | 'month'
  | 'quarter'
  | '90d'
  | 'last_week'
  | 'last_month'
  | 'last_quarter'
  | 'custom';

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

/** 按环境（POC/FAT/UAT/PROD）总账与对比 [Ref: 01_设计 D9-4] */
export interface EnvBreakdownItem {
  environment: string;
  account_id: string;
  account_display_name: string;
  total_cost: number;
  previous_period_cost?: number;
  change_pct?: number;
}

export interface CostMetrics {
  totalBillableCost: number;
  totalOptimizableSpace: number;
  globalEfficiency: number;
  domainBreakdown: DomainBreakdown[];
  /** 四环境卡片数据 [Ref: 01_设计 §按环境展示] */
  envBreakdown?: EnvBreakdownItem[];
  billDetail?: BillDetail;
  previousPeriod?: {
    totalBillableCost: number;
    totalOptimizableSpace: number;
    globalEfficiency: number;
  };
  /** D1-3：数据更新至，前端展示「数据更新至 YYYY-MM-DD HH:mm」 */
  lastUpdatedAt?: string;
}

/** 产品级成本（账单详情 top-N 与详情跳转） */
export interface ProductCostItem {
  product: string;
  cost: number;
}

export interface DomainBreakdown {
  domain: string;
  cost: number;
  optimizableSpace: number;
  efficiency: number;
  /** 成本最高的最多 4 个产品，用于详情与跳转 */
  topProducts?: ProductCostItem[];
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
