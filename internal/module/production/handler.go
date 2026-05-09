package production

import (
	"net/http"
	"time"

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

var workOrderFilterLoc = func() *time.Location {
	loc, err := time.LoadLocation("Asia/Ho_Chi_Minh")
	if err != nil {
		return time.FixedZone("Asia/Ho_Chi_Minh", 7*60*60)
	}
	return loc
}()

func parseWorkOrderCreatedAtFilter(c *gin.Context) (from, to *time.Time, ok bool) {
	dateStr := c.Query("date")
	fromStr := c.Query("from")
	toStr := c.Query("to")

	if dateStr != "" && (fromStr != "" || toStr != "") {
		c.JSON(http.StatusBadRequest, gin.H{"error": "date cannot be combined with from/to"})
		return nil, nil, false
	}
	if dateStr != "" {
		start, end, err := parseLocalDateBounds(dateStr, workOrderFilterLoc)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid date, use YYYY-MM-DD"})
			return nil, nil, false
		}
		return &start, &end, true
	}
	if fromStr == "" && toStr == "" {
		return nil, nil, true
	}
	if fromStr == "" || toStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "from and to query parameters are required together"})
		return nil, nil, false
	}

	start, err := parseDateFilterBoundary(fromStr, workOrderFilterLoc, false)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid from date, use YYYY-MM-DD or RFC3339"})
		return nil, nil, false
	}
	end, err := parseDateFilterBoundary(toStr, workOrderFilterLoc, true)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid to date, use YYYY-MM-DD or RFC3339"})
		return nil, nil, false
	}
	if !start.Before(end) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "from must be before to"})
		return nil, nil, false
	}
	return &start, &end, true
}

func parseLocalDateBounds(s string, loc *time.Location) (time.Time, time.Time, error) {
	day, err := time.ParseInLocation(time.DateOnly, s, loc)
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	return day, day.AddDate(0, 0, 1), nil
}

func parseDateFilterBoundary(s string, loc *time.Location, inclusiveDayEnd bool) (time.Time, error) {
	if day, err := time.ParseInLocation(time.DateOnly, s, loc); err == nil {
		if inclusiveDayEnd {
			return day.AddDate(0, 0, 1), nil
		}
		return day, nil
	}
	return time.Parse(time.RFC3339, s)
}

// parseWorkOrderAssignedFilter parses the ?assigned query param.
//
//	assigned=null  → assignedNull=true (filter assigned_to IS NULL)
//	assigned=<id>  → assignedTo=&id   (filter assigned_to = id)
//	assigned=""    → both zero (no filter)
//
// On a malformed UUID it writes 400 and returns ok=false.
func parseWorkOrderAssignedFilter(c *gin.Context) (assignedNull bool, assignedTo *uuid.UUID, ok bool) {
	v := c.Query("assigned")
	if v == "" {
		return false, nil, true
	}
	if v == "null" {
		return true, nil, true
	}
	parsed, err := uuid.Parse(v)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid assigned, expected 'null' or a user uuid"})
		return false, nil, false
	}
	return false, &parsed, true
}

func (h *Handler) Register(rg *gin.RouterGroup) {
	rg.POST("/work-orders", auth.RequireRole(auth.RolePlanner, auth.RoleCNCManager, auth.RoleAdmin), h.create)
	rg.GET("/work-orders", h.list)
	// /mine must be registered before /:id to avoid Gin treating "mine" as an id param.
	rg.GET("/work-orders/mine", auth.RequireRole(auth.RoleCNC), h.listMine)
	rg.GET("/work-orders/:id", h.get)
	rg.POST("/work-orders/:id/advance", auth.RequireRole(auth.RoleCNC, auth.RoleCNCManager, auth.RoleWarehouse, auth.RoleForeman, auth.RoleAdmin), h.advance)
	rg.POST("/work-orders/:id/consumptions", auth.RequireRole(auth.RoleWarehouse, auth.RoleForeman, auth.RoleAdmin), h.recordConsumption)
	rg.GET("/work-orders/:id/consumptions", h.listConsumptions)
	rg.POST("/work-orders/:id/assign", auth.RequireRole(auth.RoleCNCManager, auth.RoleAdmin), h.assign)
	rg.POST("/work-orders/:id/suggest-assignment", auth.RequireRole(auth.RoleCNCManager, auth.RoleAdmin), h.suggestAssignment)
	rg.POST("/work-orders/:id/estimated-hours", auth.RequireRole(auth.RoleCNCManager, auth.RolePlanner, auth.RoleAdmin), h.setEstimatedHours)
	rg.POST("/work-orders/:id/assign-slot", auth.RequireRole(auth.RoleCNCManager, auth.RolePlanner, auth.RoleAdmin), h.assignSlot)
	rg.POST("/work-orders/:id/unassign-slot", auth.RequireRole(auth.RoleCNCManager, auth.RolePlanner, auth.RoleAdmin), h.unassignSlot)
	rg.GET("/work-orders/:id/suggest-schedule", auth.RequireRole(auth.RoleCNCManager, auth.RolePlanner, auth.RoleAdmin), h.suggestSchedule)

	rg.POST("/machines", auth.RequireRole(auth.RoleAdmin, auth.RoleCNCManager), h.createMachine)
	rg.GET("/machines", h.listMachines)
	rg.GET("/machines/:id", h.getMachine)
	rg.DELETE("/machines/:id", auth.RequireRole(auth.RoleAdmin), h.deactivateMachine)
	rg.POST("/machines/:id/slots", auth.RequireRole(auth.RoleCNCManager, auth.RolePlanner, auth.RoleAdmin), h.createSlot)
	rg.GET("/machines/:id/slots", h.listSlots)
	rg.GET("/slots/:slotID", h.getSlot)
	rg.DELETE("/slots/:slotID", auth.RequireRole(auth.RoleCNCManager, auth.RolePlanner, auth.RoleAdmin), h.deleteSlot)
}

