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

// generateBarcode godoc
//
// @Summary      Generate barcode
// @Tags         barcode
// @Accept       json
// @Produce      json
// @Param        body  body      GenerateBarcodeInput  true  "payload"
// @Success      201   {object}  Barcode
// @Failure      400   {object}  map[string]string
// @Router       /api/v1/barcodes [post]
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

// lookupBarcode godoc
//
// @Summary      Lookup barcode
// @Tags         barcode
// @Produce      json
// @Param        id   path      string  true  "barcode id (uuid)"
// @Success      200  {object}  Barcode
// @Failure      400  {object}  map[string]string
// @Failure      404  {object}  map[string]string
// @Router       /api/v1/barcodes/{id} [get]
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

// recordScan godoc
//
// @Summary      Record scan event
// @Tags         barcode
// @Accept       json
// @Produce      json
// @Param        id    path      string          true  "barcode id (uuid)"
// @Param        body  body      RecordScanInput  true  "payload"
// @Success      201   {object}  ScanEvent
// @Failure      400   {object}  map[string]string
// @Failure      409   {object}  map[string]string
// @Router       /api/v1/barcodes/{id}/scans [post]
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

// listScans godoc
//
// @Summary      List scan events
// @Tags         barcode
// @Produce      json
// @Param        id   path      string  true  "barcode id (uuid)"
// @Success      200  {array}   ScanEvent
// @Failure      400  {object}  map[string]string
// @Failure      404  {object}  map[string]string
// @Router       /api/v1/barcodes/{id}/scans [get]
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
