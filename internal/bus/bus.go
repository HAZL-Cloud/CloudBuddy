// Package bus fans out "something changed" events to the browser's SSE
// streams. Events only say what to refetch; they never carry data.
package bus

import "sync"

type Event struct {
	Type           string `json:"type"` // conversation | conversations | connections | projects | settings
	ConversationID string `json:"conversationId,omitempty"`
}

type Bus struct {
	mu   sync.Mutex
	subs map[chan Event]struct{}
}

func New() *Bus { return &Bus{subs: map[chan Event]struct{}{}} }

func (b *Bus) Subscribe() (<-chan Event, func()) {
	ch := make(chan Event, 64)
	b.mu.Lock()
	b.subs[ch] = struct{}{}
	b.mu.Unlock()
	return ch, func() {
		b.mu.Lock()
		delete(b.subs, ch)
		b.mu.Unlock()
	}
}

func (b *Bus) Publish(e Event) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for ch := range b.subs {
		select {
		case ch <- e:
		default: // a slow browser just misses an invalidation; it refetches on the next one
		}
	}
}

// Conversation announces that one conversation (and the list) changed.
func (b *Bus) Conversation(id string) {
	b.Publish(Event{Type: "conversation", ConversationID: id})
}
