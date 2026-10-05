package context

import (
	stdctx "context"
	"errors"
	"testing"
	"time"

	"github.com/mushroomyuan/vpp-backend/decision/domain/port"
)

func TestCachingScopeResolver_ReusesFreshAndFallsBack(t *testing.T) {
	now := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	clock := func() time.Time { return now }
	resource := &fakeResource{scope: okScope()}
	obs := &countObserver{}
	resolver := NewCachingScopeResolver(CachingResolverConfig{
		Resource: resource,
		TTL:      time.Minute,
		MaxAge:   5 * time.Minute,
		Now:      func() time.Time { return clock() },
		Observer: obs,
	})
	query := port.ScopeQuery{TenantID: "tenant-1", ScopeType: port.ScopeAsset, ScopeID: "asset-1"}

	first, err := resolver.Resolve(stdctx.Background(), query)
	if err != nil || first.ResourceRevision != "rev-1" || resource.calls != 1 {
		t.Fatalf("first = %+v calls %d err %v", first.ResourceRevision, resource.calls, err)
	}
	first.ResourceRevision = "mutated"
	second, err := resolver.Resolve(stdctx.Background(), query)
	if err != nil || second.ResourceRevision != "rev-1" || resource.calls != 1 {
		t.Fatalf("cache = %+v calls %d err %v", second.ResourceRevision, resource.calls, err)
	}

	now = now.Add(time.Minute + time.Millisecond)
	resource.err = errors.New("resource down")
	fallback, err := resolver.Resolve(stdctx.Background(), query)
	if err != nil || fallback.ResourceRevision != "rev-1" || resource.calls != 2 {
		t.Fatalf("fallback = %+v calls %d err %v", fallback.ResourceRevision, resource.calls, err)
	}

	now = now.Add(5 * time.Minute)
	_, err = resolver.Resolve(stdctx.Background(), query)
	if !errors.Is(err, ErrScopeUnavailable) {
		t.Fatalf("expired err = %v", err)
	}
	if obs.counts[ResolveCache] != 1 || obs.counts[ResolveFresh] != 1 || obs.counts[ResolveFallback] != 1 || obs.counts[ResolveSkipped] != 1 {
		t.Fatalf("observations = %+v", obs.counts)
	}
}

func TestCachingScopeResolver_PrecheckFailureDropsCache(t *testing.T) {
	now := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	resource := &fakeResource{scope: okScope()}
	resolver := NewCachingScopeResolver(CachingResolverConfig{
		Resource: resource,
		TTL:      time.Minute,
		MaxAge:   5 * time.Minute,
		Now:      func() time.Time { return now },
	})
	query := port.ScopeQuery{TenantID: "tenant-1", ScopeType: port.ScopeAsset, ScopeID: "asset-1"}
	if _, err := resolver.Resolve(stdctx.Background(), query); err != nil {
		t.Fatal(err)
	}
	failed := okScope()
	failed.PrecheckOK = false
	resource.scope = failed
	now = now.Add(time.Minute + time.Millisecond)
	got, err := resolver.Resolve(stdctx.Background(), query)
	if err != nil || got.PrecheckOK {
		t.Fatalf("precheck result ok=%v err=%v", got.PrecheckOK, err)
	}
	resource.err = errors.New("resource down")
	now = now.Add(time.Millisecond)
	_, err = resolver.Resolve(stdctx.Background(), query)
	if !errors.Is(err, ErrScopeUnavailable) {
		t.Fatalf("err = %v, want unavailable after a failed precheck", err)
	}
}

type fakeResource struct {
	scope port.ResolvedScope
	err   error
	calls int
}

func (f *fakeResource) ResolveScope(stdctx.Context, port.ScopeQuery) (port.ResolvedScope, error) {
	f.calls++
	if f.err != nil {
		return port.ResolvedScope{}, f.err
	}
	return f.scope, nil
}

type countObserver struct {
	counts map[string]int
}

func (o *countObserver) ObserveScopeResolve(result string) {
	if o.counts == nil {
		o.counts = map[string]int{}
	}
	o.counts[result]++
}

func okScope() port.ResolvedScope {
	return port.ResolvedScope{
		ScopeType:        port.ScopeAsset,
		ScopeID:          "asset-1",
		ResourceRevision: "rev-1",
		PrecheckOK:       true,
		Members: []port.ResolvedCU{{
			CUID:    "cu-1",
			AssetID: "asset-1",
		}},
	}
}
