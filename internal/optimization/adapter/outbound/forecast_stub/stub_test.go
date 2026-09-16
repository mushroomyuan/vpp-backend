package forecaststub

import (
	"context"
	"errors"
	"testing"
)

func TestStub_GetLatestPrediction_AlwaysNotImplemented(t *testing.T) {
	s := New()
	pred, err := s.GetLatestPrediction(context.Background(), "tenant-1", "cu-1", "soc")
	if !errors.Is(err, ErrNotImplemented) {
		t.Fatalf("got err=%v, want ErrNotImplemented", err)
	}
	if pred != nil {
		t.Errorf("expected a nil Prediction, got %+v", pred)
	}
}
