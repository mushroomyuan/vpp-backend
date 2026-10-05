package command

import (
	"context"
	"fmt"
	"strings"

	"github.com/mushroomyuan/vpp-backend/decision/domain/policy"
	"github.com/mushroomyuan/vpp-backend/decision/domain/port"
)

// requireSOCScope calls Resource directly. Enable and update stay strict:
// a Resource failure is returned, not replaced by a cached scope.
func requireSOCScope(ctx context.Context, resources port.ResourcePort, p *policy.Policy) error {
	if resources == nil {
		return fmt.Errorf("resolve scope: resource port is required")
	}
	query, err := policy.SOCScopeQuery(p)
	if err != nil {
		return policy.Invalid(err)
	}
	resolved, err := resources.ResolveScope(ctx, query)
	if err != nil {
		return fmt.Errorf("resolve scope: %w", err)
	}
	if resolved.PrecheckOK {
		return nil
	}
	return &policy.PrecheckError{
		Failures:   append([]port.ScopePrecheckFailure(nil), resolved.PrecheckFailures...),
		Exclusions: append([]port.ScopeExclusion(nil), resolved.Exclusions...),
	}
}

func loadAtVersion(
	ctx context.Context,
	repo policy.Repository,
	tenantID, id string,
	version int64,
) (*policy.Policy, error) {
	tenantID = strings.TrimSpace(tenantID)
	id = strings.TrimSpace(id)
	if tenantID == "" || id == "" {
		return nil, policy.Invalid(fmt.Errorf("policy: tenant_id and id are required"))
	}
	if version <= 0 {
		return nil, policy.Invalid(fmt.Errorf("policy: version must be positive"))
	}
	current, err := repo.FindByID(ctx, tenantID, id)
	if err != nil {
		return nil, err
	}
	if current.Version != version {
		return nil, policy.ErrVersionConflict
	}
	return current, nil
}
