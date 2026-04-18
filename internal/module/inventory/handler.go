package inventory

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/vmarble/warehouse-management-service/internal/domain"
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
	inv := rg.Group("/inventory")
	inv.POST("/lots", auth.RequireRole(auth.RoleWarehouse, auth.RoleAdmin), h.receiveStock)
	inv.GET("/lots", h.listLots)
	inv.DELETE("/lots/:id", auth.RequireRole(auth.RoleWarehouse, auth.RoleAdmin), h.deleteLot)
	inv.GET("/sheets", h.listSheets)
	inv.GET("/sheets/:id", h.getSheet)
	inv.GET("/sheets/:id/lineage", h.lineage)
	inv.POST("/cuts", auth.RequireRole(auth.RoleWarehouse, auth.RoleCNC, auth.RoleCNCManager), h.recordCut)
	inv.GET("/remnants", h.listRemnants)
	inv.GET("/remnants/suggestions", h.suggestRemnants)
	inv.GET("/remnants/:id", h.getRemnant)
	inv.GET("/remnants/:id/lineage", h.getRemnantLineage)
	inv.POST("/remnants/:id/allocate", auth.RequireRole(auth.RoleWarehouse, auth.RoleCNC, auth.RoleCNCManager), h.allocateRemnant)
	inv.POST("/remnants/:id/waste", auth.RequireRole(auth.RoleWarehouse, auth.RoleAdmin), h.markWaste)
	inv.POST("/remnants/:id/stock", auth.RequireRole(auth.RoleWarehouse), h.stockRemnant)

	rg.GET("/storage-locations", h.listStorageLocations)
}

// receiveStock godoc
//
// @Summary      Receive stock (create inventory lot)
// @Tags         inventory
// @Accept       json
// @Produce      json
// @Param        body  body      ReceiveStockInput  true  "payload"
// @Security     BearerAuth
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
// @Summary      List inventory lots (paginated)
// @Tags         inventory
// @Produce      json
// @Param        page      query     int     false  "page number (default 1)"
// @Param        limit     query     int     false  "items per page (default 10, max 100)"
// @Param        search    query     string  false  "filter by supplier_ref (case-insensitive)"
// @Param        sort_by   query     string  false  "sort column: supplier_ref (default received_at)"
// @Param        order     query     string  false  "asc or desc (default desc)"
// @Security     BearerAuth
// @Success      200  {object}  httpkit.PagedResult[InventoryLot]
// @Failure      500  {object}  map[string]string
// @Router       /api/v1/inventory/lots [get]
func (h *Handler) listLots(c *gin.Context) {
	p := httpkit.BindPageParams(c)
	result, err := h.svc.ListLots(c.Request.Context(), p)
	if err != nil {
		httpkit.Error(c, err)
		return
	}
	c.JSON(http.StatusOK, result)
}

// deleteLot godoc
//
// @Summary      Deactivate inventory lot (soft delete)
// @Tags         inventory
// @Produce      json
// @Param        id   path      string  true  "lot id (uuid)"
// @Success      204
// @Failure      400  {object}  map[string]string
// @Failure      404  {object}  map[string]string
// @Security     BearerAuth
// @Router       /api/v1/inventory/lots/{id} [delete]
func (h *Handler) deleteLot(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	if err := h.svc.DeactivateLot(c.Request.Context(), id); err != nil {
		httpkit.Error(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// listSheets godoc
//
// @Summary      List available board sheets (paginated)
// @Tags         inventory
// @Produce      json
// @Param        material_id  query     string  false  "filter by material id (uuid)"
// @Param        page         query     int     false  "page number (default 1)"
// @Param        limit        query     int     false  "items per page (default 10, max 100)"
// @Param        sort_by      query     string  false  "sort column: length_mm|width_mm (default id)"
// @Param        order        query     string  false  "asc or desc (default asc)"
// @Security     BearerAuth
// @Success      200  {object}  httpkit.PagedResult[BoardSheet]
// @Failure      400  {object}  map[string]string
// @Failure      500  {object}  map[string]string
// @Router       /api/v1/inventory/sheets [get]
func (h *Handler) listSheets(c *gin.Context) {
	var materialID *uuid.UUID
	if midStr := c.Query("material_id"); midStr != "" {
		parsed, err := uuid.Parse(midStr)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid material_id"})
			return
		}
		materialID = &parsed
	}
	p := httpkit.BindPageParams(c)
	result, err := h.svc.ListAvailableSheets(c.Request.Context(), p, materialID)
	if err != nil {
		httpkit.Error(c, err)
		return
	}
	c.JSON(http.StatusOK, result)
}

// getSheet godoc
//
// @Summary      Get board sheet
// @Tags         inventory
// @Produce      json
// @Param        id   path      string  true  "sheet id (uuid)"
// @Security     BearerAuth
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
// @Security     BearerAuth
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
// @Security     BearerAuth
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

// listRemnants godoc
//
// @Summary      List remnants with filters (paginated)
// @Tags         inventory
// @Produce      json
// @Param        min_length_mm  query     int     false  "Minimum usable length in mm (bounding box)"
// @Param        min_width_mm   query     int     false  "Minimum usable width in mm (bounding box)"
// @Param        status         query     string  false  "Remnant status (default: AVAILABLE)"  Enums(AVAILABLE,ALLOCATED,CONSUMED,WASTE)
// @Param        page           query     int     false  "Page number (default 1)"
// @Param        limit          query     int     false  "Items per page (default 10, max 100)"
// @Security     BearerAuth
// @Success      200            {object}  httpkit.PagedResult[Remnant]
// @Failure      500            {object}  map[string]string
// @Router       /api/v1/inventory/remnants [get]
func (h *Handler) listRemnants(c *gin.Context) {
	minLength, _ := strconv.Atoi(c.DefaultQuery("min_length_mm", "0"))
	minWidth, _ := strconv.Atoi(c.DefaultQuery("min_width_mm", "0"))
	status := domain.RemnantStatus(c.DefaultQuery("status", string(domain.RemnantAvailable)))

	f := RemnantFilter{
		MinLengthMM: minLength,
		MinWidthMM:  minWidth,
		Status:      status,
	}
	p := httpkit.BindPageParams(c)

	result, err := h.svc.ListRemnants(c.Request.Context(), f, p)
	if err != nil {
		httpkit.Error(c, err)
		return
	}
	c.JSON(http.StatusOK, result)
}

// suggestRemnants godoc
//
// @Summary      Suggest best-fit remnants for a required dimension
// @Description  Returns up to `limit` AVAILABLE remnants ranked by Best Fit (smallest area) + FIFO (oldest first). Each suggestion includes the remnant's storage location when available.
// @Tags         inventory
// @Produce      json
// @Param        length_mm  query     int   true   "required length in mm"
// @Param        width_mm   query     int   true   "required width in mm"
// @Param        limit      query     int   false  "max results (default 3, max 10)"
// @Security     BearerAuth
// @Success      200  {array}   RemnantSuggestion
// @Failure      400  {object}  map[string]string
// @Router       /api/v1/inventory/remnants/suggestions [get]
func (h *Handler) suggestRemnants(c *gin.Context) {
	lengthMM, err := strconv.Atoi(c.Query("length_mm"))
	if err != nil || lengthMM <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "length_mm must be a positive integer"})
		return
	}
	widthMM, err := strconv.Atoi(c.Query("width_mm"))
	if err != nil || widthMM <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "width_mm must be a positive integer"})
		return
	}
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "3"))

	suggestions, err := h.svc.SuggestRemnants(c.Request.Context(), SuggestRemnantsInput{
		RequiredDimension: domain.Dimension{LengthMM: lengthMM, WidthMM: widthMM},
		Limit:             limit,
	})
	if err != nil {
		httpkit.Error(c, err)
		return
	}
	c.JSON(http.StatusOK, suggestions)
}

