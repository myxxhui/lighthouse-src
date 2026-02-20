// Package server provides HTTP server implementation for Lighthouse API.
package server

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/myxxhui/lighthouse-src/internal/config"
	"github.com/myxxhui/lighthouse-src/internal/server/middleware"
	"github.com/myxxhui/lighthouse-src/internal/server/service"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
)

// HTTPServer encapsulates the HTTP server with Gin engine and configuration.
type HTTPServer struct {
	config      *config.Config
	engine      *gin.Engine
	server      *http.Server
	costService *service.CostService
}

// NewHTTPServer creates a new HTTP server instance. Uses Mock data if costService is nil.
func NewHTTPServer(cfg *config.Config, costService *service.CostService) *HTTPServer {
	// Set Gin mode based on environment
	if cfg.Env == config.EnvProduction {
		gin.SetMode(gin.ReleaseMode)
	} else {
		gin.SetMode(gin.DebugMode)
	}

	engine := gin.New()

	// Apply global middleware
	engine.Use(middleware.RequestID())
	engine.Use(middleware.Logger())
	engine.Use(middleware.Recovery())
	engine.Use(middleware.CORS())

	srv := &HTTPServer{
		config:      cfg,
		engine:      engine,
		costService: costService,
	}

	// Setup routes
	srv.setupRoutes()

	return srv
}

// setupRoutes registers all API routes and middleware.
func (s *HTTPServer) setupRoutes() {
	// Health check endpoint
	s.engine.GET("/health", s.healthCheck)

	// API v1 routes
	apiV1 := s.engine.Group("/api/v1")
	{
		// Cost routes - will be implemented by routes package
		costGroup := apiV1.Group("/cost")
		s.registerCostRoutes(costGroup)

		// SLO routes
		sloGroup := apiV1.Group("/slo")
		s.registerSLORoutes(sloGroup)

		// ROI routes
		roiGroup := apiV1.Group("/roi")
		s.registerROIRoutes(roiGroup)
	}

	// Swagger documentation - enable in non-production environments
	if s.config.Env != config.EnvProduction {
		s.engine.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))
	}

	// 404 handler
	s.engine.NoRoute(func(c *gin.Context) {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "Not Found",
			"code":  "NOT_FOUND",
		})
	})
}

// registerCostRoutes registers cost-related routes (temporary implementation).
func (s *HTTPServer) registerCostRoutes(group *gin.RouterGroup) {
	// Global cost overview
	group.GET("/global", s.globalCost)
	// Namespace list (aggregated for frontend cost table)
	group.GET("/namespaces", s.listNamespaces)
	// Namespace cost
	group.GET("/namespace/:namespace", s.namespaceCost)
	// Drilldown
	group.GET("/drilldown/:level/:identifier", s.drilldownCost)
}

// registerSLORoutes registers SLO-related routes (temporary implementation).
func (s *HTTPServer) registerSLORoutes(group *gin.RouterGroup) {
	group.GET("/health", s.sloHealth)
}

// registerROIRoutes registers ROI-related routes (temporary implementation).
func (s *HTTPServer) registerROIRoutes(group *gin.RouterGroup) {
	group.GET("/dashboard", s.roiDashboard)
}

// healthCheck handles the health check endpoint.
func (s *HTTPServer) healthCheck(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status":    "healthy",
		"timestamp": time.Now().UTC(),
		"version":   "1.0.0",
	})
}

// globalCost handles GET /api/v1/cost/global
func (s *HTTPServer) globalCost(c *gin.Context) {
	if s.costService != nil {
		resp, err := s.costService.GetGlobalCost(c.Request.Context())
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, resp)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"total_cost": 10000.0,
		"namespaces": []map[string]interface{}{
			{"name": "default", "cost": 5000.0},
			{"name": "kube-system", "cost": 3000.0},
			{"name": "monitoring", "cost": 2000.0},
		},
		"timestamp": time.Now().UTC(),
	})
}

// listNamespaces handles GET /api/v1/cost/namespaces
func (s *HTTPServer) listNamespaces(c *gin.Context) {
	if s.costService != nil {
		list, err := s.costService.ListNamespaces(c.Request.Context())
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, list)
		return
	}
	c.JSON(http.StatusOK, []map[string]interface{}{
		{"name": "default", "cost": 5000.0, "grade": "Healthy", "pod_count": 10, "node_count": 0},
		{"name": "kube-system", "cost": 3000.0, "grade": "Healthy", "pod_count": 5, "node_count": 0},
		{"name": "monitoring", "cost": 2000.0, "grade": "Healthy", "pod_count": 3, "node_count": 0},
	})
}

// namespaceCost handles GET /api/v1/cost/namespace/:namespace
func (s *HTTPServer) namespaceCost(c *gin.Context) {
	namespace := c.Param("namespace")
	if s.costService != nil {
		resp, err := s.costService.GetNamespaceCost(c.Request.Context(), namespace)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, resp)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"namespace": namespace,
		"cost":      5000.0,
		"breakdown": map[string]float64{"cpu": 3000.0, "memory": 2000.0},
		"timestamp": time.Now().UTC(),
	})
}

// typeToLevel maps frontend type to backend level: namespace->L1, node->L2, workload->L3, pod->L4
var typeToLevel = map[string]string{
	"namespace": "L1", "node": "L2", "workload": "L3", "pod": "L4",
}

