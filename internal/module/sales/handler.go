package sales

import (
	"encoding/csv"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
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

	// Customer SKU mappings (#304). Reads stay open like the rest of sales;
	// writes (including bulk-import) require PLANNER+ which collapses to
	// planner / accountant / admin per the persona-tier convention.
	rg.POST("/customer-sku-mappings", auth.RequirePlannerUp(), h.createCustomerSKUMapping)
	rg.GET("/customer-sku-mappings", h.listCustomerSKUMappings)
	rg.PATCH("/customer-sku-mappings/:customerID/:code", auth.RequirePlannerUp(), h.patchCustomerSKUMapping)
	rg.DELETE("/customer-sku-mappings/:customerID/:code", auth.RequirePlannerUp(), h.deleteCustomerSKUMapping)
	rg.POST("/customer-sku-mappings/bulk-import", auth.RequirePlannerUp(), h.bulkImportCustomerSKUMappings)
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

// ── Customer SKU mappings (#304) ─────────────────────────────────────────────

type createCustomerSKUMappingRequest struct {
	CustomerID      string `json:"customer_id" binding:"required"`
	CustomerSKUCode string `json:"customer_sku_code" binding:"required"`
	SKUID           string `json:"sku_id" binding:"required"`
	Notes           string `json:"notes"`
}

type patchCustomerSKUMappingRequest struct {
	SKUID *string `json:"sku_id"`
	Notes *string `json:"notes"`
}

// createCustomerSKUMapping godoc
//
// @Summary      Create customer SKU mapping
// @Description  Bridges a customer-facing SKU code (as it appears in the customer's packing-list Excel) with the internal catalog SKU id. (#304, BR-CSM02)
// @Tags         sales
// @Accept       json
// @Produce      json
// @Param        body  body      createCustomerSKUMappingRequest  true  "payload"
// @Success      201   {object}  CustomerSKUMapping
// @Failure      400   {object}  map[string]string
// @Security     BearerAuth
// @Failure      401  {object}  map[string]string
// @Router       /api/v1/customer-sku-mappings [post]
func (h *Handler) createCustomerSKUMapping(c *gin.Context) {
	var req createCustomerSKUMappingRequest
	if !httpkit.Bind(c, &req) {
		return
	}
	customerID, err := uuid.Parse(req.CustomerID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid customer_id"})
		return
	}
	skuID, err := uuid.Parse(req.SKUID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid sku_id"})
		return
	}
	m, err := h.svc.CreateCustomerSKUMapping(c.Request.Context(), CreateCustomerSKUMappingInput{
		CustomerID:      customerID,
		CustomerSKUCode: req.CustomerSKUCode,
		SKUID:           skuID,
		Notes:           req.Notes,
		ActorID:         callerID(c),
	})
	if err != nil {
		httpkit.Error(c, err)
		return
	}
	c.JSON(http.StatusCreated, m)
}

// listCustomerSKUMappings godoc
//
// @Summary      List customer SKU mappings
// @Tags         sales
// @Produce      json
// @Param        page         query     int     false  "page number (default 1)"
// @Param        limit        query     int     false  "items per page (default 10, max 100)"
// @Param        customer_id  query     string  false  "filter by customer id (uuid); omit for all customers"
// @Success      200  {object}  httpkit.PagedResult[CustomerSKUMapping]
// @Failure      400  {object}  map[string]string
// @Security     BearerAuth
// @Failure      401  {object}  map[string]string
// @Router       /api/v1/customer-sku-mappings [get]
func (h *Handler) listCustomerSKUMappings(c *gin.Context) {
	p := httpkit.BindPageParams(c)
	var f CustomerSKUMappingFilter
	if raw := c.Query("customer_id"); raw != "" {
		id, err := uuid.Parse(raw)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid customer_id"})
			return
		}
		f.CustomerID = &id
	}
	res, err := h.svc.ListCustomerSKUMappings(c.Request.Context(), p, f)
	if err != nil {
		httpkit.Error(c, err)
		return
	}
	c.JSON(http.StatusOK, res)
}

