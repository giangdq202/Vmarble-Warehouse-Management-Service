package delivery

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

// Register wires the delivery endpoints. Floor staff (warehouse foreman)
// add/remove lines via WorkerUp; allocation moves and lifecycle transitions
// require PlannerUp; reopen is admin-only because it undoes a sealed
// shipment audit trail.
func (h *Handler) Register(rg *gin.RouterGroup) {
	rg.POST("/containers", auth.RequirePlannerUp(), h.create)
	rg.GET("/containers", h.list)
	rg.GET("/containers/:id", h.get)
	rg.GET("/containers/:id/status-log", h.statusLog)

	rg.POST("/containers/:id/lines", auth.RequireWorkerUp(), h.addLine)
	rg.DELETE("/containers/:id/lines/:line_id", auth.RequireWorkerUp(), h.deleteLine)
	rg.POST("/containers/:id/transfer-line", auth.RequirePlannerUp(), h.transferLine)

	rg.POST("/containers/:id/seal", auth.RequirePlannerUp(), h.seal)
	rg.POST("/containers/:id/reopen", auth.RequireAdminOnly(), h.reopen)
	rg.POST("/containers/:id/ship", auth.RequirePlannerUp(), h.ship)
	rg.POST("/containers/:id/cancel", auth.RequirePlannerUp(), h.cancel)

	// Loading plans (#301). Upload requires the planner persona; approve is
	// admin-only because it locks the version that #291's VERIFY-mode kiosk
	// reconciles scans against.
	rg.POST("/containers/:id/loading-plan", auth.RequirePlannerUp(), h.uploadLoadingPlan)
	rg.GET("/containers/:id/loading-plan", h.getActiveLoadingPlan)
	rg.GET("/containers/:id/lines-history", h.listLinesHistory)
	rg.GET("/loading-plans/:id", h.getLoadingPlan)
	rg.GET("/loading-plans/:id/diff", h.diffLoadingPlan)
	rg.POST("/loading-plans/:id/approve", auth.RequireAdminOnly(), h.approveLoadingPlan)
}

// ── Container CRUD ──────────────────────────────────────────────────────────

// createContainer godoc
//
// @Summary      Create container (OPEN)
// @Tags         delivery
// @Accept       json
// @Produce      json
// @Param        body  body      CreateContainerInput  true  "payload"
// @Success      201   {object}  Container
// @Failure      400   {object}  map[string]string
// @Security     BearerAuth
// @Failure      401  {object}  map[string]string
// @Router       /api/v1/containers [post]
func (h *Handler) create(c *gin.Context) {
	var in CreateContainerInput
	if !httpkit.Bind(c, &in) {
		return
	}
	in.CreatedBy = callerID(c)
	out, err := h.svc.CreateContainer(c.Request.Context(), in)
	if err != nil {
		httpkit.Error(c, err)
		return
	}
	c.JSON(http.StatusCreated, out)
}

// listContainers godoc
//
// @Summary      List containers
// @Tags         delivery
// @Produce      json
// @Param        page            query  int     false  "page (default 1)"
// @Param        limit           query  int     false  "limit (default 10, max 100)"
// @Param        search          query  string  false  "ILIKE on container code"
// @Param        status          query  string  false  "filter by status"
// @Param        container_type  query  string  false  "20GP / 40GP / 40HC"
// @Success      200  {object}  httpkit.PagedResult[Container]
// @Security     BearerAuth
// @Router       /api/v1/containers [get]
func (h *Handler) list(c *gin.Context) {
	p := httpkit.BindPageParams(c)
	f := ContainerListFilter{Status: c.Query("status"), ContainerType: c.Query("container_type")}
	res, err := h.svc.ListContainers(c.Request.Context(), p, f)
	if err != nil {
		httpkit.Error(c, err)
		return
	}
	c.JSON(http.StatusOK, res)
}

