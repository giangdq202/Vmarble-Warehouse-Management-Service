package httpkit

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/vmarble/warehouse-management-service/internal/domain"
)

const (
	defaultPageLimit = 10
	maxPageLimit     = 100

	// MaxOffset bounds how deep an OFFSET-paginated request may walk before
	// the handler refuses with 400. Set to 10,000 because past this depth
	// Postgres still has to scan + discard the first N rows, work_mem
	// pressure spikes, and on a multi-million row table the response
	// degrades to multi-second on warm cache. Tables that legitimately
	// need deeper paging belong on keyset (see Cursor in cursor.go).
	MaxOffset = 10_000

	// CountThreshold is the row-count below which list endpoints should
	// run a real COUNT(*) for accuracy. Above it, callers should fall
	// back to EstimateRowCount: COUNT(*) on a 5M-row table is a multi-
	// second scan even with the visibility map, while the autovacuum
	// estimate is a single index hit and 5–10% off — perfectly fine for
	// "showing ~5.2M items" UI.
	CountThreshold = 50_000
)

// PageParams holds the validated pagination and search query parameters.
// Handlers bind these from query strings using BindPageParams.
type PageParams struct {
	Page   int    // 1-based page number (default: 1)
	Limit  int    // items per page (default: 10, max: 100)
	Search string // optional search keyword (ILIKE match)
	SortBy string // optional column name to sort by
	Order  string // "asc" or "desc" (default: "asc")
}

// PagedResult is the standard paginated response envelope returned by all
// list endpoints that support pagination.
//
// TotalIsEstimate is true when TotalItems came from
// pg_stat_user_tables.n_live_tup rather than a real COUNT(*) — clients
// should render the number as "~1.2M" so users know not to trust the last
// few digits. The field is omitted when false so existing FE code that
// branches on its presence keeps working.
type PagedResult[T any] struct {
	Items           []T  `json:"items"`
	TotalItems      int  `json:"total_items"`
	TotalPages      int  `json:"total_pages"`
	CurrentPage     int  `json:"current_page"`
	Limit           int  `json:"limit"`
	TotalIsEstimate bool `json:"total_is_estimate,omitempty"`
}

// NewPagedResult builds a PagedResult from the fetched items and metadata.
// items must already be the correct page slice (len <= params.Limit).
func NewPagedResult[T any](items []T, totalItems int, params PageParams) PagedResult[T] {
	return newPagedResult(items, totalItems, params, false)
}

// NewPagedResultEstimated is like NewPagedResult but flags TotalItems as an
// estimate. Use this when totalItems came from pg_stat_user_tables instead
// of a real COUNT(*) — see EstimateRowCount.
func NewPagedResultEstimated[T any](items []T, totalItems int, params PageParams) PagedResult[T] {
	return newPagedResult(items, totalItems, params, true)
}

func newPagedResult[T any](items []T, totalItems int, params PageParams, isEstimate bool) PagedResult[T] {
	if items == nil {
		items = []T{}
	}
	limit := params.Limit
	if limit <= 0 {
		limit = defaultPageLimit
	}
	totalPages := (totalItems + limit - 1) / limit
	if totalPages == 0 {
		totalPages = 1
	}
	return PagedResult[T]{
		Items:           items,
		TotalItems:      totalItems,
		TotalPages:      totalPages,
		CurrentPage:     params.Page,
		Limit:           limit,
		TotalIsEstimate: isEstimate,
	}
}

// Offset returns the SQL OFFSET value for the given page parameters.
func (p PageParams) Offset() int {
	return (p.Page - 1) * p.Limit
}

// BindPageParams reads page, limit, search, sort_by, and order from the
// request query string, applies defaults and caps, and returns a validated
// PageParams. It never writes an error response — callers receive safe values
// even when parameters are absent or malformed.
//
// Note: BindPageParams does NOT enforce MaxOffset. Call ValidateOffset
// (or use Error to translate its result) once the params are bound, so the
// 400 path stays explicit at the handler layer.
func BindPageParams(c *gin.Context) PageParams {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	if page < 1 {
		page = 1
	}

	limit, _ := strconv.Atoi(c.DefaultQuery("limit", strconv.Itoa(defaultPageLimit)))
	if limit < 1 {
		limit = defaultPageLimit
	}
	if limit > maxPageLimit {
		limit = maxPageLimit
	}

	order := c.DefaultQuery("order", "asc")
	if order != "asc" && order != "desc" {
		order = "asc"
	}

	return PageParams{
		Page:   page,
		Limit:  limit,
		Search: c.Query("search"),
		SortBy: c.Query("sort_by"),
		Order:  order,
	}
}

