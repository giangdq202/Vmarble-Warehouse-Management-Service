package inventory

import (
	"net/http"
	"strconv"
	"time"

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
	inv.POST("/lots/:id/qc-pass", auth.RequireWorkerUp(), h.qcPassLot)
	inv.POST("/lots/:id/reject", auth.RequireWorkerUp(), h.rejectLot)
	inv.GET("/material-rejections", auth.RequireWorkerUp(), h.listRejections)
	inv.GET("/material-rejections/:id", auth.RequireWorkerUp(), h.getRejection)
	inv.PATCH("/material-rejections/:id", auth.RequireRole(auth.RoleAccountant, auth.RoleAdmin), h.updateRejectionClaim)
	inv.GET("/reports/rejections", auth.RequireRole(auth.RoleAccountant, auth.RoleAdmin), h.rejectionReport)
	inv.GET("/overflow-status", h.getOverflowStatus)
	inv.GET("/sheets", h.listSheets)
	inv.GET("/sheets/:id", h.getSheet)
	inv.GET("/sheets/:id/lineage", h.lineage)
	inv.POST("/cuts", auth.RequireRole(auth.RoleWarehouse, auth.RoleCNC, auth.RoleCNCManager, auth.RoleAdmin), h.recordCut)
	inv.GET("/cutting-records", h.listCuttingRecords)
	inv.GET("/remnants", h.listRemnants)
	inv.GET("/remnants/suggestions", h.suggestRemnants)
	inv.GET("/remnants/:id", h.getRemnant)
	inv.GET("/remnants/:id/lineage", h.getRemnantLineage)
	inv.GET("/remnants/:id/label.pdf", h.getRemnantLabelPDF)
	inv.POST("/cutting-records/:id/labels", auth.RequireRole(auth.RoleWarehouse, auth.RoleCNC, auth.RoleCNCManager, auth.RoleAdmin), h.generateCutLabels)
	inv.POST("/remnants/:id/allocate", auth.RequireRole(auth.RoleWarehouse, auth.RoleCNC, auth.RoleCNCManager, auth.RoleAdmin), h.allocateRemnant)
	inv.POST("/remnants/:id/waste", auth.RequireRole(auth.RoleWarehouse, auth.RoleAdmin), h.markWaste)
	inv.POST("/remnants/:id/stock", auth.RequireRole(auth.RoleWarehouse, auth.RoleCNC, auth.RoleCNCManager, auth.RoleAdmin), h.stockRemnant)

	inv.POST("/transfers", auth.RequireRole(auth.RoleWarehouse, auth.RoleAdmin), h.transfer)
	inv.GET("/audit-log", auth.RequireRole(auth.RoleAccountant, auth.RoleAdmin), h.listAuditLogByAction)
	inv.GET("/audit-log/:entity_type/:entity_id", h.listAuditLog)
	inv.POST("/cycle-counts", auth.RequireRole(auth.RoleWarehouse, auth.RoleAdmin), h.createCycleCount)
	inv.GET("/cycle-counts/:id", h.getCycleCount)
	inv.POST("/cycle-counts/:id/lines", auth.RequireRole(auth.RoleWarehouse, auth.RoleAdmin), h.addCycleCountLine)
	inv.GET("/cycle-counts/:id/lines", h.listCycleCountLines)
	inv.POST("/cycle-counts/:id/post", auth.RequireRole(auth.RoleWarehouse, auth.RoleAdmin), h.postCycleCount)
	inv.POST("/cycle-counts/:id/cancel", auth.RequireRole(auth.RoleWarehouse, auth.RoleAdmin), h.cancelCycleCount)

	inv.GET("/work-orders/:id/pick-slip", auth.RequireRole(auth.RoleWarehouse, auth.RoleCNC, auth.RoleCNCManager, auth.RoleForeman, auth.RoleAdmin), h.getPickSlipPDF)

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

// getOverflowStatus godoc
//
// @Summary      Get inventory overflow status
// @Tags         inventory
// @Produce      json
// @Security     BearerAuth
// @Success      200  {object}  OverflowStatus
// @Failure      500  {object}  map[string]string
// @Router       /api/v1/inventory/overflow-status [get]
func (h *Handler) getOverflowStatus(c *gin.Context) {
	status, err := h.svc.GetOverflowStatus(c.Request.Context())
	if err != nil {
		httpkit.Error(c, err)
		return
	}
	c.JSON(http.StatusOK, status)
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

// listCuttingRecords godoc
//
// @Summary      List cutting records (history report, keyset paginated)
// @Description  Returns a keyset-paginated history of cut events enriched with SKU code/name and the work-order assignee. Ordered by created_at DESC. Optional filters: user_id (maps to work_orders.assigned_to), work_order_id, from/to (RFC3339).
// @Tags         inventory
// @Produce      json
// @Param        user_id         query     string  false  "filter by assigned worker (uuid)"
// @Param        work_order_id   query     string  false  "filter by work order (uuid)"
// @Param        from            query     string  false  "start of date range (RFC3339)"
// @Param        to              query     string  false  "end of date range (RFC3339)"
// @Param        cursor          query     string  false  "opaque cursor token returned in next_cursor; omit for first page"
// @Param        limit           query     int     false  "page size (default 50, max 200)"
// @Security     BearerAuth
// @Success      200  {object}  httpkit.CursorResult[inventory.CuttingRecordReport]
// @Failure      400  {object}  map[string]string
// @Failure      500  {object}  map[string]string
// @Router       /api/v1/inventory/cutting-records [get]
func (h *Handler) listCuttingRecords(c *gin.Context) {
	f := CuttingRecordFilter{}

	if s := c.Query("user_id"); s != "" {
		id, err := uuid.Parse(s)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid user_id"})
			return
		}
		f.UserID = &id
	}
	if s := c.Query("work_order_id"); s != "" {
		id, err := uuid.Parse(s)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid work_order_id"})
			return
		}
		f.WorkOrderID = &id
	}
	if s := c.Query("from"); s != "" {
		t, err := time.Parse(time.RFC3339, s)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid from (RFC3339 expected)"})
			return
		}
		f.From = t
	}
	if s := c.Query("to"); s != "" {
		t, err := time.Parse(time.RFC3339, s)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid to (RFC3339 expected)"})
			return
		}
		f.To = t
	}

	params := httpkit.BindCursorParams(c)
	result, err := h.svc.ListCuttingRecords(c.Request.Context(), f, params)
	if err != nil {
		httpkit.Error(c, err)
		return
	}
	c.JSON(http.StatusOK, result)
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

