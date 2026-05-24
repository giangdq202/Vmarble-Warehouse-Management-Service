package planning

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

func (h *Handler) Register(rg *gin.RouterGroup) {
	rg.POST("/plans", auth.RequireRole(auth.RolePlanner, auth.RoleAdmin), h.create)
	rg.GET("/plans", h.list)
	rg.GET("/plans/lookup", h.lookup)
	rg.GET("/plans/:id", h.get)
	rg.POST("/plans/:id/approve", auth.RequireRole(auth.RolePlanner, auth.RoleAdmin), h.approve)
	rg.POST("/plans/:id/cancel", auth.RequireRole(auth.RolePlanner, auth.RoleAdmin), h.cancel)
}

// createPlan godoc
//
// @Summary      Create production plan
// @Tags         planning
// @Accept       json
// @Produce      json
// @Param        body  body      CreatePlanInput  true  "payload"
// @Success      201   {object}  Plan
// @Failure      400   {object}  map[string]string
// @Security     BearerAuth
// @Failure      401  {object}  map[string]string
// @Router       /api/v1/plans [post]
func (h *Handler) create(c *gin.Context) {
	var in CreatePlanInput
	if !httpkit.Bind(c, &in) {
		return
	}
	plan, err := h.svc.CreatePlan(c.Request.Context(), in)
	if err != nil {
		httpkit.Error(c, err)
		return
	}
	c.JSON(http.StatusCreated, plan)
}

// listPlans godoc
//
// @Summary      List production plans
// @Tags         planning
// @Produce      json
// @Param        page     query     int     false  "page number (default 1)"
// @Param        limit    query     int     false  "items per page (default 10, max 100)"
// @Param        search   query     string  false  "search by plan code or PO code (ILIKE)"
// @Param        status   query     string  false  "filter by status: DRAFT, APPROVED, CANCELED"
// @Param        from     query     string  false  "inclusive lower bound on created_at (YYYY-MM-DD)"
// @Param        to       query     string  false  "inclusive upper bound on created_at (YYYY-MM-DD)"
// @Param        sort_by  query     string  false  "sort column: created_at, deadline (default created_at)"
// @Param        order    query     string  false  "sort direction: asc, desc (default desc)"
// @Success      200  {object}  httpkit.PagedResult[Plan]
// @Failure      400  {object}  map[string]string
// @Failure      500  {object}  map[string]string
// @Security     BearerAuth
// @Failure      401  {object}  map[string]string
// @Router       /api/v1/plans [get]
func (h *Handler) list(c *gin.Context) {
	p := httpkit.BindPageParams(c)
	status := c.Query("status")

	// Optional date window on pp.created_at. Parsed as YYYY-MM-DD in UTC; the
	// upper bound is widened to the end of the day so the inclusive contract
	// matches what the FE sends ("plans created on `to`" should be visible).
	var from, to *time.Time
	if s := c.Query("from"); s != "" {
		t, err := time.Parse(time.DateOnly, s)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid from, use YYYY-MM-DD"})
			return
		}
		from = &t
	}
	if s := c.Query("to"); s != "" {
		t, err := time.Parse(time.DateOnly, s)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid to, use YYYY-MM-DD"})
			return
		}
		// End-of-day so the inclusive contract holds for plans created at any time on `to`.
		eod := t.Add(24*time.Hour - time.Nanosecond)
		to = &eod
	}

	result, err := h.svc.ListPlans(c.Request.Context(), p, status, from, to)
	if err != nil {
		httpkit.Error(c, err)
		return
	}
	c.JSON(http.StatusOK, result)
}

