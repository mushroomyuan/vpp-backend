package grpc

import (
	"context"

	resourcepb "github.com/mushroomyuan/vpp-backend/api/resource/proto/gen"
	"github.com/mushroomyuan/vpp-backend/resource/application/query"
)

func (s *Server) ResolveScope(
	ctx context.Context,
	req *resourcepb.ResolveScopeRequest,
) (*resourcepb.ResolveScopeResponse, error) {
	logIn(ctx, "resolve_scope")

	metrics := make([]query.RequiredMetric, 0, len(req.GetRequiredMetrics()))
	for _, metric := range req.GetRequiredMetrics() {
		metrics = append(metrics, query.RequiredMetric{
			MetricID: metric.GetMetricID(),
			Access:   metricAccessProtoToDomain(metric.GetAccess()),
		})
	}
	resolved, err := s.resolveScope.Handle(ctx, query.ResolveScope{
		TenantID:              req.GetTenantID(),
		ScopeType:             scopeTypeProtoToDomain(req.GetScopeType()),
		ScopeID:               req.GetScopeID(),
		RequiredCapabilityIDs: req.GetRequiredCapabilityIDs(),
		RequiredMetrics:       metrics,
	})
	if err != nil {
		return nil, toGRPCError(err)
	}
	return resolvedScopeToProto(resolved)
}
