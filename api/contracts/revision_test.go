package contracts

import (
	"testing"
)

func TestResourceRevisionOrderIndependent(t *testing.T) {
	t.Parallel()

	scope := RevisionScope{TenantID: "tenant-1", ScopeType: RevisionEntitySite, ScopeID: "site-1"}
	nodes := []RevisionNode{
		{EntityType: RevisionEntityCU, EntityID: "cu-b", Version: 3},
		{EntityType: RevisionEntitySite, EntityID: "site-1", Version: 1},
		{EntityType: RevisionEntityAsset, EntityID: "asset-2", Version: 4},
		{EntityType: RevisionEntityAsset, EntityID: "asset-1", Version: 2},
		{EntityType: RevisionEntityCU, EntityID: "cu-a", Version: 8},
	}
	capabilities := []RevisionCapability{
		{CUID: "cu-b", CapabilityID: string(CapabilityPowerGeneration), Version: 2},
		{CUID: "cu-a", CapabilityID: string(CapabilityEnergyStorage), Version: 5},
		{CUID: "cu-a", CapabilityID: string(CapabilityPowerConsumption), Version: 1},
	}
	bindings := []RevisionBinding{
		{CUID: "cu-b", MetricID: string(MetricElectricalActivePower), Version: 9},
		{CUID: "cu-a", MetricID: string(MetricElectricalActivePowerSetpoint), Version: 4},
		{CUID: "cu-a", MetricID: string(MetricEnergyStorageStateOfCharge), Version: 7},
	}

	forward, err := ResourceRevision(scope, nodes, capabilities, bindings)
	if err != nil {
		t.Fatal(err)
	}
	reversedNodes := reverseNodes(nodes)
	reversedCaps := reverseCapabilities(capabilities)
	reversedBindings := reverseBindings(bindings)
	backward, err := ResourceRevision(scope, reversedNodes, reversedCaps, reversedBindings)
	if err != nil {
		t.Fatal(err)
	}
	if forward != backward {
		t.Fatalf("order changed revision: %s vs %s", forward, backward)
	}
	if len(forward) != 64 {
		t.Fatalf("revision = %q, want 64 lowercase hex chars", forward)
	}

	// Lock the encoding. A change here is a resource_revision compatibility break.
	const golden = "e57371c21b3e1aa0800abc1c16835a0a4ae60fd14490ad730fac790cbe121423"
	if forward != golden {
		t.Fatalf("revision = %s, want %s", forward, golden)
	}
}

func TestResourceRevisionDetectsVersionChanges(t *testing.T) {
	t.Parallel()

	scope := RevisionScope{TenantID: "tenant-1", ScopeType: RevisionEntityAsset, ScopeID: "asset-1"}
	baseNodes := []RevisionNode{
		{EntityType: RevisionEntityAsset, EntityID: "asset-1", Version: 1},
		{EntityType: RevisionEntityCU, EntityID: "cu-1", Version: 2},
		{EntityType: RevisionEntityCU, EntityID: "cu-2", Version: 3},
	}
	base, err := ResourceRevision(scope, baseNodes, nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	bumped := append([]RevisionNode(nil), baseNodes...)
	bumped[2].Version = 4
	changed, err := ResourceRevision(scope, bumped, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if changed == base {
		t.Fatal("node version change did not change revision")
	}

	// MAX(version) would treat a single node at version 4 as the same snapshot
	// as nodes at versions 2 and 3. Those snapshots must differ.
	maxOnly, err := ResourceRevision(scope, []RevisionNode{
		{EntityType: RevisionEntityAsset, EntityID: "asset-1", Version: 4},
	}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if maxOnly == changed {
		t.Fatal("revision collapsed to MAX(version)")
	}

	otherScope := scope
	otherScope.ScopeID = "asset-2"
	moved, err := ResourceRevision(otherScope, baseNodes, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if moved == base {
		t.Fatal("scope identity change did not change revision")
	}
}

func TestResourceRevisionRejectsUnstableIdentity(t *testing.T) {
	t.Parallel()

	_, err := ResourceRevision(RevisionScope{}, nil, nil, nil)
	if err == nil {
		t.Fatal("expected missing scope identity to fail")
	}
	_, err = ResourceRevision(RevisionScope{
		TenantID: "t", ScopeType: "portfolio", ScopeID: "p",
	}, nil, nil, nil)
	if err == nil {
		t.Fatal("expected unknown scope type to fail")
	}
	_, err = ResourceRevision(RevisionScope{
		TenantID: "t", ScopeType: RevisionEntityCU, ScopeID: "cu-1",
	}, []RevisionNode{{EntityType: "point", EntityID: "p", Version: 1}}, nil, nil)
	if err == nil {
		t.Fatal("expected point entity type to fail")
	}
}

func reverseNodes(in []RevisionNode) []RevisionNode {
	out := append([]RevisionNode(nil), in...)
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out
}

func reverseCapabilities(in []RevisionCapability) []RevisionCapability {
	out := append([]RevisionCapability(nil), in...)
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out
}

func reverseBindings(in []RevisionBinding) []RevisionBinding {
	out := append([]RevisionBinding(nil), in...)
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out
}
