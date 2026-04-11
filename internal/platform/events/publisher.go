package events

import (
	"context"
	"encoding/json"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Publisher calls SELECT pg_notify on the shared pool.
// The notification is delivered to listeners only after the caller's
// transaction commits (PostgreSQL guarantees this).
type Publisher struct {
	pool *pgxpool.Pool
}

func NewPublisher(pool *pgxpool.Pool) *Publisher {
	return &Publisher{pool: pool}
}

// NotifyAssignment fires a NEW_ASSIGNMENT event on pgChannel.
func (p *Publisher) NotifyAssignment(ctx context.Context, userID, woID, sku string) error {
	payload, err := json.Marshal(Event{
		UserID: userID,
		Type:   EventTypeNewAssignment,
		WoID:   woID,
		SKU:    sku,
	})
	if err != nil {
		return err
	}
	_, err = p.pool.Exec(ctx, "SELECT pg_notify($1, $2)", pgChannel, string(payload))
	return err
}
