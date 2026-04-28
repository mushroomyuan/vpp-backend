package builder

import (
	"github.com/mushroomyuan/vpp-backend/platform/util"
	"gorm.io/gorm"
)

// Site builds a GORM query for the sites table.
// All setter methods return *Site to allow method chaining.
type Site struct {
	tenantID string
	ids      []string
	statuses []int8
	nameLike string
	limit    int
	offset   int
}

func NewSite() *Site { return &Site{} }

func (s *Site) TenantID(v string) *Site          { s.tenantID = v; return s }
func (s *Site) IDs(v ...string) *Site            { s.ids = v; return s }
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

// Fill applies all stored conditions to db and returns the modified chain.
// Pagination (LIMIT/OFFSET) is applied only when Paginate has been called.
func (s *Site) Fill(db *gorm.DB) *gorm.DB {
	db = db.Order("created_at DESC")
	if s.tenantID != "" {
		db = db.Where("tenant_id = ?", s.tenantID)
	}
	if len(s.ids) > 0 {
		db = db.Where("id IN ?", s.ids)
	}
	if len(s.statuses) > 0 {
		db = db.Where("status IN ?", s.statuses)
	}
	if s.nameLike != "" {
		db = db.Where("name ILIKE ?", "%"+s.nameLike+"%")
	}
	if s.limit > 0 {
		db = db.Limit(s.limit).Offset(s.offset)
	}
	return db
}

// FormatArg implements logging.ArgFormatter so WhenDB can log the query params.
func (s *Site) FormatArg() (string, error) {
	return util.MarshalString(s)
}

var siteStatusNameToInt = map[string]int8{
	"under_construction": 1,
	"operating":          2,
	"fault":              3,
	"offline":            4,
}
