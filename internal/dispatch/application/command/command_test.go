package command

import (
	"context"
	"errors"
	"testing"
	"time"

	appport "github.com/mushroomyuan/vpp-backend/dispatch/application/port"
	"github.com/mushroomyuan/vpp-backend/dispatch/domain/model"
	"github.com/mushroomyuan/vpp-backend/dispatch/domain/port"
	"github.com/mushroomyuan/vpp-backend/dispatch/domain/service"
)

type stubTaskRepo struct {
	saved   *model.DispatchTask
	byCmd   map[string]*model.DispatchTask
	updates int
	saveErr error
}

func (r *stubTaskRepo) Save(_ context.Context, task *model.DispatchTask) error {
	if r.saveErr != nil {
		return r.saveErr
	}
	r.saved = task
	if r.byCmd == nil {
		r.byCmd = map[string]*model.DispatchTask{}
	}
	for _, a := range task.Actions {
		for _, c := range a.Commands {
			r.byCmd[c.ID] = task
		}
	}
	return nil
}
func (r *stubTaskRepo) Update(_ context.Context, task *model.DispatchTask) error {
	r.updates++
	r.saved = task
	return nil
}
func (r *stubTaskRepo) FindByID(context.Context, string) (*model.DispatchTask, error) {
	return r.saved, nil
}
func (r *stubTaskRepo) FindByCommandID(_ context.Context, commandID string) (*model.DispatchTask, error) {
	if t, ok := r.byCmd[commandID]; ok {
		return t, nil
	}
	return nil, errors.New("not found")
}

type stubActionRepo struct{ updates int }

func (r *stubActionRepo) Update(context.Context, *model.DispatchAction) error {
	r.updates++
	return nil
}

type stubCommandRepo struct {
	updates int
	expired []*model.ControlCommand
}

func (r *stubCommandRepo) Update(context.Context, *model.ControlCommand) error {
	r.updates++
	return nil
}
func (r *stubCommandRepo) FindExpiredSending(context.Context, time.Time) ([]*model.ControlCommand, error) {
	return r.expired, nil
}

type stubGateway struct {
	status appport.GatewayAcceptanceStatus
	err    error
	n      int
	last   *model.ControlCommand
}

func (g *stubGateway) ExecuteCommand(_ context.Context, cmd *model.ControlCommand) (*appport.GatewayExecuteResult, error) {
	g.n++
	g.last = cmd
	if g.err != nil {
		return nil, g.err
	}
	return &appport.GatewayExecuteResult{Status: g.status, Message: "ok"}, nil
}

type stubPublisher struct {
	started, completed, failed int
}

func (p *stubPublisher) PublishTaskStarted(context.Context, *model.DispatchTask) error {
	p.started++
	return nil
}
func (p *stubPublisher) PublishTaskCompleted(context.Context, *model.DispatchTask) error {
	p.completed++
	return nil
}
func (p *stubPublisher) PublishTaskFailed(context.Context, *model.DispatchTask) error {
	p.failed++
	return nil
}

var (
	_ port.TaskRepository      = (*stubTaskRepo)(nil)
	_ port.ActionRepository    = (*stubActionRepo)(nil)
	_ port.CommandRepository   = (*stubCommandRepo)(nil)
	_ appport.GatewayPort      = (*stubGateway)(nil)
	_ port.TaskEventPublisher  = (*stubPublisher)(nil)
)

func TestSubmitTask_Accepted(t *testing.T) {
	t.Parallel()
	tasks := &stubTaskRepo{}
	actions := &stubActionRepo{}
	cmds := &stubCommandRepo{}
	gw := &stubGateway{status: appport.GatewayAccepted}
	pub := &stubPublisher{}
	h := submitTaskHandler{
		helper: newDispatchHelper(tasks, actions, cmds, gw, pub, service.NewDispatcher()),
		validator:             service.NewValidator(),
		defaultCommandTimeout: 30 * time.Second,
		defaultMaxRetries:     3,
	}

	res, err := h.Handle(context.Background(), SubmitTask{
		TenantID: "ten", Name: "ctrl",
		Actions: []SubmitActionDTO{{
			Name: "a1", Sequence: 1, ExecutionPolicy: model.Sequential,
			Commands: []SubmitCommandDTO{{
				CUCode: "cu", PointKey: "p", Value: model.FloatCommandValue(1.2),
			}},
		}},
	})
	if err != nil || res.TaskID == "" {
		t.Fatalf("err=%v res=%+v", err, res)
	}
	if tasks.saved == nil || tasks.saved.Status != model.TaskStatusRunning {
		t.Fatalf("task=%+v", tasks.saved)
	}
	if gw.n != 1 || pub.started != 1 {
		t.Fatalf("gw=%d started=%d", gw.n, pub.started)
	}
	cmd := tasks.saved.Actions[0].Commands[0]
	if cmd.Status != model.CommandStatusSending {
		t.Fatalf("cmd status=%s", cmd.Status)
	}
}

