package grpc

import (
	"context"

	gatewaypb "github.com/mushroomyuan/vpp-backend/api/gateway/proto/gen"
	"github.com/mushroomyuan/vpp-backend/gateway/application/command"
)

// ExecuteCommand handles dispatch → gateway → EMS control flow.
func (s *Server) ExecuteCommand(
	ctx context.Context,
	req *gatewaypb.ExecuteCommandRequest,
) (*gatewaypb.ExecuteCommandResponse, error) {
	res, err := s.executeCommand.Handle(ctx, command.ExecuteCommand{
		TenantID: req.GetTenantID(),
		CUCode:   req.GetCUCode(),
		Command:  req.GetCommand(),
		Value:    req.GetValue(),
	})
	if err != nil {
		return nil, toGRPCError(err)
	}
	return &gatewaypb.ExecuteCommandResponse{
		ExternalID:     res.ExternalID,
		ExternalSystem: res.ExternalSystem,
	}, nil
}
