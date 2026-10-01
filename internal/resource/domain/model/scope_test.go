package model

import (
	"testing"

	"github.com/mushroomyuan/vpp-backend/api/contracts"
)

const storageSpec = `{"usable_energy_kwh":100,"max_charge_power_kw":50,"max_discharge_power_kw":40}`

func TestResolveSnapshotSelectsStorageAndExcludesOtherDevices(t *testing.T) {
	t.Parallel()

	snapshot := siteSnapshot(
		storageCU("cu-storage", "asset-1", NodeLifecycleActive, storageSpec, true, true, true),
		generationCU("cu-pv", "asset-1"),
	)
	caps, metrics := storageRequirements()
	resolved, err := ResolveSnapshot(snapshot, ScopeTypeSite, caps, metrics)
	if err != nil {
		t.Fatal(err)
	}
	if !resolved.PrecheckOK {
		t.Fatalf("precheck failed: %+v", resolved.PrecheckFailures)
	}
	if len(resolved.Members) != 1 || resolved.Members[0].CUID != "cu-storage" {
		t.Fatalf("members = %+v", resolved.Members)
	}
	if resolved.Members[0].AssetID != "asset-1" {
		t.Fatalf("asset = %s", resolved.Members[0].AssetID)
	}
	if len(resolved.Exclusions) != 1 ||
		resolved.Exclusions[0].CUID != "cu-pv" ||
		resolved.Exclusions[0].Reason != ExclusionMissingCapability {
		t.Fatalf("exclusions = %+v", resolved.Exclusions)
	}
	if len(resolved.ResourceRevision) != 64 {
		t.Fatalf("revision = %q", resolved.ResourceRevision)
	}
}

func TestResolveSnapshotPrecheckFailsClosed(t *testing.T) {
	t.Parallel()

	caps, metrics := storageRequirements()
	tests := []struct {
		name   string
		bad    ScopeCU
		reason PrecheckFailureReason
	}{
		{
			name:   "missing soc read binding",
			bad:    storageCU("cu-bad", "asset-1", NodeLifecycleActive, storageSpec, true, false, true),
			reason: PrecheckMissingMetricBinding,
		},
		{
			name:   "missing setpoint write binding",
			bad:    storageCU("cu-bad", "asset-1", NodeLifecycleActive, storageSpec, true, true, false),
			reason: PrecheckMissingMetricBinding,
		},
		{
			name:   "disabled setpoint binding",
			bad:    storageCU("cu-bad", "asset-1", NodeLifecycleActive, storageSpec, true, true, true, withBindingEnabled(contracts.MetricElectricalActivePowerSetpoint, false)),
			reason: PrecheckMissingMetricBinding,
		},
		{
			name:   "setpoint binding is not writable",
			bad:    storageCU("cu-bad", "asset-1", NodeLifecycleActive, storageSpec, true, true, true, withAccessMode(contracts.MetricElectricalActivePowerSetpoint, AccessModeRead)),
			reason: PrecheckMissingMetricBinding,
		},
		{
			name:   "invalid storage spec",
			bad:    storageCU("cu-bad", "asset-1", NodeLifecycleActive, `{"usable_energy_kwh":100,"unexpected":1}`, true, true, true),
			reason: PrecheckInvalidCapabilitySpec,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			snapshot := siteSnapshot(
				storageCU("cu-good", "asset-1", NodeLifecycleActive, storageSpec, true, true, true),
				tt.bad,
				generationCU("cu-pv", "asset-2"),
			)
			resolved, err := ResolveSnapshot(snapshot, ScopeTypeSite, caps, metrics)
			if err != nil {
				t.Fatal(err)
			}
			if resolved.PrecheckOK {
				t.Fatal("precheck succeeded")
			}
			if len(resolved.Members) != 0 {
				t.Fatalf("failed precheck returned members: %+v", resolved.Members)
			}
			if len(resolved.PrecheckFailures) == 0 || resolved.PrecheckFailures[0].CUID != "cu-bad" || resolved.PrecheckFailures[0].Reason != tt.reason {
				t.Fatalf("failures = %+v", resolved.PrecheckFailures)
			}
			if len(resolved.Exclusions) != 1 || resolved.Exclusions[0].CUID != "cu-pv" {
				t.Fatalf("exclusions = %+v", resolved.Exclusions)
			}
		})
	}
}

