# vpp-resource

VPP 平台的**资源管理服务**。负责维护 VPP 内部完整的资产层级树（Site → Asset → CU → Point），提供资源配置、生命周期管理与批量导入能力。

---

## 目录

- [服务职责](#服务职责)
- [架构设计](#架构设计)
- [数据存储](#数据存储)
- [API 接口](#api-接口)
- [目录结构](#目录结构)
- [依赖组件](#依赖组件)
- [启动方式](#启动方式)
- [关键设计约定](#关键设计约定)

---

## 服务职责

| 职责 | 说明 |
|---|---|
| **资产树管理** | 维护 Site / Asset / CU / Point 四层资源层级，支持创建、更新、删除、移动、重命名 |
| **生命周期管理** | 资源状态流转（active / inactive / decommissioned），变更时发布 Kafka 事件 |
| **批量导入** | 通过异步 ImportWorker 支持大批量 Asset / CU / Point 的后台导入，带进度跟踪与重试 |
| **运行时状态** | 以 Redis 为热存储，聚合展示 Asset / CU / Point 的实时运行状态（功率、连接状态、点位值等） |
| **事件发布** | 向 `vpp.resource.events` topic 发布 CU 创建/删除/生命周期变更事件，供 gateway 消费 |

本服务**不负责**：外部设备接入协议、连接状态写入（由 gateway 写入 Redis）、时序数据存储（属于 telemetry 服务）。

---

## 架构设计

服务采用**六边形架构 + CQRS**，对外暴露 **gRPC + HTTP 双协议**（HTTP 由 grpc-gateway 代理 gRPC 实现）。

```
管理端 / 内部服务
      │ HTTP :8082 (grpc-gateway)
      │ gRPC :5002
      ▼
┌─────────────────────────────────────────────────────┐
│                  Inbound Adapters                    │
│   adapter/inbound/grpc/       adapter/inbound/http/  │
│   · Site/Asset/CU/Point CRUD   · gin + grpc-gateway  │
│   · 批量导入 & 任务查询                               │
└───────────────────────┬─────────────────────────────┘
                        │ Command / Query
                        ▼
┌─────────────────────────────────────────────────────┐
│                 Application Layer (CQRS)             │
│   application/command/   application/query/          │
│   application/worker/    (ImportWorker)              │
└──────────┬───────────────────────┬──────────────────┘
           │ port.XxxRepository    │ port.XxxRuntimeCache
           ▼                       ▼
┌──────────────────────┐  ┌───────────────────────────┐
│  Outbound Adapters   │  │   Outbound Adapters        │
│  adapter/outbound/   │  │   adapter/outbound/        │
│    postgres/         │  │     redis/   kafka/        │
└──────────┬───────────┘  └──────────┬────────────────┘
           ▼                          ▼
┌──────────────────────┐  ┌──────────┐ ┌─────────────┐
│   infrastructure/    │  │  Redis   │ │    Kafka    │
│ persistent/postgres/ │  │  db=0    │ │  resource   │
│  (GORM + Builder)    │  │  runtime │ │   events    │
└──────────────────────┘  └──────────┘ └─────────────┘
           ▼
     PostgreSQL
```

### 分层依赖规则

```
domain ← application ← adapter ← infrastructure
```

- `domain`：实体、值对象、领域错误、port 接口定义，不依赖任何其他本模块包
- `application`：CQRS handlers + ImportWorker，只依赖 domain
- `adapter`：port 接口的具体实现，依赖 domain + infrastructure
- `infrastructure`：GORM 模型、原始数据库操作、查询构建器，只依赖外部库

### CQRS + Decorator

所有用例 Handler 通过 `decorator.ApplyCommandDecorators` / `ApplyQueryDecorators` 统一注入 tracing 与 metrics，业务代码零感知：

```
Command → ApplyCommandDecorators[tracing + metrics] → xxxHandler.Handle()
Query   → ApplyQueryDecorators[tracing + metrics]   → xxxHandler.Handle()
```

---

## 数据存储

### PostgreSQL（持久化）

| 表 | 职责 |
|---|---|
| `nodes` | 资源树公共字段（ID、父节点、路径、深度、生命周期状态、软删除） |
| `sites` | Site 扩展字段（运营状态、地理位置 JSONB） |
| `assets` | Asset 扩展字段（调度状态、额定容量、能源类型、市场参与标志） |
| `cus` | CU 扩展字段（协议、连接配置 JSONB、能力标签 JSONB） |
| `points` | 测点（数据类型、外部地址、安全阈值 JSONB、控制标志） |
| `import_jobs` | 批量导入任务（状态机、进度计数、结果 JSONB、重试控制） |

Node 是所有资源类型的公共行（统一树结构），其他表以 `node_id` 为主键关联扩展字段。

### Redis db=0（运行时热数据）

| Key 模式 | 数据类型 | 内容 |
|---|---|---|
| `asset:{tenantID}:{assetID}:runtime` | JSON String | 功率、SOC、可调度状态、在线状态 |
| `cu:{tenantID}:{cuID}:runtime` | JSON String | 连接状态、最后心跳时间、延迟 |
| `point:{tenantID}:{pointID}:runtime` | JSON String | 点位当前值、质量状态、采样时间 |

运行时数据由 gateway / telemetry 服务通过 `PatchXxxRuntime` 写入，resource 服务在查询时合并返回给调用方。

---

## API 接口

服务通过 gRPC（`:5002`）对外暴露接口，HTTP（`:8082`）由 grpc-gateway 自动代理为 REST。接口定义见 `api/resource/proto/`。

### Site 管理

| 接口 | 说明 |
|---|---|
| `CreateSite` | 创建站点，指定名称、地理位置、描述 |
| `GetSite` | 按 ID 获取站点详情 |
| `ListSites` | 列表查询，支持按名称模糊搜索、状态过滤、分页 |
| `UpdateSite` | 更新站点名称、位置、描述 |

### Asset 管理

| 接口 | 说明 |
|---|---|
| `CreateAsset` | 在指定 Site 下创建资产（支持子类型、额定容量、能源类型等） |
| `GetAsset` | 按 ID 获取资产详情（含 Redis 运行时状态） |
| `ListAssets` | 列表查询，支持按 SiteID、类型、名称、分页过滤 |
| `UpdateAsset` | 更新资产属性（名称、调度状态、额定容量、市场参与等） |

### CU 管理

| 接口 | 说明 |
|---|---|
| `CreateCU` | 在指定 Asset 下创建控制单元 |
| `GetCU` | 按 ID 获取 CU 详情（含 Redis 连接运行时状态） |
| `ListCUs` | 列表查询，支持按 AssetID、名称、分页过滤 |
| `UpdateCU` | 更新 CU 属性（名称、协议配置、能力标签等） |

### Point 管理

| 接口 | 说明 |
|---|---|
| `CreatePoint` | 在指定 Asset 下（关联 CU）创建测点 |
| `GetPoint` | 按 ID 获取测点详情（含 Redis 最新点位值） |
| `ListPoints` | 列表查询，支持按 SiteID、CUID、数据类型、是否虚点、分页过滤 |
| `UpdatePoint` | 更新测点属性（数据类型、外部地址、安全阈值等） |
| `DeletePoint` | 删除测点（软删除） |

### 资源树操作（通用）

| 接口 | 说明 |
|---|---|
| `DeleteResource` | 删除任意节点，可选 `include_descendants` 级联删除子树 |
| `MoveResource` | 将节点迁移至新的父节点下 |
| `BatchMoveResources` | 批量将多个节点迁移至同一新父节点 |
| `RenameResource` | 重命名任意节点 |
| `ChangeResourceLifecycle` | 变更资源生命周期状态（active / inactive / decommissioned），触发 Kafka 事件 |
| `GetResourceDetail` | 获取任意节点的通用详情（含树路径信息） |
| `ListChildren` | 获取指定节点的直接子节点列表（支持分页） |
| `GetBreadcrumb` | 获取从根节点到指定节点的完整祖先链（面包屑导航） |
| `ExportResourceTree` | 按最大深度导出以指定节点为根的子树（扁平列表） |

### 批量导入

| 接口 | 说明 |
|---|---|
| `SubmitBatchImport` | 提交批量导入任务（Asset / CU / Point 三种），返回 JobID；校验失败的条目同步返回，其余异步处理 |
| `GetJob` | 查询导入任务状态、进度（total / succeeded / failed）及结果详情 |
| `RetryJob` | 将失败任务重置为 pending，由 ImportWorker 重新认领执行 |

---

## 目录结构

```
internal/resource/
├── cmd/main.go                         # 可执行入口（thin main）
├── app.go                              # cobra + viper 配置加载
├── run.go                              # 初始化 tracing，调用 createServer
├── server.go                           # 全局 wiring + PrepareRun/Run + graceful shutdown
├── config/config.go                    # Options → Config（内部配置）
├── options/options.go                  # 外部输入 Options（viper 绑定）
├── domain/
│   ├── errors.go                       # 领域哨兵错误（ErrXxxNotFound 等）
│   ├── model/                          # 实体 & 值对象（Site / Asset / CU / Point / Job / Runtime）
│   └── port/                           # 仓储接口 + 运行时缓存接口 + 事件发布接口
├── application/
│   ├── command/                        # 写侧用例 Handler（CreateXxx / UpdateXxx / DeleteXxx 等）
│   ├── query/                          # 读侧用例 Handler（GetXxx / ListXxx / ExportXxx 等）
│   ├── batch/                          # 批量创建辅助逻辑
│   ├── worker/                         # ImportWorker（异步任务执行器注册表 + 主循环）
│   │   └── executors/                  # Asset / CU / Point 导入执行器
│   ├── types/                          # 跨层共享的 DTO（ImportSpec / Item / Payload / 错误）
│   └── app.go                          # Application 组装（Commands / Queries / Workers）
├── adapter/
│   ├── inbound/
│   │   ├── grpc/                       # gRPC 服务实现（各资源 handler + proto 转换）
│   │   └── http/                       # grpc-gateway 挂载（gin engine）
│   └── outbound/
│       ├── postgres/                   # port.XxxRepository 实现（依赖 infrastructure）
│       ├── redis/                      # port.XxxRuntimeCache 实现（JSON KV）
│       └── kafka/                      # port.ResourceEventPublisher 实现
├── infrastructure/
│   └── persistent/postgres/
│       ├── models.go                   # GORM 模型（XxxModel + TableName）
│       ├── db.go                       # Postgres 连接池封装
│       ├── *_repo.go                   # 原始 GORM 操作（无 domain 感知）
│       └── builder/                    # 多表 JOIN 流式查询构建器
└── docs/                               # 架构决策、编码规范、模块上下文等内部文档
```

---

## 依赖组件

| 组件 | 用途 | 默认地址 |
|---|---|---|
| PostgreSQL | 资源树持久化 | `127.0.0.1:5432`，库 `resource` |
| Redis db=0 | Asset / CU / Point 运行时热数据 | `127.0.0.1:6379` |
| Kafka | 发布 `vpp.resource.events`（lifecycle 事件） | 配置项 `kafka.brokers` |
| Consul | 服务注册与发现 | 配置项 `resource.consul-addr` |
| Jaeger / OTEL | 分布式 tracing | 配置项 `tracing.endpoint` |
| Prometheus | 指标暴露 `/metrics` | `:9091`（默认） |

> Kafka 未配置时（`kafka.brokers` 为空）服务正常启动，事件发布降级为 log-only，不影响 CRUD 功能。
> Consul 和 tracing 同样可选，缺失时服务正常运行。

---

## 启动方式

**推荐在模块根目录运行**（路径与 CI 一致）：

```bash
cd internal/resource
go run ./cmd/ -c /path/to/resource.yaml
```

**构建二进制：**

```bash
go build -o bin/vpp-resource ./cmd/
./bin/vpp-resource -c /path/to/resource.yaml
```

**配置文件搜索顺序**（未指定 `-c` 时）：`./config/resource.yaml` → `../../config/resource.yaml` → 当前目录 → 仅用默认值 + 环境变量。

### 最小配置示例

```yaml
resource:
  grpc-addr: ":5002"
  http-addr: ":8082"
  metrics-addr: ":9091"
  service-name: "vpp-resource"
  worker-poll-interval: "5s"

database:
  driver: postgres
  host: 127.0.0.1
  port: 5432
  user: postgres
  password: postgres
  dbname: resource
  params:
    sslmode: disable
    TimeZone: Asia/Shanghai

redis:
  addr: "127.0.0.1:6379"
  db: 0

# 可选；不配置则事件仅打印日志
kafka:
  brokers:
    - "127.0.0.1:9092"
  topic: "vpp.resource.events"

# 可选
tracing:
  endpoint: "http://127.0.0.1:4318"
  insecure: true
```

### 优雅停机

服务在收到 `SIGINT` / `SIGTERM` 后按顺序关闭（30s deadline）：

1. gRPC GracefulStop + HTTP Shutdown（停止接收新流量）
2. 等待 ImportWorker 当前任务完成
3. Flush Kafka 缓冲消息并关闭 writer
4. 关闭 Redis 连接池
5. 停止 Prometheus metrics server

---

## 关键设计约定

### CU UUID = gateway CUCode

resource 服务分配的 **CU UUID 必须作为 gateway `DeviceMapping.CUCode`** 使用。这是 lifecycle sync 的基础：CU 被删除或禁用时，gateway 的 `lifecycle_consumer` 通过 `ResourceID`（= CU UUID）查找并自动 disable 对应 mapping。

### ImportWorker：单 goroutine + DB 行级锁

ImportWorker 使用 `SELECT FOR UPDATE SKIP LOCKED` 从 `import_jobs` 表认领任务，保证多节点部署下同一任务只被一个 worker 执行，无需 Redis 分布式锁。Worker 主循环是单 goroutine，不在 `processNext` 内部开新 goroutine。

### 运行时状态分离

CU 的连接状态（`ConnStatus`、`LastSeenAt`）**不存在 Postgres**，只在 Redis `CURuntime` 中维护。写入方是 gateway，读取合并由 resource 查询接口完成。`cus.conn_status` 列保留但不写入（历史遗留，下次 migration 可删）。

### Kafka 事件为非对称设计

```
CU 删除 / 禁用  → 发布 Kafka 事件 → gateway 自动 disable mapping   ✅
CU 创建        → 不发布自动关联事件                                  ✅ 有意设计
```

创建 mapping 需要 `ExternalSystem` 和 `ExternalID`，这些信息在 CU 创建时未知，因此 Onboarding 流程由管理端**显式调用** `resource.CreateCU` + `gateway.CreateMapping` 两步完成，两个服务互不感知。