// transfer godoc
//
// @Summary      Transfer inventory item to a new bin location
// @Tags         inventory
// @Accept       json
// @Produce      json
// @Param        body  body      TransferInput  true  "payload"
// @Security     BearerAuth
// @Success      201   {object}  TransferResult
// @Failure      400   {object}  map[string]string
// @Failure      404   {object}  map[string]string
// @Failure      412   {object}  map[string]string
// @Router       /api/v1/inventory/transfers [post]
func (h *Handler) transfer(c *gin.Context) {
	var in TransferInput
	if !httpkit.Bind(c, &in) {
		return
	}
	identity, ok := auth.FromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "missing auth identity"})
		return
	}
	actorID, err := uuid.Parse(identity.UserID)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid auth identity"})
		return
	}
	in.ActorID = actorID
	result, err := h.svc.Transfer(c.Request.Context(), in)
	if err != nil {
		httpkit.Error(c, err)
		return
	}
	c.JSON(http.StatusCreated, result)
}

// listAuditLogByAction godoc
//
// @Summary      List audit log entries by action across all entities (keyset paginated)
// @Description  Useful for accountant/admin review (e.g. action=REMNANT_BYPASSED,
// @Description  OVERFLOW_BYPASSED). Restricted to accountant + admin roles.
// @Tags         inventory
// @Produce      json
// @Param        action  query     string  true   "audit action (REMNANT_BYPASSED, OVERFLOW_BYPASSED, TRANSFER, ADJUSTMENT)"
// @Param        cursor  query     string  false  "opaque cursor token returned in next_cursor; omit for first page"
// @Param        limit   query     int     false  "page size (default 50, max 200)"
// @Security     BearerAuth
// @Success      200  {object}  httpkit.CursorResult[inventory.AuditLogEntry]
// @Failure      400  {object}  map[string]string
// @Router       /api/v1/inventory/audit-log [get]
func (h *Handler) listAuditLogByAction(c *gin.Context) {
	action := c.Query("action")
	params := httpkit.BindCursorParams(c)
	res, err := h.svc.ListAuditLogByAction(c.Request.Context(), action, params)
	if err != nil {
		httpkit.Error(c, err)
		return
	}
	c.JSON(http.StatusOK, res)
}

