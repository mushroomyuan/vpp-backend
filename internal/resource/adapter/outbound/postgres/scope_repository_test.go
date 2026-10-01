package postgres

import (
	"testing"

	"github.com/mushroomyuan/vpp-backend/api/contracts"
	"github.com/mushroomyuan/vpp-backend/resource/domain"
	"github.com/mushroomyuan/vpp-backend/resource/domain/model"
)

func TestSnapshotFromPayloadOmitsExternalAddress(t *testing.T) {
	t.Parallel()

	raw := []byte(`{
		"found": true,
		"root_id": "11111111-1111-1111-1111-111111111111",
		"root_type": "site",
		"nodes": [
			{"entity_type":"site","entity_id":"11111111-1111-1111-1111-111111111111","version":"2"},
			{"entity_type":"cu","entity_id":"22222222-2222-2222-2222-222222222222","version":"3"}
		],
		"cus": [{
			"cu_id": "22222222-2222-2222-2222-222222222222",
			"asset_id": "33333333-3333-3333-3333-333333333333",
			"lifecycle_status": "active",
			"node_version": "3",
			"capabilities": [{
				"capability_id": "energy.storage.v1",
				"schema_version": 1,
				"spec": {"usable_energy_kwh": 10, "max_charge_power_kw": 2, "max_discharge_power_kw": 2},
				"enabled": true,
				"version": "4"
			}],
			"bindings": [{
				"metric_id": "energy_storage.state_of_charge.v1",
				"access_mode": "read",
				"enabled": true,
				"revision": "5",
				"min_value": 0,
				"max_value": 100,
				"max_change_per_second": null,
				"safety_version": "1"
			}]
		}]
	}`)

	snapshot, err := snapshotFromPayload("tenant-1", raw)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.ScopeID != "11111111-1111-1111-1111-111111111111" || snapshot.RootType != model.ScopeTypeSite {
		t.Fatalf("snapshot identity = %+v", snapshot)
	}
	if len(snapshot.CUs) != 1 || snapshot.CUs[0].AssetID != "33333333-3333-3333-3333-333333333333" {
		t.Fatalf("cus = %+v", snapshot.CUs)
	}
	binding := snapshot.CUs[0].Bindings[0]
	if binding.MetricID != contracts.MetricEnergyStorageStateOfCharge || binding.Revision != 5 {
		t.Fatalf("binding = %+v", binding)
	}
	if binding.Safety == nil || binding.Safety.MaxValue == nil || *binding.Safety.MaxValue != 100 {
		t.Fatalf("safety = %+v", binding.Safety)
	}
	if err := contracts.ValidateCapabilitySpec(
		contracts.CapabilityEnergyStorage,
		snapshot.CUs[0].Capabilities[0].SchemaVersion,
		snapshot.CUs[0].Capabilities[0].Spec,
	); err != nil {
		t.Fatal(err)
	}
}

func TestSnapshotFromPayloadNotFound(t *testing.T) {
	t.Parallel()

	_, err := snapshotFromPayload("tenant-1", []byte(`{"found":false,"nodes":[],"cus":[]}`))
	if err != domain.ErrScopeNotFound {
		t.Fatalf("error = %v", err)
	}
}
