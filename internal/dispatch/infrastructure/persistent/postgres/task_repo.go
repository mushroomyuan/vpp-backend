package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/mushroomyuan/vpp-backend/platform/logging"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ErrIdempotencyConflict is returned when a task insert loses the
// (tenant_id, idempotency_key) race. The adapter maps it to the domain error.
var ErrIdempotencyConflict = errors.New("dispatch task idempotency conflict")

// TaskRepository provides raw GORM access to dispatch_tasks and the nested
// action/command rows needed to load or create a full task tree.
// It operates exclusively on *Model types; domain conversion lives in the adapter.
type TaskRepository struct {
	pg *Postgres
}

func NewTaskRepository(pg *Postgres) *TaskRepository {
	return &TaskRepository{pg: pg}
}

// CreateTaskTree inserts the task, all actions, and all commands in a single
// transaction. Callers must supply a complete tree; partial trees are rejected
// by the domain layer before reaching this method.
func (r *TaskRepository) CreateTaskTree(
	ctx context.Context,
	task *DispatchTaskModel,
	actions []*DispatchActionModel,
	commands []*ControlCommandModel,
) (err error) {
	_, deferLog := logging.WhenDB(ctx, "TaskRepository.CreateTaskTree", task.ID)
	defer func() { deferLog(nil, &err) }()

	return r.pg.StartTransaction(func(tx *gorm.DB) error {
		tx = tx.WithContext(ctx)
		insert := tx
		if task.IdempotencyKey != nil && *task.IdempotencyKey != "" {
			// Partial unique index: a lost race inserts nothing and leaves the winner's row.
			insert = tx.Clauses(clause.OnConflict{
				Columns: []clause.Column{{Name: "tenant_id"}, {Name: "idempotency_key"}},
				TargetWhere: clause.Where{Exprs: []clause.Expression{
					clause.Expr{SQL: "idempotency_key IS NOT NULL AND idempotency_key <> ''"},
				}},
				DoNothing: true,
			})
		}
		result := insert.Create(task)
		if result.Error != nil {
			return fmt.Errorf("insert dispatch_task: %w", result.Error)
		}
		if task.IdempotencyKey != nil && *task.IdempotencyKey != "" && result.RowsAffected == 0 {
			return ErrIdempotencyConflict
		}
		if len(actions) > 0 {
			if err := tx.Create(&actions).Error; err != nil {
				return fmt.Errorf("insert dispatch_actions: %w", err)
			}
		}
		if len(commands) > 0 {
			if err := tx.Create(&commands).Error; err != nil {
				return fmt.Errorf("insert control_commands: %w", err)
			}
		}
		return nil
	})
}

// UpdateTask persists only the dispatch_tasks row fields that change at runtime
// (status, started_at, finished_at). Actions and commands are not touched.
func (r *TaskRepository) UpdateTask(ctx context.Context, task *DispatchTaskModel) (err error) {
	_, deferLog := logging.WhenDB(ctx, "TaskRepository.UpdateTask", task.ID)
	defer func() { deferLog(nil, &err) }()

	result := r.pg.DB().WithContext(ctx).
		Model(&DispatchTaskModel{}).
		Where("id = ?", task.ID).
		Updates(map[string]any{
			"status":      task.Status,
			"started_at":  task.StartedAt,
			"finished_at": task.FinishedAt,
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

// FindTaskByID loads the complete task tree (task + actions + commands).
// Actions are ordered by sequence ascending; commands within each action by sequence.
// Returns gorm.ErrRecordNotFound when the task does not exist.
func (r *TaskRepository) FindTaskByID(ctx context.Context, id string) (tree *TaskTree, err error) {
	_, deferLog := logging.WhenDB(ctx, "TaskRepository.FindTaskByID", id)
	defer func() { deferLog(tree, &err) }()

	return r.loadTaskTree(ctx, "id = ?", id)
}

// FindTaskByIdempotencyKey loads the task tree stored for one tenant and key.
// Returns gorm.ErrRecordNotFound when no row matches.
func (r *TaskRepository) FindTaskByIdempotencyKey(ctx context.Context, tenantID, key string) (tree *TaskTree, err error) {
	_, deferLog := logging.WhenDB(ctx, "TaskRepository.FindTaskByIdempotencyKey", tenantID)
	defer func() { deferLog(tree, &err) }()

	if tenantID == "" || key == "" {
		return nil, gorm.ErrRecordNotFound
	}
	return r.loadTaskTree(ctx, "tenant_id = ? AND idempotency_key = ?", tenantID, key)
}

// FindTaskByCommandID loads the complete task tree for the task that owns the
// given command ID. Returns gorm.ErrRecordNotFound when the command (or its
// parent task) does not exist.
func (r *TaskRepository) FindTaskByCommandID(ctx context.Context, commandID string) (tree *TaskTree, err error) {
	_, deferLog := logging.WhenDB(ctx, "TaskRepository.FindTaskByCommandID", commandID)
	defer func() { deferLog(tree, &err) }()

	var taskID string
	err = r.pg.DB().WithContext(ctx).
		Table("control_commands").
		Select("dispatch_actions.task_id").
		Joins("JOIN dispatch_actions ON dispatch_actions.id = control_commands.action_id").
		Where("control_commands.id = ?", commandID).
		Scan(&taskID).Error
	if err != nil {
		return nil, err
	}
	if taskID == "" {
		return nil, gorm.ErrRecordNotFound
	}
	return r.loadTaskTree(ctx, "id = ?", taskID)
}

func (r *TaskRepository) loadTaskTree(ctx context.Context, where string, args ...any) (*TaskTree, error) {
	var task DispatchTaskModel
	if err := r.pg.DB().WithContext(ctx).Where(where, args...).First(&task).Error; err != nil {
		return nil, err
	}

	var actions []*DispatchActionModel
	if err := r.pg.DB().WithContext(ctx).
		Where("task_id = ?", task.ID).
		Order("sequence ASC").
		Find(&actions).Error; err != nil {
		return nil, err
	}

	actionIDs := make([]string, 0, len(actions))
	for _, a := range actions {
		actionIDs = append(actionIDs, a.ID)
	}

	var commands []*ControlCommandModel
	if len(actionIDs) > 0 {
		if err := r.pg.DB().WithContext(ctx).
			Where("action_id IN ?", actionIDs).
			Order("action_id ASC, sequence ASC").
			Find(&commands).Error; err != nil {
			return nil, err
		}
	}

	return &TaskTree{
		Task:     &task,
		Actions:  actions,
		Commands: commands,
	}, nil
}
