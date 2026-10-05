package ports

import (
	"context"
	"net/http"
	"net/textproto"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	decisionpb "github.com/mushroomyuan/vpp-backend/api/decision/proto/gen"
)

// Mount dials the local Decision gRPC server and exposes the same RPCs over HTTP.
// /healthz stays on the Gin engine; unmatched paths go to the gateway.
// Dialing (instead of an in-process server) keeps requests on the gRPC interceptor chain.
func Mount(ctx context.Context, r *gin.Engine, grpcTarget string) error {
	mux := runtime.NewServeMux(runtime.WithIncomingHeaderMatcher(incomingHeaderMatcher))
	opts := []grpc.DialOption{grpc.WithTransportCredentials(insecure.NewCredentials())}
	if err := decisionpb.RegisterDecisionServiceHandlerFromEndpoint(ctx, mux, grpcTarget, opts); err != nil {
		return err
	}
	// Gin marks NoRoute as 404 before the handler runs. The gateway writes a
	// success body without calling WriteHeader, which would keep that 404.
	// Error paths still call WriteHeader and replace this provisional 200.
	r.NoRoute(func(c *gin.Context) {
		c.Writer.WriteHeader(http.StatusOK)
		mux.ServeHTTP(c.Writer, c.Request)
	})
	return nil
}

// DialTarget turns a listen address into a client target.
// ":5008" listens on every interface; the gateway in this process dials loopback.
func DialTarget(grpcAddr string) string {
	if strings.HasPrefix(grpcAddr, ":") {
		return "127.0.0.1" + grpcAddr
	}
	return grpcAddr
}

func incomingHeaderMatcher(key string) (string, bool) {
	if textproto.CanonicalMIMEHeaderKey(key) == "X-Userinfo" {
		return "x-userinfo", true
	}
	return runtime.DefaultHeaderMatcher(key)
}
