package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/mushroomyuan/vpp-backend/api/contracts"
	"github.com/mushroomyuan/vpp-backend/decision/application/command"
	appport "github.com/mushroomyuan/vpp-backend/decision/application/port"
	"github.com/mushroomyuan/vpp-backend/decision/domain/model"
	"github.com/mushroomyuan/vpp-backend/decision/domain/objective"
	"github.com/mushroomyuan/vpp-backend/decision/domain/plan"
	"github.com/mushroomyuan/vpp-backend/decision/domain/policy"
	"github.com/mushroomyuan/vpp-backend/decision/domain/port"
	platformpostgres "github.com/mushroomyuan/vpp-backend/platform/postgres"
)

const policyID = "11111111-1111-4111-8111-111111111111"

func TestPlanRepository_SaveWritesOutboxAndRollsBackLostCooldown(t *testing.T) {
	repo, db := newPlanDB(t, 1)
	now := planNow()
	if err := repo.Save(context.Background(), readyInput(t, now, planIDs("2"), "idem-1")); err != nil {
		t.Fatal(err)
	}
	if countRows(t, db, &ObjectiveModel{}) != 1 || countRows(t, db, &PowerObjectiveModel{}) != 1 ||
		countRows(t, db, &PlanModel{}) != 1 || countRows(t, db, &StepModel{}) != 1 ||
		countRows(t, db, &CommandModel{}) != 1 || countRows(t, db, &OutboxModel{}) != 1 ||
		countRows(t, db, &ExecutionModel{}) != 0 || countRows(t, db, &CooldownModel{}) != 1 {
		t.Fatalf("rows obj=%d power=%d plan=%d step=%d cmd=%d outbox=%d exec=%d cool=%d",
			countRows(t, db, &ObjectiveModel{}), countRows(t, db, &PowerObjectiveModel{}),
			countRows(t, db, &PlanModel{}), countRows(t, db, &StepModel{}),
			countRows(t, db, &CommandModel{}), countRows(t, db, &OutboxModel{}),
			countRows(t, db, &ExecutionModel{}), countRows(t, db, &CooldownModel{}))
	}

	rejected := readyInput(t, now, planIDs("3"), "idem-rejected")
	rejected.Plan = rejectedPlan(t, now, planIDs("3"), rejected.Objective)
	rejected.Direction = ""
	rejected.CooldownUntil = time.Time{}
	if err := repo.Save(context.Background(), rejected); err != nil {
		t.Fatal(err)
	}
	if countRows(t, db, &OutboxModel{}) != 1 || countRows(t, db, &ObjectiveModel{}) != 2 {
		t.Fatal("rejected plan must be stored without an outbox row")
	}

	repo, db = newPlanDB(t, 1)
	if err := repo.Save(context.Background(), readyInput(t, now, planIDs("2"), "idem-1")); err != nil {
		t.Fatal(err)
	}
	err := repo.Save(context.Background(), readyInput(t, now, planIDs("4"), "idem-2"))
	if !errors.Is(err, plan.ErrCooldownHeld) {
		t.Fatalf("second trigger err = %v", err)
	}
	if countRows(t, db, &ObjectiveModel{}) != 1 || countRows(t, db, &OutboxModel{}) != 1 || countRows(t, db, &PlanModel{}) != 1 {
		t.Fatal("lost cooldown must roll the plan back")
	}
}

