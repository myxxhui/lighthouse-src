# ETL Worker

小时级/日级 ETL 占位目录，符合 04_ 多仓库协作 §2.2.1。

- **hourly_worker.go**: 小时级成本/工作负载写入 `cost_hourly_workload`（Phase2 实现）。
- **daily_worker.go**: 日级命名空间成本写入 `cost_daily_namespace`（Phase2 实现）。
- **scheduler.go**: 调度器占位（Phase2 实现）。

表名与 06_ 存储架构与ETL规范 一致。
