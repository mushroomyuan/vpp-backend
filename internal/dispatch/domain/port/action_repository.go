package port

import (
	"context"

	"github.com/mushroomyuan/vpp-backend/dispatch/domain/model"
)

// ActionRepository handles persistence for DispatchAction entities.
//
// Write frequency: medium.
// Updated when an action transitions between states (Pending → Running → Completed|Failed|Cancelled).
type ActionRepository interface {
	// Update persists only the DispatchAction row (status field).
	// Does NOT touch the parent DispatchTask or child ControlCommands.
	Update(ctx context.Context, action *model.DispatchAction) error
}
