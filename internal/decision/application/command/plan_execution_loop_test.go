package command

import (
	"context"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestUncertainDispatch(t *testing.T) {
	if !uncertainDispatch(context.DeadlineExceeded) {
		t.Fatal("deadline exceeded must be retried")
	}
	if !uncertainDispatch(status.Error(codes.Unavailable, "down")) {
		t.Fatal("unavailable must be retried")
	}
	if uncertainDispatch(status.Error(codes.InvalidArgument, "bad")) {
		t.Fatal("invalid argument is a definite rejection")
	}
	if uncertainDispatch(nil) {
		t.Fatal("nil error is not uncertain")
	}
}
