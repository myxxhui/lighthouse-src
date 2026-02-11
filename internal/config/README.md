# Lighthouse 配置框架

## 概述

Lighthouse配置框架是一个统一、类型安全、环境感知的配置加载与管理框架，遵循12-factor应用原则和配置与代码分离的安全共识。

## 核心特性

### 1. 三级存储架构配置
- **PostgreSQL控制平面**: 元数据、配置、快照、日报存储
- **ClickHouse证据平面**: 日志、Trace、采样数据存储
- **Prometheus信号平面**: 实时指标、SLO状态查询

### 2. 安全性设计
- 敏感信息（密码、令牌、密钥）通过环境变量注入
- 配置文件仅包含非敏感配置模板
- 支持数据加密和资源限制
- 生产环境强制安全验证

### 3. 多环境支持
- dev/staging/prod 环境标识
- 环境特定配置覆盖机制
- 环境特定验证规则

### 4. 配置加载优先级
1. 环境变量 (最高优先级)
2. 环境特定配置文件 (config.{env}.yaml)
3. 基础配置文件 (config.yaml)
4. 默认值 (最低优先级)

## 配置结构

### 主配置结构 (Config)
```go
type Config struct {
    Env            Environment
    Server         ServerConfig
    Postgres       PostgresConfig      // 控制平面
    ClickHouse     ClickHouseConfig    // 证据平面
    Prometheus     PrometheusConfig    // 信号平面
    Kubernetes     KubernetesConfig
    AnalysisEngine AnalysisEngineConfig
    Retention      RetentionConfig
    Business       BusinessConfig
    Security       SecurityConfig
}
```

### 敏感字段处理
所有敏感字段使用 `mapstructure:"-"` 标记，强制从环境变量加载：
```go
Password string `mapstructure:"-" env:"PG_PASSWORD"`
BearerToken string `mapstructure:"-" env:"PROMETHEUS_BEARER_TOKEN"`
APIKey string `mapstructure:"-" env:"ANALYSIS_ENGINE_API_KEY"`
```

## 使用方法

### 1. 配置加载
```go
loader := config.NewFileLoader("./configs")
cfg, err := loader.Load()
if err != nil {
    log.Fatal(err)
}
```

### 2. 配置验证
```go
validator := config.NewConfigValidator()
if err := validator.Validate(cfg); err != nil {
    log.Fatal(err)
}
```

### 3. 环境变量映射
```go
mapping := config.GetEnvMapping()
for env, desc := range mapping {
    fmt.Printf("%s: %s\n", env, desc)
}
```

## 配置文件示例

### 基础配置文件 (config.yaml)
```yaml
env: dev

server:
  port: 8080
  log_level: debug

postgres:
  host: localhost
  port: 5432
  user: lighthouse_control
  password: "[SECRET]" # 从 PG_PASSWORD 环境变量注入

# ... 其他配置
```

### 环境特定配置 (config.prod.yaml)
```yaml
server:
  log_level: info

security:
  enable_data_encryption: true
  resource_limits:
    cpu_limit: "1"
    memory_limit: "2Gi"
```

## 环境变量

完整的环境变量映射见 [env.go](env.go)，包含100+个配置项，涵盖：

- 服务器配置 (SERVER_*)
- 数据库连接 (PG_*, CH_*)
- 监控配置 (PROMETHEUS_*)
- 业务参数 (COST_*, SLO_*, ROI_*)
- 安全设置 (SECURITY_*)

## 多仓库策略遵循

### lighthouse-src 职责
- 提供配置模板 (`config.example.yaml`)
- 定义配置结构和加载逻辑
- 不包含任何敏感信息

### lighthouse-deploy 职责
- 管理环境特定配置 (`environments/{env}/values.yaml`)
- 通过环境变量注入敏感信息
- 使用Secret Manager管理密钥

## 验证与测试

### 单元测试
```bash
go test ./internal/config/... -v
```

### 测试覆盖率
```bash
go test ./internal/config/... -cover
```

### 安全验证
运行安全验证脚本检查敏感字段处理：
```bash
# 检查敏感字段标记
grep -r 'mapstructure:"-" env:' internal/config/
```

## 最佳实践

1. **开发环境**: 使用 `config.example.yaml` 作为模板
2. **生产环境**: 通过 `lighthouse-deploy` 管理配置
3. **敏感信息**: 永远不要提交到代码仓库
4. **配置变更**: 经过安全审查和测试
5. **环境分离**: 保持dev/staging/prod配置独立

## 扩展指南

### 添加新配置项
1. 在 `config.go` 中添加字段
2. 添加 `mapstructure` 和 `env` 标签
3. 更新 `env.go` 中的映射
4. 更新 `validator.go` 中的验证逻辑
5. 更新 `config.example.yaml` 示例
6. 添加单元测试

### 支持新环境
1. 在 `Environment` 类型中添加常量
2. 更新验证逻辑中的环境特定规则
3. 创建对应的配置文件模板

## 相关文档

- [安全共识文档](../../../lighthouse-doc/03_原子目标与协议/04_多仓库协作与技术规格.md)
- [配置框架设计文档](../../../lighthouse-doc/04_阶段规划与实践/Phase1_物理骨架与领域定义/03_配置框架.md)
- [三级存储架构说明](../../../lighthouse-doc/03_原子目标与协议/04_多仓库协作与技术规格.md#三级存储分离)