package grpc

import (
	"context"
	"fmt"

	gatewaypb "github.com/mushroomyuan/vpp-backend/api/gateway/proto/gen"
	"github.com/mushroomyuan/vpp-backend/gateway/application/command"
)

// ExecuteCommand handles dispatch → gateway → EMS control flow.
func (s *Server) ExecuteCommand(
	ctx context.Context,
	req *gatewaypb.ExecuteCommandRequest,
) (*gatewaypb.ExecuteCommandResponse, error) {
	value, err := protoValueToFloat(req)
	if err != nil {
		return nil, toGRPCError(err)
	}

	res, err := s.executeCommand.Handle(ctx, command.ExecuteCommand{
		CommandID: req.GetCommandID(),
		TenantID:  req.GetTenantID(),
		CUCode:    req.GetCUCode(),
		PointKey:  req.GetPointKey(),
		Value:     value,
	})
	if err != nil {
		return nil, toGRPCError(err)
	}
	return &gatewaypb.ExecuteCommandResponse{
		ExternalID:     res.ExternalID,
		ExternalSystem: res.ExternalSystem,
	}, nil
}

func protoValueToFloat(req *gatewaypb.ExecuteCommandRequest) (float64, error) {
	switch v := req.GetValue().(type) {
	case *gatewaypb.ExecuteCommandRequest_BoolValue:
		if v.BoolValue {
			return 1, nil
		}
		return 0, nil
	case *gatewaypb.ExecuteCommandRequest_IntValue:
		return float64(v.IntValue), nil
	case *gatewaypb.ExecuteCommandRequest_FloatValue:
		return v.FloatValue, nil
	case *gatewaypb.ExecuteCommandRequest_StringValue:
		return 0, fmt.Errorf("string_value is not supported by EMS v1 adapter")
	default:
		return 0, fmt.Errorf("value is required")
	}
}
