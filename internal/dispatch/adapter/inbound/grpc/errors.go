package grpc

import (
	"errors"
	"strings"

	"github.com/mushroomyuan/vpp-backend/dispatch/domain"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func toGRPCError(err error) error {
	if err == nil {
		return nil
	}
	switch {
	case errors.Is(err, domain.ErrTaskNotFound):
		return status.Error(codes.NotFound, err.Error())
	case errors.Is(err, domain.ErrCommandNotFound):
		return status.Error(codes.NotFound, err.Error())
	default:
		msg := err.Error()
		lower := strings.ToLower(msg)
		if strings.Contains(lower, "required") ||
			strings.Contains(lower, "invalid") ||
			strings.Contains(lower, "must ") {
			return status.Error(codes.InvalidArgument, msg)
		}
		return status.Error(codes.Internal, msg)
	}
}
