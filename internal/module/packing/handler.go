package packing

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

// Register wires the packing endpoints. Listing the FG pool is open to any
// authenticated tier; scan and report-defect are floor activities (Worker
// up); resolving a defect is a planner action because it determines whether
// the FG re-enters the pool (REWORK) or counts as a loss (DISCARD/RETURN_NCC).
func (h *Handler) Register(rg *gin.RouterGroup) {
	rg.GET("/fg-pool", h.list)
	rg.GET("/fg-pool/:id", h.get)
	rg.POST("/packing/scan", auth.RequireWorkerUp(), h.scan)
	rg.POST("/packing/defect", auth.RequireWorkerUp(), h.reportDefect)
	rg.POST("/packing/defect/:id/resolve", auth.RequirePlannerUp(), h.resolveDefect)
}

// listFGPool godoc
//
// @Summary      List Finished-Goods pool entries
// @Tags         packing
// @Produce      json
// @Param        page          query  int     false  "page (default 1)"
// @Param        limit         query  int     false  "limit (default 10, max 100)"
// @Param        status        query  string  false  "AVAILABLE / RESERVED / LOADED / DEFECT / DISPOSED"
// @Param        sku_id        query  string  false  "filter by SKU id (uuid)"
// @Param        so_line_id    query  string  false  "filter by sales order line id (uuid)"
// @Param        wo_id         query  string  false  "filter by work order id (uuid)"
// @Success      200  {object}  httpkit.PagedResult[FGPool]
// @Security     BearerAuth
// @Router       /api/v1/fg-pool [get]
func (h *Handler) list(c *gin.Context) {
	p := httpkit.BindPageParams(c)
	f := FGListFilter{Status: c.Query("status")}
	if v := c.Query("sku_id"); v != "" {
		id, err := uuid.Parse(v)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid sku_id"})
			return
		}
		f.SKUID = &id
	}
	if v := c.Query("so_line_id"); v != "" {
		id, err := uuid.Parse(v)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid so_line_id"})
			return
		}
		f.SOLineID = &id
	}
	if v := c.Query("wo_id"); v != "" {
		id, err := uuid.Parse(v)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid wo_id"})
			return
		}
		f.WorkOrderID = &id
	}
	res, err := h.svc.ListFG(c.Request.Context(), p, f)
	if err != nil {
		httpkit.Error(c, err)
		return
	}
	c.JSON(http.StatusOK, res)
}

// getFGPool godoc
//
// @Summary      Get one FG pool entry
// @Tags         packing
// @Produce      json
// @Param        id   path      string  true  "fg id (uuid)"
// @Success      200  {object}  FGPool
// @Failure      404  {object}  map[string]string
// @Security     BearerAuth
// @Router       /api/v1/fg-pool/{id} [get]
func (h *Handler) get(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	out, err := h.svc.GetFG(c.Request.Context(), id)
	if err != nil {
		httpkit.Error(c, err)
		return
	}
	c.JSON(http.StatusOK, out)
}

// scanBarcode godoc
//
// @Summary      Scan FG barcode at packing station
// @Description  Resolves the barcode → FG and returns suggested loadable containers.
// @Description  Returns 412 when the underlying WO is not yet COMPLETED (BR-PK01).
// @Tags         packing
// @Accept       json
// @Produce      json
// @Param        body  body      scanRequest  true  "payload"
// @Success      200   {object}  ScanResult
// @Failure      400   {object}  map[string]string
// @Failure      404   {object}  map[string]string
// @Failure      412   {object}  map[string]string
// @Security     BearerAuth
// @Router       /api/v1/packing/scan [post]
func (h *Handler) scan(c *gin.Context) {
	var in scanRequest
	if !httpkit.Bind(c, &in) {
		return
	}
	bcID, err := uuid.Parse(in.BarcodeID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid barcode_id"})
		return
	}
	out, err := h.svc.ScanBarcode(c.Request.Context(), bcID, callerID(c))
	if err != nil {
		httpkit.Error(c, err)
		return
	}
	c.JSON(http.StatusOK, out)
}

// reportDefect godoc
//
// @Summary      Report defect on a finished good
// @Description  Flips fg_pool.status to DEFECT and records reason/photos. If the
// @Description  FG was RESERVED, its container_line is auto-removed first (BR-PK03).
// @Tags         packing
// @Accept       json
// @Produce      json
// @Param        body  body      ReportDefectInput  true  "payload"
// @Success      201   {object}  FGDefect
// @Failure      400   {object}  map[string]string
// @Failure      409   {object}  map[string]string
// @Security     BearerAuth
// @Router       /api/v1/packing/defect [post]
func (h *Handler) reportDefect(c *gin.Context) {
	var in ReportDefectInput
	if !httpkit.Bind(c, &in) {
		return
	}
	in.DetectedBy = callerID(c)
	out, err := h.svc.ReportDefect(c.Request.Context(), in)
	if err != nil {
		httpkit.Error(c, err)
		return
	}
	c.JSON(http.StatusCreated, out)
}

// resolveDefect godoc
//
// @Summary      Resolve a reported defect
// @Description  DISCARD/RETURN_NCC → DISPOSED, REWORK → AVAILABLE so the FG can re-enter the pool.
// @Tags         packing
// @Accept       json
// @Produce      json
// @Param        id    path      string              true  "defect id (uuid)"
// @Param        body  body      resolveDefectBody   true  "payload"
// @Success      200   {object}  FGDefect
// @Failure      400   {object}  map[string]string
// @Failure      404   {object}  map[string]string
// @Failure      409   {object}  map[string]string
// @Security     BearerAuth
// @Router       /api/v1/packing/defect/{id}/resolve [post]
func (h *Handler) resolveDefect(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	var body resolveDefectBody
	if !httpkit.Bind(c, &body) {
		return
	}
	out, err := h.svc.ResolveDefect(c.Request.Context(), ResolveDefectInput{
		DefectID:   id,
		Resolution: body.Resolution,
		Note:       body.Note,
		ResolvedBy: callerID(c),
	})
	if err != nil {
		httpkit.Error(c, err)
		return
	}
	c.JSON(http.StatusOK, out)
}

type scanRequest struct {
	BarcodeID string `json:"barcode_id" binding:"required"`
}

type resolveDefectBody struct {
	Resolution string `json:"resolution" binding:"required"`
	Note       string `json:"note,omitempty"`
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