// listAuditLog godoc
//
// @Summary      List audit log entries for an inventory entity (keyset paginated)
// @Tags         inventory
// @Produce      json
// @Param        entity_type  path      string  true   "entity type: REMNANT or BOARD_SHEET"
// @Param        entity_id    path      string  true   "entity id (uuid)"
// @Param        cursor       query     string  false  "opaque cursor token returned in next_cursor; omit for first page"
// @Param        limit        query     int     false  "page size (default 50, max 200)"
// @Security     BearerAuth
// @Success      200  {object}  httpkit.CursorResult[inventory.AuditLogEntry]
// @Failure      400  {object}  map[string]string
// @Router       /api/v1/inventory/audit-log/{entity_type}/{entity_id} [get]
func (h *Handler) listAuditLog(c *gin.Context) {
	entityType := c.Param("entity_type")
	entityID, err := uuid.Parse(c.Param("entity_id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid entity_id"})
		return
	}
	params := httpkit.BindCursorParams(c)
	res, err := h.svc.ListAuditLog(c.Request.Context(), entityID, entityType, params)
	if err != nil {
		httpkit.Error(c, err)
		return
	}
	c.JSON(http.StatusOK, res)
}

// createCycleCount godoc
//
// @Summary      Create a cycle count session
// @Tags         inventory
// @Accept       json
// @Produce      json
// @Param        body  body      CreateCycleCountInput  true  "payload"
// @Security     BearerAuth
// @Success      201   {object}  CycleCountSession
// @Failure      400   {object}  map[string]string
// @Router       /api/v1/inventory/cycle-counts [post]
func (h *Handler) createCycleCount(c *gin.Context) {
	var in CreateCycleCountInput
	if !httpkit.Bind(c, &in) {
		return
	}
	identity, ok := auth.FromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "missing auth identity"})
		return
	}
	actorID, err := uuid.Parse(identity.UserID)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid auth identity"})
		return
	}
	in.ActorID = actorID
	sess, err := h.svc.CreateCycleCountSession(c.Request.Context(), in)
	if err != nil {
		httpkit.Error(c, err)
		return
	}
	c.JSON(http.StatusCreated, sess)
}

// getCycleCount godoc
//
// @Summary      Get a cycle count session by ID
// @Tags         inventory
// @Produce      json
// @Param        id   path      string  true  "session id (uuid)"
// @Security     BearerAuth
// @Success      200  {object}  CycleCountSession
// @Failure      404  {object}  map[string]string
// @Router       /api/v1/inventory/cycle-counts/{id} [get]
func (h *Handler) getCycleCount(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	sess, err := h.svc.GetCycleCountSession(c.Request.Context(), id)
	if err != nil {
		httpkit.Error(c, err)
		return
	}
	c.JSON(http.StatusOK, sess)
}

// addCycleCountLine godoc
//
// @Summary      Add a count line to a cycle count session
// @Tags         inventory
// @Accept       json
// @Produce      json
// @Param        id    path      string           true  "session id (uuid)"
// @Param        body  body      AddCountLineInput  true  "payload"
// @Security     BearerAuth
// @Success      201   {object}  CycleCountLine
// @Failure      400   {object}  map[string]string
// @Failure      409   {object}  map[string]string
// @Router       /api/v1/inventory/cycle-counts/{id}/lines [post]
func (h *Handler) addCycleCountLine(c *gin.Context) {
	sessionID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	var in AddCountLineInput
	if !httpkit.Bind(c, &in) {
		return
	}
	in.SessionID = sessionID
	line, err := h.svc.AddCycleCountLine(c.Request.Context(), in)
	if err != nil {
		httpkit.Error(c, err)
		return
	}
	c.JSON(http.StatusCreated, line)
}

