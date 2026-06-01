package redisruntime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	platformredis "github.com/mushroomyuan/vpp-backend/platform/redis"
)

// ErrNotFound is returned when a patch targets a Redis key that does not exist.
var ErrNotFound = errors.New("redisruntime: runtime record not found")

// Lua: merge JSON patch into stored JSON document, atomically replacing the whole value.
// ARGV[1] = TTL in milliseconds ("0" = no TTL). ARGV[2] = patch JSON object.
// Returns int: 1 on success, -1 if key missing.
const luaMergePatch = `
local raw = redis.call('GET', KEYS[1])
if not raw then
  return -1
end
local doc = cjson.decode(raw)
local patch = cjson.decode(ARGV[2])
for k, v in pairs(patch) do
  doc[k] = v
end
local ttl = tonumber(ARGV[1])
local out = cjson.encode(doc)
if ttl ~= nil and ttl > 0 then
  redis.call('SET', KEYS[1], out, 'PX', ttl)
else
  redis.call('SET', KEYS[1], out)
end
return 1
`

func ttlMillisArg(ttl time.Duration) string {
	if ttl <= 0 {
		return "0"
	}
	return strconv.FormatInt(ttl.Milliseconds(), 10)
}

func mergePatchJSON(ctx context.Context, c *platformredis.Client, key string, ttl time.Duration, patch map[string]any) error {
	if c == nil {
		return fmt.Errorf("redis client is nil")
	}
	rdb := c.Client()
	if rdb == nil {
		return fmt.Errorf("redis underlying client is nil")
	}
	if len(patch) == 0 {
		return nil
	}
	b, err := json.Marshal(patch)
	if err != nil {
		return fmt.Errorf("marshal patch: %w", err)
	}
	res, err := rdb.Eval(ctx, luaMergePatch, []string{key}, ttlMillisArg(ttl), string(b)).Result()
	if err != nil {
		return err
	}
	n, ok := res.(int64)
	if !ok {
		return fmt.Errorf("unexpected lua result type %T", res)
	}
	if n < 0 {
		return ErrNotFound
	}
	return nil
}
