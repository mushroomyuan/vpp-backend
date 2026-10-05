package command

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/mushroomyuan/vpp-backend/decision/domain/policy"
	"github.com/mushroomyuan/vpp-backend/platform/decorator"
)

// DeletePolicy soft-deletes one policy. The name can be reused afterwards.
type DeletePolicy struct {
	TenantID string
	ID       string
}

// DeletePolicyHandler marks a policy deleted.
type DeletePolicyHandler = decorator.CommandHandler[DeletePolicy, struct{}]

type deletePolicyHandler struct {
	repo policy.Repository
}

func NewDeletePolicyHandler(repo policy.Repository, metricsClient decorator.MetricsClient) DeletePolicyHandler {
	if repo == nil || metricsClient == nil {
		panic("NewDeletePolicyHandler: repository and metrics are required")
	}
	return decorator.ApplyCommandDecorators[DeletePolicy, struct{}](
		deletePolicyHandler{repo: repo},
		metricsClient,
	)
}

func (h deletePolicyHandler) Handle(ctx context.Context, cmd DeletePolicy) (struct{}, error) {
	tenantID := strings.TrimSpace(cmd.TenantID)
	id := strings.TrimSpace(cmd.ID)
	if tenantID == "" || id == "" {
		return struct{}{}, policy.Invalid(fmt.Errorf("policy: tenant_id and id are required"))
	}
	if _, err := h.repo.FindByID(ctx, tenantID, id); err != nil {
		return struct{}{}, err
	}
	return struct{}{}, h.repo.SoftDelete(ctx, tenantID, id, actorFrom(ctx), time.Now().UTC())
}
