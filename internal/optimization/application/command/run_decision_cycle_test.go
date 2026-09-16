package command

import (
	"context"
	"errors"
	"testing"
	"time"

	appport "github.com/mushroomyuan/vpp-backend/optimization/application/port"
	"github.com/mushroomyuan/vpp-backend/optimization/domain/model"
	"github.com/mushroomyuan/vpp-backend/optimization/domain/port"
	"github.com/mushroomyuan/vpp-backend/optimization/domain/service"
)

type stubEvaluator struct {
	targets []model.Target
	err     error
}

func (s stubEvaluator) Evaluate(context.Context, string, time.Time) ([]model.Target, error) {
	return s.targets, s.err
}

type stubResource struct {
	capacities map[string]float64
	err        error
}

func (s *stubResource) GetCapacityKW(_ context.Context, _ string, scope []string) (map[string]float64, error) {
	if s.err != nil {
		return nil, s.err
	}
	out := make(map[string]float64)
	for _, id := range scope {
		if kw, ok := s.capacities[id]; ok {
			out[id] = kw
		}
	}
	return out, nil
}

type submitCall struct {
	tenantID string
	name     string
	commands []model.CommandSpec
}

type stubDispatch struct {
	calls []submitCall
	err   error
}

func (s *stubDispatch) SubmitTask(_ context.Context, tenantID, name string, commands []model.CommandSpec) (appport.SubmitResult, error) {
	s.calls = append(s.calls, submitCall{tenantID: tenantID, name: name, commands: commands})
	if s.err != nil {
		return appport.SubmitResult{}, s.err
	}
	return appport.SubmitResult{TaskID: "task-" + name, Status: "running"}, nil
}

func pointTarget(cu string, value float64) model.PointTarget {
	return model.PointTarget{
		Tenant:   "tenant-1",
		Src:      model.SourceInternalRule,
		CUCode:   cu,
		PointKey: "active_power_setpoint_kw",
		Value:    model.FloatCommandValue(value),
	}
}

func TestRunDecisionCycle_RequiresTenantID(t *testing.T) {
	h := newRunDecisionCycleHandler(stubEvaluator{}, &stubResource{}, &stubDispatch{}, nil, nil)
	_, err := h.Handle(context.Background(), RunDecisionCycle{})
	if err == nil {
		t.Fatal("expected error when tenant_id is empty")
	}
}

func TestRunDecisionCycle_NoTargetsDoesNotSubmit(t *testing.T) {
	dis := &stubDispatch{}
	h := newRunDecisionCycleHandler(stubEvaluator{}, &stubResource{}, dis, nil, nil)

	res, err := h.Handle(context.Background(), RunDecisionCycle{TenantID: "tenant-1", Now: time.Now()})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.TargetsFired != 0 || res.TasksSubmitted != 0 {
		t.Errorf("expected empty result, got %+v", res)
	}
	if len(dis.calls) != 0 {
		t.Errorf("expected no SubmitTask calls, got %d", len(dis.calls))
	}
}

func TestRunDecisionCycle_PointTargetSubmitsOneTask(t *testing.T) {
	dis := &stubDispatch{}
	h := newRunDecisionCycleHandler(stubEvaluator{
		targets: []model.Target{pointTarget("cu-1", 50)},
	}, &stubResource{}, dis, nil, nil)

	res, err := h.Handle(context.Background(), RunDecisionCycle{TenantID: "tenant-1", Now: time.Now()})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.TargetsFired != 1 || res.TasksSubmitted != 1 {
		t.Fatalf("expected 1 fired / 1 submitted, got %+v", res)
	}
	if len(dis.calls) != 1 {
		t.Fatalf("expected 1 SubmitTask, got %d", len(dis.calls))
	}
	call := dis.calls[0]
	if call.tenantID != "tenant-1" {
		t.Errorf("tenantID = %q, want tenant-1", call.tenantID)
	}
	if call.name != "opt:internal_rule:cu-1" {
		t.Errorf("name = %q, want opt:internal_rule:cu-1", call.name)
	}
	if len(call.commands) != 1 || call.commands[0].CUCode != "cu-1" {
		t.Errorf("unexpected commands: %+v", call.commands)
	}
	if call.commands[0].Value.FloatValue == nil || *call.commands[0].Value.FloatValue != 50 {
		t.Errorf("unexpected command value: %+v", call.commands[0].Value)
	}
}

