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
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
)

// HTTPServer encapsulates the HTTP server with Gin engine and configuration.
type HTTPServer struct {
	config *config.Config
	engine *gin.Engine
	server *http.Server
}

// NewHTTPServer creates a new HTTP server instance.
func NewHTTPServer(cfg *config.Config) *HTTPServer {
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
		config: cfg,
		engine: engine,
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
	// TODO: Integrate with business logic
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

// namespaceCost handles GET /api/v1/cost/namespace/:namespace
func (s *HTTPServer) namespaceCost(c *gin.Context) {
	namespace := c.Param("namespace")
	// TODO: Integrate with business logic
	c.JSON(http.StatusOK, gin.H{
		"namespace": namespace,
		"cost":      5000.0,
		"breakdown": map[string]float64{
			"cpu":    3000.0,
			"memory": 2000.0,
		},
		"timestamp": time.Now().UTC(),
	})
}

// drilldownCost handles GET /api/v1/cost/drilldown/:level/:identifier
func (s *HTTPServer) drilldownCost(c *gin.Context) {
	level := c.Param("level")
	identifier := c.Param("identifier")
	// TODO: Integrate with business logic
	c.JSON(http.StatusOK, gin.H{
		"level":      level,
		"identifier": identifier,
		"cost":       2500.0,
		"details":    "Drilldown data will be implemented in Phase 2",
	})
}

// sloHealth handles GET /api/v1/slo/health
func (s *HTTPServer) sloHealth(c *gin.Context) {
	// TODO: Integrate with SLO business logic
	c.JSON(http.StatusOK, gin.H{
		"status": "healthy",
		"metrics": map[string]interface{}{
			"availability": 99.95,
			"latency_p95":  150,
			"error_rate":   0.01,
		},
		"timestamp": time.Now().UTC(),
	})
}

// roiDashboard handles GET /api/v1/roi/dashboard
func (s *HTTPServer) roiDashboard(c *gin.Context) {
	// TODO: Integrate with ROI business logic
	c.JSON(http.StatusOK, gin.H{
		"roi_percentage": 45.2,
		"total_savings":  125000.0,
		"trend":          "improving",
		"timestamp":      time.Now().UTC(),
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