func TestResolveSnapshotInactiveCUDoesNotFailPrecheck(t *testing.T) {
	t.Parallel()

	snapshot := siteSnapshot(
		storageCU("cu-off", "asset-1", NodeLifecycleDisabled, storageSpec, true, false, false),
		storageCU("cu-on", "asset-1", NodeLifecycleActive, storageSpec, true, true, true),
	)
	caps, metrics := storageRequirements()
	resolved, err := ResolveSnapshot(snapshot, ScopeTypeSite, caps, metrics)
	if err != nil {
		t.Fatal(err)
	}
	if !resolved.PrecheckOK || len(resolved.Members) != 1 || resolved.Members[0].CUID != "cu-on" {
		t.Fatalf("resolved = %+v", resolved)
	}
	if len(resolved.Exclusions) != 1 || resolved.Exclusions[0].Reason != ExclusionNotActive {
		t.Fatalf("exclusions = %+v", resolved.Exclusions)
	}
}

func TestResolveSnapshotDisabledCapabilityIsExclusion(t *testing.T) {
	t.Parallel()

	snapshot := siteSnapshot(
		storageCU("cu-disabled-cap", "asset-1", NodeLifecycleActive, storageSpec, false, false, false),
	)
	snapshot.RootType = ScopeTypeAsset
	snapshot.ScopeID = "asset-1"
	caps, metrics := storageRequirements()
	resolved, err := ResolveSnapshot(snapshot, ScopeTypeAsset, caps, metrics)
	if err != nil {
		t.Fatal(err)
	}
	if !resolved.PrecheckOK || len(resolved.Members) != 0 {
		t.Fatalf("resolved = %+v", resolved)
	}
	if len(resolved.Exclusions) != 1 || resolved.Exclusions[0].Reason != ExclusionCapabilityDisabled {
		t.Fatalf("exclusions = %+v", resolved.Exclusions)
	}
}

func TestResolveSnapshotRevisionIgnoresInputOrder(t *testing.T) {
	t.Parallel()

	first := siteSnapshot(
		storageCU("cu-b", "asset-2", NodeLifecycleActive, storageSpec, true, true, true),
		storageCU("cu-a", "asset-1", NodeLifecycleActive, storageSpec, true, true, true),
	)
	second := siteSnapshot(
		storageCU("cu-a", "asset-1", NodeLifecycleActive, storageSpec, true, true, true),
		storageCU("cu-b", "asset-2", NodeLifecycleActive, storageSpec, true, true, true),
	)
	for i, j := 0, len(second.Nodes)-1; i < j; i, j = i+1, j-1 {
		second.Nodes[i], second.Nodes[j] = second.Nodes[j], second.Nodes[i]
	}
	second.CUs[0].Capabilities = []ScopeCapability{
		second.CUs[0].Capabilities[0],
	}
	second.CUs[0].Bindings = []ScopeBinding{
		second.CUs[0].Bindings[1],
		second.CUs[0].Bindings[0],
	}

	caps, metrics := storageRequirements()
	left, err := ResolveSnapshot(first, ScopeTypeSite, caps, metrics)
	if err != nil {
		t.Fatal(err)
	}
	right, err := ResolveSnapshot(second, ScopeTypeSite, caps, metrics)
	if err != nil {
		t.Fatal(err)
	}
	if left.ResourceRevision != right.ResourceRevision {
		t.Fatalf("revision changed with input order: %s vs %s", left.ResourceRevision, right.ResourceRevision)
	}
	if left.Members[0].CUID != "cu-a" || left.Members[1].CUID != "cu-b" {
		t.Fatalf("members not sorted: %+v", left.Members)
	}
	if left.Members[0].Bindings[0].MetricID != contracts.MetricElectricalActivePowerSetpoint {
		t.Fatalf("bindings not sorted: %+v", left.Members[0].Bindings)
	}
}

func TestResolveSnapshotReadWriteBindingSatisfiesWrite(t *testing.T) {
	t.Parallel()

	cu := storageCU("cu-1", "asset-1", NodeLifecycleActive, storageSpec, true, true, true)
	for i := range cu.Bindings {
		if cu.Bindings[i].MetricID == contracts.MetricElectricalActivePowerSetpoint {
			cu.Bindings[i].AccessMode = AccessModeReadWrite
		}
	}
	caps, metrics := storageRequirements()
	resolved, err := ResolveSnapshot(siteSnapshot(cu), ScopeTypeSite, caps, metrics)
	if err != nil {
		t.Fatal(err)
	}
	if !resolved.PrecheckOK || len(resolved.Members) != 1 {
		t.Fatalf("resolved = %+v", resolved)
	}
}

func TestResolveSnapshotRejectsTypeMismatch(t *testing.T) {
	t.Parallel()

	snapshot := siteSnapshot(storageCU("cu-1", "asset-1", NodeLifecycleActive, storageSpec, true, true, true))
	_, err := ResolveSnapshot(snapshot, ScopeTypeCU, nil, nil)
	if err == nil {
		t.Fatal("expected scope type mismatch")
	}
}

