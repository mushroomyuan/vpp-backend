package postgres

import (
	"context"

	"github.com/mushroomyuan/vpp-backend/platform/logging"
	"github.com/mushroomyuan/vpp-backend/resource/infrastructure/persistent/postgres/builder"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type CURepository struct {
	pg *Postgres
}

func NewCURepository(pg *Postgres) *CURepository {
	return &CURepository{pg: pg}
}

func (r *CURepository) CreateCU(ctx context.Context, m *CUModel) (err error) {
	_, deferLog := logging.WhenDB(ctx, "CURepository.CreateCU", m)
	defer func() { deferLog(m, &err) }()
	return r.pg.DB().WithContext(ctx).Clauses(clause.Returning{}).Create(m).Error
}

func (r *CURepository) BatchCreateCU(ctx context.Context, ms []*CUModel) (err error) {
	_, deferLog := logging.WhenDB(ctx, "CURepository.BatchCreateCU", ms)
	defer func() { deferLog(nil, &err) }()
	return r.pg.DB().WithContext(ctx).CreateInBatches(ms, 500).Error
}

func (r *CURepository) UpdateCU(ctx context.Context, m *CUModel) (err error) {
	_, deferLog := logging.WhenDB(ctx, "CURepository.UpdateCU", m)
	defer func() { deferLog(nil, &err) }()
	result := r.pg.DB().WithContext(ctx).
		Model(&CUModel{}).
		Where("id = ?", m.ID).
		Updates(map[string]any{
			"parent_cu_id":    m.ParentCUID,
			"name":            m.Name,
			"type":            m.Type,
			"capability_tags": m.CapabilityTags,
			"metadata":        m.Metadata,
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (r *CURepository) FindCUByID(ctx context.Context, query *builder.CU) (result *CUModel, err error) {
	_, deferLog := logging.WhenDB(ctx, "CURepository.FindCUByID", query)
	defer func() { deferLog(result, &err) }()
	var m CUModel
	err = query.Fill(r.pg.DB().WithContext(ctx)).First(&m).Error
	if err != nil {
		return nil, err
	}
	result = &m
	return
}

func (r *CURepository) ListCUs(ctx context.Context, query *builder.CU) (results []*CUModel, err error) {
	_, deferLog := logging.WhenDB(ctx, "CURepository.ListCUs", query)
	defer func() { deferLog(results, &err) }()
	err = query.Fill(r.pg.DB().WithContext(ctx)).Find(&results).Error
	return
}

func (r *CURepository) SoftDeleteCU(ctx context.Context, query *builder.CU) (err error) {
	_, deferLog := logging.WhenDB(ctx, "CURepository.SoftDeleteCU", query)
	defer func() { deferLog(nil, &err) }()
	result := query.Fill(r.pg.DB().WithContext(ctx)).Delete(&CUModel{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

// BatchDeleteCU soft-deletes multiple CUs. Tenant isolation is enforced via a
// subquery on the resources table since the cus table has no tenant_id column.
func (r *CURepository) BatchDeleteCU(ctx context.Context, tenantID string, ids []string) (err error) {
	_, deferLog := logging.WhenDB(ctx, "CURepository.BatchDeleteCU", ids)
	defer func() { deferLog(nil, &err) }()
	if len(ids) == 0 {
		return nil
	}
	subQuery := r.pg.DB().Model(&ResourceModel{}).
		Where("tenant_id = ?", tenantID).
		Select("id")
	return r.pg.DB().WithContext(ctx).
		Model(&CUModel{}).
		Where("id IN ? AND resource_id IN (?)", ids, subQuery).
		Delete(&CUModel{}).Error
}
