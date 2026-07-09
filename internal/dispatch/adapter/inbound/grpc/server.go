package grpc

import (
	dispatchpb "github.com/mushroomyuan/vpp-backend/api/dispatch/proto/gen"
	"github.com/mushroomyuan/vpp-backend/dispatch/application"
	"github.com/mushroomyuan/vpp-backend/dispatch/application/command"
	"github.com/mushroomyuan/vpp-backend/dispatch/application/query"
)

// Server implements dispatchpb.DispatchServiceServer.
type Server struct {
	dispatchpb.UnimplementedDispatchServiceServer

	submitTask command.SubmitTaskHandler
	getTask    query.GetTaskHandler
}

func NewServer(app application.Application) *Server {
	return &Server{
		submitTask: app.Commands.SubmitTask,
		getTask:    app.Queries.GetTask,
	}
}