type bindingOption func(*ScopeCU)

func withBindingEnabled(metricID contracts.MetricID, enabled bool) bindingOption {
	return func(cu *ScopeCU) {
		for i := range cu.Bindings {
			if cu.Bindings[i].MetricID == metricID {
				cu.Bindings[i].Enabled = enabled
			}
		}
	}
}

func withAccessMode(metricID contracts.MetricID, mode AccessMode) bindingOption {
	return func(cu *ScopeCU) {
		for i := range cu.Bindings {
			if cu.Bindings[i].MetricID == metricID {
				cu.Bindings[i].AccessMode = mode
			}
		}
	}
}

func storageRequirements() ([]contracts.CapabilityID, []MetricRequirement) {
	return []contracts.CapabilityID{contracts.CapabilityEnergyStorage}, []MetricRequirement{
		{MetricID: contracts.MetricEnergyStorageStateOfCharge, Access: MetricAccessNeedRead},
		{MetricID: contracts.MetricElectricalActivePowerSetpoint, Access: MetricAccessNeedWrite},
	}
}

func siteSnapshot(cus ...ScopeCU) ScopeSnapshot {
	nodes := []contracts.RevisionNode{
		{EntityType: contracts.RevisionEntitySite, EntityID: "site-1", Version: 1},
		{EntityType: contracts.RevisionEntityAsset, EntityID: "asset-1", Version: 2},
	}
	seenAssets := map[string]struct{}{"asset-1": {}}
	for _, cu := range cus {
		nodes = append(nodes, contracts.RevisionNode{
			EntityType: contracts.RevisionEntityCU,
			EntityID:   cu.CUID,
			Version:    cu.NodeVersion,
		})
		if cu.AssetID != "" {
			if _, ok := seenAssets[cu.AssetID]; !ok {
				seenAssets[cu.AssetID] = struct{}{}
				nodes = append(nodes, contracts.RevisionNode{
					EntityType: contracts.RevisionEntityAsset,
					EntityID:   cu.AssetID,
					Version:    2,
				})
			}
		}
	}
	return ScopeSnapshot{
		TenantID: "tenant-1",
		ScopeID:  "site-1",
		RootType: ScopeTypeSite,
		Nodes:    nodes,
		CUs:      cus,
	}
}

func storageCU(
	id, assetID string,
	lifecycle NodeLifecycleStatus,
	spec string,
	capabilityEnabled, socRead, setpointWrite bool,
	opts ...bindingOption,
) ScopeCU {
	minValue := 0.0
	maxValue := 100.0
	cu := ScopeCU{
		CUID:        id,
		AssetID:     assetID,
		Lifecycle:   lifecycle,
		NodeVersion: 3,
		Capabilities: []ScopeCapability{{
			CapabilityID:  string(contracts.CapabilityEnergyStorage),
			SchemaVersion: 1,
			Spec:          []byte(spec),
			Enabled:       capabilityEnabled,
			Version:       4,
		}},
	}
	if socRead {
		cu.Bindings = append(cu.Bindings, ScopeBinding{
			MetricID:   contracts.MetricEnergyStorageStateOfCharge,
			AccessMode: AccessModeRead,
			Enabled:    true,
			Revision:   5,
			Safety:     &PointSafetyConstraint{MinValue: &minValue, MaxValue: &maxValue, Version: 1},
		})
	}
	if setpointWrite {
		cu.Bindings = append(cu.Bindings, ScopeBinding{
			MetricID:   contracts.MetricElectricalActivePowerSetpoint,
			AccessMode: AccessModeWrite,
			Enabled:    true,
			Revision:   6,
		})
	}
	for _, opt := range opts {
		opt(&cu)
	}
	return cu
}

func generationCU(id, assetID string) ScopeCU {
	return ScopeCU{
		CUID:        id,
		AssetID:     assetID,
		Lifecycle:   NodeLifecycleActive,
		NodeVersion: 7,
		Capabilities: []ScopeCapability{{
			CapabilityID:  string(contracts.CapabilityPowerGeneration),
			SchemaVersion: 1,
			Spec:          []byte(`{"rated_power_kw":30,"min_power_kw":0,"max_ramp_kw_per_minute":5}`),
			Enabled:       true,
			Version:       1,
		}},
		Bindings: []ScopeBinding{{
			MetricID:   contracts.MetricElectricalActivePower,
			AccessMode: AccessModeRead,
			Enabled:    true,
			Revision:   1,
		}},
	}
}
