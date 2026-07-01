package ports

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	resourcepb "github.com/mushroomyuan/vpp-backend/api/resource/proto/gen"
)

// MountGateway mounts grpc-gateway handlers onto a gin router.
//
// The generated gateway routes already include the "/api" prefix (see resource_service.proto annotations),
// so callers typically mount at "/" on the gin engine.
func MountGateway(ctx context.Context, r *gin.Engine, grpcServer resourcepb.ResourceServiceServer) error {
	mux := runtime.NewServeMux()
	if err := resourcepb.RegisterResourceServiceHandlerServer(ctx, mux, grpcServer); err != nil {
		return err
	}
	r.Any("/*any", gin.WrapH(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		mux.ServeHTTP(w, req)
	})))
	return nil
}
