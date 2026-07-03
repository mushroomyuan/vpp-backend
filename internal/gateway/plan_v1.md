# Gateway 服务设计方案（v1，结合项目现状改进版）

## 一、服务定位

Gateway 是 VPP 与外部系统（EMS / IoT Platform）的集成边界，同时也是内部调度服务向外部系统下发指令的出口。

**职责：**
- 接收外部系统遥测数据，转换并转发到 telemetry 服务
- 接收内部调度指令（来自 dispatch），转换后发送给外部系统
- 管理外部设备 ID 与内部 CU 标识（CUCode）的映射关系

**不负责：**
- 资源管理（resource 服务）
- 时序存储（telemetry 服务）
- 调度算法（dispatch 服务）

### 双协议入站设计

Gateway 需要同时对外暴露两类接口，对应两类调用方：

| 接口 | 协议 | 调用方 | 原因 |
|------|------|--------|------|
| 遥测接收、映射管理 | HTTP | EMS / IoT Platform（外部系统） | 外部系统通常只支持 REST |
| 指令下发 | gRPC | dispatch（内部服务） | 内部服务间统一使用 gRPC |

> 项目内所有内部服务间通信均使用 gRPC（resource `:5002`、telemetry `:5003`），dispatch 调用 gateway 也应遵循此约定，而不是 HTTP。HTTP 仅面向外部系统。

---

## 二、Proto 定义

### 新增 `api/gateway/proto/root.proto`

```
api/gateway/proto/
├── root.proto          # GatewayService gRPC 定义
├── buf.yaml
├── buf.gen.yaml
├── buf.lock
└── gen/                # 生成的 .pb.go 文件（codegen 产物）
```

`root.proto` 核心定义（内部 gRPC，供 dispatch 调用）：

```proto
syntax = "proto3";
package gatewaypb;

option go_package = "github.com/mushroomyuan/vpp-backend/api/gateway/proto/gen;gatewaypb";

service GatewayService {
  // ExecuteCommand 由内部 dispatch 服务调用，将调度指令转发至外部 EMS
  rpc ExecuteCommand(ExecuteCommandRequest) returns (ExecuteCommandResponse);
}

message ExecuteCommandRequest {
  string TenantID  = 1;
  string CUCode    = 2;   // 内部标识，gateway 负责查映射转为外部 ID
  string Command   = 3;   // e.g. "set_power"
  double Value     = 4;
}

message ExecuteCommandResponse {
  string ExternalID = 1;  // 实际下发的外部设备 ID（用于日志追踪）
}
```

---

## 三、目录结构

与 resource、telemetry 服务完全对齐的六边形 + CQRS 布局：

```
internal/gateway/
├── cmd/main.go
├── app.go                                # NewApp("vpp-gateway")
├── run.go                                # runApp()，config loading
├── server.go                             # createServer() 组合根（含 gRPC + HTTP 双服务器）
├── go.mod                                # module github.com/mushroomyuan/vpp-backend/gateway
├── config/config.go                      # Config struct
├── options/options.go                    # Viper mapstructure + defaults + Validate()
│
├── domain/
│   ├── model/
│   │   ├── device_mapping.go             # DeviceMapping（含 status 字段）
│   │   ├── external_telemetry.go         # ExternalTelemetry + ExternalMetric
│   │   └── standard_telemetry.go        # StandardTelemetry（对齐 IngestTelemetry command）
│   ├── port/
│   │   ├── mapping_repository.go         # 本地 DB 接口
│   │   ├── telemetry_client.go           # gRPC 出站接口（调 telemetry 服务）
│   │   └── ems_client.go                 # EMS 接口（v1: log-only）
│   └── errors.go
│
├── application/
│   ├── app.go
│   └── command/
│       ├── receive_telemetry.go          # HTTP 入站 → telemetry gRPC 出站
│       ├── execute_command.go            # gRPC 入站（dispatch）→ EMS 出站
│       ├── create_mapping.go
│       ├── delete_mapping.go
│       └── disable_mapping.go            # 手动禁用映射（v1 stale mapping 兜底）
│
├── adapter/
│   ├── inbound/
│   │   ├── grpc/
│   │   │   └── handler.go               # GatewayService gRPC 实现（ExecuteCommand）
│   │   └── http/
│   │       ├── router.go
│   │       ├── telemetry_handler.go
│   │       └── mapping_handler.go
│   └── outbound/
│       ├── postgres/
│       │   └── mapping_repository.go    # port.MappingRepository GORM 实现
│       ├── telemetry_grpc/
│       │   └── client.go               # port.TelemetryClient gRPC 实现
│       └── ems_log/
│           └── client.go               # port.EMSClient log-only 实现
│
└── infrastructure/persistent/postgres/
    ├── db.go                            # GORM init（复用 resource 模式）
    ├── models.go                        # DeviceMappingModel
    └── mapping_repository.go
```

