package context

import "time"

// EventType represents the type of agent lifecycle event
type EventType int

const (
	EventRequestStart EventType = iota
	EventPhase
	EventPhaseDone
	EventPhaseSkip
	EventLLMCall
	EventLLMDone
	EventToolCall
	EventToolDone
	EventHook
	EventHookDone
	EventA2AStart
	EventA2ADone
	EventRequestEnd
	EventContextFork
	EventContextRelease
)

// AgentEventMsg is sent from RequestLogger to the TUI Program
type AgentEventMsg struct {
	RequestID   string
	ParentID    string
	AssistantID string
	Event       EventType
	Data        map[string]interface{}
}

// AppLogLevel represents the severity of application-side output
type AppLogLevel int

const (
	AppLogLevelLog AppLogLevel = iota
	AppLogLevelInfo
	AppLogLevelWarn
	AppLogLevelError
	AppLogLevelException
)

// AppLogMsg is sent from the DevWriter (gou layer) to the TUI Program
type AppLogMsg struct {
	Level   AppLogLevel
	Content string
}

// AppLogEntry stores a single application output entry
type AppLogEntry struct {
	Level   AppLogLevel
	Content string
	Time    time.Time
}

// PanelStatus represents the lifecycle state of a request panel
type PanelStatus int

const (
	PanelRunning PanelStatus = iota
	PanelSuccess
	PanelFailed
)

// NodeKind represents the type of a tree node within a request panel
type NodeKind int

const (
	NodePhase NodeKind = iota
	NodeLLM
	NodeTool
	NodeHook
	NodeA2A
)

// NodeStatus represents the state of a tree node
type NodeStatus int

const (
	NodePending NodeStatus = iota
	NodeRunning
	NodeDone
	NodeFailed
)

// TickMsg triggers periodic UI refresh for elapsed time display
type TickMsg time.Time
