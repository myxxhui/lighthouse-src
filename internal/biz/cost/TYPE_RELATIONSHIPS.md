# Lighthouse 核心类型关系图

本文档描述了Lighthouse项目的核心类型系统及其关系，基于DDD（领域驱动设计）原则构建。

## 1. 核心类型层次结构

### 1.1 成本计算领域 (Cost Domain)
```
ResourceMetric (基础资源指标)
    │
    ├── CostResult (单资源成本结果)
    │
    └── DualCostResult (双重成本结果 - CPU+Memory)
            │
            ├── CPUBillableCost, CPUUsageCost, CPUWasteCost
            ├── MemBillableCost, MemUsageCost, MemWasteCost
            ├── CPUEfficiencyScore, MemEfficiencyScore
            └── CPUGrade, MemGrade, OverallGrade
```

### 1.2 聚合领域 (Aggregation Domain)
```
Aggregator (聚合接口)
    │
    ├── Level() AggregationLevel
    ├── Aggregate([]DualCostResult) (*AggregationResult, error)
    └── SupportsDimension(string) bool
            │
            ├── NamespaceAggregator (L1: 命名空间级)
            ├── NodeAggregator (L2: 节点级)
            ├── WorkloadAggregator (L3: 工作负载级)
            ├── PodAggregator (L4: Pod级)
            └── ClusterAggregator (L0: 集群级)

AggregationResult (聚合结果基类)
    │
    ├── NamespaceAggregationResult (L1扩展)
    ├── NodeAggregationResult (L2扩展)
    ├── WorkloadAggregationResult (L3扩展)
    └── PodAggregationResult (L4扩展)
```

### 1.3 SLO领域 (SLO Domain)
```
SLOStatus (状态枚举)
    │
    ├── SLOMetrics (SLO指标)
    ├── SLOConfig (SLO配置)
    └── SLOResult (SLO评估结果)
            │
            ├── AvailabilityScore (可用性得分)
            ├── LatencyP95 (延迟P95)
            └── SLOViolationDetails (违规详情)

EvidenceChain (证据链)
    │
    ├── EvidenceImpact (影响维度)
    ├── EvidenceChange (变更维度)
    └── EvidenceResource (资源维度)
```

### 1.4 ROI领域 (ROI Domain)
```
BaselineSnapshot (基线快照)
    │
    ├── DailyComparison (每日对比)
    ├── FinancialSavings (财务节省)
    └── EfficiencyGains (效率提升)
            │
            ├── CostSavingsBreakdown (成本节省细分)
            ├── ResourceRecoveryMetrics (资源回收指标)
            └── FinancialImpactAnalysis (财务影响分析)
```

## 2. 四层钻取架构 (L1-L4)

### 2.1 层级定义
- **L0 (集群级)**: 全局视图，跨集群聚合
- **L1 (命名空间级)**: 业务域视图，按命名空间分组
- **L2 (节点级)**: 基础设施视图，按物理/虚拟节点分组
- **L3 (工作负载级)**: 应用视图，按Deployment/StatefulSet分组
- **L4 (Pod级)**: 实例视图，按单个Pod分组

### 2.2 数据流
```
原始指标 → 双重成本计算 → 层级聚合 → 可视化展示
    │           │              │            │
    ↓           ↓              ↓            ↓
ResourceMetric → DualCostResult → AggregationResult → Dashboard
```

## 3. 关键类型关系

### 3.1 双重成本模型关系
```
┌─────────────┐    ┌─────────────┐    ┌─────────────┐
│ Resource    │    │ DualCost    │    │ Aggregation │
│ Metric      │───▶│ Result      │───▶│ Result      │
└─────────────┘    └─────────────┘    └─────────────┘
        │                  │                  │
        │ 计算成本         │ 聚合统计         │ 分级展示
        ↓                  ↓                  ↓
┌─────────────┐    ┌─────────────┐    ┌─────────────┐
│ 账单成本    │    │ 命名空间    │    │ 仪表板      │
│ 使用成本    │    │ 节点        │    │ 报告        │
│ 浪费成本    │    │ 工作负载    │    │ 告警        │
└─────────────┘    │ Pod         │    └─────────────┘
                   └─────────────┘
```

### 3.2 SLO与证据链关系
```
┌─────────────┐    ┌─────────────┐    ┌─────────────┐
│ SLO         │    │ Snapshot    │    │ Evidence    │
│ Violation   │───▶│ Trigger     │───▶│ Chain       │
└─────────────┘    └─────────────┘    └─────────────┘
        │                  │                  │
        │ 触发快照         │ 收集证据         │ 根因分析
        ↓                  ↓                  ↓
┌─────────────┐    ┌─────────────┐    ┌─────────────┐
│ 告警        │    │ 日志        │    │ 修复建议    │
│ 通知        │    │ 指标        │    │ 预防措施    │
│ 升级        │    │ 事件        │    │ 知识库      │
└─────────────┘    │ 跟踪        │    └─────────────┘
                   └─────────────┘
```

