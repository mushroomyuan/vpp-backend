package command

import (
	"context"
	"errors"
	"math"
	"testing"
	"time"

	"github.com/mushroomyuan/vpp-backend/api/contracts"
	"github.com/mushroomyuan/vpp-backend/decision/adapter/outbound/memory"
	"github.com/mushroomyuan/vpp-backend/decision/domain/allocation"
	dctx "github.com/mushroomyuan/vpp-backend/decision/domain/context"
	"github.com/mushroomyuan/vpp-backend/decision/domain/evaluation"
	"github.com/mushroomyuan/vpp-backend/decision/domain/plan"
	"github.com/mushroomyuan/vpp-backend/decision/domain/planning"
	"github.com/mushroomyuan/vpp-backend/decision/domain/policy"
	"github.com/mushroomyuan/vpp-backend/decision/domain/port"
)

func TestDecisionCycle_NoTriggerWithinBand(t *testing.T) {
	at := cycleAt()
	// 10 and 90 weight to SOC 50, inside 20..90, even though one CU is under the minimum.
	cycle, tel := newCycle(t, assetPolicy("policy-1", "asset-1"), []cycleCU{
		{id: "cu-a", soc: 10, energy: 100, charge: 100, discharge: 100},
		{id: "cu-b", soc: 90, energy: 100, charge: 100, discharge: 100},
	})
	result, err := cycle.Handle(context.Background(), at)
	if err != nil || len(result.Plans) != 0 {
		t.Fatalf("plans=%d err=%v", len(result.Plans), err)
	}
	if tel.calls != 1 {
		t.Fatalf("telemetry calls = %d", tel.calls)
	}
}

func TestDecisionCycle_SkipsStaleWithoutError(t *testing.T) {
	at := cycleAt()
	cus := []cycleCU{{id: "cu-1", soc: 5, energy: 100, charge: 50, discharge: 50, stale: true}}
	cycle, _ := newCycle(t, cuPolicy("policy-1", "cu-1"), cus)
	result, err := cycle.Handle(context.Background(), at)
	if err != nil || len(result.Plans) != 0 {
		t.Fatalf("plans=%d err=%v", len(result.Plans), err)
	}
}

func TestDecisionCycle_StalePolicyDoesNotBlockAnother(t *testing.T) {
	at := cycleAt()
	repo := memory.NewPolicyRepository()
	mustSave(t, repo, cuPolicy("policy-stale", "cu-old"))
	alive := cuPolicy("policy-ok", "cu-ok")
	alive.Name = "ok"
	alive.SOC.ChargePowerKW = 40
	mustSave(t, repo, alive)
	tel := &cycleTelemetry{byCU: map[string]port.CUSnapshot{
		"cu-old": cycleSnap("cu-old", 5, at, true),
		"cu-ok":  cycleSnap("cu-ok", 5, at, false),
	}}
	resource := &cycleResource{byID: map[string]port.ResolvedScope{
		"cu-old": cycleScope(port.ScopeCU, "cu-old", []cycleCU{{id: "cu-old", soc: 5, energy: 100, charge: 50, discharge: 50}}),
		"cu-ok":  cycleScope(port.ScopeCU, "cu-ok", []cycleCU{{id: "cu-ok", soc: 5, energy: 100, charge: 40, discharge: 40}}),
	}}
	cycle := wireCycle(repo, resource, tel, memory.NewCooldownStore(), nil)
	result, err := cycle.Handle(context.Background(), at)
	if err != nil || len(result.Plans) != 1 || result.Plans[0].PolicyID != "policy-ok" {
		t.Fatalf("plans=%v err=%v", policyIDs(result.Plans), err)
	}
	if tel.calls != 1 {
		t.Fatalf("telemetry calls = %d, want one tenant batch", tel.calls)
	}
}

