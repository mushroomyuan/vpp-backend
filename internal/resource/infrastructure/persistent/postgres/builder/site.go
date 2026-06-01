package builder

import (
	"github.com/mushroomyuan/vpp-backend/platform/util"
	"gorm.io/gorm"
)

// Site builds a GORM query for the sites extension table.
type Site struct {
	tenantID string
	nodeIDs  []string // sites.node_id (same as site node id)
	statuses []int8   // operating_status
	nameLike string   // nodes.display_name
	limit    int
	offset   int
}

func NewSite() *Site { return &Site{} }

func (s *Site) TenantID(v string) *Site          { s.tenantID = v; return s }
func (s *Site) IDs(v ...string) *Site            { s.nodeIDs = v; return s }
func (s *Site) NameLike(v string) *Site          { s.nameLike = v; return s }
func (s *Site) Paginate(limit, offset int) *Site { s.limit = limit; s.offset = offset; return s }

// StatusNames converts human-readable status strings to their DB int8 values.
// Unrecognised strings are silently ignored.
func (s *Site) StatusNames(names ...string) *Site {
	for _, name := range names {
		if v, ok := siteStatusNameToInt[name]; ok {
			s.statuses = append(s.statuses, v)
		}
	}
	return s
}

func (s *Site) Fill(db *gorm.DB) *gorm.DB {
	db = db.Table("sites").Order("sites.created_at DESC")
	if s.tenantID != "" {
		db = db.Where("sites.tenant_id = ?", s.tenantID)
	}
	if len(s.nodeIDs) > 0 {
		db = db.Where("sites.node_id IN ?", s.nodeIDs)
	}
	if len(s.statuses) > 0 {
		db = db.Where("sites.operating_status IN ?", s.statuses)
	}
	if s.nameLike != "" {
		db = db.Joins(`JOIN nodes AS sn ON sn.id = sites.node_id AND sn.deleted_at IS NULL`).
			Where("sn.display_name ILIKE ?", "%"+s.nameLike+"%").
			Select("sites.*")
	}
	if s.limit > 0 {
		db = db.Limit(s.limit).Offset(s.offset)
	}
	return db
}

func (s *Site) FormatArg() (string, error) {
	return util.MarshalString(s)
}

var siteStatusNameToInt = map[string]int8{
	"under_construction": 1,
	"operating":          2,
	"fault":              3,
	"offline":            4,
}
