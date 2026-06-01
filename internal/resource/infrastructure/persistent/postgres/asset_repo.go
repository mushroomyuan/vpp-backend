package postgres

import (
	"context"

	"github.com/mushroomyuan/vpp-backend/platform/logging"
	"github.com/mushroomyuan/vpp-backend/resource/infrastructure/persistent/postgres/builder"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type AssetRepository struct {
	pg *Postgres
}

func NewAssetRepository(pg *Postgres) *AssetRepository {
	return &AssetRepository{pg: pg}
}

func (r *AssetRepository) CreateAsset(ctx context.Context, m *AssetModel) (err error) {
	_, deferLog := logging.WhenDB(ctx, "AssetRepository.CreateAsset", m)
	defer func() { deferLog(m, &err) }()
	return r.pg.DB().WithContext(ctx).Clauses(clause.Returning{}).Create(m).Error
}

func (r *AssetRepository) BatchCreateAsset(ctx context.Context, ms []*AssetModel) (err error) {
	_, deferLog := logging.WhenDB(ctx, "AssetRepository.BatchCreateAsset", ms)
	defer func() { deferLog(nil, &err) }()
	return r.pg.DB().WithContext(ctx).CreateInBatches(ms, 500).Error
}

func (r *AssetRepository) UpdateAsset(ctx context.Context, m *AssetModel) (err error) {
	_, deferLog := logging.WhenDB(ctx, "AssetRepository.UpdateAsset", m)
	defer func() { deferLog(nil, &err) }()
	result := r.pg.DB().WithContext(ctx).
		Model(&AssetModel{}).
		Where("node_id = ? AND tenant_id = ?", m.NodeID, m.TenantID).
		Updates(map[string]any{
			"dispatch_status":   m.DispatchStatus,
			"rated_capacity_kw": m.RatedCapacityKW,
			"dispatch_mode":     m.DispatchMode,
			"energy_type":       m.EnergyType,
			"owner_type":        m.OwnerType,
			"market_enabled":    m.MarketEnabled,
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (r *AssetRepository) FindAssetByID(ctx context.Context, query *builder.Asset) (result *AssetModel, err error) {
	_, deferLog := logging.WhenDB(ctx, "AssetRepository.FindAssetByID", query)
	defer func() { deferLog(result, &err) }()
	var m AssetModel
	err = query.Fill(r.pg.DB().WithContext(ctx)).First(&m).Error
	if err != nil {
		return nil, err
	}
	result = &m
	return
}

func (r *AssetRepository) ListAssets(ctx context.Context, query *builder.Asset) (results []*AssetModel, totalCount int64, err error) {
	_, deferLog := logging.WhenDB(ctx, "AssetRepository.ListAssets", query)
	defer func() { deferLog(results, &err) }()

	countDB := query.Fill(r.pg.DB().WithContext(ctx)).Session(&gorm.Session{}).Limit(-1).Offset(-1)
	if err = countDB.Count(&totalCount).Error; err != nil {
		return
	}
	if totalCount == 0 {
		return nil, 0, nil
	}

	err = query.Fill(r.pg.DB().WithContext(ctx)).Find(&results).Error
	return
}

func (r *AssetRepository) DeleteAsset(ctx context.Context, query *builder.Asset) (err error) {
	_, deferLog := logging.WhenDB(ctx, "AssetRepository.DeleteAsset", query)
	defer func() { deferLog(nil, &err) }()
	result := query.Fill(r.pg.DB().WithContext(ctx)).Delete(&AssetModel{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (r *AssetRepository) SoftDeleteAsset(ctx context.Context, query *builder.Asset) (err error) {
	return r.DeleteAsset(ctx, query)
}

func (r *AssetRepository) BatchDeleteAsset(ctx context.Context, tenantID string, nodeIDs []string) (err error) {
	_, deferLog := logging.WhenDB(ctx, "AssetRepository.BatchDeleteAsset", nodeIDs)
	defer func() { deferLog(nil, &err) }()
	if len(nodeIDs) == 0 {
		return nil
	}
	q := builder.NewAsset().TenantID(tenantID).IDs(nodeIDs...)
	return q.Fill(r.pg.DB().WithContext(ctx)).Delete(&AssetModel{}).Error
}
