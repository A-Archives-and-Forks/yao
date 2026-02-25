package events

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	eventtypes "github.com/yaoapp/yao/event/types"
)

func TestDeliveryHandler_Handle(t *testing.T) {
	handler := NewDeliveryHandler()

	t.Run("processes valid delivery payload", func(t *testing.T) {
		ev := &eventtypes.Event{
			Type: Delivery,
			ID:   "test-event-1",
			Payload: DeliveryPayload{
				ExecutionID: "exec-1",
				MemberID:    "member-1",
				TeamID:      "team-1",
				Content:     map[string]interface{}{"summary": "test"},
			},
		}
		resp := make(chan eventtypes.Result, 1)
		handler.Handle(context.Background(), ev, resp)
		// Fire-and-forget: no response expected for Push
	})

	t.Run("handles call mode with response", func(t *testing.T) {
		ev := &eventtypes.Event{
			Type:   Delivery,
			ID:     "test-event-2",
			IsCall: true,
			Payload: DeliveryPayload{
				ExecutionID: "exec-2",
				MemberID:    "member-2",
			},
		}
		resp := make(chan eventtypes.Result, 1)
		handler.Handle(context.Background(), ev, resp)

		result := <-resp
		require.NotNil(t, result.Data)
		assert.Contains(t, result.Data.(string), "exec-2")
	})

	t.Run("handles invalid payload gracefully", func(t *testing.T) {
		ev := &eventtypes.Event{
			Type:    Delivery,
			Payload: "invalid",
		}
		resp := make(chan eventtypes.Result, 1)
		handler.Handle(context.Background(), ev, resp)
	})
}

func TestDeliveryHandler_Shutdown(t *testing.T) {
	handler := NewDeliveryHandler()
	err := handler.Shutdown(context.Background())
	assert.NoError(t, err)
}
