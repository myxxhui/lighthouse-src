# Lighthouse (基础设施经营决策驾驶舱)

Lighthouse 核心代码库，Monorepo 模式。规范见 `lighthouse-doc` 仓库 §04_多仓库协作与技术规格。

## 目录职责

| 路径 | 职责 |
|------|------|
| `api/` | API 规范文档 (OpenAPI/Swagger) |
| `cmd/server/` | 主服务入口 |
| `internal/biz/cost` | 成本计算领域 |
| `internal/biz/slo` | SLO 诊断领域 |
| `internal/biz/roi` | ROI 追踪领域 |
| `internal/config` | 配置管理 |
| `internal/data/` | 数据访问 (k8s, postgres, prometheus) |
| `internal/server/` | HTTP 服务 (dto, middleware, routes) |
| `testdata/` | 测试数据 |
| `web/` | 前端应用 (React) |

## 构建与验证

```bash
make help    # 查看可用命令
make         # 默认显示帮助
go build ./cmd/server
go build ./...
make clean
```

## 下一步

完成骨架搭建后，进入 **步骤02: 领域建模**。
