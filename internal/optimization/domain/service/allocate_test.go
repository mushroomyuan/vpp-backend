package service

import (
	"context"
	"errors"
	"testing"

	"github.com/mushroomyuan/vpp-backend/optimization/domain/model"
	"github.com/mushroomyuan/vpp-backend/optimization/domain/port"
)

// fakeResource is an in-memory port.ResourcePort for unit tests.
type fakeResource struct {
	capacities map[string]float64
	err        error
}

func (f *fakeResource) GetCapacityKW(_ context.Context, _ string, scope []string) (map[string]float64, error) {
	if f.err != nil {
		return nil, f.err
	}
	out := make(map[string]float64)
	for _, id := range scope {
		if kw, ok := f.capacities[id]; ok {
			out[id] = kw
		}
	}
	return out, nil
}

func TestAllocate_PointTarget_MapsOneToOne(t *testing.T) {
	target := model.PointTarget{
		Tenant:   "tenant-1",
		Src:      model.SourceInternalRule,
		CUCode:   "cu-1",
		PointKey: "active_power_setpoint_kw",
		Value:    model.FloatCommandValue(42),
	}

	specs, err := callAllocate(t, &fakeResource{}, target)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(specs) != 1 {
		t.Fatalf("expected 1 spec, got %d", len(specs))
	}
	if specs[0].CUCode != "cu-1" || specs[0].PointKey != "active_power_setpoint_kw" {
		t.Errorf("unexpected spec: %+v", specs[0])
	}
	if specs[0].Value.FloatValue == nil || *specs[0].Value.FloatValue != 42 {
		t.Errorf("expected value 42, got %+v", specs[0].Value)
	}
}

func TestAllocate_AggregateTarget_SplitsProportionally(t *testing.T) {
	target := model.AggregateTarget{
		Tenant:     "tenant-1",
		Src:        model.SourceExternalDR,
		Scope:      []string{"cu-a", "cu-b"},
		Metric:     "active_power_kw",
		DeltaValue: -300, // e.g. reduce load by 300kW total
	}
	res := &fakeResource{capacities: map[string]float64{"cu-a": 100, "cu-b": 300}} // 1:3 ratio

	specs, err := callAllocate(t, res, target)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(specs) != 2 {
		t.Fatalf("expected 2 specs, got %d", len(specs))
	}

	byCU := map[string]float64{}
	for _, s := range specs {
		if s.Value.FloatValue == nil {
			t.Fatalf("expected float value, got %+v", s.Value)
		}
		byCU[s.CUCode] = *s.Value.FloatValue
		if s.PointKey != "active_power_kw" {
			t.Errorf("expected PointKey to pass through Metric, got %q", s.PointKey)
		}
	}
	if got, want := byCU["cu-a"], -75.0; got != want {
		t.Errorf("cu-a share = %v, want %v", got, want)
	}
	if got, want := byCU["cu-b"], -225.0; got != want {
		t.Errorf("cu-b share = %v, want %v", got, want)
	}
}

func TestAllocate_AggregateTarget_ExcludesUnknownCapacity(t *testing.T) {
	target := model.AggregateTarget{
		Tenant:     "tenant-1",
		Src:        model.SourceExternalDR,
		Scope:      []string{"cu-a", "cu-unknown"},
		Metric:     "active_power_kw",
		DeltaValue: -100,
	}
	res := &fakeResource{capacities: map[string]float64{"cu-a": 100}} // cu-unknown has no entry

	specs, err := callAllocate(t, res, target)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(specs) != 1 {
		t.Fatalf("expected only cu-a to receive a command, got %d specs: %+v", len(specs), specs)
	}
	if specs[0].CUCode != "cu-a" {
		t.Errorf("expected cu-a, got %q", specs[0].CUCode)
	}
	if *specs[0].Value.FloatValue != -100 {
		t.Errorf("expected the full delta to land on the only known-capacity CU, got %v", *specs[0].Value.FloatValue)
	}
}

func TestAllocate_AggregateTarget_EmptyScopeErrors(t *testing.T) {
	target := model.AggregateTarget{Tenant: "tenant-1", Src: model.SourceExternalDR}
	_, err := callAllocate(t, &fakeResource{}, target)
	if err == nil {
		t.Fatal("expected an error for an empty scope")
	}
}

func TestAllocate_AggregateTarget_ZeroTotalCapacityErrors(t *testing.T) {
	target := model.AggregateTarget{
		Tenant: "tenant-1", Src: model.SourceExternalDR,
		Scope: []string{"cu-a"}, Metric: "active_power_kw", DeltaValue: -100,
	}
	res := &fakeResource{capacities: map[string]float64{"cu-a": 0}}
	_, err := callAllocate(t, res, target)
	if err == nil {
		t.Fatal("expected an error when total capacity is zero")
	}
}

func TestAllocate_AggregateTarget_PropagatesResourcePortError(t *testing.T) {
	target := model.AggregateTarget{
		Tenant: "tenant-1", Src: model.SourceExternalDR,
		Scope: []string{"cu-a"}, Metric: "active_power_kw", DeltaValue: -100,
	}
	res := &fakeResource{err: errors.New("resource unavailable")}
	_, err := callAllocate(t, res, target)
	if err == nil {
		t.Fatal("expected the ResourcePort error to propagate")
	}
}

func TestAllocate_UnsupportedTargetType(t *testing.T) {
	_, err := Allocate(context.Background(), &fakeResource{}, unsupportedTarget{})
	if err == nil {
		t.Fatal("expected an error for an unsupported Target implementation")
	}
}

// unsupportedTarget exists only to exercise Allocate's default case —
// see model.Target's doc comment on the lack of compile-time exhaustiveness.
type unsupportedTarget struct{}

func (unsupportedTarget) TenantID() string { return "t" }
func (unsupportedTarget) Source() string   { return "unsupported" }

// callAllocate is a thin wrapper so table-style tests above read a
// little cleaner; it is not exported and has no behavior of its own.
func callAllocate(t *testing.T, res port.ResourcePort, target model.Target) ([]model.CommandSpec, error) {
	t.Helper()
	return Allocate(context.Background(), res, target)
}
