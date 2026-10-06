---
name: Gateway CU Lifecycle Sync
overview: 在 gateway 服务中新增 Kafka Consumer，订阅 `vpp.resource.events`，当 CU 被删除或生命周期变更为禁用/归档时自动 Disable 对应的 DeviceMapping，替代现有的纯手动 `DisableMapping` 机制。
todos:
  - id: gateway-kafka-config
    content: 修改 options/options.go 加 KafkaOptions，修改 config/config.go 加 KafkaConfig
    status: pending
  - id: gateway-disable-by-cu
    content: 新增 application/command/disable_mapping_by_cu.go：DisableMappingByCUCode 命令
    status: pending
  - id: gateway-consumer
    content: 新增 adapter/inbound/kafka_sub/lifecycle_consumer.go：订阅 vpp.resource.events，处理 TypeResourceDeleted 和 TypeLifecycleChanged
    status: pending
  - id: gateway-server-wiring
    content: 修改 server.go：wire consumer 进 errgroup，shutdown 时 Close；go.mod 加 kafka-go
    status: pending
isProject: false
---

# Gateway CU 生命周期映射同步计划

## 前提约定（必须确认）

**CUCode 等于 Resource CU UUID 的约定**

Gateway `DeviceMapping.CUCode` 当前是操作员手填的字符串。要实现自动同步，必须确立系统约定：**在 gateway 创建 DeviceMapping 时，`CUCode` 字段使用 resource 服务分配的 CU `ID`（UUID v7）**。这样 resource 事件里的 `ResourceID` 就可以直接用于 `GetByCUCode` 查找。

---

## 事件处理逻辑

```mermaid
TypeResourceDeletedflowchart TD
    ResourceKafka["vpp.resource.events"]
    Consumer["lifecycle_consumer"]
    Switch{EventType}
    ResourceDeleted["TypeResourceDeleted"]
    LifecycleChanged["TypeLifecycleChanged"]
    GetByCUCode["GetByCUCode(tenantID, resourceID)"]
    NotFound{"ErrMappingNotFound?"}
    Disable["Disable(tenantID, mapping.ID)"]
    Skip["no-op, commit"]
    StatusCheck{"status in disabled/archived?"}

    ResourceKafka --> Consumer
    Consumer --> Switch
    Switch -->|resource.deleted| ResourceDeleted
    Switch -->|resource.lifecycle.changed| LifecycleChanged
    Switch -->|其他 EventType| Skip
    ResourceDeleted --> GetByCUCode
    LifecycleChanged --> StatusCheck
    StatusCheck -->|Yes| GetByCUCode
    StatusCheck -->|No| Skip
    GetByCUCode --> NotFound
    NotFound -->|Yes| Skip
    NotFound -->|No| Disable
```



**已知限制：** `delete_resource.go` 在 `IncludeDescendants=true` 时只发一条 `TypeResourceDeleted` 事件（根节点），子树中的 CU 不会触发各自的 disable。作为后续工作，可在 resource 层对子树内每个 CU 单独发事件。

---

## 整体结构变化

```
internal/gateway/
├── options/options.go              ← 修改：加 KafkaOptions
├── config/config.go                ← 修改：加 KafkaConfig
├── application/
│   ├── app.go                      ← 修改：Dependencies 加 MappingRepo 供 consumer 直接注入
│   └── command/
│       └── disable_mapping_by_cu.go  ← 新增：DisableMappingByCUCode 命令
├── adapter/inbound/kafka_sub/
│   └── lifecycle_consumer.go       ← 新增：Kafka consumer
└── server.go                       ← 修改：初始化 consumer，接入 errgroup + shutdown
```

---

## Step 1 — `options.go` + `config.go`

`**[internal/gateway/options/options.go](internal/gateway/options/options.go)**` 加：

```go
type KafkaOptions struct {
    Brokers  []string `mapstructure:"brokers"`
    Topic    string   `mapstructure:"topic"`   // default: vpp.resource.events
    GroupID  string   `mapstructure:"group-id"` // default: vpp-gateway-resource-events
}
```