// ValidateOffset rejects page/limit combinations that would force Postgres
// to scan past MaxOffset rows. Callers should propagate the returned error
// to httpkit.Error, which maps it to 400. The hint message names the
// alternative — apply a date filter, or migrate the endpoint to keyset
// (see Cursor in cursor.go) for unbounded paging.
func ValidateOffset(p PageParams) error {
	if p.Offset() > MaxOffset {
		return domain.NewBizError(domain.ErrInvalidInput,
			"page * limit exceeds offset cap of 10000; narrow the result set with a date filter or use the cursor-based endpoint")
	}
	return nil
}

// CountOrEstimate returns either a real COUNT(*) (when the table is small
// enough that COUNT is cheap) or pg_stat_user_tables.n_live_tup. The bool
// is true when the second value is an estimate.
//
// Decision rule: read EstimateRowCount once. If the table-wide estimate is
// under CountThreshold, run realCount() — even with WHERE filters, COUNT
// stays sub-second on a 50K-row table. Above threshold, skip the real count
// entirely and return the estimate; FE renders "~1.2M" via the
// total_is_estimate flag.
//
// This trades 5–10% accuracy on the upper bound for predictably bounded
// query latency on hot list endpoints, which is the right call for
// dashboards and admin tables. Endpoints that need exact counts (billing,
// audit reconciliation) should bypass this helper.
func CountOrEstimate(ctx context.Context, pool *pgxpool.Pool, table string, realCount func() (int, error)) (count int, isEstimate bool, err error) {
	est, err := EstimateRowCount(ctx, pool, table)
	if err != nil {
		return 0, false, err
	}
	if est < CountThreshold {
		n, err := realCount()
		if err != nil {
			return 0, false, err
		}
		return n, false, nil
	}
	return int(est), true, nil
}

// healthz godoc
//
// @Summary      Health check
// @Description  Liveness probe
// @Tags         system
// @Accept       json
// @Produce      json
// @Success      200  {object}  map[string]string
// @Router       /healthz [get]
type dbPinger interface {
	Ping(ctx context.Context) error
}

func healthz(pinger dbPinger) gin.HandlerFunc {
	return func(c *gin.Context) {
		pingCtx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
		defer cancel()

		payload := gin.H{
			"client_ip":  c.ClientIP(),
			"user_agent": c.Request.UserAgent(),
			"method":     c.Request.Method,
			"path":       c.Request.URL.Path,
		}

		if err := pinger.Ping(pingCtx); err != nil {
			payload["status"] = "error"
			payload["db"] = "disconnected"
			c.JSON(http.StatusServiceUnavailable, payload)
			return
		}

		payload["status"] = "ok"
		payload["db"] = "connected"
		c.JSON(http.StatusOK, payload)
	}
}

func NewRouter(pinger dbPinger) *gin.Engine {
	r := gin.Default()

	r.GET("/healthz", healthz(pinger))

	return r
}

// Bind reads JSON from the request body into v.
// Returns false and writes an error response if binding fails.
func Bind(c *gin.Context, v any) bool {
	if err := c.ShouldBindJSON(v); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid JSON body"})
		return false
	}
	return true
}

