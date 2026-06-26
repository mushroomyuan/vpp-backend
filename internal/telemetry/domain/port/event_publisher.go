package port

import (
	"context"

	"github.com/mushroomyuan/vpp-backend/telemetry/domain/model"
)

type EventPublisher interface {
	PublishSOE(
		ctx context.Context,
		event *model.SOEEvent,
	) error
}
