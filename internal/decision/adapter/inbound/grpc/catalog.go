package grpc

import decisionpb "github.com/mushroomyuan/vpp-backend/api/decision/proto/gen"

// CatalogOf maps DecisionService methods to permission pairs.
func CatalogOf(fullMethod string) (resource, action string, ok bool) {
	switch fullMethod {
	case decisionpb.DecisionService_GetPolicy_FullMethodName,
		decisionpb.DecisionService_ListPolicies_FullMethodName:
		return "decision:policies", "read", true
	case decisionpb.DecisionService_CreatePolicy_FullMethodName,
		decisionpb.DecisionService_UpdatePolicy_FullMethodName,
		decisionpb.DecisionService_DeletePolicy_FullMethodName,
		decisionpb.DecisionService_EnablePolicy_FullMethodName,
		decisionpb.DecisionService_DisablePolicy_FullMethodName:
		return "decision:policies", "write", true
	default:
		return "", "", false
	}
}
