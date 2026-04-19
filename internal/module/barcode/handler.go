package barcode

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
	rg.POST("/barcodes", auth.RequireRole(auth.RoleWarehouse, auth.RoleCNC, auth.RoleCNCManager), h.generate)
	rg.GET("/barcodes", h.listByWorkOrder)
	rg.GET("/barcodes/:id", h.lookup)
	rg.GET("/barcodes/:id/qr", h.generateQR)
	rg.GET("/barcodes/:id/label.pdf", h.generateLabelPDF)
	rg.POST("/barcodes/batch-print", auth.RequireRole(auth.RoleWarehouse, auth.RoleCNC, auth.RoleCNCManager), h.generateBatchLabelPDF)
	rg.POST("/barcodes/:id/scans", auth.RequireRole(auth.RoleCNC, auth.RoleWarehouse, auth.RoleForeman), h.recordScan)
	rg.GET("/barcodes/:id/scans", h.listScans)
}

// listBarcodesByWorkOrder godoc
//
// @Summary      List barcodes by work order
// @Tags         barcode
// @Produce      json
// @Param        work_order_id  query     string  true  "work order id (uuid)"
// @Success      200            {array}   Barcode
// @Failure      400            {object}  map[string]string
// @Security     BearerAuth
// @Failure      401  {object}  map[string]string
// @Router       /api/v1/barcodes [get]
func (h *Handler) listByWorkOrder(c *gin.Context) {
	woID, err := uuid.Parse(c.Query("work_order_id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid work_order_id"})
		return
	}
	barcodes, err := h.svc.ListBarcodesByWorkOrder(c.Request.Context(), woID)
	if err != nil {
		httpkit.Error(c, err)
		return
	}
	if barcodes == nil {
		barcodes = []Barcode{}
	}
	c.JSON(http.StatusOK, barcodes)
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
// @Security     BearerAuth
// @Failure      401  {object}  map[string]string
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
// @Security     BearerAuth
// @Failure      401  {object}  map[string]string
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

// generateQR godoc
//
// @Summary      Generate QR code image for a barcode
// @Tags         barcode
// @Produce      image/png
// @Param        id   path      string  true  "barcode id (uuid)"
// @Success      200  {file}    binary
// @Failure      400  {object}  map[string]string
// @Failure      404  {object}  map[string]string
// @Security     BearerAuth
// @Failure      401  {object}  map[string]string
// @Router       /api/v1/barcodes/{id}/qr [get]
func (h *Handler) generateQR(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	png, err := h.svc.GenerateQRCode(c.Request.Context(), id)
	if err != nil {
		httpkit.Error(c, err)
		return
	}
	c.Data(http.StatusOK, "image/png", png)
}

// generateLabelPDF godoc
//
// @Summary      Generate printable PDF label for a barcode
// @Tags         barcode
// @Produce      application/pdf
// @Param        id    path      string  true  "barcode id (uuid)"
// @Param        size  query     string  false  "label size: 50x30 or 100x70"
// @Success      200   {file}    binary
// @Failure      400   {object}  map[string]string
// @Failure      404   {object}  map[string]string
// @Security     BearerAuth
// @Failure      401  {object}  map[string]string
// @Router       /api/v1/barcodes/{id}/label.pdf [get]
func (h *Handler) generateLabelPDF(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	size := LabelSize(c.Query("size"))
	if size == "" {
		size = LabelSize50x30
	}

	pdf, err := h.svc.GenerateLabelPDF(c.Request.Context(), id, size)
	if err != nil {
		httpkit.Error(c, err)
		return
	}
	c.Header("Content-Disposition", "inline; filename=barcode-label-"+id.String()+"-"+string(size)+".pdf")
	c.Data(http.StatusOK, "application/pdf", pdf)
}

// recordScan godoc
//
// @Summary      Record scan event
// @Tags         barcode
// @Accept       json
// @Produce      json
// @Param        id    path      string          true  "barcode id (uuid)"
// @Param        body  body      object  true  "payload"
// @Success      201   {object}  ScanResult
// @Failure      400   {object}  map[string]string
// @Failure      409   {object}  map[string]string
// @Security     BearerAuth
// @Failure      401  {object}  map[string]string
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
	identity, ok := auth.FromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "missing auth identity"})
		return
	}
	scannedBy, err := uuid.Parse(identity.UserID)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid auth identity"})
		return
	}
	in.BarcodeID = id
	in.ScannedBy = scannedBy
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
// @Security     BearerAuth
// @Failure      401  {object}  map[string]string
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

// generateBatchLabelPDF godoc
//
// @Summary      Generate printable batch PDF labels
// @Tags         barcode
// @Accept       json
// @Produce      application/pdf
// @Param        body  body      BatchPrintInput  true  "payload"
// @Success      200   {file}    binary
// @Failure      400   {object}  map[string]string
// @Failure      404   {object}  map[string]string
// @Security     BearerAuth
// @Failure      401  {object}  map[string]string
// @Router       /api/v1/barcodes/batch-print [post]
func (h *Handler) generateBatchLabelPDF(c *gin.Context) {
	var in BatchPrintInput
	if !httpkit.Bind(c, &in) {
		return
	}
	pdf, err := h.svc.GenerateBatchLabelPDF(c.Request.Context(), in)
	if err != nil {
		httpkit.Error(c, err)
		return
	}
	c.Header("Content-Disposition", "inline; filename=barcode-label-batch.pdf")
	c.Data(http.StatusOK, "application/pdf", pdf)
}
