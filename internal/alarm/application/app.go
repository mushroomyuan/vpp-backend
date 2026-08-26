package application

import (
	"context"
	"fmt"

	"github.com/mushroomyuan/vpp-backend/alarm/application/command"
	"github.com/mushroomyuan/vpp-backend/alarm/application/query"
	"github.com/mushroomyuan/vpp-backend/alarm/domain/model"
	"github.com/mushroomyuan/vpp-backend/alarm/domain/port"
	"github.com/mushroomyuan/vpp-backend/alarm/domain/service"
	"github.com/mushroomyuan/vpp-backend/platform/decorator"
)

// Application is the composition root of the alarm use-case layer.
// Inbound adapters (Kafka, HTTP) depend only on this struct.
type Application struct {
	Commands Commands
	Queries  Queries

	repo     port.AlarmRepository
	observer port.Observer
}

type Commands struct {
	IngestEvent command.IngestEventHandler
	Acknowledge command.AcknowledgeHandler
	Close       command.CloseHandler
}

type Queries struct {
	ListAlarms query.ListAlarmsHandler
	GetAlarm   query.GetAlarmHandler
}

type Dependencies struct {
	Repo     port.AlarmRepository
	Notifier port.Notifier
	Rules    *service.Rules // nil → DefaultRules
	Metrics  decorator.MetricsClient
	Observer port.Observer
}

func NewApplication(deps Dependencies) Application {
	if deps.Repo == nil {
		panic("NewApplication: Repo is required")
	}
	if deps.Notifier == nil {
		panic("NewApplication: Notifier is required")
	}
	rules := service.DefaultRules()
	if deps.Rules != nil {
		rules = *deps.Rules
	}
	evaluator := service.NewEvaluator(rules)
	return Application{
		Commands: Commands{
			IngestEvent: command.NewIngestEventHandler(evaluator, deps.Repo, deps.Notifier, deps.Metrics, deps.Observer),
			Acknowledge: command.NewAcknowledgeHandler(deps.Repo, deps.Metrics, deps.Observer),
			Close:       command.NewCloseHandler(deps.Repo, deps.Metrics, deps.Observer),
		},
		Queries: Queries{
			ListAlarms: query.NewListAlarmsHandler(deps.Repo, deps.Metrics),
			GetAlarm:   query.NewGetAlarmHandler(deps.Repo, deps.Metrics),
		},
		repo:     deps.Repo,
		observer: deps.Observer,
	}
}

// CalibrateOpenAlarms snapshots current non-closed rows into the process-local
// open gauge. Call once at process start, before consumers run. Not a scrape hook.
func (a Application) CalibrateOpenAlarms(ctx context.Context) error {
	if a.observer == nil {
		return nil
	}
	counts, err := a.repo.CountOpenBySource(ctx)
	if err != nil {
		return fmt.Errorf("calibrate open alarms: %w", err)
	}
	a.observer.SetOpenCount(string(model.SourceDispatch), counts[model.SourceDispatch])
	a.observer.SetOpenCount(string(model.SourceSOE), counts[model.SourceSOE])
	return nil
}
