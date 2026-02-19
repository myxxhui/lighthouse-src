package main

import (
	"errors"
	"log"

	_ "github.com/myxxhui/lighthouse-src/api" // 注册 Swagger docs 供 gin-swagger 使用
	"github.com/myxxhui/lighthouse-src/internal/config"
	"github.com/myxxhui/lighthouse-src/internal/data/postgres"
	"github.com/myxxhui/lighthouse-src/internal/server"
	"github.com/myxxhui/lighthouse-src/internal/server/service"
)

func main() {
	// Lighthouse Server - Infrastructure Decision Cockpit (Phase3 Mock)
	cfg, err := loadConfig()
	if err != nil {
		log.Printf("WARN: config load failed, using defaults: %v", err)
		cfg = defaultConfig()
	}

	// Mock data layer (Phase3)
	mockRepo := postgres.NewMockRepository(postgres.DefaultMockConfig())
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