// getContainer godoc
//
// @Summary      Get container with lines + fill_pct
// @Tags         delivery
// @Produce      json
// @Param        id   path      string  true  "container id (uuid)"
// @Success      200  {object}  Container
// @Failure      404  {object}  map[string]string
// @Security     BearerAuth
// @Router       /api/v1/containers/{id} [get]
func (h *Handler) get(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	out, err := h.svc.GetContainer(c.Request.Context(), id)
	if err != nil {
		httpkit.Error(c, err)
		return
	}
	c.JSON(http.StatusOK, out)
}

// statusLog godoc
//
// @Summary      Container status transition history
// @Tags         delivery
// @Produce      json
// @Param        id   path      string  true  "container id (uuid)"
// @Success      200  {array}   ContainerStatusLogEntry
// @Failure      404  {object}  map[string]string
// @Security     BearerAuth
// @Router       /api/v1/containers/{id}/status-log [get]
func (h *Handler) statusLog(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	entries, err := h.svc.ListStatusLog(c.Request.Context(), id)
	if err != nil {
		httpkit.Error(c, err)
		return
	}
	c.JSON(http.StatusOK, entries)
}

// ── Line operations ─────────────────────────────────────────────────────────

// addLine godoc
//
// @Summary      Add a finished-goods line to a container
// @Tags         delivery
// @Accept       json
// @Produce      json
// @Param        id    path      string        true  "container id (uuid)"
// @Param        body  body      AddLineInput  true  "payload"
// @Success      201   {object}  ContainerLine
// @Failure      400   {object}  map[string]string
// @Failure      404   {object}  map[string]string
// @Failure      409   {object}  map[string]string
// @Security     BearerAuth
// @Router       /api/v1/containers/{id}/lines [post]
func (h *Handler) addLine(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	var in AddLineInput
	if !httpkit.Bind(c, &in) {
		return
	}
	in.ContainerID = id
	in.AddedBy = callerID(c)
	line, err := h.svc.AddLine(c.Request.Context(), in)
	if err != nil {
		httpkit.Error(c, err)
		return
	}
	c.JSON(http.StatusCreated, line)
}

