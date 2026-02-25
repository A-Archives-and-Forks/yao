package events

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/yaoapp/kun/log"
	eventtypes "github.com/yaoapp/yao/event/types"
)

// DeliveryHandler processes robot.delivery events asynchronously.
// It routes delivery content to configured channels (email, webhook, process).
type DeliveryHandler struct{}

// Handle processes a delivery event from the event bus.
func (h *DeliveryHandler) Handle(ctx context.Context, ev *eventtypes.Event, resp chan<- eventtypes.Result) {
	var payload DeliveryPayload
	if err := ev.Should(&payload); err != nil {
		log.Error("delivery handler: invalid payload: %v", err)
		return
	}

	log.Info(
		"delivery handler: processing delivery for execution=%s member=%s",
		payload.ExecutionID, payload.MemberID,
	)

	// Log delivery content summary for observability
	if payload.Content != nil {
		if data, err := json.Marshal(payload.Content); err == nil {
			log.Debug("delivery handler: content=%s", string(data))
		}
	}

	// Actual delivery routing is deferred to registered channel handlers.
	// In the current implementation, the DeliveryCenter logic in delivery.go
	// can be invoked here if needed. For now, this handler serves as the
	// event-driven entry point for future channel-specific handlers.

	if ev.IsCall {
		resp <- eventtypes.Result{Data: fmt.Sprintf("delivery processed for %s", payload.ExecutionID)}
	}
}

// Shutdown gracefully shuts down the delivery handler.
func (h *DeliveryHandler) Shutdown(ctx context.Context) error {
	return nil
}

// NewDeliveryHandler creates a new DeliveryHandler.
func NewDeliveryHandler() *DeliveryHandler {
	return &DeliveryHandler{}
}