// getRemnantLineage godoc
//
// @Summary      Get full lineage tree for a remnant
// @Description  Returns all remnants that share the same parent board as the given remnant, ordered by created_at ASC.
// @Tags         inventory
// @Produce      json
// @Param        id   path      string  true  "Remnant ID (uuid)"
// @Security     BearerAuth
// @Success      200  {array}   Remnant
// @Failure      400  {object}  map[string]string
// @Failure      404  {object}  map[string]string
// @Router       /api/v1/inventory/remnants/{id}/lineage [get]
func (h *Handler) getRemnantLineage(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	remnants, err := h.svc.GetRemnantLineageByRemnant(c.Request.Context(), id)
	if err != nil {
		httpkit.Error(c, err)
		return
	}
	c.JSON(http.StatusOK, remnants)
}

// listStorageLocations godoc
//
// @Summary      List active storage locations
// @Description  Returns all storage locations where is_active = true, ordered by zone, rack, shelf.
// @Tags         inventory
// @Produce      json
// @Security     BearerAuth
// @Success      200  {array}   StorageLocation
// @Failure      500  {object}  map[string]string
// @Router       /api/v1/storage-locations [get]
func (h *Handler) listStorageLocations(c *gin.Context) {
	locs, err := h.svc.ListStorageLocations(c.Request.Context())
	if err != nil {
		httpkit.Error(c, err)
		return
	}
	c.JSON(http.StatusOK, locs)
}

// allocateRemnant godoc
//
// @Summary      Allocate remnant to work order
// @Tags         inventory
// @Accept       json
// @Produce      json
// @Param        id    path      string  true  "remnant id (uuid)"
// @Param        body  body      object  true  "payload"  SchemaExample({"work_order_id":"00000000-0000-0000-0000-000000000000"})
// @Security     BearerAuth
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
// @Security     BearerAuth
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

// getRemnant godoc
//
// @Summary      Get remnant by ID
// @Tags         inventory
// @Produce      json
// @Param        id   path      string  true  "remnant id (uuid)"
// @Security     BearerAuth
// @Success      200  {object}  Remnant
// @Failure      400  {object}  map[string]string
// @Failure      404  {object}  map[string]string
// @Router       /api/v1/inventory/remnants/{id} [get]
func (h *Handler) getRemnant(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	remnant, err := h.svc.GetRemnant(c.Request.Context(), id)
	if err != nil {
		httpkit.Error(c, err)
		return
	}
	c.JSON(http.StatusOK, remnant)
}

// stockRemnant godoc
//
// @Summary      Assign a remnant to a physical storage bin
// @Tags         inventory
// @Accept       json
// @Produce      json
// @Param        id    path      string  true  "remnant id (uuid)"
// @Param        body  body      object  true  "location barcode"
// @Security     BearerAuth
// @Success      200   {object}  map[string]string
// @Failure      400   {object}  map[string]string
// @Failure      404   {object}  map[string]string
// @Router       /api/v1/inventory/remnants/{id}/stock [post]
func (h *Handler) stockRemnant(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	var body struct {
		LocationBarcode string `json:"location_barcode"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || body.LocationBarcode == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "location_barcode is required"})
		return
	}
	if err := h.svc.StockRemnant(c.Request.Context(), id, body.LocationBarcode); err != nil {
		httpkit.Error(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "stocked"})
}
