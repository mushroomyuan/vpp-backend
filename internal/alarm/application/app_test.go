package application

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/mushroomyuan/vpp-backend/alarm/domain"
	"github.com/mushroomyuan/vpp-backend/alarm/domain/model"
	"github.com/mushroomyuan/vpp-backend/alarm/domain/port"
	"github.com/mushroomyuan/vpp-backend/platform/decorator"
)

type nopMetrics struct{}

func (nopMetrics) Count(string, string, string)           {}
func (nopMetrics) CountN(string, string, string, float64) {}
func (nopMetrics) Observe(string, string, time.Duration)  {}
func (nopMetrics) TrackInFlight(string, string) func()    { return func() {} }

var _ decorator.MetricsClient = nopMetrics{}

type nopNotifier struct{}

func (nopNotifier) Notify(context.Context, *model.Alarm) error { return nil }

type recObs struct {
	set map[string]int
}

func (o *recObs) AlarmOpened(string) {}
func (o *recObs) AlarmClosed(string) {}
func (o *recObs) AckConflict()       {}
func (o *recObs) CloseConflict()     {}
func (o *recObs) SetOpenCount(source string, n int) {
	if o.set == nil {
		o.set = map[string]int{}
	}
	o.set[source] = n
}

type countRepo struct {
	counts map[model.Source]int
}

func (r *countRepo) Ingest(context.Context, string, model.Decision) (port.IngestResult, error) {
	return port.IngestResult{}, errors.New("unused")
}
func (r *countRepo) FindByID(context.Context, string, string) (*model.Alarm, error) {
	return nil, domain.ErrNotFound
}
func (r *countRepo) List(context.Context, port.ListFilter) ([]*model.Alarm, int, error) {
	return nil, 0, nil
}
func (r *countRepo) Acknowledge(context.Context, string, string, int, string, time.Time) (*model.Alarm, error) {
	return nil, errors.New("unused")
}
func (r *countRepo) Close(context.Context, string, string, int, string, time.Time) (*model.Alarm, error) {
	return nil, errors.New("unused")
}
func (r *countRepo) CountOpenBySource(context.Context) (map[model.Source]int, error) {
	return r.counts, nil
}

func TestCalibrateOpenAlarms(t *testing.T) {
	t.Parallel()
	obs := &recObs{}
	app := NewApplication(Dependencies{
		Repo: &countRepo{counts: map[model.Source]int{
			model.SourceDispatch: 3,
			model.SourceSOE:      4,
		}},
		Notifier: nopNotifier{},
		Metrics:  nopMetrics{},
		Observer: obs,
	})
	if err := app.CalibrateOpenAlarms(context.Background()); err != nil {
		t.Fatal(err)
	}
	if obs.set[string(model.SourceDispatch)] != 3 || obs.set[string(model.SourceSOE)] != 4 {
		t.Fatalf("%v", obs.set)
	}
}

func TestCalibrateOpenAlarms_NilObserver(t *testing.T) {
	t.Parallel()
	app := NewApplication(Dependencies{
		Repo:     &countRepo{counts: map[model.Source]int{model.SourceSOE: 1}},
		Notifier: nopNotifier{},
		Metrics:  nopMetrics{},
	})
	if err := app.CalibrateOpenAlarms(context.Background()); err != nil {
		t.Fatal(err)
	}
}
