package postgres

import (
	"context"
	"errors"

	"github.com/mushroomyuan/vpp-backend/resource/domain"
	"github.com/mushroomyuan/vpp-backend/resource/domain/model"
	"github.com/mushroomyuan/vpp-backend/resource/domain/port"
	infra "github.com/mushroomyuan/vpp-backend/resource/infrastructure/persistent/postgres"
	"gorm.io/gorm"
)

type CUCapabilityRepositoryPostgres struct {
	repo *infra.CUCapabilityRepository
}

func NewCUCapabilityRepositoryPostgres(
	repo *infra.CUCapabilityRepository,
) *CUCapabilityRepositoryPostgres {
	if repo == nil {
		panic("NewCUCapabilityRepositoryPostgres: repo is required")
	}
	return &CUCapabilityRepositoryPostgres{repo: repo}
}

var _ port.CUCapabilityRepository = (*CUCapabilityRepositoryPostgres)(nil)

func (r *CUCapabilityRepositoryPostgres) Create(
	ctx context.Context,
	capability *model.CUCapability,
) error {
	return r.repo.Create(ctx, CUCapabilityDomainToDB(capability))
}

func (r *CUCapabilityRepositoryPostgres) Update(
	ctx context.Context,
	capability *model.CUCapability,
) error {
	err := r.repo.Update(ctx, CUCapabilityDomainToDB(capability))
	if errors.Is(err, infra.ErrOptimisticLock) {
		return domain.ErrVersionConflict
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return domain.ErrCUCapabilityNotFound
	}
	return err
}

func (r *CUCapabilityRepositoryPostgres) FindByID(
	ctx context.Context,
	tenantID, id string,
) (*model.CUCapability, error) {
	row, err := r.repo.FindByID(ctx, tenantID, id)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, domain.ErrCUCapabilityNotFound
	}
	if err != nil {
		return nil, err
	}
	return CUCapabilityDBToDomain(row), nil
}

func (r *CUCapabilityRepositoryPostgres) List(
	ctx context.Context,
	filter port.CUCapabilityFilter,
) ([]*model.CUCapability, error) {
	rows, err := r.repo.List(
		ctx, filter.TenantID, filter.CUID, filter.CapabilityIDs, filter.EnabledOnly,
	)
	if err != nil {
		return nil, err
	}
	out := make([]*model.CUCapability, 0, len(rows))
	for _, row := range rows {
		out = append(out, CUCapabilityDBToDomain(row))
	}
	return out, nil
}

func (r *CUCapabilityRepositoryPostgres) Delete(ctx context.Context, tenantID, id string) error {
	err := r.repo.Delete(ctx, tenantID, id)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return domain.ErrCUCapabilityNotFound
	}
	return err
}