// deleteLine godoc
//
// @Summary      Remove a line from a container
// @Tags         delivery
// @Param        id        path  string  true  "container id (uuid)"
// @Param        line_id   path  string  true  "line id (uuid)"
// @Success      204
// @Failure      404  {object}  map[string]string
// @Failure      409  {object}  map[string]string
// @Security     BearerAuth
// @Router       /api/v1/containers/{id}/lines/{line_id} [delete]
func (h *Handler) deleteLine(c *gin.Context) {
	containerID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	lineID, err := uuid.Parse(c.Param("line_id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid line_id"})
		return
	}
	if err := h.svc.DeleteLine(c.Request.Context(), containerID, lineID, callerID(c)); err != nil {
		httpkit.Error(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// transferLine godoc
//
// @Summary      Transfer a line (full or partial) to another container
// @Tags         delivery
// @Accept       json
// @Produce      json
// @Param        id    path      string             true  "source container id (uuid)"
// @Param        body  body      TransferLineInput  true  "payload"
// @Success      200   {object}  TransferLineResult
// @Failure      400   {object}  map[string]string
// @Failure      404   {object}  map[string]string
// @Failure      409   {object}  map[string]string
// @Security     BearerAuth
// @Router       /api/v1/containers/{id}/transfer-line [post]
func (h *Handler) transferLine(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	var in TransferLineInput
	if !httpkit.Bind(c, &in) {
		return
	}
	in.ContainerID = id
	in.ActorID = callerID(c)
	out, err := h.svc.TransferLine(c.Request.Context(), in)
	if err != nil {
		httpkit.Error(c, err)
		return
	}
	c.JSON(http.StatusOK, out)
}

// ── Lifecycle ────────────────────────────────────────────────────────────────

// sealContainer godoc
//
// @Summary      Seal container (atomically bumps qty_shipped on SO lines)
// @Tags         delivery
// @Accept       json
// @Produce      json
// @Param        id    path      string                  true  "container id (uuid)"
// @Param        body  body      sealReopenShipRequest   false "payload"
// @Success      200   {object}  Container
// @Failure      404   {object}  map[string]string
// @Failure      409   {object}  map[string]string
// @Failure      412   {object}  map[string]string
// @Security     BearerAuth
// @Router       /api/v1/containers/{id}/seal [post]
func (h *Handler) seal(c *gin.Context) {
	id, body, ok := h.bindLifecycleRequest(c)
	if !ok {
		return
	}
	out, err := h.svc.Seal(c.Request.Context(), SealInput{
		ContainerID: id,
		ActorID:     callerID(c),
		Note:        body.Note,
	})
	if err != nil {
		httpkit.Error(c, err)
		return
	}
	c.JSON(http.StatusOK, out)
}

// reopenContainer godoc
//
// @Summary      Reopen sealed container (admin only, requires reason)
// @Tags         delivery
// @Accept       json
// @Produce      json
// @Param        id    path      string                  true  "container id (uuid)"
// @Param        body  body      sealReopenShipRequest   true  "payload (reason required)"
// @Success      200   {object}  Container
// @Failure      400   {object}  map[string]string
// @Failure      404   {object}  map[string]string
// @Failure      409   {object}  map[string]string
// @Security     BearerAuth
// @Router       /api/v1/containers/{id}/reopen [post]
func (h *Handler) reopen(c *gin.Context) {
	id, body, ok := h.bindLifecycleRequest(c)
	if !ok {
		return
	}
	out, err := h.svc.Reopen(c.Request.Context(), ReopenInput{
		ContainerID: id,
		ActorID:     callerID(c),
		Reason:      body.Reason,
	})
	if err != nil {
		httpkit.Error(c, err)
		return
	}
	c.JSON(http.StatusOK, out)
}

// shipContainer godoc
//
// @Summary      Mark container SHIPPED (after seal)
// @Tags         delivery
// @Accept       json
// @Produce      json
// @Param        id    path      string                  true  "container id (uuid)"
// @Param        body  body      sealReopenShipRequest   false "payload"
// @Success      200   {object}  Container
// @Failure      409   {object}  map[string]string
// @Security     BearerAuth
// @Router       /api/v1/containers/{id}/ship [post]
func (h *Handler) ship(c *gin.Context) {
	id, body, ok := h.bindLifecycleRequest(c)
	if !ok {
		return
	}
	out, err := h.svc.Ship(c.Request.Context(), ShipInput{
		ContainerID: id,
		ActorID:     callerID(c),
		Note:        body.Note,
	})
	if err != nil {
		httpkit.Error(c, err)
		return
	}
	c.JSON(http.StatusOK, out)
}

// cancelContainer godoc
//
// @Summary      Cancel an OPEN/LOADING container
// @Tags         delivery
// @Accept       json
// @Produce      json
// @Param        id    path      string                  true  "container id (uuid)"
// @Param        body  body      sealReopenShipRequest   false "payload"
// @Success      200   {object}  Container
// @Failure      409   {object}  map[string]string
// @Security     BearerAuth
// @Router       /api/v1/containers/{id}/cancel [post]
func (h *Handler) cancel(c *gin.Context) {
	id, body, ok := h.bindLifecycleRequest(c)
	if !ok {
		return
	}
	out, err := h.svc.Cancel(c.Request.Context(), CancelInput{
		ContainerID: id,
		ActorID:     callerID(c),
		Reason:      body.Reason,
	})
	if err != nil {
		httpkit.Error(c, err)
		return
	}
	c.JSON(http.StatusOK, out)
}

// sealReopenShipRequest is the shared body shape for the lifecycle endpoints.
// Reason is required only for reopen; note is optional everywhere else.
type sealReopenShipRequest struct {
	Note   string `json:"note,omitempty"`
	Reason string `json:"reason,omitempty"`
}

func (h *Handler) bindLifecycleRequest(c *gin.Context) (uuid.UUID, sealReopenShipRequest, bool) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return uuid.Nil, sealReopenShipRequest{}, false
	}
	var body sealReopenShipRequest
	if c.Request.ContentLength > 0 {
		if err := c.ShouldBindJSON(&body); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body"})
			return uuid.Nil, sealReopenShipRequest{}, false
		}
	}
	return id, body, true
}

