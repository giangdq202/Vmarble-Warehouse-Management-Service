package events

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
)

const reconnectDelay = 5 * time.Second

// Listener opens a dedicated PostgreSQL connection (not from the pool) and
// runs LISTEN on pgChannel. Every notification payload is decoded and
// forwarded to the Broker.
//
// The Listener MUST use its own pgx.Connect connection — sharing the pool
// would block the pool's connection slots with a long-lived LISTEN session.
type Listener struct {
	connStr string
	broker  *Broker
}

func NewListener(connStr string, broker *Broker) *Listener {
	return &Listener{connStr: connStr, broker: broker}
}

// Start runs the listen loop until ctx is cancelled.
// On any connection error it waits reconnectDelay then reconnects automatically.
func (l *Listener) Start(ctx context.Context) {
	for {
		if err := l.listen(ctx); err != nil {
			if ctx.Err() != nil {
				return // clean shutdown
			}
			slog.Warn("events listener disconnected; reconnecting", "err", err, "delay", reconnectDelay)
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(reconnectDelay):
		}
	}
}

func (l *Listener) listen(ctx context.Context) error {
	conn, err := pgx.Connect(ctx, l.connStr)
	if err != nil {
		return err
	}
	defer conn.Close(ctx) //nolint:errcheck

	if _, err := conn.Exec(ctx, "LISTEN vwm_events"); err != nil {
		return err
	}
	slog.Info("events listener ready", "channel", pgChannel)

	for {
		n, err := conn.WaitForNotification(ctx)
		if err != nil {
			return err
		}
		var e Event
		if err := json.Unmarshal([]byte(n.Payload), &e); err != nil {
			slog.Warn("events listener: malformed payload", "payload", n.Payload, "err", err)
			continue
		}
		l.broker.Publish(e)
	}
}
