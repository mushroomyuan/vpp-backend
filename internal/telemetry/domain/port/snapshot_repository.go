package port

import (
	"context"

	"github.com/mushroomyuan/vpp-backend/telemetry/domain/model"
)

type SnapshotRepository interface {
	Save(
		ctx context.Context,
		snapshot *model.Snapshot,
	) error

	// Find returns the latest snapshot for a single CU.
	// Returns domain.ErrSnapshotNotFound when no snapshot exists yet.
	Find(
		ctx context.Context,
		tenantID string,
		cuCode string,
	) (*model.Snapshot, error)

	// FindAll returns snapshots for every CU belonging to the given tenant.
	// Used by dashboard aggregation and fleet-level staleness checks.
	FindAll(
		ctx context.Context,
		tenantID string,
	) ([]*model.Snapshot, error)
}
