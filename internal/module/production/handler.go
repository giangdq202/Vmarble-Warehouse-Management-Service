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
	rg.POST("/work-orders", auth.RequireRole(auth.RolePlanner, auth.RoleCNCManager, auth.RoleAdmin), h.create)
	rg.GET("/work-orders", h.list)
	// /mine must be registered before /:id to avoid Gin treating "mine" as an id param.
	rg.GET("/work-orders/mine", auth.RequireRole(auth.RoleCNC), h.listMine)
	rg.GET("/work-orders/:id", h.get)
	rg.POST("/work-orders/:id/advance", auth.RequireRole(auth.RoleCNC, auth.RoleCNCManager, auth.RoleWarehouse, auth.RoleForeman), h.advance)
	rg.POST("/work-orders/:id/consumptions", auth.RequireRole(auth.RoleWarehouse, auth.RoleForeman), h.recordConsumption)
	rg.GET("/work-orders/:id/consumptions", h.listConsumptions)
	rg.POST("/work-orders/:id/assign", auth.RequireRole(auth.RoleCNCManager), h.assign)
	rg.POST("/work-orders/:id/suggest-assignment", auth.RequireRole(auth.RoleCNCManager), h.suggestAssignment)
	rg.POST("/work-orders/:id/estimated-hours", auth.RequireRole(auth.RoleCNCManager, auth.RolePlanner), h.setEstimatedHours)
	rg.POST("/work-orders/:id/assign-slot", auth.RequireRole(auth.RoleCNCManager, auth.RolePlanner), h.assignSlot)
	rg.POST("/work-orders/:id/unassign-slot", auth.RequireRole(auth.RoleCNCManager, auth.RolePlanner), h.unassignSlot)
	rg.GET("/work-orders/:id/suggest-schedule", auth.RequireRole(auth.RoleCNCManager, auth.RolePlanner), h.suggestSchedule)

	rg.POST("/machines", auth.RequireRole(auth.RoleAdmin, auth.RoleCNCManager), h.createMachine)
	rg.GET("/machines", h.listMachines)
	rg.GET("/machines/:id", h.getMachine)
	rg.DELETE("/machines/:id", auth.RequireRole(auth.RoleAdmin), h.deactivateMachine)
	rg.POST("/machines/:id/slots", auth.RequireRole(auth.RoleCNCManager, auth.RolePlanner), h.createSlot)
	rg.GET("/machines/:id/slots", h.listSlots)
	rg.GET("/slots/:slotID", h.getSlot)
	rg.DELETE("/slots/:slotID", auth.RequireRole(auth.RoleCNCManager, auth.RolePlanner), h.deleteSlot)
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
// @Param        plan_id  query     string  false  "filter by plan id (uuid)"
// @Param        page     query     int     false  "page number (default 1)"
// @Param        limit    query     int     false  "items per page (default 10, max 100)"
// @Param        status   query     string  false  "filter by status: PLANNED, IN_CUTTING, IN_PROCESSING, COMPLETED, COSTED"
// @Param        sort_by  query     string  false  "sort column: created_at, status (default created_at)"
// @Param        order    query     string  false  "sort direction: asc, desc (default desc)"
// @Success      200      {object}  httpkit.PagedResult[WorkOrder]
// @Failure      400      {object}  map[string]string
// @Failure      500      {object}  map[string]string
// @Security     BearerAuth
// @Failure      401  {object}  map[string]string
// @Router       /api/v1/work-orders [get]
func (h *Handler) list(c *gin.Context) {
	var planID *uuid.UUID
	if planIDStr := c.Query("plan_id"); planIDStr != "" {
		parsed, err := uuid.Parse(planIDStr)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid plan_id"})
			return
		}
		planID = &parsed
	}

	p := httpkit.BindPageParams(c)
	status := c.Query("status")
	result, err := h.svc.ListWorkOrders(c.Request.Context(), p, status, planID)
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
	// Populate CallerID from JWT so service can enforce operator identity check.
	if identity, ok := auth.FromContext(c); ok {
		if callerID, err := uuid.Parse(identity.UserID); err == nil {
			in.CallerID = &callerID
		}
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

// setEstimatedHours godoc
//
// @Summary      Set estimated hours for scheduling
// @Tags         production
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id    path      string                  true  "work order id (uuid)"
// @Param        body  body      SetEstimatedHoursInput  true  "payload"
// @Success      200   {object}  WorkOrder
// @Failure      400   {object}  map[string]string
// @Failure      404   {object}  map[string]string
// @Router       /api/v1/work-orders/{id}/estimated-hours [post]
func (h *Handler) setEstimatedHours(c *gin.Context) {
	woID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	var in SetEstimatedHoursInput
	if !httpkit.Bind(c, &in) {
		return
	}
	in.WorkOrderID = woID
	wo, err := h.svc.SetEstimatedHours(c.Request.Context(), in)
	if err != nil {
		httpkit.Error(c, err)
		return
	}
	c.JSON(http.StatusOK, wo)
}

// assignSlot godoc
//
// @Summary      Assign a machine shift slot to a work order
// @Tags         production
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id    path      string          true  "work order id (uuid)"
// @Param        body  body      AssignSlotInput true  "payload"
// @Success      200   {object}  WorkOrder
// @Failure      400   {object}  map[string]string
// @Failure      404   {object}  map[string]string
// @Failure      412   {object}  map[string]string
// @Router       /api/v1/work-orders/{id}/assign-slot [post]
func (h *Handler) assignSlot(c *gin.Context) {
	woID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	var in AssignSlotInput
	if !httpkit.Bind(c, &in) {
		return
	}
	in.WorkOrderID = woID
	wo, err := h.svc.AssignSlot(c.Request.Context(), in)
	if err != nil {
		httpkit.Error(c, err)
		return
	}
	c.JSON(http.StatusOK, wo)
}

// unassignSlot godoc
//
// @Summary      Remove machine slot assignment from a work order
// @Tags         production
// @Produce      json
// @Security     BearerAuth
// @Param        id  path      string  true  "work order id (uuid)"
// @Success      200 {object}  WorkOrder
// @Failure      400 {object}  map[string]string
// @Failure      404 {object}  map[string]string
// @Router       /api/v1/work-orders/{id}/unassign-slot [post]
func (h *Handler) unassignSlot(c *gin.Context) {
	woID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	wo, err := h.svc.UnassignSlot(c.Request.Context(), woID)
	if err != nil {
		httpkit.Error(c, err)
		return
	}
	c.JSON(http.StatusOK, wo)
}

// suggestSchedule godoc
//
// @Summary      Suggest available machine shift slots for a work order
// @Tags         production
// @Produce      json
// @Security     BearerAuth
// @Param        id  path      string  true  "work order id (uuid)"
// @Success      200 {array}   ScheduleSuggestion
// @Failure      400 {object}  map[string]string
// @Failure      412 {object}  map[string]string
// @Router       /api/v1/work-orders/{id}/suggest-schedule [get]
func (h *Handler) suggestSchedule(c *gin.Context) {
	woID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	suggestions, err := h.svc.SuggestSchedule(c.Request.Context(), woID)
	if err != nil {
		httpkit.Error(c, err)
		return
	}
	c.JSON(http.StatusOK, suggestions)
}

// createMachine godoc
//
// @Summary      Create a CNC machine
// @Tags         production
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        body  body      CreateMachineInput  true  "payload"
// @Success      201   {object}  Machine
// @Failure      400   {object}  map[string]string
// @Router       /api/v1/machines [post]
func (h *Handler) createMachine(c *gin.Context) {
	var in CreateMachineInput
	if !httpkit.Bind(c, &in) {
		return
	}
	m, err := h.svc.CreateMachine(c.Request.Context(), in)
	if err != nil {
		httpkit.Error(c, err)
		return
	}
	c.JSON(http.StatusCreated, m)
}

// listMachines godoc
//
// @Summary      List all machines
// @Tags         production
// @Produce      json
// @Security     BearerAuth
// @Success      200 {array}   Machine
// @Router       /api/v1/machines [get]
func (h *Handler) listMachines(c *gin.Context) {
	machines, err := h.svc.ListMachines(c.Request.Context())
	if err != nil {
		httpkit.Error(c, err)
		return
	}
	c.JSON(http.StatusOK, machines)
}

// getMachine godoc
//
// @Summary      Get machine by ID
// @Tags         production
// @Produce      json
// @Security     BearerAuth
// @Param        id  path      string  true  "machine id (uuid)"
// @Success      200 {object}  Machine
// @Failure      404 {object}  map[string]string
// @Router       /api/v1/machines/{id} [get]
func (h *Handler) getMachine(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	m, err := h.svc.GetMachine(c.Request.Context(), id)
	if err != nil {
		httpkit.Error(c, err)
		return
	}
	c.JSON(http.StatusOK, m)
}

// deactivateMachine godoc
//
// @Summary      Deactivate a machine
// @Tags         production
// @Produce      json
// @Security     BearerAuth
// @Param        id  path      string  true  "machine id (uuid)"
// @Success      204
// @Failure      404 {object}  map[string]string
// @Router       /api/v1/machines/{id} [delete]
func (h *Handler) deactivateMachine(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	if err := h.svc.DeactivateMachine(c.Request.Context(), id); err != nil {
		httpkit.Error(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// createSlot godoc
//
// @Summary      Create a machine shift slot
// @Tags         production
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id    path      string          true  "machine id (uuid)"
// @Param        body  body      CreateSlotInput true  "payload"
// @Success      201   {object}  MachineShiftSlot
// @Failure      400   {object}  map[string]string
// @Router       /api/v1/machines/{id}/slots [post]
func (h *Handler) createSlot(c *gin.Context) {
	machineID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid machine id"})
		return
	}
	var in CreateSlotInput
	if !httpkit.Bind(c, &in) {
		return
	}
	in.MachineID = machineID
	sl, err := h.svc.CreateSlot(c.Request.Context(), in)
	if err != nil {
		httpkit.Error(c, err)
		return
	}
	c.JSON(http.StatusCreated, sl)
}

// listSlots godoc
//
// @Summary      List slots for a machine in a date range
// @Tags         production
// @Produce      json
// @Security     BearerAuth
// @Param        id    path      string  true   "machine id (uuid)"
// @Param        from  query     string  true   "start date (RFC3339 or YYYY-MM-DD)"
// @Param        to    query     string  true   "end date (RFC3339 or YYYY-MM-DD)"
// @Success      200   {array}   MachineShiftSlot
// @Failure      400   {object}  map[string]string
// @Router       /api/v1/machines/{id}/slots [get]
func (h *Handler) listSlots(c *gin.Context) {
	machineID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid machine id"})
		return
	}
	from, to, ok := httpkit.ParseDateRange(c)
	if !ok {
		return
	}
	slots, err := h.svc.ListSlots(c.Request.Context(), machineID, from, to)
	if err != nil {
		httpkit.Error(c, err)
		return
	}
	c.JSON(http.StatusOK, slots)
}

// getSlot godoc
//
// @Summary      Get a machine shift slot by ID
// @Tags         production
// @Produce      json
// @Security     BearerAuth
// @Param        slotID  path      string  true  "slot id (uuid)"
// @Success      200     {object}  MachineShiftSlot
// @Failure      404     {object}  map[string]string
// @Router       /api/v1/slots/{slotID} [get]
func (h *Handler) getSlot(c *gin.Context) {
	slotID, err := uuid.Parse(c.Param("slotID"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid slot id"})
		return
	}
	sl, err := h.svc.GetSlot(c.Request.Context(), slotID)
	if err != nil {
		httpkit.Error(c, err)
		return
	}
	c.JSON(http.StatusOK, sl)
}

// deleteSlot godoc
//
// @Summary      Delete a machine shift slot
// @Tags         production
// @Produce      json
// @Security     BearerAuth
// @Param        slotID  path      string  true  "slot id (uuid)"
// @Success      204
// @Failure      404     {object}  map[string]string
// @Router       /api/v1/slots/{slotID} [delete]
func (h *Handler) deleteSlot(c *gin.Context) {
	slotID, err := uuid.Parse(c.Param("slotID"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid slot id"})
		return
	}
	if err := h.svc.DeleteSlot(c.Request.Context(), slotID); err != nil {
		httpkit.Error(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}