// createWorkOrder godoc
//
// @Summary      Create work order
// @Description  Creates a PLANNED work order. If the SKU has dimensions and there
// @Description  is at least one fitting remnant in stock, the system writes a
// @Description  REMNANT_BYPASSED row to inventory_audit_log to record that the
// @Description  planner did not allocate any of the suggestions (BR-K05). Provide
// @Description  `bypass_reason` to attach a free-text note to that audit row.
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
	if identity, ok := auth.FromContext(c); ok {
		if callerID, err := uuid.Parse(identity.UserID); err == nil {
			in.CallerID = &callerID
		}
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
// @Param        date     query     string  false  "filter by local created date (Asia/Ho_Chi_Minh), format YYYY-MM-DD"
// @Param        from     query     string  false  "filter created_at from (RFC3339 or YYYY-MM-DD)"
// @Param        to       query     string  false  "filter created_at to (RFC3339 or YYYY-MM-DD); date-only means inclusive local day end"
// @Param        sort_by  query     string  false  "sort column: created_at, status (default created_at)"
// @Param        order    query     string  false  "sort direction: asc, desc (default desc)"
// @Param        preset   query     string  false  "operational preset: dashboard_default — shows PLANNED today/yesterday then active, excludes COMPLETED/COSTED; mutually exclusive with status/date/from/to/assigned filters"
// @Param        assigned query     string  false  "filter by assignment: 'null' for unassigned WOs, or a user UUID for WOs assigned to that user; mutually exclusive with preset"
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

	preset := c.Query("preset")
	if preset != "" && preset != "dashboard_default" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "unknown preset, supported values: dashboard_default"})
		return
	}

	assignedNull, assignedTo, ok := parseWorkOrderAssignedFilter(c)
	if !ok {
		return
	}

	if preset == "dashboard_default" {
		if c.Query("status") != "" || c.Query("date") != "" || c.Query("from") != "" || c.Query("to") != "" || c.Query("assigned") != "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "preset=dashboard_default cannot be combined with status, date, from, to, or assigned filters"})
			return
		}
		now := time.Now().In(workOrderFilterLoc)
		todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, workOrderFilterLoc)
		todayEnd := todayStart.AddDate(0, 0, 1)
		p := httpkit.BindPageParams(c)
		result, err := h.svc.ListWorkOrders(c.Request.Context(), p, WorkOrderListFilter{
			PlanID:          planID,
			DashboardPreset: true,
			TodayStart:      todayStart,
			TodayEnd:        todayEnd,
		})
		if err != nil {
			httpkit.Error(c, err)
			return
		}
		c.JSON(http.StatusOK, result)
		return
	}

	createdFrom, createdTo, ok := parseWorkOrderCreatedAtFilter(c)
	if !ok {
		return
	}

	p := httpkit.BindPageParams(c)
	result, err := h.svc.ListWorkOrders(c.Request.Context(), p, WorkOrderListFilter{
		Status:       c.Query("status"),
		PlanID:       planID,
		CreatedFrom:  createdFrom,
		CreatedTo:    createdTo,
		AssignedNull: assignedNull,
		AssignedTo:   assignedTo,
	})
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
// @Description  Transitions a work order. When advancing PLANNED→IN_CUTTING with `sheet_id`,
// @Description  the inventory module pre-assigns the sheet and rejects the call with 412 if
// @Description  remnant overflow is RED (≥15% of total stock). Admin callers may force the
// @Description  bypass by setting `bypass_overflow=true` together with a non-empty
// @Description  `bypass_reason`; the bypass is recorded in `inventory_audit_log` with
// @Description  action=OVERFLOW_BYPASSED.
// @Tags         production
// @Accept       json
// @Produce      json
// @Param        id    path      string             true  "work order id (uuid)"
// @Param        body  body      AdvanceStatusInput true  "payload"
// @Success      200   {object}  map[string]string
// @Failure      400   {object}  map[string]string  "invalid input (e.g. bypass_overflow=true without bypass_reason)"
// @Failure      409   {object}  map[string]string  "invalid status transition"
// @Failure      412   {object}  map[string]string  "precondition failed (operator mismatch, costing missing, remnant overflow lock, non-admin bypass)"
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
	// Populate caller identity from JWT so service can enforce operator identity
	// checks while still allowing admin super-user override.
	if identity, ok := auth.FromContext(c); ok {
		in.CallerRole = identity.Role
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
