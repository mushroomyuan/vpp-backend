package postgres

import (
	"context"
	"time"

	"github.com/mushroomyuan/vpp-backend/platform/logging"
	"gorm.io/gorm"
)

// CommandRepository provides raw GORM access to the control_commands table.
type CommandRepository struct {
	pg *Postgres
}

func NewCommandRepository(pg *Postgres) *CommandRepository {
	return &CommandRepository{pg: pg}
}

// UpdateCommandRuntime persists the hot-path runtime fields of a single command:
// status, retry_count, sent_at, deadline_at, finished_at, result.
// Returns gorm.ErrRecordNotFound when no row was updated.
func (r *CommandRepository) UpdateCommandRuntime(ctx context.Context, cmd *ControlCommandModel) (err error) {
	_, deferLog := logging.WhenDB(ctx, "CommandRepository.UpdateCommandRuntime", cmd.ID)
	defer func() { deferLog(nil, &err) }()

	result := r.pg.DB().WithContext(ctx).
		Model(&ControlCommandModel{}).
		Where("id = ?", cmd.ID).
		Updates(map[string]any{
			"status":      cmd.Status,
			"retry_count": cmd.RetryCount,
			"sent_at":     cmd.SentAt,
			"deadline_at": cmd.DeadlineAt,
			"finished_at": cmd.FinishedAt,
			"result":      cmd.Result,
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

// FindExpiredSending returns commands currently in Sending state whose deadline
// has passed. Only the columns needed by TimeoutScanner are selected; the caller
// loads the full task tree via TaskRepository.FindTaskByCommandID.
func (r *CommandRepository) FindExpiredSending(
	ctx context.Context,
	before time.Time,
) (results []*ControlCommandModel, err error) {
	_, deferLog := logging.WhenDB(ctx, "CommandRepository.FindExpiredSending", before)
	defer func() { deferLog(results, &err) }()

	err = r.pg.DB().WithContext(ctx).
		Select("id", "action_id", "tenant_id", "status", "deadline_at").
		Where("status = ? AND deadline_at < ?", "sending", before).
		Order("deadline_at ASC").
		Find(&results).Error
	return
}
