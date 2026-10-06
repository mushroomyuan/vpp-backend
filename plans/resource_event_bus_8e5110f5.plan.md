---
name: Resource Event Bus
overview: 在 `internal/platform/event/` 下建立统一事件模块，在 resource 服务中通过 `EventPublisher` 端口抽象广播所有资源生命周期事件，为未来 Outbox 模式预留切换点。
todos:
  - id: platform-event-module
    content: 在 internal/platform/event/ 下新建 envelope.go 和 resource/topic.go + resource/events.go
    status: completed
  - id: resource-port
    content: 新增 resource/domain/port/event_publisher.go，定义 ResourceEventPublisher 接口
    status: completed
  - id: kafka-adapter
    content: 新增 resource/adapter/outbound/kafka_pub/event_publisher.go，实现 Kafka 发布（含 no-op 降级）
    status: completed
  - id: command-handlers
    content: 修改 12 个 command handler，注入 publisher 并在写库成功后广播事件
    status: completed
  - id: import-executors
    content: 修改 3 个 import executor，在 job 完成后发 TypeImportCompleted 事件
    status: completed
  - id: app-wiring
    content: 更新 application/app.go 的 Dependencies 和 NewApplication，传递 EventPublisher
    status: completed
  - id: server-wiring
    content: 更新 config/config.go 加 KafkaConfig，更新 server.go 初始化并关闭 publisher
    status: completed
isProject: false
---

# Resource 事件总线计划

## 目标

- Resource 成为资源 Source of Truth，所有增删改及导入完成均广播事件
- 统一事件 Envelope、Topic、EventType 常量，放入 `internal/platform` 共享层
- 通过 `EventPublisher` 接口（定义在 `resource/domain/port`）隔离 Kafka 细节，未来替换为 Outbox 只需换实现

---

## 整体结构变化

```
internal/
├── platform/
│   └── event/                          ← 新增，platform module 内新 package
│       ├── envelope.go                 ← 公共泛型 Envelope[T]
│       └── resource/
│           ├── events.go               ← 各类 payload struct（CUCreated 等）
│           └── topic.go                ← Topic 常量、EventType 常量、Version
└── resource/
    ├── domain/port/
    │   └── event_publisher.go          ← 新增：ResourceEventPublisher 接口
    ├── adapter/outbound/kafka_pub/
    │   └── event_publisher.go          ← 新增：Kafka 实现（含 no-op 降级）
    ├── application/
    │   ├── app.go                      ← 修改：Dependencies 加 EventPublisher
    │   ├── command/*.go                ← 修改：注入 publisher，写后调用
    │   └── worker/executors/*.go       ← 修改：import 完成后发事件
    ├── config/config.go                ← 修改：加 KafkaConfig
    └── server.go                       ← 修改：初始化 publisher，接入 shutdown
```

---

## Step 1 — `internal/platform/event/` 公共模块

**`envelope.go`** 定义泛型信封，所有服务共享：

```go
// package platform/event
type Envelope[T any] struct {
    EventID      string    // UUID v7
    EventType    string    // e.g. "resource.cu.created"
    Version      string    // "v1"
    TenantID     string
    OccurredAt   time.Time
    Payload      T
}
```

**`resource/topic.go`** 定义常量（无业务逻辑）：

```go
const (
    TopicResourceEvents = "vpp.resource.events"
    VersionV1           = "v1"

    TypeSiteCreated      = "resource.site.created"
    TypeSiteUpdated      = "resource.site.updated"
    TypeAssetCreated     = "resource.asset.created"
    TypeAssetUpdated     = "resource.asset.updated"
    TypeCUCreated        = "resource.cu.created"
    TypeCUUpdated        = "resource.cu.updated"
    TypeResourceDeleted  = "resource.deleted"        // 节点级通用删除
    TypePointCreated     = "resource.point.created"
    TypePointUpdated     = "resource.point.updated"
    TypePointDeleted     = "resource.point.deleted"
    TypeLifecycleChanged = "resource.lifecycle.changed"
    TypeImportCompleted  = "resource.import.completed"
)
```

**`resource/events.go`** 定义各事件的 payload struct（CUCreatedPayload、ResourceDeletedPayload、ImportCompletedPayload 等）。

---

## Step 2 — `resource/domain/port/event_publisher.go`

