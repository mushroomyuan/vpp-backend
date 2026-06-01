package redisruntime

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

// CURuntimeStore implements port.CURuntimeCache with JSON values in Redis.
type CURuntimeStore struct {
	client *platformredis.Client
	ttl    time.Duration
}

// NewCURuntimeCache constructs a CURuntimeStore. ttl <= 0 means no TTL.
func NewCURuntimeCache(client *platformredis.Client, ttl time.Duration) *CURuntimeStore {
	return &CURuntimeStore{client: client, ttl: ttl}
}

func (s *CURuntimeStore) rdb(ctx context.Context) (*goredis.Client, error) {
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

func (s *CURuntimeStore) GetCURuntime(ctx context.Context, tenantID, cuID string) (*model.CURuntime, error) {
	rdb, err := s.rdb(ctx)
	if err != nil {
		return nil, err
	}
	val, err := rdb.Get(ctx, cuRuntimeKey(tenantID, cuID)).Result()
	if errors.Is(err, goredis.Nil) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out model.CURuntime
	if err := json.Unmarshal([]byte(val), &out); err != nil {
		return nil, fmt.Errorf("decode cu runtime: %w", err)
	}
	return &out, nil
}

func (s *CURuntimeStore) ListCURuntimes(ctx context.Context, tenantID string, cuIDs []string) ([]*model.CURuntime, error) {
	rdb, err := s.rdb(ctx)
	if err != nil {
		return nil, err
	}
	if len(cuIDs) == 0 {
		return nil, nil
	}
	keys := make([]string, len(cuIDs))
	for i, id := range cuIDs {
		keys[i] = cuRuntimeKey(tenantID, id)
	}
	vals, err := rdb.MGet(ctx, keys...).Result()
	if err != nil {
		return nil, err
	}
	out := make([]*model.CURuntime, len(cuIDs))
	for i, v := range vals {
		if v == nil {
			continue
		}
		sval, ok := v.(string)
		if !ok || sval == "" {
			continue
		}
		var row model.CURuntime
		if err := json.Unmarshal([]byte(sval), &row); err != nil {
			return nil, fmt.Errorf("decode cu runtime %s: %w", cuIDs[i], err)
		}
		cp := row
		out[i] = &cp
	}
	return out, nil
}

func (s *CURuntimeStore) SetCURuntime(ctx context.Context, r *model.CURuntime) error {
	if r == nil {
		return fmt.Errorf("cu runtime is nil")
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
		return fmt.Errorf("encode cu runtime: %w", err)
	}
	return rdb.Set(ctx, cuRuntimeKey(cp.TenantID, cp.CUID), string(payload), s.ttl).Err()
}

func (s *CURuntimeStore) PatchCURuntime(ctx context.Context, tenantID, cuID string, patch port.CURuntimePatch) error {
	key := cuRuntimeKey(tenantID, cuID)
	m := cuPatchToMap(patch)
	m["UpdatedAt"] = time.Now().UTC().Format(time.RFC3339Nano)
	return mergePatchJSON(ctx, s.client, key, s.ttl, m)
}

func (s *CURuntimeStore) DeleteCURuntime(ctx context.Context, tenantID, cuID string) error {
	rdb, err := s.rdb(ctx)
	if err != nil {
		return err
	}
	return rdb.Del(ctx, cuRuntimeKey(tenantID, cuID)).Err()
}

func cuPatchToMap(p port.CURuntimePatch) map[string]any {
	m := map[string]any{}
	if p.ConnStatus != nil {
		m["ConnStatus"] = *p.ConnStatus
	}
	if p.LastSeenAt != nil {
		m["LastSeenAt"] = p.LastSeenAt.UTC().Format(time.RFC3339Nano)
	}
	if p.LatencyMS != nil {
		m["LatencyMS"] = *p.LatencyMS
	}
	if p.LastError != nil {
		m["LastError"] = *p.LastError
	}
	return m
}

var _ port.CURuntimeCache = (*CURuntimeStore)(nil)
