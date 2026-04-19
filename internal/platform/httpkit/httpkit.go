package httpkit

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/vmarble/warehouse-management-service/internal/domain"
)

const (
	defaultPageLimit = 10
	maxPageLimit     = 100
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
type PagedResult[T any] struct {
	Items       []T `json:"items"`
	TotalItems  int `json:"total_items"`
	TotalPages  int `json:"total_pages"`
	CurrentPage int `json:"current_page"`
	Limit       int `json:"limit"`
}

// NewPagedResult builds a PagedResult from the fetched items and metadata.
// items must already be the correct page slice (len <= params.Limit).
func NewPagedResult[T any](items []T, totalItems int, params PageParams) PagedResult[T] {
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
		Items:       items,
		TotalItems:  totalItems,
		TotalPages:  totalPages,
		CurrentPage: params.Page,
		Limit:       limit,
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
