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
		Where("node_id = ? AND tenant_id = ?", m.NodeID, m.TenantID).
		Updates(map[string]any{
			"provider":        m.Provider,
			"external_id":     m.ExternalID,
			"protocol":        m.Protocol,
			"protocol_config": m.ProtocolConfig,
			"connection":      m.Connection,
			"capability_tags": m.CapabilityTags,
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
	err = query.Fill(r.pg.DB().WithContext(ctx), true).First(&m).Error
	if err != nil {
		return nil, err
	}
	result = &m
	return
}

func (r *CURepository) ListCUs(ctx context.Context, query *builder.CU) (results []*CUModel, totalCount int64, err error) {
	_, deferLog := logging.WhenDB(ctx, "CURepository.ListCUs", query)
	defer func() { deferLog(results, &err) }()

	countDB := query.Fill(r.pg.DB().WithContext(ctx), false).Session(&gorm.Session{}).Limit(-1).Offset(-1)
	if err = countDB.Count(&totalCount).Error; err != nil {
		return
	}
	if totalCount == 0 {
		return nil, 0, nil
	}

	err = query.Fill(r.pg.DB().WithContext(ctx), true).Find(&results).Error
	return
}

func (r *CURepository) SoftDeleteCU(ctx context.Context, query *builder.CU) (err error) {
	_, deferLog := logging.WhenDB(ctx, "CURepository.SoftDeleteCU", query)
	defer func() { deferLog(nil, &err) }()
	result := query.Fill(r.pg.DB().WithContext(ctx), false).Delete(&CUModel{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

// BatchDeleteCU deletes extension rows for the given CU node ids scoped by tenant.
func (r *CURepository) BatchDeleteCU(ctx context.Context, tenantID string, nodeIDs []string) (err error) {
	_, deferLog := logging.WhenDB(ctx, "CURepository.BatchDeleteCU", nodeIDs)
	defer func() { deferLog(nil, &err) }()
	if len(nodeIDs) == 0 {
		return nil
	}
	return r.pg.DB().WithContext(ctx).
		Model(&CUModel{}).
		Where("node_id IN ? AND tenant_id = ?", nodeIDs, tenantID).
		Delete(&CUModel{}).Error
}
