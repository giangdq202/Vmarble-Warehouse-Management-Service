package production

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
	rg.POST("/work-orders", h.create)
	rg.GET("/work-orders", h.list)
	// /mine must be registered before /:id to avoid Gin treating "mine" as an id param.
	rg.GET("/work-orders/mine", auth.RequireRole(auth.RoleCNC), h.listMine)
	rg.GET("/work-orders/:id", h.get)
	rg.POST("/work-orders/:id/advance", h.advance)
	rg.POST("/work-orders/:id/consumptions", h.recordConsumption)
	rg.GET("/work-orders/:id/consumptions", h.listConsumptions)
	rg.POST("/work-orders/:id/assign", auth.RequireRole(auth.RoleCNCManager), h.assign)
	rg.POST("/work-orders/:id/suggest-assignment", auth.RequireRole(auth.RoleCNCManager), h.suggestAssignment)
}

// createWorkOrder godoc
//
// @Summary      Create work order
// @Tags         production
// @Accept       json
// @Produce      json
// @Param        body  body      CreateWOInput  true  "payload"
// @Success      201   {object}  WorkOrder
// @Failure      400   {object}  map[string]string
// @Security     BearerAuth
// @Failure      401  {object}  map[string]string
// @Router       /api/v1/work-orders [post]
func (h *Handler) create(c *gin.Context) {
	var in CreateWOInput
	if !httpkit.Bind(c, &in) {
		return
	}
	wo, err := h.svc.CreateWorkOrder(c.Request.Context(), in)
	if err != nil {
		httpkit.Error(c, err)
		return
	}
	c.JSON(http.StatusCreated, wo)
}

// listWorkOrders godoc
//
// @Summary      List work orders
// @Tags         production
// @Produce      json
// @Param        plan_id  query     string  false  "filter by plan id (uuid) — returns full list, no pagination"
// @Param        page     query     int     false  "page number (default 1)"
// @Param        limit    query     int     false  "items per page (default 10, max 100)"
// @Param        status   query     string  false  "filter by status: PLANNED, IN_CUTTING, IN_PROCESSING, COMPLETED, COSTED"
// @Param        sort_by  query     string  false  "sort column: created_at, status (default created_at)"
// @Param        order    query     string  false  "sort direction: asc, desc (default desc)"
// @Success      200      {object}  httpkit.PagedResult[WorkOrder]  "paginated list (when plan_id is absent)"
// @Success      200      {array}   WorkOrder                       "full list (when plan_id is present)"
// @Failure      400      {object}  map[string]string
// @Failure      500      {object}  map[string]string
// @Security     BearerAuth
// @Failure      401  {object}  map[string]string
// @Router       /api/v1/work-orders [get]
func (h *Handler) list(c *gin.Context) {
	planIDStr := c.Query("plan_id")
	if planIDStr != "" {
		planID, err := uuid.Parse(planIDStr)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid plan_id"})
			return
		}
		wos, err := h.svc.ListWorkOrdersByPlan(c.Request.Context(), planID)
		if err != nil {
			httpkit.Error(c, err)
			return
		}
		c.JSON(http.StatusOK, wos)
		return
	}
	p := httpkit.BindPageParams(c)
	status := c.Query("status")
	result, err := h.svc.ListWorkOrders(c.Request.Context(), p, status)
	if err != nil {
		httpkit.Error(c, err)
		return
	}
	c.JSON(http.StatusOK, result)
}

// getWorkOrder godoc
//
// @Summary      Get work order
// @Tags         production
// @Produce      json
// @Param        id   path      string  true  "work order id (uuid)"
// @Success      200  {object}  WorkOrder
// @Failure      400  {object}  map[string]string
// @Failure      404  {object}  map[string]string
// @Security     BearerAuth
// @Failure      401  {object}  map[string]string
// @Router       /api/v1/work-orders/{id} [get]
func (h *Handler) get(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	wo, err := h.svc.GetWorkOrder(c.Request.Context(), id)
	if err != nil {
		httpkit.Error(c, err)
		return
	}
	c.JSON(http.StatusOK, wo)
}

