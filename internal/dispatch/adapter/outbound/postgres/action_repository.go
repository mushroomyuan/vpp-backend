package postgres

import (
	"context"

	"github.com/mushroomyuan/vpp-backend/dispatch/domain/model"
	"github.com/mushroomyuan/vpp-backend/dispatch/domain/port"
	infrapg "github.com/mushroomyuan/vpp-backend/dispatch/infrastructure/persistent/postgres"
)

// ActionRepositoryPostgres implements port.ActionRepository.
type ActionRepositoryPostgres struct {
	repo *infrapg.ActionRepository
}

func NewActionRepositoryPostgres(repo *infrapg.ActionRepository) *ActionRepositoryPostgres {
	if repo == nil {
		panic("NewActionRepositoryPostgres: repo is required")
	}
	return &ActionRepositoryPostgres{repo: repo}
}

var _ port.ActionRepository = (*ActionRepositoryPostgres)(nil)

func (r *ActionRepositoryPostgres) Update(ctx context.Context, action *model.DispatchAction) error {
	return r.repo.UpdateActionStatus(ctx, actionDomainToDB(action))
}
