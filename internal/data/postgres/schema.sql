-- PostgreSQL 控制平面 Schema 占位（与 06_存储架构与ETL规范 一致）
-- 成本域
-- cost_daily_namespace: 每日命名空间账单
CREATE TABLE IF NOT EXISTS cost_daily_namespace (
    day             DATE NOT NULL,
    namespace       VARCHAR(64) NOT NULL,
    billable_cost   DECIMAL(10, 2),
    usage_cost      DECIMAL(10, 2),
    waste_cost      DECIMAL(10, 2),
    efficiency      DECIMAL(5, 2),
    pod_count       INT,
    zombie_count    INT,
    PRIMARY KEY (day, namespace)
);

-- cost_hourly_workload: 工作负载小时级趋势
CREATE TABLE IF NOT EXISTS cost_hourly_workload (
    time_bucket     TIMESTAMP NOT NULL,
    namespace       VARCHAR(64),
    workload_name   VARCHAR(128),
    workload_kind   VARCHAR(32),
    request_cores   DECIMAL(10, 4),
    limit_cores     DECIMAL(10, 4),
    max_cpu_usage   DECIMAL(10, 4),
    p95_cpu_usage   DECIMAL(10, 4),
    avg_cpu_usage   DECIMAL(10, 4),
    PRIMARY KEY (time_bucket, namespace, workload_name)
);

-- cost_roi_events: 优化动作流水
CREATE TABLE IF NOT EXISTS cost_roi_events (
    id              SERIAL PRIMARY KEY,
    event_time      TIMESTAMP DEFAULT NOW(),
    namespace       VARCHAR(64),
    service_name    VARCHAR(128),
    event_type      VARCHAR(32),
    savings_amount  DECIMAL(10, 2),
    description     TEXT
);
