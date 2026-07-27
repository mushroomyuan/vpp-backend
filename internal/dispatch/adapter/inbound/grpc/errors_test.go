package grpc

import (
	"errors"
	"testing"

	"github.com/mushroomyuan/vpp-backend/dispatch/domain"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestToGRPCError(t *testing.T) {
	t.Parallel()
	if toGRPCError(nil) != nil {
		t.Fatal("nil")
	}
	cases := []struct {
		err  error
		code codes.Code
	}{
		{domain.ErrTaskNotFound, codes.NotFound},
		{domain.ErrCommandNotFound, codes.NotFound},
		{errors.New("tenant_id is required"), codes.InvalidArgument},
		{errors.New("invalid command value"), codes.InvalidArgument},
		{errors.New("action must have commands"), codes.InvalidArgument},
		{errors.New("boom"), codes.Internal},
	}
	for _, tc := range cases {
		st, ok := status.FromError(toGRPCError(tc.err))
		if !ok || st.Code() != tc.code {
			t.Fatalf("%v → %v want %v", tc.err, st, tc.code)
		}
	}
}
