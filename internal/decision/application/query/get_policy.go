package query

import (
	"context"
	"fmt"
	"strings"

	"github.com/mushroomyuan/vpp-backend/decision/domain/policy"
	"github.com/mushroomyuan/vpp-backend/platform/decorator"
)

// GetPolicy loads one live policy.
type GetPolicy struct {
	TenantID string
	ID       string
}

// GetPolicyHandler reads a policy by id.
type GetPolicyHandler = decorator.QueryHandler[GetPolicy, *policy.Policy]

type getPolicyHandler struct {
	repo policy.Repository
}

func NewGetPolicyHandler(repo policy.Repository, metricsClient decorator.MetricsClient) GetPolicyHandler {
	if repo == nil || metricsClient == nil {
		panic("NewGetPolicyHandler: repository and metrics are required")
	}
	return decorator.ApplyQueryDecorators[GetPolicy, *policy.Policy](
		getPolicyHandler{repo: repo},
		metricsClient,
	)
}

func (h getPolicyHandler) Handle(ctx context.Context, q GetPolicy) (*policy.Policy, error) {
	tenantID := strings.TrimSpace(q.TenantID)
	id := strings.TrimSpace(q.ID)
	if tenantID == "" || id == "" {
		return nil, policy.Invalid(fmt.Errorf("policy: tenant_id and id are required"))
	}
	return h.repo.FindByID(ctx, tenantID, id)
}
