package adapters

import (
	"context"
	"errors"

	"github.com/mushroomyuan/vpp-backend/resource/domain"
	"github.com/mushroomyuan/vpp-backend/resource/domain/model"
	"github.com/mushroomyuan/vpp-backend/resource/domain/port"
	"github.com/mushroomyuan/vpp-backend/resource/infrastructure/persistent/postgres"
	"github.com/mushroomyuan/vpp-backend/resource/infrastructure/persistent/postgres/builder"
	"gorm.io/gorm"
)

type SiteRepositoryPostgres struct {
	repo *postgres.SiteRepository
}

func NewSiteRepositoryPostgres(repo *postgres.SiteRepository) *SiteRepositoryPostgres {
	return &SiteRepositoryPostgres{repo: repo}
}

var _ port.SiteRepository = (*SiteRepositoryPostgres)(nil)

func (r *SiteRepositoryPostgres) Create(ctx context.Context, s *model.Site) (*model.Site, error) {
	row, err := SiteDomainToDB(s)
	if err != nil {
		return nil, err
	}
	if err := r.repo.CreateSite(ctx, row); err != nil {
		return nil, err
	}
	res, err := SiteDBToDomain(row)
	if err != nil {
		return nil, err
	}
	return res, nil
}

func (r *SiteRepositoryPostgres) Update(ctx context.Context, s *model.Site) error {
	row, err := SiteDomainToDB(s)
	if err != nil {
		return err
	}
	err = r.repo.UpdateSite(ctx, row)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return domain.ErrSiteNotFound
	}
	return err
}

func (r *SiteRepositoryPostgres) FindByID(ctx context.Context, tenantID, id string) (*model.Site, error) {
	row, err := r.repo.FindSiteByID(ctx,
		builder.NewSite().TenantID(tenantID).IDs(id),
	)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, domain.ErrSiteNotFound
	}
	if err != nil {
		return nil, err
	}
	return SiteDBToDomain(row)
}

func (r *SiteRepositoryPostgres) List(ctx context.Context, f port.SiteFilter) ([]*model.Site, error) {
	q := builder.NewSite().
		TenantID(f.TenantID).
		IDs(f.IDs...).
		StatusNames(f.Status...).
		NameLike(f.NameLike).
		Paginate(f.Limit, f.Offset)
	rows, err := r.repo.ListSites(ctx, q)
	if err != nil {
		return nil, err
	}
	return BatchSiteDBToDomain(rows)
}

func (r *SiteRepositoryPostgres) SoftDelete(ctx context.Context, tenantID, id string) error {
	err := r.repo.SoftDeleteSite(ctx,
		builder.NewSite().TenantID(tenantID).IDs(id),
	)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return domain.ErrSiteNotFound
	}
	return err
}
