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

// 按时间范围生成确定性的基准倍数（用于区分 7d/30d/month/quarter 数据差异）
const periodMultipliers: Record<CostTimeRange, { cost: number; optim: number; efficiency: number }> = {
  '7d': { cost: 0.25, optim: 0.28, efficiency: 68 },       // 近7天：约 1/4 月成本，效率略低
  '30d': { cost: 1, optim: 1, efficiency: 70 },            // 近30天：基准
  'month': { cost: 1.05, optim: 1.02, efficiency: 71 },    // 本月：略高于30天
  'quarter': { cost: 3.2, optim: 3.1, efficiency: 72 },     // 本季度：约 3 倍月
};

// 上一周期相对本期的固定比例，保证环比可校验：环比 = (本期 - 上期) / 上期 * 100
const PREVIOUS_COST_RATIO = 0.94;     // 上期总账单 = 本期 * 0.94 -> 环比约 +6.4%
const PREVIOUS_OPTIM_RATIO = 0.92;   // 上期可优化空间 = 本期 * 0.92 -> 环比约 +8.7%
const PREVIOUS_EFF_DELTA = -2;       // 上期效率 = 本期 - 2 个百分点

const generateMockCostMetrics = (opts?: {
  period?: CostTimeRange;
  compareMode?: CostCompareMode;
}): CostMetrics => {
  const period = opts?.period ?? '30d';
  const m = periodMultipliers[period];
  const baseCost = Math.round(125000 * m.cost);
  const baseOptim = Math.round(37500 * m.optim);
  const baseEff = m.efficiency;

  const domainBreakdown = [
    { domain: '计算资源', cost: Math.round(85000 * m.cost), optimizableSpace: Math.round(25500 * m.optim), efficiency: baseEff },
    { domain: '存储资源', cost: Math.round(25000 * m.cost), optimizableSpace: Math.round(7500 * m.optim), efficiency: baseEff },
    { domain: '网络资源', cost: Math.round(15000 * m.cost), optimizableSpace: Math.round(4500 * m.optim), efficiency: baseEff },
  ];

  const result: CostMetrics = {
    totalBillableCost: baseCost,
    totalOptimizableSpace: baseOptim,
    globalEfficiency: baseEff,
    domainBreakdown,
    billDetail: {
      compute: Math.round(85000 * m.cost),
      storage: Math.round(25000 * m.cost),
      network: Math.round(15000 * m.cost),
      other: 0,
    },
  };

  if (opts?.compareMode === 'previous') {
    result.previousPeriod = {
      totalBillableCost: Math.round(baseCost * PREVIOUS_COST_RATIO),
      totalOptimizableSpace: Math.round(baseOptim * PREVIOUS_OPTIM_RATIO),
      globalEfficiency: Math.max(0, baseEff + PREVIOUS_EFF_DELTA),
    };
  }
  return result;
};

const generateMockNamespaceCosts = (opts?: { period?: CostTimeRange }): NamespaceCost[] => {
  const period = opts?.period ?? '30d';
  const m = periodMultipliers[period];
  return [
    {
      namespace: 'production',
      cost: Math.round(65000 * m.cost),
      optimizableSpace: Math.round(19500 * m.optim),
      efficiency: m.efficiency,
      resourceUsage: {
        cpu: 65,
        memory: 72,
        storage: 58,
      },
      recommendations: ['优化Pod资源配置', '合并相似工作负载'],
    },
    {
      namespace: 'staging',
      cost: Math.round(35000 * m.cost),
      optimizableSpace: Math.round(10500 * m.optim),
      efficiency: m.efficiency,
      resourceUsage: {
        cpu: 45,
        memory: 52,
        storage: 62,
      },
      recommendations: ['减少副本数量', '优化存储使用'],
    },
    {
      namespace: 'development',
      cost: Math.round(25000 * m.cost),
      optimizableSpace: Math.round(7500 * m.optim),
      efficiency: m.efficiency,
      resourceUsage: {
        cpu: 35,
        memory: 42,
        storage: 55,
      },
      recommendations: ['清理未使用的资源', '优化开发环境配置'],
    },
  ];
};

