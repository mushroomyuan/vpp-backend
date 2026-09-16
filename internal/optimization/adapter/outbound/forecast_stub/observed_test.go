package forecaststub

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/mushroomyuan/vpp-backend/optimization/domain/port"
	"github.com/mushroomyuan/vpp-backend/optimization/metrics"
)

type recordingObserver struct {
	forecast []string
}

func (r *recordingObserver) ObserveCycle(time.Duration, error) {}
func (r *recordingObserver) ObserveRulesFired(string, int)     {}
func (r *recordingObserver) ObserveSubmit(bool)                {}
func (r *recordingObserver) ObserveForecast(result string)     { r.forecast = append(r.forecast, result) }

func TestNewObserved_NilObserverReturnsInner(t *testing.T) {
	inner := New()
	got := NewObserved(inner, nil)
	if got != inner {
		t.Fatal("nil observer should return the inner port unchanged")
	}
}

func TestObserved_RecordsNotImplemented(t *testing.T) {
	obs := &recordingObserver{}
	p := NewObserved(New(), obs)
	_, err := p.GetLatestPrediction(context.Background(), "t", "cu", "soc")
	if !errors.Is(err, ErrNotImplemented) {
		t.Fatalf("err=%v", err)
	}
	if len(obs.forecast) != 1 || obs.forecast[0] != metrics.ResultNotImplemented {
		t.Fatalf("forecast observations = %v", obs.forecast)
	}
}

func TestObserved_RecordsError(t *testing.T) {
	obs := &recordingObserver{}
	p := NewObserved(errForecast{err: errors.New("boom")}, obs)
	_, err := p.GetLatestPrediction(context.Background(), "t", "cu", "soc")
	if err == nil {
		t.Fatal("expected error")
	}
	if len(obs.forecast) != 1 || obs.forecast[0] != metrics.ResultError {
		t.Fatalf("forecast observations = %v", obs.forecast)
	}
}

type errForecast struct{ err error }

func (e errForecast) GetLatestPrediction(context.Context, string, string, string) (*port.Prediction, error) {
	return nil, e.err
}
