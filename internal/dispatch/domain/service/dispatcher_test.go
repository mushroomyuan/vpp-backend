package service

import (
	"errors"
	"testing"
	"time"

	"github.com/mushroomyuan/vpp-backend/dispatch/domain"
	"github.com/mushroomyuan/vpp-backend/dispatch/domain/model"
)

func TestValidator_ValidateTask(t *testing.T) {
	t.Parallel()
	v := NewValidator()
	task := validTask(t)
	if err := v.ValidateTask(task); err != nil {
		t.Fatal(err)
	}

	dup := validTask(t)
	dup.Actions = append(dup.Actions, &model.DispatchAction{
		ID: "a2", Name: "dup", Sequence: 1, Status: model.ActionStatusPending,
		Commands: []*model.ControlCommand{validCmd("c9")},
	})
	if err := v.ValidateTask(dup); err == nil {
		t.Fatal("want duplicate sequence")
	}

	empty := validTask(t)
	empty.Actions[0].Commands = nil
	if err := v.ValidateTask(empty); err == nil {
		t.Fatal("want empty commands")
	}

	badVal := validTask(t)
	badVal.Actions[0].Commands[0].Value = model.CommandValue{}
	if err := v.ValidateTask(badVal); err == nil {
		t.Fatal("want value error")
	}
}

func TestDispatcher_SequentialSuccess(t *testing.T) {
	t.Parallel()
	d := NewDispatcher()
	task := runningSequentialTask("c1", "c2")

	out, err := d.OnCommandResult(task, "c1", model.NewSuccessResult(time.Now()))
	if err != nil {
		t.Fatal(err)
	}
	if task.Actions[0].Commands[0].Status != model.CommandStatusSucceeded {
		t.Fatal("c1 not succeeded")
	}
	if len(out.NextCommands) != 1 || out.NextCommands[0].ID != "c2" || out.TaskFinished {
		t.Fatalf("next = %+v finished=%v", out.NextCommands, out.TaskFinished)
	}

	_ = out.NextCommands[0].MarkSending(time.Now())
	out, err = d.OnCommandResult(task, "c2", model.NewSuccessResult(time.Now()))
	if err != nil {
		t.Fatal(err)
	}
	if task.Status != model.TaskStatusCompleted || !out.TaskFinished || !out.TaskChanged {
		t.Fatalf("task=%s finished=%v", task.Status, out.TaskFinished)
	}
	if task.Actions[0].Status != model.ActionStatusCompleted {
		t.Fatal(task.Actions[0].Status)
	}
	if len(out.NextCommands) != 0 {
		t.Fatal(out.NextCommands)
	}
}

func TestDispatcher_FailFastCircuitBreaker(t *testing.T) {
	t.Parallel()
	d := NewDispatcher()
	c1 := validCmd("c1")
	c1.Status = model.CommandStatusSending
	c2 := validCmd("c2") // pending in same action
	c3 := validCmd("c3") // pending in next action
	a1 := &model.DispatchAction{
		ID: "a1", Sequence: 1, Status: model.ActionStatusRunning,
		ExecutionPolicy: model.Sequential,
		Commands:        []*model.ControlCommand{c1, c2},
	}
	a2 := &model.DispatchAction{
		ID: "a2", Sequence: 2, Status: model.ActionStatusPending,
		ExecutionPolicy: model.Sequential,
		Commands:        []*model.ControlCommand{c3},
	}
	task := &model.DispatchTask{
		ID: "t1", TenantID: "ten", Name: "n", Status: model.TaskStatusRunning,
		FailurePolicy: model.FailFast, Actions: []*model.DispatchAction{a1, a2},
	}

	out, err := d.OnCommandResult(task, "c1", model.NewFailureResult("gw", "rejected"))
	if err != nil {
		t.Fatal(err)
	}
	if !out.TaskFinished || task.Status != model.TaskStatusFailed {
		t.Fatalf("task=%s finished=%v", task.Status, out.TaskFinished)
	}
	if c1.Status != model.CommandStatusFailed || c2.Status != model.CommandStatusCancelled {
		t.Fatalf("c1=%s c2=%s", c1.Status, c2.Status)
	}
	if a1.Status != model.ActionStatusFailed || a2.Status != model.ActionStatusCancelled {
		t.Fatalf("a1=%s a2=%s", a1.Status, a2.Status)
	}
	if c3.Status != model.CommandStatusCancelled || len(out.NextCommands) != 0 {
		t.Fatalf("c3=%s next=%v", c3.Status, out.NextCommands)
	}
}

func TestDispatcher_AdvanceToNextAction(t *testing.T) {
	t.Parallel()
	d := NewDispatcher()
	c1 := validCmd("c1")
	c1.Status = model.CommandStatusSending
	c2 := validCmd("c2")
	a1 := &model.DispatchAction{
		ID: "a1", Sequence: 1, Status: model.ActionStatusRunning,
		ExecutionPolicy: model.Sequential, Commands: []*model.ControlCommand{c1},
	}
	a2 := &model.DispatchAction{
		ID: "a2", Sequence: 2, Status: model.ActionStatusPending,
		ExecutionPolicy: model.Sequential, Commands: []*model.ControlCommand{c2},
	}
	task := &model.DispatchTask{
		ID: "t1", TenantID: "ten", Name: "n", Status: model.TaskStatusRunning,
		Actions: []*model.DispatchAction{a1, a2},
	}

	out, err := d.OnCommandResult(task, "c1", model.NewSuccessResult(time.Now()))
	if err != nil {
		t.Fatal(err)
	}
	if a1.Status != model.ActionStatusCompleted || a2.Status != model.ActionStatusRunning {
		t.Fatalf("a1=%s a2=%s", a1.Status, a2.Status)
	}
	if len(out.NextCommands) != 1 || out.NextCommands[0].ID != "c2" || out.TaskFinished {
		t.Fatalf("%+v", out)
	}
}