### 3.3 ROI追踪关系
```
┌─────────────┐    ┌─────────────┐    ┌─────────────┐
│ Baseline    │    │ Optimization│    │ ROI         │
│ Snapshot    │───▶│ Activity    │───▶│ Tracking    │
└─────────────┘    └─────────────┘    └─────────────┘
        │                  │                  │
        │ 对比基准         │ 记录优化         │ 追踪回报
        ↓                  ↓                  ↓
┌─────────────┐    ┌─────────────┐    ┌─────────────┐
│ 初始状态    │    │ 节省金额    │    │ 财务报告    │
│ 资源使用    │    │ 资源回收    │    │ 效率报告    │
│ 成本基线    │    │ 等效节点    │    │ 投资回报    │
└─────────────┘    └─────────────┘    └─────────────┘
```

## 4. 精度与错误控制

### 4.1 精度要求
- **成本计算**: 2位小数 (0.01精度)
- **效率分数**: 2位小数 (0.01%精度)
- **资源指标**: 4位小数 (0.0001核心精度)
- **最大误差**: <1% (MaxErrorPercentage: 0.01)

### 4.2 错误边界
```
PrecisionConfig (精度配置)
    ├── CostDecimalPlaces: 2
    ├── EfficiencyDecimalPlaces: 2
    ├── ResourceDecimalPlaces: 4
    └── MaxErrorPercentage: 0.01
```

## 5. 可扩展性设计

### 5.1 维度扩展
聚合系统支持通过`dimensions`字段扩展新的聚合维度：
- 成本维度 (cost)
- 效率维度 (efficiency)
- 浪费维度 (waste)
- 资源维度 (resource_count)
- 自定义维度 (custom)

### 5.2 聚合器扩展
通过实现`Aggregator`接口可以添加新的聚合层级：
```go
type CustomAggregator struct {
    BaseAggregator
    // 自定义字段
}
```

### 5.3 证据链扩展
证据链采用维度设计，可轻松添加新的证据维度：
- 影响维度 (impact)
- 变更维度 (change)
- 资源维度 (resource)
- 自定义维度 (custom)

## 6. 类型安全设计

### 6.1 枚举类型
- `EfficiencyGrade`: Zombie, OverProvisioned, Healthy, UnderProvisioned, Risk
- `AggregationLevel`: Namespace, Node, Workload, Pod, Cluster
- `SLOStatus`: Healthy, Warning, Critical

### 6.2 接口设计
- `Aggregator`: 聚合接口，确保多态性
- 所有接口都遵循单一职责原则

### 6.3 验证方法
- 所有数值类型都有合理的默认值和边界检查
- JSON序列化/反序列化支持
- 时间戳标准化处理

## 7. 文件组织

### 7.1 包结构
```
pkg/costmodel/types.go          # 公共核心类型
internal/biz/cost/types.go      # 成本业务类型
internal/biz/cost/aggregator.go # 聚合器实现
internal/biz/slo/types.go       # SLO业务类型
internal/biz/roi/types.go       # ROI业务类型
```

### 7.2 依赖关系
```
internal/biz/* → pkg/costmodel → 无外部依赖
    │               │
    │               └── 核心类型定义
    └── 业务特定类型扩展
```

## 8. 后续开发指南

### 8.1 添加新类型
1. 确定类型所属领域（成本/SLO/ROI）
2. 在对应包中定义类型
3. 添加必要的JSON标签和文档注释
4. 考虑与其他类型的兼容性

### 8.2 扩展聚合逻辑
1. 实现新的`Aggregator`接口
2. 在`AggregatorFactory`中注册
3. 添加对应的`AggregationResult`扩展
4. 更新类型关系文档

### 8.3 精度控制
1. 使用`PrecisionConfig`控制计算精度
2. 确保误差<1%要求
3. 添加精度验证测试

## 9. 总结

Lighthouse的类型系统设计遵循以下原则：
- **领域驱动**: 按业务领域组织类型
- **类型安全**: 使用强类型和接口约束
- **可扩展**: 支持未来功能扩展
- **可测试**: 所有类型都支持单元测试
- **文档化**: 完整的类型文档和关系说明

这个类型系统为Lighthouse的核心业务逻辑提供了坚实的基础，支持双重成本计算、四层聚合、SLO监控和ROI追踪等关键功能。