package grpc

import (
	"errors"
	"fmt"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	decisionpb "github.com/mushroomyuan/vpp-backend/api/decision/proto/gen"
	"github.com/mushroomyuan/vpp-backend/decision/domain/policy"
	"github.com/mushroomyuan/vpp-backend/decision/domain/port"
)

func errNilRequest() error {
	return policy.Invalid(fmt.Errorf("policy: request is required"))
}

func toGRPCError(err error) error {
	if err == nil {
		return nil
	}
	var precheck *policy.PrecheckError
	switch {
	case errors.As(err, &precheck):
		return precheckStatus(precheck)
	case errors.Is(err, policy.ErrNotFound):
		return status.Error(codes.NotFound, err.Error())
	case errors.Is(err, policy.ErrVersionConflict):
		return status.Error(codes.Aborted, err.Error())
	case errors.Is(err, policy.ErrNameTaken):
		return status.Error(codes.AlreadyExists, err.Error())
	case errors.Is(err, policy.ErrInvalid):
		return status.Error(codes.InvalidArgument, err.Error())
	default:
		return status.Error(codes.Internal, err.Error())
	}
}

func precheckStatus(err *policy.PrecheckError) error {
	st := status.New(codes.FailedPrecondition, err.Error())
	detail := &decisionpb.PolicyPrecheckFailure{
		Failures:   failureIssues(err.Failures),
		Exclusions: exclusionIssues(err.Exclusions),
	}
	withDetails, detailErr := st.WithDetails(detail)
	if detailErr != nil {
		return st.Err()
	}
	return withDetails.Err()
}

func failureIssues(items []port.ScopePrecheckFailure) []*decisionpb.PolicyPrecheckIssue {
	out := make([]*decisionpb.PolicyPrecheckIssue, 0, len(items))
	for _, item := range items {
		out = append(out, &decisionpb.PolicyPrecheckIssue{
			CUID:    item.CUID,
			AssetID: item.AssetID,
			Reason:  string(item.Reason),
			Detail:  item.Detail,
		})
	}
	return out
}

func exclusionIssues(items []port.ScopeExclusion) []*decisionpb.PolicyPrecheckIssue {
	out := make([]*decisionpb.PolicyPrecheckIssue, 0, len(items))
	for _, item := range items {
		out = append(out, &decisionpb.PolicyPrecheckIssue{
			CUID:    item.CUID,
			AssetID: item.AssetID,
			Reason:  string(item.Reason),
			Detail:  item.Detail,
		})
	}
	return out
}
