package postgres

import (
	"context"

	"github.com/mushroomyuan/vpp-backend/platform/logging"
	"github.com/mushroomyuan/vpp-backend/resource/infrastructure/persistent/postgres/builder"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type PointRepository struct {
	pg *Postgres
}

func NewPointRepository(pg *Postgres) *PointRepository {
	return &PointRepository{pg: pg}
}

func (r *PointRepository) CreatePoint(ctx context.Context, m *PointModel) (err error) {
	_, deferLog := logging.WhenDB(ctx, "PointRepository.CreatePoint", m)
	defer func() { deferLog(m, &err) }()
	return r.pg.DB().WithContext(ctx).Clauses(clause.Returning{}).Create(m).Error
}

func (r *PointRepository) BatchCreatePoint(ctx context.Context, ms []*PointModel) (err error) {
	_, deferLog := logging.WhenDB(ctx, "PointRepository.BatchCreatePoint", ms)
	defer func() { deferLog(nil, &err) }()
	return r.pg.DB().WithContext(ctx).CreateInBatches(ms, 500).Error
}

func (r *PointRepository) UpdatePoint(ctx context.Context, m *PointModel) (err error) {
	_, deferLog := logging.WhenDB(ctx, "PointRepository.UpdatePoint", m)
	defer func() { deferLog(nil, &err) }()
	result := r.pg.DB().WithContext(ctx).
		Model(&PointModel{}).
		Where("id = ?", m.ID).
		Updates(map[string]any{
			"point_key":         m.PointKey,
			"external_address":  m.ExternalAddress,
			"data_type":         m.DataType,
			"ext_config":        m.ExtConfig,
			"description":       m.Description,
			"control_flag":      m.ControlFlag,
			"is_virtual":        m.IsVirtual,
			"safety_thresholds": m.SafetyThresholds,
			"cache_key_alias":   m.CacheKeyAlias,
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (r *PointRepository) FindPointByID(ctx context.Context, query *builder.Point) (result *PointModel, err error) {
	_, deferLog := logging.WhenDB(ctx, "PointRepository.FindPointByID", query)
	defer func() { deferLog(result, &err) }()
	var m PointModel
	err = query.Fill(r.pg.DB().WithContext(ctx)).First(&m).Error
	if err != nil {
		return nil, err
	}
	result = &m
	return
}

func (r *PointRepository) ListPoints(ctx context.Context, query *builder.Point) (results []*PointModel, err error) {
	_, deferLog := logging.WhenDB(ctx, "PointRepository.ListPoints", query)
	defer func() { deferLog(results, &err) }()
	err = query.Fill(r.pg.DB().WithContext(ctx)).Find(&results).Error
	return
}

func (r *PointRepository) SoftDeletePoint(ctx context.Context, query *builder.Point) (err error) {
	_, deferLog := logging.WhenDB(ctx, "PointRepository.SoftDeletePoint", query)
	defer func() { deferLog(nil, &err) }()
	result := query.Fill(r.pg.DB().WithContext(ctx)).Delete(&PointModel{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

// BatchDeletePoint soft-deletes multiple points scoped to a tenant (via resources join).
func (r *PointRepository) BatchDeletePoint(ctx context.Context, tenantID string, ids []string) (err error) {
	_, deferLog := logging.WhenDB(ctx, "PointRepository.BatchDeletePoint", ids)
	defer func() { deferLog(nil, &err) }()
	if len(ids) == 0 {
		return nil
	}
	q := builder.NewPoint().TenantID(tenantID).IDs(ids...)
	return q.Fill(r.pg.DB().WithContext(ctx)).Delete(&PointModel{}).Error
}
