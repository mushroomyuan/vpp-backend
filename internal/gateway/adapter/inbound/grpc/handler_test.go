package grpc

import (
	"errors"
	"strings"
	"testing"

	gatewaypb "github.com/mushroomyuan/vpp-backend/api/gateway/proto/gen"
	"github.com/mushroomyuan/vpp-backend/gateway/domain"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestProtoValueToFloat(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		req  *gatewaypb.ExecuteCommandRequest
		want float64
		err  bool
	}{
		{"bool true", &gatewaypb.ExecuteCommandRequest{Value: &gatewaypb.ExecuteCommandRequest_BoolValue{BoolValue: true}}, 1, false},
		{"bool false", &gatewaypb.ExecuteCommandRequest{Value: &gatewaypb.ExecuteCommandRequest_BoolValue{BoolValue: false}}, 0, false},
		{"int", &gatewaypb.ExecuteCommandRequest{Value: &gatewaypb.ExecuteCommandRequest_IntValue{IntValue: 42}}, 42, false},
		{"float", &gatewaypb.ExecuteCommandRequest{Value: &gatewaypb.ExecuteCommandRequest_FloatValue{FloatValue: 3.5}}, 3.5, false},
		{"string", &gatewaypb.ExecuteCommandRequest{Value: &gatewaypb.ExecuteCommandRequest_StringValue{StringValue: "x"}}, 0, true},
		{"nil", &gatewaypb.ExecuteCommandRequest{}, 0, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := protoValueToFloat(tc.req)
			if tc.err {
				if err == nil {
					t.Fatal("want error")
				}
				return
			}
			if err != nil || got != tc.want {
				t.Fatalf("got %v err=%v want %v", got, err, tc.want)
			}
		})
	}
}

func TestToGRPCError(t *testing.T) {
	t.Parallel()

	if toGRPCError(nil) != nil {
		t.Fatal("nil")
	}

	cases := []struct {
		err  error
		code codes.Code
	}{
		{domain.ErrMappingNotFound, codes.NotFound},
		{domain.ErrMappingDisabled, codes.FailedPrecondition},
		{domain.ErrMappingConflict, codes.AlreadyExists},
		{errors.New("tenant_id is required"), codes.InvalidArgument},
		{errors.New("invalid telemetry"), codes.InvalidArgument},
		{errors.New("boom"), codes.Internal},
	}
	for _, tc := range cases {
		st, ok := status.FromError(toGRPCError(tc.err))
		if !ok || st.Code() != tc.code {
			t.Fatalf("%v → %v want %v", tc.err, st, tc.code)
		}
	}

	wrapped := errors.Join(errors.New("wrap"), domain.ErrMappingNotFound)
	st, _ := status.FromError(toGRPCError(wrapped))
	if st.Code() != codes.NotFound {
		t.Fatalf("wrapped NotFound → %v", st.Code())
	}
	if !strings.Contains(st.Message(), "device mapping not found") {
		t.Fatalf("message = %q", st.Message())
	}
}
