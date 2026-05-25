package sales

import (
	"net/http"
	"strconv"
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

// Register wires the sales endpoints. All write paths are gated to PLANNER+
// per the persona-tier convention; reads stay open to any authenticated user
// (the auth middleware already runs upstream of this group).
func (h *Handler) Register(rg *gin.RouterGroup) {
	rg.POST("/customers", auth.RequirePlannerUp(), h.createCustomer)
	rg.GET("/customers", h.listCustomers)
	rg.PATCH("/customers/:id", auth.RequirePlannerUp(), h.patchCustomer)

	rg.POST("/sales-orders", auth.RequirePlannerUp(), h.createSO)
	rg.GET("/sales-orders", h.listSOs)
	rg.GET("/sales-orders/:id", h.getSO)
	rg.PATCH("/sales-orders/:id", auth.RequirePlannerUp(), h.patchSO)
	rg.POST("/sales-orders/:id/confirm", auth.RequirePlannerUp(), h.confirmSO)
	rg.POST("/sales-orders/:id/cancel", auth.RequirePlannerUp(), h.cancelSO)
	rg.POST("/sales-orders/:id/split-to-plan", auth.RequirePlannerUp(), h.splitToPlan)
}

// ── Customer endpoints ───────────────────────────────────────────────────────

// createCustomer godoc
//
// @Summary      Create customer
// @Tags         sales
// @Accept       json
// @Produce      json
// @Param        body  body      CreateCustomerInput  true  "payload"
// @Success      201   {object}  Customer
// @Failure      400   {object}  map[string]string
// @Security     BearerAuth
// @Failure      401  {object}  map[string]string
// @Router       /api/v1/customers [post]
func (h *Handler) createCustomer(c *gin.Context) {
	var in CreateCustomerInput
	if !httpkit.Bind(c, &in) {
		return
	}
	cust, err := h.svc.CreateCustomer(c.Request.Context(), in)
	if err != nil {
		httpkit.Error(c, err)
		return
	}
	c.JSON(http.StatusCreated, cust)
}

// listCustomers godoc
//
// @Summary      List customers
// @Tags         sales
// @Produce      json
// @Param        page         query     int     false  "page number (default 1)"
// @Param        limit        query     int     false  "items per page (default 10, max 100)"
// @Param        active_only  query     bool    false  "only active customers (default false)"
// @Success      200  {object}  httpkit.PagedResult[Customer]
// @Failure      400  {object}  map[string]string
// @Failure      500  {object}  map[string]string
// @Security     BearerAuth
// @Failure      401  {object}  map[string]string
// @Router       /api/v1/customers [get]
func (h *Handler) listCustomers(c *gin.Context) {
	p := httpkit.BindPageParams(c)
	activeOnly, _ := strconv.ParseBool(c.DefaultQuery("active_only", "false"))
	res, err := h.svc.ListCustomers(c.Request.Context(), p, activeOnly)
	if err != nil {
		httpkit.Error(c, err)
		return
	}
	c.JSON(http.StatusOK, res)
}

// patchCustomerRequest mirrors PatchCustomerInput on the wire so JSON omitted
// fields stay nil and present-but-empty fields are honoured by the service.
type patchCustomerRequest struct {
	Name          *string `json:"name,omitempty"`
	CountryCode   *string `json:"country_code,omitempty"`
	Address       *string `json:"address,omitempty"`
	ContactPerson *string `json:"contact_person,omitempty"`
	ContactPhone  *string `json:"contact_phone,omitempty"`
	ContactEmail  *string `json:"contact_email,omitempty"`
	IsActive      *bool   `json:"is_active,omitempty"`
}

// patchCustomer godoc
//
// @Summary      Patch customer
// @Tags         sales
// @Accept       json
// @Produce      json
// @Param        id    path      string                true  "customer id (uuid)"
// @Param        body  body      patchCustomerRequest  true  "fields to update"
// @Success      200   {object}  Customer
// @Failure      400   {object}  map[string]string
// @Failure      404   {object}  map[string]string
// @Security     BearerAuth
// @Failure      401  {object}  map[string]string
// @Router       /api/v1/customers/{id} [patch]
func (h *Handler) patchCustomer(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	var body patchCustomerRequest
	if !httpkit.Bind(c, &body) {
		return
	}
	in := PatchCustomerInput{
		ID:            id,
		Name:          body.Name,
		CountryCode:   body.CountryCode,
		Address:       body.Address,
		ContactPerson: body.ContactPerson,
		ContactPhone:  body.ContactPhone,
		ContactEmail:  body.ContactEmail,
		IsActive:      body.IsActive,
	}
	cust, err := h.svc.PatchCustomer(c.Request.Context(), in)
	if err != nil {
		httpkit.Error(c, err)
		return
	}
	c.JSON(http.StatusOK, cust)
}

// ── Sales order endpoints ───────────────────────────────────────────────────

// createSO godoc
//
// @Summary      Create sales order (DRAFT)
// @Tags         sales
// @Accept       json
// @Produce      json
// @Param        body  body      CreateSOInput  true  "payload"
// @Success      201   {object}  SalesOrder
// @Failure      400   {object}  map[string]string
// @Security     BearerAuth
// @Failure      401  {object}  map[string]string
// @Router       /api/v1/sales-orders [post]
func (h *Handler) createSO(c *gin.Context) {
	var in CreateSOInput
	if !httpkit.Bind(c, &in) {
		return
	}
	in.CreatedBy = callerID(c)
	so, err := h.svc.CreateSO(c.Request.Context(), in)
	if err != nil {
		httpkit.Error(c, err)
		return
	}
	c.JSON(http.StatusCreated, so)
}

// listSOs godoc
//
// @Summary      List sales orders
// @Tags         sales
// @Produce      json
// @Param        page         query     int     false  "page number (default 1)"
// @Param        limit        query     int     false  "items per page (default 10, max 100)"
// @Param        status       query     string  false  "filter by status (DRAFT, CONFIRMED, IN_PRODUCTION, PARTIALLY_SHIPPED, SHIPPED, CANCELLED)"
// @Param        customer_id  query     string  false  "filter by customer uuid"
// @Success      200  {object}  httpkit.PagedResult[SalesOrder]
// @Failure      400  {object}  map[string]string
// @Failure      500  {object}  map[string]string
// @Security     BearerAuth
// @Failure      401  {object}  map[string]string
// @Router       /api/v1/sales-orders [get]
func (h *Handler) listSOs(c *gin.Context) {
	p := httpkit.BindPageParams(c)
	f := SOListFilter{Status: c.Query("status")}
	if raw := c.Query("customer_id"); raw != "" {
		cid, err := uuid.Parse(raw)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid customer_id"})
			return
		}
		f.CustomerID = &cid
	}
	res, err := h.svc.ListSOs(c.Request.Context(), p, f)
	if err != nil {
		httpkit.Error(c, err)
		return
	}
	c.JSON(http.StatusOK, res)
}

