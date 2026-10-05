package memory

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/mushroomyuan/vpp-backend/decision/domain/policy"
)

// CooldownStore is the in-memory CooldownStore used by unit tests.
// The service uses the Postgres store, which competes through the plan transaction.
type CooldownStore struct {
	mu    sync.Mutex
	until map[policy.CooldownKey]time.Time
}

// NewCooldownStore returns an empty store.
func NewCooldownStore() *CooldownStore {
	return &CooldownStore{until: map[policy.CooldownKey]time.Time{}}
}

var _ policy.CooldownStore = (*CooldownStore)(nil)

// CoolingDown reports whether now is still before the exclusive end of the window.
func (s *CooldownStore) CoolingDown(_ context.Context, key policy.CooldownKey, now time.Time) (bool, error) {
	key = normalizeCooldown(key)
	if err := key.Validate(); err != nil {
		return false, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	until, ok := s.until[key]
	return ok && now.Before(until), nil
}

// MarkTriggered stores the exclusive end of the cooldown window.
func (s *CooldownStore) MarkTriggered(_ context.Context, key policy.CooldownKey, until time.Time) error {
	key = normalizeCooldown(key)
	if err := key.Validate(); err != nil {
		return err
	}
	if until.IsZero() {
		return policy.Invalid(fmt.Errorf("policy: cooldown until is required"))
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.until[key] = until
	return nil
}

func normalizeCooldown(key policy.CooldownKey) policy.CooldownKey {
	key.TenantID = strings.TrimSpace(key.TenantID)
	key.PolicyID = strings.TrimSpace(key.PolicyID)
	return key
}
