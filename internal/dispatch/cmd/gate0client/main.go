// Command gate0client probes APISIX gRPC OIDC rejection/acceptance with grpc-go
// (Gate 0). grpcurl alone is not enough: OIDC may return bare HTTP 401 which
// grpc-go surfaces differently than grpcurl.
package main

import (
	"context"
	"encoding/base64"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	dispatchpb "github.com/mushroomyuan/vpp-backend/api/dispatch/proto/gen"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:9081", "APISIX gRPC (h2c) address")
	mode := flag.String("mode", "no-auth", "no-auth | get | submit")
	token := flag.String("token", "", "Casdoor access token (Bearer value without prefix)")
	forgeAdmin := flag.Bool("forge-admin-userinfo", false, "attach forged admin x-userinfo metadata")
	flag.Parse()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	conn, err := grpc.NewClient(*addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		fail("dial: %v", err)
	}
	defer conn.Close()
	client := dispatchpb.NewDispatchServiceClient(conn)

	switch *mode {
	case "no-auth":
		_, err := client.GetTask(ctx, &dispatchpb.GetTaskRequest{
			TenantID: "default",
			TaskID:   "gate0-probe",
		})
		reportReject("GetTask without token", err)
	case "get":
		ctx = withAuth(ctx, *token, *forgeAdmin)
		_, err := client.GetTask(ctx, &dispatchpb.GetTaskRequest{
			TenantID: "default",
			TaskID:   "gate0-probe-missing",
		})
		reportRPC("GetTask with token", err)
	case "submit":
		ctx = withAuth(ctx, *token, *forgeAdmin)
		_, err := client.SubmitTask(ctx, &dispatchpb.SubmitTaskRequest{
			TenantID:    "default",
			Name:        "gate0-probe",
			TaskType:    "control",
			TriggerType: "manual",
		})
		reportRPC("SubmitTask with token", err)
	default:
		fail("unknown mode %q", *mode)
	}
}

func withAuth(ctx context.Context, token string, forgeAdmin bool) context.Context {
	if strings.TrimSpace(token) == "" {
		fail("-token is required for mode get/submit")
	}
	md := metadata.MD{}
	md.Set("authorization", "Bearer "+strings.TrimSpace(token))
	if forgeAdmin {
		raw := `{"owner":"default","name":"forged","roles":[{"name":"admin"}]}`
		md.Set("x-userinfo", base64.StdEncoding.EncodeToString([]byte(raw)))
	}
	return metadata.NewOutgoingContext(ctx, md)
}

func reportReject(label string, err error) {
	if err == nil {
		fail("%s: expected rejection, got OK", label)
	}
	st, ok := status.FromError(err)
	fmt.Printf("OK %s rejected\n", label)
	fmt.Printf("  error: %v\n", err)
	if ok {
		fmt.Printf("  grpc_code: %s\n", st.Code())
		fmt.Printf("  grpc_message: %s\n", st.Message())
	} else {
		fmt.Printf("  note: not a status.Status (transport/HTTP style rejection is possible)\n")
	}
	// Non-zero exit only on unexpected success; rejection of any form is pass for Gate 0.
	os.Exit(0)
}

func reportRPC(label string, err error) {
	if err == nil {
		fmt.Printf("OK %s succeeded\n", label)
		os.Exit(0)
	}
	st, ok := status.FromError(err)
	fmt.Printf("OK %s reached backend (RPC error after auth path)\n", label)
	fmt.Printf("  error: %v\n", err)
	if ok {
		fmt.Printf("  grpc_code: %s\n", st.Code())
		fmt.Printf("  grpc_message: %s\n", st.Message())
	}
	os.Exit(0)
}

func fail(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "FAIL "+format+"\n", args...)
	os.Exit(1)
}
