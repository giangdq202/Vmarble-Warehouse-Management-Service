package events

import "sync"

// Broker fans out events to all subscribed SSE clients.
//
// Routing: each subscription registers a userID AND a role. A published event
// is delivered to a subscription when (Event.UserID == sub.userID) OR
// (sub.role appears in Event.Roles). The two scopes are additive — an
// assignment event can fan out to the assignee (UserID) AND to managers
// (Roles), and the assignee receives only one copy because the broker
// deduplicates per-channel.
//
// Safe for concurrent use.
type Broker struct {
	mu      sync.Mutex
	clients map[int64]*subscription
	nextID  int64
}

type subscription struct {
	userID string
	role   string
	ch     chan Event
}

func NewBroker() *Broker {
	return &Broker{
		clients: make(map[int64]*subscription),
	}
}

// Subscribe registers a new SSE client. Both userID and role are recorded so
// the same connection can receive personal events (UserID-routed) and
// role-broadcast events. Pass an empty role when the subscriber's role is
// unknown — they will only receive personal events.
//
// Returns a read-only channel and an unsubscribe function. The caller MUST
// call unsubscribe when the client disconnects to prevent memory leaks.
func (b *Broker) Subscribe(userID, role string) (<-chan Event, func()) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.nextID++
	id := b.nextID

	ch := make(chan Event, 16) // buffered so Publish never blocks on a slow client
	b.clients[id] = &subscription{userID: userID, role: role, ch: ch}

	unsubscribe := func() {
		b.mu.Lock()
		defer b.mu.Unlock()
		if _, ok := b.clients[id]; !ok {
			return
		}
		delete(b.clients, id)
		close(ch)
	}
	return ch, unsubscribe
}

// Publish sends the event to every subscription whose userID matches
// Event.UserID OR whose role appears in Event.Roles. Each subscription
// receives the event at most once even when both conditions match.
//
// Events are dropped (not queued) if a client's channel buffer is full —
// favouring liveness over delivery guarantees.
func (b *Broker) Publish(e Event) {
	b.mu.Lock()
	defer b.mu.Unlock()

	roleSet := map[string]struct{}{}
	for _, r := range e.Roles {
		roleSet[r] = struct{}{}
	}

	for _, sub := range b.clients {
		match := false
		if e.UserID != "" && sub.userID == e.UserID {
			match = true
		}
		if !match && sub.role != "" {
			if _, ok := roleSet[sub.role]; ok {
				match = true
			}
		}
		if !match {
			continue
		}
		select {
		case sub.ch <- e:
		default:
			// client too slow; drop rather than block
		}
	}
}
