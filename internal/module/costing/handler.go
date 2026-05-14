package costing

import (
	"encoding/csv"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/vmarble/warehouse-management-service/internal/platform/auth"
	"github.com/vmarble/warehouse-management-service/internal/platform/httpkit"
)

var wasteReportFilterLoc = func() *time.Location {
	loc, err := time.LoadLocation("Asia/Ho_Chi_Minh")
	if err != nil {
		return time.FixedZone("Asia/Ho_Chi_Minh", 7*60*60)
	}
	return loc
}()

type Handler struct {
	svc Service
}

func NewHandler(s Service) *Handler {
	return &Handler{svc: s}
}

func (h *Handler) Register(rg *gin.RouterGroup) {
	rg.POST("/costing/:workOrderID/compute", auth.RequireRole(auth.RoleAccountant, auth.RoleAdmin), h.compute)
	rg.POST("/costing/:workOrderID/finalize", auth.RequireRole(auth.RoleAccountant, auth.RoleAdmin), h.finalize)
	rg.GET("/costing/:workOrderID", auth.RequireRole(auth.RoleAccountant, auth.RolePlanner, auth.RoleAdmin), h.get)
	rg.GET("/costing", auth.RequireRole(auth.RoleAccountant, auth.RoleAdmin), h.list)
	rg.POST("/costing/:workOrderID/adjustments", auth.RequireRole(auth.RoleAccountant, auth.RoleAdmin), h.createAdjustment)
	rg.GET("/costing/:workOrderID/adjustments", auth.RequireRole(auth.RoleAccountant, auth.RolePlanner, auth.RoleAdmin), h.listAdjustments)
	rg.GET("/costing/waste-report", auth.RequireRole(auth.RoleAccountant, auth.RoleAdmin), h.wasteReport)
}

// computeCost godoc
//
// @Summary      Compute costing for a work order
// @Description  Computes (or re-computes) the costing record for a work order. ACTUAL costing on a COMPLETED WO requires at least one non-zero cost component (material/auxiliary/labor); otherwise returns 412 with the Vietnamese message "WO chưa có chi phí vật tư/nhân công, không thể tính giá thành". Estimated costing on PLANNED WOs is exempt.
// @Tags         costing
// @Produce      json
// @Param        workOrderID  path      string  true  "work order id (uuid)"
// @Success      200          {object}  CostingRecord
// @Failure      400          {object}  map[string]string
// @Failure      409          {object}  map[string]string
// @Failure      412          {object}  map[string]string
// @Failure      422          {object}  map[string]string
// @Security     BearerAuth
// @Failure      401  {object}  map[string]string
// @Router       /api/v1/costing/{workOrderID}/compute [post]
func (h *Handler) compute(c *gin.Context) {
	woID, err := uuid.Parse(c.Param("workOrderID"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid workOrderID"})
		return
	}
	record, err := h.svc.ComputeCost(c.Request.Context(), woID)
	if err != nil {
		httpkit.Error(c, err)
		return
	}
	c.JSON(http.StatusOK, record)
}

// finalizeCost godoc
//
// @Summary      Finalize costing for a work order
// @Tags         costing
// @Produce      json
// @Param        workOrderID  path      string  true  "work order id (uuid)"
// @Success      200          {object}  map[string]string
// @Failure      400          {object}  map[string]string
// @Failure      409          {object}  map[string]string
// @Security     BearerAuth
// @Failure      401  {object}  map[string]string
// @Router       /api/v1/costing/{workOrderID}/finalize [post]
func (h *Handler) finalize(c *gin.Context) {
	woID, err := uuid.Parse(c.Param("workOrderID"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid workOrderID"})
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
	if err := h.svc.FinalizeCost(c.Request.Context(), woID, actorID); err != nil {
		httpkit.Error(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "finalized"})
}

// createAdjustment godoc
//
// @Summary      Create a costing adjustment for a finalized work order
// @Tags         costing
// @Accept       json
// @Produce      json
// @Param        workOrderID  path      string                true  "work order id (uuid)"
// @Param        body         body      CreateAdjustmentInput true  "adjustment payload"
// @Success      201          {object}  CostingAdjustment
// @Failure      400          {object}  map[string]string
// @Failure      404          {object}  map[string]string
// @Failure      412          {object}  map[string]string
// @Security     BearerAuth
// @Failure      401  {object}  map[string]string
// @Router       /api/v1/costing/{workOrderID}/adjustments [post]
func (h *Handler) createAdjustment(c *gin.Context) {
	woID, err := uuid.Parse(c.Param("workOrderID"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid workOrderID"})
		return
	}
	var in CreateAdjustmentInput
	if !httpkit.Bind(c, &in) {
		return
	}
	identity, ok := auth.FromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "missing auth identity"})
		return
	}
	createdBy, err := uuid.Parse(identity.UserID)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid auth identity"})
		return
	}
	in.WorkOrderID = woID
	in.CreatedBy = createdBy
	adj, err := h.svc.CreateAdjustment(c.Request.Context(), in)
	if err != nil {
		httpkit.Error(c, err)
		return
	}
	c.JSON(http.StatusCreated, adj)
}