func TestRunDecisionCycle_TwoTargetsSubmitIndependently(t *testing.T) {
	dis := &stubDispatch{}
	h := newRunDecisionCycleHandler(stubEvaluator{
		targets: []model.Target{pointTarget("cu-1", 50), pointTarget("cu-2", -50)},
	}, &stubResource{}, dis, nil, nil)

	res, err := h.Handle(context.Background(), RunDecisionCycle{TenantID: "tenant-1", Now: time.Now()})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.TargetsFired != 2 || res.TasksSubmitted != 2 {
		t.Fatalf("expected 2/2, got %+v", res)
	}
	if len(dis.calls) != 2 {
		t.Fatalf("expected 2 SubmitTask calls, got %d", len(dis.calls))
	}
}

func TestRunDecisionCycle_EvaluateErrorStillSubmitsSuccessfulTargets(t *testing.T) {
	dis := &stubDispatch{}
	h := newRunDecisionCycleHandler(stubEvaluator{
		targets: []model.Target{pointTarget("cu-ok", 50)},
		err:     errors.New("cu-broken: telemetry unavailable"),
	}, &stubResource{}, dis, nil, nil)

	res, err := h.Handle(context.Background(), RunDecisionCycle{TenantID: "tenant-1", Now: time.Now()})
	if err == nil {
		t.Fatal("expected joined evaluate error")
	}
	if res.TasksSubmitted != 1 {
		t.Fatalf("expected the successful target to still submit, got %+v", res)
	}
}

func TestRunDecisionCycle_AllocateErrorDoesNotBlockOtherTargets(t *testing.T) {
	dis := &stubDispatch{}
	bad := model.AggregateTarget{
		Tenant:     "tenant-1",
		Src:        model.SourceExternalDR,
		Scope:      []string{}, // empty scope → Allocate error
		Metric:     "active_power_kw",
		DeltaValue: -100,
	}
	h := newRunDecisionCycleHandler(stubEvaluator{
		targets: []model.Target{bad, pointTarget("cu-ok", 50)},
	}, &stubResource{}, dis, nil, nil)

	res, err := h.Handle(context.Background(), RunDecisionCycle{TenantID: "tenant-1", Now: time.Now()})
	if err == nil {
		t.Fatal("expected Allocate error to surface")
	}
	if res.TasksSubmitted != 1 {
		t.Fatalf("expected cu-ok to still submit, got %+v", res)
	}
	if len(dis.calls) != 1 || dis.calls[0].name != "opt:internal_rule:cu-ok" {
		t.Errorf("unexpected submits: %+v", dis.calls)
	}
}

func TestRunDecisionCycle_SubmitErrorDoesNotBlockOtherTargets(t *testing.T) {
	dis := &stubDispatch{err: errors.New("dispatch unavailable")}
	h := newRunDecisionCycleHandler(stubEvaluator{
		targets: []model.Target{pointTarget("cu-1", 50), pointTarget("cu-2", -50)},
	}, &stubResource{}, dis, nil, nil)

	res, err := h.Handle(context.Background(), RunDecisionCycle{TenantID: "tenant-1", Now: time.Now()})
	if err == nil {
		t.Fatal("expected SubmitTask error to surface")
	}
	if res.TasksSubmitted != 0 {
		t.Errorf("expected no successful submits, got %+v", res)
	}
	if len(dis.calls) != 2 {
		t.Errorf("expected both targets to attempt SubmitTask, got %d", len(dis.calls))
	}
}

