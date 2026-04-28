package builder

import (
	"github.com/mushroomyuan/vpp-backend/platform/util"
	"gorm.io/gorm"
)

// Resource builds a GORM query for the resources table.
type Resource struct {
	tenantID string
	siteID   string
	ids      []string
	types    []string
	nameLike string
	limit    int
	offset   int
}

func NewResource() *Resource { return &Resource{} }

func (r *Resource) TenantID(v string) *Resource { r.tenantID = v; return r }
func (r *Resource) SiteID(v string) *Resource   { r.siteID = v; return r }
func (r *Resource) IDs(v ...string) *Resource   { r.ids = v; return r }
func (r *Resource) Types(v ...string) *Resource { r.types = v; return r }
func (r *Resource) NameLike(v string) *Resource { r.nameLike = v; return r }
func (r *Resource) Paginate(limit, offset int) *Resource {
	r.limit = limit
	r.offset = offset
	return r
}

func (r *Resource) Fill(db *gorm.DB) *gorm.DB {
	db = db.Order("created_at DESC")
	if r.tenantID != "" {
		db = db.Where("tenant_id = ?", r.tenantID)
	}
	if r.siteID != "" {
		db = db.Where("site_id = ?", r.siteID)
	}
	if len(r.ids) > 0 {
		db = db.Where("id IN ?", r.ids)
	}
	if len(r.types) > 0 {
		db = db.Where("type IN ?", r.types)
	}
	if r.nameLike != "" {
		db = db.Where("name ILIKE ?", "%"+r.nameLike+"%")
	}
	if r.limit > 0 {
		db = db.Limit(r.limit).Offset(r.offset)
	}
	return db
}

func (r *Resource) FormatArg() (string, error) {
	return util.MarshalString(r)
}
