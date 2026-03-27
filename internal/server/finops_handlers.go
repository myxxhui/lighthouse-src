package server

import (
	"database/sql"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/myxxhui/lighthouse-src/internal/server/dto"
	"github.com/myxxhui/lighthouse-src/internal/server/service"
)

func (s *HTTPServer) registerFinOpsRoutes(g *gin.RouterGroup) {
	g.GET("/effective-config", s.finopsEffectiveConfig)
	g.POST("/sync-jobs", s.finopsCreateSyncJob)
	// 须在 :id 之前注册，否则 "active" 会被当成 id [Ref: 03_Phase6/01_FinOps 主动同步]
	g.GET("/sync-jobs/active", s.finopsGetActiveSyncJob)
	g.GET("/sync-jobs/:id", s.finopsGetSyncJob)
}

// GET /api/v1/finops/effective-config — 只读生效配置（与 FINOPS_CG_SOURCE / ETL 环境变量一致，无密钥）。[Ref: 03_Phase6/01_FinOps 主动同步]
func (s *HTTPServer) finopsEffectiveConfig(c *gin.Context) {
	c.JSON(http.StatusOK, service.BuildFinOpsEffectiveConfigDTO(s.config))
}

// POST /api/v1/finops/sync-jobs — 202 + job_id。[Ref: 03_Phase6/01_FinOps 主动同步]
func (s *HTTPServer) finopsCreateSyncJob(c *gin.Context) {
	if s.finopsSync == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "finops sync runner not configured",
			"code":  "FINOPS_SYNC_UNAVAILABLE",
		})
		return
	}
	if !s.finopsSyncCreateAuthorized(c) {
		return
	}
	id, err := s.finopsSync.CreateJob(c.Request.Context())
	if errors.Is(err, service.ErrFinOpsSyncActive) {
		body := gin.H{
			"error": err.Error(),
			"code":  "FINOPS_SYNC_ACTIVE",
		}
		if aid, e := s.finopsSync.ActiveJobID(c.Request.Context()); e == nil && aid > 0 {
			body["active_job_id"] = aid
		}
		c.JSON(http.StatusConflict, body)
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusAccepted, dto.FinOpsSyncJobCreateResponse{JobID: id})
}

// GET /api/v1/finops/sync-jobs/active — 当前 queued/running Job 的完整状态；无则 404。[Ref: 03_Phase6/01_FinOps 主动同步]
func (s *HTTPServer) finopsGetActiveSyncJob(c *gin.Context) {
	if s.finopsSync == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "finops sync runner not configured",
			"code":  "FINOPS_SYNC_UNAVAILABLE",
		})
		return
	}
	aid, err := s.finopsSync.ActiveJobID(c.Request.Context())
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if errors.Is(err, sql.ErrNoRows) || aid <= 0 {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "no active sync job",
			"code":  "FINOPS_SYNC_NO_ACTIVE",
		})
		return
	}
	resp, err := s.finopsSync.GetJob(c.Request.Context(), aid)
	if errors.Is(err, sql.ErrNoRows) {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "no active sync job",
			"code":  "FINOPS_SYNC_NO_ACTIVE",
		})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, resp)
}

// GET /api/v1/finops/sync-jobs/:id — Job 状态。[Ref: 03_Phase6/01_FinOps 主动同步]
func (s *HTTPServer) finopsGetSyncJob(c *gin.Context) {
	if s.finopsSync == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "finops sync runner not configured",
			"code":  "FINOPS_SYNC_UNAVAILABLE",
		})
		return
	}
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || id <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid job id", "code": "INVALID_JOB_ID"})
		return
	}
	resp, err := s.finopsSync.GetJob(c.Request.Context(), id)
	if errors.Is(err, sql.ErrNoRows) {
		c.JSON(http.StatusNotFound, gin.H{"error": "job not found", "code": "FINOPS_SYNC_JOB_NOT_FOUND"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, resp)
}

// finopsSyncCreateAuthorized 当配置了 FINOPS_SYNC_JOB_API_KEY 时，要求 X-FinOps-Sync-Key 或 Authorization: Bearer 一致。[Ref: 03_Phase6/01_FinOps 主动同步]
func (s *HTTPServer) finopsSyncCreateAuthorized(c *gin.Context) bool {
	key := strings.TrimSpace(s.config.FinOpsSyncJobAPIKey)
	if key == "" {
		return true
	}
	hdr := strings.TrimSpace(c.GetHeader("X-FinOps-Sync-Key"))
	if hdr == "" {
		a := c.GetHeader("Authorization")
		if len(a) >= 7 && strings.EqualFold(a[:7], "bearer ") {
			hdr = strings.TrimSpace(a[7:])
		}
	}
	if hdr != key {
		c.JSON(http.StatusUnauthorized, gin.H{
			"error": "finops sync job creation requires valid credentials",
			"code":  "FINOPS_SYNC_AUTH_REQUIRED",
		})
		return false
	}
	return true
}
