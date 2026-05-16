package reports

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/vmarble/warehouse-management-service/internal/platform/auth"
	"github.com/vmarble/warehouse-management-service/internal/platform/httpkit"
)

// Handler exposes the five export endpoints under /reports/export.
type Handler struct {
	svc Service
}

func NewHandler(svc Service) *Handler {
	return &Handler{svc: svc}
}

// Register wires the routes onto the protected /api/v1 group. Only admin and
// accountant reach the handlers — these reports include financial figures the
// other roles do not need to see.
func (h *Handler) Register(rg *gin.RouterGroup) {
	guard := auth.RequireRole(auth.RoleAccountant, auth.RoleAdmin)
	rg.GET("/reports/export/costings.xlsx", guard, h.exportCostings)
	rg.GET("/reports/export/purchase-orders.xlsx", guard, h.exportPurchaseOrders)
	rg.GET("/reports/export/skus.xlsx", guard, h.exportSKUs)
	rg.GET("/reports/export/work-orders.xlsx", guard, h.exportWorkOrders)
	rg.GET("/reports/export/waste.xlsx", guard, h.exportWaste)
}

// parsePeriod reads ?from=YYYY-MM-DD&to=YYYY-MM-DD. Either bound may be
// omitted; the To bound is treated as exclusive (start of next day) to align
// with the costing waste-report convention. Returns 400 when a date is set
// but unparseable so the FE never silently exports a different range.
func parsePeriod(c *gin.Context) (Period, bool) {
	const layout = "2006-01-02"
	var p Period
	if raw := c.Query("from"); raw != "" {
		t, err := time.Parse(layout, raw)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid from (expect YYYY-MM-DD)"})
			return Period{}, false
		}
		p.From = &t
	}
	if raw := c.Query("to"); raw != "" {
		t, err := time.Parse(layout, raw)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid to (expect YYYY-MM-DD)"})
			return Period{}, false
		}
		// Treat To as exclusive end-of-day so callers passing equal dates get
		// records that fall on that day rather than a zero-row export.
		end := t.Add(24 * time.Hour)
		p.To = &end
	}
	return p, true
}

func writeXLSX(c *gin.Context, file ExportFile) {
	c.Header("Content-Disposition", `attachment; filename="`+file.Filename+`"`)
	c.Data(http.StatusOK, "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", file.Bytes)
}

// @Summary      Export costing records to Excel
// @Tags         reports
// @Produce      application/vnd.openxmlformats-officedocument.spreadsheetml.sheet
// @Param        from  query  string  false  "From date (YYYY-MM-DD)"
// @Param        to    query  string  false  "To date (YYYY-MM-DD, inclusive)"
// @Success      200 {file} binary
// @Failure      400 {object} map[string]string
// @Router       /api/v1/reports/export/costings.xlsx [get]
// @Security     BearerAuth
func (h *Handler) exportCostings(c *gin.Context) {
	p, ok := parsePeriod(c)
	if !ok {
		return
	}
	file, err := h.svc.ExportCostings(c.Request.Context(), p)
	if err != nil {
		httpkit.Error(c, err)
		return
	}
	writeXLSX(c, file)
}

// @Summary      Export purchase orders to Excel
// @Tags         reports
// @Produce      application/vnd.openxmlformats-officedocument.spreadsheetml.sheet
// @Param        from  query  string  false  "From date (YYYY-MM-DD)"
// @Param        to    query  string  false  "To date (YYYY-MM-DD, inclusive)"
// @Success      200 {file} binary
// @Failure      400 {object} map[string]string
// @Router       /api/v1/reports/export/purchase-orders.xlsx [get]
// @Security     BearerAuth
func (h *Handler) exportPurchaseOrders(c *gin.Context) {
	p, ok := parsePeriod(c)
	if !ok {
		return
	}
	file, err := h.svc.ExportPurchaseOrders(c.Request.Context(), p)
	if err != nil {
		httpkit.Error(c, err)
		return
	}
	writeXLSX(c, file)
}

// @Summary      Export SKU catalog to Excel
// @Tags         reports
// @Produce      application/vnd.openxmlformats-officedocument.spreadsheetml.sheet
// @Success      200 {file} binary
// @Failure      400 {object} map[string]string
// @Router       /api/v1/reports/export/skus.xlsx [get]
// @Security     BearerAuth
func (h *Handler) exportSKUs(c *gin.Context) {
	file, err := h.svc.ExportSKUs(c.Request.Context())
	if err != nil {
		httpkit.Error(c, err)
		return
	}
	writeXLSX(c, file)
}

// @Summary      Export work orders to Excel
// @Tags         reports
// @Produce      application/vnd.openxmlformats-officedocument.spreadsheetml.sheet
// @Param        from  query  string  false  "From date (YYYY-MM-DD)"
// @Param        to    query  string  false  "To date (YYYY-MM-DD, inclusive)"
// @Success      200 {file} binary
// @Failure      400 {object} map[string]string
// @Router       /api/v1/reports/export/work-orders.xlsx [get]
// @Security     BearerAuth
func (h *Handler) exportWorkOrders(c *gin.Context) {
	p, ok := parsePeriod(c)
	if !ok {
		return
	}
	file, err := h.svc.ExportWorkOrders(c.Request.Context(), p)
	if err != nil {
		httpkit.Error(c, err)
		return
	}
	writeXLSX(c, file)
}

// @Summary      Export waste report to Excel
// @Tags         reports
// @Produce      application/vnd.openxmlformats-officedocument.spreadsheetml.sheet
// @Param        from  query  string  false  "From date (YYYY-MM-DD)"
// @Param        to    query  string  false  "To date (YYYY-MM-DD, inclusive)"
// @Success      200 {file} binary
// @Failure      400 {object} map[string]string
// @Router       /api/v1/reports/export/waste.xlsx [get]
// @Security     BearerAuth
func (h *Handler) exportWaste(c *gin.Context) {
	p, ok := parsePeriod(c)
	if !ok {
		return
	}
	file, err := h.svc.ExportWaste(c.Request.Context(), p)
	if err != nil {
		httpkit.Error(c, err)
		return
	}
	writeXLSX(c, file)
}
