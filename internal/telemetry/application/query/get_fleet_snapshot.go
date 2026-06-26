package query

import (
	"context"
	"time"

	"github.com/mushroomyuan/vpp-backend/platform/decorator"
	platformtelemetry "github.com/mushroomyuan/vpp-backend/platform/telemetry"
	"github.com/mushroomyuan/vpp-backend/telemetry/domain/port"
)

// GetFleetSnapshot returns the real-time snapshot for every CU belonging to a
// tenant. Used by dashboard aggregation and fleet-level staleness checks.
type GetFleetSnapshot struct {
	TenantID string
	// StaleAge overrides the default staleness threshold.
	// Zero means use defaultStaleAge defined in views.go.
	StaleAge time.Duration
}

type GetFleetSnapshotHandler = decorator.QueryHandler[GetFleetSnapshot, []*SnapshotView]

type getFleetSnapshotHandler struct {
	snapshotRepo port.SnapshotRepository
}

func NewGetFleetSnapshotHandler(
	snapshotRepo port.SnapshotRepository,
	metricsClient decorator.MetricsClient,
) GetFleetSnapshotHandler {
	if snapshotRepo == nil {
		panic("NewGetFleetSnapshotHandler: snapshotRepo is required")
	}
	return decorator.ApplyQueryDecorators[GetFleetSnapshot, []*SnapshotView](
		getFleetSnapshotHandler{snapshotRepo: snapshotRepo},
		metricsClient,
	)
}

func (h getFleetSnapshotHandler) Handle(ctx context.Context, q GetFleetSnapshot) ([]*SnapshotView, error) {
	ctx, span := platformtelemetry.Start(ctx, "get_fleet_snapshot")
	defer span.End()

	snapshots, err := h.snapshotRepo.FindAll(ctx, q.TenantID)
	if err != nil {
		return nil, err
	}

	age := q.StaleAge
	if age == 0 {
		age = defaultStaleAge
	}
	views := make([]*SnapshotView, 0, len(snapshots))
	for _, s := range snapshots {
		views = append(views, snapshotToView(s, age))
	}
	return views, nil
}
