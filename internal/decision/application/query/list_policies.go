package query

import (
	"context"
	"fmt"
	"strings"

	"github.com/mushroomyuan/vpp-backend/decision/domain/policy"
	"github.com/mushroomyuan/vpp-backend/platform/decorator"
)

// ListPolicies returns live policies for one tenant, optionally only enabled ones.
type ListPolicies struct {
	TenantID    string
	EnabledOnly bool
}

// ListPoliciesHandler lists policies.
type ListPoliciesHandler = decorator.QueryHandler[ListPolicies, []*policy.Policy]

type listPoliciesHandler struct {
	repo policy.Repository
}

func NewListPoliciesHandler(repo policy.Repository, metricsClient decorator.MetricsClient) ListPoliciesHandler {
	if repo == nil || metricsClient == nil {
		panic("NewListPoliciesHandler: repository and metrics are required")
	}
	return decorator.ApplyQueryDecorators[ListPolicies, []*policy.Policy](
		listPoliciesHandler{repo: repo},
		metricsClient,
	)
}

func (h listPoliciesHandler) Handle(ctx context.Context, q ListPolicies) ([]*policy.Policy, error) {
	tenantID := strings.TrimSpace(q.TenantID)
	if tenantID == "" {
		return nil, policy.Invalid(fmt.Errorf("policy: tenant_id is required"))
	}
	return h.repo.List(ctx, policy.ListFilter{TenantID: tenantID, EnabledOnly: q.EnabledOnly})
}
