package command

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/mushroomyuan/vpp-backend/platform/decorator"
	platformtelemetry "github.com/mushroomyuan/vpp-backend/platform/telemetry"
	"github.com/mushroomyuan/vpp-backend/telemetry/domain"
	"github.com/mushroomyuan/vpp-backend/telemetry/domain/model"
	"github.com/mushroomyuan/vpp-backend/telemetry/domain/port"
)

// MetricInput is the application-layer DTO for a single metric reading.
// Using domain value types (MetricType, QualityStatus) is intentional:
// they carry no infrastructure coupling and are safe to expose here.
type MetricInput struct {
	Name    string
	Value   float64
	Type    model.MetricType
	Quality model.QualityStatus
}

// IngestTelemetry carries one push from a single CU: one timestamp, N metrics.
// One command = one CU push = one TelemetryRecord. Batch ingestion across
// multiple CUs is handled by the inbound adapter calling this handler in a loop.
type IngestTelemetry struct {
	TenantID  string
	CUCode    string
	Timestamp time.Time
	Metrics   []MetricInput
}

type IngestTelemetryResult struct {
	SOECount int
}

type IngestTelemetryHandler = decorator.CommandHandler[IngestTelemetry, *IngestTelemetryResult]

type ingestTelemetryHandler struct {
	telemetryRepo port.TelemetryRepository
	snapshotRepo  port.SnapshotRepository
	publisher     port.EventPublisher
}

func NewIngestTelemetryHandler(
	telemetryRepo port.TelemetryRepository,
	snapshotRepo port.SnapshotRepository,
	publisher port.EventPublisher,
	metricsClient decorator.MetricsClient,
) IngestTelemetryHandler {
	if telemetryRepo == nil {
		panic("NewIngestTelemetryHandler: telemetryRepo is required")
	}
	if snapshotRepo == nil {
		panic("NewIngestTelemetryHandler: snapshotRepo is required")
	}
	if publisher == nil {
		panic("NewIngestTelemetryHandler: publisher is required")
	}
	return decorator.ApplyCommandDecorators[IngestTelemetry, *IngestTelemetryResult](
		ingestTelemetryHandler{
			telemetryRepo: telemetryRepo,
			snapshotRepo:  snapshotRepo,
			publisher:     publisher,
		},
		metricsClient,
	)
}

func (h ingestTelemetryHandler) Handle(ctx context.Context, cmd IngestTelemetry) (*IngestTelemetryResult, error) {
	ctx, span := platformtelemetry.Start(ctx, "ingest_telemetry")
	defer span.End()

	// Step 1: build and validate the domain record.
	metrics := make([]model.Metric, 0, len(cmd.Metrics))
	for _, m := range cmd.Metrics {
		metrics = append(metrics, model.NewMetricWithQuality(m.Name, m.Value, m.Type, m.Quality))
	}
	record, err := model.NewTelemetryRecord(cmd.TenantID, cmd.CUCode, cmd.Timestamp, metrics)
	if err != nil {
		return nil, fmt.Errorf("build telemetry record: %w", err)
	}

	// Step 2: persist to time-series store.
	if err := h.telemetryRepo.SaveBatch(ctx, []*model.TelemetryRecord{record}); err != nil {
		return nil, fmt.Errorf("save telemetry record: %w", err)
	}

	// Step 3: load (or create) the snapshot and apply the record.
	snapshot, err := h.snapshotRepo.Find(ctx, cmd.TenantID, cmd.CUCode)
	if err != nil {
		if !errors.Is(err, domain.ErrSnapshotNotFound) {
			return nil, fmt.Errorf("load snapshot: %w", err)
		}
		snapshot = model.NewSnapshot(cmd.TenantID, cmd.CUCode)
	}
	soeEvents := snapshot.Apply(record)

	// Step 4: persist the updated snapshot.
	if err := h.snapshotRepo.Save(ctx, snapshot); err != nil {
		return nil, fmt.Errorf("save snapshot: %w", err)
	}

	// Step 5: publish SOE events (best-effort).
	// A publish failure must not roll back the already-persisted record or
	// snapshot; downstream consumers are eventually consistent.
	for _, event := range soeEvents {
		_ = h.publisher.PublishSOE(ctx, event)
	}

	return &IngestTelemetryResult{SOECount: len(soeEvents)}, nil
}
