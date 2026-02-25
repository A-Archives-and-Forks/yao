package events

// Robot event type constants for event.Push integration.
// Events are fire-and-forget; handlers are registered via event.Register().
const (
	TaskNeedInput = "robot.task.need_input"
	TaskFailed    = "robot.task.failed"
	TaskCompleted = "robot.task.completed"
	ExecWaiting   = "robot.exec.waiting"
	ExecResumed   = "robot.exec.resumed"
	ExecCompleted = "robot.exec.completed"
	ExecFailed    = "robot.exec.failed"
	ExecCancelled = "robot.exec.cancelled"
	Delivery      = "robot.delivery"
)

// NeedInputPayload is the event payload for TaskNeedInput / ExecWaiting events.
type NeedInputPayload struct {
	ExecutionID string `json:"execution_id"`
	MemberID    string `json:"member_id"`
	TeamID      string `json:"team_id"`
	TaskID      string `json:"task_id"`
	Question    string `json:"question"`
	ChatID      string `json:"chat_id,omitempty"`
}

// ExecPayload is a generic execution event payload.
type ExecPayload struct {
	ExecutionID string `json:"execution_id"`
	MemberID    string `json:"member_id"`
	TeamID      string `json:"team_id"`
	Status      string `json:"status,omitempty"`
	Error       string `json:"error,omitempty"`
	ChatID      string `json:"chat_id,omitempty"`
}

// TaskPayload is the event payload for TaskFailed / TaskCompleted events.
type TaskPayload struct {
	ExecutionID string `json:"execution_id"`
	MemberID    string `json:"member_id"`
	TeamID      string `json:"team_id"`
	TaskID      string `json:"task_id"`
	Error       string `json:"error,omitempty"`
	ChatID      string `json:"chat_id,omitempty"`
}

// DeliveryPayload is the event payload for Delivery events.
type DeliveryPayload struct {
	ExecutionID string      `json:"execution_id"`
	MemberID    string      `json:"member_id"`
	TeamID      string      `json:"team_id"`
	ChatID      string      `json:"chat_id,omitempty"`
	Result      interface{} `json:"result,omitempty"`
	Content     interface{} `json:"content,omitempty"`     // DeliveryContent from agent
	Preferences interface{} `json:"preferences,omitempty"` // DeliveryPreferences for routing
}
