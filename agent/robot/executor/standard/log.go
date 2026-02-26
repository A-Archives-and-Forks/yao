package standard

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/yaoapp/kun/log"
	"github.com/yaoapp/yao/config"

	robottypes "github.com/yaoapp/yao/agent/robot/types"
)

// execLogger provides structured, developer-facing logging for a single Robot execution.
// Each execution (Executor.ExecuteWithControl) creates one instance; it is passed to
// RunTasks (P2) and Runner (P3) so every log line carries the same identity.
//
// Output routing:
//   - development mode (config.IsDevelopment): human-readable console via fmt.Printf
//   - production mode: structured fields via kun/log
type execLogger struct {
	robot  *robottypes.Robot
	execID string
}

func newExecLogger(robot *robottypes.Robot, execID string) *execLogger {
	return &execLogger{robot: robot, execID: execID}
}

func (l *execLogger) robotID() string {
	if l.robot != nil {
		return l.robot.MemberID
	}
	return ""
}

func (l *execLogger) connector() string {
	if l.robot != nil {
		return l.robot.LanguageModel
	}
	return ""
}

// ---------------------------------------------------------------------------
// P2: Task Overview — called once after RunTasks successfully generates tasks
// ---------------------------------------------------------------------------

func (l *execLogger) logTaskOverview(tasks []robottypes.Task) {
	if config.IsDevelopment() {
		l.devTaskOverview(tasks)
	}
	// Always emit structured log (Info level, hidden in prod unless needed)
	log.With(log.F{
		"robot_id":       l.robotID(),
		"execution_id":   l.execID,
		"phase":          "tasks",
		"task_count":     len(tasks),
		"language_model": l.connector(),
	}).Info("P2 task overview: %d tasks generated", len(tasks))
}

func (l *execLogger) devTaskOverview(tasks []robottypes.Task) {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("%s ══════ P2: Task Overview ══════\n", l.prefix()))
	if l.connector() != "" {
		sb.WriteString(fmt.Sprintf("  Language Model: %s\n", l.connector()))
	}
	for i, t := range tasks {
		desc := t.Description
		if desc == "" && len(t.Messages) > 0 {
			if s, ok := t.Messages[0].GetContentAsString(); ok {
				desc = s
			}
		}
		desc = truncate(desc, 80)
		sb.WriteString(fmt.Sprintf("  #%d %s [%s:%s] %q\n", i+1, t.ID, t.ExecutorType, t.ExecutorID, desc))
	}
	sb.WriteString(fmt.Sprintf("  Total: %d tasks\n", len(tasks)))
	fmt.Print(sb.String())
}

// ---------------------------------------------------------------------------
// P3: Task Input — called before each task execution with the full prompt
// ---------------------------------------------------------------------------

func (l *execLogger) logTaskInput(task *robottypes.Task, prompt string) {
	if config.IsDevelopment() {
		l.devTaskInput(task, prompt)
	}
	log.With(log.F{
		"robot_id":       l.robotID(),
		"execution_id":   l.execID,
		"task_id":        task.ID,
		"executor_type":  string(task.ExecutorType),
		"executor_id":    task.ExecutorID,
		"prompt_len":     len(prompt),
		"language_model": l.connector(),
	}).Info("Task input: %s [%s]", task.ID, task.ExecutorID)
}

func (l *execLogger) devTaskInput(task *robottypes.Task, prompt string) {
	sep := strings.Repeat("─", 40)
	fmt.Printf("%s ▶ Task %s [%s:%s]\n", l.prefix(), task.ID, task.ExecutorType, task.ExecutorID)
	fmt.Printf("  Prompt (%d chars):\n  %s\n%s\n  %s\n",
		len(prompt), sep, indentText(prompt, "  "), sep)
}

// ---------------------------------------------------------------------------
// P3: Task Output — called after each task execution with the result
// ---------------------------------------------------------------------------

