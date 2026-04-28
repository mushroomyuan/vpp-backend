package builder

import (
	"github.com/mushroomyuan/vpp-backend/platform/util"
	"gorm.io/gorm"
)

// Point builds a GORM query for the points table.
// When TenantID or SiteID is set, a JOIN onto the resources table is added
// automatically because the points table carries neither of those columns.
type Point struct {
	tenantID  string
	siteID    string
	cuID      string
	ids       []string
	pointKeys []string
	dataTypes []string
	isVirtual *bool
	limit     int
	offset    int
}

func NewPoint() *Point { return &Point{} }

func (p *Point) TenantID(v string) *Point          { p.tenantID = v; return p }
func (p *Point) SiteID(v string) *Point            { p.siteID = v; return p }
func (p *Point) CUID(v string) *Point              { p.cuID = v; return p }
func (p *Point) IDs(v ...string) *Point            { p.ids = v; return p }
func (p *Point) PointKeys(v ...string) *Point      { p.pointKeys = v; return p }
func (p *Point) DataTypes(v ...string) *Point      { p.dataTypes = v; return p }
func (p *Point) IsVirtual(v bool) *Point           { p.isVirtual = &v; return p }
func (p *Point) Paginate(limit, offset int) *Point { p.limit = limit; p.offset = offset; return p }

func (p *Point) Fill(db *gorm.DB) *gorm.DB {
	db = db.Order("points.created_at DESC")

	if p.tenantID != "" || p.siteID != "" {
		db = db.Joins("JOIN resources ON resources.id = points.resource_id AND resources.deleted_at IS NULL")
		if p.tenantID != "" {
			db = db.Where("resources.tenant_id = ?", p.tenantID)
		}
		if p.siteID != "" {
			db = db.Where("resources.site_id = ?", p.siteID)
		}
	}
	if p.cuID != "" {
		db = db.Where("points.cu_id = ?", p.cuID)
	}
	if len(p.ids) > 0 {
		db = db.Where("points.id IN ?", p.ids)
	}
	if len(p.pointKeys) > 0 {
		db = db.Where("points.point_key IN ?", p.pointKeys)
	}
	if len(p.dataTypes) > 0 {
		db = db.Where("points.data_type IN ?", p.dataTypes)
	}
	if p.isVirtual != nil {
		db = db.Where("points.is_virtual = ?", *p.isVirtual)
	}
	if p.limit > 0 {
		db = db.Limit(p.limit).Offset(p.offset)
	}
	return db
}

func (p *Point) FormatArg() (string, error) {
	return util.MarshalString(p)
}
