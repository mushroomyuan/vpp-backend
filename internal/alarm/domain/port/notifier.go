package port

import (
	"context"

	"github.com/mushroomyuan/vpp-backend/alarm/domain/model"
)

// Notifier is the outbound notification port. v1 logs only; no mail/SMS.
// Called after a new open ticket is created, not on SOE merge or dedup hit.
type Notifier interface {
	Notify(ctx context.Context, alarm *model.Alarm) error
}
