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
	return r.pg.DB().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		constraint := m.SafetyConstraint
		m.SafetyConstraint = nil
		if err := tx.Clauses(clause.Returning{}).Create(m).Error; err != nil {
			return err
		}
		m.SafetyConstraint = constraint
		if constraint != nil {
			return tx.Create(constraint).Error
		}
		return nil
	})
}

func (r *PointRepository) BatchCreatePoint(ctx context.Context, ms []*PointModel) (err error) {
	_, deferLog := logging.WhenDB(ctx, "PointRepository.BatchCreatePoint", ms)
	defer func() { deferLog(nil, &err) }()
	return r.pg.DB().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		constraints := make([]*PointSafetyConstraintModel, 0)
		byPointID := make(map[string]*PointSafetyConstraintModel)
		for _, m := range ms {
			if m.SafetyConstraint != nil {
				constraints = append(constraints, m.SafetyConstraint)
				byPointID[m.ID] = m.SafetyConstraint
				m.SafetyConstraint = nil
			}
		}
		defer func() {
			for _, m := range ms {
				m.SafetyConstraint = byPointID[m.ID]
			}
		}()
		if err := tx.CreateInBatches(ms, 500).Error; err != nil {
			return err
		}
		if len(constraints) > 0 {
			if err := tx.CreateInBatches(constraints, 500).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func (r *PointRepository) UpdatePoint(ctx context.Context, m *PointModel) (err error) {
	_, deferLog := logging.WhenDB(ctx, "PointRepository.UpdatePoint", m)
	defer func() { deferLog(nil, &err) }()
	return r.pg.DB().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		updates := map[string]any{
			"metric_id":        m.MetricID,
			"external_address": m.ExternalAddress,
			"access_mode":      m.AccessMode,
			"scale":            m.Scale,
			"offset":           m.Offset,
			"enabled":          m.Enabled,
			"revision":         m.Revision,
			"tenant_id":        m.TenantID,
			"node_id":          m.NodeID,
			"asset_id":         m.AssetID,
			"cu_id":            m.CUID,
		}
		result := tx.Model(&PointModel{}).
			Where("id = ? AND tenant_id = ? AND revision = ?", m.ID, m.TenantID, m.Revision-1).
			Updates(updates)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			var count int64
			if err := tx.Model(&PointModel{}).
				Where("id = ? AND tenant_id = ?", m.ID, m.TenantID).
				Count(&count).Error; err != nil {
				return err
			}
			if count == 0 {
				return gorm.ErrRecordNotFound
			}
			return ErrOptimisticLock
		}
		if m.SafetyConstraint == nil {
			return tx.Where("point_id = ?", m.ID).Delete(&PointSafetyConstraintModel{}).Error
		}
		return tx.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "point_id"}},
			UpdateAll: true,
		}).Create(m.SafetyConstraint).Error
	})
}

func (r *PointRepository) FindPointByID(ctx context.Context, query *builder.Point) (result *PointModel, err error) {
	_, deferLog := logging.WhenDB(ctx, "PointRepository.FindPointByID", query)
	defer func() { deferLog(result, &err) }()
	var m PointModel
	err = query.Fill(r.pg.DB().WithContext(ctx)).Preload("SafetyConstraint").First(&m).Error
	if err != nil {
		return nil, err
	}
	result = &m
	return
}

func (r *PointRepository) ListPoints(ctx context.Context, query *builder.Point) (results []*PointModel, totalCount int64, err error) {
	_, deferLog := logging.WhenDB(ctx, "PointRepository.ListPoints", query)
	defer func() { deferLog(results, &err) }()

	countDB := query.Fill(r.pg.DB().WithContext(ctx)).Session(&gorm.Session{}).Limit(-1).Offset(-1)
	if err = countDB.Count(&totalCount).Error; err != nil {
		return
	}
	if totalCount == 0 {
		return nil, 0, nil
	}

	err = query.Fill(r.pg.DB().WithContext(ctx)).Preload("SafetyConstraint").Find(&results).Error
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

// BatchDeletePoint deletes points scoped by tenant.
func (r *PointRepository) BatchDeletePoint(ctx context.Context, tenantID string, ids []string) (err error) {
	_, deferLog := logging.WhenDB(ctx, "PointRepository.BatchDeletePoint", ids)
	defer func() { deferLog(nil, &err) }()
	if len(ids) == 0 {
		return nil
	}
	q := builder.NewPoint().TenantID(tenantID).IDs(ids...)
	return q.Fill(r.pg.DB().WithContext(ctx)).Delete(&PointModel{}).Error
}
