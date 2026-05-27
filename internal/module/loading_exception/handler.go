package loading_exception

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

// Register wires the loading-exception endpoints. Per CLAUDE.md persona
// helpers: create is WorkerUp (packers raise variance from the floor),
// approve / reject are PlannerUp (admin/sales adjudicate).
func (h *Handler) Register(rg *gin.RouterGroup) {
	rg.POST("/containers/:id/exceptions", auth.RequireWorkerUp(), h.create)
	rg.GET("/containers/:id/exceptions", h.list)
	rg.GET("/loading-exceptions/:id", h.get)
	rg.PATCH("/loading-exceptions/:id/approve", auth.RequirePlannerUp(), h.approve)
	rg.PATCH("/loading-exceptions/:id/reject", auth.RequirePlannerUp(), h.reject)
}

// createException godoc
//
// @Summary      Raise a loading exception against a container
// @Description  Status starts pending (approved_by NULL). Type must be one of
// @Description  SHORT_SHIPPED / OVER_LOADED / WRONG_SKU / SUBSTITUTION /
// @Description  DAMAGED_AT_LOADING / UNPLANNED_UNIT / CUSTOMER_CHANGE.
// @Tags         loading-exceptions
// @Accept       json
// @Produce      json
// @Param        id    path      string          true  "container id (uuid)"
// @Param        body  body      createRequest   true  "payload"
// @Success      201   {object}  LoadingException
// @Failure      400   {object}  map[string]string
// @Failure      404   {object}  map[string]string
// @Security     BearerAuth
// @Router       /api/v1/containers/{id}/exceptions [post]
func (h *Handler) create(c *gin.Context) {
	containerID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid container id"})
		return
	}
	var body createRequest
	if !httpkit.Bind(c, &body) {
		return
	}
	in := CreateInput{
		ContainerID:   containerID,
		ExceptionType: body.ExceptionType,
		Reason:        body.Reason,
		PhotoURLs:     body.PhotoURLs,
		CreatedBy:     callerID(c),
	}
	if body.LoadingPlanID != "" {
		id, err := uuid.Parse(body.LoadingPlanID)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid loading_plan_id"})
			return
		}
		in.LoadingPlanID = &id
	}
	if body.SKUID != "" {
		id, err := uuid.Parse(body.SKUID)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid sku_id"})
			return
		}
		in.SKUID = &id
	}
	if body.SOLineID != "" {
		id, err := uuid.Parse(body.SOLineID)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid so_line_id"})
			return
		}
		in.SOLineID = &id
	}
	if body.Qty != nil {
		v := *body.Qty
		in.Qty = &v
	}
	out, err := h.svc.Create(c.Request.Context(), in)
	if err != nil {
		httpkit.Error(c, err)
		return
	}
	c.JSON(http.StatusCreated, out)
}

// listExceptions godoc
//
// @Summary      List loading exceptions for a container (keyset paginated)
// @Tags         loading-exceptions
// @Produce      json
// @Param        id      path      string  true   "container id (uuid)"
// @Param        status  query     string  false  "pending | approved | all (default all)"
// @Param        cursor  query     string  false  "opaque cursor token"
// @Param        limit   query     int     false  "page size"
// @Success      200     {object}  httpkit.CursorResult[LoadingException]
// @Security     BearerAuth
// @Router       /api/v1/containers/{id}/exceptions [get]
func (h *Handler) list(c *gin.Context) {
	containerID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid container id"})
		return
	}
	res, err := h.svc.List(c.Request.Context(), containerID,
		ListFilter{Status: c.Query("status")},
		httpkit.BindCursorParams(c),
	)
	if err != nil {
		httpkit.Error(c, err)
		return
	}
	c.JSON(http.StatusOK, res)
}

// getException godoc
//
// @Summary      Get one loading exception by id
// @Tags         loading-exceptions
// @Produce      json
// @Param        id   path      string  true  "exception id (uuid)"
// @Success      200  {object}  LoadingException
// @Failure      404  {object}  map[string]string
// @Security     BearerAuth
// @Router       /api/v1/loading-exceptions/{id} [get]
func (h *Handler) get(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	out, err := h.svc.Get(c.Request.Context(), id)
	if err != nil {
		httpkit.Error(c, err)
		return
	}
	c.JSON(http.StatusOK, out)
}

