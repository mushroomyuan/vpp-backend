package command

import (
	"context"
	"time"

	"github.com/mushroomyuan/vpp-backend/decision/domain/policy"
	"github.com/mushroomyuan/vpp-backend/decision/domain/port"
	"github.com/mushroomyuan/vpp-backend/platform/decorator"
)

// EnablePolicy turns a policy on after ResolveScope accepts the scope.
type EnablePolicy struct {
	TenantID string
	ID       string
	Version  int64
}

// EnablePolicyHandler prechecks, then sets enabled.
type EnablePolicyHandler = decorator.CommandHandler[EnablePolicy, *policy.Policy]

type enablePolicyHandler struct {
	repo     policy.Repository
	resource port.ResourcePort
}

func NewEnablePolicyHandler(repo policy.Repository, resource port.ResourcePort, metricsClient decorator.MetricsClient) EnablePolicyHandler {
	if repo == nil || resource == nil || metricsClient == nil {
		panic("NewEnablePolicyHandler: repository, resource port, and metrics are required")
	}
	return decorator.ApplyCommandDecorators[EnablePolicy, *policy.Policy](
		enablePolicyHandler{repo: repo, resource: resource},
		metricsClient,
	)
}

func (h enablePolicyHandler) Handle(ctx context.Context, cmd EnablePolicy) (*policy.Policy, error) {
	current, err := loadAtVersion(ctx, h.repo, cmd.TenantID, cmd.ID, cmd.Version)
	if err != nil {
		return nil, err
	}
	if err := requireSOCScope(ctx, h.resource, current); err != nil {
		return nil, err
	}
	if current.Enabled {
		return current, nil
	}
	next := current.Clone()
	next.Enabled = true
	next.Version = current.Version + 1
	next.UpdatedAt = time.Now().UTC()
	next.UpdatedBy = actorFrom(ctx)
	if err := h.repo.Update(ctx, next, current.Version); err != nil {
		return nil, err
	}
	return h.repo.FindByID(ctx, next.TenantID, next.ID)
}
