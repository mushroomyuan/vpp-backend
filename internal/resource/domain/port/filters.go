package port

type BaseFilter struct {
	TenantID string
	Offset   int
	Limit    int
}

// ---- Site ----
type SiteFilter struct {
	BaseFilter
	IDs      []string // 可选
	Status   []string // 运行状态过滤
	NameLike string   // 模糊搜索
}

// ---- Resource ----
type ResourceFilter struct {
	BaseFilter
	SiteID   string   // 必须：站点下的资源
	IDs      []string // 可选
	Types    []string // 可选：PV / ESS / Load / Wind ...
	NameLike string   // 模糊搜索
}

// ---- CU ----
type CUFilter struct {
	BaseFilter
	SiteID     string   // 可选
	ResourceID string   // 可选：资源下的 CU
	ParentCUID string   // 支持嵌套聚合
	Capability []string // ["frequency_regulation","peak_shaving"]
	IDs        []string
	NameLike   string
}

// ---- Point ----
type PointFilter struct {
	BaseFilter
	SiteID    string // 可选
	CUID      string
	PointKeys []string
	IsVirtual *bool
	DataTypes []string
	IDs       []string
}
