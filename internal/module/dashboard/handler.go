package dashboard

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/vmarble/warehouse-management-service/internal/platform/auth"
	"github.com/vmarble/warehouse-management-service/internal/platform/httpkit"
)

type Handler struct {
	svc Service
}

func NewHandler(s Service) *Handler {
	return &Handler{svc: s}
}

func (h *Handler) Register(rg *gin.RouterGroup) {
	rg.GET("/dashboard/overview", auth.RequireRole(auth.RoleAdmin, auth.RolePlanner, auth.RoleAccountant), h.overview)
}

// getDashboardOverview godoc
//
// @Summary      Get dashboard overview
// @Description  Aggregated KPI, chart, and recent activity data for admin dashboard
// @Tags         dashboard
// @Produce      json
// @Success      200  {object}  OverviewOutput
// @Failure      401  {object}  map[string]string
// @Failure      403  {object}  map[string]string
// @Failure      500  {object}  map[string]string
// @Security     BearerAuth
// @Router       /api/v1/dashboard/overview [get]
func (h *Handler) overview(c *gin.Context) {
	out, err := h.svc.GetOverview(c.Request.Context())
	if err != nil {
		httpkit.Error(c, err)
		return
	}
	c.JSON(http.StatusOK, out)
}
