package command

import (
	"context"
	"time"

	"github.com/mushroomyuan/vpp-backend/decision/domain/policy"
	"github.com/mushroomyuan/vpp-backend/platform/decorator"
)

// DisablePolicy turns a policy off. It does not call Resource.
type DisablePolicy struct {
	TenantID string
	ID       string
	Version  int64
}

// DisablePolicyHandler clears enabled under the optimistic version.
type DisablePolicyHandler = decorator.CommandHandler[DisablePolicy, *policy.Policy]

type disablePolicyHandler struct {
	repo policy.Repository
}

func NewDisablePolicyHandler(repo policy.Repository, metricsClient decorator.MetricsClient) DisablePolicyHandler {
	if repo == nil || metricsClient == nil {
		panic("NewDisablePolicyHandler: repository and metrics are required")
	}
	return decorator.ApplyCommandDecorators[DisablePolicy, *policy.Policy](
		disablePolicyHandler{repo: repo},
		metricsClient,
	)
}

func (h disablePolicyHandler) Handle(ctx context.Context, cmd DisablePolicy) (*policy.Policy, error) {
	current, err := loadAtVersion(ctx, h.repo, cmd.TenantID, cmd.ID, cmd.Version)
	if err != nil {
		return nil, err
	}
	if !current.Enabled {
		return current, nil
	}
	next := current.Clone()
	next.Enabled = false
	next.Version = current.Version + 1
	next.UpdatedAt = time.Now().UTC()
	next.UpdatedBy = actorFrom(ctx)
	if err := h.repo.Update(ctx, next, current.Version); err != nil {
		return nil, err
	}
	return h.repo.FindByID(ctx, next.TenantID, next.ID)
}
