package redis

import (
	"context"
	"os"
	"testing"
)

func TestCURuntimeStore_SetAndGet(t *testing.T) {
	client, cleanup := newTestRedisClient(t)
	defer cleanup()

	store := NewCURuntimeCache(client, 0)
	flushSeedKeys(t, client)
	seedCURuntime(t, store)

	got, err := store.GetCURuntime(context.Background(), seedTenantID, seedCUID)
	if err != nil {
		t.Fatalf("GetCURuntime: %v", err)
	}
	if got == nil {
		t.Fatal("expected cu runtime, got nil")
	}
	if got.CUID != seedCUID || got.ConnStatus != "connected" {
		t.Fatalf("unexpected cu runtime: %+v", got)
	}
	if got.LatencyMS == nil || *got.LatencyMS != 35 {
		t.Fatalf("unexpected LatencyMS: %v", got.LatencyMS)
	}
}

func TestCURuntimeStore_ListCURuntimes(t *testing.T) {
	client, cleanup := newTestRedisClient(t)
	defer cleanup()

	store := NewCURuntimeCache(client, 0)
	flushSeedKeys(t, client)
	seedCURuntime(t, store)

	rows, err := store.ListCURuntimes(context.Background(), seedTenantID, []string{seedCUID, "cu-missing"})
	if err != nil {
		t.Fatalf("ListCURuntimes: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("expected 2 slots, got %d", len(rows))
	}
	if rows[0] == nil || rows[0].CUID != seedCUID {
		t.Fatal("expected first slot to contain seeded cu runtime")
	}
	if rows[1] != nil {
		t.Fatal("expected missing cu slot to be nil")
	}
}

func TestSeedCURuntimeToRedis(t *testing.T) {
	if os.Getenv("SEED_REDIS") != "1" {
		t.Skip("set SEED_REDIS=1 (and optionally REDIS_ADDR=127.0.0.1:6379) to seed local Redis")
	}

	client, cleanup := newTestRedisClient(t)
	defer cleanup()

	store := NewCURuntimeCache(client, 0)
	seedCURuntime(t, store)

	key := cuRuntimeKey(seedTenantID, seedCUID)
	assertRedisKeyExists(t, client, key)
	t.Logf("seeded cu runtime: %s", key)
}
