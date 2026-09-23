package collector

import (
	"fmt"
	"sync"
)

// subscriberBuffer is the channel depth handed to each subscriber. A small
// buffer absorbs a slow render frame without dropping the newest sample.
const subscriberBuffer = 8

// broadcaster fans out samples to any number of subscribers.
//
// The broadcaster owns every channel it hands out and is the only party that
// closes them. Publishing and closing both happen under the same mutex, so a
// send can never race a close. The previous per-collector implementations
// snapshotted the subscriber list, released the lock, and then sent from
// separate goroutines while Unsubscribe closed the same channels, which panics
// with "send on closed channel" under load.
//
// Sends are non-blocking: a subscriber that is not keeping up drops samples
// rather than stalling collection. Dropping is the right trade-off for a
// dashboard, where only the newest sample matters.
type broadcaster[T any] struct {
	mu     sync.Mutex
	subs   map[string]chan T
	nextID uint64
	closed bool
}

func newBroadcaster[T any]() *broadcaster[T] {
	return &broadcaster[T]{subs: make(map[string]chan T)}
}

// subscribe registers a new subscriber and returns its channel and id. The
// channel is closed by unsubscribe or closeAll, never by the caller.
func (b *broadcaster[T]) subscribe(buffer int) (<-chan T, string) {
	if buffer < 1 {
		buffer = 1
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	ch := make(chan T, buffer)

	if b.closed {
		// The collector has already stopped; hand back a closed channel so the
		// caller's range loop terminates immediately instead of blocking.
		close(ch)
		return ch, ""
	}

	b.nextID++
	id := fmt.Sprintf("sub-%d", b.nextID)
	b.subs[id] = ch

	return ch, id
}

// unsubscribe removes a subscriber and closes its channel. Unknown ids are
// ignored, so it is safe to call more than once.
func (b *broadcaster[T]) unsubscribe(id string) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if ch, ok := b.subs[id]; ok {
		delete(b.subs, id)
		close(ch)
	}
}

// publish delivers v to every subscriber, skipping any whose buffer is full.
func (b *broadcaster[T]) publish(v T) {
	b.mu.Lock()
	defer b.mu.Unlock()

	for _, ch := range b.subs {
		select {
		case ch <- v:
		default:
			// Subscriber is behind; drop this sample for it.
		}
	}
}

// closeAll closes every subscriber channel and marks the broadcaster closed.
// Subsequent publishes are no-ops and subsequent subscribes return a closed
// channel.
func (b *broadcaster[T]) closeAll() {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.closed {
		return
	}
	b.closed = true

	for id, ch := range b.subs {
		delete(b.subs, id)
		close(ch)
	}
}

// subscriberCount reports the number of active subscribers.
func (b *broadcaster[T]) subscriberCount() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.subs)
}
