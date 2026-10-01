package query

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/mushroomyuan/vpp-backend/platform/decorator"
	"github.com/mushroomyuan/vpp-backend/telemetry/domain/model"
	"github.com/mushroomyuan/vpp-backend/telemetry/domain/port"
)

const (
	maxSnapshotCUs     = 1000
	maxSnapshotMetrics = 32
)

// GetSnapshots returns real-time state for an explicit CU list and metric set.
// It does not scan the tenant. CUs with no snapshot are omitted. A returned
// snapshot includes only requested metrics that are present.
type GetSnapshots struct {
	TenantID  string
	CUCodes   []string
	MetricIDs []string
	// StaleAge overrides the default staleness threshold.
	// Zero means use defaultStaleAge defined in views.go.
	StaleAge time.Duration
}

type GetSnapshotsHandler = decorator.QueryHandler[GetSnapshots, []*SnapshotView]

type getSnapshotsHandler struct {
	snapshotRepo port.SnapshotRepository
}

func NewGetSnapshotsHandler(
	snapshotRepo port.SnapshotRepository,
	metricsClient decorator.MetricsClient,
) GetSnapshotsHandler {
	if snapshotRepo == nil {
		panic("NewGetSnapshotsHandler: snapshotRepo is required")
	}
	return decorator.ApplyQueryDecorators[GetSnapshots, []*SnapshotView](
		getSnapshotsHandler{snapshotRepo: snapshotRepo},
		metricsClient,
	)
}

func (h getSnapshotsHandler) Handle(ctx context.Context, q GetSnapshots) ([]*SnapshotView, error) {
	cuCodes, err := normalizeIDs(q.CUCodes, maxSnapshotCUs, "cu_code")
	if err != nil {
		return nil, err
	}
	metricIDs, err := normalizeIDs(q.MetricIDs, maxSnapshotMetrics, "metric_id")
	if err != nil {
		return nil, err
	}
	for _, id := range metricIDs {
		if err := model.RequireNumericMetricID(id); err != nil {
			return nil, err
		}
	}
	if strings.TrimSpace(q.TenantID) == "" {
		return nil, fmt.Errorf("invalid snapshot query: tenant_id is required")
	}

	snapshots, err := h.snapshotRepo.FindByCUs(ctx, q.TenantID, cuCodes)
	if err != nil {
		return nil, err
	}
	byCU := make(map[string]*model.Snapshot, len(snapshots))
	for _, snapshot := range snapshots {
		byCU[snapshot.CUCode] = snapshot
	}

	age := q.StaleAge
	if age == 0 {
		age = defaultStaleAge
	}
	views := make([]*SnapshotView, 0, len(cuCodes))
	for _, cu := range cuCodes {
		snapshot, ok := byCU[cu]
		if !ok {
			continue
		}
		views = append(views, snapshotToView(snapshot, metricIDs, age))
	}
	return views, nil
}

func normalizeIDs(ids []string, limit int, field string) ([]string, error) {
	if len(ids) == 0 {
		return nil, fmt.Errorf("invalid snapshot query: %s is required", field)
	}
	out := make([]string, 0, len(ids))
	seen := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" {
			return nil, fmt.Errorf("invalid snapshot query: %s is empty", field)
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	if len(out) > limit {
		return nil, fmt.Errorf("invalid snapshot query: %s count %d exceeds %d", field, len(out), limit)
	}
	return out, nil
}