// getSO godoc
//
// @Summary      Get sales order with lines
// @Tags         sales
// @Produce      json
// @Param        id   path      string  true  "sales order id (uuid)"
// @Success      200  {object}  SalesOrder
// @Failure      400  {object}  map[string]string
// @Failure      404  {object}  map[string]string
// @Security     BearerAuth
// @Failure      401  {object}  map[string]string
// @Router       /api/v1/sales-orders/{id} [get]
func (h *Handler) getSO(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	so, err := h.svc.GetSO(c.Request.Context(), id)
	if err != nil {
		httpkit.Error(c, err)
		return
	}
	c.JSON(http.StatusOK, so)
}

// patchSORequest is the wire shape for PATCH /sales-orders/:id. Pointer
// fields stay nil when omitted; ClearExpectedShipDate is a separate boolean
// because JSON cannot distinguish "omitted" from "explicitly null" without
// custom unmarshaling.
type patchSORequest struct {
	Incoterm              *string              `json:"incoterm,omitempty"`
	PortOfLoading         *string              `json:"port_of_loading,omitempty"`
	PortOfDischarge       *string              `json:"port_of_discharge,omitempty"`
	Currency              *string              `json:"currency,omitempty"`
	ExpectedShipDate      *string              `json:"expected_ship_date,omitempty"`
	ClearExpectedShipDate bool                 `json:"clear_expected_ship_date,omitempty"`
	Note                  *string              `json:"note,omitempty"`
	Lines                 *[]CreateSOLineInput `json:"lines,omitempty"`
}

// patchSO godoc
//
// @Summary      Patch sales order (DRAFT only)
// @Tags         sales
// @Accept       json
// @Produce      json
// @Param        id    path      string          true  "sales order id (uuid)"
// @Param        body  body      patchSORequest  true  "fields to update"
// @Success      200   {object}  SalesOrder
// @Failure      400   {object}  map[string]string
// @Failure      404   {object}  map[string]string
// @Failure      409   {object}  map[string]string
// @Security     BearerAuth
// @Failure      401  {object}  map[string]string
// @Router       /api/v1/sales-orders/{id} [patch]
func (h *Handler) patchSO(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	var body patchSORequest
	if !httpkit.Bind(c, &body) {
		return
	}

	in := PatchSOInput{
		ID:                    id,
		Incoterm:              body.Incoterm,
		PortOfLoading:         body.PortOfLoading,
		PortOfDischarge:       body.PortOfDischarge,
		Currency:              body.Currency,
		Note:                  body.Note,
		Lines:                 body.Lines,
		ClearExpectedShipDate: body.ClearExpectedShipDate,
	}
	if body.ExpectedShipDate != nil {
		t, err := parseFlexibleTimestamp(*body.ExpectedShipDate)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid expected_ship_date, use YYYY-MM-DD or RFC3339"})
			return
		}
		in.ExpectedShipDate = &t
	}

	so, err := h.svc.PatchSO(c.Request.Context(), in)
	if err != nil {
		httpkit.Error(c, err)
		return
	}
	c.JSON(http.StatusOK, so)
}

