import { CostMetrics, NamespaceCost, DrilldownItem, SLOStatus, ROITrend } from '@/types';

// Mock数据生成器
const generateMockCostMetrics = (): CostMetrics => ({
  totalBillableCost: 125000,
  totalOptimizableSpace: 37500,
  globalEfficiency: 70,
  domainBreakdown: [
    {
      domain: '计算资源',
      cost: 85000,
      optimizableSpace: 25500,
      efficiency: 70,
    },
    {
      domain: '存储资源',
      cost: 25000,
      optimizableSpace: 7500,
      efficiency: 70,
    },
    {
      domain: '网络资源',
      cost: 15000,
      optimizableSpace: 4500,
      efficiency: 70,
    },
  ],
});

const generateMockNamespaceCosts = (): NamespaceCost[] => [
  {
    namespace: 'production',
    cost: 65000,
    optimizableSpace: 19500,
    efficiency: 70,
    resourceUsage: {
      cpu: 65,
      memory: 72,
      storage: 58,
    },
    recommendations: ['优化Pod资源配置', '合并相似工作负载'],
  },
  {
    namespace: 'staging',
    cost: 35000,
    optimizableSpace: 10500,
    efficiency: 70,
    resourceUsage: {
      cpu: 45,
      memory: 52,
      storage: 62,
    },
    recommendations: ['减少副本数量', '优化存储使用'],
  },
  {
    namespace: 'development',
    cost: 25000,
    optimizableSpace: 7500,
    efficiency: 70,
    resourceUsage: {
      cpu: 35,
      memory: 42,
      storage: 55,
    },
    recommendations: ['清理未使用的资源', '优化开发环境配置'],
  },
];

const generateMockDrilldownData = (
  type: 'namespace' | 'node' | 'workload' | 'pod',
  id: string,
): DrilldownItem => {
  const baseItem: DrilldownItem = {
    id,
    name: `${type}-${id}`,
    type,
    cost: Math.random() * 10000 + 1000,
    optimizableSpace: Math.random() * 3000 + 300,
    efficiency: Math.floor(Math.random() * 30) + 70,
  };

  if (type === 'namespace') {
    baseItem.children = [
      {
        id: 'node-1',
        name: 'node-1',
        type: 'node',
        cost: 5000,
        optimizableSpace: 1500,
        efficiency: 70,
        children: [
          {
            id: 'workload-1',
            name: 'workload-1',
            type: 'workload',
            cost: 2500,
            optimizableSpace: 750,
            efficiency: 70,
            children: [
              {
                id: 'pod-1',
                name: 'pod-1',
                type: 'pod',
                cost: 1250,
                optimizableSpace: 375,
                efficiency: 70,
              },
            ],
          },
        ],
      },
    ];
  }

  return baseItem;
};

const generateMockSLOStatus = (): SLOStatus[] => [
  {
    serviceName: 'api-gateway',
    status: 'healthy',
    uptime: 99.95,
    responseTime: 120,
    errorRate: 0.02,
  },
  {
    serviceName: 'auth-service',
    status: 'warning',
    uptime: 99.85,
    responseTime: 250,
    errorRate: 0.15,
  },
  {
    serviceName: 'payment-service',
    status: 'critical',
    uptime: 98.5,
    responseTime: 850,
    errorRate: 2.3,
  },
];

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
  getGlobalCostMetrics: (): Promise<CostMetrics> => {
    return Promise.resolve(generateMockCostMetrics());
  },

  getNamespaceCosts: (): Promise<NamespaceCost[]> => {
    return Promise.resolve(generateMockNamespaceCosts());
  },

  getDrilldownData: (
    type: 'namespace' | 'node' | 'workload' | 'pod',
    id: string,
  ): Promise<DrilldownItem> => {
    return Promise.resolve(generateMockDrilldownData(type, id));
  },

  getSLOStatus: (): Promise<SLOStatus[]> => {
    return Promise.resolve(generateMockSLOStatus());
  },

  getROITrends: (): Promise<ROITrend[]> => {
    return Promise.resolve(generateMockROITrends());
  },
};