// listAdjustments godoc
//
// @Summary      List costing adjustments for a work order
// @Tags         costing
// @Produce      json
// @Param        workOrderID  path      string  true  "work order id (uuid)"
// @Success      200          {array}   CostingAdjustment
// @Failure      400          {object}  map[string]string
// @Failure      404          {object}  map[string]string
// @Security     BearerAuth
// @Failure      401  {object}  map[string]string
// @Router       /api/v1/costing/{workOrderID}/adjustments [get]
func (h *Handler) listAdjustments(c *gin.Context) {
	woID, err := uuid.Parse(c.Param("workOrderID"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid workOrderID"})
		return
	}
	adjs, err := h.svc.ListAdjustments(c.Request.Context(), woID)
	if err != nil {
		httpkit.Error(c, err)
		return
	}
	c.JSON(http.StatusOK, adjs)
}

// getCostingRecord godoc
//
// @Summary      Get costing record by work order
// @Tags         costing
// @Produce      json
// @Param        workOrderID  path      string  true  "work order id (uuid)"
// @Success      200          {object}  CostingRecord
// @Failure      400          {object}  map[string]string
// @Failure      404          {object}  map[string]string
// @Security     BearerAuth
// @Failure      401  {object}  map[string]string
// @Router       /api/v1/costing/{workOrderID} [get]
func (h *Handler) get(c *gin.Context) {
	woID, err := uuid.Parse(c.Param("workOrderID"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid workOrderID"})
		return
	}
	record, err := h.svc.GetCostingRecord(c.Request.Context(), woID)
	if err != nil {
		httpkit.Error(c, err)
		return
	}
	c.JSON(http.StatusOK, record)
}

// wasteReport godoc
//
// @Summary      Per-material waste-cost ledger (BR-C03)
// @Description  Aggregates per-cut waste area into a per-material report.
// @Description  Waste cost is allocated using the originating board sheet's
// @Description  cost-per-mm² (sheet_cost / sheet_area).
// @Tags         costing
// @Produce      json
// @Param        from         query     string  false  "from date (Asia/Ho_Chi_Minh, YYYY-MM-DD)"
// @Param        to           query     string  false  "to date inclusive (Asia/Ho_Chi_Minh, YYYY-MM-DD)"
// @Param        material_id  query     string  false  "filter by material id (uuid)"
// @Param        format       query     string  false  "response format: json (default) or csv"
// @Success      200          {array}   WasteReportRow
// @Failure      400          {object}  map[string]string
// @Security     BearerAuth
// @Failure      401  {object}  map[string]string
// @Router       /api/v1/costing/waste-report [get]
func (h *Handler) wasteReport(c *gin.Context) {
	from, to, ok := parseWasteReportDateRange(c)
	if !ok {
		return
	}

	var materialID *uuid.UUID
	if v := c.Query("material_id"); v != "" {
		mid, err := uuid.Parse(v)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid material_id"})
			return
		}
		materialID = &mid
	}

	rows, err := h.svc.ListWasteReport(c.Request.Context(), WasteReportFilter{
		From:       from,
		To:         to,
		MaterialID: materialID,
	})
	if err != nil {
		httpkit.Error(c, err)
		return
	}

	if c.Query("format") == "csv" {
		writeWasteReportCSV(c, rows)
		return
	}
	c.JSON(http.StatusOK, rows)
}

func parseWasteReportDateRange(c *gin.Context) (from, to *time.Time, ok bool) {
	fromStr := c.Query("from")
	toStr := c.Query("to")
	if fromStr == "" && toStr == "" {
		return nil, nil, true
	}
	if fromStr != "" {
		day, err := time.ParseInLocation(time.DateOnly, fromStr, wasteReportFilterLoc)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid from, use YYYY-MM-DD"})
			return nil, nil, false
		}
		from = &day
	}
	if toStr != "" {
		day, err := time.ParseInLocation(time.DateOnly, toStr, wasteReportFilterLoc)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid to, use YYYY-MM-DD"})
			return nil, nil, false
		}
		// to is inclusive day; query uses half-open [from, to) so add 1 day.
		exclusive := day.AddDate(0, 0, 1)
		to = &exclusive
	}
	return from, to, true
}

