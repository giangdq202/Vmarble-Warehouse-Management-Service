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

// receiveStock godoc
//
// @Summary      Receive stock (create inventory lot)
// @Tags         inventory
// @Accept       json
// @Produce      json
// @Param        body  body      ReceiveStockInput  true  "payload"
// @Success      201   {object}  InventoryLot
// @Failure      400   {object}  map[string]string
// @Router       /api/v1/inventory/lots [post]
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

// listLots godoc
//
// @Summary      List inventory lots
// @Tags         inventory
// @Produce      json
// @Success      200  {array}   InventoryLot
// @Failure      500  {object}  map[string]string
// @Router       /api/v1/inventory/lots [get]
func (h *Handler) listLots(c *gin.Context) {
	lots, err := h.svc.ListLots(c.Request.Context())
	if err != nil {
		httpkit.Error(c, err)
		return
	}
	c.JSON(http.StatusOK, lots)
}

// listSheets godoc
//
// @Summary      List available board sheets
// @Tags         inventory
// @Produce      json
// @Success      200  {array}   BoardSheet
// @Failure      500  {object}  map[string]string
// @Router       /api/v1/inventory/sheets [get]
func (h *Handler) listSheets(c *gin.Context) {
	sheets, err := h.svc.ListAvailableSheets(c.Request.Context())
	if err != nil {
		httpkit.Error(c, err)
		return
	}
	c.JSON(http.StatusOK, sheets)
}

// getSheet godoc
//
// @Summary      Get board sheet
// @Tags         inventory
// @Produce      json
// @Param        id   path      string  true  "sheet id (uuid)"
// @Success      200  {object}  BoardSheet
// @Failure      400  {object}  map[string]string
// @Failure      404  {object}  map[string]string
// @Router       /api/v1/inventory/sheets/{id} [get]
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

// getLineage godoc
//
// @Summary      Get remnant lineage by sheet
// @Tags         inventory
// @Produce      json
// @Param        id   path      string  true  "sheet id (uuid)"
// @Success      200  {array}   Remnant
// @Failure      400  {object}  map[string]string
// @Failure      404  {object}  map[string]string
// @Router       /api/v1/inventory/sheets/{id}/lineage [get]
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

// recordCut godoc
//
// @Summary      Record cutting operation
// @Tags         inventory
// @Accept       json
// @Produce      json
// @Param        body  body      RecordCutInput  true  "payload"
// @Success      201   {object}  CutResult
// @Failure      400   {object}  map[string]string
// @Failure      422   {object}  map[string]string
// @Router       /api/v1/inventory/cuts [post]
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

// findRemnants godoc
//
// @Summary      Find available remnants
// @Tags         inventory
// @Produce      json
// @Param        min_length_mm  query     int  false  "min length (mm)"
// @Param        min_width_mm   query     int  false  "min width (mm)"
// @Success      200            {array}   Remnant
// @Failure      500            {object}  map[string]string
// @Router       /api/v1/inventory/remnants [get]
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

// allocateRemnant godoc
//
// @Summary      Allocate remnant to work order
// @Tags         inventory
// @Accept       json
// @Produce      json
// @Param        id    path      string  true  "remnant id (uuid)"
// @Param        body  body      object  true  "payload"  SchemaExample({"work_order_id":"00000000-0000-0000-0000-000000000000"})
// @Success      200   {object}  map[string]string
// @Failure      400   {object}  map[string]string
// @Failure      409   {object}  map[string]string
// @Router       /api/v1/inventory/remnants/{id}/allocate [post]
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

// markWaste godoc
//
// @Summary      Mark remnant as waste
// @Tags         inventory
// @Produce      json
// @Param        id   path      string  true  "remnant id (uuid)"
// @Success      200  {object}  map[string]string
// @Failure      400  {object}  map[string]string
// @Failure      409  {object}  map[string]string
// @Router       /api/v1/inventory/remnants/{id}/waste [post]
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
