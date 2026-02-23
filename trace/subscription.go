package trace

import (
	"fmt"

	"github.com/yaoapp/yao/event"
	eventTypes "github.com/yaoapp/yao/event/types"
	"github.com/yaoapp/yao/trace/types"
)

func dedupKey(u *types.TraceUpdate) string {
	return fmt.Sprintf("%s:%s:%d", u.Type, u.NodeID, u.Timestamp)
}

// Subscribe creates a new subscription for trace updates (replays all historical events from the beginning)
func (m *manager) Subscribe() (<-chan *types.TraceUpdate, error) {
	return m.subscribe(0)
}

// SubscribeFrom creates a subscription starting from a specific timestamp
func (m *manager) SubscribeFrom(since int64) (<-chan *types.TraceUpdate, error) {
	return m.subscribe(since)
}

// subscribe creates a subscription channel that first replays historical
// updates, then streams live events via the event service's Subscriber.
// The subscriber is registered BEFORE reading historical state to prevent
// missing events that occur between the state snapshot and subscriber setup.
func (m *manager) subscribe(since int64) (<-chan *types.TraceUpdate, error) {
	bufferSize := 1000

	out := make(chan *types.TraceUpdate, bufferSize)

	// Register live subscriber FIRST to avoid missing events between snapshot and subscribe.
	liveCh := make(chan *eventTypes.Event, bufferSize)
	traceID := m.traceID
	subID := event.Subscribe("trace.*", liveCh, event.Filter(func(ev *eventTypes.Event) bool {
		update, ok := ev.Payload.(*types.TraceUpdate)
		if !ok {
			return false
		}
		return update.TraceID == traceID
	}))

	// THEN snapshot historical updates (may overlap with live events).
	historical := m.stateGetUpdates(since)

	// Build a set of historical event identifiers for dedup.
	// Key: "type:nodeID:timestamp" is unique enough for trace events.
	histSeen := make(map[string]struct{}, len(historical))
	for _, u := range historical {
		histSeen[dedupKey(u)] = struct{}{}
	}

	go func() {
		defer close(out)
		defer event.Unsubscribe(subID)

		for _, update := range historical {
			out <- update
		}

		for ev := range liveCh {
			update, ok := ev.Payload.(*types.TraceUpdate)
			if !ok {
				continue
			}
			key := dedupKey(update)
			if _, dup := histSeen[key]; dup {
				delete(histSeen, key)
				continue
			}
			out <- update
			if update.Type == types.UpdateTypeComplete {
				return
			}
		}
	}()

	return out, nil
}
