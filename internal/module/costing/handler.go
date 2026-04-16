package costing

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
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
	rg.POST("/costing/:workOrderID/compute", auth.RequireRole(auth.RoleAccountant, auth.RoleAdmin), h.compute)
	rg.POST("/costing/:workOrderID/finalize", auth.RequireRole(auth.RoleAccountant, auth.RoleAdmin), h.finalize)
	rg.GET("/costing/:workOrderID", auth.RequireRole(auth.RoleAccountant, auth.RolePlanner, auth.RoleAdmin), h.get)
	rg.GET("/costing", auth.RequireRole(auth.RoleAccountant, auth.RoleAdmin), h.list)
}

// computeCost godoc
//
// @Summary      Compute costing for a work order
// @Tags         costing
// @Produce      json
// @Param        workOrderID  path      string  true  "work order id (uuid)"
// @Success      200          {object}  CostingRecord
// @Failure      400          {object}  map[string]string
// @Failure      409          {object}  map[string]string
// @Failure      422          {object}  map[string]string
// @Security     BearerAuth
// @Failure      401  {object}  map[string]string
// @Router       /api/v1/costing/{workOrderID}/compute [post]
func (h *Handler) compute(c *gin.Context) {
	woID, err := uuid.Parse(c.Param("workOrderID"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid workOrderID"})
		return
	}
	record, err := h.svc.ComputeCost(c.Request.Context(), woID)
	if err != nil {
		httpkit.Error(c, err)
		return
	}
	c.JSON(http.StatusOK, record)
}

// finalizeCost godoc
//
// @Summary      Finalize costing for a work order
// @Tags         costing
// @Produce      json
// @Param        workOrderID  path      string  true  "work order id (uuid)"
// @Success      200          {object}  map[string]string
// @Failure      400          {object}  map[string]string
// @Failure      409          {object}  map[string]string
// @Security     BearerAuth
// @Failure      401  {object}  map[string]string
// @Router       /api/v1/costing/{workOrderID}/finalize [post]
func (h *Handler) finalize(c *gin.Context) {
	woID, err := uuid.Parse(c.Param("workOrderID"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid workOrderID"})
		return
	}
	if err := h.svc.FinalizeCost(c.Request.Context(), woID); err != nil {
		httpkit.Error(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "finalized"})
}

// getCostingRecord godoc
//
// @Summary      Get costing record by work order
// @Tags         costing
// @Produce      json
// @Param        workOrderID  path      string  true  "work order id (uuid)"
// @Success      200          {object}  CostingRecord
// @Failure      400          {object}  map[string]string
// @Failure      404          {object}  map[string]string
// @Security     BearerAuth
// @Failure      401  {object}  map[string]string
// @Router       /api/v1/costing/{workOrderID} [get]
func (h *Handler) get(c *gin.Context) {
	woID, err := uuid.Parse(c.Param("workOrderID"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid workOrderID"})
		return
	}
	record, err := h.svc.GetCostingRecord(c.Request.Context(), woID)
	if err != nil {
		httpkit.Error(c, err)
		return
	}
	c.JSON(http.StatusOK, record)
}

// listCostingRecords godoc
//
// @Summary      List costing records
// @Tags         costing
// @Produce      json
// @Param        page       query     int     false  "page number (default 1)"
// @Param        limit      query     int     false  "items per page (default 10, max 100)"
// @Param        finalized  query     bool    false  "filter by finalized: true or false (omit for all)"
// @Param        order      query     string  false  "sort direction: asc, desc (default asc)"
// @Success      200  {object}  httpkit.PagedResult[CostingRecord]
// @Failure      500  {object}  map[string]string
// @Security     BearerAuth
// @Failure      401  {object}  map[string]string
// @Router       /api/v1/costing [get]
func (h *Handler) list(c *gin.Context) {
	p := httpkit.BindPageParams(c)
	var finalized *bool
	if v := c.Query("finalized"); v != "" {
		b := v == "true"
		finalized = &b
	}
	result, err := h.svc.ListCostingRecords(c.Request.Context(), p, finalized)
	if err != nil {
		httpkit.Error(c, err)
		return
	}
	c.JSON(http.StatusOK, result)
}
