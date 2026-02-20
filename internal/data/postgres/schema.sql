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

-- cost_bill_account_summary: 云账户总账单汇总（与 05_ 设计 4.0 一致）
-- 供周期对比与总账单→计算资源层级使用；AKSK 仅环境变量/Secret，不在此表
CREATE TABLE IF NOT EXISTS cost_bill_account_summary (
    account_id      VARCHAR(64) NOT NULL,
    period_type     VARCHAR(32) NOT NULL,
    period_start    DATE NOT NULL,
    period_end      DATE NOT NULL,
    total_amount    DECIMAL(12, 2),
    currency        VARCHAR(8) DEFAULT 'CNY',
    by_category     JSONB,
    created_at      TIMESTAMP DEFAULT NOW(),
    PRIMARY KEY (account_id, period_type, period_start)
);

-- cost_daily_storage: 存储维度钻取
CREATE TABLE IF NOT EXISTS cost_daily_storage (
    day             DATE NOT NULL,
    namespace       VARCHAR(64) NOT NULL,
    storage_class   VARCHAR(64),
    pvc_name        VARCHAR(256),
    cost            DECIMAL(10, 2),
    created_at      TIMESTAMP DEFAULT NOW(),
    PRIMARY KEY (day, namespace, pvc_name)
);

-- cost_daily_network: 网络维度钻取
CREATE TABLE IF NOT EXISTS cost_daily_network (
    day             DATE NOT NULL,
    namespace       VARCHAR(64) NOT NULL,
    resource_type   VARCHAR(64),
    resource_id     VARCHAR(256),
    cost            DECIMAL(10, 2),
    created_at      TIMESTAMP DEFAULT NOW(),
    PRIMARY KEY (day, namespace, resource_id)
);
