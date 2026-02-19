//go:build integration

package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/myxxhui/lighthouse-src/internal/config"
	"github.com/myxxhui/lighthouse-src/internal/data/postgres"
	"github.com/myxxhui/lighthouse-src/internal/server/service"
	"github.com/stretchr/testify/assert"
)

// Integration test: Cost API with Mock data - L0 performance and data consistency

func TestIntegration_CostGlobal_L0Performance(t *testing.T) {
	mockRepo := postgres.NewMockRepository(postgres.DefaultMockConfig())
	costSvc := service.NewCostService(mockRepo)
	cfg := &config.Config{
		Env: config.EnvDevelopment,
		Server: config.ServerConfig{
			Port:         8080,
			ReadTimeout:  30 * time.Second,
			WriteTimeout: 30 * time.Second,
		},
	}
	srv := NewHTTPServer(cfg, costSvc)
	engine := srv.Engine()

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/cost/global", nil)
	start := time.Now()
	engine.ServeHTTP(w, req)
	elapsed := time.Since(start)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Less(t, elapsed.Milliseconds(), int64(10), "L0 GET /api/v1/cost/global must respond in <10ms")
}

func TestIntegration_CostGlobal_L0EqualsL1(t *testing.T) {
	mockRepo := postgres.NewMockRepository(postgres.DefaultMockConfig())
	costSvc := service.NewCostService(mockRepo)
	cfg := &config.Config{
		Env: config.EnvDevelopment,
		Server: config.ServerConfig{
			Port:         8080,
			ReadTimeout:  30 * time.Second,
			WriteTimeout: 30 * time.Second,
		},
	}
	srv := NewHTTPServer(cfg, costSvc)
	engine := srv.Engine()

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/cost/global", nil)
	engine.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp struct {
		TotalCost  float64 `json:"total_cost"`
		Namespaces []struct {
			Name string  `json:"name"`
			Cost float64 `json:"cost"`
		} `json:"namespaces"`
	}
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	assert.NoError(t, err)

	var sumL1 float64
	for _, ns := range resp.Namespaces {
		sumL1 += ns.Cost
	}
	assert.InDelta(t, resp.TotalCost, sumL1, 0.01, "L0 100%% = sum of L1")
}
