package grpc

import (
	dispatchpb "github.com/mushroomyuan/vpp-backend/api/dispatch/proto/gen"
)

// CatalogOf maps DispatchService full methods to §7.1 catalog pairs.
func CatalogOf(fullMethod string) (resource, action string, ok bool) {
	switch fullMethod {
	case dispatchpb.DispatchService_SubmitTask_FullMethodName:
		return "dispatch:tasks", "submit", true
	case dispatchpb.DispatchService_GetTask_FullMethodName:
		return "dispatch:tasks", "read", true
	case dispatchpb.DispatchService_CancelTask_FullMethodName:
		return "dispatch:tasks", "cancel", true
	default:
		return "", "", false
	}
}