func writeWasteReportCSV(c *gin.Context, rows []WasteReportRow) {
	c.Header("Content-Type", "text/csv; charset=utf-8")
	c.Header("Content-Disposition", `attachment; filename="waste-report.csv"`)
	w := csv.NewWriter(c.Writer)
	_ = w.Write([]string{
		"material_id",
		"material_name",
		"sheets_consumed",
		"waste_area_mm2",
		"avg_sheet_cost",
		"total_waste_cost",
		"currency",
	})
	for _, r := range rows {
		_ = w.Write([]string{
			r.MaterialID.String(),
			r.MaterialName,
			strconv.Itoa(r.SheetsConsumed),
			strconv.FormatInt(r.WasteAreaMM2, 10),
			strconv.FormatInt(r.AvgSheetCost.Amount, 10),
			strconv.FormatInt(r.TotalWasteCost.Amount, 10),
			r.TotalWasteCost.Currency,
		})
	}
	w.Flush()
	if err := w.Error(); err != nil {
		// Headers already written; best-effort log via standard error response is no longer possible.
		// Append a comment line so downstream tooling sees the failure rather than a silently truncated file.
		_, _ = fmt.Fprintf(c.Writer, "# csv flush error: %s\n", err.Error())
	}
}

// listCostingRecords godoc
//
// @Summary      List costing records
// @Tags         costing
// @Produce      json
// @Param        page       query     int     false  "page number (default 1)"
// @Param        limit      query     int     false  "items per page (default 10, max 100)"
// @Param        finalized  query     bool    false  "filter by finalized: true or false (omit for all)"
// @Param        order      query     string  false  "sort direction: asc, desc (default asc)"
// @Success      200  {object}  httpkit.PagedResult[CostingRecord]
// @Failure      500  {object}  map[string]string
// @Security     BearerAuth
// @Failure      401  {object}  map[string]string
// @Router       /api/v1/costing [get]
func (h *Handler) list(c *gin.Context) {
	p := httpkit.BindPageParams(c)
	var finalized *bool
	if v := c.Query("finalized"); v != "" {
		b := v == "true"
		finalized = &b
	}
	result, err := h.svc.ListCostingRecords(c.Request.Context(), p, finalized)
	if err != nil {
		httpkit.Error(c, err)
		return
	}
	c.JSON(http.StatusOK, result)
}
