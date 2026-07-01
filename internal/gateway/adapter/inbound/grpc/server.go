package grpc

import (
	gatewaypb "github.com/mushroomyuan/vpp-backend/api/gateway/proto/gen"
	"github.com/mushroomyuan/vpp-backend/gateway/application"
	"github.com/mushroomyuan/vpp-backend/gateway/application/command"
)

// Server implements gatewaypb.GatewayServiceServer.
type Server struct {
	gatewaypb.UnimplementedGatewayServiceServer

	executeCommand command.ExecuteCommandHandler
}

func NewServer(app application.Application) *Server {
	return &Server{
		executeCommand: app.Commands.ExecuteCommand,
	}
}