// 为算力钻取节点生成成本分解（cpu+memory 为主，storage/network 为辅）
const mockCostBreakdown = (
  cost: number,
  opts?: { cpuRatio?: number; memRatio?: number; storageRatio?: number; networkRatio?: number },
): DrilldownItem['costBreakdown'] => {
  const cpu = opts?.cpuRatio ?? 0.5;
  const mem = opts?.memRatio ?? 0.35;
  const storage = opts?.storageRatio ?? 0.1;
  const network = opts?.networkRatio ?? 0.05;
  return {
    cpu: Math.round(cost * cpu),
    memory: Math.round(cost * mem),
    storage: Math.round(cost * storage),
    network: Math.round(cost * network),
  };
};

// 存储钻取 Mock：namespace -> storage_class -> pvc
const generateMockStorageDrilldown = (type: string, id: string): DrilldownItem => {
  const cost = type === 'namespace' ? 12000 : type === 'storage_class' ? 6000 : 2000;
  const optimizableSpace = Math.round(cost * 0.3);
  const efficiency = 68;
  const item: DrilldownItem = {
    id,
    name: type === 'namespace' ? id : `${type}-${id}`,
    type: type as DrilldownItem['type'],
    cost,
    optimizableSpace,
    efficiency,
    costBreakdown: mockCostBreakdown(cost, { cpuRatio: 0.05, memRatio: 0.05, storageRatio: 0.85, networkRatio: 0.05 }),
  };
  if (type === 'namespace') {
    item.children = [
      {
        id: 'standard',
        name: 'standard',
        type: 'storage_class',
        cost: 6000,
        optimizableSpace: 1800,
        efficiency: 70,
        costBreakdown: mockCostBreakdown(6000, { cpuRatio: 0, memRatio: 0, storageRatio: 0.9, networkRatio: 0.1 }),
        children: [
          {
            id: 'pvc-data-1',
            name: 'pvc-data-1',
            type: 'pvc',
            cost: 2000,
            optimizableSpace: 600,
            efficiency: 70,
            costBreakdown: mockCostBreakdown(2000, { cpuRatio: 0, memRatio: 0, storageRatio: 1, networkRatio: 0 }),
          },
          {
            id: 'pvc-data-2',
            name: 'pvc-data-2',
            type: 'pvc',
            cost: 4000,
            optimizableSpace: 1200,
            efficiency: 70,
            costBreakdown: mockCostBreakdown(4000, { cpuRatio: 0, memRatio: 0, storageRatio: 1, networkRatio: 0 }),
          },
        ],
      },
    ];
  }
  if (type === 'storage_class') {
    item.children = [
      {
        id: 'pvc-data-1',
        name: 'pvc-data-1',
        type: 'pvc',
        cost: 2000,
        optimizableSpace: 600,
        efficiency: 70,
        costBreakdown: mockCostBreakdown(2000, { cpuRatio: 0, memRatio: 0, storageRatio: 1, networkRatio: 0 }),
      },
      {
        id: 'pvc-data-2',
        name: 'pvc-data-2',
        type: 'pvc',
        cost: 4000,
        optimizableSpace: 1200,
        efficiency: 70,
        costBreakdown: mockCostBreakdown(4000, { cpuRatio: 0, memRatio: 0, storageRatio: 1, networkRatio: 0 }),
      },
    ];
  }
  return item;
};

