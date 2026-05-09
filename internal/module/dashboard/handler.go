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
	rg.GET("/dashboard/board-stock-summary", auth.RequireRole(auth.RoleAdmin, auth.RolePlanner, auth.RoleWarehouse), h.boardStockSummary)
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

// boardStockSummary godoc
//
// @Summary      Get whole board sheet stock summary by material
// @Description  Returns available and allocated whole board sheet counts and total area per material type
// @Tags         dashboard
// @Produce      json
// @Success      200  {array}   BoardStockSummaryItem
// @Failure      401  {object}  map[string]string
// @Failure      403  {object}  map[string]string
// @Failure      500  {object}  map[string]string
// @Security     BearerAuth
// @Router       /api/v1/dashboard/board-stock-summary [get]
func (h *Handler) boardStockSummary(c *gin.Context) {
	out, err := h.svc.GetBoardStockSummary(c.Request.Context())
	if err != nil {
		httpkit.Error(c, err)
		return
	}
	c.JSON(http.StatusOK, out)
}
