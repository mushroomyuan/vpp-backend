package model

import (
	"strings"
	"testing"
	"time"
)

func TestCommandValue_Validate(t *testing.T) {
	t.Parallel()
	if err := FloatCommandValue(1.5).Validate(); err != nil {
		t.Fatal(err)
	}
	if FloatCommandValue(1).Kind() != "float" {
		t.Fatal(FloatCommandValue(1).Kind())
	}
	if err := (CommandValue{}).Validate(); err == nil {
		t.Fatal("want empty error")
	}
	b := true
	i := int64(1)
	if err := (CommandValue{BoolValue: &b, IntValue: &i}).Validate(); err == nil {
		t.Fatal("want multi-field error")
	}
}

func TestControlCommand_Transitions(t *testing.T) {
	t.Parallel()
	c := &ControlCommand{
		ID: "c1", TenantID: "t", CUCode: "cu", PointKey: "p",
		Value: FloatCommandValue(1), Status: CommandStatusPending,
		MaxRetries: 2, Timeout: 5 * time.Second,
	}
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}

	sent := time.Unix(100, 0).UTC()
	if err := c.MarkSending(sent); err != nil {
		t.Fatal(err)
	}
	if c.Status != CommandStatusSending || c.DeadlineAt == nil || !c.DeadlineAt.Equal(sent.Add(5*time.Second)) {
		t.Fatalf("%+v", c)
	}
	if err := c.MarkSending(sent); err == nil {
		t.Fatal("double sending")
	}

	if err := c.MarkSucceeded(NewSuccessResult(time.Now())); err != nil {
		t.Fatal(err)
	}
	if !c.IsTerminal() || c.Status != CommandStatusSucceeded {
		t.Fatal(c.Status)
	}

	c2 := &ControlCommand{ID: "c2", Status: CommandStatusSending, MaxRetries: 1, Timeout: time.Second}
	if err := c2.MarkTimeout(); err != nil {
		t.Fatal(err)
	}
	if !c2.CanRetry() {
		t.Fatal("should retry")
	}
	if err := c2.ResetForRetry(); err != nil {
		t.Fatal(err)
	}
	if c2.Status != CommandStatusPending || c2.RetryCount != 1 || c2.SentAt != nil {
		t.Fatalf("%+v", c2)
	}
	_ = c2.MarkSending(time.Now())
	_ = c2.MarkTimeout()
	if c2.CanRetry() {
		t.Fatal("retries exhausted")
	}

	c3 := &ControlCommand{ID: "c3", Status: CommandStatusPending}
	if err := c3.Cancel(); err != nil {
		t.Fatal(err)
	}
	if c3.Status != CommandStatusCancelled || !c3.IsTerminal() {
		t.Fatal(c3.Status)
	}

	c4 := &ControlCommand{ID: "c4", Status: CommandStatusSending, DeadlineAt: ptrTime(time.Now().Add(-time.Second))}
	if !c4.IsExpired(time.Now()) {
		t.Fatal("want expired")
	}
}

func TestControlCommand_Validate(t *testing.T) {
	t.Parallel()
	base := ControlCommand{
		ID: "c", TenantID: "t", CUCode: "cu", PointKey: "p",
		Value: FloatCommandValue(1), MaxRetries: 0, Timeout: time.Second,
	}
	if err := base.Validate(); err != nil {
		t.Fatal(err)
	}
	bad := base
	bad.PointKey = ""
	if err := bad.Validate(); err == nil || !strings.Contains(err.Error(), "point_key") {
		t.Fatal(err)
	}
	bad = base
	bad.Timeout = 0
	if err := bad.Validate(); err == nil {
		t.Fatal("timeout")
	}
}

func TestDispatchAction_CommandsToDispatch(t *testing.T) {
	t.Parallel()

	pending := func(id string) *ControlCommand {
		return &ControlCommand{ID: id, Status: CommandStatusPending}
	}
	sending := func(id string) *ControlCommand {
		return &ControlCommand{ID: id, Status: CommandStatusSending}
	}

	seq := &DispatchAction{
		ExecutionPolicy: Sequential,
		Commands:        []*ControlCommand{pending("a"), pending("b")},
	}
	got := seq.CommandsToDispatch()
	if len(got) != 1 || got[0].ID != "a" {
		t.Fatalf("seq first = %+v", got)
	}
	seq.Commands[0].Status = CommandStatusSending
	if seq.CommandsToDispatch() != nil {
		t.Fatal("seq with in-flight should wait")
	}

	par := &DispatchAction{
		ExecutionPolicy: Parallel,
		Commands:        []*ControlCommand{pending("a"), sending("b"), pending("c")},
	}
	got = par.CommandsToDispatch()
	if len(got) != 2 || got[0].ID != "a" || got[1].ID != "c" {
		t.Fatalf("parallel = %+v", got)
	}

	done := &DispatchAction{
		Commands: []*ControlCommand{
			{ID: "x", Status: CommandStatusSucceeded},
			{ID: "y", Status: CommandStatusFailed},
		},
	}
	if !done.AllCommandsFinished() || !done.AnyCommandFailed() {
		t.Fatal("finished/failed flags")
	}
}

func TestDispatchTask_NextPendingAndFind(t *testing.T) {
	t.Parallel()
	a2 := &DispatchAction{ID: "a2", Sequence: 2, Status: ActionStatusPending}
	a1 := &DispatchAction{
		ID: "a1", Sequence: 1, Status: ActionStatusRunning,
		Commands: []*ControlCommand{{ID: "c1", Status: CommandStatusSending}},
	}
	task := &DispatchTask{
		ID: "t1", TenantID: "ten", Name: "n", Status: TaskStatusRunning,
		Actions: []*DispatchAction{a2, a1}, // unsorted on purpose
	}
	next := task.NextPendingAction()
	if next == nil || next.ID != "a2" {
		t.Fatalf("next = %+v", next)
	}
	act, cmd := task.FindCommand("c1")
	if act == nil || cmd == nil || act.ID != "a1" {
		t.Fatal("FindCommand")
	}
	if _, miss := task.FindCommand("nope"); miss != nil {
		t.Fatal("miss")
	}

	if err := task.Validate(); err != nil {
		t.Fatal(err)
	}
	if err := (&DispatchTask{}).Validate(); err == nil {
		t.Fatal("empty task")
	}

	if err := task.Complete(); err != nil {
		t.Fatal(err)
	}
	if !task.IsFinished() || task.Cancel() == nil {
		t.Fatal("terminal")
	}
}

func ptrTime(t time.Time) *time.Time { return &t }
