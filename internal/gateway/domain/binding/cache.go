package binding

import (
	"context"
	"fmt"
	"sync"
	"time"
)

const (
	// OriginCache means the copy was still inside TTL, so Resource was not called.
	OriginCache = "cache"
	// OriginFresh means Resource just returned this copy.
	OriginFresh = "fresh"
	// OriginFallback means Resource failed and a copy younger than MaxAge was reused.
	OriginFallback = "fallback"
)

// Catalog loads the current bindings for one CU. ListPoints is the source.
// The cache calls it only after TTL expiry or invalidation. Event handlers must not.
type Catalog interface {
	ListCUBindings(ctx context.Context, tenantID, cuID string) ([]Binding, error)
}

// Observer records load outcomes. A nil observer is ignored.
type Observer interface {
	ObserveBindingLoad(result string)
}

// CacheConfig builds a last-known-good cache in front of Catalog.
// TTL is how long a successful load is reused. MaxAge is how long that copy
// may still be used after Catalog fails. An invalidation drops the copy immediately.
type CacheConfig struct {
	Catalog  Catalog
	TTL      time.Duration
	MaxAge   time.Duration
	Now      func() time.Time
	Observer Observer
}

// Cache stores bindings by tenant and CU. Events only delete entries.
type Cache struct {
	catalog  Catalog
	ttl      time.Duration
	maxAge   time.Duration
	now      func() time.Time
	observer Observer

	mu          sync.Mutex
	entries     map[cuKey]entry
	epoch       map[cuKey]uint64
	tenantEpoch map[string]uint64
}

type cuKey struct {
	tenant string
	cu     string
}

type entry struct {
	snapshot Snapshot
	storedAt time.Time
}

// NewCache requires a catalog, a positive TTL, and a max age at least as long as the TTL.
func NewCache(cfg CacheConfig) *Cache {
	if cfg.Catalog == nil {
		panic("binding cache: catalog is required")
	}
	if cfg.TTL <= 0 {
		panic("binding cache: ttl must be positive")
	}
	if cfg.MaxAge < cfg.TTL {
		panic("binding cache: max age must be at least ttl")
	}
	now := cfg.Now
	if now == nil {
		now = time.Now
	}
	return &Cache{
		catalog:     cfg.Catalog,
		ttl:         cfg.TTL,
		maxAge:      cfg.MaxAge,
		now:         now,
		observer:    cfg.Observer,
		entries:     map[cuKey]entry{},
		epoch:       map[cuKey]uint64{},
		tenantEpoch: map[string]uint64{},
	}
}

// Load returns bindings for one CU.
// Inside TTL it does not call Resource. After that it lists the CU again.
// A Resource error reuses a copy that is still inside MaxAge.
// An older copy is dropped. A successful list that races an invalidation is not stored.
func (c *Cache) Load(ctx context.Context, tenantID, cuID string) (Snapshot, string, error) {
	if tenantID == "" || cuID == "" {
		return Snapshot{}, "", fmt.Errorf("binding cache: tenant and cu are required")
	}
	key := cuKey{tenant: tenantID, cu: cuID}
	now := c.now()
	if snap, ok := c.lookup(key, now, c.ttl); ok {
		c.observe(OriginCache)
		return snap, OriginCache, nil
	}

	// Two attempts cover one invalidation that lands while ListPoints is in flight.
	// The in-flight response is not stored, because it may predate the change.
	for attempt := 0; attempt < 2; attempt++ {
		cuEpoch, tenantEpoch := c.generations(key)
		bindings, err := c.catalog.ListCUBindings(ctx, tenantID, cuID)
		if err != nil {
			if ctx.Err() != nil {
				return Snapshot{}, "", ctx.Err()
			}
			if snap, ok := c.lookup(key, now, c.maxAge); ok {
				c.observe(OriginFallback)
				return snap, OriginFallback, nil
			}
			c.observe("skipped")
			return Snapshot{}, "", fmt.Errorf("binding cache: %w", err)
		}
		snap := Snapshot{Bindings: cloneBindings(bindings)}
		if c.store(key, cuEpoch, tenantEpoch, snap, c.now()) {
			c.observe(OriginFresh)
			return cloneSnapshot(snap), OriginFresh, nil
		}
	}
	c.observe("skipped")
	return Snapshot{}, "", fmt.Errorf("binding cache: bindings changed while loading")
}

// InvalidateCU drops one CU. The next Load lists it again.
func (c *Cache) InvalidateCU(tenantID, cuID string) {
	if tenantID == "" || cuID == "" {
		return
	}
	c.invalidate(cuKey{tenant: tenantID, cu: cuID})
}

// InvalidatePoint drops every cached CU in the tenant that currently holds the point.
// Point update and delete events do not carry a CU id.
func (c *Cache) InvalidatePoint(tenantID, pointID string) {
	if tenantID == "" || pointID == "" {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	for key, item := range c.entries {
		if key.tenant != tenantID || !containsPoint(item.snapshot.Bindings, pointID) {
			continue
		}
		delete(c.entries, key)
		c.epoch[key]++
	}
}

// InvalidateTenant drops every CU cached for the tenant.
// A descendant delete or a point import does not name each CU.
func (c *Cache) InvalidateTenant(tenantID string) {
	if tenantID == "" {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.tenantEpoch[tenantID]++
	for key := range c.entries {
		if key.tenant != tenantID {
			continue
		}
		delete(c.entries, key)
		c.epoch[key]++
	}
}

func (c *Cache) lookup(key cuKey, now time.Time, max time.Duration) (Snapshot, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	item, ok := c.entries[key]
	if !ok || now.Sub(item.storedAt) > max {
		return Snapshot{}, false
	}
	return cloneSnapshot(item.snapshot), true
}

func (c *Cache) generations(key cuKey) (cuEpoch, tenantEpoch uint64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.epoch[key], c.tenantEpoch[key.tenant]
}

func (c *Cache) store(key cuKey, cuEpoch, tenantEpoch uint64, snap Snapshot, storedAt time.Time) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.epoch[key] != cuEpoch || c.tenantEpoch[key.tenant] != tenantEpoch {
		return false
	}
	c.entries[key] = entry{snapshot: cloneSnapshot(snap), storedAt: storedAt}
	return true
}

func (c *Cache) invalidate(key cuKey) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.entries, key)
	c.epoch[key]++
}

func (c *Cache) observe(result string) {
	if c.observer != nil {
		c.observer.ObserveBindingLoad(result)
	}
}

func containsPoint(bindings []Binding, pointID string) bool {
	for _, binding := range bindings {
		if binding.PointID == pointID {
			return true
		}
	}
	return false
}

func cloneSnapshot(in Snapshot) Snapshot {
	return Snapshot{Bindings: cloneBindings(in.Bindings)}
}

func cloneBindings(in []Binding) []Binding {
	if in == nil {
		return []Binding{}
	}
	out := make([]Binding, len(in))
	for i, binding := range in {
		out[i] = binding
		out[i].Safety = cloneSafety(binding.Safety)
	}
	return out
}

func cloneSafety(in *Safety) *Safety {
	if in == nil {
		return nil
	}
	out := *in
	out.MinValue = cloneFloat(in.MinValue)
	out.MaxValue = cloneFloat(in.MaxValue)
	out.MaxChangePerSecond = cloneFloat(in.MaxChangePerSecond)
	return &out
}

func cloneFloat(v *float64) *float64 {
	if v == nil {
		return nil
	}
	c := *v
	return &c
}
