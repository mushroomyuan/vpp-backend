package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/mushroomyuan/vpp-backend/dispatch/domain"
	"github.com/mushroomyuan/vpp-backend/dispatch/domain/model"
	"github.com/mushroomyuan/vpp-backend/dispatch/domain/port"
	infrapg "github.com/mushroomyuan/vpp-backend/dispatch/infrastructure/persistent/postgres"
	"gorm.io/gorm"
)

// tenantIdempotencyConstraint is the partial unique index on dispatch_tasks.
const tenantIdempotencyConstraint = "uq_dispatch_tasks_tenant_idempotency"

// TaskRepositoryPostgres implements port.TaskRepository.
type TaskRepositoryPostgres struct {
	repo *infrapg.TaskRepository
}

func NewTaskRepositoryPostgres(repo *infrapg.TaskRepository) *TaskRepositoryPostgres {
	if repo == nil {
		panic("NewTaskRepositoryPostgres: repo is required")
	}
	return &TaskRepositoryPostgres{repo: repo}
}

var _ port.TaskRepository = (*TaskRepositoryPostgres)(nil)

func (r *TaskRepositoryPostgres) Save(ctx context.Context, task *model.DispatchTask) error {
	taskRow := taskDomainToDB(task)

	actions := make([]*infrapg.DispatchActionModel, 0, len(task.Actions))
	commands := make([]*infrapg.ControlCommandModel, 0)
	for _, action := range task.Actions {
		actions = append(actions, actionDomainToDB(action))
		for i, cmd := range action.Commands {
			cmdRow, err := commandDomainToDB(cmd, i)
			if err != nil {
				return fmt.Errorf("marshal command %s: %w", cmd.ID, err)
			}
			commands = append(commands, cmdRow)
		}
	}

	err := r.repo.CreateTaskTree(ctx, taskRow, actions, commands)
	if errors.Is(err, infrapg.ErrIdempotencyConflict) || isIdempotencyConflict(err) {
		return domain.ErrIdempotencyConflict
	}
	return err
}

func isIdempotencyConflict(err error) bool {
	if err == nil {
		return false
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return pgErr.ConstraintName == tenantIdempotencyConstraint ||
			strings.Contains(pgErr.Message, tenantIdempotencyConstraint)
	}
	return strings.Contains(err.Error(), tenantIdempotencyConstraint)
}

func (r *TaskRepositoryPostgres) Update(ctx context.Context, task *model.DispatchTask) error {
	err := r.repo.UpdateTask(ctx, taskDomainToDB(task))
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return domain.ErrTaskNotFound
	}
	return err
}

func (r *TaskRepositoryPostgres) FindByID(ctx context.Context, id string) (*model.DispatchTask, error) {
	tree, err := r.repo.FindTaskByID(ctx, id)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, domain.ErrTaskNotFound
	}
	if err != nil {
		return nil, err
	}
	return taskTreeToDomain(tree)
}

func (r *TaskRepositoryPostgres) FindByIdempotencyKey(ctx context.Context, tenantID, key string) (*model.DispatchTask, error) {
	tree, err := r.repo.FindTaskByIdempotencyKey(ctx, strings.TrimSpace(tenantID), strings.TrimSpace(key))
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, domain.ErrTaskNotFound
	}
	if err != nil {
		return nil, err
	}
	return taskTreeToDomain(tree)
}

func (r *TaskRepositoryPostgres) FindByCommandID(ctx context.Context, commandID string) (*model.DispatchTask, error) {
	tree, err := r.repo.FindTaskByCommandID(ctx, commandID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, domain.ErrTaskNotFound
	}
	if err != nil {
		return nil, err
	}
	return taskTreeToDomain(tree)
}
