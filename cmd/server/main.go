package main

import (
	"context"
	"errors"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	_ "github.com/myxxhui/lighthouse-src/api" // 注册 Swagger docs 供 gin-swagger 使用
	"github.com/myxxhui/lighthouse-src/internal/config"
	"github.com/myxxhui/lighthouse-src/internal/data/cloudbilling"
	"github.com/myxxhui/lighthouse-src/internal/data/postgres"
	"github.com/myxxhui/lighthouse-src/internal/server"
	"github.com/myxxhui/lighthouse-src/internal/server/service"
	"github.com/myxxhui/lighthouse-src/internal/worker/etl"
	"github.com/robfig/cron/v3"
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
	// [Ref: lighthouse-deploy] 统一部署 YAML 打底，环境变量覆盖；见 config/lighthouse_deploy_yaml.go
	deployDoc, errDeployYAML := config.LoadLighthouseDeployYAML("")
	if errDeployYAML != nil {
		log.Printf("WARN: lighthouse deploy yaml load: %v", errDeployYAML)
	}
	if deployDoc != nil {
		config.ApplyLighthouseDeployYAML(cfg, deployDoc)
	}
	// [Ref: 04_Phase4/01_成本透视真实数据] 支持 deploy 侧 POSTGRES_* 与 config 侧 PG_* 一致
	fillPostgresFromEnv(cfg)
	fillCloudBillingFromEnv(cfg)
	fillFinOpsFromEnv(cfg)
	var projectsDoc *config.CloudAccountsProjectsFile
	if deployDoc != nil {
		projectsDoc = &config.CloudAccountsProjectsFile{Version: deployDoc.Version, Projects: deployDoc.Projects}
		config.MergeFinOpsCGSourceFromProjects(projectsDoc, cfg)
	}

	var billingWorkers []*etl.BillingWorker
	var repo postgres.Repository
	if cfg.Postgres.Host != "" {
		pgRepo, err := postgres.NewPGRepository(cfg.Postgres)
		if err != nil {
			log.Printf("WARN: PG repository init failed, using Mock: %v", err)
			repo = postgres.NewMockRepository(postgres.DefaultMockConfig())
		} else {
			repo = pgRepo
			// [Ref: 01_多环境 UAT] [Ref: 03_Phase6 项目云账号] YAML 项目配置优先，否则扁平 ALIBABA_* 环境变量
			if cfg.CloudBilling.Provider == "aliyun" {
				if projectsDoc != nil && len(projectsDoc.Projects) > 0 {
					billingWorkers = buildBillingWorkersFromProjects(repo, cfg, projectsDoc)
				}
				if len(billingWorkers) == 0 {
					billingWorkers = buildBillingWorkers(repo, cfg)
				}
				if len(billingWorkers) > 0 {
					log.Printf("billing ETL schedule (config): %s", cfg.CloudBilling.EffectiveETLScheduleCron())
					// [Ref: 修复] 启动 ETL 改为后台执行，避免阻塞 HTTP 服务；健康检查通过前 ETL 可能未完成，API 返回空或历史数据
					// BILLING_ETL_SKIP_STARTUP=1：仅跳过进程启动时首轮全周期（便于本地/无 zoneinfo 栈快速验收）；定时 cron 仍注册。
					if strings.TrimSpace(os.Getenv("BILLING_ETL_SKIP_STARTUP")) == "1" {
						log.Printf("billing ETL: startup cycle skipped (BILLING_ETL_SKIP_STARTUP=1)")
					} else {
						go func() {
							for _, w := range billingWorkers {
								warns, err := etl.RunFullETLCycle(context.Background(), w, 2*time.Hour)
								for _, msg := range warns {
									log.Printf("WARN: billing ETL: %s", msg)
								}
								if err != nil {
									log.Printf("WARN: billing ETL pipeline: %v", err)
								}
							}
						}()
					}
					go runScheduledBillingETL(billingWorkers, cfg.CloudBilling.EffectiveETLScheduleCron())
				} else {
					log.Printf("billing ETL skipped: CLOUD_BILLING_PROVIDER=aliyun but no AK/SK (set ALIBABA_CLOUD_ACCESS_KEY_ID/SECRET or project-scoped ALIBABA_CLOUD_ACCESS_KEY_ID_<ENVIRONMENT_KEY> per YAML, or POC/UAT short suffix)")
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
	rawCG := os.Getenv("FINOPS_CG_SOURCE")
	effCG := config.EffectiveFinOpsCGSource(cfg.FinOpsCGSource)
	if rawCG != "" {
		low := strings.ToLower(strings.TrimSpace(rawCG))
		if low != "oss" && low != "api" {
			log.Printf("WARN: FINOPS_CG_SOURCE=%q is not oss|api, using oss", rawCG)
		}
	}
	log.Printf("FINOPS_CG_SOURCE default=%s (raw=%q) byEnv=%v", effCG, rawCG, cfg.FinOpsCGSourceByEnv)

	costSvc := service.NewCostService(repo, cfg.FinOpsCGSource, cfg.FinOpsCGSourceByEnv)
	if cloudDisp, err := config.LoadCloudAccountDisplay(""); err != nil {
		log.Printf("WARN: cloud-account-display YAML: %v", err)
	} else if cloudDisp != nil {
		costSvc.SetCloudAccountDisplay(cloudDisp)
		log.Printf("cloud-account-display: loaded %d project(s)", len(cloudDisp.Projects))
	}
	var finOpsAuxSync func(context.Context) error
	if len(billingWorkers) > 0 {
		finOpsAuxSync = func(ctx context.Context) error {
			now := time.Now().UTC()
			var firstErr error
			for _, w := range billingWorkers {
				if e := w.SyncFinOpsAuxiliary(ctx, now); e != nil && firstErr == nil {
					firstErr = e
				}
			}
			return firstErr
		}
		costSvc.SetFinOpsAuxiliarySync(finOpsAuxSync)
	}
	finopsSync := service.NewFinOpsSyncRunner(repo, cfg, billingWorkers, finOpsAuxSync)
	srv := server.NewHTTPServer(cfg, costSvc, finopsSync)
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

// buildBillingWorkers 为每个有 AK/SK 的环境创建 BillingWorker（POC/FAT/UAT/PROD），供多环境并列 ETL。[Ref: 01_多环境 UAT]
func buildBillingWorkers(repo postgres.Repository, cfg *config.Config) []*etl.BillingWorker {
	cycle := cfg.CloudBilling.BillingCycle
	if cycle == "" {
		cycle = time.Now().Format("2006-01")
	}
	etlData := &etl.ETLDataConfig{
		DailyPullMonths:        cfg.CloudBilling.ETLData.DailyPullMonths,
		DailyRetentionMonths:   cfg.CloudBilling.ETLData.DailyRetentionMonths,
		MonthlyPullMonths:      cfg.CloudBilling.ETLData.MonthlyPullMonths,
		MonthlyRetentionMonths: cfg.CloudBilling.ETLData.MonthlyRetentionMonths,
	}
	onFail := func(step string, err error) {
		log.Printf("WARN: billing ETL pipeline failed [%s]: %v", step, err)
	}
	var out []*etl.BillingWorker
	// 先试单账号（无后缀）
	fetcher := cloudbilling.NewFetcher(cloudbilling.CloudBillingConfig{
		Provider:     cfg.CloudBilling.Provider,
		Endpoint:     cfg.CloudBilling.Endpoint,
		BillingCycle: cycle,
		PeriodType:   cfg.CloudBilling.PeriodType,
	})
	if fetcher != nil {
		w := etl.NewBillingWorker(fetcher, repo, cycle)
		w.EnvKey = "POC" // 单账号时沿用 POC 展示（与 cost_env_account_config.environment、OSS AK 后缀一致）
		w.ETLData = etlData
		w.OnPipelineFailAlert = onFail
		out = append(out, w)
		log.Printf("billing ETL: using single-account credentials")
		return out
	}
	// 多账号：按环境后缀收集所有有凭证的环境
	for _, env := range []string{"POC", "FAT", "UAT", "PROD"} {
		f := cloudbilling.NewFetcherForEnv(env)
		if f == nil {
			continue
		}
		w := etl.NewBillingWorker(f, repo, cycle)
		w.EnvKey = env
		w.ETLData = etlData
		w.OnPipelineFailAlert = onFail
		out = append(out, w)
		log.Printf("billing ETL: using credentials for environment %s (ALIBABA_CLOUD_ACCESS_KEY_ID_%s)", env, env)
	}
	return out
}

// buildBillingWorkersFromProjects 从 CLOUD_ACCOUNTS_PROJECTS_YAML 为每个项目×环境创建 Worker。
// environment_key 须与 cost_env_account_config.environment 一致（如 C66_UAT）；AK/SK 可用 ${VAR} 引用环境变量。[Ref: 03_Phase6 项目云账号]
func buildBillingWorkersFromProjects(repo postgres.Repository, cfg *config.Config, doc *config.CloudAccountsProjectsFile) []*etl.BillingWorker {
	cycle := cfg.CloudBilling.BillingCycle
	if cycle == "" {
		cycle = time.Now().Format("2006-01")
	}
	etlData := &etl.ETLDataConfig{
		DailyPullMonths:        cfg.CloudBilling.ETLData.DailyPullMonths,
		DailyRetentionMonths:   cfg.CloudBilling.ETLData.DailyRetentionMonths,
		MonthlyPullMonths:      cfg.CloudBilling.ETLData.MonthlyPullMonths,
		MonthlyRetentionMonths: cfg.CloudBilling.ETLData.MonthlyRetentionMonths,
	}
	onFail := func(step string, err error) {
		log.Printf("WARN: billing ETL pipeline failed [%s]: %v", step, err)
	}
	var out []*etl.BillingWorker
	for _, p := range doc.Projects {
		pid := strings.TrimSpace(p.ID)
		for _, e := range p.Environments {
			ak := strings.TrimSpace(e.AccessKeyID)
			sk := strings.TrimSpace(e.AccessKeySecret)
			if ak == "" || sk == "" {
				log.Printf("billing ETL: skip project env without AK/SK: project=%s name=%s", pid, e.Name)
				continue
			}
			key := strings.TrimSpace(e.EnvironmentKey)
			if key == "" {
				continue
			}
			key = strings.ToUpper(key)
			bss := strings.TrimSpace(e.BSSEndpoint)
			if bss == "" {
				bss = strings.TrimSpace(cfg.CloudBilling.Endpoint)
			}
			if bss == "" {
				bss = strings.TrimSpace(os.Getenv("CLOUD_BILLING_ENDPOINT"))
			}
			f := cloudbilling.NewAliyunFetcherFromCredentials(ak, sk, bss)
			if f == nil {
				continue
			}
			w := etl.NewBillingWorker(f, repo, cycle)
			w.EnvKey = key
			w.ProjectID = pid
			prof := &etl.ProjectCloudProfile{
				ProjectID:       pid,
				EnvironmentKey:  key,
				AccessKeyID:     ak,
				AccessKeySecret: sk,
			}
			if e.OSSBilling != nil {
				prof.OSSBucket = strings.TrimSpace(e.OSSBilling.Bucket)
				prof.OSSPrefix = strings.TrimSpace(e.OSSBilling.Prefix)
				prof.OSSEndpoint = strings.TrimSpace(e.OSSBilling.Endpoint)
			}
			w.ProjectCloudProfile = prof
			w.ETLData = etlData
			w.OnPipelineFailAlert = onFail
			out = append(out, w)
			log.Printf("billing ETL: project cloud worker project=%s env_key=%s (YAML)", pid, key)
		}
	}
	return out
}

// runScheduledBillingETL 使用 ETL_SCHEDULE_CRON（与 config EffectiveETLScheduleCron 一致，默认 0 1 * * * = 每日 UTC 01:00）注册定时任务。[Ref: 04_采集 §七]
func runScheduledBillingETL(workers []*etl.BillingWorker, cronExpr string) {
	if len(workers) == 0 {
		return
	}
	expr := strings.TrimSpace(cronExpr)
	if expr == "" {
		expr = "0 1 * * *"
	}
	c := cron.New(cron.WithLocation(time.UTC))
	job := func() {
		for _, w := range workers {
			warns, err := etl.RunFullETLCycle(context.Background(), w, 2*time.Hour)
			for _, msg := range warns {
				log.Printf("WARN: billing ETL: %s", msg)
			}
			if err != nil {
				log.Printf("WARN: billing ETL pipeline: %v", err)
			}
		}
	}
	if _, err := c.AddFunc(expr, job); err != nil {
		log.Printf("WARN: invalid ETL_SCHEDULE_CRON %q, using default 0 1 * * * (UTC): %v", expr, err)
		expr = "0 1 * * *"
		if _, err2 := c.AddFunc(expr, job); err2 != nil {
			log.Printf("FATAL: billing ETL cron register failed: %v", err2)
			return
		}
	}
	log.Printf("billing ETL: cron registered (UTC): %s", expr)
	c.Start()
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
	// [Ref: 01_实践 配置控制拉取与保存长度] 日/月拉取与保留月数可由环境变量覆盖
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
	if v := os.Getenv("ETL_SCHEDULE_CRON"); v != "" {
		cfg.CloudBilling.ETLScheduleCron = v
	}
}

// fillFinOpsFromEnv 五维 C/G：FINOPS_CG_SOURCE 默认 + 任意 FINOPS_CG_SOURCE_<ENV> 按环境覆盖（与 cost_env_account_config.environment 一致）。[Ref: 03_Phase6/01_FinOps]
func fillFinOpsFromEnv(cfg *config.Config) {
	if v := os.Getenv("FINOPS_CG_SOURCE"); v != "" {
		cfg.FinOpsCGSource = v
	}
	config.MergeFinOpsCGFromEnviron(cfg)
	if v := strings.TrimSpace(os.Getenv("FINOPS_SYNC_AUX_TIMEOUT")); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			cfg.FinOpsSyncAuxTimeout = d
		} else {
			log.Printf("WARN: FINOPS_SYNC_AUX_TIMEOUT=%q invalid, using default 30m: %v", v, err)
		}
	}
	if v := strings.TrimSpace(os.Getenv("FINOPS_SYNC_JOB_API_KEY")); v != "" {
		cfg.FinOpsSyncJobAPIKey = v
	}
}
