package query

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/mushroomyuan/vpp-backend/api/contracts"
	"github.com/mushroomyuan/vpp-backend/platform/decorator"
	"github.com/mushroomyuan/vpp-backend/resource/domain/model"
	"github.com/mushroomyuan/vpp-backend/resource/domain/port"
)

var scopeUUIDPattern = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

type RequiredMetric struct {
	MetricID string
	Access   string
}

type ResolveScope struct {
	TenantID              string
	ScopeType             string
	ScopeID               string
	RequiredCapabilityIDs []string
	RequiredMetrics       []RequiredMetric
}

type ResolveScopeHandler decorator.QueryHandler[ResolveScope, *model.ResolvedScope]

type resolveScopeHandler struct {
	repo port.ScopeRepository
}

func NewResolveScopeHandler(
	repo port.ScopeRepository,
	metricClient decorator.MetricsClient,
) ResolveScopeHandler {
	if repo == nil {
		panic("NewResolveScopeHandler parameter repo is nil")
	}
	return decorator.ApplyQueryDecorators[ResolveScope, *model.ResolvedScope](
		resolveScopeHandler{repo: repo},
		metricClient,
	)
}

func (h resolveScopeHandler) Handle(ctx context.Context, q ResolveScope) (*model.ResolvedScope, error) {
	scopeType, capabilityIDs, metrics, err := normalizeResolveScope(q)
	if err != nil {
		return nil, err
	}
	snapshot, err := h.repo.Load(ctx, strings.TrimSpace(q.TenantID), strings.TrimSpace(q.ScopeID))
	if err != nil {
		return nil, err
	}
	return model.ResolveSnapshot(*snapshot, scopeType, capabilityIDs, metrics)
}

func normalizeResolveScope(
	q ResolveScope,
) (model.ScopeType, []contracts.CapabilityID, []model.MetricRequirement, error) {
	if strings.TrimSpace(q.TenantID) == "" {
		return "", nil, nil, fmt.Errorf("tenant_id is required")
	}
	scopeID := strings.TrimSpace(q.ScopeID)
	if !scopeUUIDPattern.MatchString(scopeID) {
		return "", nil, nil, fmt.Errorf("invalid scope id %q", q.ScopeID)
	}
	scopeType, err := model.ParseScopeType(q.ScopeType)
	if err != nil {
		return "", nil, nil, err
	}

	capabilityIDs := make([]contracts.CapabilityID, 0, len(q.RequiredCapabilityIDs))
	seenCapabilities := make(map[contracts.CapabilityID]struct{}, len(q.RequiredCapabilityIDs))
	for _, raw := range q.RequiredCapabilityIDs {
		capabilityID, err := contracts.ParseCapabilityID(strings.TrimSpace(raw))
		if err != nil {
			return "", nil, nil, fmt.Errorf("invalid capability id: %w", err)
		}
		if _, ok := seenCapabilities[capabilityID]; ok {
			return "", nil, nil, fmt.Errorf("invalid duplicate capability id %q", capabilityID)
		}
		seenCapabilities[capabilityID] = struct{}{}
		capabilityIDs = append(capabilityIDs, capabilityID)
	}

	metrics := make([]model.MetricRequirement, 0, len(q.RequiredMetrics))
	seenMetrics := make(map[string]struct{}, len(q.RequiredMetrics))
	for _, raw := range q.RequiredMetrics {
		metricID, err := contracts.ParseMetricID(strings.TrimSpace(raw.MetricID))
		if err != nil {
			return "", nil, nil, fmt.Errorf("invalid metric id: %w", err)
		}
		var access model.MetricAccessNeed
		switch strings.TrimSpace(raw.Access) {
		case string(model.MetricAccessNeedRead):
			access = model.MetricAccessNeedRead
		case string(model.MetricAccessNeedWrite):
			access = model.MetricAccessNeedWrite
		default:
			return "", nil, nil, fmt.Errorf("invalid metric access %q", raw.Access)
		}
		key := string(metricID) + ":" + string(access)
		if _, ok := seenMetrics[key]; ok {
			return "", nil, nil, fmt.Errorf("invalid duplicate metric requirement %s", key)
		}
		seenMetrics[key] = struct{}{}
		metrics = append(metrics, model.MetricRequirement{MetricID: metricID, Access: access})
	}
	return scopeType, capabilityIDs, metrics, nil
}
