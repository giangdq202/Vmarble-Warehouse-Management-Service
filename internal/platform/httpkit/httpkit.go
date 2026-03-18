package httpkit

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/vmarble/warehouse-management-service/internal/domain"
)

// healthz godoc
//
// @Summary      Health check
// @Description  Liveness probe
// @Tags         system
// @Accept       json
// @Produce      json
// @Success      200  {object}  map[string]string
// @Router       /healthz [get]
func healthz(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func NewRouter() *gin.Engine {
	r := gin.Default()

	r.GET("/healthz", healthz)

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
