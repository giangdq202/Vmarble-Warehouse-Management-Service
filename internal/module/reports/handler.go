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

// exportStream pre-validates the period, sets the .xlsx headers, then drives
// the streaming Export call. The service runs validate() + reader-nil checks
// before opening the StreamWriter, so 4xx responses always land before any
// byte is sent. If the export fails AFTER the writer has flushed (rare —
// only if the underlying reader errors mid-iteration), we cannot rewrite the
// response, so we log via gin and let the client see a truncated download.
func exportStream(c *gin.Context, p Period, run func(*gin.Context) (string, error), prefix string) {
	if err := p.validate(); err != nil {
		httpkit.Error(c, err)
		return
	}
	name := filename(prefix, p)
	c.Header("Content-Disposition", `attachment; filename="`+name+`"`)
	c.Header("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	if _, err := run(c); err != nil {
		if !c.Writer.Written() {
			c.Writer.Header().Del("Content-Disposition")
			c.Writer.Header().Del("Content-Type")
			httpkit.Error(c, err)
			return
		}
		_ = c.Error(err)
	}
}

// @Summary      Export costing records to Excel
// @Tags         reports
// @Produce      application/vnd.openxmlformats-officedocument.spreadsheetml.sheet
// @Param        from  query  string  false  "From date (YYYY-MM-DD)"
// @Param        to    query  string  false  "To date (YYYY-MM-DD, inclusive; max 90-day span)"
// @Success      200 {file} binary
// @Failure      400 {object} map[string]string
// @Router       /api/v1/reports/export/costings.xlsx [get]
// @Security     BearerAuth
func (h *Handler) exportCostings(c *gin.Context) {
	p, ok := parsePeriod(c)
	if !ok {
		return
	}
	exportStream(c, p, func(ctx *gin.Context) (string, error) {
		return h.svc.ExportCostings(ctx.Request.Context(), ctx.Writer, p)
	}, "costings")
}

// @Summary      Export purchase orders to Excel
// @Tags         reports
// @Produce      application/vnd.openxmlformats-officedocument.spreadsheetml.sheet
// @Param        from  query  string  false  "From date (YYYY-MM-DD)"
// @Param        to    query  string  false  "To date (YYYY-MM-DD, inclusive; max 90-day span)"
// @Success      200 {file} binary
// @Failure      400 {object} map[string]string
// @Router       /api/v1/reports/export/purchase-orders.xlsx [get]
// @Security     BearerAuth
func (h *Handler) exportPurchaseOrders(c *gin.Context) {
	p, ok := parsePeriod(c)
	if !ok {
		return
	}
	exportStream(c, p, func(ctx *gin.Context) (string, error) {
		return h.svc.ExportPurchaseOrders(ctx.Request.Context(), ctx.Writer, p)
	}, "purchase-orders")
}

// @Summary      Export SKU catalog to Excel
// @Tags         reports
// @Produce      application/vnd.openxmlformats-officedocument.spreadsheetml.sheet
// @Success      200 {file} binary
// @Failure      400 {object} map[string]string
// @Router       /api/v1/reports/export/skus.xlsx [get]
// @Security     BearerAuth
func (h *Handler) exportSKUs(c *gin.Context) {
	exportStream(c, Period{}, func(ctx *gin.Context) (string, error) {
		return h.svc.ExportSKUs(ctx.Request.Context(), ctx.Writer)
	}, "skus")
}

// @Summary      Export work orders to Excel
// @Tags         reports
// @Produce      application/vnd.openxmlformats-officedocument.spreadsheetml.sheet
// @Param        from  query  string  false  "From date (YYYY-MM-DD)"
// @Param        to    query  string  false  "To date (YYYY-MM-DD, inclusive; max 90-day span)"
// @Success      200 {file} binary
// @Failure      400 {object} map[string]string
// @Router       /api/v1/reports/export/work-orders.xlsx [get]
// @Security     BearerAuth
func (h *Handler) exportWorkOrders(c *gin.Context) {
	p, ok := parsePeriod(c)
	if !ok {
		return
	}
	exportStream(c, p, func(ctx *gin.Context) (string, error) {
		return h.svc.ExportWorkOrders(ctx.Request.Context(), ctx.Writer, p)
	}, "work-orders")
}

// @Summary      Export waste report to Excel
// @Tags         reports
// @Produce      application/vnd.openxmlformats-officedocument.spreadsheetml.sheet
// @Param        from  query  string  false  "From date (YYYY-MM-DD)"
// @Param        to    query  string  false  "To date (YYYY-MM-DD, inclusive; max 90-day span)"
// @Success      200 {file} binary
// @Failure      400 {object} map[string]string
// @Router       /api/v1/reports/export/waste.xlsx [get]
// @Security     BearerAuth
func (h *Handler) exportWaste(c *gin.Context) {
	p, ok := parsePeriod(c)
	if !ok {
		return
	}
	exportStream(c, p, func(ctx *gin.Context) (string, error) {
		return h.svc.ExportWaste(ctx.Request.Context(), ctx.Writer, p)
	}, "waste")
}