// patchCustomerSKUMapping godoc
//
// @Summary      Update customer SKU mapping
// @Description  Partial update; nil fields leave the column untouched. The (customer_id, customer_sku_code) PK is immutable — to rename the customer code, delete and re-create.
// @Tags         sales
// @Accept       json
// @Produce      json
// @Param        customerID  path      string                          true  "customer id (uuid)"
// @Param        code        path      string                          true  "customer SKU code (URL-encoded)"
// @Param        body        body      patchCustomerSKUMappingRequest  true  "patch payload"
// @Success      200         {object}  CustomerSKUMapping
// @Failure      400         {object}  map[string]string
// @Failure      404         {object}  map[string]string
// @Security     BearerAuth
// @Failure      401  {object}  map[string]string
// @Router       /api/v1/customer-sku-mappings/{customerID}/{code} [patch]
func (h *Handler) patchCustomerSKUMapping(c *gin.Context) {
	customerID, err := uuid.Parse(c.Param("customerID"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid customerID"})
		return
	}
	code := c.Param("code")
	var req patchCustomerSKUMappingRequest
	if !httpkit.Bind(c, &req) {
		return
	}
	in := PatchCustomerSKUMappingInput{
		CustomerID:      customerID,
		CustomerSKUCode: code,
		Notes:           req.Notes,
		ActorID:         callerID(c),
	}
	if req.SKUID != nil {
		skuID, err := uuid.Parse(*req.SKUID)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid sku_id"})
			return
		}
		in.SKUID = &skuID
	}
	m, err := h.svc.PatchCustomerSKUMapping(c.Request.Context(), in)
	if err != nil {
		httpkit.Error(c, err)
		return
	}
	c.JSON(http.StatusOK, m)
}

// deleteCustomerSKUMapping godoc
//
// @Summary      Delete customer SKU mapping
// @Tags         sales
// @Param        customerID  path  string  true  "customer id (uuid)"
// @Param        code        path  string  true  "customer SKU code (URL-encoded)"
// @Success      204
// @Failure      400  {object}  map[string]string
// @Failure      404  {object}  map[string]string
// @Security     BearerAuth
// @Failure      401  {object}  map[string]string
// @Router       /api/v1/customer-sku-mappings/{customerID}/{code} [delete]
func (h *Handler) deleteCustomerSKUMapping(c *gin.Context) {
	customerID, err := uuid.Parse(c.Param("customerID"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid customerID"})
		return
	}
	code := c.Param("code")
	if err := h.svc.DeleteCustomerSKUMapping(c.Request.Context(), DeleteCustomerSKUMappingInput{
		CustomerID:      customerID,
		CustomerSKUCode: code,
		ActorID:         callerID(c),
	}); err != nil {
		httpkit.Error(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// bulkImportCustomerSKUMappings godoc
//
// @Summary      Bulk-import customer SKU mappings from CSV
// @Description  Multipart upload with form fields `customer_id` (uuid) and `file` (CSV, UTF-8 with optional BOM). CSV header: `customer_sku_code,sku_id,notes`. Fail-all: any row error rolls the whole batch back and returns 422 with per-row errors.
// @Tags         sales
// @Accept       multipart/form-data
// @Produce      json
// @Param        customer_id  formData  string  true  "customer id (uuid)"
// @Param        file         formData  file    true  "CSV file"
// @Success      200          {object}  BulkImportResult
// @Failure      400          {object}  map[string]string
// @Failure      422          {object}  BulkImportResult
// @Security     BearerAuth
// @Failure      401  {object}  map[string]string
// @Router       /api/v1/customer-sku-mappings/bulk-import [post]
func (h *Handler) bulkImportCustomerSKUMappings(c *gin.Context) {
	customerID, err := uuid.Parse(c.PostForm("customer_id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid customer_id"})
		return
	}
	fh, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "file is required"})
		return
	}
	f, err := fh.Open()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "could not open uploaded file"})
		return
	}
	defer f.Close()

	rows, parseErrs, err := parseCustomerSKUMappingCSV(f)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if len(parseErrs) > 0 {
		c.JSON(http.StatusUnprocessableEntity, BulkImportResult{Errors: parseErrs})
		return
	}

	res, err := h.svc.BulkImportCustomerSKUMappings(c.Request.Context(), BulkImportCustomerSKUMappingsInput{
		CustomerID: customerID,
		Rows:       rows,
		ActorID:    callerID(c),
	})
	if err != nil {
		// Fail-all surface: when row errors are present, return them with 422
		// so the FE can render per-row toasts. Other errors fall through to
		// the standard httpkit.Error mapping.
		if len(res.Errors) > 0 {
			c.JSON(http.StatusUnprocessableEntity, res)
			return
		}
		httpkit.Error(c, err)
		return
	}
	c.JSON(http.StatusOK, res)
}

