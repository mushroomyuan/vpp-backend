package memory

import (
	"context"
	"testing"
	"time"

	"github.com/mushroomyuan/vpp-backend/decision/domain/policy"
)

func TestCooldownStore_DirectionIsIndependent(t *testing.T) {
	store := NewCooldownStore()
	now := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	charge := policy.CooldownKey{TenantID: "tenant-1", PolicyID: "policy-1", Direction: policy.DirectionCharge}
	discharge := policy.CooldownKey{TenantID: "tenant-1", PolicyID: "policy-1", Direction: policy.DirectionDischarge}

	cooling, err := store.CoolingDown(context.Background(), charge, now)
	if err != nil || cooling {
		t.Fatalf("before mark cooling=%v err=%v", cooling, err)
	}
	if err := store.MarkTriggered(context.Background(), charge, now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	cooling, err = store.CoolingDown(context.Background(), charge, now.Add(30*time.Second))
	if err != nil || !cooling {
		t.Fatalf("charge cooling=%v err=%v", cooling, err)
	}
	cooling, err = store.CoolingDown(context.Background(), discharge, now.Add(30*time.Second))
	if err != nil || cooling {
		t.Fatalf("discharge cooling=%v err=%v", cooling, err)
	}
	cooling, err = store.CoolingDown(context.Background(), charge, now.Add(time.Minute))
	if err != nil || cooling {
		t.Fatalf("at exclusive end cooling=%v err=%v", cooling, err)
	}
}
