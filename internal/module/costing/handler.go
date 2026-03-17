package costing

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/vmarble/warehouse-management-service/internal/platform/httpkit"
)

type Handler struct {
	svc Service
}

func NewHandler(s Service) *Handler {
	return &Handler{svc: s}
}

func (h *Handler) Register(rg *gin.RouterGroup) {
	rg.POST("/costing/:workOrderID/compute", h.compute)
	rg.POST("/costing/:workOrderID/finalize", h.finalize)
	rg.GET("/costing/:workOrderID", h.get)
	rg.GET("/costing", h.list)
}

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

func (h *Handler) list(c *gin.Context) {
	records, err := h.svc.ListCostingRecords(c.Request.Context())
	if err != nil {
		httpkit.Error(c, err)
		return
	}
	c.JSON(http.StatusOK, records)
}