// 网络钻取 Mock：namespace -> service -> ingress/lb
const generateMockNetworkDrilldown = (type: string, id: string): DrilldownItem => {
  const cost = type === 'namespace' ? 8000 : type === 'service' ? 3500 : 1500;
  const optimizableSpace = Math.round(cost * 0.25);
  const efficiency = 72;
  const item: DrilldownItem = {
    id,
    name: type === 'namespace' ? id : `${type}-${id}`,
    type: type as DrilldownItem['type'],
    cost,
    optimizableSpace,
    efficiency,
    costBreakdown: mockCostBreakdown(cost, { cpuRatio: 0.05, memRatio: 0.05, storageRatio: 0.05, networkRatio: 0.85 }),
  };
  if (type === 'namespace') {
    item.children = [
      {
        id: 'api-gateway',
        name: 'api-gateway',
        type: 'service',
        cost: 3500,
        optimizableSpace: 875,
        efficiency: 75,
        costBreakdown: mockCostBreakdown(3500, { cpuRatio: 0.05, memRatio: 0.05, storageRatio: 0, networkRatio: 0.9 }),
        children: [
          {
            id: 'ingress-main',
            name: 'ingress-main',
            type: 'ingress',
            cost: 1500,
            optimizableSpace: 375,
            efficiency: 75,
            costBreakdown: mockCostBreakdown(1500, { cpuRatio: 0, memRatio: 0, storageRatio: 0, networkRatio: 1 }),
          },
        ],
      },
    ];
  }
  if (type === 'service') {
    item.children = [
      {
        id: 'ingress-main',
        name: 'ingress-main',
        type: 'ingress',
        cost: 1500,
        optimizableSpace: 375,
        efficiency: 75,
        costBreakdown: mockCostBreakdown(1500, { cpuRatio: 0, memRatio: 0, storageRatio: 0, networkRatio: 1 }),
      },
    ];
  }
  return item;
};

const generateMockDrilldownData = (
  type: string,
  id: string,
  dimension: ResourceDimension = 'compute',
): DrilldownItem => {
  if (dimension === 'storage') {
    return generateMockStorageDrilldown(type, id);
  }
  if (dimension === 'network') {
    return generateMockNetworkDrilldown(type, id);
  }

  const cost = Math.random() * 10000 + 1000;
  const optimizableSpace = Math.random() * 3000 + 300;
  const efficiency = Math.floor(Math.random() * 30) + 70;
  const baseItem: DrilldownItem = {
    id,
    name: `${type}-${id}`,
    type: type as DrilldownItem['type'],
    cost,
    optimizableSpace,
    efficiency,
    costBreakdown: mockCostBreakdown(cost),
  };

  if (type === 'namespace') {
    const childCost = 5000;
    const childOptim = 1500;
    baseItem.children = [
      {
        id: 'node-1',
        name: 'node-1',
        type: 'node',
        cost: childCost,
        optimizableSpace: childOptim,
        efficiency: 70,
        costBreakdown: mockCostBreakdown(childCost, { cpuRatio: 0.55, memRatio: 0.38, storageRatio: 0.04, networkRatio: 0.03 }),
        children: [
          {
            id: 'workload-1',
            name: 'workload-1',
            type: 'workload',
            cost: 2500,
            optimizableSpace: 750,
            efficiency: 70,
            costBreakdown: mockCostBreakdown(2500),
            children: [
              {
                id: 'pod-1',
                name: 'pod-1',
                type: 'pod',
                cost: 1250,
                optimizableSpace: 375,
                efficiency: 70,
                costBreakdown: mockCostBreakdown(1250),
              },
            ],
          },
        ],
      },
    ];
  }

  return baseItem;
};

