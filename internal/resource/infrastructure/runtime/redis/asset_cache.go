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

// AssetRuntimeStore implements port.AssetRuntimeCache with JSON values in Redis.
type AssetRuntimeStore struct {
	client *platformredis.Client
	ttl    time.Duration
}

// NewAssetRuntimeCache constructs an AssetRuntimeStore. ttl <= 0 means no TTL.
func NewAssetRuntimeCache(client *platformredis.Client, ttl time.Duration) *AssetRuntimeStore {
	return &AssetRuntimeStore{client: client, ttl: ttl}
}

func (s *AssetRuntimeStore) rdb(ctx context.Context) (*goredis.Client, error) {
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

func (s *AssetRuntimeStore) GetAssetRuntime(ctx context.Context, tenantID, assetID string) (*model.AssetRuntime, error) {
	rdb, err := s.rdb(ctx)
	if err != nil {
		return nil, err
	}
	val, err := rdb.Get(ctx, assetRuntimeKey(tenantID, assetID)).Result()
	if errors.Is(err, goredis.Nil) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out model.AssetRuntime
	if err := json.Unmarshal([]byte(val), &out); err != nil {
		return nil, fmt.Errorf("decode asset runtime: %w", err)
	}
	return &out, nil
}

func (s *AssetRuntimeStore) ListAssetRuntimes(ctx context.Context, tenantID string, assetIDs []string) ([]*model.AssetRuntime, error) {
	rdb, err := s.rdb(ctx)
	if err != nil {
		return nil, err
	}
	if len(assetIDs) == 0 {
		return nil, nil
	}
	keys := make([]string, len(assetIDs))
	for i, id := range assetIDs {
		keys[i] = assetRuntimeKey(tenantID, id)
	}
	vals, err := rdb.MGet(ctx, keys...).Result()
	if err != nil {
		return nil, err
	}
	out := make([]*model.AssetRuntime, len(assetIDs))
	for i, v := range vals {
		if v == nil {
			continue
		}
		sval, ok := v.(string)
		if !ok || sval == "" {
			continue
		}
		var row model.AssetRuntime
		if err := json.Unmarshal([]byte(sval), &row); err != nil {
			return nil, fmt.Errorf("decode asset runtime %s: %w", assetIDs[i], err)
		}
		cp := row
		out[i] = &cp
	}
	return out, nil
}

func (s *AssetRuntimeStore) SetAssetRuntime(ctx context.Context, r *model.AssetRuntime) error {
	if r == nil {
		return fmt.Errorf("asset runtime is nil")
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
		return fmt.Errorf("encode asset runtime: %w", err)
	}
	return rdb.Set(ctx, assetRuntimeKey(cp.TenantID, cp.AssetID), string(payload), s.ttl).Err()
}

func (s *AssetRuntimeStore) PatchAssetRuntime(ctx context.Context, tenantID, assetID string, patch port.AssetRuntimePatch) error {
	key := assetRuntimeKey(tenantID, assetID)
	m := assetPatchToMap(patch)
	m["UpdatedAt"] = time.Now().UTC().Format(time.RFC3339Nano)
	return mergePatchJSON(ctx, s.client, key, s.ttl, m)
}

func (s *AssetRuntimeStore) DeleteAssetRuntime(ctx context.Context, tenantID, assetID string) error {
	rdb, err := s.rdb(ctx)
	if err != nil {
		return err
	}
	return rdb.Del(ctx, assetRuntimeKey(tenantID, assetID)).Err()
}

func assetPatchToMap(p port.AssetRuntimePatch) map[string]any {
	m := map[string]any{}
	if p.Online != nil {
		m["Online"] = *p.Online
	}
	if p.CurrentPowerKW != nil {
		m["CurrentPowerKW"] = *p.CurrentPowerKW
	}
	if p.AvailablePowerKW != nil {
		m["AvailablePowerKW"] = *p.AvailablePowerKW
	}
	if p.SOC != nil {
		m["SOC"] = *p.SOC
	}
	if p.Dispatchable != nil {
		m["Dispatchable"] = *p.Dispatchable
	}
	if p.NotDispatchableReason != nil {
		m["NotDispatchableReason"] = *p.NotDispatchableReason
	}
	if p.MaxChargePowerKW != nil {
		m["MaxChargePowerKW"] = *p.MaxChargePowerKW
	}
	if p.MaxDischargePowerKW != nil {
		m["MaxDischargePowerKW"] = *p.MaxDischargePowerKW
	}
	return m
}

var _ port.AssetRuntimeCache = (*AssetRuntimeStore)(nil)
