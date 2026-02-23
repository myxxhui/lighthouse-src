package main

import (
	"context"
	"errors"
	"log"
	"os"
	"time"

	_ "github.com/myxxhui/lighthouse-src/api" // 注册 Swagger docs 供 gin-swagger 使用
	"github.com/myxxhui/lighthouse-src/internal/config"
	"github.com/myxxhui/lighthouse-src/internal/data/postgres"
	"github.com/myxxhui/lighthouse-src/internal/server"
	"github.com/myxxhui/lighthouse-src/internal/server/service"
)

func main() {
	// Lighthouse Server - Infrastructure Decision Cockpit (Phase3 Mock; Phase4 01_ 成本透视真实数据)
	cfg, err := loadConfig()
	if err != nil {
		log.Printf("WARN: config load failed, using defaults: %v", err)
		cfg = defaultConfig()
	}

	// Mock data layer (Phase3)；Phase4 01_ 云账单路径由 cost_cloud_bill_summary 或本 Mock 注入提供
	mockRepo := postgres.NewMockRepository(postgres.DefaultMockConfig())
	if os.Getenv("SEED_CLOUD_BILL") == "1" {
		// [Ref: 04_Phase4/01_成本透视真实数据] 本地前后端全系统测试：模拟真实结构数据，使 GET /api/v1/cost/global 返回 total_cost、domain_breakdown
		day := time.Now().UTC().Truncate(24 * time.Hour)
		_ = mockRepo.SaveCloudBillSummary(context.Background(), postgres.CloudBillSummary{
			Day:          day,
			BillingCycle: time.Now().Format("2006-01"),
			TotalAmount:  125000,
			ProductBreakdown: map[string]float64{
				"计算资源": 85000,
				"存储":    25000,
				"网络":    15000,
			},
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		})
	}
	costSvc := service.NewCostService(mockRepo)

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
