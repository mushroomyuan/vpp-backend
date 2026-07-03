package redis

import (
	"context"
	"os"
	"testing"

	"github.com/mushroomyuan/vpp-backend/resource/domain/model"
)

func TestPointRuntimeStore_SetAndGet(t *testing.T) {
	client, cleanup := newTestRedisClient(t)
	defer cleanup()

	store := NewPointRuntimeCache(client, 0)
	flushSeedKeys(t, client)
	seedPointRuntime(t, store)

	got, err := store.GetPointRuntime(context.Background(), seedTenantID, seedPointID)
	if err != nil {
		t.Fatalf("GetPointRuntime: %v", err)
	}
	if got == nil {
		t.Fatal("expected point runtime, got nil")
	}
	if got.PointID != seedPointID || got.Sequence != 1001 {
		t.Fatalf("unexpected point runtime: %+v", got)
	}
	if got.NumericValue == nil || *got.NumericValue != 42.5 {
		t.Fatalf("unexpected NumericValue: %v", got.NumericValue)
	}
}

func TestPointRuntimeStore_MGetPointRuntimes(t *testing.T) {
	client, cleanup := newTestRedisClient(t)
	defer cleanup()

	store := NewPointRuntimeCache(client, 0)
	flushSeedKeys(t, client)
	seedPointRuntime(t, store)

	byID, err := store.MGetPointRuntimes(context.Background(), seedTenantID, []string{seedPointID, "point-missing"})
	if err != nil {
		t.Fatalf("MGetPointRuntimes: %v", err)
	}
	if len(byID) != 1 {
		t.Fatalf("expected 1 hit, got %d", len(byID))
	}
	got := byID[seedPointID]
	if got == nil || got.PointID != seedPointID {
		t.Fatal("expected seeded point in map")
	}
}

func TestPointRuntimeStore_MSetPointRuntimes(t *testing.T) {
	client, cleanup := newTestRedisClient(t)
	defer cleanup()

	store := NewPointRuntimeCache(client, 0)
	flushSeedKeys(t, client)

	seedPointRuntime(t, store)

	extraID := "point-demo-002"
	value := "1"
	quality := "good"
	err := store.MSetPointRuntimes(context.Background(), []*model.PointRuntime{
		{
			TenantID:      seedTenantID,
			PointID:       extraID,
			Value:         &value,
			NumericValue:  float64Ptr(1),
			QualityStatus: &quality,
			Sequence:      2,
		},
	})
	if err != nil {
		t.Fatalf("MSetPointRuntimes: %v", err)
	}

	byID, err := store.MGetPointRuntimes(context.Background(), seedTenantID, []string{seedPointID, extraID})
	if err != nil {
		t.Fatalf("MGetPointRuntimes: %v", err)
	}
	if len(byID) != 2 {
		t.Fatalf("expected 2 hits, got %d", len(byID))
	}
}

func TestSeedPointRuntimeToRedis(t *testing.T) {
	if os.Getenv("SEED_REDIS") != "1" {
		t.Skip("set SEED_REDIS=1 (and optionally REDIS_ADDR=127.0.0.1:6379) to seed local Redis")
	}

	client, cleanup := newTestRedisClient(t)
	defer cleanup()

	store := NewPointRuntimeCache(client, 0)
	seedPointRuntime(t, store)

	key := pointRuntimeKey(seedTenantID, seedPointID)
	assertRedisKeyExists(t, client, key)
	t.Logf("seeded point runtime: %s", key)
}

func TestSeedAllRuntimeFixturesToRedis(t *testing.T) {
	if os.Getenv("SEED_REDIS") != "1" {
		t.Skip("set SEED_REDIS=1 (and optionally REDIS_ADDR=127.0.0.1:6379) to seed local Redis")
	}

	client, cleanup := newTestRedisClient(t)
	defer cleanup()

	seedAssetRuntime(t, NewAssetRuntimeCache(client, 0))
	seedCURuntime(t, NewCURuntimeCache(client, 0))
	seedPointRuntime(t, NewPointRuntimeCache(client, 0))

	assertRedisKeyExists(t, client, assetRuntimeKey(seedTenantID, seedAssetID))
	assertRedisKeyExists(t, client, cuRuntimeKey(seedTenantID, seedCUID))
	assertRedisKeyExists(t, client, pointRuntimeKey(seedTenantID, seedPointID))

	t.Logf("seeded runtime keys for tenant=%s asset=%s cu=%s point=%s",
		seedTenantID, seedAssetID, seedCUID, seedPointID)
}
