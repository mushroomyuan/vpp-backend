package command

import (
	"context"
	"time"

	"github.com/mushroomyuan/vpp-backend/decision/domain/policy"
	"github.com/mushroomyuan/vpp-backend/decision/domain/port"
	"github.com/mushroomyuan/vpp-backend/platform/decorator"
)

// UpdatePolicy replaces name, scope, cooldown, and the SOC spec.
// Kind and Enabled stay as stored. An enabled policy is prechecked against
// the replacement; failure leaves the row unchanged.
type UpdatePolicy struct {
	TenantID string
	ID       string
	Version  int64
	Name     string
	Scope    policy.TargetScope
	Cooldown time.Duration
	SOC      *policy.SOCThresholdSpec
}

// UpdatePolicyHandler applies an optimistic update.
type UpdatePolicyHandler = decorator.CommandHandler[UpdatePolicy, *policy.Policy]

type updatePolicyHandler struct {
	repo     policy.Repository
	resource port.ResourcePort
}

func NewUpdatePolicyHandler(repo policy.Repository, resource port.ResourcePort, metricsClient decorator.MetricsClient) UpdatePolicyHandler {
	if repo == nil || resource == nil || metricsClient == nil {
		panic("NewUpdatePolicyHandler: repository, resource port, and metrics are required")
	}
	return decorator.ApplyCommandDecorators[UpdatePolicy, *policy.Policy](
		updatePolicyHandler{repo: repo, resource: resource},
		metricsClient,
	)
}

func (h updatePolicyHandler) Handle(ctx context.Context, cmd UpdatePolicy) (*policy.Policy, error) {
	current, err := loadAtVersion(ctx, h.repo, cmd.TenantID, cmd.ID, cmd.Version)
	if err != nil {
		return nil, err
	}
	next := current.Clone()
	next.Name = cmd.Name
	next.Scope = policy.TargetScope{Type: cmd.Scope.Type, ID: cmd.Scope.ID}
	next.Cooldown = cmd.Cooldown
	if cmd.SOC != nil {
		copied := *cmd.SOC
		next.SOC = &copied
	} else {
		next.SOC = nil
	}
	next.Version = current.Version + 1
	next.UpdatedAt = time.Now().UTC()
	next.UpdatedBy = actorFrom(ctx)
	if err := next.Validate(); err != nil {
		return nil, policy.Invalid(err)
	}
	if next.Enabled {
		if err := requireSOCScope(ctx, h.resource, next); err != nil {
			return nil, err
		}
	}
	if err := h.repo.Update(ctx, next, current.Version); err != nil {
		return nil, err
	}
	return h.repo.FindByID(ctx, next.TenantID, next.ID)
}
