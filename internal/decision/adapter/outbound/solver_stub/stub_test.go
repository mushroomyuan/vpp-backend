package solverstub

import (
	"context"
	"errors"
	"testing"

	"github.com/mushroomyuan/vpp-backend/decision/domain/allocation"
)

func TestStub_SolveIsNotImplemented(t *testing.T) {
	_, err := New().Solve(context.Background(), allocation.Problem{})
	if !errors.Is(err, ErrNotImplemented) {
		t.Fatalf("err = %v, want ErrNotImplemented", err)
	}
}