func TestDispatcher_OnCommandTimeout(t *testing.T) {
	t.Parallel()
	d := NewDispatcher()

	t.Run("retry", func(t *testing.T) {
		t.Parallel()
		c := validCmd("c1")
		c.Status = model.CommandStatusSending
		c.MaxRetries = 2
		c.RetryCount = 0
		task := singleCmdRunningTask(c)

		out, err := d.OnCommandTimeout(task, "c1")
		if err != nil {
			t.Fatal(err)
		}
		if c.Status != model.CommandStatusPending || c.RetryCount != 1 {
			t.Fatalf("%+v", c)
		}
		if len(out.NextCommands) != 1 || out.TaskFinished {
			t.Fatalf("%+v", out)
		}
	})

	t.Run("exhausted fails task", func(t *testing.T) {
		t.Parallel()
		c := validCmd("c1")
		c.Status = model.CommandStatusSending
		c.MaxRetries = 0
		c.RetryCount = 0
		task := singleCmdRunningTask(c)

		out, err := d.OnCommandTimeout(task, "c1")
		if err != nil {
			t.Fatal(err)
		}
		if c.Status != model.CommandStatusFailed || task.Status != model.TaskStatusFailed || !out.TaskFinished {
			t.Fatalf("c=%s task=%s finished=%v", c.Status, task.Status, out.TaskFinished)
		}
	})

	t.Run("idempotent when not sending", func(t *testing.T) {
		t.Parallel()
		c := validCmd("c1")
		c.Status = model.CommandStatusSucceeded
		task := singleCmdRunningTask(c)
		out, err := d.OnCommandTimeout(task, "c1")
		if err != nil || out.TaskFinished || len(out.ChangedCommands) != 0 {
			t.Fatalf("err=%v out=%+v", err, out)
		}
	})

	t.Run("missing command", func(t *testing.T) {
		t.Parallel()
		task := singleCmdRunningTask(validCmd("c1"))
		_, err := d.OnCommandTimeout(task, "missing")
		if !errors.Is(err, domain.ErrCommandNotFound) {
			t.Fatalf("%v", err)
		}
	})
}

func TestDispatcher_ParallelPartialSuccess(t *testing.T) {
	t.Parallel()
	d := NewDispatcher()
	c1 := validCmd("c1")
	c1.Status = model.CommandStatusSending
	c2 := validCmd("c2")
	c2.Status = model.CommandStatusSending
	a := &model.DispatchAction{
		ID: "a1", Sequence: 1, Status: model.ActionStatusRunning,
		ExecutionPolicy: model.Parallel,
		Commands:        []*model.ControlCommand{c1, c2},
	}
	task := &model.DispatchTask{
		ID: "t1", TenantID: "ten", Name: "n", Status: model.TaskStatusRunning,
		Actions: []*model.DispatchAction{a},
	}

	out, err := d.OnCommandResult(task, "c1", model.NewSuccessResult(time.Now()))
	if err != nil {
		t.Fatal(err)
	}
	// c2 still sending → action not complete
	if out.TaskFinished || a.Status != model.ActionStatusRunning || len(out.NextCommands) != 0 {
		t.Fatalf("partial: action=%s finished=%v next=%d", a.Status, out.TaskFinished, len(out.NextCommands))
	}

	out, err = d.OnCommandResult(task, "c2", model.NewSuccessResult(time.Now()))
	if err != nil {
		t.Fatal(err)
	}
	if a.Status != model.ActionStatusCompleted || task.Status != model.TaskStatusCompleted || !out.TaskFinished {
		t.Fatalf("final action=%s task=%s", a.Status, task.Status)
	}
}

func validCmd(id string) *model.ControlCommand {
	return &model.ControlCommand{
		ID: id, TenantID: "ten", CUCode: "cu", PointKey: "set_power",
		Value: model.FloatCommandValue(1), Status: model.CommandStatusPending,
		MaxRetries: 3, Timeout: 30 * time.Second,
	}
}

func validTask(t *testing.T) *model.DispatchTask {
	t.Helper()
	return &model.DispatchTask{
		ID: "t1", TenantID: "ten", Name: "task", Status: model.TaskStatusPending,
		Actions: []*model.DispatchAction{{
			ID: "a1", Name: "act", Sequence: 1, Status: model.ActionStatusPending,
			ExecutionPolicy: model.Sequential,
			Commands:        []*model.ControlCommand{validCmd("c1")},
		}},
	}
}

func runningSequentialTask(ids ...string) *model.DispatchTask {
	cmds := make([]*model.ControlCommand, len(ids))
	for i, id := range ids {
		cmds[i] = validCmd(id)
	}
	cmds[0].Status = model.CommandStatusSending
	return &model.DispatchTask{
		ID: "t1", TenantID: "ten", Name: "n", Status: model.TaskStatusRunning,
		Actions: []*model.DispatchAction{{
			ID: "a1", Sequence: 1, Status: model.ActionStatusRunning,
			ExecutionPolicy: model.Sequential, Commands: cmds,
		}},
	}
}

func singleCmdRunningTask(c *model.ControlCommand) *model.DispatchTask {
	return &model.DispatchTask{
		ID: "t1", TenantID: "ten", Name: "n", Status: model.TaskStatusRunning,
		Actions: []*model.DispatchAction{{
			ID: "a1", Sequence: 1, Status: model.ActionStatusRunning,
			ExecutionPolicy: model.Sequential, Commands: []*model.ControlCommand{c},
		}},
	}
}