// listCycleCountLines godoc
//
// @Summary      List count lines for a cycle count session
// @Tags         inventory
// @Produce      json
// @Param        id   path      string  true  "session id (uuid)"
// @Security     BearerAuth
// @Success      200  {array}   CycleCountLine
// @Failure      404  {object}  map[string]string
// @Router       /api/v1/inventory/cycle-counts/{id}/lines [get]
func (h *Handler) listCycleCountLines(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	lines, err := h.svc.ListCycleCountLines(c.Request.Context(), id)
	if err != nil {
		httpkit.Error(c, err)
		return
	}
	if lines == nil {
		lines = []CycleCountLine{}
	}
	c.JSON(http.StatusOK, lines)
}

// postCycleCount godoc
//
// @Summary      Post a cycle count session (apply adjustments)
// @Tags         inventory
// @Produce      json
// @Param        id   path      string  true  "session id (uuid)"
// @Security     BearerAuth
// @Success      200  {object}  map[string]string
// @Failure      404  {object}  map[string]string
// @Failure      409  {object}  map[string]string
// @Router       /api/v1/inventory/cycle-counts/{id}/post [post]
func (h *Handler) postCycleCount(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	identity, ok := auth.FromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "missing auth identity"})
		return
	}
	actorID, err := uuid.Parse(identity.UserID)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid auth identity"})
		return
	}
	if err := h.svc.PostCycleCount(c.Request.Context(), PostCycleCountInput{
		SessionID: id,
		ActorID:   actorID,
	}); err != nil {
		httpkit.Error(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "posted"})
}

// cancelCycleCount godoc
//
// @Summary      Cancel a cycle count session
// @Tags         inventory
// @Produce      json
// @Param        id   path      string  true  "session id (uuid)"
// @Security     BearerAuth
// @Success      200  {object}  map[string]string
// @Failure      404  {object}  map[string]string
// @Failure      409  {object}  map[string]string
// @Router       /api/v1/inventory/cycle-counts/{id}/cancel [post]
func (h *Handler) cancelCycleCount(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	identity, ok := auth.FromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "missing auth identity"})
		return
	}
	actorID, err := uuid.Parse(identity.UserID)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid auth identity"})
		return
	}
	if err := h.svc.CancelCycleCountSession(c.Request.Context(), id, actorID); err != nil {
		httpkit.Error(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "cancelled"})
}

// getRemnantLabelPDF godoc
//
// @Summary      Generate printable PDF label for a remnant (stock label)
// @Tags         inventory
// @Produce      application/pdf
// @Param        id    path      string  true   "remnant id (uuid)"
// @Param        size  query     string  false  "label size: 50x30 or 100x70 (default 50x30)"
// @Success      200   {file}    binary
// @Failure      400   {object}  map[string]string
// @Failure      404   {object}  map[string]string
// @Security     BearerAuth
// @Failure      401   {object}  map[string]string
// @Router       /api/v1/inventory/remnants/{id}/label.pdf [get]
func (h *Handler) getRemnantLabelPDF(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	size := RemnantLabelSize(c.Query("size"))
	if size == "" {
		size = RemnantLabelSize50x30
	}
	pdf, err := h.svc.GenerateRemnantLabelPDF(c.Request.Context(), RemnantLabelInput{
		RemnantID: id,
		Size:      size,
	})
	if err != nil {
		httpkit.Error(c, err)
		return
	}
	c.Header("Content-Disposition", "inline; filename=remnant-label-"+id.String()+"-"+string(size)+".pdf")
	c.Data(http.StatusOK, "application/pdf", pdf)
}