func TestRunDecisionCycle_ThroughRealEvaluator(t *testing.T) {
	tel := &fakeTelemetry{snapshots: map[string]port.Snapshot{
		"cu-1": {CUCode: "cu-1", Metrics: map[string]float64{"soc": 10}},
	}}
	rules := model.Rules{SOCThresholds: []model.SOCThresholdRule{{
		Enabled:          true,
		CUCode:           "cu-1",
		ReadPointKey:     "soc",
		WritePointKey:    "active_power_setpoint_kw",
		MinSOC:           20,
		MaxSOC:           90,
		ChargePowerKW:    50,
		DischargePowerKW: -50,
	}}}
	ev := service.NewEvaluator(rules, tel, time.Minute)
	dis := &stubDispatch{}
	h := NewRunDecisionCycleHandler(ev, &stubResource{}, dis, nil, nil)

	res, err := h.Handle(context.Background(), RunDecisionCycle{TenantID: "tenant-1", Now: time.Now()})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.TasksSubmitted != 1 {
		t.Fatalf("expected 1 submit, got %+v", res)
	}
	if dis.calls[0].commands[0].PointKey != "active_power_setpoint_kw" {
		t.Errorf("unexpected PointKey: %s", dis.calls[0].commands[0].PointKey)
	}
}

func TestNewRunDecisionCycleHandler_PanicsWithoutEvaluator(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("expected panic")
		}
	}()
	NewRunDecisionCycleHandler(nil, &stubResource{}, &stubDispatch{}, nil, nil)
}

func TestRunDecisionCycle_ObserverSeesCycleRulesAndSubmit(t *testing.T) {
	obs := &recordingObserver{}
	dis := &stubDispatch{}
	h := newRunDecisionCycleHandler(stubEvaluator{
		targets: []model.Target{pointTarget("cu-1", 50), pointTarget("cu-2", -50)},
	}, &stubResource{}, dis, nil, obs)

	_, err := h.Handle(context.Background(), RunDecisionCycle{TenantID: "tenant-1", Now: time.Now()})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if obs.cycles != 1 || obs.cycleErr {
		t.Fatalf("cycles=%d cycleErr=%v", obs.cycles, obs.cycleErr)
	}
	if obs.fired[string(model.RuleSOCThreshold)] != 2 {
		t.Fatalf("rules fired = %v", obs.fired)
	}
	if obs.submitOK != 2 || obs.submitFail != 0 {
		t.Fatalf("submit ok=%d fail=%d", obs.submitOK, obs.submitFail)
	}
}

func TestRunDecisionCycle_ObserverRecordsSubmitFailure(t *testing.T) {
	obs := &recordingObserver{}
	h := newRunDecisionCycleHandler(stubEvaluator{
		targets: []model.Target{pointTarget("cu-1", 50)},
	}, &stubResource{}, &stubDispatch{err: errors.New("dispatch down")}, nil, obs)

	_, err := h.Handle(context.Background(), RunDecisionCycle{TenantID: "tenant-1", Now: time.Now()})
	if err == nil {
		t.Fatal("expected submit error")
	}
	if obs.submitFail != 1 || obs.submitOK != 0 {
		t.Fatalf("submit ok=%d fail=%d", obs.submitOK, obs.submitFail)
	}
	if !obs.cycleErr {
		t.Fatal("cycle should be recorded as error")
	}
}

type recordingObserver struct {
	cycles     int
	cycleErr   bool
	fired      map[string]int
	submitOK   int
	submitFail int
}

func (r *recordingObserver) ObserveCycle(_ time.Duration, err error) {
	r.cycles++
	r.cycleErr = err != nil
}
func (r *recordingObserver) ObserveRulesFired(ruleID string, n int) {
	if r.fired == nil {
		r.fired = map[string]int{}
	}
	r.fired[ruleID] += n
}
func (r *recordingObserver) ObserveSubmit(success bool) {
	if success {
		r.submitOK++
	} else {
		r.submitFail++
	}
}
func (r *recordingObserver) ObserveForecast(string) {}

var _ port.Observer = (*recordingObserver)(nil)

// fakeTelemetry is a minimal port.TelemetryPort for the real-Evaluator test.
type fakeTelemetry struct {
	snapshots map[string]port.Snapshot
}

func (f *fakeTelemetry) GetSnapshot(_ context.Context, _, cuCode string) (port.Snapshot, error) {
	return f.snapshots[cuCode], nil
}