// lookupPlans godoc
//
// @Summary      Lookup plans for async combobox
// @Description  Lightweight endpoint for async searchable dropdown. Returns plan id, code, po_code, status, deadline only (no items).
// @Tags         planning
// @Produce      json
// @Param        search         query     string  false  "ILIKE search on plan code or PO code"
// @Param        status         query     string  false  "filter by status: DRAFT, APPROVED, CANCELED"
// @Param        deadline_from  query     string  false  "inclusive lower bound on deadline (YYYY-MM-DD)"
// @Param        deadline_to    query     string  false  "inclusive upper bound on deadline (YYYY-MM-DD)"
// @Param        page           query     int     false  "page number (default 1)"
// @Param        limit          query     int     false  "items per page (default 20, max 50)"
// @Success      200  {object}  httpkit.PagedResult[PlanLookupItem]
// @Failure      400  {object}  map[string]string
// @Security     BearerAuth
// @Failure      401  {object}  map[string]string
// @Router       /api/v1/plans/lookup [get]
func (h *Handler) lookup(c *gin.Context) {
	in := LookupPlansInput{
		Search: c.Query("search"),
		Status: c.Query("status"),
	}

	if s := c.Query("deadline_from"); s != "" {
		t, err := time.Parse(time.DateOnly, s)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid deadline_from, use YYYY-MM-DD"})
			return
		}
		in.DeadlineFrom = &t
	}
	if s := c.Query("deadline_to"); s != "" {
		t, err := time.Parse(time.DateOnly, s)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid deadline_to, use YYYY-MM-DD"})
			return
		}
		in.DeadlineTo = &t
	}

	in.Page, _ = strconv.Atoi(c.DefaultQuery("page", "1"))
	in.Limit, _ = strconv.Atoi(c.DefaultQuery("limit", "20"))

	result, err := h.svc.LookupPlans(c.Request.Context(), in)
	if err != nil {
		httpkit.Error(c, err)
		return
	}
	c.JSON(http.StatusOK, result)
}

// getPlan godoc
//
// @Summary      Get production plan
// @Tags         planning
// @Produce      json
// @Param        id   path      string  true  "plan id (uuid)"
// @Success      200  {object}  Plan
// @Failure      400  {object}  map[string]string
// @Failure      404  {object}  map[string]string
// @Security     BearerAuth
// @Failure      401  {object}  map[string]string
// @Router       /api/v1/plans/{id} [get]
func (h *Handler) get(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	plan, err := h.svc.GetPlan(c.Request.Context(), id)
	if err != nil {
		httpkit.Error(c, err)
		return
	}
	c.JSON(http.StatusOK, plan)
}

// approvePlan godoc
//
// @Summary      Approve production plan
// @Tags         planning
// @Produce      json
// @Param        id   path      string  true  "plan id (uuid)"
// @Success      200  {object}  map[string]string
// @Failure      400  {object}  map[string]string
// @Failure      409  {object}  map[string]string
// @Security     BearerAuth
// @Failure      401  {object}  map[string]string
// @Router       /api/v1/plans/{id}/approve [post]
func (h *Handler) approve(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	if err := h.svc.ApprovePlan(c.Request.Context(), id); err != nil {
		httpkit.Error(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "approved"})
}

// cancelPlan godoc
//
// @Summary      Cancel production plan
// @Description  Cancels a DRAFT or APPROVED plan. APPROVED cancels require a non-empty reason and cascade-cancel any PLANNED work orders. Refused if any work order has progressed past PLANNED.
// @Tags         planning
// @Accept       json
// @Produce      json
// @Param        id    path      string                 true  "plan id (uuid)"
// @Param        body  body      cancelPlanRequest      false "cancel reason (required when plan is APPROVED)"
// @Success      200  {object}  map[string]string
// @Failure      400  {object}  map[string]string
// @Failure      409  {object}  map[string]string
// @Security     BearerAuth
// @Failure      401  {object}  map[string]string
// @Router       /api/v1/plans/{id}/cancel [post]
func (h *Handler) cancel(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	// Body is optional: DRAFT cancels accept an empty reason. We only fail if
	// the JSON itself is malformed; an empty body is treated as no reason.
	var body cancelPlanRequest
	if c.Request.ContentLength > 0 {
		if err := c.ShouldBindJSON(&body); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body"})
			return
		}
	}

	in := CancelPlanInput{PlanID: id, Reason: body.Reason}
	if ident, ok := auth.FromContext(c); ok {
		if uid, err := uuid.Parse(ident.UserID); err == nil {
			in.ActorID = uid
		}
	}

	if err := h.svc.CancelPlan(c.Request.Context(), in); err != nil {
		httpkit.Error(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "canceled"})
}

// cancelPlanRequest is the JSON body for POST /plans/{id}/cancel.
// Reason is required when the plan is APPROVED; ignored for DRAFT cancels.
type cancelPlanRequest struct {
	Reason string `json:"reason"`
}
