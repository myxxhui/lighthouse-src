// [Ref: 成本透视实践优化 D2-6] 首次或按需全量回填：拉取 10 个月日数据 + 5 年月数据，落库后执行一次聚合。常规应用启动/部署不执行；通过本命令或独立 Job 触发。
package main

import (
	"context"
	"errors"
	"log"
	"os"
	"strconv"
	"time"

	"github.com/myxxhui/lighthouse-src/internal/config"
	"github.com/myxxhui/lighthouse-src/internal/data/cloudbilling"
	"github.com/myxxhui/lighthouse-src/internal/data/postgres"
	"github.com/myxxhui/lighthouse-src/internal/worker/etl"
)

func main() {
	cfg, err := loadConfig()
	if err != nil {
		log.Printf("WARN: config load failed, using defaults: %v", err)
		cfg = defaultConfig()
	}
	fillPostgresFromEnv(cfg)
	fillCloudBillingFromEnv(cfg)

	if cfg.Postgres.Host == "" {
		log.Fatal("billing-backfill: PG_HOST/POSTGRES_HOST required")
	}
	if cfg.CloudBilling.Provider != "aliyun" || os.Getenv("ALIBABA_CLOUD_ACCESS_KEY_ID") == "" || os.Getenv("ALIBABA_CLOUD_ACCESS_KEY_SECRET") == "" {
		log.Fatal("billing-backfill: CLOUD_BILLING_PROVIDER=aliyun and ALIBABA_CLOUD_ACCESS_KEY_ID/SECRET required")
	}

	repo, err := postgres.NewPGRepository(cfg.Postgres)
	if err != nil {
		log.Fatalf("billing-backfill: PG init failed: %v", err)
	}

	fetcher := cloudbilling.NewFetcher(cloudbilling.CloudBillingConfig{
		Provider:     cfg.CloudBilling.Provider,
		Endpoint:     cfg.CloudBilling.Endpoint,
		BillingCycle: cfg.CloudBilling.BillingCycle,
		PeriodType:   cfg.CloudBilling.PeriodType,
	})
	cycle := cfg.CloudBilling.BillingCycle
	if cycle == "" {
		cycle = time.Now().Format("2006-01")
	}
	worker := etl.NewBillingWorker(fetcher, repo, cycle)
	worker.ETLData = &etl.ETLDataConfig{
		DailyPullMonths:        cfg.CloudBilling.ETLData.DailyPullMonths,
		DailyRetentionMonths:   cfg.CloudBilling.ETLData.DailyRetentionMonths,
		MonthlyPullMonths:     cfg.CloudBilling.ETLData.MonthlyPullMonths,
		MonthlyRetentionMonths: cfg.CloudBilling.ETLData.MonthlyRetentionMonths,
	}
	worker.OnPipelineFailAlert = func(step string, err error) {
		log.Printf("WARN: billing backfill [%s]: %v", step, err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Hour)
	defer cancel()
	if err := worker.RunFullBackfill(ctx); err != nil {
		log.Fatalf("billing-backfill failed: %v", err)
	}
	log.Println("billing-backfill done")
}

func loadConfig() (*config.Config, error) {
	for _, p := range []string{"./configs", "../configs", ".", "internal/config"} {
		loader := config.NewFileLoader(p)
		if cfg, err := loader.Load(); err == nil {
			return cfg, nil
		}
	}
	return nil, errors.New("no config file found")
}

func defaultConfig() *config.Config {
	return &config.Config{}
}

func fillPostgresFromEnv(cfg *config.Config) {
	get := func(keys ...string) string {
		for _, k := range keys {
			if v := os.Getenv(k); v != "" {
				return v
			}
		}
		return ""
	}
	if v := get("PG_HOST", "POSTGRES_HOST"); v != "" {
		cfg.Postgres.Host = v
	}
	if v := get("PG_PORT", "POSTGRES_PORT"); v != "" {
		if p, err := strconv.Atoi(v); err == nil {
			cfg.Postgres.Port = p
		}
	}
	if v := get("PG_USER", "POSTGRES_USER"); v != "" {
		cfg.Postgres.User = v
	}
	if v := get("PG_PASSWORD", "POSTGRES_PASSWORD"); v != "" {
		cfg.Postgres.Password = v
	}
	if v := get("PG_DATABASE", "POSTGRES_DB"); v != "" {
		cfg.Postgres.Database = v
	}
	if v := get("PG_SSL_MODE"); v != "" {
		cfg.Postgres.SSLMode = v
	}
}

func fillCloudBillingFromEnv(cfg *config.Config) {
	if v := os.Getenv("CLOUD_BILLING_PROVIDER"); v != "" {
		cfg.CloudBilling.Provider = v
	}
	if v := os.Getenv("CLOUD_BILLING_ENDPOINT"); v != "" {
		cfg.CloudBilling.Endpoint = v
	}
	if v := os.Getenv("CLOUD_BILLING_CYCLE"); v != "" {
		cfg.CloudBilling.BillingCycle = v
	}
	if v := os.Getenv("CLOUD_BILLING_PERIOD"); v != "" {
		cfg.CloudBilling.PeriodType = v
	}
	if v := os.Getenv("BILLING_DAILY_PULL_MONTHS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			cfg.CloudBilling.ETLData.DailyPullMonths = n
		}
	}
	if v := os.Getenv("BILLING_DAILY_RETENTION_MONTHS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			cfg.CloudBilling.ETLData.DailyRetentionMonths = n
		}
	}
	if v := os.Getenv("BILLING_MONTHLY_PULL_MONTHS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			cfg.CloudBilling.ETLData.MonthlyPullMonths = n
		}
	}
	if v := os.Getenv("BILLING_MONTHLY_RETENTION_MONTHS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			cfg.CloudBilling.ETLData.MonthlyRetentionMonths = n
		}
	}
}