新增配置和迁移文件（在 monorepo 根目录）：
- `config/gateway.yaml`
- `migrations/gateway/000001_init.up.sql`
- `migrations/initdb/40-gateway-schema.sql`

---

## 四、领域模型

### DeviceMapping（补全 TenantID、CUCode、Status）

```go
// domain/model/device_mapping.go

type MappingStatus string

const (
    MappingStatusActive   MappingStatus = "active"
    MappingStatusDisabled MappingStatus = "disabled"  // 手动禁用，v1 stale mapping 兜底
)

type DeviceMapping struct {
    ID             string        // UUID v7（platform/idgen）
    TenantID       string
    ExternalSystem string        // e.g. "ems-sg"
    ExternalID     string        // EMS 侧设备 ID，如 "SG001"
    CUCode         string        // telemetry 服务的 CU 标识（非 resource UUID）
    Status         MappingStatus // active | disabled
    CreatedAt      time.Time
    UpdatedAt      time.Time
}
```

> **CUCode 而非 ResourceID**：telemetry 服务使用 `(TenantID, CUCode)` 寻址，而非 resource 服务的 UUID 节点 ID。gateway 查询映射后直接构造 `IngestTelemetry` command，无需再查 resource 服务。

> **Status 字段**：v1 通过手动 `DisableMapping` API 处理 stale mapping；v2 通过 Kafka 事件自动同步（见第七节）。

### ExternalTelemetry（多指标，对齐 telemetry 服务 batch 结构）

```go
// domain/model/external_telemetry.go

type ExternalMetric struct {
    Name  string
    Value float64
}

type ExternalTelemetry struct {
    TenantID       string
    ExternalSystem string
    ExternalID     string
    Timestamp      time.Time
    Metrics        []ExternalMetric
}
```

### StandardTelemetry（对齐 `command.IngestTelemetry`）

telemetry 服务的入站命令结构（`internal/telemetry/application/command/ingest_telemetry.go`）：

```go
type IngestTelemetry struct {
    TenantID  string
    CUCode    string
    Timestamp time.Time
    Metrics   []MetricInput  // {Name, Value, Type MetricType, Quality QualityStatus}
}
```

gateway 的 StandardTelemetry 与之对应：

```go
// domain/model/standard_telemetry.go

type MetricValue struct {
    Name    string
    Value   float64
    Type    string  // "ANALOG" | "DISCRETE"，v1 默认 "ANALOG"
    Quality string  // "GOOD" | "BAD"，v1 默认 "GOOD"
}

type StandardTelemetry struct {
    TenantID  string
    CUCode    string
    Timestamp time.Time
    Metrics   []MetricValue
}
```

---

## 五、Ports（依赖接口）

```go
// domain/port/mapping_repository.go
type MappingRepository interface {
    Create(ctx context.Context, m *model.DeviceMapping) error
    Delete(ctx context.Context, tenantID, id string) error
    Disable(ctx context.Context, tenantID, id string) error
    GetByExternalID(ctx context.Context, tenantID, externalSystem, externalID string) (*model.DeviceMapping, error)
    GetByCUCode(ctx context.Context, tenantID, cuCode string) (*model.DeviceMapping, error)
    List(ctx context.Context, tenantID string) ([]*model.DeviceMapping, error)
}

// domain/port/telemetry_client.go
type TelemetryClient interface {
    Ingest(ctx context.Context, t *model.StandardTelemetry) error
}

// domain/port/ems_client.go
type EMSClient interface {
    SendCommand(ctx context.Context, externalSystem, externalID, command string, value float64) error
}
```

v1 **无 ResourcePort**：mapping 存 gateway 本地 DB，无需调用 resource gRPC。

---

## 六、Application 层（CQRS，复用 platform/decorator）

所有 handler 使用 `decorator.ApplyCommandDecorators` 包装（与 telemetry 服务一致）。

### ReceiveTelemetry（HTTP 入站 → telemetry gRPC 出站）

```
ExternalTelemetry{tenantID, externalSystem, externalID, metrics}
    → MappingRepository.GetByExternalID
    → 校验 mapping.Status == active，否则返回 ErrMappingDisabled
    → 构建 StandardTelemetry（type=ANALOG, quality=GOOD）
    → TelemetryClient.Ingest
```

### ExecuteCommand（gRPC 入站，由 dispatch 调用 → EMS 出站）

```
ExecuteCommandRequest{tenantID, cuCode, command, value}
    → MappingRepository.GetByCUCode
    → 校验 mapping.Status == active
    → EMSClient.SendCommand(externalSystem, externalID, command, value)
```

