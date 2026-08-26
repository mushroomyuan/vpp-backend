package command

import (
	"context"
	"errors"
	"fmt"

	"github.com/mushroomyuan/vpp-backend/alarm/domain"
	"github.com/mushroomyuan/vpp-backend/alarm/domain/model"
	"github.com/mushroomyuan/vpp-backend/alarm/domain/port"
	"github.com/mushroomyuan/vpp-backend/alarm/domain/service"
	"github.com/mushroomyuan/vpp-backend/platform/decorator"
	"github.com/mushroomyuan/vpp-backend/platform/idgen"
)

// Outcomes match the ingest metric result labels (ok / dedup_hit / dropped).
const (
	OutcomeOK       = "ok"
	OutcomeDedupHit = "dedup_hit"
	OutcomeDropped  = "dropped"
)

// IngestEvent applies rules then persists via atomic repository.Ingest.
// Kafka adapters map Outcome to commit/skip; persistence errors stay errors
// (poison vs retry is classified next to the adapter).
type IngestEvent struct {
	Incoming model.IncomingEvent
}

type IngestEventResult struct {
	Outcome string
	AlarmID string
	Opened  bool // true only for a brand-new open row; SOE merge is false
}

type IngestEventHandler = decorator.CommandHandler[IngestEvent, *IngestEventResult]

type ingestEventHandler struct {
	evaluator *service.Evaluator
	repo      port.AlarmRepository
	notifier  port.Notifier
	observer  port.Observer
}

func NewIngestEventHandler(
	evaluator *service.Evaluator,
	repo port.AlarmRepository,
	notifier port.Notifier,
	metricsClient decorator.MetricsClient,
	observer port.Observer,
) IngestEventHandler {
	if evaluator == nil {
		panic("NewIngestEventHandler: evaluator is required")
	}
	if repo == nil {
		panic("NewIngestEventHandler: repo is required")
	}
	if notifier == nil {
		panic("NewIngestEventHandler: notifier is required")
	}
	return decorator.ApplyCommandDecorators[IngestEvent, *IngestEventResult](
		ingestEventHandler{evaluator: evaluator, repo: repo, notifier: notifier, observer: observer},
		metricsClient,
	)
}

func (h ingestEventHandler) Handle(ctx context.Context, cmd IngestEvent) (*IngestEventResult, error) {
	d, err := h.evaluator.Evaluate(cmd.Incoming)
	if err != nil {
		return nil, err
	}
	if d.Drop {
		return &IngestEventResult{Outcome: OutcomeDropped}, nil
	}

	candidateID := idgen.Must()
	res, err := h.repo.Ingest(ctx, candidateID, d)
	if err != nil {
		if errors.Is(err, domain.ErrDedupConflict) {
			return &IngestEventResult{Outcome: OutcomeDedupHit}, nil
		}
		return nil, fmt.Errorf("ingest alarm: %w", err)
	}
	if res.DedupHit() {
		return &IngestEventResult{Outcome: OutcomeDedupHit}, nil
	}
	opened := res.OpenedNew(candidateID)
	if opened {
		if alarm, nerr := model.NewOpenAlarm(candidateID, d); nerr == nil {
			_ = h.notifier.Notify(ctx, alarm)
		}
		if h.observer != nil {
			h.observer.AlarmOpened(string(d.Source))
		}
	}
	return &IngestEventResult{Outcome: OutcomeOK, AlarmID: res.AlarmID, Opened: opened}, nil
}
