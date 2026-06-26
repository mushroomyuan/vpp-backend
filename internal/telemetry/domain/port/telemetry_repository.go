package port

import (
	"context"

	"github.com/mushroomyuan/vpp-backend/telemetry/domain/model"
)

type TelemetryRepository interface {
	SaveBatch(
		ctx context.Context,
		records []*model.TelemetryRecord,
	) error

	Query(
		ctx context.Context,
		condition model.QueryCondition,
	) ([]*model.TelemetryRecord, error)
}
