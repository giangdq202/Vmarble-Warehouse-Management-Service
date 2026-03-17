package barcode

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
	rg.POST("/barcodes", h.generate)
	rg.GET("/barcodes/:id", h.lookup)
	rg.POST("/barcodes/:id/scans", h.recordScan)
	rg.GET("/barcodes/:id/scans", h.listScans)
}

func (h *Handler) generate(c *gin.Context) {
	var in GenerateBarcodeInput
	if !httpkit.Bind(c, &in) {
		return
	}
	bc, err := h.svc.GenerateBarcode(c.Request.Context(), in)
	if err != nil {
		httpkit.Error(c, err)
		return
	}
	c.JSON(http.StatusCreated, bc)
}

func (h *Handler) lookup(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	bc, err := h.svc.LookupBarcode(c.Request.Context(), id)
	if err != nil {
		httpkit.Error(c, err)
		return
	}
	c.JSON(http.StatusOK, bc)
}

func (h *Handler) recordScan(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	var in RecordScanInput
	if !httpkit.Bind(c, &in) {
		return
	}
	in.BarcodeID = id
	event, err := h.svc.RecordScan(c.Request.Context(), in)
	if err != nil {
		httpkit.Error(c, err)
		return
	}
	c.JSON(http.StatusCreated, event)
}

func (h *Handler) listScans(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	events, err := h.svc.ListScans(c.Request.Context(), id)
	if err != nil {
		httpkit.Error(c, err)
		return
	}
	c.JSON(http.StatusOK, events)
}
