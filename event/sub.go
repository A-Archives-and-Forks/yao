package event

import (
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/yaoapp/yao/event/types"
)

var subIDCounter atomic.Uint64

func nextSubID() string {
	id := subIDCounter.Add(1)
	return fmt.Sprintf("sub-%d", id)
}

// subEntry holds a dynamic subscriber registration.
type subEntry struct {
	id      string
	pattern string
	filter  func(*types.Event) bool
	ch      chan<- *types.Event
}

// subManager manages dynamic subscribers.
type subManager struct {
	mu      sync.RWMutex
	entries map[string]*subEntry // id -> entry
}

func newSubManager() *subManager {
	return &subManager{
		entries: make(map[string]*subEntry),
	}
}

// subscribe adds a dynamic subscriber. Returns the subscription ID.
func (sm *subManager) subscribe(pattern string, ch chan<- *types.Event, opts ...types.FilterOption) string {
	fe := &types.FilterEntry{Pattern: pattern}
	for _, opt := range opts {
		opt(fe)
	}

	id := nextSubID()
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.entries[id] = &subEntry{
		id:      id,
		pattern: pattern,
		filter:  fe.Filter,
		ch:      ch,
	}
	return id
}

// unsubscribe removes a subscriber by ID.
func (sm *subManager) unsubscribe(id string) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	delete(sm.entries, id)
}

// notify sends an event to all matching subscribers (non-blocking).
func (sm *subManager) notify(ev *types.Event) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	for _, entry := range sm.entries {
		if !matchPattern(entry.pattern, ev.Type) {
			continue
		}
		if entry.filter != nil && !entry.filter(ev) {
			continue
		}
		select {
		case entry.ch <- ev:
		default:
			// Subscriber chan full, skip (non-blocking)
		}
	}
}

// clear removes all subscribers. Used during Stop.
func (sm *subManager) clear() {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.entries = make(map[string]*subEntry)
}

// Subscribe dynamically subscribes to events matching the given pattern.
// Returns the subscription ID for later unsubscription.
// Event delivery is non-blocking: if ch is full, the event is skipped.
func Subscribe(pattern string, ch chan<- *types.Event, opts ...types.FilterOption) string {
	return svc.smgr.subscribe(pattern, ch, opts...)
}

// Unsubscribe removes a dynamic subscription by ID.
func Unsubscribe(id string) {
	svc.smgr.unsubscribe(id)
}
