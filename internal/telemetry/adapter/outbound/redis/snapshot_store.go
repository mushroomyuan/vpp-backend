package redisadapter

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	goredis "github.com/redis/go-redis/v9"

	"github.com/mushroomyuan/vpp-backend/telemetry/domain"
	"github.com/mushroomyuan/vpp-backend/telemetry/domain/model"
	"github.com/mushroomyuan/vpp-backend/telemetry/domain/port"
)

// SnapshotStore implements port.SnapshotRepository using Redis.
//
// Each CU snapshot is stored as a single JSON string under the key:
//
//	tenant:{tenantID}:cu:{cuCode}:snapshot
//
// This matches the PointRuntime pattern used by the resource module.
// TTL is optional; set ttl = 0 for no expiry (recommended for snapshots that
// should survive service restarts, as opposed to ephemeral hot-path caches).
type SnapshotStore struct {
	client *goredis.Client
	ttl    time.Duration
}

// NewSnapshotStore constructs a SnapshotStore. ttl <= 0 means keys never expire.
func NewSnapshotStore(client *goredis.Client, ttl time.Duration) *SnapshotStore {
	return &SnapshotStore{client: client, ttl: ttl}
}

// Save persists the snapshot. Existing values are overwritten atomically via SET.
func (s *SnapshotStore) Save(ctx context.Context, snapshot *model.Snapshot) error {
	if snapshot == nil {
		return fmt.Errorf("snapshot is nil")
	}
	payload, err := json.Marshal(snapshot)
	if err != nil {
		return fmt.Errorf("marshal snapshot: %w", err)
	}
	return s.client.Set(ctx, snapshotKey(snapshot.TenantID, snapshot.CUCode), payload, s.ttl).Err()
}

// Find returns the snapshot for a single CU.
// Returns domain.ErrSnapshotNotFound when the key does not exist in Redis
// (first ingest for this CU, or key has expired).
func (s *SnapshotStore) Find(ctx context.Context, tenantID, cuCode string) (*model.Snapshot, error) {
	val, err := s.client.Get(ctx, snapshotKey(tenantID, cuCode)).Result()
	if errors.Is(err, goredis.Nil) {
		return nil, domain.ErrSnapshotNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("redis get snapshot: %w", err)
	}
	snap, err := unmarshalSnapshot(val)
	if err != nil {
		return nil, fmt.Errorf("unmarshal snapshot: %w", err)
	}
	return snap, nil
}

// FindAll returns the latest snapshots for every CU belonging to tenantID.
// Internally uses SCAN to avoid blocking the Redis event loop (unlike KEYS *).
// Individual GET calls after SCAN are safe; if a key expires between SCAN
// and GET, the entry is silently skipped.
func (s *SnapshotStore) FindAll(ctx context.Context, tenantID string) ([]*model.Snapshot, error) {
	const scanBatch = 100

	var snapshots []*model.Snapshot
	var cursor uint64

	for {
		keys, nextCursor, err := s.client.Scan(ctx, cursor, snapshotPattern(tenantID), scanBatch).Result()
		if err != nil {
			return nil, fmt.Errorf("redis scan snapshots: %w", err)
		}
		for _, key := range keys {
			val, err := s.client.Get(ctx, key).Result()
			if errors.Is(err, goredis.Nil) {
				continue // expired between SCAN and GET
			}
			if err != nil {
				return nil, fmt.Errorf("redis get snapshot %s: %w", key, err)
			}
			snap, err := unmarshalSnapshot(val)
			if err != nil {
				return nil, fmt.Errorf("unmarshal snapshot %s: %w", key, err)
			}
			snapshots = append(snapshots, snap)
		}
		if nextCursor == 0 {
			break
		}
		cursor = nextCursor
	}
	return snapshots, nil
}

// FindByCUs loads the requested CU snapshots with one MGET.
// Missing keys are omitted. Order follows cuCodes.
func (s *SnapshotStore) FindByCUs(ctx context.Context, tenantID string, cuCodes []string) ([]*model.Snapshot, error) {
	if len(cuCodes) == 0 {
		return nil, nil
	}
	keys := make([]string, len(cuCodes))
	for i, cuCode := range cuCodes {
		keys[i] = snapshotKey(tenantID, cuCode)
	}
	vals, err := s.client.MGet(ctx, keys...).Result()
	if err != nil {
		return nil, fmt.Errorf("redis mget snapshots: %w", err)
	}
	snapshots := make([]*model.Snapshot, 0, len(vals))
	for i, val := range vals {
		if val == nil {
			continue
		}
		raw, ok := val.(string)
		if !ok {
			return nil, fmt.Errorf("redis mget snapshot %s: unexpected value type %T", keys[i], val)
		}
		snap, err := unmarshalSnapshot(raw)
		if err != nil {
			return nil, fmt.Errorf("unmarshal snapshot %s: %w", keys[i], err)
		}
		snapshots = append(snapshots, snap)
	}
	return snapshots, nil
}

func unmarshalSnapshot(raw string) (*model.Snapshot, error) {
	var snap model.Snapshot
	if err := json.Unmarshal([]byte(raw), &snap); err != nil {
		return nil, err
	}
	if snap.Metrics == nil {
		snap.Metrics = make(map[string]model.MetricState)
	}
	return &snap, nil
}

var _ port.SnapshotRepository = (*SnapshotStore)(nil)
