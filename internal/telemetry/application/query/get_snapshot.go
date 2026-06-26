package query

import (
	"context"
	"time"

	"github.com/mushroomyuan/vpp-backend/platform/decorator"
	platformtelemetry "github.com/mushroomyuan/vpp-backend/platform/telemetry"
	"github.com/mushroomyuan/vpp-backend/telemetry/domain/port"
)

type GetSnapshot struct {
	TenantID string
	CUCode   string
	// StaleAge overrides the default staleness threshold.
	// Zero means use defaultStaleAge defined in views.go.
	StaleAge time.Duration
}

type GetSnapshotHandler = decorator.QueryHandler[GetSnapshot, *SnapshotView]

type getSnapshotHandler struct {
	snapshotRepo port.SnapshotRepository
}

func NewGetSnapshotHandler(
	snapshotRepo port.SnapshotRepository,
	metricsClient decorator.MetricsClient,
) GetSnapshotHandler {
	if snapshotRepo == nil {
		panic("NewGetSnapshotHandler: snapshotRepo is required")
	}
	return decorator.ApplyQueryDecorators[GetSnapshot, *SnapshotView](
		getSnapshotHandler{snapshotRepo: snapshotRepo},
		metricsClient,
	)
}

func (h getSnapshotHandler) Handle(ctx context.Context, q GetSnapshot) (*SnapshotView, error) {
	ctx, span := platformtelemetry.Start(ctx, "get_snapshot")
	defer span.End()

	snapshot, err := h.snapshotRepo.Find(ctx, q.TenantID, q.CUCode)
	if err != nil {
		return nil, err
	}

	age := q.StaleAge
	if age == 0 {
		age = defaultStaleAge
	}
	return snapshotToView(snapshot, age), nil
}
