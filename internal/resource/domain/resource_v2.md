
// =====================================================
// 主表：统一节点表
// =====================================================

type Node struct {
	ID       string
	TenantID string

	// 树结构核心字段
	ParentID string

	DisplayName string

	// site/resource/cu/point
	Type string
    SubType string
	//draft active disabled archived deleted
	LifecycleStatus string
	Description string
    Path string   // /A/B/C
    Depth int

	Metadata map[string]any

	Version int64

	DeletedAt *time.Time
	DeletedBy string
	DeleteJobID string
    DeleteReason  string 
    RestoredAt    *time.Time  // 支持恢复
	RestoredBy    string

	CreatedAt time.Time
	UpdatedAt time.Time
}

// ========================================
// Resource Service Domain Model
// 业务背景：
// - EMS 负责底层设备管理
// - VPP 负责资源聚合、调度与控制
// ========================================

// ----------------------------------------
// Site（站点）
// 表示资源归属主体：电站、园区、楼宇、充电站等
// ----------------------------------------
type Site struct {
	NodeID       string
    TenantID 	 string
    // online offline maintenance closed construction
    OperatingStatus OperatingStatus

	Location Location
    Version int64
}

// ----------------------------------------
// Asset（资源）
// VPP 内部逻辑资源对象，可参与调度
// 例如：储能资源、光伏资源、柔性负荷资源
// ----------------------------------------
type Asset struct {
	NodeID       string
    TenantID 	 string
    DispatchStatus  DispatchStatus

	// 额定容量（kW）
	RatedCapacityKW float64

	// 调度模式：
	// centralized（集中控制）
	// autonomous（自治控制）
	// semi_auto（半自动）
	DispatchMode string
    EnergyType string // battery pv load charger
	OwnerType  string // self third_party customer
	// 是否允许参与市场交易 / 调度
	MarketEnabled bool
    
    Version int64
}

// ----------------------------------------
// CU（Control Unit，控制单元）
// 外部系统（EMS/SCADA/IoT）向 VPP 暴露的控制接入单元
// 是控制命令和状态采集的边界对象
// ----------------------------------------
type CU struct {
	NodeID       string
    TenantID 	 string
    ConnStatus   ConnStatus


	// 来源系统：
	// ems_a / ems_b / scada / iot_platform
	Provider string

	// 上游系统中的唯一标识
	ExternalID string

	// 接入协议：
	// modbus_tcp / mqtt / opcua / rest
	Protocol string
    ProtocolConfig map[string]any  // 连接参数
    // 或结构化
    Connection ConnectionConfig

	// 控制能力标签：
	// frequency_regulation（调频）
	// peak_shaving（削峰）
	// charge_control（充放电控制）
	CapabilityTags []string
    
    Version int64
}

// ----------------------------------------
// Point（点位）
// 标准化遥测点 / 遥信点 / 遥控点 / 设定点
// ----------------------------------------
type Point struct {
	ID string
    TenantID 	 string
    SiteID   string
    AssetID  string 
    CUID string

	// 标准业务键，例如：
	// read_active_power
	// read_soc
	// write_power_setpoint
	// start_cmd
	PointKey string

	// 上游原始地址：
	// 寄存器地址 / MQTT Topic / JSON Path / Tag 名称
	ExternalAddress string

	DataType DataType

	// 点位类别：
	// telemetry（遥测）
	// status（状态）
	// command（控制命令）
	// setpoint（设定值）
	PointClass string

	// 是否允许控制面写入
 	WritableMode string

	// 是否虚拟点（算法计算点）
	IsVirtual bool

	// 单位：
	// kW / kWh / % / V / A
	Unit string

	// 扩展配置：
	// 系数、偏移量、采样频率、读写权限等
	ExtConfig map[string]any

	// 安全阈值：
	// 例如 {"max_power":500}
	SafetyThresholds map[string]any

	// 缓存别名（Redis Key）
	CacheKeyAlias string

}

// ============================================================
// Asset Runtime
// 资源运行态（调度最关心）
// Redis Key: asset:{asset_id}:runtime
// ============================================================

type AssetRuntime struct {
	AssetID   string
	TenantID  string

	// 当前运行状态
	Online bool

	// 当前输出功率（kW）
	CurrentPowerKW float64

	// 当前可调容量（kW）
	AvailablePowerKW float64

	// 当前荷电状态（储能适用，0~100）
	SOC *float64

	// 当前是否允许参与调度
	Dispatchable bool
    // 增加：原因描述，方便排查为什么不能调度
    NotDispatchableReason string
    // 建议：区分充电/放电能力，VPP 核心需求
    MaxChargePowerKW    float64
    MaxDischargePowerKW float64

	// 最近一次状态更新时间
	UpdatedAt time.Time
}

// ============================================================
// CU Runtime
// 控制单元运行态（接入层最关心）
// Redis Key: cu:{cu_id}:runtime
// ============================================================

type CURuntime struct {
	CUID      string
	TenantID  string


	// connected / disconnected / degraded
	ConnStatus string

	// 最近心跳时间
	LastSeenAt time.Time

	// 最近通讯延迟（毫秒）
	LatencyMS *int64

	// 最近错误信息（可选）
	LastError string


	UpdatedAt time.Time
}

// ============================================================
// Point Runtime
// 点位最新值（最新态缓存）
// Redis Key: point:{point_id}:runtime
// ============================================================

type PointRuntime struct {
	PointID   string
	TenantID  string

	// 最新值（统一字符串存储，兼容 bool/int/float/string）
	Value string

	// 数值型值（可选，便于调度快速计算）
	NumericValue *float64

	// GOOD / BAD / STALE / OFFLINE
	QualityStatus string
    
    // 增加：数据版本号或序列号，防止乱序覆盖
    Sequence int64

	// 最新采样时间（设备上报时间）
	SampledAt time.Time

	// 写入缓存时间
	UpdatedAt time.Time
}

// ----------------------------------------
// 公共类型
// ----------------------------------------
type Location struct {
	Address   string
	Latitude  float64
	Longitude float64
}

type OperatingStatus string
type DispatchStatus  string
type ConnStatus string
type QualityStatus string
type DataType string

type SafetyThreshold struct {
    ThresholdType string  // max_power/min_soc
    Value         float64
    Action        string  // alarm/block/limit
    Severity      string  // warning/error/critical
}

type ConnectionConfig struct {
    Host         string
    Port         int
    Timeout      int
    RetryPolicy  RetryPolicy
}



//
// ============================================================
// 六、审计日志（横切基础设施能力）
// ============================================================

// AuditLog 记录谁在什么时候做了什么。
type AuditLog struct {
	ID string

	// 每次请求唯一操作号
	OperationID string

	// 批量任务时可关联 job
	JobID string

	TenantID string

	OperatorID string
	OperatorName string

	ServiceName string // resource-service

	// create / update / delete / restore / import
	Action string

	ResourceType string // node / relation / job
	ResourceID   string
	ChangedFields []string
	// 修改前快照
	BeforeData map[string]any

	// 修改后快照
	AfterData map[string]any

	// success / failed
	Status string

	ErrorMessage string

	IPAddress string

	CreatedAt time.Time
}