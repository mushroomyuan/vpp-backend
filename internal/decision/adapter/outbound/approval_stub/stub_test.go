package approvalstub

import (
	"context"
	"errors"
	"testing"

	"github.com/mushroomyuan/vpp-backend/decision/domain/plan"
)

func TestStub_DecideIsNotImplemented(t *testing.T) {
	_, err := New().Decide(context.Background(), plan.Plan{})
	if !errors.Is(err, ErrNotImplemented) {
		t.Fatalf("err = %v, want ErrNotImplemented", err)
	}
}
