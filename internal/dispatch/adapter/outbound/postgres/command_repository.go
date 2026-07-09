package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/mushroomyuan/vpp-backend/dispatch/domain"
	"github.com/mushroomyuan/vpp-backend/dispatch/domain/model"
	"github.com/mushroomyuan/vpp-backend/dispatch/domain/port"
	infrapg "github.com/mushroomyuan/vpp-backend/dispatch/infrastructure/persistent/postgres"
	"gorm.io/gorm"
)

// CommandRepositoryPostgres implements port.CommandRepository.
type CommandRepositoryPostgres struct {
	repo *infrapg.CommandRepository
}

func NewCommandRepositoryPostgres(repo *infrapg.CommandRepository) *CommandRepositoryPostgres {
	if repo == nil {
		panic("NewCommandRepositoryPostgres: repo is required")
	}
	return &CommandRepositoryPostgres{repo: repo}
}

var _ port.CommandRepository = (*CommandRepositoryPostgres)(nil)

func (r *CommandRepositoryPostgres) Update(ctx context.Context, cmd *model.ControlCommand) error {
	row, err := commandRuntimeToDB(cmd)
	if err != nil {
		return fmt.Errorf("marshal command result: %w", err)
	}
	err = r.repo.UpdateCommandRuntime(ctx, row)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return domain.ErrCommandNotFound
	}
	return err
}

func (r *CommandRepositoryPostgres) FindExpiredSending(
	ctx context.Context,
	before time.Time,
) ([]*model.ControlCommand, error) {
	rows, err := r.repo.FindExpiredSending(ctx, before)
	if err != nil {
		return nil, err
	}
	out := make([]*model.ControlCommand, 0, len(rows))
	for _, row := range rows {
		out = append(out, expiredCommandDBToDomain(row))
	}
	return out, nil
}
