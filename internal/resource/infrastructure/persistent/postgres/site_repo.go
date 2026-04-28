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
		Where("id = ? AND tenant_id = ?", s.ID, s.TenantID).
		Updates(map[string]any{
			"name":        s.Name,
			"location":    s.Location,
			"description": s.Description,
			"status":      s.Status,
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

func (r *SiteRepository) ListSites(ctx context.Context, query *builder.Site) (results []*SiteModel, err error) {
	_, deferLog := logging.WhenDB(ctx, "SiteRepository.ListSites", query)
	defer func() { deferLog(results, &err) }()
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
