package binding

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

type fakeCatalog struct {
	mu       sync.Mutex
	bindings []Binding
	err      error
	calls    int
	started  chan struct{}
	release  chan struct{}
}

func (f *fakeCatalog) ListCUBindings(context.Context, string, string) ([]Binding, error) {
	f.mu.Lock()
	f.calls++
	bindings := append([]Binding(nil), f.bindings...)
	err := f.err
	started := f.started
	release := f.release
	f.mu.Unlock()
	if started != nil {
		select {
		case <-started:
		default:
			close(started)
		}
	}
	if release != nil {
		<-release
	}
	if err != nil {
		return nil, err
	}
	return bindings, nil
}

func (f *fakeCatalog) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

func testBinding() Binding {
	min := 0.0
	return Binding{
		PointID: "pt-1", MetricID: "electrical.active_power.v1",
		ExternalAddress: "reg_40001", AccessMode: AccessRead,
		Scale: 0.1, Offset: -1, Enabled: true, Revision: 3,
		Safety: &Safety{MinValue: &min, Version: 1},
	}
}

func TestCache_ReusesCopyInsideTTL(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC)
	cat := &fakeCatalog{bindings: []Binding{testBinding()}}
	cache := NewCache(CacheConfig{
		Catalog: cat, TTL: time.Second, MaxAge: time.Minute,
		Now: func() time.Time { return now },
	})

	first, origin, err := cache.Load(context.Background(), "tenant", "cu-1")
	if err != nil || origin != OriginFresh || len(first.Bindings) != 1 {
		t.Fatalf("first = %+v %s %v", first, origin, err)
	}
	first.Bindings[0].Scale = 99
	if first.Bindings[0].Safety != nil {
		*first.Bindings[0].Safety.MinValue = 50
	}

	second, origin, err := cache.Load(context.Background(), "tenant", "cu-1")
	if err != nil || origin != OriginCache {
		t.Fatalf("second origin %s err %v", origin, err)
	}
	if second.Bindings[0].Scale != 0.1 || *second.Bindings[0].Safety.MinValue != 0 {
		t.Fatalf("cache was mutated: %+v", second.Bindings[0])
	}
	if cat.callCount() != 1 {
		t.Fatalf("calls = %d", cat.callCount())
	}
}

func TestCache_RefreshesAfterTTL(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC)
	cat := &fakeCatalog{bindings: []Binding{testBinding()}}
	cache := NewCache(CacheConfig{
		Catalog: cat, TTL: time.Second, MaxAge: time.Minute,
		Now: func() time.Time { return now },
	})
	if _, _, err := cache.Load(context.Background(), "tenant", "cu-1"); err != nil {
		t.Fatal(err)
	}
	now = now.Add(2 * time.Second)
	cat.mu.Lock()
	cat.bindings[0].Revision = 4
	cat.mu.Unlock()

	got, origin, err := cache.Load(context.Background(), "tenant", "cu-1")
	if err != nil || origin != OriginFresh || got.Bindings[0].Revision != 4 {
		t.Fatalf("got rev %d origin %s err %v", got.Bindings[0].Revision, origin, err)
	}
	if cat.callCount() != 2 {
		t.Fatalf("calls = %d", cat.callCount())
	}
}

func TestCache_FallsBackInsideMaxAge(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC)
	cat := &fakeCatalog{bindings: []Binding{testBinding()}}
	cache := NewCache(CacheConfig{
		Catalog: cat, TTL: time.Second, MaxAge: time.Minute,
		Now: func() time.Time { return now },
	})
	if _, _, err := cache.Load(context.Background(), "tenant", "cu-1"); err != nil {
		t.Fatal(err)
	}
	now = now.Add(2 * time.Second)
	cat.mu.Lock()
	cat.err = errors.New("unavailable")
	cat.mu.Unlock()

	got, origin, err := cache.Load(context.Background(), "tenant", "cu-1")
	if err != nil || origin != OriginFallback || got.Bindings[0].Revision != 3 {
		t.Fatalf("origin %s err %v", origin, err)
	}
}