// parseCustomerSKUMappingCSV reads the uploaded file into BulkMappingRow
// slices. Strips a UTF-8 BOM (Excel VN exports include one) and validates the
// header row contains the three expected columns in any order.
//
// Per-row errors (malformed UUID, wrong column count) are returned via
// parseErrs without aborting the whole parse — the service layer also
// validates content (empty code, unknown SKU) and emits its own row errors,
// so the caller picks fail-all semantics by treating any non-empty error
// slice as a 422.
func parseCustomerSKUMappingCSV(r io.Reader) ([]BulkMappingRow, []BulkImportRowError, error) {
	reader := csv.NewReader(stripBOMReader(r))
	reader.FieldsPerRecord = -1 // tolerate trailing notes column

	header, err := reader.Read()
	if err != nil {
		if errors.Is(err, io.EOF) {
			return nil, nil, errors.New("empty CSV")
		}
		return nil, nil, err
	}
	idx, err := mappingHeaderIndex(header)
	if err != nil {
		return nil, nil, err
	}

	var (
		rows      []BulkMappingRow
		parseErrs []BulkImportRowError
	)
	for line := 2; ; line++ {
		rec, err := reader.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			parseErrs = append(parseErrs, BulkImportRowError{Row: line, Code: "MALFORMED_ROW", Message: err.Error()})
			continue
		}
		if len(rec) <= idx.skuID {
			parseErrs = append(parseErrs, BulkImportRowError{Row: line, Code: "MISSING_COLUMNS", Message: "row has fewer columns than the header"})
			continue
		}
		skuRaw := strings.TrimSpace(rec[idx.skuID])
		skuID, err := uuid.Parse(skuRaw)
		if err != nil {
			parseErrs = append(parseErrs, BulkImportRowError{Row: line, Code: "INVALID_SKU_ID", Message: "sku_id is not a valid uuid: " + skuRaw})
			continue
		}
		row := BulkMappingRow{
			CustomerSKUCode: strings.TrimSpace(rec[idx.code]),
			SKUID:           skuID,
		}
		if idx.notes >= 0 && idx.notes < len(rec) {
			row.Notes = strings.TrimSpace(rec[idx.notes])
		}
		rows = append(rows, row)
	}
	return rows, parseErrs, nil
}

type csvHeaderIdx struct {
	code  int
	skuID int
	notes int
}

func mappingHeaderIndex(header []string) (csvHeaderIdx, error) {
	idx := csvHeaderIdx{code: -1, skuID: -1, notes: -1}
	for i, h := range header {
		switch strings.ToLower(strings.TrimSpace(h)) {
		case "customer_sku_code":
			idx.code = i
		case "sku_id":
			idx.skuID = i
		case "notes":
			idx.notes = i
		}
	}
	if idx.code < 0 {
		return idx, errors.New("missing required header: customer_sku_code")
	}
	if idx.skuID < 0 {
		return idx, errors.New("missing required header: sku_id")
	}
	return idx, nil
}

// stripBOMReader returns a reader that drops the UTF-8 byte-order mark if
// present. Excel for Windows + Vietnamese locale almost always emits one and
// the CSV parser would otherwise see it as part of the first header field.
func stripBOMReader(r io.Reader) io.Reader {
	br, ok := r.(io.ByteReader)
	if !ok {
		// Wrap in a buffered reader so we can peek 3 bytes.
		buffered := &bomStripper{r: r}
		return buffered
	}
	_ = br
	return &bomStripper{r: r}
}

// bomStripper drops the first three bytes if they are 0xEF 0xBB 0xBF, then
// passes the rest through unchanged. Tiny adapter so we do not pull in
// bufio just for this.
type bomStripper struct {
	r       io.Reader
	checked bool
	prefix  []byte
}

func (b *bomStripper) Read(p []byte) (int, error) {
	if !b.checked {
		b.checked = true
		head := make([]byte, 3)
		n, err := io.ReadFull(b.r, head)
		if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) && !errors.Is(err, io.EOF) {
			return 0, err
		}
		if n == 3 && head[0] == 0xEF && head[1] == 0xBB && head[2] == 0xBF {
			// drop the BOM
			b.prefix = nil
		} else {
			b.prefix = head[:n]
		}
	}
	if len(b.prefix) > 0 {
		k := copy(p, b.prefix)
		b.prefix = b.prefix[k:]
		return k, nil
	}
	return b.r.Read(p)
}

// _ keeps strconv referenced if the file ever drops other strconv use; the
// existing handler still imports strconv so this stays lint-clean. Intentional
// no-op assignment to silence editors that want to nuke unused imports.
var _ = strconv.Itoa
