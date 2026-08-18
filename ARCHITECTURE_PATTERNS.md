# 跨服务代码架构

> 本文描述各微服务**共用的设计与编码模式**。  
> 服务拓扑与调用关系见 [`architecture.md`](./architecture.md)；单服务能力见各模块 `OVERVIEW.md` / `README.md`。

本仓库业务服务（resource / gateway / telemetry / dispatch，以及 simulator）在结构上刻意对齐：同一套分层、同一套接线方式、同一套可观测与契约习惯。共享能力沉淀在 [`internal/platform`](./internal/platform/README.md)。

---

## 1. 总体形态

| 维度 | 做法 |
|------|------|
| 模块 | 每服务独立 Go module；`platform` 与 `api/*/proto` 被依赖 |
| 进程 | 一服务一进程；`cmd` → `app`（配置）→ `run` / `server`（组装与生命周期） |
| 分层 | `domain` ← `application` ← `adapter` ← `infrastructure` |
| 对外协议 | 以 **gRPC + Protobuf** 为主；部分服务另挂 HTTP（Gin 或 grpc-gateway） |
| 协作 | 同步 RPC + Kafka 事件；事件类型与 topic 定义在 `platform/event` |

依赖方向只允许向内：领域不依赖框架与存储；最外层 adapter / infrastructure 实现接口并向内注入。

```
┌─────────────────────────────────────────┐
│  adapter/inbound   (gRPC · HTTP · Kafka) │
│  adapter/outbound  (DB · Redis · RPC · MQ)│
└──────────────────┬──────────────────────┘
                   │ 调用
┌──────────────────▼──────────────────────┐
│  application  (Command / Query / Worker) │
└──────────────────┬──────────────────────┘
                   │ 依赖 port
┌──────────────────▼──────────────────────┐
│  domain  (model · port · errors · service)│
└─────────────────────────────────────────┘
         ▲
         │ 实现 port
┌────────┴────────────────────────────────┐
│  infrastructure（如 GORM Model、原始 SQL） │
└─────────────────────────────────────────┘
```

---

## 2. 六边形架构（Ports & Adapters）

核心思想：**应用内核稳定，边缘可替换**。

- **Port**：在 `domain/port`（或 application 侧集成 port）定义接口——仓储、外部客户端、事件发布等  
- **Adapter**：在 `adapter/` 实现这些接口  
  - **Inbound**：把外部请求变成用例调用（gRPC Handler、HTTP、Kafka Consumer）  
  - **Outbound**：把用例意图变成技术细节（Postgres、Redis、下游 gRPC、Kafka Producer）  

换存储、换消息中间件、换外部 EMS，优先加/换 Adapter，而不是改 Application / Domain。

实现文件通常带编译期断言，防止接口漂移：

```go
var _ port.MappingRepository = (*MappingRepositoryPostgres)(nil)
```

---

## 3. CQRS：读写分离

Application 层按意图拆开：

| 侧 | 包 | 职责 |
|----|-----|------|
| **Command** | `application/command` | 写操作：校验、改状态、调出站、发事件 |
| **Query** | `application/query` | 读操作：组装视图，尽量不产生副作用 |
| **Worker**（部分服务） | `application/worker` | 异步后台任务（如 Resource 导入） |

Handler 形状统一为 `Handle(ctx, cmd/query) (result, error)`，经 `platform/decorator` 包装后挂到 `Application` 结构体上，由 inbound adapter 调用。

好处：写路径可严、可读模型可瘦；查询不必复用写侧聚合加载方式。

---

## 4. 领域驱动与充血模型

- **限界上下文 ≈ 微服务**：Resource 管资产树，Gateway 管映射与出入站，Telemetry 管时序，Dispatch 管任务编排  
- **领域模型**：实体带行为，不只是数据袋  
  - 用 `NewXxx(...)` 构造并校验，避免外部直接拼出非法对象  
  - 状态迁移放在模型方法里（如 `Job.Start/Complete/Fail`、`Command.MarkSending`、`Snapshot.Apply`）  
  - Application **编排**用例，不直接改 `Status` 等字段绕过不变式  
- **领域服务**：无自然归属单一实体的规则（如 Dispatch 的 `Dispatcher`）放在 `domain/service`，保持无 I/O  
- **哨兵错误**：`domain/errors.go` 定义 `ErrXxxNotFound` 等；基础设施把驱动错误译成哨兵；上层用 `errors.Is`

这就是常说的**充血模型**：数据与规则同居一处，应用层变薄、可测性更好。

---

## 5. 依赖注入与组合根

