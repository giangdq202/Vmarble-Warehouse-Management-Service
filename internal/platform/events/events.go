package events

// Event is the payload broadcast to SSE clients.
//
// Routing rules: a subscriber receives the event when their UserID matches
// Event.UserID OR their Role appears in Event.Roles. The two fields are
// additive — set both to fan out a personal notification AND inform a role
// audience (e.g. NEW_ASSIGNMENT to a single CNC operator + their managers).
type Event struct {
	// UserID delivers the event to a single user (e.g. assignee). Empty when
	// the event is purely a role-scoped broadcast.
	UserID string `json:"user_id,omitempty"`
	// Roles fan-out the event to every connected client whose role matches one
	// of the listed roles.
	Roles []string `json:"roles,omitempty"`
	Type  string   `json:"type"`

	// WoID anchors most events to a work order so the FE can invalidate the
	// right query without parsing Payload. May be empty for events not tied
	// to a WO. Kept under the original `wo_id` JSON key for backward
	// compatibility with the FE assignment-toast handler.
	WoID string `json:"wo_id,omitempty"`
	// SKU is included on assignment events so the FE can show a meaningful
	// toast without an extra fetch. Other events leave it blank.
	SKU string `json:"sku,omitempty"`

	// Payload carries event-specific fields that don't justify a top-level
	// column. Used for things like {"status":"IN_CUTTING"} on
	// WO_STATUS_CHANGED or {"checkpoint":"CNC_COMPLETE"} on SCAN_CHECKPOINT.
	Payload map[string]any `json:"payload,omitempty"`
}

const (
	// EventTypeNewAssignment fires when a planner assigns a CNC operator to a WO.
	// Routed by UserID (the assignee).
	EventTypeNewAssignment = "NEW_ASSIGNMENT"

	// EventTypeWOStatusChanged fires after a WO transitions state
	// (PLANNED → IN_CUTTING → IN_PROCESSING → COMPLETED → COSTED).
	// Routed by Roles: [planner, cnc_manager, foreman, accountant, admin].
	EventTypeWOStatusChanged = "WO_STATUS_CHANGED"

	// EventTypeCuttingRecorded fires after RecordCut writes a cutting record.
	// Routed by Roles: [planner, accountant, cnc_manager, admin].
	EventTypeCuttingRecorded = "CUTTING_RECORDED"

	// EventTypeScanCheckpoint fires after a barcode scan at any checkpoint
	// (CNC_COMPLETE / FINISHED_GOODS / SHIPPED).
	// Routed by Roles: [cnc_manager, foreman, accountant, admin].
	EventTypeScanCheckpoint = "SCAN_CHECKPOINT"

	// EventTypeCostingComputed fires after ComputeCost persists a record.
	// Routed by Roles: [accountant, admin].
	EventTypeCostingComputed = "COSTING_COMPUTED"

	// EventTypeFGDefectCreated fires after packing.ReportDefect persists a row.
	// Routed by Roles: [planner, foreman, accountant, admin] so dashboards
	// surface defects without polling.
	EventTypeFGDefectCreated = "FG_DEFECT_CREATED"

	// EventTypeFGDefectResolved fires after packing.ResolveDefect closes the
	// defect. Same audience as FG_DEFECT_CREATED.
	EventTypeFGDefectResolved = "FG_DEFECT_RESOLVED"

	// pgChannel is the PostgreSQL LISTEN/NOTIFY channel name.
	pgChannel = "vwm_events"
)
