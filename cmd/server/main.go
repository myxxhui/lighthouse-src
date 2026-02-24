package main

import (
	"context"
	"errors"
	"log"
	"os"
	"strconv"
	"time"

	_ "github.com/myxxhui/lighthouse-src/api" // 注册 Swagger docs 供 gin-swagger 使用
	"github.com/myxxhui/lighthouse-src/internal/config"
	"github.com/myxxhui/lighthouse-src/internal/data/cloudbilling"
	"github.com/myxxhui/lighthouse-src/internal/data/postgres"
	"github.com/myxxhui/lighthouse-src/internal/server"
	"github.com/myxxhui/lighthouse-src/internal/server/service"
	"github.com/myxxhui/lighthouse-src/internal/worker/etl"
)

// Version, GitCommit, BuildTime 由构建时 ldflags 注入 [Ref: 04_Phase4/01_成本透视真实数据]
var (
	Version   string
	GitCommit string
	BuildTime string
)

func main() {
	// Lighthouse Server - Infrastructure Decision Cockpit (Phase3 Mock; Phase4 01_ 成本透视真实数据)
	cfg, err := loadConfig()
	if err != nil {
		log.Printf("WARN: config load failed, using defaults: %v", err)
		cfg = defaultConfig()
	}
	// [Ref: 04_Phase4/01_成本透视真实数据] 支持 deploy 侧 POSTGRES_* 与 config 侧 PG_* 一致
	fillPostgresFromEnv(cfg)
	fillCloudBillingFromEnv(cfg)

	var repo postgres.Repository
	if cfg.Postgres.Host != "" {
		pgRepo, err := postgres.NewPGRepository(cfg.Postgres)
		if err != nil {
			log.Printf("WARN: PG repository init failed, using Mock: %v", err)
			repo = postgres.NewMockRepository(postgres.DefaultMockConfig())
		} else {
			repo = pgRepo
			// 启动时执行一次云账单 ETL（PG + 云账单凭证已配置时）
			if cfg.CloudBilling.Provider == "aliyun" && os.Getenv("ALIBABA_CLOUD_ACCESS_KEY_ID") != "" && os.Getenv("ALIBABA_CLOUD_ACCESS_KEY_SECRET") != "" {
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
				ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
				if err := worker.Run(ctx); err != nil {
					log.Printf("WARN: billing ETL run failed: %v", err)
				}
				cancel()
			}
		}
	} else {
		repo = postgres.NewMockRepository(postgres.DefaultMockConfig())
		if os.Getenv("SEED_CLOUD_BILL") == "1" {
			// [Ref: 04_Phase4/01_成本透视真实数据] 无 PG 时可选：模拟真实结构数据，使 GET /api/v1/cost/global 返回 total_cost、domain_breakdown
			day := time.Now().UTC().Truncate(24 * time.Hour)
			_ = repo.SaveCloudBillSummary(context.Background(), postgres.CloudBillSummary{
				Day:              day,
				BillingCycle:     time.Now().Format("2006-01"),
				TotalAmount:      125000,
				ProductBreakdown: map[string]float64{"计算资源": 85000, "存储": 25000, "网络": 15000},
				CreatedAt:        time.Now(),
				UpdatedAt:        time.Now(),
			})
		}
	}
	costSvc := service.NewCostService(repo)

	srv := server.NewHTTPServer(cfg, costSvc)
	if err := srv.StartWithGracefulShutdown(); err != nil {
		log.Fatal(err)
	}
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
	return &config.Config{
		Env: config.EnvDevelopment,
		Server: config.ServerConfig{
			Port:         8080,
			ReadTimeout:  30000000000,  // 30s
			WriteTimeout: 30000000000,  // 30s
			LogLevel:     "debug",
			MaxConn:      100,
			GracePeriod:  30000000000,  // 30s
		},
	}
}

// fillPostgresFromEnv 从环境变量填充 Postgres 配置，兼容 POSTGRES_*（deploy）与 PG_*（config）。
// [Ref: 04_Phase4/01_成本透视真实数据 T2.1]
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

// fillCloudBillingFromEnv 从环境变量填充云账单配置（如 CLOUD_BILLING_PROVIDER）。
// [Ref: 04_Phase4/01_成本透视真实数据] 国际站阿里云需设 CLOUD_BILLING_ENDPOINT=business.ap-southeast-1.aliyuncs.com
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
}