const generateMockSLOStatusByScope = (scope?: SLOScope): SLOStatus[] => {
  const serviceRows: SLOStatus[] = [
    {
      serviceName: 'api-gateway',
      status: 'healthy',
      uptime: 99.95,
      responseTime: 120,
      errorRate: 0.02,
      scope: 'service',
      scopeId: 'api-gateway',
      scopeName: 'api-gateway',
    },
    {
      serviceName: 'auth-service',
      status: 'warning',
      uptime: 99.85,
      responseTime: 250,
      errorRate: 0.15,
      scope: 'service',
      scopeId: 'auth-service',
      scopeName: 'auth-service',
    },
    {
      serviceName: 'payment-service',
      status: 'critical',
      uptime: 98.5,
      responseTime: 850,
      errorRate: 2.3,
      scope: 'service',
      scopeId: 'payment-service',
      scopeName: 'payment-service',
    },
  ];

  if (scope === 'global') {
    const items = serviceRows;
    const uptime = items.reduce((s, i) => s + i.uptime, 0) / items.length;
    const responseTime = Math.round(
      items.reduce((s, i) => s + i.responseTime, 0) / items.length,
    );
    const errorRate = items.reduce((s, i) => s + i.errorRate, 0) / items.length;
    const status: 'healthy' | 'warning' | 'critical' =
      items.some(i => i.status === 'critical') ? 'critical' : items.some(i => i.status === 'warning') ? 'warning' : 'healthy';
    return [
      {
        serviceName: '全域',
        status,
        uptime,
        responseTime,
        errorRate,
        scope: 'global',
        scopeId: 'global',
        scopeName: '全域',
      },
    ];
  }

  if (scope === 'domain') {
    return [
      {
        serviceName: '计算资源域',
        status: 'warning',
        uptime: 99.2,
        responseTime: 280,
        errorRate: 0.4,
        scope: 'domain',
        scopeId: 'compute',
        scopeName: '计算资源域',
      },
      {
        serviceName: '存储资源域',
        status: 'healthy',
        uptime: 99.9,
        responseTime: 150,
        errorRate: 0.05,
        scope: 'domain',
        scopeId: 'storage',
        scopeName: '存储资源域',
      },
      {
        serviceName: '网络资源域',
        status: 'healthy',
        uptime: 99.7,
        responseTime: 180,
        errorRate: 0.12,
        scope: 'domain',
        scopeId: 'network',
        scopeName: '网络资源域',
      },
    ];
  }

  if (scope === 'pod') {
    return [
      {
        serviceName: 'api-gateway-7d8f-xyz',
        status: 'healthy',
        uptime: 99.98,
        responseTime: 115,
        errorRate: 0.01,
        scope: 'pod',
        scopeId: 'api-gateway-7d8f-xyz',
        scopeName: 'api-gateway-7d8f-xyz',
      },
      {
        serviceName: 'auth-service-9k2m-abc',
        status: 'warning',
        uptime: 99.8,
        responseTime: 260,
        errorRate: 0.18,
        scope: 'pod',
        scopeId: 'auth-service-9k2m-abc',
        scopeName: 'auth-service-9k2m-abc',
      },
      {
        serviceName: 'payment-service-4n1p-def',
        status: 'critical',
        uptime: 98.2,
        responseTime: 920,
        errorRate: 2.8,
        scope: 'pod',
        scopeId: 'payment-service-4n1p-def',
        scopeName: 'payment-service-4n1p-def',
      },
    ];
  }

  // default: service level
  return serviceRows;
};

const generateMockROITrends = (): ROITrend[] => {
  const trends: ROITrend[] = [];
  const today = new Date();

  for (let i = 30; i >= 0; i--) {
    const date = new Date(today);
    date.setDate(date.getDate() - i);
    const dateString = date.toISOString().split('T')[0];

    trends.push({
      date: dateString,
      value: Math.random() * 100000 + 50000,
      cost: Math.random() * 80000 + 20000,
      efficiency: Math.floor(Math.random() * 30) + 70,
    });
  }

  return trends;
};

export const mockApi = {
  getGlobalCostMetrics: (params?: {
    period?: CostTimeRange;
    compareMode?: CostCompareMode;
  }): Promise<CostMetrics> => {
    return Promise.resolve(generateMockCostMetrics(params));
  },

  getNamespaceCosts: (params?: { period?: CostTimeRange }): Promise<NamespaceCost[]> => {
    return Promise.resolve(generateMockNamespaceCosts(params));
  },

  getDrilldownData: (
    type: string,
    id: string,
    dimension: ResourceDimension = 'compute',
  ): Promise<DrilldownItem> => {
    return Promise.resolve(generateMockDrilldownData(type, id, dimension));
  },

  getSLOStatus: (scope?: SLOScope): Promise<SLOStatus[]> => {
    return Promise.resolve(generateMockSLOStatusByScope(scope));
  },

  getROITrends: (): Promise<ROITrend[]> => {
    return Promise.resolve(generateMockROITrends());
  },
};
