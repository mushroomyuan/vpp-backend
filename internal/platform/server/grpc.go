package server

import (
	"net"

	grpc_logrus "github.com/grpc-ecosystem/go-grpc-middleware/logging/logrus"
	grpc_tags "github.com/grpc-ecosystem/go-grpc-middleware/tags"
	"github.com/mushroomyuan/vpp-backend/platform/logging"
	"github.com/sirupsen/logrus"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"google.golang.org/grpc"
)

func init() {
	logger := logrus.New()
	logger.SetLevel(logrus.WarnLevel)
	grpc_logrus.ReplaceGrpcLogger(logrus.NewEntry(logger))
}

// NewGRPCServer returns a *grpc.Server pre-configured with the standard
// interceptor stack: OpenTelemetry tracing, grpc_tags, structured logging.
// The caller registers services and owns the Serve / GracefulStop lifecycle.
func NewGRPCServer() *grpc.Server {
	logrusEntry := logrus.NewEntry(logrus.StandardLogger())
	return grpc.NewServer(
		grpc.StatsHandler(otelgrpc.NewServerHandler()),
		grpc.ChainUnaryInterceptor(
			grpc_tags.UnaryServerInterceptor(grpc_tags.WithFieldExtractor(grpc_tags.CodeGenRequestFieldExtractor)),
			grpc_logrus.UnaryServerInterceptor(logrusEntry),
			logging.GRPCUnaryInterceptor,
		),
		grpc.ChainStreamInterceptor(
			grpc_tags.StreamServerInterceptor(grpc_tags.WithFieldExtractor(grpc_tags.CodeGenRequestFieldExtractor)),
			grpc_logrus.StreamServerInterceptor(logrusEntry),
		),
	)
}

// RunGRPCServerOnAddr is a convenience wrapper for simple services that do not
// need graceful shutdown. It builds a server via NewGRPCServer, calls
// registerServer to mount services, then blocks on Serve.
func RunGRPCServerOnAddr(addr string, registerServer func(server *grpc.Server)) {
	grpcServer := NewGRPCServer()
	registerServer(grpcServer)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		logrus.Panic(err)
	}
	logrus.Infof("Starting gRPC Server,Listening:%s", addr)
	if err := grpcServer.Serve(listener); err != nil {
		logrus.Panic(err)
	}
}
