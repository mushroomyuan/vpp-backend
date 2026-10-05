package context

import (
	stdctx "context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/mushroomyuan/vpp-backend/decision/domain/port"
)

const (
	ResolveCache    = "cache"
	ResolveFresh    = "fresh"
	ResolveFallback = "fallback"
	ResolveSkipped  = "skipped"
)

// ResolveObserver records scope-cache outcomes. A nil observer is ignored.
type ResolveObserver interface {
	ObserveScopeResolve(result string)
}

// CachingResolverConfig builds a ScopeResolver over ResourcePort.
// TTL is how long a successful resolution is reused without calling Resource.
// MaxAge is how long that copy may still be used after Resource fails.
// Enable and update do not use this cache.
type CachingResolverConfig struct {
	Resource port.ResourcePort
	TTL      time.Duration
	MaxAge   time.Duration
	Now      func() time.Time
	Observer ResolveObserver
}

// CachingScopeResolver is a last-known-good cache in front of ResolveScope.
type CachingScopeResolver struct {
	resource port.ResourcePort
	ttl      time.Duration
	maxAge   time.Duration
	now      func() time.Time
	observer ResolveObserver

	mu    sync.Mutex
	cache map[scopeCacheKey]cachedScope
}

type scopeCacheKey struct {
	tenant    string
	scopeType port.ScopeType
	scopeID   string
	caps      string
	metrics   string
}

type cachedScope struct {
	scope    port.ResolvedScope
	storedAt time.Time
}

// NewCachingScopeResolver caches successful prechecked resolutions.
func NewCachingScopeResolver(cfg CachingResolverConfig) *CachingScopeResolver {
	if cfg.Resource == nil {
		panic("NewCachingScopeResolver: resource port is required")
	}
	if cfg.TTL <= 0 {
		panic("NewCachingScopeResolver: ttl must be positive")
	}
	if cfg.MaxAge < cfg.TTL {
		panic("NewCachingScopeResolver: max age must be at least ttl")
	}
	now := cfg.Now
	if now == nil {
		now = time.Now
	}
	return &CachingScopeResolver{
		resource: cfg.Resource,
		ttl:      cfg.TTL,
		maxAge:   cfg.MaxAge,
		now:      now,
		observer: cfg.Observer,
		cache:    map[scopeCacheKey]cachedScope{},
	}
}

var _ ScopeResolver = (*CachingScopeResolver)(nil)

// Resolve returns a cached scope inside TTL. After TTL it refreshes.
// A Resource error reuses a copy that is still inside MaxAge.
// An older copy is dropped and ErrScopeUnavailable is returned.
// A fresh precheck failure is returned as-is and is not cached.
func (r *CachingScopeResolver) Resolve(ctx stdctx.Context, query port.ScopeQuery) (port.ResolvedScope, error) {
	key := cacheKey(query)
	now := r.now()
	if scope, ok := r.lookup(key, now, r.ttl); ok {
		r.observe(ResolveCache)
		return scope, nil
	}

	resolved, err := r.resource.ResolveScope(ctx, query)
	if err != nil {
		if scope, ok := r.lookup(key, now, r.maxAge); ok {
			r.observe(ResolveFallback)
			return scope, nil
		}
		r.observe(ResolveSkipped)
		return port.ResolvedScope{}, fmt.Errorf("scope resolver: %w: %v", ErrScopeUnavailable, err)
	}
	if !resolved.PrecheckOK || strings.TrimSpace(resolved.ResourceRevision) == "" {
		r.invalidate(key)
		r.observe(ResolveFresh)
		return cloneScope(resolved), nil
	}
	stored := cloneScope(resolved)
	r.mu.Lock()
	r.cache[key] = cachedScope{scope: stored, storedAt: now}
	r.mu.Unlock()
	r.observe(ResolveFresh)
	return cloneScope(stored), nil
}

func (r *CachingScopeResolver) lookup(key scopeCacheKey, now time.Time, max time.Duration) (port.ResolvedScope, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	item, ok := r.cache[key]
	if !ok || now.Sub(item.storedAt) > max {
		return port.ResolvedScope{}, false
	}
	return cloneScope(item.scope), true
}

func (r *CachingScopeResolver) invalidate(key scopeCacheKey) {
	r.mu.Lock()
	delete(r.cache, key)
	r.mu.Unlock()
}

func (r *CachingScopeResolver) observe(result string) {
	if r.observer != nil {
		r.observer.ObserveScopeResolve(result)
	}
}

func cacheKey(query port.ScopeQuery) scopeCacheKey {
	caps := make([]string, len(query.RequiredCapabilityIDs))
	for i, id := range query.RequiredCapabilityIDs {
		caps[i] = string(id)
	}
	metrics := make([]string, len(query.RequiredMetrics))
	for i, metric := range query.RequiredMetrics {
		metrics[i] = string(metric.MetricID) + ":" + string(metric.Access)
	}
	return scopeCacheKey{
		tenant:    strings.TrimSpace(query.TenantID),
		scopeType: query.ScopeType,
		scopeID:   strings.TrimSpace(query.ScopeID),
		caps:      strings.Join(caps, ","),
		metrics:   strings.Join(metrics, ","),
	}
}

func cloneScope(in port.ResolvedScope) port.ResolvedScope {
	out := in
	out.ScopeID = strings.TrimSpace(in.ScopeID)
	out.ResourceRevision = strings.TrimSpace(in.ResourceRevision)
	out.Members = make([]port.ResolvedCU, len(in.Members))
	for i, member := range in.Members {
		out.Members[i] = member
		out.Members[i].CUID = strings.TrimSpace(member.CUID)
		out.Members[i].AssetID = strings.TrimSpace(member.AssetID)
		out.Members[i].Capabilities = cloneCapabilities(member.Capabilities)
		out.Members[i].Bindings = cloneBindings(member.Bindings)
	}
	out.Exclusions = append([]port.ScopeExclusion(nil), in.Exclusions...)
	out.PrecheckFailures = append([]port.ScopePrecheckFailure(nil), in.PrecheckFailures...)
	return out
}

func cloneCapabilities(in []port.ResolvedCapability) []port.ResolvedCapability {
	out := make([]port.ResolvedCapability, len(in))
	for i, cap := range in {
		out[i] = cap
		out[i].Spec = cloneSpec(cap.Spec)
	}
	return out
}

func cloneSpec(in map[string]any) map[string]any {
	if in == nil {
		return nil
	}
	out := make(map[string]any, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}

func cloneBindings(in []port.ResolvedBinding) []port.ResolvedBinding {
	out := make([]port.ResolvedBinding, len(in))
	for i, binding := range in {
		out[i] = binding
		out[i].Safety = cloneBindingSafety(binding.Safety)
	}
	return out
}

func cloneBindingSafety(in *port.SafetyConstraint) *port.SafetyConstraint {
	if in == nil {
		return nil
	}
	out := *in
	out.MinValue = cloneFloatPtr(in.MinValue)
	out.MaxValue = cloneFloatPtr(in.MaxValue)
	out.MaxChangePerSecond = cloneFloatPtr(in.MaxChangePerSecond)
	return &out
}

func cloneFloatPtr(v *float64) *float64 {
	if v == nil {
		return nil
	}
	c := *v
	return &c
}
