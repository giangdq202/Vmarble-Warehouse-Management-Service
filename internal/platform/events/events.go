package events

// Event is the payload broadcast to SSE clients.
type Event struct {
	UserID string `json:"user_id"`
	Type   string `json:"type"`
	WoID   string `json:"wo_id"`
	SKU    string `json:"sku"`
}

const (
	EventTypeNewAssignment = "NEW_ASSIGNMENT"

	// pgChannel is the PostgreSQL LISTEN/NOTIFY channel name.
	pgChannel = "vwm_events"
)