// approveException godoc
//
// @Summary      Approve a pending loading exception
// @Description  Resolution must be one of BACKORDER / CANCEL_FROM_SO /
// @Description  SUBSTITUTE_ACCEPTED / WRITE_OFF / DEFER_TO_NEXT.
// @Description  BR-D17 BACKORDER: parent_so_line_id is required and a
// @Description  carry-over sales_order_lines row is created in the same tx.
// @Description  BR-D18 SUBSTITUTE_ACCEPTED: substitute_sku_id is required.
// @Tags         loading-exceptions
// @Accept       json
// @Produce      json
// @Param        id    path      string          true  "exception id (uuid)"
// @Param        body  body      approveRequest  true  "payload"
// @Success      200   {object}  LoadingException
// @Failure      400   {object}  map[string]string
// @Failure      404   {object}  map[string]string
// @Failure      409   {object}  map[string]string
// @Security     BearerAuth
// @Router       /api/v1/loading-exceptions/{id}/approve [patch]
func (h *Handler) approve(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	var body approveRequest
	if !httpkit.Bind(c, &body) {
		return
	}
	in := ApproveInput{
		ID:              id,
		Resolution:      body.Resolution,
		ResolutionNotes: body.ResolutionNotes,
		ApprovedBy:      callerID(c),
	}
	if body.SubstituteSKUID != "" {
		sid, err := uuid.Parse(body.SubstituteSKUID)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid substitute_sku_id"})
			return
		}
		in.SubstituteSKUID = &sid
	}
	if body.ParentSOLineID != "" {
		sid, err := uuid.Parse(body.ParentSOLineID)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid parent_so_line_id"})
			return
		}
		in.ParentSOLineID = &sid
	}
	out, err := h.svc.Approve(c.Request.Context(), in)
	if err != nil {
		httpkit.Error(c, err)
		return
	}
	c.JSON(http.StatusOK, out)
}

// rejectException godoc
//
// @Summary      Reject a pending loading exception
// @Description  Closes the exception without picking a resolution.
// @Description  resolution column stays NULL but approved_by/approved_at are stamped
// @Description  so the SEAL guard treats it as resolved.
// @Tags         loading-exceptions
// @Accept       json
// @Produce      json
// @Param        id    path      string         true  "exception id (uuid)"
// @Param        body  body      rejectRequest  true  "payload"
// @Success      200   {object}  LoadingException
// @Failure      400   {object}  map[string]string
// @Failure      404   {object}  map[string]string
// @Failure      409   {object}  map[string]string
// @Security     BearerAuth
// @Router       /api/v1/loading-exceptions/{id}/reject [patch]
func (h *Handler) reject(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	var body rejectRequest
	if !httpkit.Bind(c, &body) {
		return
	}
	out, err := h.svc.Reject(c.Request.Context(), RejectInput{
		ID:         id,
		Reason:     body.Reason,
		ApprovedBy: callerID(c),
	})
	if err != nil {
		httpkit.Error(c, err)
		return
	}
	c.JSON(http.StatusOK, out)
}

type createRequest struct {
	ExceptionType string   `json:"exception_type" binding:"required"`
	LoadingPlanID string   `json:"loading_plan_id,omitempty"`
	SKUID         string   `json:"sku_id,omitempty"`
	SOLineID      string   `json:"so_line_id,omitempty"`
	Qty           *int     `json:"qty,omitempty"`
	Reason        string   `json:"reason" binding:"required"`
	PhotoURLs     []string `json:"photo_urls,omitempty"`
}

type approveRequest struct {
	Resolution      string `json:"resolution" binding:"required"`
	ResolutionNotes string `json:"resolution_notes,omitempty"`
	SubstituteSKUID string `json:"substitute_sku_id,omitempty"`
	ParentSOLineID  string `json:"parent_so_line_id,omitempty"`
}

type rejectRequest struct {
	Reason string `json:"reason" binding:"required"`
}

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
