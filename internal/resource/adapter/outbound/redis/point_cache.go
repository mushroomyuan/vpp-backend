package redis

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	goredis "github.com/redis/go-redis/v9"

	platformredis "github.com/mushroomyuan/vpp-backend/platform/redis"
	"github.com/mushroomyuan/vpp-backend/resource/domain/model"
	"github.com/mushroomyuan/vpp-backend/resource/domain/port"
)

// PointRuntimeStore implements port.PointRuntimeCache with JSON values in Redis.
type PointRuntimeStore struct {
	client *platformredis.Client
	ttl    time.Duration
}

// NewPointRuntimeCache constructs a PointRuntimeStore. ttl <= 0 means no TTL.
func NewPointRuntimeCache(client *platformredis.Client, ttl time.Duration) *PointRuntimeStore {
	return &PointRuntimeStore{client: client, ttl: ttl}
}

func (s *PointRuntimeStore) rdb(ctx context.Context) (*goredis.Client, error) {
	_ = ctx
	if s == nil || s.client == nil {
		return nil, fmt.Errorf("redis client is nil")
	}
	rdb := s.client.Client()
	if rdb == nil {
		return nil, fmt.Errorf("redis underlying client is nil")
	}
	return rdb, nil
}

func (s *PointRuntimeStore) GetPointRuntime(ctx context.Context, tenantID, pointID string) (*model.PointRuntime, error) {
	rdb, err := s.rdb(ctx)
	if err != nil {
		return nil, err
	}
	val, err := rdb.Get(ctx, pointRuntimeKey(tenantID, pointID)).Result()
	if errors.Is(err, goredis.Nil) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out model.PointRuntime
	if err := json.Unmarshal([]byte(val), &out); err != nil {
		return nil, fmt.Errorf("decode point runtime: %w", err)
	}
	return &out, nil
}

func (s *PointRuntimeStore) MGetPointRuntimes(ctx context.Context, tenantID string, pointIDs []string) (map[string]*model.PointRuntime, error) {
	rdb, err := s.rdb(ctx)
	if err != nil {
		return nil, err
	}
	if len(pointIDs) == 0 {
		return map[string]*model.PointRuntime{}, nil
	}
	keys := make([]string, len(pointIDs))
	for i, id := range pointIDs {
		keys[i] = pointRuntimeKey(tenantID, id)
	}
	vals, err := rdb.MGet(ctx, keys...).Result()
	if err != nil {
		return nil, err
	}
	out := make(map[string]*model.PointRuntime, len(pointIDs))
	for i, v := range vals {
		pid := pointIDs[i]
		if v == nil {
			continue
		}
		sval, ok := v.(string)
		if !ok || sval == "" {
			continue
		}
		var row model.PointRuntime
		if err := json.Unmarshal([]byte(sval), &row); err != nil {
			return nil, fmt.Errorf("decode point runtime %s: %w", pid, err)
		}
		cp := row
		out[pid] = &cp
	}
	return out, nil
}

func (s *PointRuntimeStore) SetPointRuntime(ctx context.Context, r *model.PointRuntime) error {
	if r == nil {
		return fmt.Errorf("point runtime is nil")
	}
	rdb, err := s.rdb(ctx)
	if err != nil {
		return err
	}
	cp := *r
	if cp.UpdatedAt.IsZero() {
		cp.UpdatedAt = time.Now()
	}
	payload, err := json.Marshal(cp)
	if err != nil {
		return fmt.Errorf("encode point runtime: %w", err)
	}
	return rdb.Set(ctx, pointRuntimeKey(cp.TenantID, cp.PointID), string(payload), s.ttl).Err()
}

func (s *PointRuntimeStore) MSetPointRuntimes(ctx context.Context, runtimes []*model.PointRuntime) error {
	if len(runtimes) == 0 {
		return nil
	}
	rdb, err := s.rdb(ctx)
	if err != nil {
		return err
	}

	pipe := rdb.Pipeline()
	for _, r := range runtimes {
		if r == nil {
			return fmt.Errorf("point runtime slice contains nil")
		}
		cp := *r
		if cp.UpdatedAt.IsZero() {
			cp.UpdatedAt = time.Now()
		}
		payload, mErr := json.Marshal(cp)
		if mErr != nil {
			return fmt.Errorf("encode point runtime %s: %w", cp.PointID, mErr)
		}
		pipe.Set(ctx, pointRuntimeKey(cp.TenantID, cp.PointID), string(payload), s.ttl)
	}
	_, err = pipe.Exec(ctx)
	return err
}

func (s *PointRuntimeStore) DeletePointRuntime(ctx context.Context, tenantID, pointID string) error {
	rdb, err := s.rdb(ctx)
	if err != nil {
		return err
	}
	return rdb.Del(ctx, pointRuntimeKey(tenantID, pointID)).Err()
}

var _ port.PointRuntimeCache = (*PointRuntimeStore)(nil)
