package rediscache

import (
	"context"
	"testing"
	"time"

	miniredis "github.com/alicebob/miniredis/v2"

	"github.com/mushroomyuan/vpp-backend/forecast/domain/model"
	platformredis "github.com/mushroomyuan/vpp-backend/platform/redis"
)

func TestLatestBatchKey(t *testing.T) {
	got := latestBatchKey("tenant-a", "cu-battery-1", "active_power_kw")
	want := "forecast:latest:tenant-a:cu-battery-1:active_power_kw"
	if got != want {
		t.Errorf("key = %q, want %q", got, want)
	}
}

func TestCache_SetAndGetRoundTrip(t *testing.T) {
	client, _, cleanup := newTestRedis(t)
	defer cleanup()

	cache := NewCache(client, time.Hour)
	ctx := context.Background()
	batch := sampleBatch()

	if err := cache.SetLatestBatch(ctx, batch); err != nil {
		t.Fatalf("SetLatestBatch: %v", err)
	}
	got, err := cache.GetLatestBatch(ctx, "tenant-a", "cu-battery-1", "active_power_kw")
	if err != nil {
		t.Fatalf("GetLatestBatch: %v", err)
	}
	if len(got) != len(batch) {
		t.Fatalf("len = %d, want %d", len(got), len(batch))
	}
	for i := range batch {
		if got[i].TenantID != batch[i].TenantID ||
			got[i].CUCode != batch[i].CUCode ||
			got[i].MetricName != batch[i].MetricName ||
			got[i].PredictedValue != batch[i].PredictedValue ||
			got[i].AlgorithmVersion != batch[i].AlgorithmVersion {
			t.Errorf("point %d = %+v, want %+v", i, got[i], batch[i])
		}
		if !got[i].GeneratedAt.Equal(batch[i].GeneratedAt.UTC()) {
			t.Errorf("point %d GeneratedAt = %s", i, got[i].GeneratedAt)
		}
		if !got[i].TargetTimestamp.Equal(batch[i].TargetTimestamp.UTC()) {
			t.Errorf("point %d TargetTimestamp = %s", i, got[i].TargetTimestamp)
		}
	}
}

func TestCache_GetLatestBatch_MissIsNilNil(t *testing.T) {
	client, _, cleanup := newTestRedis(t)
	defer cleanup()

	cache := NewCache(client, time.Hour)
	got, err := cache.GetLatestBatch(context.Background(), "tenant-a", "cu-battery-1", "active_power_kw")
	if err != nil {
		t.Fatalf("GetLatestBatch: %v", err)
	}
	if got != nil {
		t.Fatalf("miss must be nil, got %+v", got)
	}
}

func TestCache_GetLatestBatch_CorruptJSONIsError(t *testing.T) {
	client, _, cleanup := newTestRedis(t)
	defer cleanup()

	key := latestBatchKey("tenant-a", "cu-battery-1", "active_power_kw")
	if err := client.Client().Set(context.Background(), key, "not-json", 0).Err(); err != nil {
		t.Fatalf("seed corrupt key: %v", err)
	}
	cache := NewCache(client, time.Hour)
	if _, err := cache.GetLatestBatch(context.Background(), "tenant-a", "cu-battery-1", "active_power_kw"); err == nil {
		t.Fatal("expected decode error")
	}
}

func TestCache_SetLatestBatch_RejectsEmptyAndMixedIdentity(t *testing.T) {
	client, _, cleanup := newTestRedis(t)
	defer cleanup()
	cache := NewCache(client, time.Hour)
	ctx := context.Background()

	if err := cache.SetLatestBatch(ctx, nil); err == nil {
		t.Fatal("empty batch must error")
	}

	batch := sampleBatch()
	batch[1].CUCode = "other-cu"
	if err := cache.SetLatestBatch(ctx, batch); err == nil {
		t.Fatal("mixed-identity batch must error")
	}
}

func TestCache_TTLExpiresTheKey(t *testing.T) {
	client, mr, cleanup := newTestRedis(t)
	defer cleanup()

	ttl := 30 * time.Second
	cache := NewCache(client, ttl)
	if err := cache.SetLatestBatch(context.Background(), sampleBatch()); err != nil {
		t.Fatalf("SetLatestBatch: %v", err)
	}
	key := latestBatchKey("tenant-a", "cu-battery-1", "active_power_kw")
	if !mr.Exists(key) {
		t.Fatal("expected key to exist before TTL")
	}
	mr.FastForward(ttl + time.Second)
	got, err := cache.GetLatestBatch(context.Background(), "tenant-a", "cu-battery-1", "active_power_kw")
	if err != nil {
		t.Fatalf("GetLatestBatch after TTL: %v", err)
	}
	if got != nil {
		t.Fatalf("expired key must miss, got %+v", got)
	}
}

func TestCache_RequiresIdentityOnGet(t *testing.T) {
	client, _, cleanup := newTestRedis(t)
	defer cleanup()
	cache := NewCache(client, time.Hour)
	if _, err := cache.GetLatestBatch(context.Background(), "", "cu", "kw"); err == nil {
		t.Fatal("empty tenant must error")
	}
}

func newTestRedis(t *testing.T) (*platformredis.Client, *miniredis.Miniredis, func()) {
	t.Helper()
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("start miniredis: %v", err)
	}
	client, err := platformredis.New(platformredis.Config{Addr: mr.Addr()})
	if err != nil {
		mr.Close()
		t.Fatalf("connect miniredis: %v", err)
	}
	return client, mr, func() {
		_ = client.Close()
		mr.Close()
	}
}

func sampleBatch() []model.Prediction {
	generated := time.Date(2026, 9, 16, 10, 7, 0, 0, time.UTC)
	return []model.Prediction{
		{
			TenantID: "tenant-a", CUCode: "cu-battery-1", MetricName: "active_power_kw",
			GeneratedAt: generated, TargetTimestamp: time.Date(2026, 9, 16, 10, 15, 0, 0, time.UTC),
			PredictedValue: 100, AlgorithmVersion: "moving_average",
		},
		{
			TenantID: "tenant-a", CUCode: "cu-battery-1", MetricName: "active_power_kw",
			GeneratedAt: generated, TargetTimestamp: time.Date(2026, 9, 16, 10, 30, 0, 0, time.UTC),
			PredictedValue: 100, AlgorithmVersion: "moving_average",
		},
	}
}