func TestDecisionCycle_CooldownDoesNotSuppressOppositeDirection(t *testing.T) {
	at := cycleAt()
	p := cuPolicy("policy-1", "cu-1")
	p.Cooldown = time.Minute
	p.SOC.ChargePowerKW = 50
	p.SOC.DischargePowerKW = 80
	cu := cycleCU{id: "cu-1", soc: 5, energy: 200, charge: 80, discharge: 80}
	repo := memory.NewPolicyRepository()
	mustSave(t, repo, p)
	tel := &cycleTelemetry{byCU: map[string]port.CUSnapshot{"cu-1": cycleSnap("cu-1", 5, at, false)}}
	resource := &cycleResource{byID: map[string]port.ResolvedScope{
		"cu-1": cycleScope(port.ScopeCU, "cu-1", []cycleCU{cu}),
	}}
	cycle := wireCycle(repo, resource, tel, memory.NewCooldownStore(), nil)

	first, err := cycle.Handle(context.Background(), at)
	if err != nil || len(first.Plans) != 1 || !near(commandPower(t, first.Plans[0]), -50) {
		t.Fatalf("charge plans=%d power=%v err=%v", len(first.Plans), powerOrZero(t, first.Plans), err)
	}
	tel.byCU["cu-1"] = cycleSnap("cu-1", 95, at, false)
	second, err := cycle.Handle(context.Background(), at.Add(10*time.Second))
	if err != nil || len(second.Plans) != 1 || !near(commandPower(t, second.Plans[0]), 80) {
		t.Fatalf("discharge plans=%d power=%v err=%v", len(second.Plans), powerOrZero(t, second.Plans), err)
	}
	tel.byCU["cu-1"] = cycleSnap("cu-1", 5, at, false)
	third, err := cycle.Handle(context.Background(), at.Add(15*time.Second))
	if err != nil || len(third.Plans) != 0 {
		t.Fatalf("charge should stay in cooldown, plans=%d err=%v", len(third.Plans), err)
	}
	if resource.calls != 1 {
		t.Fatalf("resource calls = %d, want the scope cache to absorb the later ticks", resource.calls)
	}
}

func TestDecisionCycle_AllocatesByCapacityAndSOCMargin(t *testing.T) {
	at := cycleAt()
	p := assetPolicy("policy-1", "asset-1")
	p.SOC.MinSOC = 60
	p.SOC.MaxSOC = 95
	p.SOC.ChargePowerKW = 100
	cus := []cycleCU{
		{id: "cu-a", soc: 20, energy: 100, charge: 100, discharge: 100},
		{id: "cu-b", soc: 80, energy: 100, charge: 100, discharge: 100},
	}
	cycle, _ := newCycle(t, p, cus)
	result, err := cycle.Handle(context.Background(), at)
	if err != nil || len(result.Plans) != 1 {
		t.Fatalf("plans=%d err=%v", len(result.Plans), err)
	}
	got := result.Plans[0]
	if got.Feasibility != plan.FeasibilityFeasible || len(got.Steps) != 1 || !got.Steps[0].ExecuteAt.Equal(at) {
		t.Fatalf("plan = %+v", got)
	}
	values := map[string]float64{}
	for _, cmd := range got.Steps[0].Commands {
		if cmd.MetricID != contracts.MetricElectricalActivePowerSetpoint {
			t.Fatalf("metric %s", cmd.MetricID)
		}
		values[cmd.CUCode] = *cmd.Value.FloatValue
	}
	if !near(values["cu-a"], -80) || !near(values["cu-b"], -20) {
		t.Fatalf("shares = %+v", values)
	}
}

func TestDecisionCycle_ZeroFeasiblePowerRejects(t *testing.T) {
	at := cycleAt()
	min := 0.0
	max := 40.0
	p := cuPolicy("policy-1", "cu-1")
	p.SOC.ChargePowerKW = 100
	cu := cycleCU{
		id: "cu-1", soc: 10, energy: 100, charge: 100, discharge: 100,
		safety: &port.SafetyConstraint{MinValue: &min, MaxValue: &max, Version: 2},
	}
	cycle, _ := newCycle(t, p, []cycleCU{cu})
	result, err := cycle.Handle(context.Background(), at)
	if err != nil || len(result.Plans) != 1 {
		t.Fatalf("plans=%d err=%v", len(result.Plans), err)
	}
	got := result.Plans[0]
	if got.Status != plan.StatusRejected || got.Feasibility != plan.FeasibilityInfeasible || len(got.Steps) != 0 || !near(got.UnmetPowerKW, 100) {
		t.Fatalf("plan status=%s feasibility=%s unmet=%v steps=%d", got.Status, got.Feasibility, got.UnmetPowerKW, len(got.Steps))
	}
}

func TestDecisionCycle_ExpiredScopeSkips(t *testing.T) {
	at := cycleAt()
	repo := memory.NewPolicyRepository()
	mustSave(t, repo, cuPolicy("policy-1", "cu-1"))
	resource := &cycleResource{err: errors.New("resource down")}
	tel := &cycleTelemetry{byCU: map[string]port.CUSnapshot{}}
	cycle := wireCycle(repo, resource, tel, memory.NewCooldownStore(), nil)
	result, err := cycle.Handle(context.Background(), at)
	if err != nil || len(result.Plans) != 0 || tel.calls != 0 {
		t.Fatalf("plans=%d tel=%d err=%v", len(result.Plans), tel.calls, err)
	}
}

