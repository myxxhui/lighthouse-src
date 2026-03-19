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

-- [Ref: 16_云账单动态对账与高可靠处理规范 §三] 行级流水明细表（幂等写入，含负数冲正条目）
CREATE TABLE IF NOT EXISTS cost_cloud_bill_line_items (
    record_id           VARCHAR(128) NOT NULL,
    bill_date           DATE NOT NULL,
    billing_cycle       VARCHAR(32) NOT NULL,
    product_code        VARCHAR(64),
    product_name        VARCHAR(128),
    sub_order_id        VARCHAR(128),
    instance_id         VARCHAR(128),
    billing_item        VARCHAR(128),
    subscription_type   VARCHAR(32),
    cash_amount         NUMERIC(14, 6) NOT NULL,
    pretax_amount       NUMERIC(14, 6),
    pretax_gross_amount NUMERIC(14, 6),
    currency            VARCHAR(8) DEFAULT 'CNY',
    is_reversal         BOOLEAN NOT NULL DEFAULT FALSE,
    account_id          VARCHAR(64) NOT NULL DEFAULT '',
    region              VARCHAR(32),
    raw_payload         JSONB,
    synced_at           TIMESTAMP NOT NULL DEFAULT NOW(),
    created_at          TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMP NOT NULL DEFAULT NOW(),
    PRIMARY KEY (record_id)
);
CREATE INDEX IF NOT EXISTS idx_line_items_bill_date     ON cost_cloud_bill_line_items(bill_date, account_id);
CREATE INDEX IF NOT EXISTS idx_line_items_billing_cycle ON cost_cloud_bill_line_items(billing_cycle, account_id);
CREATE INDEX IF NOT EXISTS idx_line_items_reversal      ON cost_cloud_bill_line_items(bill_date) WHERE is_reversal = TRUE;

-- [Ref: 16_云账单动态对账与高可靠处理规范 §三] 月度对账状态追踪表
CREATE TABLE IF NOT EXISTS cost_cloud_bill_month_status (
    billing_cycle       VARCHAR(32) NOT NULL,
    account_id          VARCHAR(64) NOT NULL DEFAULT '',
    data_status         VARCHAR(32) NOT NULL DEFAULT 'PRELIMINARY',
    line_items_sum      NUMERIC(14, 2),
    monthly_api_total   NUMERIC(14, 2),
    drift_amount        NUMERIC(14, 2),
    last_reconciled_at  TIMESTAMP,
    last_full_sync_at   TIMESTAMP,
    finalized_at        TIMESTAMP,
    notes               TEXT,
    created_at          TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMP NOT NULL DEFAULT NOW(),
    PRIMARY KEY (billing_cycle, account_id)
);

-- [Ref: 06_ 成本云账单三表] 月/日原始表主键含 account_id，多环境各写一行 [Ref: 01_多环境 UAT]
CREATE TABLE IF NOT EXISTS cost_cloud_bill_monthly_raw (
    billing_cycle   VARCHAR(32) NOT NULL,
    total_amount    DECIMAL(12, 2) NOT NULL,
    product_breakdown JSONB NOT NULL,
    snapshot_at     TIMESTAMP DEFAULT NOW(),
    created_at      TIMESTAMP DEFAULT NOW(),
    account_id      VARCHAR(64) NOT NULL DEFAULT '',
    region          VARCHAR(32),
    PRIMARY KEY (billing_cycle, account_id)
);
CREATE TABLE IF NOT EXISTS cost_cloud_bill_daily_raw (
    bill_date       DATE NOT NULL,
    total_amount    DECIMAL(12, 2) NOT NULL,
    product_breakdown JSONB NOT NULL,
    snapshot_at     TIMESTAMP DEFAULT NOW(),
    created_at      TIMESTAMP DEFAULT NOW(),
    account_id      VARCHAR(64) NOT NULL DEFAULT '',
    region          VARCHAR(32),
    PRIMARY KEY (bill_date, account_id)
);
-- [Ref: 01_设计 D9-5] 聚合表主键 (report_type, period_key, account_id)；含 data_status 供 API 透传
CREATE TABLE IF NOT EXISTS cost_cloud_bill_aggregate (
    report_type     VARCHAR(16) NOT NULL,
    period_key      VARCHAR(32) NOT NULL,
    account_id      VARCHAR(64) NOT NULL DEFAULT '',
    total_amount    DECIMAL(12, 2) NOT NULL,
    product_breakdown JSONB,
    data_status     VARCHAR(32) NOT NULL DEFAULT 'PRELIMINARY',
    last_success_at TIMESTAMP,
    created_at      TIMESTAMP DEFAULT NOW(),
    updated_at      TIMESTAMP DEFAULT NOW(),
    region          VARCHAR(32),
    PRIMARY KEY (report_type, period_key, account_id)
);
CREATE INDEX IF NOT EXISTS idx_cloud_bill_aggregate_period ON cost_cloud_bill_aggregate(report_type, period_key);

-- [Ref: 01_设计 §环境与云账号配置、06_ §2.1] 环境与云账号映射（POC/FAT/UAT/PROD）
CREATE TABLE IF NOT EXISTS cost_env_account_config (
    id              SERIAL PRIMARY KEY,
    environment     VARCHAR(16) NOT NULL,
    account_id      VARCHAR(64) NOT NULL,
    display_name    VARCHAR(128),
    sort_order      INT DEFAULT 0,
    created_at      TIMESTAMP DEFAULT NOW()
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_env_account ON cost_env_account_config(environment);
-- [Ref: 01_设计 §环境与云账号配置] 单账号时至少一条映射，供按环境总账展示；多账号可后续插入 FAT/UAT/PROD
INSERT INTO cost_env_account_config (environment, account_id, display_name, sort_order) VALUES ('POC', 'POC', 'POC 演示账号', 1) ON CONFLICT (environment) DO NOTHING;

-- [Ref: 01_设计 §产品分类与按环境钻取、06_ §2.1] 云产品与成本分类映射
CREATE TABLE IF NOT EXISTS product_category_mapping (
    id              SERIAL PRIMARY KEY,
    product_code    VARCHAR(64) NOT NULL,
    category        VARCHAR(16) NOT NULL,
    created_at      TIMESTAMP DEFAULT NOW()
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_product_category ON product_category_mapping(product_code);
