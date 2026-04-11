package events

import "sync"

// Broker fans out events to all subscribed SSE clients, keyed by user ID.
// It is safe for concurrent use.
type Broker struct {
	mu      sync.Mutex
	clients map[string]map[int64]chan Event
	nextID  int64
}

func NewBroker() *Broker {
	return &Broker{
		clients: make(map[string]map[int64]chan Event),
	}
}

// Subscribe registers a new SSE client for the given userID.
// It returns a read-only channel and an unsubscribe function.
// The caller MUST call unsubscribe when the client disconnects to prevent memory leaks.
func (b *Broker) Subscribe(userID string) (<-chan Event, func()) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.nextID++
	id := b.nextID

	ch := make(chan Event, 16) // buffered so Publish never blocks on a slow client
	if b.clients[userID] == nil {
		b.clients[userID] = make(map[int64]chan Event)
	}
	b.clients[userID][id] = ch

	unsubscribe := func() {
		b.mu.Lock()
		defer b.mu.Unlock()
		delete(b.clients[userID], id)
		if len(b.clients[userID]) == 0 {
			delete(b.clients, userID)
		}
		close(ch)
	}
	return ch, unsubscribe
}

// Publish sends an event to all clients subscribed for event.UserID.
// Events are dropped (not queued) if a client's channel buffer is full.
func (b *Broker) Publish(e Event) {
	b.mu.Lock()
	defer b.mu.Unlock()

	for _, ch := range b.clients[e.UserID] {
		select {
		case ch <- e:
		default:
			// client too slow; drop rather than block
		}
	}
}
