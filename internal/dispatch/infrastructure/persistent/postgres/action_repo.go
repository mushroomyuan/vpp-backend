package postgres

import (
	"context"

	"github.com/mushroomyuan/vpp-backend/platform/logging"
	"gorm.io/gorm"
)

// ActionRepository provides raw GORM access to the dispatch_actions table.
type ActionRepository struct {
	pg *Postgres
}

func NewActionRepository(pg *Postgres) *ActionRepository {
	return &ActionRepository{pg: pg}
}

// UpdateActionStatus persists only the status column of a dispatch_actions row.
// Returns gorm.ErrRecordNotFound when no row was updated.
func (r *ActionRepository) UpdateActionStatus(ctx context.Context, action *DispatchActionModel) (err error) {
	_, deferLog := logging.WhenDB(ctx, "ActionRepository.UpdateActionStatus", action.ID)
	defer func() { deferLog(nil, &err) }()

	result := r.pg.DB().WithContext(ctx).
		Model(&DispatchActionModel{}).
		Where("id = ?", action.ID).
		Update("status", action.Status)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}
