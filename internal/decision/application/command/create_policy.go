package command

import (
	"context"
	"time"

	"github.com/mushroomyuan/vpp-backend/decision/domain/policy"
	"github.com/mushroomyuan/vpp-backend/platform/decorator"
	"github.com/mushroomyuan/vpp-backend/platform/idgen"
)

// CreatePolicy starts a disabled policy. Enable is a separate call so the
// scope precheck cannot be skipped.
type CreatePolicy struct {
	TenantID string
	Name     string
	Kind     policy.Kind
	Scope    policy.TargetScope
	Cooldown time.Duration
	SOC      *policy.SOCThresholdSpec
}

// CreatePolicyHandler stores a new policy.
type CreatePolicyHandler = decorator.CommandHandler[CreatePolicy, *policy.Policy]

type createPolicyHandler struct {
	repo policy.Repository
}

func NewCreatePolicyHandler(repo policy.Repository, metricsClient decorator.MetricsClient) CreatePolicyHandler {
	if repo == nil || metricsClient == nil {
		panic("NewCreatePolicyHandler: repository and metrics are required")
	}
	return decorator.ApplyCommandDecorators[CreatePolicy, *policy.Policy](
		createPolicyHandler{repo: repo},
		metricsClient,
	)
}

func (h createPolicyHandler) Handle(ctx context.Context, cmd CreatePolicy) (*policy.Policy, error) {
	var spec *policy.SOCThresholdSpec
	if cmd.SOC != nil {
		copied := *cmd.SOC
		spec = &copied
	}
	created, err := policy.NewPolicy(policy.NewPolicyParams{
		ID:       idgen.Must(),
		TenantID: cmd.TenantID,
		Name:     cmd.Name,
		Kind:     cmd.Kind,
		Scope:    cmd.Scope,
		Enabled:  false,
		Cooldown: cmd.Cooldown,
		SOC:      spec,
	})
	if err != nil {
		return nil, policy.Invalid(err)
	}
	now := time.Now().UTC()
	actor := actorFrom(ctx)
	created.CreatedAt = now
	created.UpdatedAt = now
	created.CreatedBy = actor
	created.UpdatedBy = actor
	if err := h.repo.Create(ctx, created); err != nil {
		return nil, err
	}
	return h.repo.FindByID(ctx, created.TenantID, created.ID)
}
