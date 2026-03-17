package inventory

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/vmarble/warehouse-management-service/internal/domain"
	"github.com/vmarble/warehouse-management-service/internal/platform/httpkit"
)

type Handler struct {
	svc Service
}

func NewHandler(s Service) *Handler {
	return &Handler{svc: s}
}

func (h *Handler) Register(rg *gin.RouterGroup) {
	inv := rg.Group("/inventory")
	inv.POST("/lots", h.receiveStock)
	inv.GET("/lots", h.listLots)
	inv.GET("/sheets", h.listSheets)
	inv.GET("/sheets/:id", h.getSheet)
	inv.GET("/sheets/:id/lineage", h.lineage)
	inv.POST("/cuts", h.recordCut)
	inv.GET("/remnants", h.findRemnants)
	inv.POST("/remnants/:id/allocate", h.allocateRemnant)
	inv.POST("/remnants/:id/waste", h.markWaste)
}

func (h *Handler) receiveStock(c *gin.Context) {
	var in ReceiveStockInput
	if !httpkit.Bind(c, &in) {
		return
	}
	lot, err := h.svc.ReceiveStock(c.Request.Context(), in)
	if err != nil {
		httpkit.Error(c, err)
		return
	}
	c.JSON(http.StatusCreated, lot)
}

func (h *Handler) listLots(c *gin.Context) {
	lots, err := h.svc.ListLots(c.Request.Context())
	if err != nil {
		httpkit.Error(c, err)
		return
	}
	c.JSON(http.StatusOK, lots)
}

func (h *Handler) listSheets(c *gin.Context) {
	sheets, err := h.svc.ListAvailableSheets(c.Request.Context())
	if err != nil {
		httpkit.Error(c, err)
		return
	}
	c.JSON(http.StatusOK, sheets)
}

func (h *Handler) getSheet(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	sheet, err := h.svc.GetSheet(c.Request.Context(), id)
	if err != nil {
		httpkit.Error(c, err)
		return
	}
	c.JSON(http.StatusOK, sheet)
}

func (h *Handler) lineage(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	remnants, err := h.svc.GetRemnantLineage(c.Request.Context(), id)
	if err != nil {
		httpkit.Error(c, err)
		return
	}
	c.JSON(http.StatusOK, remnants)
}

func (h *Handler) recordCut(c *gin.Context) {
	var in RecordCutInput
	if !httpkit.Bind(c, &in) {
		return
	}
	result, err := h.svc.RecordCut(c.Request.Context(), in)
	if err != nil {
		httpkit.Error(c, err)
		return
	}
	c.JSON(http.StatusCreated, result)
}

func (h *Handler) findRemnants(c *gin.Context) {
	minLength, _ := strconv.Atoi(c.DefaultQuery("min_length_mm", "0"))
	minWidth, _ := strconv.Atoi(c.DefaultQuery("min_width_mm", "0"))
	minDim := domain.Dimension{LengthMM: minLength, WidthMM: minWidth}

	remnants, err := h.svc.FindAvailableRemnants(c.Request.Context(), minDim)
	if err != nil {
		httpkit.Error(c, err)
		return
	}
	c.JSON(http.StatusOK, remnants)
}

func (h *Handler) allocateRemnant(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	var body struct {
		WorkOrderID uuid.UUID `json:"work_order_id"`
	}
	if !httpkit.Bind(c, &body) {
		return
	}
	if err := h.svc.AllocateRemnant(c.Request.Context(), id, body.WorkOrderID); err != nil {
		httpkit.Error(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "allocated"})
}

func (h *Handler) markWaste(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	if err := h.svc.MarkRemnantWaste(c.Request.Context(), id); err != nil {
		httpkit.Error(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "waste"})
}