func (l *execLogger) logTaskOutput(task *robottypes.Task, result *robottypes.TaskResult) {
	if config.IsDevelopment() {
		l.devTaskOutput(task, result)
	}

	fields := log.F{
		"robot_id":       l.robotID(),
		"execution_id":   l.execID,
		"task_id":        result.TaskID,
		"success":        result.Success,
		"duration_ms":    result.Duration,
		"language_model": l.connector(),
	}
	if result.Output != nil {
		fields["output_type"] = fmt.Sprintf("%T", result.Output)
		fields["output_len"] = outputLen(result.Output)
	}
	if result.Error != "" {
		fields["error"] = result.Error
	}
	if result.Success {
		log.With(fields).Info("Task completed: %s (%dms)", result.TaskID, result.Duration)
	} else {
		log.With(fields).Warn("Task failed: %s (%dms) %s", result.TaskID, result.Duration, result.Error)
	}
}

func (l *execLogger) devTaskOutput(task *robottypes.Task, result *robottypes.TaskResult) {
	if result.Success {
		fmt.Printf("%s ✓ Task %s completed (%dms)\n", l.prefix(), result.TaskID, result.Duration)
		fmt.Printf("  Output: %s\n", outputSummary(result.Output))
	} else {
		fmt.Printf("%s ✗ Task %s failed (%dms)\n", l.prefix(), result.TaskID, result.Duration)
		fmt.Printf("  Error: %s\n", result.Error)
	}
}

// ---------------------------------------------------------------------------
// Agent Call — called after every AgentCaller.Call returns
// ---------------------------------------------------------------------------

func (l *execLogger) logAgentCall(agentID string, result *CallResult) {
	if result == nil {
		return
	}
	if config.IsDevelopment() {
		l.devAgentCall(agentID, result)
	}

	fields := log.F{
		"robot_id":       l.robotID(),
		"execution_id":   l.execID,
		"agent_id":       agentID,
		"content_len":    len(result.Content),
		"language_model": l.connector(),
	}
	if result.Next != nil {
		fields["next_type"] = fmt.Sprintf("%T", result.Next)
		fields["next_len"] = outputLen(result.Next)
	}
	log.With(fields).Info("Agent call: %s (content=%d, next=%T)", agentID, len(result.Content), result.Next)
}

func (l *execLogger) devAgentCall(agentID string, result *CallResult) {
	nextInfo := "<nil>"
	if result.Next != nil {
		nextInfo = fmt.Sprintf("%T(len=%d)", result.Next, outputLen(result.Next))
	}
	fmt.Printf("%s   Agent(%s) → Content(len=%d) Next=%s\n",
		l.prefix(), agentID, len(result.Content), nextInfo)
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func (l *execLogger) prefix() string {
	if l.connector() != "" {
		return fmt.Sprintf("[robot:%s|exec:%s|model:%s]", l.robotID(), l.execID, l.connector())
	}
	return fmt.Sprintf("[robot:%s|exec:%s]", l.robotID(), l.execID)
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

func indentText(s string, prefix string) string {
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		lines[i] = prefix + line
	}
	return strings.Join(lines, "\n")
}

func outputSummary(v interface{}) string {
	if v == nil {
		return "<nil>"
	}
	switch val := v.(type) {
	case string:
		if len(val) > 500 {
			return fmt.Sprintf("string(len=%d) %s...", len(val), val[:500])
		}
		return fmt.Sprintf("string(len=%d) %s", len(val), val)
	case map[string]interface{}:
		keys := make([]string, 0, len(val))
		for k := range val {
			keys = append(keys, k)
		}
		return fmt.Sprintf("map{%s}", strings.Join(keys, ", "))
	default:
		raw, err := json.Marshal(v)
		if err != nil {
			return fmt.Sprintf("%T(marshal-error)", v)
		}
		s := string(raw)
		if len(s) > 500 {
			return fmt.Sprintf("%T(len=%d) %s...", v, len(s), s[:500])
		}
		return fmt.Sprintf("%T %s", v, s)
	}
}

func outputLen(v interface{}) int {
	if v == nil {
		return 0
	}
	switch val := v.(type) {
	case string:
		return len(val)
	default:
		raw, err := json.Marshal(v)
		if err != nil {
			return 0
		}
		return len(raw)
	}
}
