package manager

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yaoapp/yao/agent/robot/store"
	"github.com/yaoapp/yao/agent/robot/types"
)

func TestBuildRobotStatusSnapshot(t *testing.T) {
	m := &Manager{}

	t.Run("nil robot returns nil", func(t *testing.T) {
		snap := m.buildRobotStatusSnapshot(nil)
		assert.Nil(t, snap)
	})

	t.Run("robot with quota", func(t *testing.T) {
		robot := &types.Robot{
			MemberID: "test-member",
			Config: &types.Config{
				Quota: &types.Quota{Max: 5},
			},
		}
		snap := m.buildRobotStatusSnapshot(robot)
		require.NotNil(t, snap)
		assert.Equal(t, 5, snap.MaxQuota)
	})

	t.Run("robot without quota uses default", func(t *testing.T) {
		robot := &types.Robot{
			MemberID: "test-member",
		}
		snap := m.buildRobotStatusSnapshot(robot)
		require.NotNil(t, snap)
		assert.Equal(t, 2, snap.MaxQuota) // robot.MaxQuota() returns 2 for nil config
	})
}

func TestFindWaitingTask(t *testing.T) {
	m := &Manager{}

	t.Run("returns nil when no waiting task id", func(t *testing.T) {
		record := &store.ExecutionRecord{
			Tasks: []types.Task{
				{ID: "task-1"},
			},
		}
		task := m.findWaitingTask(record)
		assert.Nil(t, task)
	})

	t.Run("finds matching task", func(t *testing.T) {
		record := &store.ExecutionRecord{
			WaitingTaskID: "task-2",
			Tasks: []types.Task{
				{ID: "task-1"},
				{ID: "task-2", Status: types.TaskWaitingInput},
				{ID: "task-3"},
			},
		}
		task := m.findWaitingTask(record)
		require.NotNil(t, task)
		assert.Equal(t, "task-2", task.ID)
	})

	t.Run("returns nil when task not found", func(t *testing.T) {
		record := &store.ExecutionRecord{
			WaitingTaskID: "nonexistent",
			Tasks: []types.Task{
				{ID: "task-1"},
			},
		}
		task := m.findWaitingTask(record)
		assert.Nil(t, task)
	})
}

func TestBuildHostContext(t *testing.T) {
	m := &Manager{}

	t.Run("builds context with goals and tasks", func(t *testing.T) {
		robot := &types.Robot{MemberID: "test"}
		record := &store.ExecutionRecord{
			Goals: &types.Goals{Content: "test goals"},
			Tasks: []types.Task{
				{ID: "task-1"},
			},
			WaitingQuestion: "What is the answer?",
		}
		waitingTask := &types.Task{ID: "task-1", Status: types.TaskWaitingInput}

		hostCtx := m.buildHostContext(robot, record, waitingTask)
		require.NotNil(t, hostCtx)
		assert.NotNil(t, hostCtx.Goals)
		assert.Equal(t, "test goals", hostCtx.Goals.Content)
		assert.Len(t, hostCtx.Tasks, 1)
		assert.NotNil(t, hostCtx.CurrentTask)
		assert.Equal(t, "What is the answer?", hostCtx.AgentReply)
	})

	t.Run("builds context without optional fields", func(t *testing.T) {
		robot := &types.Robot{MemberID: "test"}
		record := &store.ExecutionRecord{}

		hostCtx := m.buildHostContext(robot, record, nil)
		require.NotNil(t, hostCtx)
		assert.Nil(t, hostCtx.Goals)
		assert.Nil(t, hostCtx.Tasks)
		assert.Nil(t, hostCtx.CurrentTask)
		assert.Empty(t, hostCtx.AgentReply)
	})
}

func TestProcessHostAction(t *testing.T) {
	m := &Manager{}

	t.Run("wait_for_more returns waiting status", func(t *testing.T) {
		output := &types.HostOutput{
			Reply:       "Please provide more details",
			WaitForMore: true,
		}
		record := &store.ExecutionRecord{}
		robot := &types.Robot{}
		execStore := store.NewExecutionStore()

		resp, err := m.processHostAction(types.NewContext(nil, nil), robot, record, output, execStore)
		require.NoError(t, err)
		assert.Equal(t, "waiting_for_more", resp.Status)
		assert.Equal(t, "Please provide more details", resp.Reply)
		assert.True(t, resp.WaitForMore)
	})

	t.Run("unknown action returns acknowledged", func(t *testing.T) {
		output := &types.HostOutput{
			Reply:  "Got it",
			Action: "unknown_action",
		}
		record := &store.ExecutionRecord{}
		robot := &types.Robot{}
		execStore := store.NewExecutionStore()

		resp, err := m.processHostAction(types.NewContext(nil, nil), robot, record, output, execStore)
		require.NoError(t, err)
		assert.Equal(t, "acknowledged", resp.Status)
	})
}

func TestHandleInteractValidation(t *testing.T) {
	m := &Manager{started: true}

	t.Run("empty member_id returns error", func(t *testing.T) {
		_, err := m.HandleInteract(types.NewContext(nil, nil), "", &InteractRequest{Message: "test"})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "member_id is required")
	})

	t.Run("nil request returns error", func(t *testing.T) {
		_, err := m.HandleInteract(types.NewContext(nil, nil), "member-1", nil)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "message is required")
	})

	t.Run("empty message returns error", func(t *testing.T) {
		_, err := m.HandleInteract(types.NewContext(nil, nil), "member-1", &InteractRequest{})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "message is required")
	})

	t.Run("manager not started returns error", func(t *testing.T) {
		m2 := &Manager{started: false}
		_, err := m2.HandleInteract(types.NewContext(nil, nil), "member-1", &InteractRequest{Message: "test"})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "manager not started")
	})
}

func TestCancelExecutionValidation(t *testing.T) {
	m := &Manager{started: false}

	t.Run("manager not started returns error", func(t *testing.T) {
		err := m.CancelExecution(types.NewContext(nil, nil), "exec-1")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "manager not started")
	})
}