```go
// ResourceEventPublisher is the outbound port for resource lifecycle events.
// Application layer depends only on this interface; the Kafka producer (or a
// future Outbox implementation) satisfies it from the infrastructure side.
type ResourceEventPublisher interface {
    Publish(ctx context.Context, eventType string, tenantID string, payload any) error
    Close() error
}
```

放在 `domain/port` 而非 application 层，是为了明确这是 Hexagonal 的 **driven port（出站端口）**，与 Repository 接口并列。

---

## Step 3 — `resource/adapter/outbound/kafka_pub/event_publisher.go`

参照 [`internal/telemetry/adapter/outbound/kafka_pub/event_publisher.go`](internal/telemetry/adapter/outbound/kafka_pub/event_publisher.go) 的模式：

- `kafka.Writer`，Topic = `vpp.resource.events`，Async = true
- `Balancer: &kafka.Hash{}`，分区键 = `tenantID:resourceID`（保证同资源有序）
- Brokers 为空时 no-op 降级（同 telemetry）
- `Publish()` 将 payload marshal 为 `Envelope[json.RawMessage]` 后写入 Kafka
- 实现 `var _ port.ResourceEventPublisher = (*EventPublisher)(nil)` 编译期验证

---

## Step 4 — 修改 Command Handlers

每个命令处理器在构造函数中增加 `publisher port.ResourceEventPublisher` 参数，成功写库后调用 `Publish`，**错误仅 log 不返回**（与 telemetry SOE 一致，事件是 best-effort）。

需修改的文件（共 12 个）：

- `create_site.go` → 发 `TypeSiteCreated`
- `update_site.go` → 发 `TypeSiteUpdated`
- `create_asset.go` → 发 `TypeAssetCreated`
- `update_asset.go` → 发 `TypeAssetUpdated`
- `create_cu.go` → 发 `TypeCUCreated`（优先级最高，gateway 依赖）
- `update_cu.go` → 发 `TypeCUUpdated`
- `delete_resource.go` → 发 `TypeResourceDeleted`
- `create_point.go` → 发 `TypePointCreated`
- `update_point.go` → 发 `TypePointUpdated`
- `delete_point.go` → 发 `TypePointDeleted`
- `change_resource_lifecycle.go` → 发 `TypeLifecycleChanged`
- `rename_resource.go` → 发 `TypeResourceUpdated`（重命名视为 update）

**Import 完成事件**（共 3 个 executor）：

- `cu_import_executor.go`、`asset_import_executor.go`、`point_import_executor.go`

在 executor 的 `Execute()` 完成后发 `TypeImportCompleted`，payload 包含 `job_id`、`target` 类型、成功/失败计数。

---

## Step 5 — 修改 `application/app.go`

[`internal/resource/application/app.go`](internal/resource/application/app.go) 中 `Dependencies` 增加：

```go
type Dependencies struct {
    // ... 现有字段 ...
    EventPublisher port.ResourceEventPublisher  // 新增
}
```

`NewApplication` 将 `deps.EventPublisher` 传入所有需要的 Handler 构造函数。

---

## Step 6 — 修改 `config` 和 `server.go`

**`config/config.go`** 加 `KafkaConfig`（参照 telemetry config 的 `KafkaOptions`）：

```go
type KafkaConfig struct {
    Brokers []string `yaml:"brokers"`
    Topic   string   `yaml:"topic"`   // default: vpp.resource.events
}
```

**[`internal/resource/server.go`](internal/resource/server.go)** 的 `createServer()`：

1. 从 `appCfg` 读 KafkaConfig
2. `kafkapub.NewEventPublisher(cfg)` 构造 publisher
3. 传入 `application.Dependencies{..., EventPublisher: publisher}`
4. 在 `shutdown coordinator` 的步骤 2 之后增加 `publisher.Close()`

---

## 关于未来 Outbox 模式

由于 application 层只依赖 `port.ResourceEventPublisher` 接口，未来启用 Outbox 时只需：

1. 实现 `OutboxEventPublisher`（将消息写入同一事务的 `resource_outbox` 表）
2. 独立 relay goroutine 从表中读并发往 Kafka
3. 在 `server.go` 中将 `kafkapub.EventPublisher` 替换为 `outbox.EventPublisher`

**Application 层业务代码零修改。**
