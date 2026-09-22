package rediscache

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	goredis "github.com/redis/go-redis/v9"

	"github.com/mushroomyuan/vpp-backend/forecast/domain/model"
	"github.com/mushroomyuan/vpp-backend/forecast/domain/port"
	platformredis "github.com/mushroomyuan/vpp-backend/platform/redis"
)

// Cache implements port.CachePort with JSON values in Redis.
// ttl <= 0 means no expiration (tests); production wires
// HorizonSteps×StepSeconds+CycleInterval (config.RedisTTL).
type Cache struct {
	client *platformredis.Client
	ttl    time.Duration
}

// NewCache constructs a Cache. ttl is the only freshness-adjacent
// parameter the adapter owns: it keeps a deleted target from occupying
// Redis forever. SelectNextPoint, not TTL, decides whether a cached
// point is still the "next" one.
func NewCache(client *platformredis.Client, ttl time.Duration) *Cache {
	return &Cache{client: client, ttl: ttl}
}

var _ port.CachePort = (*Cache)(nil)

func (c *Cache) rdb() (*goredis.Client, error) {
	if c == nil || c.client == nil {
		return nil, fmt.Errorf("redis_cache: redis client is nil")
	}
	rdb := c.client.Client()
	if rdb == nil {
		return nil, fmt.Errorf("redis_cache: underlying redis client is nil")
	}
	return rdb, nil
}

// GetLatestBatch returns the cached horizon for one target.
// A miss is (nil, nil), not an error.
func (c *Cache) GetLatestBatch(ctx context.Context, tenantID, cuCode, metricName string) ([]model.Prediction, error) {
	if err := requireIdentity(tenantID, cuCode, metricName); err != nil {
		return nil, fmt.Errorf("redis_cache: GetLatestBatch: %w", err)
	}
	rdb, err := c.rdb()
	if err != nil {
		return nil, err
	}
	val, err := rdb.Get(ctx, latestBatchKey(tenantID, cuCode, metricName)).Result()
	if errors.Is(err, goredis.Nil) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("redis_cache: GetLatestBatch: %w", err)
	}
	var out []model.Prediction
	if err := json.Unmarshal([]byte(val), &out); err != nil {
		return nil, fmt.Errorf("redis_cache: decode latest batch: %w", err)
	}
	for i := range out {
		out[i].GeneratedAt = out[i].GeneratedAt.UTC()
		out[i].TargetTimestamp = out[i].TargetTimestamp.UTC()
	}
	return out, nil
}

// SetLatestBatch overwrites the cached horizon. batch must be non-empty
// and share one (tenant, CU, metric) identity — the key is derived from
// the first point.
func (c *Cache) SetLatestBatch(ctx context.Context, batch []model.Prediction) error {
	if len(batch) == 0 {
		return fmt.Errorf("redis_cache: SetLatestBatch: batch is empty")
	}
	first := batch[0]
	if err := requireIdentity(first.TenantID, first.CUCode, first.MetricName); err != nil {
		return fmt.Errorf("redis_cache: SetLatestBatch: %w", err)
	}
	for i, p := range batch {
		if p.TenantID != first.TenantID || p.CUCode != first.CUCode || p.MetricName != first.MetricName {
			return fmt.Errorf("redis_cache: SetLatestBatch: point %d identity differs from the batch", i)
		}
	}
	rdb, err := c.rdb()
	if err != nil {
		return err
	}
	stored := make([]model.Prediction, len(batch))
	for i, p := range batch {
		p.GeneratedAt = p.GeneratedAt.UTC()
		p.TargetTimestamp = p.TargetTimestamp.UTC()
		stored[i] = p
	}
	payload, err := json.Marshal(stored)
	if err != nil {
		return fmt.Errorf("redis_cache: encode latest batch: %w", err)
	}
	if err := rdb.Set(ctx, latestBatchKey(first.TenantID, first.CUCode, first.MetricName), payload, c.ttl).Err(); err != nil {
		return fmt.Errorf("redis_cache: SetLatestBatch: %w", err)
	}
	return nil
}

func requireIdentity(tenantID, cuCode, metricName string) error {
	if tenantID == "" || cuCode == "" || metricName == "" {
		return fmt.Errorf("tenant_id, cu_code, and metric_name are required")
	}
	return nil
}
