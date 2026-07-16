package command

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/mushroomyuan/vpp-backend/platform/decorator"
	"github.com/mushroomyuan/vpp-backend/platform/logging"
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
	metrics       decorator.MetricsClient
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
			metrics:       metricsClient,
		},
		metricsClient,
	)
}

func (h ingestTelemetryHandler) Handle(ctx context.Context, cmd IngestTelemetry) (*IngestTelemetryResult, error) {
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
	// This is the hard gate: if TimescaleDB is unavailable the ingest must fail
	// so the caller can retry and the data is not silently lost.
	if err := h.telemetryRepo.SaveBatch(ctx, []*model.TelemetryRecord{record}); err != nil {
		return nil, fmt.Errorf("save telemetry record: %w", err)
	}

	// Step 3: load (or create) the snapshot for this CU.
	//
	// Redis failure is treated as "snapshot not found": we fall back to an
	// empty baseline and continue. SOE detection may miss one transition in
	// this cycle, but the next successful ingest will restore consistency.
	// Returning an error here would cause the caller to retry; since the
	// record is already in TimescaleDB (ON CONFLICT DO NOTHING), the retry
	// would be a no-op for the DB but would re-run SOE detection against a
	// stale baseline, potentially generating duplicate events.
	snapshot, err := h.snapshotRepo.Find(ctx, cmd.TenantID, cmd.CUCode)
	if err != nil {
		if !errors.Is(err, domain.ErrSnapshotNotFound) {
			logging.Errorf(ctx, logrus.Fields{
				"component": "IngestTelemetry",
				"tenant_id": cmd.TenantID,
				"cu_code":   cmd.CUCode,
				"error":     err.Error(),
			}, "snapshot read failed — using empty baseline for this cycle")
			h.countMetric("snapshot", "read", "failure")
		}
		snapshot = model.NewSnapshot(cmd.TenantID, cmd.CUCode)
	}
	soeEvents := snapshot.Apply(record)

	// Step 4: persist the updated snapshot.
	//
	// Redis write failure does NOT abort the handler. The original record is
	// already durable in TimescaleDB. Returning an error would cause the
	// caller to retry, which would produce a duplicate SOE publish (the
	// Apply() above already calculated the events). Instead, we log at Error
	// level (triggering alert rules on Error-rate dashboards) and continue so
	// that SOE events are published and the caller receives a success response.
	if err := h.snapshotRepo.Save(ctx, snapshot); err != nil {
		logging.Errorf(ctx, logrus.Fields{
			"component": "IngestTelemetry",
			"tenant_id": cmd.TenantID,
			"cu_code":   cmd.CUCode,
			"error":     err.Error(),
		}, "snapshot write failed — snapshot stale until next successful ingest")
		h.countMetric("snapshot", "write", "failure")
	}

	// Step 5: publish SOE events (best-effort).
	// A publish failure must not roll back the already-persisted record or
	// snapshot; downstream consumers are eventually consistent.
	for _, event := range soeEvents {
		_ = h.publisher.PublishSOE(ctx, event)
	}

	return &IngestTelemetryResult{SOECount: len(soeEvents)}, nil
}

// countMetric is a nil-safe wrapper around the metrics client.
func (h ingestTelemetryHandler) countMetric(kind, action, status string) {
	if h.metrics != nil {
		h.metrics.Count(kind, action, status)
	}
}
