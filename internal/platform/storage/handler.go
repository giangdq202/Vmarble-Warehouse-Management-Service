package storage

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/vmarble/warehouse-management-service/internal/platform/auth"
)

// Handler exposes the presign endpoint.
type Handler struct {
	presigner Presigner
}

// NewHandler creates a handler. When presigner is nil the endpoint returns 503.
func NewHandler(p Presigner) *Handler {
	return &Handler{presigner: p}
}

// Register wires the upload routes into rg.
// POST /api/v1/uploads/presign requires any authenticated tier (WorkerUp).
func (h *Handler) Register(rg *gin.RouterGroup) {
	rg.POST("/uploads/presign", auth.RequireWorkerUp(), h.presign)
}

type presignRequest struct {
	ContentType string `json:"content_type" binding:"required"`
}

// presign godoc
//
// @Summary      Generate a presigned upload URL for R2 object storage
// @Description  Returns a short-lived PUT URL (5 min) and a permanent public URL.
// @Description  The client PUTs the file bytes directly to upload_url, then stores
// @Description  public_url in the relevant entity (loading exception, defect, rejection).
// @Tags         uploads
// @Accept       json
// @Produce      json
// @Param        body  body      presignRequest  true  "content_type: image/jpeg | image/png | image/webp"
// @Success      200   {object}  PresignResult
// @Failure      400   {object}  map[string]string
// @Failure      503   {object}  map[string]string
// @Security     BearerAuth
// @Router       /api/v1/uploads/presign [post]
func (h *Handler) presign(c *gin.Context) {
	if h.presigner == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "photo storage not configured"})
		return
	}

	var req presignRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "content_type is required"})
		return
	}

	if !AllowedContentTypes[req.ContentType] {
		c.JSON(http.StatusBadRequest, gin.H{"error": "content_type must be image/jpeg, image/png, or image/webp"})
		return
	}

	result, err := h.presigner.Presign(c.Request.Context(), req.ContentType)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to generate upload URL"})
		return
	}

	c.JSON(http.StatusOK, result)
}
