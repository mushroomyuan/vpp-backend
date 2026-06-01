package redisruntime

import (
	"context"
	"os"
	"testing"
)

func TestAssetRuntimeStore_SetAndGet(t *testing.T) {
	client, cleanup := newTestRedisClient(t)
	defer cleanup()

	store := NewAssetRuntimeCache(client, 0)
	flushSeedKeys(t, client)
	seedAssetRuntime(t, store)

	got, err := store.GetAssetRuntime(context.Background(), seedTenantID, seedAssetID)
	if err != nil {
		t.Fatalf("GetAssetRuntime: %v", err)
	}
	if got == nil {
		t.Fatal("expected asset runtime, got nil")
	}
	if got.AssetID != seedAssetID || got.TenantID != seedTenantID {
		t.Fatalf("unexpected ids: tenant=%q asset=%q", got.TenantID, got.AssetID)
	}
	if !got.Online || !got.Dispatchable {
		t.Fatal("expected online and dispatchable")
	}
	if got.CurrentPowerKW == nil || *got.CurrentPowerKW != 120.5 {
		t.Fatalf("unexpected CurrentPowerKW: %v", got.CurrentPowerKW)
	}
}

func TestAssetRuntimeStore_ListAssetRuntimes(t *testing.T) {
	client, cleanup := newTestRedisClient(t)
	defer cleanup()

	store := NewAssetRuntimeCache(client, 0)
	flushSeedKeys(t, client)
	seedAssetRuntime(t, store)

	rows, err := store.ListAssetRuntimes(context.Background(), seedTenantID, []string{seedAssetID, "asset-missing"})
	if err != nil {
		t.Fatalf("ListAssetRuntimes: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("expected 2 slots, got %d", len(rows))
	}
	if rows[0] == nil || rows[0].AssetID != seedAssetID {
		t.Fatal("expected first slot to contain seeded asset runtime")
	}
	if rows[1] != nil {
		t.Fatal("expected missing asset slot to be nil")
	}
}

func TestSeedAssetRuntimeToRedis(t *testing.T) {
	if os.Getenv("SEED_REDIS") != "1" {
		t.Skip("set SEED_REDIS=1 (and optionally REDIS_ADDR=127.0.0.1:6379) to seed local Redis")
	}

	client, cleanup := newTestRedisClient(t)
	defer cleanup()

	store := NewAssetRuntimeCache(client, 0)
	seedAssetRuntime(t, store)

	key := assetRuntimeKey(seedTenantID, seedAssetID)
	assertRedisKeyExists(t, client, key)
	t.Logf("seeded asset runtime: %s", key)
}