// generateCutLabels godoc
//
// @Summary      Generate combined WIP + remnant labels PDF for a cutting record
// @Description  Returns a single PDF document containing one WIP label page for the cutting record. When the cut produced a leftover remnant, an additional remnant label page is appended. Used by the cutting kiosk to auto-print labels at the moment a cut is reported.
// @Tags         inventory
// @Produce      application/pdf
// @Param        id    path      string  true   "cutting record id (uuid)"
// @Param        size  query     string  false  "label size: 50x30 or 100x70 (default 50x30)"
// @Success      200   {file}    binary
// @Failure      400   {object}  map[string]string
// @Failure      401   {object}  map[string]string
// @Failure      404   {object}  map[string]string
// @Security     BearerAuth
// @Router       /api/v1/inventory/cutting-records/{id}/labels [post]
func (h *Handler) generateCutLabels(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	size := RemnantLabelSize(c.Query("size"))
	if size == "" {
		size = RemnantLabelSize50x30
	}
	pdf, err := h.svc.GenerateCutLabelsPDF(c.Request.Context(), CutLabelsInput{
		CuttingRecordID: id,
		Size:            size,
	})
	if err != nil {
		httpkit.Error(c, err)
		return
	}
	c.Header("Content-Disposition", "inline; filename=cut-labels-"+id.String()+"-"+string(size)+".pdf")
	c.Data(http.StatusOK, "application/pdf", pdf)
}

// getPickSlipPDF godoc
//
// @Summary      Generate pick-slip PDF for a work order
// @Description  Returns an A4 PDF listing every ALLOCATED remnant for the work order, grouped by storage zone for efficient walking order. Returns 404 when no remnants are allocated.
// @Tags         inventory
// @Produce      application/pdf
// @Param        id   path      string  true  "work order id (uuid)"
// @Success      200  {file}    binary
// @Failure      400  {object}  map[string]string
// @Failure      404  {object}  map[string]string
// @Security     BearerAuth
// @Router       /api/v1/inventory/work-orders/{id}/pick-slip [get]
func (h *Handler) getPickSlipPDF(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	pdf, err := h.svc.GeneratePickSlipPDF(c.Request.Context(), id)
	if err != nil {
		httpkit.Error(c, err)
		return
	}
	c.Header("Content-Disposition", "inline; filename=pick-slip-"+id.String()+".pdf")
	c.Data(http.StatusOK, "application/pdf", pdf)
}

// ── BR-INV01..06: Material rejection + supplier claim ───────────────────────

