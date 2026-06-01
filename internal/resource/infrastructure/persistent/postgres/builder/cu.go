package builder

import (
	"encoding/json"

	"github.com/mushroomyuan/vpp-backend/platform/util"
	"gorm.io/gorm"
)

// CU builds a GORM query for the cus extension table.
// TenantID filters cus.tenant_id. SiteID / assetID / ParentCUID use nodes joins
// (site node id, parent asset node id, parent node id respectively).
type CU struct {
	tenantID     string
	siteID       string
	assetID      string // asset node id (CU node parent in tree)
	parentCUID   string // parent node id (another CU or asset)
	ids          []string
	capabilities []string
	nameLike     string
	limit        int
	offset       int
}

func NewCU() *CU { return &CU{} }

func (c *CU) TenantID(v string) *CU          { c.tenantID = v; return c }
func (c *CU) SiteID(v string) *CU            { c.siteID = v; return c }
func (c *CU) AssetID(v string) *CU           { c.assetID = v; return c }
func (c *CU) ParentCUID(v string) *CU        { c.parentCUID = v; return c }
func (c *CU) IDs(v ...string) *CU            { c.ids = v; return c }
func (c *CU) Capabilities(v ...string) *CU   { c.capabilities = v; return c }
func (c *CU) NameLike(v string) *CU          { c.nameLike = v; return c }
func (c *CU) Paginate(limit, offset int) *CU { c.limit = limit; c.offset = offset; return c }

// Fill builds filters, joins, order, and pagination against table cus.
//
// When scanCUSRows is true and the query joins nodes (site/asset/parent/name filters),
// SELECT cus.* is added so query results map only to CUModel columns; joined tables also
// expose tenant_id / created_at / updated_at and would collide with cus.* without it.
//
// Use scanCUSRows=false for COUNT and DELETE: GORM turns SELECT into count(...) and
// PostgreSQL rejects count(cus.*) ("column cus.* does not exist").
func (c *CU) Fill(db *gorm.DB, scanCUSRows bool) *gorm.DB {
	db = db.Table("cus").Order("cus.created_at DESC")
	if c.tenantID != "" {
		db = db.Where("cus.tenant_id = ?", c.tenantID)
	}

	joinedCuNode := false
	ensureCuNode := func() {
		if !joinedCuNode {
			db = db.Joins(`JOIN nodes AS cu_node ON cu_node.id = cus.node_id AND cu_node.deleted_at IS NULL`)
			joinedCuNode = true
		}
	}

	if c.assetID != "" {
		ensureCuNode()
		db = db.Where("cu_node.parent_id = ?", c.assetID)
	}
	if c.parentCUID != "" {
		ensureCuNode()
		db = db.Where("cu_node.parent_id = ?", c.parentCUID)
	}
	if c.siteID != "" {
		ensureCuNode()
		db = db.Joins(`JOIN nodes AS asset_node ON asset_node.id = cu_node.parent_id AND asset_node.type = 'asset' AND asset_node.deleted_at IS NULL`).
			Where("asset_node.parent_id = ?", c.siteID)
	}
	if c.nameLike != "" {
		ensureCuNode()
		db = db.Where("cu_node.display_name ILIKE ?", "%"+c.nameLike+"%")
	}
	if len(c.ids) > 0 {
		db = db.Where("cus.node_id IN ?", c.ids)
	}
	if joinedCuNode && scanCUSRows {
		db = db.Select("cus.*")
	}
	if len(c.capabilities) > 0 {
		caps, _ := json.Marshal(c.capabilities)
		db = db.Where("cus.capability_tags @> ?::jsonb", string(caps))
	}
	if c.limit > 0 {
		db = db.Limit(c.limit).Offset(c.offset)
	}
	return db
}

func (c *CU) FormatArg() (string, error) {
	return util.MarshalString(c)
}
