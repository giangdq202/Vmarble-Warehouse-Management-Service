package events

import (
	"context"
	"encoding/json"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Publisher calls SELECT pg_notify on the shared pool.
// The notification is delivered to listeners only after the caller's
// transaction commits (PostgreSQL guarantees this for the session-default
// `transactional` notify behaviour). When called outside a transaction, the
// notify is delivered immediately — both modes are valid for our use cases.
//
// Per-call errors are returned but every caller is expected to treat
// publish as best-effort: the business write must commit even if the notify
// fails. Wire callers to log + continue rather than rolling back.
type Publisher struct {
	pool *pgxpool.Pool
}

func NewPublisher(pool *pgxpool.Pool) *Publisher {
	return &Publisher{pool: pool}
}

func (p *Publisher) publish(ctx context.Context, e Event) error {
	payload, err := json.Marshal(e)
	if err != nil {
		return err
	}
	_, err = p.pool.Exec(ctx, "SELECT pg_notify($1, $2)", pgChannel, string(payload))
	return err
}

// NotifyAssignment fires a NEW_ASSIGNMENT event for the assignee plus the
// manager-side audience that needs to know about new dispatches.
func (p *Publisher) NotifyAssignment(ctx context.Context, userID, woID, sku string) error {
	return p.publish(ctx, Event{
		UserID: userID,
		Roles:  []string{"planner", "cnc_manager", "foreman", "admin"},
		Type:   EventTypeNewAssignment,
		WoID:   woID,
		SKU:    sku,
	})
}

// NotifyWOStatusChanged fires WO_STATUS_CHANGED on the role audience that
// has to react to status transitions (worker queue, manager dashboards,
// accountant cost trigger).
func (p *Publisher) NotifyWOStatusChanged(ctx context.Context, woID, status string) error {
	return p.publish(ctx, Event{
		Roles:   []string{"planner", "cnc_manager", "foreman", "accountant", "admin"},
		Type:    EventTypeWOStatusChanged,
		WoID:    woID,
		Payload: map[string]any{"status": status},
	})
}

// NotifyCuttingRecorded fires CUTTING_RECORDED so the planner queue and
// accountant cost panel can refresh after a cut is reported.
func (p *Publisher) NotifyCuttingRecorded(ctx context.Context, woID, cuttingRecordID string) error {
	return p.publish(ctx, Event{
		Roles:   []string{"planner", "accountant", "cnc_manager", "admin"},
		Type:    EventTypeCuttingRecorded,
		WoID:    woID,
		Payload: map[string]any{"cutting_record_id": cuttingRecordID},
	})
}

// NotifyScanCheckpoint fires SCAN_CHECKPOINT for managers + accountant so
// downstream stages know the unit progressed through CNC / FG / shipped.
func (p *Publisher) NotifyScanCheckpoint(ctx context.Context, woID, checkpoint string) error {
	return p.publish(ctx, Event{
		Roles:   []string{"cnc_manager", "foreman", "accountant", "admin"},
		Type:    EventTypeScanCheckpoint,
		WoID:    woID,
		Payload: map[string]any{"checkpoint": checkpoint},
	})
}

// NotifyCostingComputed fires COSTING_COMPUTED so the accountant panel and
// admin dashboard refresh without polling.
func (p *Publisher) NotifyCostingComputed(ctx context.Context, woID, costingType string) error {
	return p.publish(ctx, Event{
		Roles:   []string{"accountant", "admin"},
		Type:    EventTypeCostingComputed,
		WoID:    woID,
		Payload: map[string]any{"costing_type": costingType},
	})
}

// NotifyFGDefect fires FG_DEFECT_CREATED so planner/foreman dashboards
// surface defects without polling. Audience matches the manager-tier roles
// that triage rework / discard decisions.
func (p *Publisher) NotifyFGDefect(ctx context.Context, fgID, skuCode, reason string) error {
	return p.publish(ctx, Event{
		Roles: []string{"planner", "foreman", "accountant", "admin"},
		Type:  EventTypeFGDefectCreated,
		Payload: map[string]any{
			"fg_id":    fgID,
			"sku_code": skuCode,
			"reason":   reason,
		},
	})
}

// NotifyFGDefectResolved fires FG_DEFECT_RESOLVED after packing.ResolveDefect
// closes the defect — same audience as FG_DEFECT_CREATED so the dashboards
// can clear the open-defect badge in lockstep.
func (p *Publisher) NotifyFGDefectResolved(ctx context.Context, fgID, resolution string) error {
	return p.publish(ctx, Event{
		Roles: []string{"planner", "foreman", "accountant", "admin"},
		Type:  EventTypeFGDefectResolved,
		Payload: map[string]any{
			"fg_id":      fgID,
			"resolution": resolution,
		},
	})
}

// NotifyPlanReload fires PLAN_RELOAD after a v2 supersede wipes
// container_lines on a container (BR-D13). The audience covers every role
// that might be looking at that container's kiosk or planning view: line
// operators (cnc, warehouse) so the kiosk can force-refresh, plus managers
// and admin for dashboard sync. supersededLines lets the kiosk render
// "{N} units cleared, scan from zero" without an extra fetch.
func (p *Publisher) NotifyPlanReload(ctx context.Context, containerID, planID string, version, supersededLines int) error {
	return p.publish(ctx, Event{
		Roles: []string{"warehouse", "cnc", "cnc_manager", "planner", "foreman", "accountant", "admin"},
		Type:  EventTypePlanReload,
		Payload: map[string]any{
			"container_id":     containerID,
			"plan_id":          planID,
			"version":          version,
			"superseded_lines": supersededLines,
		},
	})
}