// callerID extracts the caller UUID from the auth identity. Returns uuid.Nil
// when the identity is missing or malformed (the auth middleware already
// rejects unauthenticated requests upstream — falling through here keeps the
// service from crashing on a malformed JWT we somehow let through).
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

// ── Loading plans (#301) ────────────────────────────────────────────────────

// uploadLoadingPlan godoc
//
// @Summary      Upload customer packing-list Excel as a loading plan (PARSED)
// @Tags         delivery
// @Accept       multipart/form-data
// @Produce      json
// @Param        id           path      string  true  "container id"
// @Param        customer_id  formData  string  true  "customer uuid that owns the SKU mappings"
// @Param        file         formData  file    true  "packing-list .xlsx"
// @Param        notes        formData  string  false "free-form notes"
// @Param        excel_url    formData  string  false "external URL where the file is archived"
// @Success      200          {object}  LoadingPlanUploadResult
// @Failure      400          {object}  LoadingPlanUploadResult
// @Failure      409          {object}  map[string]string
// @Security     BearerAuth
// @Router       /api/v1/containers/{id}/loading-plan [post]
func (h *Handler) uploadLoadingPlan(c *gin.Context) {
	containerID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid container id"})
		return
	}
	customerIDRaw := c.PostForm("customer_id")
	customerID, err := uuid.Parse(customerIDRaw)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "customer_id is required"})
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
	defer func() { _ = f.Close() }()

	res, err := h.svc.UploadLoadingPlan(c.Request.Context(), UploadLoadingPlanInput{
		ContainerID:  containerID,
		CustomerID:   customerID,
		ExcelFileURL: c.PostForm("excel_url"),
		UploadedBy:   callerID(c),
		Notes:        c.PostForm("notes"),
		File:         f,
	})
	if err != nil {
		// When validation collected per-row errors, bubble them up alongside
		// the canonical 400 so the FE can render the row toasts.
		if len(res.Errors) > 0 {
			c.JSON(http.StatusBadRequest, res)
			return
		}
		httpkit.Error(c, err)
		return
	}
	c.JSON(http.StatusOK, res)
}

// getActiveLoadingPlan godoc
//
// @Summary      Get the active (non-superseded) loading plan for a container
// @Tags         delivery
// @Produce      json
// @Param        id   path      string  true  "container id"
// @Success      200  {object}  LoadingPlan
// @Failure      404  {object}  map[string]string
// @Security     BearerAuth
// @Router       /api/v1/containers/{id}/loading-plan [get]
func (h *Handler) getActiveLoadingPlan(c *gin.Context) {
	containerID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid container id"})
		return
	}
	plan, err := h.svc.GetActiveLoadingPlan(c.Request.Context(), containerID)
	if err != nil {
		httpkit.Error(c, err)
		return
	}
	c.JSON(http.StatusOK, plan)
}

// getLoadingPlan godoc
//
// @Summary      Get one loading plan with its lines
// @Tags         delivery
// @Produce      json
// @Param        id   path      string  true  "loading plan id"
// @Success      200  {object}  LoadingPlan
// @Failure      404  {object}  map[string]string
// @Security     BearerAuth
// @Router       /api/v1/loading-plans/{id} [get]
func (h *Handler) getLoadingPlan(c *gin.Context) {
	planID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid plan id"})
		return
	}
	plan, err := h.svc.GetLoadingPlan(c.Request.Context(), planID)
	if err != nil {
		httpkit.Error(c, err)
		return
	}
	c.JSON(http.StatusOK, plan)
}

