package postgres

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/mushroomyuan/vpp-backend/gateway/domain"
	"github.com/mushroomyuan/vpp-backend/gateway/domain/model"
	"github.com/mushroomyuan/vpp-backend/gateway/domain/port"
	infrapg "github.com/mushroomyuan/vpp-backend/gateway/infrastructure/persistent/postgres"
	"gorm.io/gorm"
)

// MappingRepositoryPostgres implements port.MappingRepository.
// It wraps the infrastructure-layer MappingRepository (raw GORM) and handles
// domain-error translation (gorm.ErrRecordNotFound → domain.ErrMappingNotFound, etc.).
type MappingRepositoryPostgres struct {
	repo *infrapg.MappingRepository
}

func NewMappingRepositoryPostgres(repo *infrapg.MappingRepository) *MappingRepositoryPostgres {
	if repo == nil {
		panic("NewMappingRepositoryPostgres: repo is required")
	}
	return &MappingRepositoryPostgres{repo: repo}
}

var _ port.MappingRepository = (*MappingRepositoryPostgres)(nil)

func (r *MappingRepositoryPostgres) Create(ctx context.Context, m *model.DeviceMapping) error {
	row := domainToDB(m)
	if err := r.repo.CreateMapping(ctx, row); err != nil {
		if isUniqueViolation(err) {
			return domain.ErrMappingConflict
		}
		return err
	}
	return nil
}

func (r *MappingRepositoryPostgres) Delete(ctx context.Context, tenantID, id string) error {
	err := r.repo.DeleteMapping(ctx, tenantID, id)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return domain.ErrMappingNotFound
	}
	return err
}

func (r *MappingRepositoryPostgres) Disable(ctx context.Context, tenantID, id string) error {
	err := r.repo.DisableMapping(ctx, tenantID, id)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return domain.ErrMappingNotFound
	}
	return err
}

func (r *MappingRepositoryPostgres) GetByExternalID(
	ctx context.Context,
	tenantID, externalSystem, externalID string,
) (*model.DeviceMapping, error) {
	row, err := r.repo.FindByExternalID(ctx, tenantID, externalSystem, externalID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, domain.ErrMappingNotFound
	}
	if err != nil {
		return nil, err
	}
	return dbToDomain(row), nil
}

func (r *MappingRepositoryPostgres) GetByCUCode(
	ctx context.Context,
	tenantID, cuCode string,
) (*model.DeviceMapping, error) {
	row, err := r.repo.FindByCUCode(ctx, tenantID, cuCode)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, domain.ErrMappingNotFound
	}
	if err != nil {
		return nil, err
	}
	return dbToDomain(row), nil
}

func (r *MappingRepositoryPostgres) List(
	ctx context.Context,
	tenantID string,
) ([]*model.DeviceMapping, error) {
	rows, err := r.repo.ListByTenantID(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	out := make([]*model.DeviceMapping, 0, len(rows))
	for _, row := range rows {
		out = append(out, dbToDomain(row))
	}
	return out, nil
}

// ─── converters ──────────────────────────────────────────────────────────────

func domainToDB(m *model.DeviceMapping) *infrapg.DeviceMappingModel {
	return &infrapg.DeviceMappingModel{
		ID:             m.ID,
		TenantID:       m.TenantID,
		ExternalSystem: m.ExternalSystem,
		ExternalID:     m.ExternalID,
		CUCode:         m.CUCode,
		Status:         string(m.Status),
		CreatedAt:      m.CreatedAt,
		UpdatedAt:      m.UpdatedAt,
	}
}

func dbToDomain(row *infrapg.DeviceMappingModel) *model.DeviceMapping {
	return &model.DeviceMapping{
		ID:             row.ID,
		TenantID:       row.TenantID,
		ExternalSystem: row.ExternalSystem,
		ExternalID:     row.ExternalID,
		CUCode:         row.CUCode,
		Status:         model.MappingStatus(row.Status),
		CreatedAt:      row.CreatedAt,
		UpdatedAt:      row.UpdatedAt,
	}
}

// isUniqueViolation detects a PostgreSQL unique-constraint violation (code 23505).
func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == "23505"
	}
	return false
}