### CreateMapping / DeleteMapping / DisableMapping

- `CreateMapping`：生成 UUID v7，status=active，存 DB
- `DeleteMapping`：物理删除
- `DisableMapping`：设 status=disabled（v1 stale mapping 手动兜底）

---

## 七、Stale Mapping 问题与演进策略

### 问题描述

当 resource 服务中的 CU 被删除或停用后，gateway 本地 `device_mappings` 表中的记录不会自动失效，可能导致：
- 继续接收并写入已停用 CU 的遥测数据
- 向已不存在的外部设备继续下发指令

### v1 处理方式（手动兜底）

依赖 `DeviceMapping.status` 字段和管理 API：
- 运维通过 `PATCH /api/v1/tenants/:tenant_id/mappings/:id/disable` 手动禁用映射
- `GetByExternalID` 和 `GetByCUCode` 均过滤 `status != active` 的记录
- 适用于低频变更场景，接受一定时间窗口内的数据一致性风险

### v2 事件驱动同步（有前置依赖）

**前置条件**：resource 服务需先向 Kafka 发布 CU 生命周期事件（当前 resource 服务**未实现**此功能）。

完整演进路径：

```
Step 1：改造 resource 服务
    CU 被 disable / delete 时，发布事件到 Kafka topic: vpp.resource.events
    事件结构：{ type: "CULifecycleChanged", tenantID, cuCode, status: "disabled"|"deleted" }

Step 2：gateway 新增 Kafka consumer
    订阅 vpp.resource.events
    收到事件后自动 disable / delete 对应 DeviceMapping
    实现位置：adapter/inbound/kafka/lifecycle_consumer.go
```

> v2 不修改核心领域模型，仅新增一个 inbound adapter。

---

## 八、接口设计

### HTTP API（外部，供 EMS 调用）

| 方法 | 路由 | 用途 |
|------|------|------|
| POST | `/api/v1/tenants/:tenant_id/telemetry:ingest` | 接收 EMS 遥测数据 |
| POST | `/api/v1/tenants/:tenant_id/mappings` | 创建设备映射 |
| DELETE | `/api/v1/tenants/:tenant_id/mappings/:id` | 删除映射 |
| PATCH | `/api/v1/tenants/:tenant_id/mappings/:id/disable` | 禁用映射（stale mapping 兜底） |
| GET | `/api/v1/tenants/:tenant_id/mappings` | 查询映射列表 |

HTTP engine 使用 `platform/server.NewGinEngine`（OTel + 日志中间件，纯 Gin，无 grpc-gateway）。

**遥测接收请求示例：**

```json
{
  "external_system": "ems-sg",
  "external_id": "SG001",
  "timestamp": "2026-06-29T10:00:00Z",
  "metrics": [
    {"name": "p_act", "value": 100.5},
    {"name": "q_act", "value": 20.0}
  ]
}
```

### gRPC API（内部，供 dispatch 调用）

`GatewayService.ExecuteCommand`，定义见 `api/gateway/proto/root.proto`（第二节）。

---

## 九、数据库模型与迁移

### GORM Model（对齐 resource 服务 postgres/models.go 风格）

```go
// infrastructure/persistent/postgres/models.go
type DeviceMappingModel struct {
    ID             string    `gorm:"primaryKey;type:varchar(36)"`
    TenantID       string    `gorm:"not null;index:idx_tenant"`
    ExternalSystem string    `gorm:"not null"`
    ExternalID     string    `gorm:"not null"`
    CUCode         string    `gorm:"not null"`
    Status         string    `gorm:"not null;default:active"`
    CreatedAt      time.Time
    UpdatedAt      time.Time
}
```

### 迁移文件（`migrations/gateway/000001_init.up.sql`）

```sql
CREATE TABLE IF NOT EXISTS device_mappings (
    id              VARCHAR(36)  PRIMARY KEY,
    tenant_id       VARCHAR(64)  NOT NULL,
    external_system VARCHAR(64)  NOT NULL,
    external_id     VARCHAR(128) NOT NULL,
    cu_code         VARCHAR(128) NOT NULL,
    status          VARCHAR(16)  NOT NULL DEFAULT 'active',
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    UNIQUE (tenant_id, external_system, external_id)
);

CREATE INDEX IF NOT EXISTS idx_device_mappings_cu
    ON device_mappings(tenant_id, cu_code);
CREATE INDEX IF NOT EXISTS idx_device_mappings_status
    ON device_mappings(tenant_id, status);
```

---

## 十、出站 Adapter：TelemetryClient gRPC

`go.mod` replace 指令引用已有 proto：

