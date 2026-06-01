package builder

import (
	"github.com/mushroomyuan/vpp-backend/platform/util"
	"gorm.io/gorm"
)

// Asset builds a GORM query for the assets extension table.
// Filters that need display name or subtype join the nodes row (type = asset).
type Asset struct {
	tenantID string
	siteID   string // site node id (parent of asset nodes)
	nodeIDs  []string
	subTypes []string // nodes.sub_type
	nameLike string
	limit    int
	offset   int
}

func NewAsset() *Asset { return &Asset{} }

func (a *Asset) TenantID(v string) *Asset { a.tenantID = v; return a }
func (a *Asset) SiteID(v string) *Asset   { a.siteID = v; return a }
func (a *Asset) IDs(v ...string) *Asset   { a.nodeIDs = v; return a }
func (a *Asset) Types(v ...string) *Asset {
	a.subTypes = v
	return a
}
func (a *Asset) NameLike(v string) *Asset { a.nameLike = v; return a }
func (a *Asset) Paginate(limit, offset int) *Asset {
	a.limit = limit
	a.offset = offset
	return a
}

func (a *Asset) Fill(db *gorm.DB) *gorm.DB {
	db = db.Table("assets").Order("assets.created_at DESC")
	if a.tenantID != "" {
		db = db.Where("assets.tenant_id = ?", a.tenantID)
	}
	if a.siteID != "" {
		db = db.Joins(`JOIN nodes AS asset_node ON asset_node.id = assets.node_id AND asset_node.type = 'asset' AND asset_node.deleted_at IS NULL`).
			Where("asset_node.parent_id = ?", a.siteID)
	}
	if len(a.nodeIDs) > 0 {
		db = db.Where("assets.node_id IN ?", a.nodeIDs)
	}
	needNodeJoin := len(a.subTypes) > 0 || a.nameLike != ""
	if needNodeJoin {
		db = db.Joins(`JOIN nodes AS n ON n.id = assets.node_id AND n.deleted_at IS NULL`)
		db = db.Select("assets.*")
	}
	if len(a.subTypes) > 0 {
		db = db.Where("n.sub_type IN ?", a.subTypes)
	}
	if a.nameLike != "" {
		db = db.Where("n.display_name ILIKE ?", "%"+a.nameLike+"%")
	}
	if a.limit > 0 {
		db = db.Limit(a.limit).Offset(a.offset)
	}
	return db
}

func (a *Asset) FormatArg() (string, error) {
	return util.MarshalString(a)
}