// confirmSO godoc
//
// @Summary      Confirm sales order (DRAFT → CONFIRMED)
// @Tags         sales
// @Produce      json
// @Param        id   path      string  true  "sales order id (uuid)"
// @Success      200  {object}  map[string]string
// @Failure      400  {object}  map[string]string
// @Failure      404  {object}  map[string]string
// @Failure      409  {object}  map[string]string
// @Security     BearerAuth
// @Failure      401  {object}  map[string]string
// @Router       /api/v1/sales-orders/{id}/confirm [post]
func (h *Handler) confirmSO(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	if err := h.svc.ConfirmSO(c.Request.Context(), id); err != nil {
		httpkit.Error(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "confirmed"})
}

// cancelSORequest carries the optional cancellation reason. Empty string is
// allowed — the service does not require a reason for sales-order cancels
// today, but the field is reserved for future audit-trail use.
type cancelSORequest struct {
	Reason string `json:"reason"`
}

// cancelSO godoc
//
// @Summary      Cancel sales order
// @Tags         sales
// @Accept       json
// @Produce      json
// @Param        id    path      string           true  "sales order id (uuid)"
// @Param        body  body      cancelSORequest  false "cancellation reason"
// @Success      200   {object}  map[string]string
// @Failure      400   {object}  map[string]string
// @Failure      404   {object}  map[string]string
// @Failure      409   {object}  map[string]string
// @Security     BearerAuth
// @Failure      401  {object}  map[string]string
// @Router       /api/v1/sales-orders/{id}/cancel [post]
func (h *Handler) cancelSO(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	var body cancelSORequest
	if c.Request.ContentLength > 0 {
		if err := c.ShouldBindJSON(&body); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body"})
			return
		}
	}
	in := CancelSOInput{ID: id, Reason: body.Reason, ActorID: callerID(c)}
	if err := h.svc.CancelSO(c.Request.Context(), in); err != nil {
		httpkit.Error(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "cancelled"})
}

// splitToPlanRequest is the wire shape for POST /sales-orders/:id/split-to-plan.
// Deadline is optional on the wire (empty string = inherit SO.expected_ship_date);
// the service layer applies the inheritance.
type splitToPlanRequest struct {
	Allocations []SplitAllocation `json:"allocations"`
	Deadline    string            `json:"deadline,omitempty"`
}

// splitToPlan godoc
//
// @Summary      Split sales order lines into a production plan + work orders
// @Tags         sales
// @Accept       json
// @Produce      json
// @Param        id    path      string              true  "sales order id (uuid)"
// @Param        body  body      splitToPlanRequest  true  "allocations + optional deadline"
// @Success      201   {object}  SplitToPlanResult
// @Failure      400   {object}  map[string]string
// @Failure      404   {object}  map[string]string
// @Failure      409   {object}  map[string]string
// @Failure      412   {object}  map[string]string
// @Security     BearerAuth
// @Failure      401  {object}  map[string]string
// @Router       /api/v1/sales-orders/{id}/split-to-plan [post]
func (h *Handler) splitToPlan(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	var body splitToPlanRequest
	if !httpkit.Bind(c, &body) {
		return
	}
	in := SplitToPlanInput{
		SalesOrderID: id,
		Allocations:  body.Allocations,
		ActorID:      callerID(c),
	}
	if body.Deadline != "" {
		t, err := parseFlexibleTimestamp(body.Deadline)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid deadline, use YYYY-MM-DD or RFC3339"})
			return
		}
		in.Deadline = t
	}
	res, err := h.svc.SplitToPlan(c.Request.Context(), in)
	if err != nil {
		httpkit.Error(c, err)
		return
	}
	c.JSON(http.StatusCreated, res)
}

// ── helpers ──────────────────────────────────────────────────────────────────

// callerID extracts the caller's UUID from the auth identity. Returns
// uuid.Nil when the identity is missing or malformed — the auth middleware
// already rejects unauthenticated requests, so reaching this branch with a
// nil id means the token survived validation but carried a non-UUID
// user_id, which we treat as audit-only (the service still records the
// actor field, just blank).
func callerID(c *gin.Context) uuid.UUID {
	id, ok := auth.FromContext(c)
	if !ok {
		return uuid.Nil
	}
	uid, err := uuid.Parse(id.UserID)
	if err != nil {
		return uuid.Nil
	}
	return uid
}

// parseFlexibleTimestamp accepts either YYYY-MM-DD (interpreted in UTC at
// 00:00) or RFC3339. Mirrors the planning-handler pattern so deadlines
// expressed by ops staff using a date picker drop in seamlessly.
func parseFlexibleTimestamp(s string) (time.Time, error) {
	if t, err := time.Parse(time.DateOnly, s); err == nil {
		return t, nil
	}
	return time.Parse(time.RFC3339, s)
}
