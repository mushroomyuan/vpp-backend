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
	cancelTask command.CancelTaskHandler
	getTask    query.GetTaskHandler
}

func NewServer(app application.Application) *Server {
	return &Server{
		submitTask: app.Commands.SubmitTask,
		cancelTask: app.Commands.CancelTask,
		getTask:    app.Queries.GetTask,
	}
}
