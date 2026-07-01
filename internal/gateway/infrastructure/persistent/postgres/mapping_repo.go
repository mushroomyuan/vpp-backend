package postgres

import (
	"context"

	"github.com/mushroomyuan/vpp-backend/platform/logging"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// MappingRepository provides raw GORM access to the device_mappings table.
// It operates exclusively on DeviceMappingModel; domain-model translation and
// port.MappingRepository contract fulfilment live in the adapter layer.
type MappingRepository struct {
	pg *Postgres
}

func NewMappingRepository(pg *Postgres) *MappingRepository {
	return &MappingRepository{pg: pg}
}

// CreateMapping inserts a new row. Callers must ensure the unique constraint
// (tenant_id, external_system, external_id) is not violated before calling;
// a Postgres unique-violation error is propagated as-is.
func (r *MappingRepository) CreateMapping(ctx context.Context, m *DeviceMappingModel) (err error) {
	_, deferLog := logging.WhenDB(ctx, "MappingRepository.CreateMapping", m)
	defer func() { deferLog(nil, &err) }()
	return r.pg.DB().WithContext(ctx).Clauses(clause.Returning{}).Create(m).Error
}

// DeleteMapping hard-deletes the row identified by (tenantID, id).
// Returns gorm.ErrRecordNotFound when no row was deleted.
func (r *MappingRepository) DeleteMapping(ctx context.Context, tenantID, id string) (err error) {
	_, deferLog := logging.WhenDB(ctx, "MappingRepository.DeleteMapping", id)
	defer func() { deferLog(nil, &err) }()
	result := r.pg.DB().WithContext(ctx).
		Where("id = ? AND tenant_id = ?", id, tenantID).
		Delete(&DeviceMappingModel{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

// DisableMapping sets status='disabled' for the row identified by (tenantID, id).
// Returns gorm.ErrRecordNotFound when no row was updated.
func (r *MappingRepository) DisableMapping(ctx context.Context, tenantID, id string) (err error) {
	_, deferLog := logging.WhenDB(ctx, "MappingRepository.DisableMapping", id)
	defer func() { deferLog(nil, &err) }()
	result := r.pg.DB().WithContext(ctx).
		Model(&DeviceMappingModel{}).
		Where("id = ? AND tenant_id = ?", id, tenantID).
		Update("status", "disabled")
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

// FindByExternalID returns the row matching (tenantID, externalSystem, externalID).
// Returns gorm.ErrRecordNotFound when no match is found.
// The caller is responsible for checking the returned Status field.
func (r *MappingRepository) FindByExternalID(
	ctx context.Context,
	tenantID, externalSystem, externalID string,
) (result *DeviceMappingModel, err error) {
	_, deferLog := logging.WhenDB(ctx, "MappingRepository.FindByExternalID", externalID)
	defer func() { deferLog(result, &err) }()
	var m DeviceMappingModel
	err = r.pg.DB().WithContext(ctx).
		Where("tenant_id = ? AND external_system = ? AND external_id = ?",
			tenantID, externalSystem, externalID).
		First(&m).Error
	if err != nil {
		return nil, err
	}
	return &m, nil
}

// FindByCUCode returns the first active mapping for (tenantID, cuCode).
// Only rows with status='active' are considered; disabled mappings are ignored.
// Returns gorm.ErrRecordNotFound when no active match is found.
func (r *MappingRepository) FindByCUCode(
	ctx context.Context,
	tenantID, cuCode string,
) (result *DeviceMappingModel, err error) {
	_, deferLog := logging.WhenDB(ctx, "MappingRepository.FindByCUCode", cuCode)
	defer func() { deferLog(result, &err) }()
	var m DeviceMappingModel
	err = r.pg.DB().WithContext(ctx).
		Where("tenant_id = ? AND cu_code = ? AND status = ?", tenantID, cuCode, "active").
		First(&m).Error
	if err != nil {
		return nil, err
	}
	return &m, nil
}

// ListByTenantID returns all mapping rows for a tenant, regardless of status.
func (r *MappingRepository) ListByTenantID(
	ctx context.Context,
	tenantID string,
) (results []*DeviceMappingModel, err error) {
	_, deferLog := logging.WhenDB(ctx, "MappingRepository.ListByTenantID", tenantID)
	defer func() { deferLog(results, &err) }()
	err = r.pg.DB().WithContext(ctx).
		Where("tenant_id = ?", tenantID).
		Order("created_at DESC").
		Find(&results).Error
	return
}