// advanceWorkOrder godoc
//
// @Summary      Advance work order status
// @Tags         production
// @Accept       json
// @Produce      json
// @Param        id    path      string             true  "work order id (uuid)"
// @Param        body  body      AdvanceStatusInput true  "payload"
// @Success      200   {object}  map[string]string
// @Failure      400   {object}  map[string]string
// @Failure      409   {object}  map[string]string
// @Security     BearerAuth
// @Failure      401  {object}  map[string]string
// @Router       /api/v1/work-orders/{id}/advance [post]
func (h *Handler) advance(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	var in AdvanceStatusInput
	if !httpkit.Bind(c, &in) {
		return
	}
	if err := h.svc.AdvanceStatus(c.Request.Context(), id, in); err != nil {
		httpkit.Error(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": string(in.To)})
}

// recordConsumption godoc
//
// @Summary      Record material consumption for work order
// @Tags         production
// @Accept       json
// @Produce      json
// @Param        id    path      string                true  "work order id (uuid)"
// @Param        body  body      RecordConsumptionInput true  "payload"
// @Success      201   {object}  ConsumptionRecord
// @Failure      400   {object}  map[string]string
// @Failure      422   {object}  map[string]string
// @Security     BearerAuth
// @Failure      401  {object}  map[string]string
// @Router       /api/v1/work-orders/{id}/consumptions [post]
func (h *Handler) recordConsumption(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	var in RecordConsumptionInput
	if !httpkit.Bind(c, &in) {
		return
	}
	in.WorkOrderID = id
	cr, err := h.svc.RecordConsumption(c.Request.Context(), in)
	if err != nil {
		httpkit.Error(c, err)
		return
	}
	c.JSON(http.StatusCreated, cr)
}

// listConsumptions godoc
//
// @Summary      List consumption records for work order
// @Tags         production
// @Produce      json
// @Param        id   path      string  true  "work order id (uuid)"
// @Success      200  {array}   ConsumptionRecord
// @Failure      400  {object}  map[string]string
// @Failure      404  {object}  map[string]string
// @Security     BearerAuth
// @Failure      401  {object}  map[string]string
// @Router       /api/v1/work-orders/{id}/consumptions [get]
func (h *Handler) listConsumptions(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	records, err := h.svc.ListConsumptions(c.Request.Context(), id)
	if err != nil {
		httpkit.Error(c, err)
		return
	}
	c.JSON(http.StatusOK, records)
}

// assign godoc
//
// @Summary      Assign a PLANNED work order to a CNC operator
// @Tags         production
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id    path      string  true  "work order id (uuid)"
// @Param        body  body      object  true  "payload"  SchemaExample({"user_id":"<uuid>"})
// @Success      200   {object}  WorkOrder
// @Failure      400   {object}  map[string]string
// @Failure      403   {object}  map[string]string
// @Failure      404   {object}  map[string]string
// @Failure      412   {object}  map[string]string
// @Router       /api/v1/work-orders/{id}/assign [post]
func (h *Handler) assign(c *gin.Context) {
	woID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	var body struct {
		UserID uuid.UUID `json:"user_id"`
	}
	if !httpkit.Bind(c, &body) {
		return
	}
	if body.UserID == uuid.Nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "user_id is required"})
		return
	}
	wo, err := h.svc.AssignWorkOrder(c.Request.Context(), AssignWorkOrderInput{
		WorkOrderID: woID,
		UserID:      body.UserID,
	})
	if err != nil {
		httpkit.Error(c, err)
		return
	}
	c.JSON(http.StatusOK, wo)
}

// suggestAssignment godoc
//
// @Summary      Suggest the least-busy CNC operator for a work order
// @Tags         production
// @Produce      json
// @Security     BearerAuth
// @Param        id  path      string  true  "work order id (uuid)"
// @Success      200 {object}  SuggestAssignmentResult
// @Failure      400 {object}  map[string]string
// @Failure      403 {object}  map[string]string
// @Failure      404 {object}  map[string]string
// @Router       /api/v1/work-orders/{id}/suggest-assignment [post]
func (h *Handler) suggestAssignment(c *gin.Context) {
	woID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	result, err := h.svc.SuggestAssignment(c.Request.Context(), woID)
	if err != nil {
		httpkit.Error(c, err)
		return
	}
	c.JSON(http.StatusOK, result)
}

// listMine godoc
//
// @Summary      List work orders assigned to the authenticated CNC operator
// @Tags         production
// @Produce      json
// @Security     BearerAuth
// @Success      200 {array}   WorkOrder
// @Failure      401 {object}  map[string]string
// @Failure      403 {object}  map[string]string
// @Router       /api/v1/work-orders/mine [get]
func (h *Handler) listMine(c *gin.Context) {
	id, ok := auth.FromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	userID, err := uuid.Parse(id.UserID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "invalid user id in token"})
		return
	}
	wos, err := h.svc.ListWorkOrdersByAssignee(c.Request.Context(), userID)
	if err != nil {
		httpkit.Error(c, err)
		return
	}
	c.JSON(http.StatusOK, wos)
}