// diffLoadingPlan godoc
//
// @Summary      Diff a loading plan against another (added/removed/changed by sku)
// @Tags         delivery
// @Produce      json
// @Param        id       path      string  true  "loading plan id"
// @Param        against  query     string  true  "loading plan id to diff against"
// @Success      200      {object}  LoadingPlanDiff
// @Failure      400      {object}  map[string]string
// @Security     BearerAuth
// @Router       /api/v1/loading-plans/{id}/diff [get]
func (h *Handler) diffLoadingPlan(c *gin.Context) {
	planID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid plan id"})
		return
	}
	againstID, err := uuid.Parse(c.Query("against"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "against query param must be a uuid"})
		return
	}
	diff, err := h.svc.DiffLoadingPlans(c.Request.Context(), planID, againstID)
	if err != nil {
		httpkit.Error(c, err)
		return
	}
	c.JSON(http.StatusOK, diff)
}

type approveLoadingPlanRequest struct {
	Notes            string `json:"notes,omitempty"`
	ConfirmSupersede bool   `json:"confirm_supersede,omitempty"`
}

// approveLoadingPlan godoc
//
// @Summary      Approve a loading plan (admin) — locks the version
// @Description  When the container already has scanned container_lines, the
// @Description  caller MUST set confirm_supersede=true. Without it the
// @Description  endpoint returns 412 so the FE can render the confirm dialog.
// @Tags         delivery
// @Accept       json
// @Produce      json
// @Param        id    path      string                      true   "loading plan id"
// @Param        body  body      approveLoadingPlanRequest   false  "optional notes + confirm_supersede flag"
// @Success      200   {object}  LoadingPlan
// @Failure      404   {object}  map[string]string
// @Failure      409   {object}  map[string]string
// @Failure      412   {object}  map[string]string  "container has scanned lines; resubmit with confirm_supersede=true"
// @Security     BearerAuth
// @Router       /api/v1/loading-plans/{id}/approve [post]
func (h *Handler) approveLoadingPlan(c *gin.Context) {
	planID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid plan id"})
		return
	}
	var req approveLoadingPlanRequest
	if c.Request.ContentLength > 0 {
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body"})
			return
		}
	}
	plan, err := h.svc.ApproveLoadingPlan(c.Request.Context(), ApproveLoadingPlanInput{
		PlanID:           planID,
		ActorID:          callerID(c),
		Notes:            req.Notes,
		ConfirmSupersede: req.ConfirmSupersede,
	})
	if err != nil {
		httpkit.Error(c, err)
		return
	}
	c.JSON(http.StatusOK, plan)
}

// listLinesHistory godoc
//
// @Summary      List container_lines_history for a container (#302)
// @Description  Audit trail of every container_lines row that was wiped by a
// @Description  v2 supersede. Optionally filter by the plan id that triggered
// @Description  the supersede via ?plan_id=<uuid>.
// @Tags         delivery
// @Produce      json
// @Param        id        path      string  true   "container id"
// @Param        plan_id   query     string  false  "loading plan id that supersededthis row"
// @Success      200       {array}   ContainerLineHistoryEntry
// @Failure      400       {object}  map[string]string
// @Security     BearerAuth
// @Router       /api/v1/containers/{id}/lines-history [get]
func (h *Handler) listLinesHistory(c *gin.Context) {
	containerID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid container id"})
		return
	}
	var planFilter *uuid.UUID
	if raw := c.Query("plan_id"); raw != "" {
		pid, err := uuid.Parse(raw)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid plan_id"})
			return
		}
		planFilter = &pid
	}
	entries, err := h.svc.ListContainerLinesHistory(c.Request.Context(), containerID, planFilter)
	if err != nil {
		httpkit.Error(c, err)
		return
	}
	c.JSON(http.StatusOK, entries)
}