// qcPassLot godoc
//
// @Summary      QC-pass an inventory lot (transition all PENDING_QC sheets to AVAILABLE)
// @Tags         inventory
// @Produce      json
// @Param        id   path      string  true  "lot id (uuid)"
// @Security     BearerAuth
// @Success      204
// @Failure      400  {object}  map[string]string
// @Failure      404  {object}  map[string]string
// @Failure      412  {object}  map[string]string
// @Router       /api/v1/inventory/lots/{id}/qc-pass [post]
func (h *Handler) qcPassLot(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	actorID, ok := actorIDFromContext(c)
	if !ok {
		return
	}
	if err := h.svc.QCPassLot(c.Request.Context(), id, actorID); err != nil {
		httpkit.Error(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// rejectLot godoc
//
// @Summary      Reject part or all of an inventory lot
// @Description  Transitions up to rejected_qty_sheets PENDING_QC sheets to REJECTED
// @Description  and creates a material_rejections row. (BR-INV02/03/04)
// @Tags         inventory
// @Accept       json
// @Produce      json
// @Param        id    path      string          true  "lot id (uuid)"
// @Param        body  body      RejectLotInput  true  "payload"
// @Security     BearerAuth
// @Success      201   {object}  RejectLotResult
// @Failure      400   {object}  map[string]string
// @Failure      412   {object}  map[string]string
// @Router       /api/v1/inventory/lots/{id}/reject [post]
func (h *Handler) rejectLot(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	var in RejectLotInput
	if !httpkit.Bind(c, &in) {
		return
	}
	actorID, ok := actorIDFromContext(c)
	if !ok {
		return
	}
	in.LotID = id
	in.ActorID = actorID
	res, err := h.svc.RejectLot(c.Request.Context(), in)
	if err != nil {
		httpkit.Error(c, err)
		return
	}
	c.JSON(http.StatusCreated, res)
}

// listRejections godoc
//
// @Summary      List material rejections (keyset paginated)
// @Tags         inventory
// @Produce      json
// @Param        claim_status  query     string  false  "filter by claim_status (OPEN|APPROVED|REJECTED|PAID)"
// @Param        lot_id        query     string  false  "filter by lot id (uuid)"
// @Param        cursor        query     string  false  "opaque cursor token; omit for first page"
// @Param        limit         query     int     false  "page size (default 50, max 200)"
// @Security     BearerAuth
// @Success      200  {object}  httpkit.CursorResult[inventory.MaterialRejection]
// @Router       /api/v1/inventory/material-rejections [get]
func (h *Handler) listRejections(c *gin.Context) {
	f := RejectionFilter{ClaimStatus: c.Query("claim_status")}
	if lotStr := c.Query("lot_id"); lotStr != "" {
		parsed, err := uuid.Parse(lotStr)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid lot_id"})
			return
		}
		f.LotID = &parsed
	}
	params := httpkit.BindCursorParams(c)
	res, err := h.svc.ListRejections(c.Request.Context(), f, params)
	if err != nil {
		httpkit.Error(c, err)
		return
	}
	c.JSON(http.StatusOK, res)
}

// getRejection godoc
//
// @Summary      Get a single material rejection
// @Tags         inventory
// @Produce      json
// @Param        id   path      string  true  "rejection id (uuid)"
// @Security     BearerAuth
// @Success      200  {object}  MaterialRejection
// @Failure      404  {object}  map[string]string
// @Router       /api/v1/inventory/material-rejections/{id} [get]
func (h *Handler) getRejection(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	r, err := h.svc.GetRejection(c.Request.Context(), id)
	if err != nil {
		httpkit.Error(c, err)
		return
	}
	c.JSON(http.StatusOK, r)
}

// updateRejectionClaim godoc
//
// @Summary      Update a material rejection's claim status
// @Description  Allowed transitions: OPEN→APPROVED, OPEN→REJECTED, APPROVED→PAID. (BR-INV05)
// @Tags         inventory
// @Accept       json
// @Produce      json
// @Param        id    path      string            true  "rejection id (uuid)"
// @Param        body  body      UpdateClaimInput  true  "payload"
// @Security     BearerAuth
// @Success      200   {object}  MaterialRejection
// @Failure      400   {object}  map[string]string
// @Failure      404   {object}  map[string]string
// @Failure      409   {object}  map[string]string
// @Router       /api/v1/inventory/material-rejections/{id} [patch]
func (h *Handler) updateRejectionClaim(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	var in UpdateClaimInput
	if !httpkit.Bind(c, &in) {
		return
	}
	actorID, ok := actorIDFromContext(c)
	if !ok {
		return
	}
	in.RejectionID = id
	in.ActorID = actorID
	r, err := h.svc.UpdateRejectionClaim(c.Request.Context(), in)
	if err != nil {
		httpkit.Error(c, err)
		return
	}
	c.JSON(http.StatusOK, r)
}

// rejectionReport godoc
//
// @Summary      Aggregate material-rejection totals by supplier (BR-INV06)
// @Tags         inventory
// @Produce      json
// @Param        from          query     string  false  "RFC3339 lower bound on reported_at (inclusive)"
// @Param        to            query     string  false  "RFC3339 upper bound on reported_at (exclusive)"
// @Param        supplier_ref  query     string  false  "case-insensitive supplier filter"
// @Security     BearerAuth
// @Success      200  {array}   RejectionReport
// @Router       /api/v1/inventory/reports/rejections [get]
func (h *Handler) rejectionReport(c *gin.Context) {
	f := RejectionReportFilter{SupplierRef: c.Query("supplier_ref")}
	if fromStr := c.Query("from"); fromStr != "" {
		t, err := time.Parse(time.RFC3339, fromStr)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid from (expected RFC3339)"})
			return
		}
		f.From = t
	}
	if toStr := c.Query("to"); toStr != "" {
		t, err := time.Parse(time.RFC3339, toStr)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid to (expected RFC3339)"})
			return
		}
		f.To = t
	}
	rows, err := h.svc.RejectionReport(c.Request.Context(), f)
	if err != nil {
		httpkit.Error(c, err)
		return
	}
	c.JSON(http.StatusOK, rows)
}

// actorIDFromContext extracts the auth identity actor uuid; writes a 401 and
// returns ok=false if missing/invalid.
func actorIDFromContext(c *gin.Context) (uuid.UUID, bool) {
	identity, ok := auth.FromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "missing auth identity"})
		return uuid.Nil, false
	}
	id, err := uuid.Parse(identity.UserID)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid auth identity"})
		return uuid.Nil, false
	}
	return id, true
}
