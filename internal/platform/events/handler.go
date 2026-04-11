package events

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/vmarble/warehouse-management-service/internal/platform/auth"
)

// Handler exposes the SSE stream endpoint.
type Handler struct {
	broker *Broker
}

func NewHandler(broker *Broker) *Handler {
	return &Handler{broker: broker}
}

// Register adds GET /notifications/stream to the router group.
// The route sits under /api/v1 so auth.Middleware is already applied.
func (h *Handler) Register(rg *gin.RouterGroup) {
	rg.GET("/notifications/stream", h.stream)
}

// stream upgrades the HTTP connection to an SSE stream.
// It subscribes to the broker for the authenticated user's events and
// writes them until the client disconnects.
//
// Required headers sent to client:
//
//	Content-Type:      text/event-stream
//	Cache-Control:     no-cache
//	X-Accel-Buffering: no   ← prevents Nginx from buffering the stream
func (h *Handler) stream(c *gin.Context) {
	id, ok := auth.FromContext(c)
	if !ok {
		c.Status(http.StatusUnauthorized)
		return
	}

	ch, unsubscribe := h.broker.Subscribe(id.UserID)
	defer unsubscribe()

	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")

	ctx := c.Request.Context()
	c.Stream(func(_ io.Writer) bool {
		select {
		case e, open := <-ch:
			if !open {
				return false
			}
			data, err := json.Marshal(e)
			if err != nil {
				slog.Warn("events handler: marshal error", "err", err)
				return true
			}
			c.SSEvent("message", string(data))
			return true
		case <-ctx.Done():
			return false
		}
	})
}
