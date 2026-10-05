// Package memory is an in-memory policy repository for tests.
package memory

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/mushroomyuan/vpp-backend/decision/domain/policy"
)

// PolicyRepository stores policies in process memory.
type PolicyRepository struct {
	mu   sync.Mutex
	rows map[string]*stored
}

type stored struct {
	policy  *policy.Policy
	deleted bool
}

// NewPolicyRepository returns an empty repository.
func NewPolicyRepository() *PolicyRepository {
	return &PolicyRepository{rows: map[string]*stored{}}
}

var _ policy.Repository = (*PolicyRepository)(nil)

func (r *PolicyRepository) Create(_ context.Context, p *policy.Policy) error {
	if err := p.Validate(); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.nameTaken(p.TenantID, p.Name, "") {
		return policy.ErrNameTaken
	}
	key := rowKey(p.TenantID, p.ID)
	if existing, ok := r.rows[key]; ok && !existing.deleted {
		return policy.ErrNameTaken
	}
	r.rows[key] = &stored{policy: p.Clone()}
	return nil
}

func (r *PolicyRepository) Update(_ context.Context, p *policy.Policy, expectedVersion int64) error {
	if err := p.Validate(); err != nil {
		return err
	}
	if p.Version != expectedVersion+1 {
		return fmt.Errorf("policy: stored version must advance from %d to %d", expectedVersion, expectedVersion+1)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	row, ok := r.rows[rowKey(p.TenantID, p.ID)]
	if !ok || row.deleted {
		return policy.ErrNotFound
	}
	if row.policy.Version != expectedVersion {
		return policy.ErrVersionConflict
	}
	if r.nameTaken(p.TenantID, p.Name, p.ID) {
		return policy.ErrNameTaken
	}
	row.policy = p.Clone()
	return nil
}

func (r *PolicyRepository) SoftDelete(_ context.Context, tenantID, id, actor string, at time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	row, ok := r.rows[rowKey(tenantID, id)]
	if !ok || row.deleted {
		return policy.ErrNotFound
	}
	next := row.policy.Clone()
	next.UpdatedAt = at
	next.UpdatedBy = actor
	row.policy = next
	row.deleted = true
	return nil
}

func (r *PolicyRepository) FindByID(_ context.Context, tenantID, id string) (*policy.Policy, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	row, ok := r.rows[rowKey(tenantID, id)]
	if !ok || row.deleted {
		return nil, policy.ErrNotFound
	}
	return row.policy.Clone(), nil
}

func (r *PolicyRepository) List(_ context.Context, filter policy.ListFilter) ([]*policy.Policy, error) {
	if filter.TenantID == "" {
		return nil, policy.Invalid(fmt.Errorf("policy: tenant_id is required"))
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]*policy.Policy, 0)
	for _, row := range r.rows {
		if row.deleted || row.policy.TenantID != filter.TenantID {
			continue
		}
		if filter.EnabledOnly && !row.policy.Enabled {
			continue
		}
		out = append(out, row.policy.Clone())
	}
	sortPolicies(out)
	return out, nil
}

func (r *PolicyRepository) ListEnabled(_ context.Context) ([]*policy.Policy, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]*policy.Policy, 0)
	for _, row := range r.rows {
		if row.deleted || !row.policy.Enabled {
			continue
		}
		out = append(out, row.policy.Clone())
	}
	sortPolicies(out)
	return out, nil
}

func (r *PolicyRepository) nameTaken(tenantID, name, exceptID string) bool {
	for _, row := range r.rows {
		if row.deleted || row.policy.TenantID != tenantID || row.policy.ID == exceptID {
			continue
		}
		if row.policy.Name == name {
			return true
		}
	}
	return false
}

func sortPolicies(out []*policy.Policy) {
	sort.Slice(out, func(i, j int) bool {
		if out[i].TenantID != out[j].TenantID {
			return out[i].TenantID < out[j].TenantID
		}
		if out[i].Name != out[j].Name {
			return out[i].Name < out[j].Name
		}
		return out[i].ID < out[j].ID
	})
}

func rowKey(tenantID, id string) string {
	return tenantID + "\x00" + id
}
