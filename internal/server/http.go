// Package server provides HTTP server implementation for Lighthouse API.
package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/myxxhui/lighthouse-src/internal/config"
	"github.com/myxxhui/lighthouse-src/internal/server/dto"
	"github.com/myxxhui/lighthouse-src/internal/server/middleware"
	"github.com/myxxhui/lighthouse-src/internal/server/service"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
)

// HTTPServer encapsulates the HTTP server with Gin engine and configuration.
type HTTPServer struct {
	config       *config.Config
	engine       *gin.Engine
	server       *http.Server
	costService  *service.CostService
	finopsSync   *service.FinOpsSyncRunner
}

// NewHTTPServer creates a new HTTP server instance. Uses Mock data if costService is nil.
// finopsSync 可为 nil：此时 POST/GET /finops/sync-jobs 返回 503；GET /finops/effective-config 仍返回进程内配置。[Ref: 03_Phase6/01_FinOps 主动同步]
func NewHTTPServer(cfg *config.Config, costService *service.CostService, finopsSync *service.FinOpsSyncRunner) *HTTPServer {
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
		finopsSync:  finopsSync,
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
		apiV1.GET("/projects", s.listCostProjects)
		// Cost routes - will be implemented by routes package
		costGroup := apiV1.Group("/cost")
		s.registerCostRoutes(costGroup)

		// SLO routes
		sloGroup := apiV1.Group("/slo")
		s.registerSLORoutes(sloGroup)

		// ROI routes
		roiGroup := apiV1.Group("/roi")
		s.registerROIRoutes(roiGroup)

		finopsGroup := apiV1.Group("/finops")
		s.registerFinOpsRoutes(finopsGroup)
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
	// 按环境云产品钻取 [Ref: 01_设计 D9-4、12_API]
	group.GET("/drilldown/env/:envId", s.drilldownEnvCost)
	// 全环境云产品明细（方案 A）[Ref: 01_设计 D9-8、12_API]；须在 /drilldown/:level/:identifier 之前注册
	group.GET("/drilldown/global", s.drilldownGlobalCost)
	// Drilldown
	group.GET("/drilldown/:level/:identifier", s.drilldownCost)
	// 成本结构趋势 [Ref: 01_设计 D9-9、12_API]
	group.GET("/trend", s.costTrend)
	// 成本数据源诊断（排查「暂无数据」根因）[Ref: 01_实践 §2.4]
	group.GET("/diagnostic", s.costDiagnostic)
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

// costDiagnostic 返回成本数据源诊断信息，便于排查「暂无数据」。[Ref: 01_实践 §2.4]
func (s *HTTPServer) costDiagnostic(c *gin.Context) {
	if s.costService == nil {
		c.JSON(http.StatusOK, gin.H{"hint": "cost service not configured"})
		return
	}
	diag, err := s.costService.GetCostDiagnostic(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, diag)
}

// parseMonthOrDay 解析 YYYY-MM 或 YYYY-MM-DD 格式的日期字符串。
func parseMonthOrDay(s string) (time.Time, error) {
	if t, err := time.Parse("2006-01", s); err == nil {
		return t, nil
	}
	return time.Parse("2006-01-02", s)
}

// parseCostTrackQuery 返回 technical|finance；非法或空返回 ""（旧客户端语义）。[Ref: 03_Phase6/01_FinOps双轨与全域成本重构/01_设计 §API、track 与 UX]
func parseCostTrackQuery(q string) string {
	q = strings.ToLower(strings.TrimSpace(q))
	switch q {
	case "technical", "finance":
		return q
	default:
		return ""
	}
}

// parseProjectIDsQuery 解析 project_ids=1,2,3；非法片段跳过。[Ref: 03_Phase6/03_前端全域成本透视/01_设计]
func parseProjectIDsQuery(q string) []int {
	q = strings.TrimSpace(q)
	if q == "" {
		return nil
	}
	var out []int
	for _, p := range strings.Split(q, ",") {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		id, err := strconv.Atoi(p)
		if err != nil {
			continue
		}
		out = append(out, id)
	}
	return out
}

// globalCost handles GET /api/v1/cost/global?period=month|quarter|... 或 date_from=YYYY-MM&date_to=YYYY-MM（自定义月份范围，从月原始表叠加 [Ref: 04_01_成本透视真实数据]）
// ledger_refresh=1|true：在读库前对各环境执行 BSS 余额/应付等 FinOps 辅助同步（拉取云 API 写入 PG）。[Ref: 03_Phase6/01_FinOps]
func (s *HTTPServer) globalCost(c *gin.Context) {
	ctx := c.Request.Context()
	if q := strings.ToLower(strings.TrimSpace(c.Query("ledger_refresh"))); q == "1" || q == "true" || q == "yes" {
		ctx = service.WithLedgerRefresh(ctx)
	}
	track := parseCostTrackQuery(c.Query("track"))
	projectIDs := parseProjectIDsQuery(c.Query("project_ids"))
	var globalEnvQuery []string
	if envsStr := c.Query("envs"); envsStr != "" {
		for _, e := range strings.Split(envsStr, ",") {
			if e = strings.TrimSpace(e); e != "" {
				globalEnvQuery = append(globalEnvQuery, e)
			}
		}
	}
	dateFrom := c.Query("date_from")
	dateTo := c.Query("date_to")
	if dateFrom != "" && dateTo != "" && s.costService != nil {
		from, err1 := parseMonthOrDay(dateFrom)
		to, err2 := parseMonthOrDay(dateTo)
		if err1 == nil && err2 == nil {
			resp, err := s.costService.GetGlobalCostByDateRange(ctx, from, to, track, projectIDs, globalEnvQuery)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}
			// 自定义日期路径：有数据返回 resp，无数据返回空结构，不回退到 period
			if resp != nil {
				c.JSON(http.StatusOK, resp)
			} else {
				empty := gin.H{
					"total_cost":        0,
					"total_optimizable": 0,
					"global_efficiency": 0,
					"domain_breakdown":  []interface{}{},
					"env_breakdown":     []interface{}{},
					"namespaces":        nil,
					"timestamp":         time.Now().UTC(),
				}
				if track != "" {
					empty["metadata"] = gin.H{"effective_track": track}
				}
				c.JSON(http.StatusOK, empty)
			}
			return
		}
	}
	period := c.DefaultQuery("period", "month")
	// 统计口径已移除，固定使用实际付款（payment）[Ref: 16_ §三]
	_ = c.Query("metric_type") // 兼容旧前端，忽略
	metricType := "payment"
	if s.costService != nil {
		resp, err := s.costService.GetGlobalCost(ctx, period, metricType, globalEnvQuery, projectIDs, track)
		if err != nil {
			// D1-4：降级查询超时返回 503
			if errors.Is(err, service.ErrFallbackTimeout) {
				c.JSON(http.StatusServiceUnavailable, gin.H{"error": "cost fallback query timeout"})
				return
			}
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

// listCostProjects handles GET /api/v1/projects [Ref: 03_Phase6/03_前端全域成本透视/01_设计]
func (s *HTTPServer) listCostProjects(c *gin.Context) {
	if s.costService == nil {
		c.JSON(http.StatusOK, gin.H{"projects": []dto.CostProjectItem{}})
		return
	}
	list, err := s.costService.ListCostProjects(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"projects": list})
}

// listNamespaces handles GET /api/v1/cost/namespaces?period=1d|7d|30d|month|quarter
// 云账单模式下无命名空间级数据，返回空数组避免前端 null.map [Ref: 04_Phase4/01_成本透视真实数据]
func (s *HTTPServer) listNamespaces(c *gin.Context) {
	period := c.DefaultQuery("period", "month")
	if s.costService != nil {
		list, err := s.costService.ListNamespaces(c.Request.Context(), period)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		if list == nil {
			list = []dto.NamespaceCostSummary{}
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

// drilldownEnvCost handles GET /api/v1/cost/drilldown/env/:envId?report_type=&period_key=&category=&sort= 或 date_from=&date_to= [Ref: 01_设计 D8 自定义日期下环境钻取]
func (s *HTTPServer) drilldownEnvCost(c *gin.Context) {
	envId := c.Param("envId")
	category := c.Query("category")
	sortOrder := c.DefaultQuery("sort", "cost_desc")
	dateFromStr := c.Query("date_from")
	dateToStr := c.Query("date_to")
	track := parseCostTrackQuery(c.Query("track"))
	if s.costService != nil && dateFromStr != "" && dateToStr != "" {
		if from, err := parseMonthOrDay(dateFromStr); err == nil {
			if to, err := parseMonthOrDay(dateToStr); err == nil {
				list, err := s.costService.GetGlobalDrilldownByDateRange(c.Request.Context(), from, to, category, sortOrder, envId, track)
				if err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
					return
				}
				if list == nil {
					list = []dto.EnvDrilldownItem{}
				}
				c.JSON(http.StatusOK, list)
				return
			}
		}
	}
	reportType := c.DefaultQuery("report_type", "30d")
	periodKey := c.Query("period_key")
	if periodKey == "" {
		now := time.Now().UTC()
		periodKey = now.AddDate(0, 0, -1).Format("2006-01-02")
	}
	if s.costService != nil {
		list, err := s.costService.GetEnvDrilldown(c.Request.Context(), envId, reportType, periodKey, category, sortOrder)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		if list == nil {
			list = []dto.EnvDrilldownItem{}
		}
		c.JSON(http.StatusOK, list)
		return
	}
	c.JSON(http.StatusOK, []dto.EnvDrilldownItem{})
}

// drilldownGlobalCost handles GET /api/v1/cost/drilldown/global?report_type=&period_key=&category=&sort=&env=&track= [Ref: 01_设计 D9-8、D6 云产品成本明细索引、12_API]
func (s *HTTPServer) drilldownGlobalCost(c *gin.Context) {
	category := c.Query("category")
	sortOrder := c.DefaultQuery("sort", "cost_desc")
	env := c.DefaultQuery("env", "all") // all | POC | FAT | UAT | PROD
	dateFromStr := c.Query("date_from")
	dateToStr := c.Query("date_to")
	track := parseCostTrackQuery(c.Query("track"))
	if s.costService != nil && dateFromStr != "" && dateToStr != "" {
		if from, err := parseMonthOrDay(dateFromStr); err == nil {
			if to, err := parseMonthOrDay(dateToStr); err == nil {
				list, err := s.costService.GetGlobalDrilldownByDateRange(c.Request.Context(), from, to, category, sortOrder, env, track)
				if err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
					return
				}
				if list == nil {
					list = []dto.EnvDrilldownItem{}
				}
				c.JSON(http.StatusOK, list)
				return
			}
		}
	}
	reportType := c.DefaultQuery("report_type", "30d")
	periodKey := c.Query("period_key")
	if periodKey == "" {
		periodKey = time.Now().UTC().AddDate(0, 0, -1).Format("2006-01-02")
	}
	if s.costService != nil {
		list, err := s.costService.GetGlobalDrilldown(c.Request.Context(), reportType, periodKey, category, sortOrder, env, track)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		if list == nil {
			list = []dto.EnvDrilldownItem{}
		}
		c.JSON(http.StatusOK, list)
		return
	}
	c.JSON(http.StatusOK, []dto.EnvDrilldownItem{})
}

// costTrend handles GET /api/v1/cost/trend?period=7d|30d|90d&env=POC|FAT|UAT|PROD|all&track=technical|finance [Ref: 01_设计 D9-9、12_API]
func (s *HTTPServer) costTrend(c *gin.Context) {
	period := c.Query("period")
	dateFromStr := c.Query("date_from")
	dateToStr := c.Query("date_to")
	envFilter := c.Query("env")
	track := parseCostTrackQuery(c.Query("track"))
	var dateFrom, dateTo *time.Time
	if dateFromStr != "" && dateToStr != "" {
		if from, err := parseMonthOrDay(dateFromStr); err == nil {
			dateFrom = &from
		}
		if to, err := parseMonthOrDay(dateToStr); err == nil {
			dateTo = &to
		}
	}
	if s.costService != nil {
		resp, err := s.costService.GetCostTrend(c.Request.Context(), period, dateFrom, dateTo, envFilter, track)
		if err != nil {
			if errors.Is(err, service.ErrFallbackTimeout) {
				c.JSON(http.StatusServiceUnavailable, gin.H{"error": "cost trend query timeout"})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		if resp == nil {
			resp = &dto.CostTrendResponse{Data: []dto.CostTrendDataPoint{}}
		}
		c.JSON(http.StatusOK, resp)
		return
	}
	c.JSON(http.StatusOK, dto.CostTrendResponse{Data: []dto.CostTrendDataPoint{}})
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
