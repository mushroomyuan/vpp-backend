package postgres

import (
	"context"

	"github.com/mushroomyuan/vpp-backend/platform/logging"
	"github.com/mushroomyuan/vpp-backend/resource/infrastructure/persistent/postgres/builder"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type ResourceRepository struct {
	pg *Postgres
}

func NewResourceRepository(pg *Postgres) *ResourceRepository {
	return &ResourceRepository{pg: pg}
}

func (r *ResourceRepository) CreateResource(ctx context.Context, m *ResourceModel) (err error) {
	_, deferLog := logging.WhenDB(ctx, "ResourceRepository.CreateResource", m)
	defer func() { deferLog(m, &err) }()
	return r.pg.DB().WithContext(ctx).Clauses(clause.Returning{}).Create(m).Error
}

func (r *ResourceRepository) BatchCreateResource(ctx context.Context, ms []*ResourceModel) (err error) {
	_, deferLog := logging.WhenDB(ctx, "ResourceRepository.BatchCreateResource", ms)
	defer func() { deferLog(nil, &err) }()
	return r.pg.DB().WithContext(ctx).CreateInBatches(ms, 500).Error
}

func (r *ResourceRepository) UpdateResource(ctx context.Context, m *ResourceModel) (err error) {
	_, deferLog := logging.WhenDB(ctx, "ResourceRepository.UpdateResource", m)
	defer func() { deferLog(nil, &err) }()
	result := r.pg.DB().WithContext(ctx).
		Model(&ResourceModel{}).
		Where("id = ? AND tenant_id = ?", m.ID, m.TenantID).
		Updates(map[string]any{
			"name":         m.Name,
			"type":         m.Type,
			"capacity":     m.Capacity,
			"manufacturer": m.Manufacturer,
			"model":        m.Model,
			"metadata":     m.Metadata,
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (r *ResourceRepository) FindResourceByID(ctx context.Context, query *builder.Resource) (result *ResourceModel, err error) {
	_, deferLog := logging.WhenDB(ctx, "ResourceRepository.FindResourceByID", query)
	defer func() { deferLog(result, &err) }()
	var m ResourceModel
	err = query.Fill(r.pg.DB().WithContext(ctx)).First(&m).Error
	if err != nil {
		return nil, err
	}
	result = &m
	return
}

func (r *ResourceRepository) ListResources(ctx context.Context, query *builder.Resource) (results []*ResourceModel, err error) {
	_, deferLog := logging.WhenDB(ctx, "ResourceRepository.ListResources", query)
	defer func() { deferLog(results, &err) }()
	err = query.Fill(r.pg.DB().WithContext(ctx)).Find(&results).Error
	return
}

func (r *ResourceRepository) SoftDeleteResource(ctx context.Context, query *builder.Resource) (err error) {
	_, deferLog := logging.WhenDB(ctx, "ResourceRepository.SoftDeleteResource", query)
	defer func() { deferLog(nil, &err) }()
	result := query.Fill(r.pg.DB().WithContext(ctx)).Delete(&ResourceModel{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

// BatchDeleteResource soft-deletes multiple resources for a tenant in one statement.
func (r *ResourceRepository) BatchDeleteResource(ctx context.Context, tenantID string, ids []string) (err error) {
	_, deferLog := logging.WhenDB(ctx, "ResourceRepository.BatchDeleteResource", ids)
	defer func() { deferLog(nil, &err) }()
	if len(ids) == 0 {
		return nil
	}
	q := builder.NewResource().TenantID(tenantID).IDs(ids...)
	return q.Fill(r.pg.DB().WithContext(ctx)).Delete(&ResourceModel{}).Error
}