func TestCache_DropsExpiredCopy(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC)
	cat := &fakeCatalog{bindings: []Binding{testBinding()}}
	cache := NewCache(CacheConfig{
		Catalog: cat, TTL: time.Second, MaxAge: time.Minute,
		Now: func() time.Time { return now },
	})
	if _, _, err := cache.Load(context.Background(), "tenant", "cu-1"); err != nil {
		t.Fatal(err)
	}
	now = now.Add(2 * time.Minute)
	cat.mu.Lock()
	cat.err = errors.New("unavailable")
	cat.mu.Unlock()

	if _, _, err := cache.Load(context.Background(), "tenant", "cu-1"); err == nil {
		t.Fatal("want error after max age")
	}
}

func TestCache_InvalidateCUForcesReload(t *testing.T) {
	t.Parallel()
	cat := &fakeCatalog{bindings: []Binding{testBinding()}}
	cache := NewCache(CacheConfig{Catalog: cat, TTL: time.Minute, MaxAge: time.Hour})
	if _, _, err := cache.Load(context.Background(), "tenant", "cu-1"); err != nil {
		t.Fatal(err)
	}
	cache.InvalidateCU("tenant", "cu-1")
	cat.mu.Lock()
	cat.bindings[0].Revision = 9
	cat.mu.Unlock()

	got, origin, err := cache.Load(context.Background(), "tenant", "cu-1")
	if err != nil || origin != OriginFresh || got.Bindings[0].Revision != 9 {
		t.Fatalf("rev %d origin %s err %v", got.Bindings[0].Revision, origin, err)
	}
}

func TestCache_InvalidatePointDropsOnlyThatCU(t *testing.T) {
	t.Parallel()
	cat := &fakeCatalog{bindings: []Binding{testBinding()}}
	cache := NewCache(CacheConfig{Catalog: cat, TTL: time.Minute, MaxAge: time.Hour})
	if _, _, err := cache.Load(context.Background(), "tenant", "cu-1"); err != nil {
		t.Fatal(err)
	}
	other := testBinding()
	other.PointID = "pt-2"
	cat.mu.Lock()
	cat.bindings = []Binding{other}
	cat.mu.Unlock()
	if _, _, err := cache.Load(context.Background(), "tenant", "cu-2"); err != nil {
		t.Fatal(err)
	}

	cache.InvalidatePoint("tenant", "pt-1")
	if _, origin, err := cache.Load(context.Background(), "tenant", "cu-2"); err != nil || origin != OriginCache {
		t.Fatalf("other cu origin %s err %v", origin, err)
	}
	if _, origin, err := cache.Load(context.Background(), "tenant", "cu-1"); err != nil || origin != OriginFresh {
		t.Fatalf("invalidated cu origin %s err %v", origin, err)
	}
}

func TestCache_DoesNotStoreListThatRacesInvalidation(t *testing.T) {
	t.Parallel()
	release := make(chan struct{})
	cat := &fakeCatalog{
		bindings: []Binding{testBinding()},
		started:  make(chan struct{}),
		release:  release,
	}
	cache := NewCache(CacheConfig{Catalog: cat, TTL: time.Minute, MaxAge: time.Hour})

	errCh := make(chan error, 1)
	go func() {
		_, origin, err := cache.Load(context.Background(), "tenant", "cu-1")
		if err != nil || origin != OriginFresh {
			errCh <- errors.New("load failed")
			return
		}
		errCh <- nil
	}()
	<-cat.started
	cache.InvalidateCU("tenant", "cu-1")
	cat.mu.Lock()
	cat.bindings = []Binding{testBinding()}
	cat.bindings[0].Revision = 8
	cat.started = nil
	cat.release = nil
	cat.mu.Unlock()
	close(release)

	if err := <-errCh; err != nil {
		t.Fatal(err)
	}
	cat.mu.Lock()
	cat.err = errors.New("down")
	cat.mu.Unlock()
	if _, origin, err := cache.Load(context.Background(), "tenant", "cu-1"); err != nil || origin != OriginCache {
		t.Fatal("second attempt should have stored the post-invalidation list")
	}
	got, _, err := cache.Load(context.Background(), "tenant", "cu-1")
	if err != nil || got.Bindings[0].Revision != 8 {
		t.Fatalf("stored revision %d err %v", got.Bindings[0].Revision, err)
	}
}