// Error maps domain sentinel errors to HTTP status codes and writes a JSON error response.
func Error(c *gin.Context, err error) {
	status := http.StatusInternalServerError
	switch {
	case errors.Is(err, domain.ErrNotFound):
		status = http.StatusNotFound
	case errors.Is(err, domain.ErrInvalidInput):
		status = http.StatusBadRequest
	case errors.Is(err, ErrInvalidCursor):
		status = http.StatusBadRequest
	case errors.Is(err, domain.ErrInsufficientStock):
		status = http.StatusUnprocessableEntity
	case errors.Is(err, domain.ErrInvalidTransition):
		status = http.StatusConflict
	case errors.Is(err, domain.ErrAreaConservation):
		status = http.StatusUnprocessableEntity
	case errors.Is(err, domain.ErrAlreadyFinalized):
		status = http.StatusConflict
	case errors.Is(err, domain.ErrPreconditionFailed):
		status = http.StatusPreconditionFailed
	case errors.Is(err, domain.ErrConflict):
		status = http.StatusConflict
	}

	slog.Error("request error", "err", err, "status", status)
	c.JSON(status, gin.H{"error": err.Error()})
}

// ParseDateRange reads "from" and "to" query parameters as dates (YYYY-MM-DD or RFC3339).
// Writes a 400 response and returns false when either parameter is missing or malformed.
func ParseDateRange(c *gin.Context) (from, to time.Time, ok bool) {
	fromStr := c.Query("from")
	toStr := c.Query("to")
	if fromStr == "" || toStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "from and to query parameters are required"})
		return time.Time{}, time.Time{}, false
	}
	for _, layout := range []string{time.DateOnly, time.RFC3339} {
		var err error
		from, err = time.Parse(layout, fromStr)
		if err == nil {
			break
		}
	}
	if from.IsZero() {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid from date, use YYYY-MM-DD or RFC3339"})
		return time.Time{}, time.Time{}, false
	}
	for _, layout := range []string{time.DateOnly, time.RFC3339} {
		var err error
		to, err = time.Parse(layout, toStr)
		if err == nil {
			break
		}
	}
	if to.IsZero() {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid to date, use YYYY-MM-DD or RFC3339"})
		return time.Time{}, time.Time{}, false
	}
	return from, to, true
}

// dateRangeLoc is the local timezone applied when a YYYY-MM-DD bound is
// supplied without a time component. The accountant typing "01/05" in Hanoi
// means 00:00 Asia/Ho_Chi_Minh, not 00:00 UTC — without the local
// interpretation a "Tháng 5" filter clips off the first 7 hours of the
// month.
var dateRangeLoc = func() *time.Location {
	loc, err := time.LoadLocation("Asia/Ho_Chi_Minh")
	if err != nil {
		return time.FixedZone("Asia/Ho_Chi_Minh", 7*60*60)
	}
	return loc
}()

// ParseDateRangeFilter reads optional "from"/"to" date-range filter
// parameters. Each may be YYYY-MM-DD (interpreted in Asia/Ho_Chi_Minh) or
// RFC3339 (interpreted as-is). The "to" bound is treated as inclusive
// end-of-day for YYYY-MM-DD inputs — equal from/to returns records that
// fall on that day rather than zero rows (INSTINCTS: To-exclusive
// end-of-day).
//
// Either or both bounds may be omitted; the corresponding return is nil so
// the consuming SQL can branch on `($1::timestamptz IS NULL OR ...)`.
//
// On a malformed value the helper writes 400 and returns ok=false; the
// handler must early-return without touching the service.
func ParseDateRangeFilter(c *gin.Context) (from, to *time.Time, ok bool) {
	if raw := c.Query("from"); raw != "" {
		t, err := parseDateRangeBoundary(raw, false)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid from date, use YYYY-MM-DD or RFC3339"})
			return nil, nil, false
		}
		from = &t
	}
	if raw := c.Query("to"); raw != "" {
		t, err := parseDateRangeBoundary(raw, true)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid to date, use YYYY-MM-DD or RFC3339"})
			return nil, nil, false
		}
		to = &t
	}
	if from != nil && to != nil && !from.Before(*to) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "from must be before to"})
		return nil, nil, false
	}
	return from, to, true
}

func parseDateRangeBoundary(s string, inclusiveDayEnd bool) (time.Time, error) {
	if day, err := time.ParseInLocation(time.DateOnly, s, dateRangeLoc); err == nil {
		if inclusiveDayEnd {
			return day.AddDate(0, 0, 1), nil
		}
		return day, nil
	}
	return time.Parse(time.RFC3339, s)
}
