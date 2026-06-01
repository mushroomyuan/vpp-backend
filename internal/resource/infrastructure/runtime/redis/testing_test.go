package redisruntime

import (
	"context"
	"os"
	"testing"
	"time"

	miniredis "github.com/alicebob/miniredis/v2"

	platformredis "github.com/mushroomyuan/vpp-backend/platform/redis"
	"github.com/mushroomyuan/vpp-backend/resource/domain/model"
)

const (
	seedTenantID = "001"
	seedAssetID  = "019e81c4-3c21-718c-85e9-1cc4e34627c1"
	seedCUID     = "019e81d8-ec58-7bff-9313-18c92b73cb1f"
	seedPointID  = "019e8209-de32-7de9-bc86-511a03ab6faf"
)

func float64Ptr(v float64) *float64 { return &v }
func int64Ptr(v int64) *int64       { return &v }
func stringPtr(v string) *string    { return &v }

func newTestRedisClient(t *testing.T) (*platformredis.Client, func()) {
	t.Helper()

	if addr := os.Getenv("REDIS_ADDR"); addr != "" {
		client, err := platformredis.New(platformredis.Config{Addr: addr})
		if err != nil {
			t.Fatalf("connect redis %s: %v", addr, err)
		}
		return client, func() { _ = client.Close() }
	}

	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("start miniredis: %v", err)
	}
	client, err := platformredis.New(platformredis.Config{Addr: mr.Addr()})
	if err != nil {
		mr.Close()
		t.Fatalf("connect miniredis: %v", err)
	}
	return client, func() {
		_ = client.Close()
		mr.Close()
	}
}

func seedAssetRuntime(t *testing.T, store *AssetRuntimeStore) {
	t.Helper()
	now := time.Now().UTC().Truncate(time.Millisecond)
	reason := "maintenance window"
	err := store.SetAssetRuntime(context.Background(), &model.AssetRuntime{
		TenantID:              seedTenantID,
		AssetID:               seedAssetID,
		Online:                true,
		CurrentPowerKW:        float64Ptr(120.5),
		AvailablePowerKW:      float64Ptr(200),
		SOC:                   float64Ptr(78.2),
		Dispatchable:          true,
		NotDispatchableReason: &reason,
		MaxChargePowerKW:      float64Ptr(150),
		MaxDischargePowerKW:   float64Ptr(180),
		UpdatedAt:             now,
	})
	if err != nil {
		t.Fatalf("seed asset runtime: %v", err)
	}
}

func seedCURuntime(t *testing.T, store *CURuntimeStore) {
	t.Helper()
	now := time.Now().UTC().Truncate(time.Millisecond)
	err := store.SetCURuntime(context.Background(), &model.CURuntime{
		TenantID:   seedTenantID,
		CUID:       seedCUID,
		ConnStatus: "connected",
		LastSeenAt: now.Add(-2 * time.Second),
		LatencyMS:  int64Ptr(35),
		LastError:  nil,
		UpdatedAt:  now,
	})
	if err != nil {
		t.Fatalf("seed cu runtime: %v", err)
	}
}

func seedPointRuntime(t *testing.T, store *PointRuntimeStore) {
	t.Helper()
	now := time.Now().UTC().Truncate(time.Millisecond)
	value := "42.5"
	quality := "good"
	err := store.SetPointRuntime(context.Background(), &model.PointRuntime{
		TenantID:      seedTenantID,
		PointID:       seedPointID,
		Value:         &value,
		NumericValue:  float64Ptr(42.5),
		QualityStatus: &quality,
		Sequence:      1001,
		SampledAt:     now.Add(-5 * time.Second),
		UpdatedAt:     now,
	})
	if err != nil {
		t.Fatalf("seed point runtime: %v", err)
	}
}

func flushSeedKeys(t *testing.T, client *platformredis.Client) {
	t.Helper()
	rdb := client.Client()
	if rdb == nil {
		t.Fatal("redis client is nil")
	}
	ctx := context.Background()
	keys := []string{
		assetRuntimeKey(seedTenantID, seedAssetID),
		cuRuntimeKey(seedTenantID, seedCUID),
		pointRuntimeKey(seedTenantID, seedPointID),
	}
	if err := rdb.Del(ctx, keys...).Err(); err != nil {
		t.Fatalf("flush seed keys: %v", err)
	}
}

func assertRedisKeyExists(t *testing.T, client *platformredis.Client, key string) {
	t.Helper()
	rdb := client.Client()
	exists, err := rdb.Exists(context.Background(), key).Result()
	if err != nil {
		t.Fatalf("exists %s: %v", key, err)
	}
	if exists != 1 {
		t.Fatalf("expected key %q to exist", key)
	}
}
