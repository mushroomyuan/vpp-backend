package postgres

import (
	"context"

	"github.com/mushroomyuan/vpp-backend/platform/logging"
	"gorm.io/gorm"
)

type CUCapabilityRepository struct {
	pg *Postgres
}

func NewCUCapabilityRepository(pg *Postgres) *CUCapabilityRepository {
	return &CUCapabilityRepository{pg: pg}
}

func (r *CUCapabilityRepository) Create(ctx context.Context, row *CUCapabilityModel) (err error) {
	_, deferLog := logging.WhenDB(ctx, "CUCapabilityRepository.Create", row)
	defer func() { deferLog(row, &err) }()
	return r.pg.DB().WithContext(ctx).Create(row).Error
}

func (r *CUCapabilityRepository) Update(ctx context.Context, row *CUCapabilityModel) (err error) {
	_, deferLog := logging.WhenDB(ctx, "CUCapabilityRepository.Update", row)
	defer func() { deferLog(nil, &err) }()
	result := r.pg.DB().WithContext(ctx).Model(&CUCapabilityModel{}).
		Where("id = ? AND tenant_id = ? AND version = ?", row.ID, row.TenantID, row.Version-1).
		Updates(map[string]any{
			"schema_version": row.SchemaVersion,
			"spec":           row.Spec,
			"enabled":        row.Enabled,
			"version":        row.Version,
			"updated_at":     row.UpdatedAt,
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		var count int64
		if err := r.pg.DB().WithContext(ctx).Model(&CUCapabilityModel{}).
			Where("id = ? AND tenant_id = ?", row.ID, row.TenantID).
			Count(&count).Error; err != nil {
			return err
		}
		if count == 0 {
			return gorm.ErrRecordNotFound
		}
		return ErrOptimisticLock
	}
	return nil
}

func (r *CUCapabilityRepository) FindByID(
	ctx context.Context,
	tenantID, id string,
) (row *CUCapabilityModel, err error) {
	_, deferLog := logging.WhenDB(ctx, "CUCapabilityRepository.FindByID", id)
	defer func() { deferLog(row, &err) }()
	var result CUCapabilityModel
	err = r.pg.DB().WithContext(ctx).
		Where("tenant_id = ? AND id = ?", tenantID, id).
		First(&result).Error
	if err != nil {
		return nil, err
	}
	return &result, nil
}

func (r *CUCapabilityRepository) List(
	ctx context.Context,
	tenantID, cuID string,
	capabilityIDs []string,
	enabledOnly bool,
) (rows []*CUCapabilityModel, err error) {
	_, deferLog := logging.WhenDB(ctx, "CUCapabilityRepository.List", cuID)
	defer func() { deferLog(rows, &err) }()
	query := r.pg.DB().WithContext(ctx).Where("tenant_id = ?", tenantID)
	if cuID != "" {
		query = query.Where("cu_id = ?", cuID)
	}
	if len(capabilityIDs) > 0 {
		query = query.Where("capability_id IN ?", capabilityIDs)
	}
	if enabledOnly {
		query = query.Where("enabled = TRUE")
	}
	err = query.Order("capability_id ASC").Find(&rows).Error
	return rows, err
}

func (r *CUCapabilityRepository) Delete(ctx context.Context, tenantID, id string) (err error) {
	_, deferLog := logging.WhenDB(ctx, "CUCapabilityRepository.Delete", id)
	defer func() { deferLog(nil, &err) }()
	result := r.pg.DB().WithContext(ctx).
		Where("tenant_id = ? AND id = ?", tenantID, id).
		Delete(&CUCapabilityModel{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}
