package builder

import (
	"github.com/mushroomyuan/vpp-backend/platform/util"
	"gorm.io/gorm"
)

// Point builds a GORM query for the points table.
// TenantID filters points.tenant_id. SiteID restricts rows to points under assets whose parent is the given site node.
type Point struct {
	tenantID   string
	siteID     string
	cuID       string
	ids        []string
	metricIDs  []string
	accessMode []string
	enabled    *bool
	limit      int
	offset     int
}

func NewPoint() *Point { return &Point{} }

func (p *Point) TenantID(v string) *Point          { p.tenantID = v; return p }
func (p *Point) SiteID(v string) *Point            { p.siteID = v; return p }
func (p *Point) CUID(v string) *Point              { p.cuID = v; return p }
func (p *Point) IDs(v ...string) *Point            { p.ids = v; return p }
func (p *Point) MetricIDs(v ...string) *Point      { p.metricIDs = v; return p }
func (p *Point) AccessModes(v ...string) *Point    { p.accessMode = v; return p }
func (p *Point) Enabled(v bool) *Point             { p.enabled = &v; return p }
func (p *Point) Paginate(limit, offset int) *Point { p.limit = limit; p.offset = offset; return p }

func (p *Point) Fill(db *gorm.DB) *gorm.DB {
	db = db.Table("points").Order("points.created_at DESC")
	if p.tenantID != "" {
		db = db.Where("points.tenant_id = ?", p.tenantID)
	}
	if p.siteID != "" {
		db = db.Joins(`JOIN nodes AS asset_node ON asset_node.id = points.asset_id AND asset_node.type = 'asset' AND asset_node.deleted_at IS NULL`).
			Where("asset_node.parent_id = ?", p.siteID).
			Select("points.*")
	}
	if p.cuID != "" {
		db = db.Where("points.cu_id = ?", p.cuID)
	}
	if len(p.ids) > 0 {
		db = db.Where("points.id IN ?", p.ids)
	}
	if len(p.metricIDs) > 0 {
		db = db.Where("points.metric_id IN ?", p.metricIDs)
	}
	if len(p.accessMode) > 0 {
		db = db.Where("points.access_mode IN ?", p.accessMode)
	}
	if p.enabled != nil {
		db = db.Where("points.enabled = ?", *p.enabled)
	}
	if p.limit > 0 {
		db = db.Limit(p.limit).Offset(p.offset)
	}
	return db
}

func (p *Point) FormatArg() (string, error) {
	return util.MarshalString(p)
}