func TestSubmitTask_RejectedFailsTask(t *testing.T) {
	t.Parallel()
	tasks := &stubTaskRepo{}
	gw := &stubGateway{status: appport.GatewayRejected}
	pub := &stubPublisher{}
	h := submitTaskHandler{
		helper: newDispatchHelper(tasks, &stubActionRepo{}, &stubCommandRepo{}, gw, pub, service.NewDispatcher()),
		validator:             service.NewValidator(),
		defaultCommandTimeout: time.Second,
		defaultMaxRetries:     1,
	}
	_, err := h.Handle(context.Background(), SubmitTask{
		TenantID: "ten", Name: "ctrl",
		Actions: []SubmitActionDTO{{
			Name: "a1", Sequence: 1,
			Commands: []SubmitCommandDTO{{
				CUCode: "cu", PointKey: "p", Value: model.BoolCommandValue(true),
			}},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if tasks.saved.Status != model.TaskStatusFailed || pub.failed != 1 {
		t.Fatalf("status=%s failedPub=%d", tasks.saved.Status, pub.failed)
	}
}

func TestSubmitTask_Validation(t *testing.T) {
	t.Parallel()
	h := submitTaskHandler{
		helper: newDispatchHelper(&stubTaskRepo{}, &stubActionRepo{}, &stubCommandRepo{},
			&stubGateway{}, &stubPublisher{}, service.NewDispatcher()),
		validator:             service.NewValidator(),
		defaultCommandTimeout: time.Second,
		defaultMaxRetries:     1,
	}
	_, err := h.Handle(context.Background(), SubmitTask{TenantID: "t", Name: "n"})
	if err == nil {
		t.Fatal("want actions required")
	}
}

func TestHandleCommandResult_SuccessContinues(t *testing.T) {
	t.Parallel()
	c1 := &model.ControlCommand{
		ID: "c1", TenantID: "ten", CUCode: "cu", PointKey: "p",
		Value: model.FloatCommandValue(1), Status: model.CommandStatusSending,
		MaxRetries: 1, Timeout: time.Second,
	}
	c2 := &model.ControlCommand{
		ID: "c2", TenantID: "ten", CUCode: "cu", PointKey: "p",
		Value: model.FloatCommandValue(2), Status: model.CommandStatusPending,
		MaxRetries: 1, Timeout: time.Second,
	}
	task := &model.DispatchTask{
		ID: "t1", TenantID: "ten", Name: "n", Status: model.TaskStatusRunning,
		Actions: []*model.DispatchAction{{
			ID: "a1", Sequence: 1, Status: model.ActionStatusRunning,
			ExecutionPolicy: model.Sequential, Commands: []*model.ControlCommand{c1, c2},
		}},
	}
	tasks := &stubTaskRepo{byCmd: map[string]*model.DispatchTask{"c1": task, "c2": task}, saved: task}
	gw := &stubGateway{status: appport.GatewayAccepted}
	cmds := &stubCommandRepo{}
	h := handleCommandResultHandler{
		helper: newDispatchHelper(tasks, &stubActionRepo{}, cmds, gw, &stubPublisher{}, service.NewDispatcher()),
	}

	_, err := h.Handle(context.Background(), HandleCommandResult{
		CommandID: "c1", Result: model.NewSuccessResult(time.Now()),
	})
	if err != nil {
		t.Fatal(err)
	}
	if c1.Status != model.CommandStatusSucceeded {
		t.Fatal(c1.Status)
	}
	// Continuation c2 should have been accepted → Sending.
	if c2.Status != model.CommandStatusSending || gw.n != 1 {
		t.Fatalf("c2=%s gw=%d", c2.Status, gw.n)
	}
}

func TestHandleCommandResult_IdempotentTerminal(t *testing.T) {
	t.Parallel()
	c1 := &model.ControlCommand{
		ID: "c1", TenantID: "ten", CUCode: "cu", PointKey: "p",
		Value: model.FloatCommandValue(1), Status: model.CommandStatusSucceeded,
		MaxRetries: 1, Timeout: time.Second,
	}
	task := &model.DispatchTask{
		ID: "t1", TenantID: "ten", Name: "n", Status: model.TaskStatusRunning,
		Actions: []*model.DispatchAction{{
			ID: "a1", Commands: []*model.ControlCommand{c1},
		}},
	}
	tasks := &stubTaskRepo{byCmd: map[string]*model.DispatchTask{"c1": task}}
	gw := &stubGateway{status: appport.GatewayAccepted}
	h := handleCommandResultHandler{
		helper: newDispatchHelper(tasks, &stubActionRepo{}, &stubCommandRepo{}, gw, &stubPublisher{}, service.NewDispatcher()),
	}
	_, err := h.Handle(context.Background(), HandleCommandResult{
		CommandID: "c1", Result: model.NewSuccessResult(time.Now()),
	})
	if err != nil || gw.n != 0 {
		t.Fatalf("err=%v gw=%d", err, gw.n)
	}
}

func TestTimeoutScanner_HandleExpiredRetry(t *testing.T) {
	t.Parallel()
	c1 := &model.ControlCommand{
		ID: "c1", TenantID: "ten", CUCode: "cu", PointKey: "p",
		Value: model.FloatCommandValue(1), Status: model.CommandStatusSending,
		MaxRetries: 2, RetryCount: 0, Timeout: time.Second,
	}
	task := &model.DispatchTask{
		ID: "t1", TenantID: "ten", Name: "n", Status: model.TaskStatusRunning,
		Actions: []*model.DispatchAction{{
			ID: "a1", Sequence: 1, Status: model.ActionStatusRunning,
			ExecutionPolicy: model.Sequential, Commands: []*model.ControlCommand{c1},
		}},
	}
	tasks := &stubTaskRepo{byCmd: map[string]*model.DispatchTask{"c1": task}, saved: task}
	gw := &stubGateway{status: appport.GatewayAccepted}
	s := &TimeoutScanner{
		helper: newDispatchHelper(tasks, &stubActionRepo{}, &stubCommandRepo{}, gw, &stubPublisher{}, service.NewDispatcher()),
	}
	if err := s.handleExpired(context.Background(), "c1"); err != nil {
		t.Fatal(err)
	}
	// Timeout → Pending → re-dispatched → Sending, RetryCount=1
	if c1.RetryCount != 1 || c1.Status != model.CommandStatusSending || gw.n != 1 {
		t.Fatalf("retryCount=%d status=%s gw=%d", c1.RetryCount, c1.Status, gw.n)
	}
}