默认值：`Topic = "vpp.resource.events"`, `GroupID = "vpp-gateway-resource-events"`，`Brokers` 为空时 consumer 不启动（skip 逻辑，类似 resource publisher 的 no-op 降级）。

`**[internal/gateway/config/config.go](internal/gateway/config/config.go)**` 加 `KafkaConfig` struct，同步映射 options 字段。

---

## Step 2 — 新增 `DisableMappingByCUCode` 命令

现有 `DisableMapping` 命令需要 mapping `ID`，而 Kafka consumer 只知道 `CUCode`（= ResourceID）。新建：

`**internal/gateway/application/command/disable_mapping_by_cu.go**`

```go
type DisableMappingByCUCode struct {
    TenantID string
    CUCode   string // equals resource CU UUID by convention
}

type DisableMappingByCUCodeHandler = decorator.CommandHandler[DisableMappingByCUCode, struct{}]
```

Handle 逻辑：

1. `repo.GetByCUCode(ctx, tenantID, cuCode)`
2. 若 `domain.ErrMappingNotFound` → return `nil`（资源不是 CU 或无对应 mapping，静默跳过）
3. 否则 `repo.Disable(ctx, tenantID, mapping.ID)`

---

## Step 3 — `adapter/inbound/kafka_sub/lifecycle_consumer.go`

新文件。使用 `segmentio/kafka-go` 的 `kafka.NewReader`（consumer group 模式）：

```go
type LifecycleConsumer struct {
    reader  *kafka.Reader
    handler command.DisableMappingByCUCodeHandler
}
```

- **GroupID** = `vpp-gateway-resource-events`（支持多实例水平扩展）
- **FetchMessage → dispatch → CommitMessages**（at-least-once）
- 反序列化为 `platEvent.Envelope[json.RawMessage]`，switch `EventType`：
  - `TypeResourceDeleted` → `DisableMappingByCUCode{TenantID, ResourceID}`
  - `TypeLifecycleChanged` → 反序列化 `LifecycleChangedPayload`；若 `Status` ∈ `{"disabled", "archived"}` → `DisableMappingByCUCode`
  - 其他：跳过，直接 commit
- 错误（Disable 失败）：log + **不 commit**，让 consumer group 重试（消息不丢）
- Brokers 为空时构造函数返回 nil，`server.go` 判空不启动

**需要为 gateway 增加 `platform/event/resource` 依赖**：在 `[internal/gateway/go.mod](internal/gateway/go.mod)` 的 `replace` 块中，`platform` 已经是本地替换，`event/resource` 包属于 `platform` module，无需额外 go.mod 条目。需要 `go get github.com/segmentio/kafka-go@v0.4.51`。

---

## Step 4 — 修改 `server.go`

`**[internal/gateway/server.go](internal/gateway/server.go)**`：

1. 构造 `LifecycleConsumer`（从 `cfg.Kafka`）
2. 在 `Run()` 的 `errgroup` 中新增 goroutine：

```go
   eg.Go(func() error {
       return s.lifecycleConsumer.Run(egCtx)
   })
   

```

1. shutdown coordinator 新增 `s.lifecycleConsumer.Close()`（在 gRPC stop 之后）

---

## Telemetry 不需要消费 Resource Events（分析）


| 场景                        | 分析                                                    | 结论  |
| ------------------------- | ----------------------------------------------------- | --- |
| CU 删除 → 清除 Redis Snapshot | telemetry 用 `CUCode` 索引，resource 事件携带 `CUID`，两者无法直接关联 | 不可行 |
| CU 删除 → TSDB 历史数据过滤       | 历史数据是审计依据，不应删除；`CUCode` 身份问题同上                        | 不必要 |
| Snapshot 过期感知             | 已有 `IsStale()` + `Stale: true` 响应字段处理                 | 已覆盖 |


**推荐的正确路径（后续版本）：**  
gateway 在调用 `DisableMappingByCUCode` 成功后，已知 `CUCode`，可以通过 gRPC 调用 telemetry 的 `InvalidateSnapshot(tenantID, cuCode)` 来清除 Redis snapshot。不需要 Kafka 绕圈。这需要在 telemetry 的 gRPC proto 中新增此接口，属于另一个独立任务。