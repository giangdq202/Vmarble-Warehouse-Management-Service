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
