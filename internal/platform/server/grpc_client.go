package server

import (
	"fmt"

	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// DialGRPC dials addr with insecure credentials and OpenTelemetry client
// instrumentation. Outbound RPCs create client spans and propagate W3C/B3
// trace context to the server (which already uses otelgrpc.NewServerHandler).
func DialGRPC(addr string, extra ...grpc.DialOption) (*grpc.ClientConn, error) {
	if addr == "" {
		return nil, fmt.Errorf("grpc dial: addr is required")
	}
	opts := []grpc.DialOption{
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithStatsHandler(otelgrpc.NewClientHandler()),
	}
	opts = append(opts, extra...)
	conn, err := grpc.NewClient(addr, opts...)
	if err != nil {
		return nil, fmt.Errorf("grpc dial %s: %w", addr, err)
	}
	return conn, nil
}