// levelToType maps backend level to frontend type
var levelToType = map[string]string{
	"L1": "namespace", "L2": "node", "L3": "workload", "L4": "pod",
}

// drilldownCost handles GET /api/v1/cost/drilldown/:level/:identifier
// level 接受 type (namespace/node/workload/pod) 或 L1/L2/L3/L4；query dimension=compute|storage|network，默认 compute
func (s *HTTPServer) drilldownCost(c *gin.Context) {
	levelOrType := c.Param("level")
	identifier := c.Param("identifier")
	dimension := c.Query("dimension")
	if dimension == "" {
		dimension = "compute"
	}
	level := typeToLevel[levelOrType]
	if level == "" {
		level = levelOrType
	}
	respType := levelToType[level]
	if respType == "" {
		respType = levelOrType
	}
	_ = level
	_ = dimension // reserved for storage/network branch
	// 成本分解：与 CostBreakdown 对齐，算力钻取每层返回
	costBreakdown := gin.H{
		"cpu":    1250.0,
		"memory": 875.0,
		"storage": 250.0,
		"network": 125.0,
	}
	c.JSON(http.StatusOK, gin.H{
		"level":            level,
		"id":               identifier,
		"name":             respType + "-" + identifier,
		"type":             respType,
		"cost":             2500.0,
		"optimizableSpace": 750.0,
		"efficiency":       70,
		"cost_breakdown":   costBreakdown,
		"children": []gin.H{
			{
				"id":               "node-1",
				"name":             "node-1",
				"type":             "node",
				"cost":             5000.0,
				"optimizableSpace": 1500.0,
				"efficiency":       70,
				"cost_breakdown":   gin.H{"cpu": 2750.0, "memory": 1750.0, "storage": 350.0, "network": 150.0},
				"children":         nil,
			},
		},
	})
}

// sloHealth handles GET /api/v1/slo/health - returns SLOStatus[] for frontend
func (s *HTTPServer) sloHealth(c *gin.Context) {
	// Mock SLO data matching frontend SLOStatus[] type
	c.JSON(http.StatusOK, []gin.H{
		{"serviceName": "api-gateway", "status": "healthy", "uptime": 99.95, "responseTime": 120, "errorRate": 0.01},
		{"serviceName": "order-service", "status": "healthy", "uptime": 99.90, "responseTime": 85, "errorRate": 0.02},
		{"serviceName": "payment-service", "status": "warning", "uptime": 99.50, "responseTime": 200, "errorRate": 0.15},
	})
}

// roiDashboard handles GET /api/v1/roi/dashboard - returns summary + ROITrend[] for frontend
func (s *HTTPServer) roiDashboard(c *gin.Context) {
	// Mock ROI dashboard: summary (roi_percentage etc.) + trends array
	trends := []gin.H{
		{"date": "2025-01-15", "value": 1.2, "cost": 100000, "efficiency": 68},
		{"date": "2025-01-22", "value": 1.35, "cost": 95000, "efficiency": 70},
		{"date": "2025-02-01", "value": 1.45, "cost": 90000, "efficiency": 72},
		{"date": "2025-02-15", "value": 1.5, "cost": 85000, "efficiency": 75},
	}
	c.JSON(http.StatusOK, gin.H{
		"roi_percentage": 45.2,
		"total_savings":  125000.0,
		"status":         "good",
		"trend":          "improving",
		"trends":         trends,
	})
}

// Start begins listening for HTTP requests.
func (s *HTTPServer) Start() error {
	addr := fmt.Sprintf(":%d", s.config.Server.Port)
	s.server = &http.Server{
		Addr:           addr,
		Handler:        s.engine,
		ReadTimeout:    s.config.Server.ReadTimeout,
		WriteTimeout:   s.config.Server.WriteTimeout,
		MaxHeaderBytes: 1 << 20, // 1MB
	}

	fmt.Printf("Starting HTTP server on %s\n", addr)
	fmt.Printf("Environment: %s\n", s.config.Env)
	if s.config.Env != config.EnvProduction {
		fmt.Printf("Swagger UI: http://localhost%s/swagger/index.html\n", addr)
	}

	return s.server.ListenAndServe()
}

// StartWithGracefulShutdown starts the server with graceful shutdown handling.
func (s *HTTPServer) StartWithGracefulShutdown() error {
	// Start server in a goroutine
	serverErr := make(chan error, 1)
	go func() {
		if err := s.Start(); err != nil && err != http.ErrServerClosed {
			serverErr <- err
		}
	}()

	// Wait for interrupt signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	select {
	case err := <-serverErr:
		return err
	case <-quit:
		fmt.Println("Shutting down server...")

		// Create a deadline for graceful shutdown
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		if err := s.server.Shutdown(ctx); err != nil {
			return fmt.Errorf("server forced to shutdown: %v", err)
		}

		fmt.Println("Server gracefully stopped")
		return nil
	}
}

// Stop gracefully stops the HTTP server.
func (s *HTTPServer) Stop(ctx context.Context) error {
	if s.server == nil {
		return nil
	}
	return s.server.Shutdown(ctx)
}

// Engine returns the underlying Gin engine (for testing purposes).
func (s *HTTPServer) Engine() *gin.Engine {
	return s.engine
}
