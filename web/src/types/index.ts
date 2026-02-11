export interface CostMetrics {
  totalBillableCost: number;
  totalOptimizableSpace: number; // 使用"可优化空间"代替"浪费"
  globalEfficiency: number;
  domainBreakdown: DomainBreakdown[];
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

export interface DrilldownItem {
  id: string;
  name: string;
  type: 'namespace' | 'node' | 'workload' | 'pod';
  cost: number;
  optimizableSpace: number;
  efficiency: number;
  children?: DrilldownItem[];
}

export interface SLOStatus {
  serviceName: string;
  status: 'healthy' | 'warning' | 'critical';
  uptime: number;
  responseTime: number;
  errorRate: number;
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