```
replace (
    github.com/mushroomyuan/vpp-backend/api/gateway/proto/gen  => ../../api/gateway/proto/gen
    github.com/mushroomyuan/vpp-backend/api/telemetry/proto/gen => ../../api/telemetry/proto/gen
    github.com/mushroomyuan/vpp-backend/platform               => ../platform
)
```

实现：

```go
// adapter/outbound/telemetry_grpc/client.go
func (c *telemetryGRPCClient) Ingest(ctx context.Context, t *model.StandardTelemetry) error {
    req := &telemetrypb.IngestTelemetryRequest{
        TenantID:  t.TenantID,
        CUCode:    t.CUCode,
        Timestamp: timestamppb.New(t.Timestamp),
        Metrics:   mapMetrics(t.Metrics),
    }
    _, err := c.client.IngestTelemetry(ctx, req)
    return err
}
```

---

## 十一、配置（`config/gateway.yaml`）

```yaml
tracing:
  endpoint: "127.0.0.1:4317"
  insecure: true

gateway:
  service-name: vpp-gateway
  grpc-addr: ":5004"         # 内部 gRPC（dispatch 调用）
  http-addr: ":8083"          # 外部 HTTP（EMS 调用）
  metrics-addr: ":9104"
  consul-addr: "127.0.0.1:8500"

database:
  driver: postgres
  host: 127.0.0.1
  port: 5432
  user: postgres
  password: postgres
  dbname: gateway
  params:
    sslmode: disable
    TimeZone: Asia/Shanghai

telemetry-grpc:
  addr: "127.0.0.1:5003"
```

端口规划延续现有约定：

| 服务 | gRPC | HTTP | Metrics |
|------|------|------|---------|
| resource | :5002 | :8082 | :9102 |
| telemetry | :5003 | — | :9103 |
| gateway | :5004 | :8083 | :9104 |

---

## 十二、server.go 组合根

Gateway 同时启动 gRPC（内部）和 HTTP（外部）两个服务器，与 resource 服务的双服务器模式一致：

```go
type gatewayServer struct {
    grpcSrv       *googlegrpc.Server   // 内部：dispatch 调用
    httpSrv       *http.Server         // 外部：EMS 调用
    app           application.Application
    cfg           *config.Config
    metricsClient *metrics.Client
    metricsCancel context.CancelFunc
}

// createServer 组合根：
// 1. metrics.New
// 2. postgres.NewPostgres(dbCfg) → MappingRepository
// 3. grpc.NewClient(telemetryGRPCAddr) → TelemetryGRPCClient
// 4. ems_log.NewClient() → EMSLogClient
// 5. application.NewApplication(...)
// 6. gRPC server：platform/server.NewGRPCServer + RegisterGatewayServiceServer
// 7. HTTP server：platform/server.NewGinEngine + 路由注册
```

shutdown 顺序与 resource/telemetry 一致：停止接受流量（gRPC GracefulStop + HTTP Shutdown）→ metricsCancel。

---

## 十三、服务调用关系

```
外部系统（EMS）
      │ HTTP POST telemetry
      ▼
┌─────────────┐   gRPC IngestTelemetry    ┌─────────────┐
│   gateway   │ ─────────────────────────▶│  telemetry  │
│  :8083 HTTP │                           │  :5003 gRPC │
│  :5004 gRPC │◀─────────────────────────│             │
└─────────────┘   gRPC ExecuteCommand     └─────────────┘
      ▲           （来自 dispatch）
      │
dispatch 服务（未来）
      │
      ▼
  EMSClient（v1: log-only → v2: 真实 EMS 适配器）
```

---

## 十四、v1 实现范围

**实现：**
- `api/gateway/proto/` 定义 `GatewayService`（含 codegen）
- `DeviceMapping` CRUD + DisableMapping（Postgres，GORM）
- `ReceiveTelemetry`（HTTP → mapping 查询 → telemetry gRPC）
- `ExecuteCommand`（gRPC 入站 → mapping 查询 → log 输出）
- `TelemetryClient` gRPC 实现
- `EMSClient` log-only 实现
- 双服务器（gRPC `:5004` + HTTP `:8083`）
- 完整启动链、metrics、OTel tracing、Consul 注册
- `config/gateway.yaml`
- `migrations/gateway/000001_init.up.sql`

**不实现（明确推迟）：**
- 多 EMS adapter（未来加 `adapter/outbound/ems_xxx/`，不改核心模型）
- Kafka consumer（需 resource 服务先发布 CU 生命周期事件，v2 实现）
- Resource 服务 gRPC 调用（CU 存在性校验，v2 可选）
- 动态协议转换框架

---

## 十五、Makefile

```makefile
run-gateway:
    cd internal/gateway && go run ./cmd/main.go -c ../../config/gateway.yaml
```