func TestDecisionCycle_CooldownHeldIsASkip(t *testing.T) {
	at := cycleAt()
	repo := memory.NewPolicyRepository()
	mustSave(t, repo, cuPolicy("policy-1", "cu-1"))
	tel := &cycleTelemetry{byCU: map[string]port.CUSnapshot{"cu-1": cycleSnap("cu-1", 5, at, false)}}
	resource := &cycleResource{byID: map[string]port.ResolvedScope{
		"cu-1": cycleScope(port.ScopeCU, "cu-1", []cycleCU{{id: "cu-1", soc: 5, energy: 100, charge: 50, discharge: 50}}),
	}}
	cycle := wireCycle(repo, resource, tel, memory.NewCooldownStore(), &savingPlans{err: plan.ErrCooldownHeld})
	result, err := cycle.Handle(context.Background(), at)
	if err != nil || len(result.Plans) != 0 {
		t.Fatalf("plans=%d err=%v", len(result.Plans), err)
	}
}

func TestGroupByTenant_MergesSplitTenants(t *testing.T) {
	policies := []*policy.Policy{
		{ID: "a", TenantID: "tenant-1"},
		nil,
		{ID: "b", TenantID: "tenant-2"},
		{ID: "c", TenantID: "tenant-1"},
	}
	groups := groupByTenant(policies)
	if len(groups) != 2 {
		t.Fatalf("tenants = %d", len(groups))
	}
	if got := idsOf(groups["tenant-1"]); len(got) != 2 || got[0] != "a" || got[1] != "c" {
		t.Fatalf("tenant-1 = %v", got)
	}
	if got := idsOf(groups["tenant-2"]); len(got) != 1 || got[0] != "b" {
		t.Fatalf("tenant-2 = %v", got)
	}
}

func idsOf(policies []*policy.Policy) []string {
	out := make([]string, len(policies))
	for i, p := range policies {
		out[i] = p.ID
	}
	return out
}

func newCycle(t *testing.T, p *policy.Policy, cus []cycleCU) (*RunDecisionCycle, *cycleTelemetry) {
	t.Helper()
	repo := memory.NewPolicyRepository()
	mustSave(t, repo, p)
	tel := &cycleTelemetry{byCU: map[string]port.CUSnapshot{}}
	for _, cu := range cus {
		tel.byCU[cu.id] = cycleSnap(cu.id, cu.soc, cycleAt(), cu.stale)
	}
	resource := &cycleResource{byID: map[string]port.ResolvedScope{
		p.Scope.ID: cycleScope(p.Scope.Type, p.Scope.ID, cus),
	}}
	return wireCycle(repo, resource, tel, memory.NewCooldownStore(), nil), tel
}

func wireCycle(repo *memory.PolicyRepository, resource port.ResourcePort, tel port.TelemetryPort, cooldown *memory.CooldownStore, plans plan.Repository) *RunDecisionCycle {
	if plans == nil {
		plans = &savingPlans{cooldown: cooldown}
	}
	return NewRunDecisionCycle(CycleDependencies{
		Policies: repo,
		Resolver: dctx.NewCachingScopeResolver(dctx.CachingResolverConfig{
			Resource: resource,
			TTL:      time.Hour,
			MaxAge:   time.Hour,
		}),
		Collector: dctx.NewSnapshotCollector(tel),
		Evaluator: evaluation.NewSOCEvaluator(cooldown, time.Minute, seqIDs()),
		Submit: NewSubmitObjectiveHandler(
			planning.NewImmediatePlanner(allocation.NewHeuristicAllocator(), seqIDs()),
			plans,
			nil,
			nopMetrics{},
		),
		StaleAge:        time.Minute,
		Window:          time.Minute,
		DefaultCooldown: time.Minute,
	})
}

// savingPlans is the unit-test plan repository. A ready plan claims the same
// cooldown store the evaluator reads. A rejected plan is recorded and does not.
type savingPlans struct {
	cooldown policy.CooldownStore
	err      error
}

func (s *savingPlans) Save(ctx context.Context, in plan.SaveInput) error {
	if s.err != nil {
		return s.err
	}
	if in.Plan == nil || in.Plan.Status == plan.StatusRejected {
		return nil
	}
	return s.cooldown.MarkTriggered(ctx, policy.CooldownKey{
		TenantID:  in.Plan.TenantID,
		PolicyID:  in.Plan.PolicyID,
		Direction: in.Direction,
	}, in.CooldownUntil)
}

func (s *savingPlans) ClaimDue(context.Context, string, time.Time, time.Duration) (plan.Claim, error) {
	return plan.Claim{}, plan.ErrNoneDue
}

func (s *savingPlans) MarkSubmitted(context.Context, plan.Claim, string, time.Time) error {
	return nil
}

func (s *savingPlans) MarkUncertain(context.Context, plan.Claim, string, time.Time, time.Time) error {
	return nil
}

func (s *savingPlans) MarkStale(context.Context, plan.Claim, string, time.Time) error {
	return nil
}

func (s *savingPlans) MarkFailed(context.Context, plan.Claim, string, time.Time) error {
	return nil
}

