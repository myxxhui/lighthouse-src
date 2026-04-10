/** 成本透视时间范围 [Ref: 01_实践 front_end_time_ranges] */
export type CostTimeRange =
  | 'month'       // 本月
  | 'last_month'  // 上月
  | 'quarter'     // 这季度
  | 'last_quarter'// 上季度
  | 'this_year'   // 今年
  | 'last_year'   // 去年
  | 'custom';     // 自定义（月范围，最多3年）

/** 成本对比模式 */
export type CostCompareMode = 'none' | 'previous';

/** FinOps 双轨视角 [Ref: 03_Phase6/01_FinOps双轨与全域成本重构/01_设计 §前端展示规划] */
export type CostTrack = 'technical' | 'finance';

/** 五维账本 C/G/P/U/B */
export interface FinOpsLedger {
  C?: number;
  G?: number;
  P?: number;
  U?: number;
  B?: number;
}

export interface FinOpsReconciliation {
  residual?: number;
  explain?: string;
}

/** 成本账单详情：按四大类拆分的费用（与领域可对齐）；仅四大类，无其它。[Ref: 用户需求 仅四大分类] */
export interface BillDetail {
  compute: number;
  storage: number;
  network: number;
  security: number;
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

/** 按环境总账与对比；consumption_cost/ledger_g/ledger_p 为按账号事实（与临时程序 Y/H/已还款 同源）；ledger_b/u 见 API [Ref: 01_设计 D9-4、03_Phase6/01_FinOps] */
export interface EnvBreakdownItem {
  environment: string;
  account_id: string;
  account_display_name: string;
  total_cost: number;
  /** 资金轨：聚合表/月原始消耗口径，与 total_cost（实付）分离 [Ref: 03_Phase6/01_FinOps] */
  consumption_cost?: number;
  previous_period_cost?: number;
  change_pct?: number;
  ledger_g?: number;
  ledger_p?: number;
  ledger_u?: number;
  ledger_b?: number;
  /** 后端 YAML 云环境账户标题，如 C66-Aliyun-Uat [Ref: 03_Phase6/03_前端全域成本透视/01_设计] */
  cloud_account_label?: string;
  /** 国内站 / 国际站 [Ref: 03_Phase6/03_前端全域成本透视/01_设计] */
  cloud_account_site_note?: string;
}

/** 成本项目汇总（与 GET /api/v1/cost/global 的 project_breakdown 一致）；ledger_* 与成员环境 env_breakdown 加总一致 [Ref: 03_Phase6/03_前端全域成本透视/01_设计] */
export interface ProjectBreakdownItem {
  project_id: number;
  code: string;
  name: string;
  sort_order: number;
  total_cost: number;
  consumption_cost?: number;
  ledger_g?: number;
  ledger_p?: number;
  ledger_u?: number;
  ledger_b?: number;
}

export interface CostMetrics {
  totalBillableCost: number;
  totalOptimizableSpace: number;
  globalEfficiency: number;
  domainBreakdown: DomainBreakdown[];
  /** 成本项目卡片 [Ref: 03_Phase6/03_前端全域成本透视/01_设计] */
  projectBreakdown?: ProjectBreakdownItem[];
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
  /**
   * 账单对账状态标识 [Ref: 16_云账单动态对账与高可靠处理规范 §三段式]
   *   FINALIZED   → 已财务核算（历史月，权威）
   *   PRELIMINARY → 动态同步（当前月/近期数据；UI 附自动调度说明）
   *   RECONCILING → 对账中
   *   DIRTY       → 数据偏差
   *   undefined   → 未知/不适用
   */
  billDataStatus?: string;
  /** 展示说明：月粒度周期净退款已抵减时后端返回，前端可展示「该周期净退款已抵减」 */
  displayNote?: string;
  /** 请求携带的 track（仅 URL 含 track= 时）；用于 Hero Tag 与口径说明 [Ref: 03_Phase6/01_FinOps双轨与全域成本重构/01_设计] */
  effectiveRequestTrack?: CostTrack;
  /** 五维快照 */
  ledger?: FinOpsLedger;
  reconciliation?: FinOpsReconciliation;
  /** 与后端 metadata.ledger_snapshot_note 一致；五维并列与守恒式说明 [Ref: 03_Phase6/01_FinOps双轨与全域成本重构/01_设计] */
  ledgerSnapshotNote?: string;
}

/** 产品级成本（账单详情 top-N 与详情跳转） */
export interface ProductCostItem {
  product: string;
  cost: number;
}

/** 全环境云产品明细项 [Ref: 01_设计 D9-8 GET /api/v1/cost/drilldown/global] */
export interface CloudProductDrilldownItem {
  product_code: string;
  product_name?: string;
  cost: number;
  category: string;
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

export interface ApiError {
  message: string;
  code: string;
  timestamp: string;
  /** POST /finops/sync-jobs 409 FINOPS_SYNC_ACTIVE 时由后端返回，用于订阅进行中任务的进度 [Ref: 03_Phase6/01_FinOps 主动同步] */
  active_job_id?: number;
}
