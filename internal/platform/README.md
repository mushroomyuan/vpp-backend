# platform

> 无业务的共享基础库。各微服务（resource / gateway / telemetry / dispatch / simulator）依赖本模块，统一横切能力与基础设施接入，避免各服务重复造轮子。

独立 Go module：`github.com/mushroomyuan/vpp-backend/platform`。

## 定位

| 是 | 不是 |
|----|------|
| 日志、追踪、指标、装饰器 | 领域模型 / 业务用例 |
| gRPC / HTTP 服务端脚手架 | 具体 API 路由与 Handler |
| Postgres / Redis 客户端工厂 | 各服务自己的仓储实现 |
| Kafka 事件契约（topic / 类型 / Envelope） | 事件生产与消费逻辑本身 |
| UUID、错误码、工具函数 | 资源树 / 调度 / 遥测语义 |

业务服务只应在本包上「接线」；业务规则仍留在各自 `internal/<service>`。

## 能力一览

### 1. CQRS 装饰链 — `decorator`

给 Command / Query Handler 统一套上横切能力，业务 Handler 只写领域逻辑：

```
Logging → Metrics → Tracing → 业务 Handler
```

入口：`ApplyCommandDecorators` / `ApplyQueryDecorators`。细节见 [`decorator/README.md`](./decorator/README.md)。

### 2. 可观测性

| 包 | 能力 |
|----|------|
| **logging** | logrus 初始化；带 context 的结构化日志；`WhenRequest` / `WhenDB` / `WhenEventPublish` 等耗时辅助；gRPC unary interceptor |
| **telemetry** | OTLP 追踪初始化（Jaeger / Tempo 等）；span 创建；Kafka 生产/消费 span 与上下文传播（W3C / B3） |
| **metrics** | Prometheus `/metrics` HTTP 服务；业务 Counter / Histogram；可选 Go runtime 指标 |

### 3. 进程入口脚手架 — `server` / `middleware`

| 能力 | 说明 |
|------|------|
| `NewGRPCServer` | 预置 otelgrpc、tags、结构化日志 interceptor |
| `DialGRPC` | 出站客户端：insecure + otel 传播 |
| `NewGinEngine` | Gin：结构化日志、Recovery、请求日志、otelgin |
| **identity** | 与传输协议、IdP 无关的 `Principal` 身份契约及 context 传播 |
| **authn/casdoor** | Casdoor userinfo wire claims → `Principal` 防腐层 |
| **middleware/grpcauth** | gRPC 用户身份、租户绑定和授权 PEP |
| **middleware** | HTTP 请求日志中间件 |
| **authz** | `PermissionChecker` + 本地 Casbin PDP + Casdoor 策略同步 / 目录 upsert + Prometheus 指标（AUTHZ C6–C9）；控制类 `DenyWritesWhenStale`（C10a） |

各服务在此基础上注册自己的 proto / 路由，并自行管理优雅停机。

### 4. 数据访问工厂 — `postgres` / `redis`

- **postgres**：共享 `Config`（含 DSN 逃生舱）与连接池封装，供 GORM 仓储使用  
- **redis**：`New` + Ping 校验；各服务用不同 db（如 resource=0、telemetry=1）  

不包含表结构或业务 SQL。

### 5. 服务发现 — `discovery`

`Registry` 接口（注册 / 注销 / 发现 / 健康检查）；Consul 实现。运行时默认不连：`consul-addr` 为空则跳过注册。集群内发现走 K8s Service DNS。

### 6. 领域事件契约 — `event`

跨服务共享的 Kafka **信封与常量**（生产/消费双方只引用此处，禁止硬编码字符串）：

| 子包 | Topic | 典型事件 |
|------|-------|----------|
| `event/resource` | `vpp.resource.events` | CU/资源生命周期、导入完成等 |
| `event/gateway` | `vpp.command.events` | `command.completed` |
| `event/dispatch` | `vpp.dispatch.events` | task started / completed / failed |
| `event/telemetry` | `vpp.soe.events` | 离散量变位（flat `SOEPayload`，无 Envelope） |

通用包装：`event.Envelope[T]`（`event_id` / `event_type` / `version` / `tenant_id` / `occurred_at` / `payload`）。**例外：** SOE 在 v1 仍是 telemetry 生产者发出的扁平 JSON，消费者直接解 `SOEPayload`，不要自行套 Envelope。

### 7. 标识与杂项

| 包 | 能力 |
|----|------|
| **idgen** | UUIDv7（`Must()` / `NewUUIDv7`）；资源 ID、CommandID、EventID 等统一用此生成 |
| **consts** / **handler/errors** | 错误码与 HTTP/业务错误输出约定 |
| **response**（`pkg`） | Gin 统一响应体（errno / message / data / trace_id） |
| **util** | JSON、断言等小工具 |
| **handler/factory** | 单例工厂辅助 |

## 目录

```
platform/
├── decorator/     # CQRS Handler 装饰链
├── logging/       # 结构化日志
├── telemetry/     # OpenTelemetry 追踪与 messaging span
├── metrics/       # Prometheus
├── server/        # gRPC / Gin 脚手架 + DialGRPC
├── middleware/    # HTTP 中间件
├── postgres/      # DB 连接工厂
├── redis/         # Redis 客户端
├── discovery/     # 服务发现（Consul）
├── event/         # Kafka 事件契约
├── idgen/         # UUIDv7
├── consts/        # 错误码等常量
├── handler/       # errors / factory
└── util/          # 通用工具
```

## 使用约定

1. **新服务**：优先接 `server` + `decorator` + `logging` / `telemetry` / `metrics`，可观测性默认一致。  
2. **跨服务事件**：只改 `event/*` 常量与 payload，再改生产/消费方。  
3. **不要**在 platform 里写某服务的领域逻辑或表访问；那属于 `internal/<service>`。  
4. 基础设施（Kafka / tracing endpoint）未配置时，各服务应能降级启动——具体降级策略由各服务 wiring 决定，platform 提供可选实现。
