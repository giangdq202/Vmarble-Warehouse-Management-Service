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
//
// Event payload (`message` event, JSON):
//
//	{
//	  "user_id": "<uuid>",        // present on personal events (e.g. NEW_ASSIGNMENT)
//	  "roles":   ["planner",...], // present on role-broadcast events
//	  "type":    "WO_STATUS_CHANGED",
//	  "wo_id":   "<uuid>",        // anchors the event to a work order when applicable
//	  "sku":     "SKU-001",       // included on NEW_ASSIGNMENT
//	  "payload": { "status": "IN_CUTTING" }
//	}
//
// Event types emitted today (issue #258):
//   - NEW_ASSIGNMENT       — assignee + planner/cnc_manager/foreman/admin
//   - WO_STATUS_CHANGED    — planner/cnc_manager/foreman/accountant/admin
//   - CUTTING_RECORDED     — planner/accountant/cnc_manager/admin
//   - SCAN_CHECKPOINT      — cnc_manager/foreman/accountant/admin
//   - COSTING_COMPUTED     — accountant/admin
//
// @Summary      Subscribe to realtime notifications (SSE)
// @Description  Server-Sent Events stream of events for the authenticated user
// @Description  and their role audience. Emits NEW_ASSIGNMENT, WO_STATUS_CHANGED,
// @Description  CUTTING_RECORDED, SCAN_CHECKPOINT, COSTING_COMPUTED.
// @Tags         events
// @Produce      text/event-stream
// @Success      200 {string} string "event-stream"
// @Failure      401 {object} map[string]string
// @Router       /api/v1/notifications/stream [get]
// @Security     BearerAuth
func (h *Handler) stream(c *gin.Context) {
	id, ok := auth.FromContext(c)
	if !ok {
		c.Status(http.StatusUnauthorized)
		return
	}

	ch, unsubscribe := h.broker.Subscribe(id.UserID, string(id.Role))
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
