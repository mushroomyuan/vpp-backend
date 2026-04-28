package builder

import (
	"encoding/json"

	"github.com/mushroomyuan/vpp-backend/platform/util"
	"gorm.io/gorm"
)

// CU builds a GORM query for the cus table.
// When TenantID or SiteID is set, a JOIN onto the resources table is added
// automatically because the cus table carries neither of those columns.
type CU struct {
	tenantID     string
	siteID       string
	resourceID   string
	parentCUID   string
	ids          []string
	capabilities []string // JSONB @> containment filter
	nameLike     string
	limit        int
	offset       int
}

func NewCU() *CU { return &CU{} }

func (c *CU) TenantID(v string) *CU          { c.tenantID = v; return c }
func (c *CU) SiteID(v string) *CU            { c.siteID = v; return c }
func (c *CU) ResourceID(v string) *CU        { c.resourceID = v; return c }
func (c *CU) ParentCUID(v string) *CU        { c.parentCUID = v; return c }
func (c *CU) IDs(v ...string) *CU            { c.ids = v; return c }
func (c *CU) Capabilities(v ...string) *CU   { c.capabilities = v; return c }
func (c *CU) NameLike(v string) *CU          { c.nameLike = v; return c }
func (c *CU) Paginate(limit, offset int) *CU { c.limit = limit; c.offset = offset; return c }

func (c *CU) Fill(db *gorm.DB) *gorm.DB {
	db = db.Order("cus.created_at DESC")

	// tenant_id and site_id live on resources; join only when needed
	if c.tenantID != "" || c.siteID != "" {
		db = db.Joins("JOIN resources ON resources.id = cus.resource_id AND resources.deleted_at IS NULL")
		if c.tenantID != "" {
			db = db.Where("resources.tenant_id = ?", c.tenantID)
		}
		if c.siteID != "" {
			db = db.Where("resources.site_id = ?", c.siteID)
		}
	}
	if c.resourceID != "" {
		db = db.Where("cus.resource_id = ?", c.resourceID)
	}
	if c.parentCUID != "" {
		db = db.Where("cus.parent_cu_id = ?", c.parentCUID)
	}
	if len(c.ids) > 0 {
		db = db.Where("cus.id IN ?", c.ids)
	}
	if c.nameLike != "" {
		db = db.Where("cus.name ILIKE ?", "%"+c.nameLike+"%")
	}
	// capability_tags is a JSONB array; @> ensures all requested tags are present
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
