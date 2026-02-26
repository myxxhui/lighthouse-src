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
			// 启动时执行一次云账单 ETL（PG + 云账单凭证已配置时）。[Ref: D2-5] 单账号用无后缀 AK/SK，多账号可用带环境后缀（POC/FAT/UAT/PROD）任一组。
			if cfg.CloudBilling.Provider == "aliyun" {
				var billingEnv string
				fetcher := cloudbilling.NewFetcher(cloudbilling.CloudBillingConfig{
					Provider:     cfg.CloudBilling.Provider,
					Endpoint:     cfg.CloudBilling.Endpoint,
					BillingCycle: cfg.CloudBilling.BillingCycle,
					PeriodType:   cfg.CloudBilling.PeriodType,
				})
				if fetcher == nil {
					// 无后缀时尝试任一带环境后缀的凭证（01_实践 §3.3(3a)）
					for _, env := range []string{"POC", "FAT", "UAT", "PROD"} {
						fetcher = cloudbilling.NewFetcherForEnv(env)
						if fetcher != nil {
							billingEnv = env
							log.Printf("billing ETL: using credentials for environment %s (ALIBABA_CLOUD_ACCESS_KEY_ID_%s)", env, env)
							break
						}
					}
				}
				if fetcher != nil {
					log.Printf("billing ETL schedule (config): %s", cfg.CloudBilling.EffectiveETLScheduleCron())
					cycle := cfg.CloudBilling.BillingCycle
					if cycle == "" {
						cycle = time.Now().Format("2006-01")
					}
					worker := etl.NewBillingWorker(fetcher, repo, cycle)
					worker.AccountID = billingEnv // account_id 取值为 POC、FAT、UAT、PROD 四者之一，与 cost_env_account_config.account_id 一致，供按环境总账展示 [Ref: 01_设计 §环境与云账号配置]
					worker.OnPipelineFailAlert = func(step string, err error) {
						log.Printf("WARN: billing ETL pipeline failed [%s]: %v", step, err)
						// D1-1：与 04_ 监控告警集成时在此触发告警
					}
					// [Ref: 01_实践 部署与每日凌晨全量检查] 每次部署：检查全量数据是否按预期存在，否则自动全量拉取+聚合；符合则仅增量+按规则删除周期外数据
					runBillingETLCycle(context.Background(), worker, 2*time.Hour)
					// 每日凌晨 1 点后执行全量检查；不符合则全量更新，符合则仅增量更新并删除周期外数据
					go runNightlyBillingETL(worker, &cfg.CloudBilling)
				} else {
					log.Printf("billing ETL skipped: CLOUD_BILLING_PROVIDER=aliyun but no AK/SK (set ALIBABA_CLOUD_ACCESS_KEY_ID/SECRET or ALIBABA_CLOUD_ACCESS_KEY_ID_POC/SECRET_POC etc.)")
				}
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

// runBillingETLCycle 执行一次账单 ETL 周期：全量检查 → 不满足则全量回填，满足则仅增量；再执行流水线（写昨日→校验→删周期外→写月→聚合）与对账。[Ref: 01_实践 部署与每日凌晨全量检查]
func runBillingETLCycle(ctx context.Context, worker *etl.BillingWorker, maxDuration time.Duration) {
	ctx, cancel := context.WithTimeout(ctx, maxDuration)
	defer cancel()
	if err := worker.Run(ctx); err != nil {
		log.Printf("WARN: billing ETL run failed: %v", err)
		if worker.OnPipelineFailAlert != nil {
			worker.OnPipelineFailAlert("run", err)
		}
	}
	needFull, err := worker.NeedsFullBackfill(ctx)
	if err != nil {
		log.Printf("WARN: billing full check failed, will run full backfill: %v", err)
		needFull = true
	}
	if needFull {
		log.Printf("billing ETL: full data check failed, running full backfill (10 months daily + 5 years monthly)")
		if err := worker.RunFullBackfill(ctx); err != nil {
			log.Printf("WARN: billing full backfill failed: %v", err)
			if worker.OnPipelineFailAlert != nil {
				worker.OnPipelineFailAlert("full_backfill", err)
			}
		}
	}
	if err := worker.RunPipeline(ctx); err != nil {
		log.Printf("WARN: billing ETL pipeline run failed: %v", err)
		if worker.OnPipelineFailAlert != nil {
			worker.OnPipelineFailAlert("pipeline", err)
		}
	}
	if err := worker.RunReconcile(ctx); err != nil {
		log.Printf("WARN: billing reconcile failed: %v", err)
	}
}

// nextDurationTo1AMUTC 返回当前时间到下一次 01:00 UTC 的时长；若已过今日 01:00 则为明日 01:00。[Ref: 01_实践 每日凌晨 1 点全量检查]
func nextDurationTo1AMUTC() time.Duration {
	now := time.Now().UTC()
	next := time.Date(now.Year(), now.Month(), now.Day(), 1, 0, 0, 0, time.UTC)
	if !now.Before(next) {
		next = next.AddDate(0, 0, 1)
	}
	return next.Sub(now)
}

// runNightlyBillingETL 每日凌晨 1 点（UTC）后执行全量检查：不符合则全量更新，符合则仅增量更新并按规则删除周期外数据。
func runNightlyBillingETL(worker *etl.BillingWorker, _ *config.CloudBillingConfig) {
	for {
		d := nextDurationTo1AMUTC()
		log.Printf("billing ETL nightly: next run in %v (after 01:00 UTC)", d.Round(time.Second))
		time.Sleep(d)
		runBillingETLCycle(context.Background(), worker, 2*time.Hour)
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
