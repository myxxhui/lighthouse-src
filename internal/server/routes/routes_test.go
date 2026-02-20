package routes

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestRegisterCostRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	group := r.Group("/api/v1/cost")
	RegisterCostRoutes(group)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/cost/global", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("GET /api/v1/cost/global want 200, got %d", rec.Code)
	}
}
