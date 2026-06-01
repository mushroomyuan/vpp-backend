package postgres

import (
	"context"

	"github.com/mushroomyuan/vpp-backend/platform/logging"
	"github.com/mushroomyuan/vpp-backend/resource/infrastructure/persistent/postgres/builder"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type SiteRepository struct {
	pg *Postgres
}

func NewSiteRepository(pg *Postgres) *SiteRepository {
	return &SiteRepository{pg: pg}
}

func (r *SiteRepository) CreateSite(ctx context.Context, s *SiteModel) (err error) {
	_, deferLog := logging.WhenDB(ctx, "SiteRepository.CreateSite", s)
	defer func() { deferLog(s, &err) }()
	return r.pg.DB().WithContext(ctx).Clauses(clause.Returning{}).Create(s).Error
}

func (r *SiteRepository) UpdateSite(ctx context.Context, s *SiteModel) (err error) {
	_, deferLog := logging.WhenDB(ctx, "SiteRepository.UpdateSite", s)
	defer func() { deferLog(nil, &err) }()
	result := r.pg.DB().WithContext(ctx).
		Model(&SiteModel{}).
		Where("node_id = ? AND tenant_id = ?", s.NodeID, s.TenantID).
		Updates(map[string]any{
			"location":         s.Location,
			"operating_status": s.OperatingStatus,
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (r *SiteRepository) FindSiteByID(ctx context.Context, query *builder.Site) (result *SiteModel, err error) {
	_, deferLog := logging.WhenDB(ctx, "SiteRepository.FindSiteByID", query)
	defer func() { deferLog(result, &err) }()
	var m SiteModel
	err = query.Fill(r.pg.DB().WithContext(ctx)).First(&m).Error
	if err != nil {
		return nil, err
	}
	result = &m
	return
}

func (r *SiteRepository) ListSites(ctx context.Context, query *builder.Site) (results []*SiteModel, totalCount int64, err error) {
	_, deferLog := logging.WhenDB(ctx, "SiteRepository.ListSites", query)
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

func (r *SiteRepository) SoftDeleteSite(ctx context.Context, query *builder.Site) (err error) {
	_, deferLog := logging.WhenDB(ctx, "SiteRepository.SoftDeleteSite", query)
	defer func() { deferLog(nil, &err) }()
	result := query.Fill(r.pg.DB().WithContext(ctx)).Delete(&SiteModel{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}