func TestPlanRepository_ClaimRecoversAfterCrashAndKeepsLease(t *testing.T) {
	repo, _ := newPlanDB(t, 1)
	now := planNow()
	ids := planIDs("2")
	if err := repo.Save(context.Background(), readyInput(t, now, ids, "idem-1")); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.ClaimDue(context.Background(), "", now, time.Minute); err == nil {
		t.Fatal("empty worker must be rejected")
	}
	first, err := repo.ClaimDue(context.Background(), "worker-a", now, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	wantKey := plan.StepIdempotencyKey(ids.step)
	if first.Attempt != 1 || first.IdempotencyKey != wantKey || first.LeaseToken != 1 {
		t.Fatalf("first claim = %+v", first)
	}
	if _, err := repo.ClaimDue(context.Background(), "worker-b", now.Add(time.Second), time.Minute); !errors.Is(err, plan.ErrNoneDue) {
		t.Fatalf("lease should hide the step, err = %v", err)
	}
	bogus := first
	bogus.WorkerID = "worker-b"
	if err := repo.MarkSubmitted(context.Background(), bogus, "other-task", now); !errors.Is(err, plan.ErrLeaseLost) {
		t.Fatalf("foreign worker err = %v", err)
	}
	if err := repo.MarkSubmitted(context.Background(), first, "task-owner", now); err != nil {
		t.Fatalf("owner submit: %v", err)
	}
	if _, err := repo.ClaimDue(context.Background(), "worker-b", now, time.Minute); !errors.Is(err, plan.ErrNoneDue) {
		t.Fatalf("submitted step must leave the queue, err = %v", err)
	}

	repo, db := newPlanDB(t, 1)
	if err := repo.Save(context.Background(), readyInput(t, now, ids, "idem-1")); err != nil {
		t.Fatal(err)
	}
	crashed, err := repo.ClaimDue(context.Background(), "worker-a", now, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	recovered, err := repo.ClaimDue(context.Background(), "worker-b", now.Add(time.Minute), time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if recovered.Attempt != 2 || recovered.IdempotencyKey != crashed.IdempotencyKey || recovered.StepID != crashed.StepID {
		t.Fatalf("recovered = %+v crashed key %s", recovered, crashed.IdempotencyKey)
	}
	var attempts []ExecutionModel
	if err := db.Order("attempt ASC").Find(&attempts).Error; err != nil {
		t.Fatal(err)
	}
	if len(attempts) != 2 || attempts[0].Status != plan.ExecutionUncertain || attempts[1].Status != plan.ExecutionClaimed {
		t.Fatalf("attempts = %+v", attempts)
	}
	if attempts[0].IdempotencyKey != attempts[1].IdempotencyKey || strings.Contains(attempts[1].IdempotencyKey, "attempt") {
		t.Fatalf("keys = %s %s", attempts[0].IdempotencyKey, attempts[1].IdempotencyKey)
	}
}

func TestPlanRepository_UncertainRetryUsesTheSameKey(t *testing.T) {
	repo, db := newPlanDB(t, 1)
	now := planNow()
	if err := repo.Save(context.Background(), readyInput(t, now, planIDs("2"), "idem-1")); err != nil {
		t.Fatal(err)
	}
	first, err := repo.ClaimDue(context.Background(), "worker-a", now, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	retryAt := now.Add(5 * time.Second)
	if err := repo.MarkUncertain(context.Background(), first, context.DeadlineExceeded.Error(), now, retryAt); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.ClaimDue(context.Background(), "worker-a", now, time.Minute); !errors.Is(err, plan.ErrNoneDue) {
		t.Fatalf("retry is not due yet, err = %v", err)
	}
	second, err := repo.ClaimDue(context.Background(), "worker-b", retryAt, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if second.Attempt != 2 || second.IdempotencyKey != first.IdempotencyKey {
		t.Fatalf("second = %+v first key %s", second, first.IdempotencyKey)
	}
	if err := repo.MarkSubmitted(context.Background(), second, "task-1", retryAt); err != nil {
		t.Fatal(err)
	}
	var planRow PlanModel
	if err := db.First(&planRow).Error; err != nil {
		t.Fatal(err)
	}
	var cmd CommandModel
	if err := db.First(&cmd).Error; err != nil {
		t.Fatal(err)
	}
	if planRow.Status != string(plan.StatusSubmitted) || cmd.DispatchTaskID != "task-1" {
		t.Fatalf("plan=%s task=%s", planRow.Status, cmd.DispatchTaskID)
	}
}

func TestCooldownStore_ConcurrentTriggerOneWinner(t *testing.T) {
	_, db := newPlanDB(t, 8)
	store := NewCooldownStore(platformpostgres.NewPostgresWithDB(db))
	now := planNow()
	until := now.Add(time.Minute)
	key := policy.CooldownKey{TenantID: "tenant-1", PolicyID: policyID, Direction: policy.DirectionCharge}

	ok, err := store.TryTrigger(context.Background(), key, now, until)
	if err != nil || !ok {
		t.Fatalf("first acquire ok=%v err=%v", ok, err)
	}
	ok, err = store.TryTrigger(context.Background(), key, now.Add(time.Second), until.Add(time.Minute))
	if err != nil || ok {
		t.Fatalf("live window ok=%v err=%v", ok, err)
	}
	cooling, err := store.CoolingDown(context.Background(), key, now.Add(time.Second))
	if err != nil || !cooling {
		t.Fatalf("cooling=%v err=%v", cooling, err)
	}
	ok, err = store.TryTrigger(context.Background(), key, until, until.Add(time.Minute))
	if err != nil || !ok {
		t.Fatalf("expired window ok=%v err=%v", ok, err)
	}

	_, db = newPlanDB(t, 8)
	store = NewCooldownStore(platformpostgres.NewPostgresWithDB(db))
	var wins atomic.Int32
	var wg sync.WaitGroup
	start := make(chan struct{})
	errCh := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			acquired, err := store.TryTrigger(context.Background(), key, now, until)
			if err != nil {
				errCh <- err
				return
			}
			if acquired {
				wins.Add(1)
			}
		}()
	}
	close(start)
	wg.Wait()
	close(errCh)
	for err := range errCh {
		t.Error(err)
	}
	if wins.Load() != 1 || countRows(t, db, &CooldownModel{}) != 1 {
		t.Fatalf("wins=%d rows=%d", wins.Load(), countRows(t, db, &CooldownModel{}))
	}
}

func TestPlanExecutionLoop_RevisionMismatchDoesNotDispatch(t *testing.T) {
	repo, db := newPlanDB(t, 1)
	now := planNow()
	if err := repo.Save(context.Background(), readyInput(t, now, planIDs("2"), "idem-1")); err != nil {
		t.Fatal(err)
	}
	dispatch := &scriptDispatch{}
	resource := &revResource{revision: "rev-2", binding: 7}
	loop := newLoop(repo, resource, dispatch)
	if err := loop.Tick(context.Background(), now); err != nil {
		t.Fatal(err)
	}
	if len(dispatch.keys) != 0 || resource.calls != 1 {
		t.Fatalf("dispatch=%v resource calls=%d", dispatch.keys, resource.calls)
	}
	var planRow PlanModel
	var out OutboxModel
	if err := db.First(&planRow).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.First(&out).Error; err != nil {
		t.Fatal(err)
	}
	if planRow.Status != string(plan.StatusStale) || out.Status != outboxStale {
		t.Fatalf("plan=%s outbox=%s", planRow.Status, out.Status)
	}
	if err := loop.Tick(context.Background(), now.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if len(dispatch.keys) != 0 {
		t.Fatalf("stale plan was dispatched: %v", dispatch.keys)
	}
}

func TestPlanExecutionLoop_BindingMismatchAndResourceError(t *testing.T) {
	now := planNow()
	repo, db := newPlanDB(t, 1)
	if err := repo.Save(context.Background(), readyInput(t, now, planIDs("2"), "idem-1")); err != nil {
		t.Fatal(err)
	}
	dispatch := &scriptDispatch{}
	loop := newLoop(repo, &revResource{revision: "rev-1", binding: 9}, dispatch)
	if err := loop.Tick(context.Background(), now); err != nil {
		t.Fatal(err)
	}
	var planRow PlanModel
	if err := db.First(&planRow).Error; err != nil {
		t.Fatal(err)
	}
	if len(dispatch.keys) != 0 || planRow.Status != string(plan.StatusStale) {
		t.Fatalf("binding mismatch plan=%s dispatch=%v", planRow.Status, dispatch.keys)
	}

	repo, db = newPlanDB(t, 1)
	if err := repo.Save(context.Background(), readyInput(t, now, planIDs("3"), "idem-1")); err != nil {
		t.Fatal(err)
	}
	dispatch = &scriptDispatch{}
	loop = newLoop(repo, &revResource{err: errors.New("resource down")}, dispatch)
	if err := loop.Tick(context.Background(), now); err != nil {
		t.Fatal(err)
	}
	planRow = PlanModel{}
	if err := db.First(&planRow).Error; err != nil {
		t.Fatal(err)
	}
	var exec ExecutionModel
	if err := db.First(&exec).Error; err != nil {
		t.Fatal(err)
	}
	if len(dispatch.keys) != 0 || planRow.Status != string(plan.StatusReady) || exec.Status != plan.ExecutionUncertain {
		t.Fatalf("resource error plan=%s exec=%s dispatch=%v", planRow.Status, exec.Status, dispatch.keys)
	}
}

func TestPlanExecutionLoop_TimeoutRetriesSameKey(t *testing.T) {
	repo, db := newPlanDB(t, 1)
	now := planNow()
	ids := planIDs("2")
	if err := repo.Save(context.Background(), readyInput(t, now, ids, "idem-1")); err != nil {
		t.Fatal(err)
	}
	dispatch := &scriptDispatch{errs: []error{context.DeadlineExceeded, nil}}
	resource := &revResource{revision: "rev-1", binding: 7}
	loop := newLoop(repo, resource, dispatch)
	if err := loop.Tick(context.Background(), now); err != nil {
		t.Fatal(err)
	}
	if err := loop.Tick(context.Background(), now.Add(5*time.Second)); err != nil {
		t.Fatal(err)
	}
	if len(dispatch.keys) != 2 || dispatch.keys[0] != dispatch.keys[1] || dispatch.keys[0] != plan.StepIdempotencyKey(ids.step) {
		t.Fatalf("keys = %v", dispatch.keys)
	}
	var planRow PlanModel
	if err := db.First(&planRow).Error; err != nil {
		t.Fatal(err)
	}
	var attempts []ExecutionModel
	if err := db.Order("attempt ASC").Find(&attempts).Error; err != nil {
		t.Fatal(err)
	}
	if planRow.Status != string(plan.StatusSubmitted) || len(attempts) != 2 ||
		attempts[0].Status != plan.ExecutionUncertain || attempts[1].Status != plan.ExecutionSubmitted ||
		attempts[1].DispatchTaskID != "task-1" || attempts[0].IdempotencyKey != attempts[1].IdempotencyKey {
		t.Fatalf("plan=%s attempts=%+v", planRow.Status, attempts)
	}
}

func TestPlanExecutionLoop_IgnoresUnsavedPlan(t *testing.T) {
	repo, _ := newPlanDB(t, 1)
	dispatch := &scriptDispatch{}
	loop := newLoop(repo, &revResource{revision: "rev-1", binding: 7}, dispatch)
	if err := loop.Tick(context.Background(), planNow()); err != nil {
		t.Fatal(err)
	}
	if len(dispatch.keys) != 0 {
		t.Fatalf("unsaved plan was executed: %v", dispatch.keys)
	}
}

type planIdentity struct {
	obj, plan, step, cmd string
}

func planIDs(nibble string) planIdentity {
	return planIdentity{
		obj:  "22222222-2222-4222-8222-22222222222" + nibble,
		plan: "33333333-3333-4333-8333-33333333333" + nibble,
		step: "44444444-4444-4444-8444-44444444444" + nibble,
		cmd:  "55555555-5555-4555-8555-55555555555" + nibble,
	}
}

func readyInput(t *testing.T, now time.Time, id planIdentity, idem string) plan.SaveInput {
	t.Helper()
	obj, err := objective.NewPowerObjective(objective.NewPowerObjectiveParams{
		ID: id.obj, TenantID: "tenant-1",
		Scope:          policy.TargetScope{Type: port.ScopeCU, ID: "cu-1"},
		MetricID:       contracts.MetricElectricalActivePowerSetpoint,
		TargetPowerKW:  -40,
		Window:         objective.TimeWindow{Start: now, End: now.Add(time.Hour)},
		Source:         objective.SourcePolicy,
		SourceID:       policyID,
		IdempotencyKey: idem,
		PolicyID:       policyID,
		PolicyVersion:  1,
	})
	if err != nil {
		t.Fatal(err)
	}
	saved, err := plan.NewPlan(plan.NewPlanParams{
		ID: id.plan, ObjectiveID: obj.ID, TenantID: obj.TenantID,
		PolicyID: obj.PolicyID, PolicyVersion: obj.PolicyVersion,
		ResourceRevision: "rev-1", PlannerID: "immediate", PlannerVersion: "v1",
		Status: plan.StatusReady, Feasibility: plan.FeasibilityFeasible,
		Window: obj.Window, GeneratedAt: now,
		Steps: []plan.PlanStep{{
			ID: id.step, Ordinal: 1, ExecuteAt: now, Status: plan.StatusReady, Version: 1,
			Commands: []plan.PlannedCommand{{
				ID: id.cmd, CUCode: "cu-1",
				MetricID:        contracts.MetricElectricalActivePowerSetpoint,
				Value:           model.FloatCommandValue(-40),
				BindingRevision: 7,
			}},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	return plan.SaveInput{
		Objective: obj, Plan: saved, Direction: policy.DirectionCharge,
		Now: now, CooldownUntil: now.Add(time.Minute),
	}
}

func rejectedPlan(t *testing.T, now time.Time, id planIdentity, obj *objective.PowerObjective) *plan.Plan {
	t.Helper()
	saved, err := plan.NewPlan(plan.NewPlanParams{
		ID: id.plan, ObjectiveID: obj.ID, TenantID: obj.TenantID,
		PolicyID: obj.PolicyID, PolicyVersion: obj.PolicyVersion,
		ResourceRevision: "rev-1", PlannerID: "immediate", PlannerVersion: "v1",
		Status: plan.StatusRejected, Feasibility: plan.FeasibilityInfeasible, UnmetPowerKW: 40,
		Window: obj.Window, GeneratedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	return saved
}

func newLoop(repo *PlanRepository, resource port.ResourcePort, dispatch appport.DispatchPort) *command.PlanExecutionLoop {
	return command.NewPlanExecutionLoop(command.ExecutionDependencies{
		Plans: repo, Resource: resource, Dispatch: dispatch,
		Lease: time.Minute, RetryAfter: 5 * time.Second, Interval: time.Second, WorkerID: "worker-loop",
	})
}

type revResource struct {
	revision string
	binding  int64
	err      error
	calls    int
}

func (r *revResource) ResolveScope(context.Context, port.ScopeQuery) (port.ResolvedScope, error) {
	r.calls++
	if r.err != nil {
		return port.ResolvedScope{}, r.err
	}
	return port.ResolvedScope{
		ScopeType: port.ScopeCU, ScopeID: "cu-1", ResourceRevision: r.revision, PrecheckOK: true,
		Members: []port.ResolvedCU{{
			CUID: "cu-1", Lifecycle: port.LifecycleActive,
			Bindings: []port.ResolvedBinding{{
				MetricID:   contracts.MetricElectricalActivePowerSetpoint,
				AccessMode: port.BindingAccessWrite,
				Enabled:    true,
				Revision:   r.binding,
			}},
		}},
	}, nil
}

type scriptDispatch struct {
	errs []error
	keys []string
	n    int
}

func (s *scriptDispatch) SubmitTask(_ context.Context, task appport.Task) (appport.SubmitResult, error) {
	s.keys = append(s.keys, task.IdempotencyKey)
	var err error
	if s.n < len(s.errs) {
		err = s.errs[s.n]
	}
	s.n++
	if err != nil {
		return appport.SubmitResult{}, err
	}
	return appport.SubmitResult{TaskID: "task-1", Status: "accepted"}, nil
}

var dbSeq atomic.Int64

func newPlanDB(t *testing.T, conns int) (*PlanRepository, *gorm.DB) {
	t.Helper()
	name := strings.NewReplacer("/", "_", " ", "_").Replace(t.Name())
	dsn := fmt.Sprintf("file:%s_%d?mode=memory&cache=shared&_pragma=busy_timeout(5000)", name, dbSeq.Add(1))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	sqlDB.SetMaxOpenConns(conns)
	if err := db.AutoMigrate(
		&ObjectiveModel{}, &PowerObjectiveModel{}, &PlanModel{}, &StepModel{},
		&CommandModel{}, &ExecutionModel{}, &CooldownModel{}, &OutboxModel{},
	); err != nil {
		t.Fatal(err)
	}
	return NewPlanRepository(platformpostgres.NewPostgresWithDB(db)), db
}

func countRows(t *testing.T, db *gorm.DB, model any) int64 {
	t.Helper()
	var n int64
	if err := db.Model(model).Count(&n).Error; err != nil {
		t.Fatal(err)
	}
	return n
}

func planNow() time.Time {
	return time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
}
