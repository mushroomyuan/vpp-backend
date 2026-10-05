package command_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/mushroomyuan/vpp-backend/api/contracts"
	"github.com/mushroomyuan/vpp-backend/decision/adapter/outbound/memory"
	"github.com/mushroomyuan/vpp-backend/decision/application"
	"github.com/mushroomyuan/vpp-backend/decision/application/command"
	"github.com/mushroomyuan/vpp-backend/decision/application/query"
	"github.com/mushroomyuan/vpp-backend/decision/domain/policy"
	"github.com/mushroomyuan/vpp-backend/decision/domain/port"
	"github.com/mushroomyuan/vpp-backend/platform/decorator"
)

func TestPolicyCRUD_Lock_Name_AndEnablePrecheck(t *testing.T) {
	repo := memory.NewPolicyRepository()
	resource := &fakeResource{scope: readyScope()}
	app := application.New(application.Dependencies{Policies: repo, Resource: resource, Metrics: nopMetrics{}})
	ctx := context.Background()

	created, err := app.Commands.CreatePolicy.Handle(ctx, sampleCreate("asset soc"))
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if created.Enabled || created.Version != 1 || created.SOC.MinSOC != 20 {
		t.Fatalf("created = %+v", created)
	}
	if created.ID == "" {
		t.Fatal("id was not assigned")
	}

	if _, err := app.Commands.CreatePolicy.Handle(ctx, sampleCreate("asset soc")); !errors.Is(err, policy.ErrNameTaken) {
		t.Fatalf("duplicate name err = %v", err)
	}
	otherTenant := sampleCreate("asset soc")
	otherTenant.TenantID = "tenant-2"
	if _, err := app.Commands.CreatePolicy.Handle(ctx, otherTenant); err != nil {
		t.Fatalf("same name in another tenant: %v", err)
	}

	got, err := app.Queries.GetPolicy.Handle(ctx, query.GetPolicy{TenantID: created.TenantID, ID: created.ID})
	if err != nil || got.Name != "asset soc" {
		t.Fatalf("get = %+v err %v", got, err)
	}

	second, err := app.Commands.CreatePolicy.Handle(ctx, sampleCreate("other soc"))
	if err != nil {
		t.Fatal(err)
	}
	listed, err := app.Queries.ListPolicies.Handle(ctx, query.ListPolicies{TenantID: created.TenantID})
	if err != nil || len(listed) != 2 || listed[0].Name != "asset soc" || listed[1].Name != "other soc" {
		t.Fatalf("list = %v err %v", names(listed), err)
	}

	renamed, err := app.Commands.UpdatePolicy.Handle(ctx, updateOf(created, "renamed soc"))
	if err != nil {
		t.Fatal(err)
	}
	if renamed.Version != 2 || renamed.Name != "renamed soc" || renamed.Enabled {
		t.Fatalf("renamed = %+v", renamed)
	}
	if resource.calls != 0 {
		t.Fatal("updating a disabled policy must not resolve scope")
	}

	stale := updateOf(created, "stale")
	stale.Version = 1
	if _, err := app.Commands.UpdatePolicy.Handle(ctx, stale); !errors.Is(err, policy.ErrVersionConflict) {
		t.Fatalf("stale update err = %v", err)
	}
	still, _ := app.Queries.GetPolicy.Handle(ctx, query.GetPolicy{TenantID: created.TenantID, ID: created.ID})
	if still.Name != "renamed soc" || still.Version != 2 {
		t.Fatalf("stale update wrote %+v", still)
	}

	taken := updateOf(renamed, second.Name)
	if _, err := app.Commands.UpdatePolicy.Handle(ctx, taken); !errors.Is(err, policy.ErrNameTaken) {
		t.Fatalf("rename onto existing err = %v", err)
	}

	if _, err := app.Commands.EnablePolicy.Handle(ctx, enableOf(renamed, renamed.Version+1)); !errors.Is(err, policy.ErrVersionConflict) {
		t.Fatalf("stale enable err = %v", err)
	}
	if resource.calls != 0 {
		t.Fatal("version conflict must not resolve scope")
	}

	resource.scope = failedScope()
	if _, err := app.Commands.EnablePolicy.Handle(ctx, enableOf(renamed, renamed.Version)); err == nil {
		t.Fatal("failed precheck should not enable")
	} else {
		var precheck *policy.PrecheckError
		if !errors.As(err, &precheck) || len(precheck.Failures) != 1 || precheck.Failures[0].CUID != "cu-1" {
			t.Fatalf("precheck err = %v", err)
		}
		if len(precheck.Exclusions) != 1 || precheck.Exclusions[0].Reason != port.ExclusionNotActive {
			t.Fatalf("exclusions = %+v", precheck.Exclusions)
		}
	}
	disabled, _ := app.Queries.GetPolicy.Handle(ctx, query.GetPolicy{TenantID: renamed.TenantID, ID: renamed.ID})
	if disabled.Enabled || disabled.Version != renamed.Version {
		t.Fatalf("failed enable changed %+v", disabled)
	}
	if resource.query.TenantID != renamed.TenantID || resource.query.ScopeID != renamed.Scope.ID {
		t.Fatalf("scope query = %+v", resource.query)
	}
	if len(resource.query.RequiredCapabilityIDs) != 1 || resource.query.RequiredCapabilityIDs[0] != contracts.CapabilityEnergyStorage {
		t.Fatalf("capabilities = %v", resource.query.RequiredCapabilityIDs)
	}
	if resource.query.RequiredMetrics[0].Access != port.MetricAccessRead || resource.query.RequiredMetrics[1].Access != port.MetricAccessWrite {
		t.Fatalf("metrics = %+v", resource.query.RequiredMetrics)
	}

	resource.scope = readyScope()
	resource.scope.Exclusions = []port.ScopeExclusion{{
		CUID: "cu-pv", Reason: port.ExclusionMissingCapability, Detail: "not storage",
	}}
	enabled, err := app.Commands.EnablePolicy.Handle(ctx, enableOf(disabled, disabled.Version))
	if err != nil {
		t.Fatal(err)
	}
	if !enabled.Enabled || enabled.Version != disabled.Version+1 {
		t.Fatalf("enabled = %+v", enabled)
	}

	resource.scope = failedScope()
	if _, err := app.Commands.EnablePolicy.Handle(ctx, enableOf(enabled, enabled.Version)); err == nil {
		t.Fatal("re-enable should still precheck")
	}
	kept, _ := app.Queries.GetPolicy.Handle(ctx, query.GetPolicy{TenantID: enabled.TenantID, ID: enabled.ID})
	if !kept.Enabled || kept.Version != enabled.Version {
		t.Fatalf("failed re-enable changed %+v", kept)
	}

	resource.scope = failedScope()
	badUpdate := updateOf(kept, "should not stick")
	if _, err := app.Commands.UpdatePolicy.Handle(ctx, badUpdate); err == nil {
		t.Fatal("enabled update must fail closed when precheck fails")
	}
	afterBad, _ := app.Queries.GetPolicy.Handle(ctx, query.GetPolicy{TenantID: kept.TenantID, ID: kept.ID})
	if afterBad.Name != kept.Name || afterBad.Version != kept.Version {
		t.Fatalf("failed enabled update wrote %+v", afterBad)
	}

	resource.scope = readyScope()
	off, err := app.Commands.DisablePolicy.Handle(ctx, disableOf(afterBad))
	if err != nil || off.Enabled || off.Version != afterBad.Version+1 {
		t.Fatalf("disable = %+v err %v", off, err)
	}
	callsBefore := resource.calls
	if _, err := app.Commands.DisablePolicy.Handle(ctx, disableOf(off)); err != nil {
		t.Fatal(err)
	}
	if resource.calls != callsBefore {
		t.Fatal("disable must not resolve scope")
	}

	enabledOnly, err := app.Queries.ListPolicies.Handle(ctx, query.ListPolicies{TenantID: created.TenantID, EnabledOnly: true})
	if err != nil || len(enabledOnly) != 0 {
		t.Fatalf("enabled list = %v err %v", names(enabledOnly), err)
	}

	if _, err := app.Commands.DeletePolicy.Handle(ctx, command.DeletePolicy{TenantID: off.TenantID, ID: off.ID}); err != nil {
		t.Fatal(err)
	}
	if _, err := app.Queries.GetPolicy.Handle(ctx, query.GetPolicy{TenantID: off.TenantID, ID: off.ID}); !errors.Is(err, policy.ErrNotFound) {
		t.Fatalf("get deleted err = %v", err)
	}
	again := sampleCreate(off.Name)
	if _, err := app.Commands.CreatePolicy.Handle(ctx, again); err != nil {
		t.Fatalf("reuse deleted name: %v", err)
	}
}

