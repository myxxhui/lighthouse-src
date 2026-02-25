-- Minimal schema for 01_ integration test: cost_cloud_bill_summary + cost_daily_namespace (06_ §2.1)
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
CREATE TABLE IF NOT EXISTS cost_cloud_bill_summary (
    day             DATE NOT NULL,
    billing_cycle   VARCHAR(32),
    total_amount    DECIMAL(12, 2) NOT NULL,
    product_breakdown JSONB,
    created_at     TIMESTAMP DEFAULT NOW(),
    updated_at     TIMESTAMP DEFAULT NOW(),
    PRIMARY KEY (day, billing_cycle)
);
CREATE TABLE IF NOT EXISTS cost_cloud_bill_monthly_raw (
    billing_cycle   VARCHAR(32) NOT NULL,
    total_amount    DECIMAL(12, 2) NOT NULL,
    product_breakdown JSONB NOT NULL,
    snapshot_at     TIMESTAMP DEFAULT NOW(),
    created_at      TIMESTAMP DEFAULT NOW(),
    PRIMARY KEY (billing_cycle)
);
CREATE TABLE IF NOT EXISTS cost_cloud_bill_daily_raw (
    bill_date       DATE NOT NULL,
    total_amount    DECIMAL(12, 2) NOT NULL,
    product_breakdown JSONB NOT NULL,
    snapshot_at     TIMESTAMP DEFAULT NOW(),
    created_at      TIMESTAMP DEFAULT NOW(),
    PRIMARY KEY (bill_date)
);
CREATE TABLE IF NOT EXISTS cost_cloud_bill_aggregate (
    report_type     VARCHAR(16) NOT NULL,
    period_key      VARCHAR(32) NOT NULL,
    total_amount    DECIMAL(12, 2) NOT NULL,
    product_breakdown JSONB,
    last_success_at TIMESTAMP,
    created_at      TIMESTAMP DEFAULT NOW(),
    updated_at      TIMESTAMP DEFAULT NOW(),
    account_id      VARCHAR(64),
    region          VARCHAR(32),
    PRIMARY KEY (report_type, period_key)
);
CREATE INDEX IF NOT EXISTS idx_cloud_bill_aggregate_period ON cost_cloud_bill_aggregate(report_type, period_key);
CREATE TABLE IF NOT EXISTS cost_env_account_config (
    id SERIAL PRIMARY KEY, environment VARCHAR(16) NOT NULL, account_id VARCHAR(64) NOT NULL,
    display_name VARCHAR(128), sort_order INT DEFAULT 0, created_at TIMESTAMP DEFAULT NOW());
CREATE UNIQUE INDEX IF NOT EXISTS idx_env_account ON cost_env_account_config(environment);
CREATE TABLE IF NOT EXISTS product_category_mapping (
    id SERIAL PRIMARY KEY, product_code VARCHAR(64) NOT NULL, category VARCHAR(16) NOT NULL, created_at TIMESTAMP DEFAULT NOW());
CREATE UNIQUE INDEX IF NOT EXISTS idx_product_category ON product_category_mapping(product_code);
