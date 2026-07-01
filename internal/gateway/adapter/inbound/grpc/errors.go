package grpc

import (
	"errors"
	"strings"

	"github.com/mushroomyuan/vpp-backend/gateway/domain"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func toGRPCError(err error) error {
	if err == nil {
		return nil
	}
	switch {
	case errors.Is(err, domain.ErrMappingNotFound):
		return status.Error(codes.NotFound, err.Error())
	case errors.Is(err, domain.ErrMappingDisabled):
		return status.Error(codes.FailedPrecondition, err.Error())
	case errors.Is(err, domain.ErrMappingConflict):
		return status.Error(codes.AlreadyExists, err.Error())
	default:
		msg := err.Error()
		lower := strings.ToLower(msg)
		if strings.Contains(lower, "required") ||
			strings.Contains(lower, "invalid") {
			return status.Error(codes.InvalidArgument, msg)
		}
		return status.Error(codes.Internal, msg)
	}
}