type fakeResource struct {
	scope port.ResolvedScope
	query port.ScopeQuery
	calls int
}

func (f *fakeResource) ResolveScope(_ context.Context, query port.ScopeQuery) (port.ResolvedScope, error) {
	f.calls++
	f.query = query
	return f.scope, nil
}

func readyScope() port.ResolvedScope {
	return port.ResolvedScope{
		PrecheckOK: true,
		Members:    []port.ResolvedCU{{CUID: "cu-1", Lifecycle: port.LifecycleActive}},
	}
}

func failedScope() port.ResolvedScope {
	return port.ResolvedScope{
		PrecheckOK: false,
		PrecheckFailures: []port.ScopePrecheckFailure{{
			CUID:    "cu-1",
			AssetID: "asset-1",
			Reason:  port.PrecheckMissingMetricBinding,
			Detail:  "energy_storage.state_of_charge.v1 read",
		}},
		Exclusions: []port.ScopeExclusion{{
			CUID:   "cu-2",
			Reason: port.ExclusionNotActive,
			Detail: "lifecycle disabled",
		}},
	}
}

func sampleCreate(name string) command.CreatePolicy {
	return command.CreatePolicy{
		TenantID: "tenant-1",
		Name:     name,
		Kind:     policy.KindSOCThreshold,
		Scope:    policy.TargetScope{Type: port.ScopeAsset, ID: "asset-1"},
		Cooldown: 2 * time.Minute,
		SOC: &policy.SOCThresholdSpec{
			MinSOC: 20, MaxSOC: 90, ChargePowerKW: 100, DischargePowerKW: 80,
		},
	}
}

func updateOf(current *policy.Policy, name string) command.UpdatePolicy {
	return command.UpdatePolicy{
		TenantID: current.TenantID,
		ID:       current.ID,
		Version:  current.Version,
		Name:     name,
		Scope:    current.Scope,
		Cooldown: current.Cooldown,
		SOC:      current.SOC,
	}
}

func enableOf(current *policy.Policy, version int64) command.EnablePolicy {
	return command.EnablePolicy{TenantID: current.TenantID, ID: current.ID, Version: version}
}

func disableOf(current *policy.Policy) command.DisablePolicy {
	return command.DisablePolicy{TenantID: current.TenantID, ID: current.ID, Version: current.Version}
}

func names(items []*policy.Policy) []string {
	out := make([]string, len(items))
	for i, item := range items {
		out[i] = item.Name
	}
	return out
}

type nopMetrics struct{}

func (nopMetrics) Count(string, string, string)           {}
func (nopMetrics) CountN(string, string, string, float64) {}
func (nopMetrics) Observe(string, string, time.Duration)  {}
func (nopMetrics) TrackInFlight(string, string) func()    { return func() {} }

var _ decorator.MetricsClient = nopMetrics{}