不引入大型 DI 框架，采用**显式构造注入**：

1. `server.go` / `createServer` 作为 **Composition Root**：创建 DB、Redis、Kafka、下游客户端等具体 Adapter  
2. 打成 `application.Dependencies`（接口类型）  
3. `NewApplication(deps)` 组装全部 Handler；对必需依赖 `nil` 则 `panic`（启动期失败，避免运行期空指针）  

业务代码依赖 **接口**，测试可替换假实现；接线集中在一处，便于审阅。

---

## 6. 横切能力与装饰器

用例级横切不靠每个 Handler 手写一遍，而靠 `platform/decorator`：

```
请求 → Logging → Metrics → Tracing → 业务 Handler → …
```

业务侧一般只写：

```go
decorator.ApplyCommandDecorators(handler, metricsClient)
decorator.ApplyQueryDecorators(handler, metricsClient)
```

进程级横切则在 `platform/server`：gRPC interceptor、Gin 中间件、OTLP 初始化、Prometheus `/metrics`。

原则：**业务逻辑与可观测 / 计量分离**；新增一层横切优先加 Middleware，而不是复制粘贴到各个用例。

---

## 7. gRPC、Protobuf 与 HTTP

| 项 | 约定 |
|----|------|
| 契约 | `api/<service>/proto/*.proto`，生成代码在 `proto/gen` |
| 服务间调用 | gRPC（如 Dispatch → Gateway、Gateway → Telemetry） |
| 对前端 / 运维 | Resource 等通过 **grpc-gateway** 暴露 HTTP；Gateway 对 EMS 另有 Gin REST |
| 入站转换 | `adapter/inbound/grpc`（或 http）负责 proto/DTO ↔ 应用命令，**不把 proto 渗入 domain** |

Protobuf 是边界语言；领域模型保持 Go 原生类型，避免生成代码绑架内核。

---

## 8. 异步边界：Kafka 事件

跨服务的「完成后通知 / 生命周期」多用事件，契约集中在 `platform/event`：

| Topic | 用途 |
|-------|------|
| `vpp.resource.events` | 资源生命周期等 → Gateway 清理 mapping |
| `vpp.command.events` | 命令终态 → Dispatch 推进状态机 |
| `vpp.dispatch.events` | 任务起止 → 告警/监控可选消费 |
| `vpp.soe.events` | 离散量变位（Telemetry） |

统一信封 `event.Envelope[T]`；生产/消费双方只引用常量，禁止硬编码 topic / event_type 字符串。

同步 RPC 解决「当下要不要受理」；事件解决「之后状态如何推进」——与 Gateway↔Dispatch 的 Accepted + completed 模式一致。

---

## 9. 其它横切约定

| 主题 | 做法 |
|------|------|
| **ID** | `platform/idgen` 统一 UUIDv7（资源、命令、事件） |
| **多租户** | 请求与表模型普遍带 `TenantID`；映射、遥测、任务均按租户隔离 |
| **配置** | 每服务 `options`（Viper）→ `config`；`config/*.yaml` + 环境变量 |
| **基础设施** | compose 提供 Postgres/Timescale、Redis、Kafka、Jaeger、Prometheus；未配置的可选组件应能降级启动 |
| **库表** | 一服务一库（或 telemetry 用 Timescale）；迁移在 `migrations/` |
| **错误上浮** | 基础设施错误包装 `%w`；领域哨兵稳定；adapter 映射为 gRPC status / HTTP 码 |

---

## 10. 一张对照：概念落在哪

| 概念 | 落点 |
|------|------|
| 六边形 / 端口适配器 | `domain/port` + `adapter/inbound|outbound` |
| CQRS | `application/command` · `application/query` |
| 充血 / DDD 实体 | `domain/model` 构造函数与状态方法 |
| 领域服务 | `domain/service`（无 I/O） |
| 依赖注入 | `Dependencies` + `NewApplication` + `server` 组装 |
| 装饰器 / 横切 | `platform/decorator`、`platform/server`、`platform/telemetry|metrics|logging` |
| 防腐层（对外） | 尤其 Gateway：外部模型 ↔ 内部 `CUCode` |
| 契约 | `api/*/proto`、`platform/event` |

---

## 11. 阅读路径建议

1. 本文 — 统一术语与分层习惯  
2. [`architecture.md`](./architecture.md) — 服务怎么连  
3. [`internal/platform/README.md`](./internal/platform/README.md) — 共享库能力  
4. 各服务 `OVERVIEW.md` — 该上下文解决什么问题  
5. 需要改代码时再下钻对应服务的 `domain` / `application` / `adapter`
