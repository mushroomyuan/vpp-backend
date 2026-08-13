package server

import (
	"net"

	grpc_logrus "github.com/grpc-ecosystem/go-grpc-middleware/logging/logrus"
	grpc_tags "github.com/grpc-ecosystem/go-grpc-middleware/tags"
	"github.com/mushroomyuan/vpp-backend/platform/logging"
	"github.com/sirupsen/logrus"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
)

func init() {
	logger := logrus.New()
	logger.SetLevel(logrus.WarnLevel)
	grpc_logrus.ReplaceGrpcLogger(logrus.NewEntry(logger))
}

// NewGRPCServer returns a *grpc.Server pre-configured with the standard
// interceptor stack: OpenTelemetry tracing, grpc_tags, structured logging.
// The caller registers services and owns the Serve / GracefulStop lifecycle.
//
// The standard gRPC health checking protocol (grpc.health.v1.Health) is
// registered and reported SERVING as soon as the server is constructed —
// this process is up and its interceptor chain is wired, which is all a
// liveness probe should assert. Readiness (e.g. degraded-but-serving states
// under authz fail-closed tiers) is intentionally out of scope for now; add
// per-service status transitions via health.NewServer().SetServingStatus if
// that granularity becomes necessary.
func NewGRPCServer(extraUnary ...grpc.UnaryServerInterceptor) *grpc.Server {
	logrusEntry := logrus.NewEntry(logrus.StandardLogger())
	unary := []grpc.UnaryServerInterceptor{
		grpc_tags.UnaryServerInterceptor(grpc_tags.WithFieldExtractor(grpc_tags.CodeGenRequestFieldExtractor)),
		grpc_logrus.UnaryServerInterceptor(logrusEntry),
		logging.GRPCUnaryInterceptor,
	}
	unary = append(unary, extraUnary...)
	grpcServer := grpc.NewServer(
		grpc.StatsHandler(otelgrpc.NewServerHandler()),
		grpc.ChainUnaryInterceptor(unary...),
		grpc.ChainStreamInterceptor(
			grpc_tags.StreamServerInterceptor(grpc_tags.WithFieldExtractor(grpc_tags.CodeGenRequestFieldExtractor)),
			grpc_logrus.StreamServerInterceptor(logrusEntry),
		),
	)

	healthSrv := health.NewServer()
	healthSrv.SetServingStatus("", healthpb.HealthCheckResponse_SERVING)
	healthpb.RegisterHealthServer(grpcServer, healthSrv)

	return grpcServer
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