func mustSave(t *testing.T, repo *memory.PolicyRepository, p *policy.Policy) {
	t.Helper()
	if err := repo.Create(context.Background(), p); err != nil {
		t.Fatal(err)
	}
}

func cuPolicy(id, cu string) *policy.Policy {
	return makePolicy(id, policy.TargetScope{Type: port.ScopeCU, ID: cu})
}

func assetPolicy(id, asset string) *policy.Policy {
	return makePolicy(id, policy.TargetScope{Type: port.ScopeAsset, ID: asset})
}

func makePolicy(id string, scope policy.TargetScope) *policy.Policy {
	p, err := policy.NewPolicy(policy.NewPolicyParams{
		ID: id, TenantID: "tenant-1", Name: id, Kind: policy.KindSOCThreshold,
		Scope: scope, Enabled: true, Cooldown: time.Minute,
		SOC: &policy.SOCThresholdSpec{MinSOC: 20, MaxSOC: 90, ChargePowerKW: 100, DischargePowerKW: 80},
	})
	if err != nil {
		panic(err)
	}
	return p
}

type cycleCU struct {
	id                string
	soc               float64
	energy            float64
	charge, discharge float64
	stale             bool
	safety            *port.SafetyConstraint
}

func cycleScope(scopeType port.ScopeType, id string, cus []cycleCU) port.ResolvedScope {
	members := make([]port.ResolvedCU, len(cus))
	for i, cu := range cus {
		members[i] = port.ResolvedCU{
			CUID: cu.id, AssetID: "asset-1", Lifecycle: port.LifecycleActive,
			Capabilities: []port.ResolvedCapability{{
				CapabilityID: contracts.CapabilityEnergyStorage, SchemaVersion: 1, Enabled: true,
				Spec: map[string]any{
					"usable_energy_kwh": cu.energy, "max_charge_power_kw": cu.charge, "max_discharge_power_kw": cu.discharge,
				},
			}},
			Bindings: []port.ResolvedBinding{{
				MetricID: contracts.MetricElectricalActivePowerSetpoint, AccessMode: port.BindingAccessWrite,
				Enabled: true, Revision: 3, Safety: cu.safety,
			}},
		}
	}
	return port.ResolvedScope{
		ScopeType: scopeType, ScopeID: id, ResourceRevision: "rev-1", PrecheckOK: true, Members: members,
	}
}

func cycleSnap(cu string, soc float64, at time.Time, stale bool) port.CUSnapshot {
	return port.CUSnapshot{
		CUCode: cu, Stale: stale, UpdatedAt: at,
		Metrics: []port.MetricSample{{
			MetricID: contracts.MetricEnergyStorageStateOfCharge, Value: soc, ObservedAt: at, Quality: port.QualityGood,
		}},
	}
}

type cycleResource struct {
	byID  map[string]port.ResolvedScope
	err   error
	calls int
}

func (f *cycleResource) ResolveScope(_ context.Context, query port.ScopeQuery) (port.ResolvedScope, error) {
	f.calls++
	if f.err != nil {
		return port.ResolvedScope{}, f.err
	}
	scope, ok := f.byID[query.ScopeID]
	if !ok {
		return port.ResolvedScope{}, errors.New("missing scope")
	}
	return scope, nil
}

type cycleTelemetry struct {
	byCU  map[string]port.CUSnapshot
	calls int
}

func (f *cycleTelemetry) GetSnapshots(_ context.Context, query port.SnapshotQuery) ([]port.CUSnapshot, error) {
	f.calls++
	out := make([]port.CUSnapshot, 0, len(query.CUCodes))
	for _, cu := range query.CUCodes {
		snap, ok := f.byCU[cu]
		if !ok {
			continue
		}
		out = append(out, snap)
	}
	return out, nil
}

func commandPower(t *testing.T, p *plan.Plan) float64 {
	t.Helper()
	if len(p.Steps) != 1 || len(p.Steps[0].Commands) != 1 || p.Steps[0].Commands[0].Value.FloatValue == nil {
		t.Fatalf("plan %+v", p)
	}
	return *p.Steps[0].Commands[0].Value.FloatValue
}

func powerOrZero(t *testing.T, plans []*plan.Plan) float64 {
	t.Helper()
	if len(plans) == 0 {
		return 0
	}
	return commandPower(t, plans[0])
}

func policyIDs(plans []*plan.Plan) []string {
	out := make([]string, len(plans))
	for i, p := range plans {
		out[i] = p.PolicyID
	}
	return out
}

func near(got, want float64) bool {
	return math.Abs(got-want) < 1e-6
}

func seqIDs() func() string {
	n := 0
	return func() string {
		n++
		return "id-" + digits(n)
	}
}

func digits(n int) string {
	var buf [8]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}

func cycleAt() time.Time {
	return time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
}
