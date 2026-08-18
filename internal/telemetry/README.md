# vpp-telemetry

VPP 平台的时序遥测微服务。负责接收控制单元（CU）上报的实时量测数据，将其持久化到时序数据库，并维护每台 CU 的实时快照状态。

---

## 目录

- [服务职责](#服务职责)
- [架构设计](#架构设计)
- [数据存储](#数据存储)
- [API 接口](#api-接口)
- [目录结构](#目录结构)
- [依赖组件](#依赖组件)
- [启动方式](#启动方式)
- [功能测试](#功能测试)
- [当前开发进度](#当前开发进度)

---

## 服务职责

| 职责 | 说明 |
|---|---|
| **写入路径** | 接收 CU 推送的量测数据，写入 TimescaleDB 超表，同时更新 Redis 实时快照，并发布离散量状态变化事件（SOE）到 Kafka |
| **原始查询** | 按租户 / CU / 指标 / 时间范围查询历史原始记录（最多 30 天窗口） |
| **实时快照** | 读取单台 CU 或整个租户所有 CU 的当前实时状态（来自 Redis，毫秒级延迟） |
| **聚合查询** | 对历史数据进行降采样聚合（AVG/MAX/MIN/SUM/COUNT/LAST），bucket 由调用方指定 |

---

## 架构设计

服务采用**六边形架构（Hexagonal Architecture）+ CQRS**，严格分层：

```
┌─────────────────────────────────────────────────────────────┐
│                        Inbound Adapter                       │
│                   adapter/inbound/grpc/                      │
│  gRPC Server  ←→  proto handler  ←→  proto↔domain converter │
└────────────────────────────┬────────────────────────────────┘
                             │ Command / Query
┌────────────────────────────▼────────────────────────────────┐
│                      Application Layer                       │
│                       application/                           │
│                                                              │
│  Commands                       Queries                      │
│  └─ IngestTelemetry             ├─ QueryTelemetry            │
│                                 ├─ GetSnapshot               │
│                                 ├─ GetFleetSnapshot          │
│                                 └─ QueryAggregation          │
└──────┬───────────────────┬──────────────────────────────────┘
       │ port interfaces   │
┌──────▼───────────────────▼──────────────────────────────────┐
│                       Domain Layer                           │
│                        domain/                               │
│  model: TelemetryRecord · Snapshot · SOEEvent · Metric      │
│  port:  TelemetryRepository · SnapshotRepository ·          │
│          AggregationRepository · EventPublisher             │
└──────┬───────────────────┬──────────────────────────────────┘
       │ implements port   │
┌──────▼───────────────────▼──────────────────────────────────┐
│                     Outbound Adapters                        │
│                  adapter/outbound/                           │
│  timescaledb/  →  TelemetryRepository + AggregationRepo     │
│  redis/        →  SnapshotRepository                        │
│  kafka/    →  EventPublisher  (当前为 stub)              │
│  influxdb/     →  TelemetryRepository + AggregationRepo     │
│                   (备用实现，未在 server.go 中激活)           │
└─────────────────────────────────────────────────────────────┘
```

### 写入流程（IngestTelemetry）

```
CU 推送
   │
   ▼
[1] 构建 TelemetryRecord 领域对象（验证非空、时间戳合法）
   │
   ▼
[2] SaveBatch → TimescaleDB  (pgx.Batch，单次网络往返)
   │
   ▼
[3] Find / Create Snapshot  ←  Redis GET
   │
   ▼
[4] snapshot.Apply(record)  →  检测离散量状态变化，生成 SOE 事件列表
   │
   ▼
[5] Save Snapshot  →  Redis SET
   │
   ▼
[6] PublishSOE (best-effort)  →  Kafka  (失败不回滚，下游最终一致)
```

---

## 数据存储

### TimescaleDB — 时序超表

```sql
-- 原始记录（narrow table：每行一个指标样本）
telemetry_records (
    ts          TIMESTAMPTZ  -- 时间戳，分区键
    tenant_id   TEXT
    cu_code     TEXT
    metric_name TEXT
    metric_type TEXT         -- ANALOG / DISCRETE
    value       DOUBLE PRECISION
    PRIMARY KEY (ts, tenant_id, cu_code, metric_name)
)
-- chunk_time_interval = 1 day

-- 15 分钟连续聚合视图（自动刷新，刷新间隔 5 分钟）
telemetry_15m  →  AVG / MAX / MIN / SUM / COUNT / LAST
                  GROUP BY (15min bucket, tenant_id, cu_code, metric_name)
```

> Schema 在服务首次启动时通过 `ApplySchema()` 自动创建，所有 DDL 语句幂等，重启安全。

### Redis — 实时快照

```
KEY:   tenant:{tenantID}:cu:{cuCode}:snapshot
VALUE: JSON { TenantID, CUCode, Metrics: map[name]→value, UpdatedAt }
TTL:   永不过期（0）— 快照应在服务重启后继续可用
```

每次成功 Ingest 后覆盖写入（Redis SET），读取为 O(1)。

### Kafka — SOE 事件（当前为 stub）

当 Discrete 型指标的值发生跳变时，产生一条 `SOEEvent`：

```go
type SOEEvent struct {
    TenantID   string
    CUCode     string
    MetricName string
    OldValue   float64
    NewValue   float64
    OccurredAt time.Time
}
```

当前 `kafka.EventPublisher` 为 no-op stub，Kafka 基础设施就绪后替换实现即可，接口不变。

---

## API 接口

服务通过 **gRPC**（`:5003`）对外暴露，proto 定义见 `api/telemetry/proto/`。

| RPC | 方向 | 说明 |
|---|---|---|
| `IngestTelemetry` | Write | 写入一台 CU 一个采样周期的所有指标 |
| `QueryTelemetry` | Read | 查询原始历史时序记录（≤ 30 天） |
| `GetSnapshot` | Read | 查询单台 CU 当前实时状态 |
| `GetFleetSnapshot` | Read | 查询租户下所有 CU 的实时状态 |
| `QueryAggregation` | Read | 查询降采样聚合数据（步长由调用方指定） |

proto 同时声明了 HTTP 路由（grpc-gateway 注解），为后续接入 HTTP 网关预留：

| HTTP 路由 | 对应 RPC |
|---|---|
| `POST /api/tenants/{TenantID}/cus/{CUCode}/telemetry:ingest` | IngestTelemetry |
| `GET  /api/tenants/{TenantID}/cus/{CUCode}/telemetry` | QueryTelemetry |
| `GET  /api/tenants/{TenantID}/cus/{CUCode}/snapshot` | GetSnapshot |
| `GET  /api/tenants/{TenantID}/snapshots` | GetFleetSnapshot |
| `GET  /api/tenants/{TenantID}/cus/{CUCode}/aggregation` | QueryAggregation |

---

## 目录结构

```
internal/telemetry/
├── cmd/
│   └── main.go                     # 进程入口
├── app.go                          # cobra 命令 + viper 配置加载
├── run.go                          # tracing + 启动
├── server.go                       # 组装根：接线所有层，生命周期管理
├── config/
│   └── config.go                   # 应用级配置（地址、服务名）
├── options/
│   └── options.go                  # viper/mapstructure 配置映射
├── application/
│   ├── app.go                      # Application 结构体（Commands + Queries）
│   ├── command/
│   │   └── ingest_telemetry.go     # 写入用例
│   └── query/
│       ├── query_telemetry.go
│       ├── get_snapshot.go
│       ├── get_fleet_snapshot.go
│       ├── query_aggregation.go
│       └── views.go                # Read Model DTO
├── domain/
│   ├── errors.go
│   ├── model/                      # 领域模型
│   │   ├── telemetry_record.go
│   │   ├── snapshot.go
│   │   ├── metric.go
│   │   ├── aggregation.go
│   │   ├── soe_event.go
│   │   └── query_condition.go
│   └── port/                       # 出站端口接口
│       ├── telemetry_repository.go
│       ├── snapshot_repository.go
│       ├── aggregation_repository.go
│       └── event_publisher.go
└── adapter/
    ├── inbound/grpc/               # gRPC 入站适配器
    │   ├── server.go
    │   ├── handlers.go
    │   └── converter.go
    └── outbound/
        ├── timescaledb/            # 时序存储（激活）
        ├── redis/                  # 快照存储（激活）
        ├── kafka/              # SOE 发布（stub）
        └── influxdb/               # 时序存储备用实现（未激活）
```

---

## 依赖组件

| 组件 | 版本 | 用途 | 端口 |
|---|---|---|---|
| TimescaleDB | latest-pg16 | 时序原始记录 + 聚合视图 | 5432 |
| Redis | 7-alpine | CU 实时快照 | 6379 |
| Kafka | — | SOE 事件发布（尚未部署） | — |
| Jaeger | latest | 分布式链路追踪 | 16686 |
| Prometheus | latest | 指标采集 | 9090 |

---

## 启动方式

### 1. 启动基础设施

```bash
# 在项目根目录执行
make infra-up
```

> **首次使用 TimescaleDB**：如果之前运行过 `postgres:16-alpine` 并保留了数据卷，需要先清除：
> ```bash
> docker compose down -v   # 删除旧数据卷
> make infra-up
> ```

### 2. 配置文件

配置文件位于 `config/telemetry.yaml`，关键字段：

```yaml
telemetry:
  grpc-addr: 127.0.0.1:5003      # gRPC 监听地址
  metrics-addr: 127.0.0.1:9103   # Prometheus /metrics 地址
  consul-addr: ""

timescaledb:
  host: 127.0.0.1
  port: 5432
  dbname: telemetry
  user: postgres
  password: postgres123

redis:
  addr: 127.0.0.1:6379
  db: 1   # db=1，与 resource 服务（db=0）隔离
```

### 3. 启动服务

```bash
# 方式一：Makefile（推荐）
make run-telemetry

# 方式二：直接运行
cd internal/telemetry
go run ./cmd/main.go -c ../../config/telemetry.yaml
```

服务启动成功的日志特征：

```
timescaledb pool connected to 127.0.0.1:5432/telemetry
timescaledb schema verified          ← 超表和聚合视图创建/确认完成
redis client connected to 127.0.0.1:6379 (db=1)
gRPC server listening on 127.0.0.1:5003
```

---

## 功能测试

推荐使用 [`grpcurl`](https://github.com/fullstorydev/grpcurl) 进行接口测试：

```bash
# 安装
go install github.com/fullstorydev/grpcurl/cmd/grpcurl@latest
```

### 查看服务方法列表

```bash
grpcurl -plaintext 127.0.0.1:5003 list telemetrypb.TelemetryService
```

---

### 1. IngestTelemetry — 写入遥测数据

```bash
grpcurl -plaintext -d '{
  "TenantID":  "tenant-001",
  "CUCode":    "cu-001",
  "Timestamp": "2026-06-26T09:00:00Z",
  "Metrics": [
    {"Name": "power_kw",   "Value": 125.5, "Type": "ANALOG",   "Quality": "QUALITY_GOOD"},
    {"Name": "voltage_v",  "Value": 220.1, "Type": "ANALOG",   "Quality": "QUALITY_GOOD"},
    {"Name": "switch_pos", "Value": 1,     "Type": "DISCRETE", "Quality": "QUALITY_GOOD"}
  ]
}' 127.0.0.1:5003 telemetrypb.TelemetryService/IngestTelemetry
```

预期响应：

```json
{ "SOECount": 0 }
```

> 再写一次并把 `switch_pos` 改为 `0`，`SOECount` 应变为 `1`（触发一次 SOE）。

---

### 2. QueryTelemetry — 查询原始历史记录

```bash
grpcurl -plaintext -d '{
  "TenantID":   "tenant-001",
  "CUCode":     "cu-001",
  "MetricName": "power_kw",
  "StartTime":  "2026-06-26T00:00:00Z",
  "EndTime":    "2026-06-26T23:59:59Z"
}' 127.0.0.1:5003 telemetrypb.TelemetryService/QueryTelemetry
```

> `MetricName` 可省略，省略时返回该 CU 在时间窗口内的所有指标记录。

---

### 3. GetSnapshot — 查询单台 CU 实时快照

```bash
grpcurl -plaintext -d '{
  "TenantID": "tenant-001",
  "CUCode":   "cu-001"
}' 127.0.0.1:5003 telemetrypb.TelemetryService/GetSnapshot
```

预期响应（示例）：

```json
{
  "TenantID": "tenant-001",
  "CUCode":   "cu-001",
  "Metrics": {
    "power_kw":   125.5,
    "voltage_v":  220.1,
    "switch_pos": 1.0
  },
  "UpdatedAt": "2026-06-26T09:00:00Z",
  "Stale": false
}
```

---

### 4. GetFleetSnapshot — 查询租户全量快照

```bash
grpcurl -plaintext -d '{
  "TenantID": "tenant-001"
}' 127.0.0.1:5003 telemetrypb.TelemetryService/GetFleetSnapshot
```

---

### 5. QueryAggregation — 查询降采样聚合数据

```bash
grpcurl -plaintext -d '{
  "TenantID":    "tenant-001",
  "CUCode":      "cu-001",
  "MetricName":  "power_kw",
  "StartTime":   "2026-06-26T00:00:00Z",
  "EndTime":     "2026-06-26T23:59:59Z",
  "StepSeconds": 900,
  "Functions":   ["AGG_AVG", "AGG_MAX", "AGG_MIN"]
}' 127.0.0.1:5003 telemetrypb.TelemetryService/QueryAggregation
```

> **注意**：聚合视图（`telemetry_15m`）由 TimescaleDB 后台每 5 分钟刷新一次。刚写入的数据不会立即出现在聚合结果里；若需立即验证写入，先用 `QueryTelemetry` 确认原始记录存在。

---

### Prometheus 指标

服务在 `:9103/metrics` 暴露 Prometheus 指标：

```bash
curl http://127.0.0.1:9103/metrics | grep app_
```

---

## 当前开发进度

### ✅ 已完成

- [x] 领域模型：`TelemetryRecord`、`Snapshot`、`SOEEvent`、`Metric`、`AggregatedPoint`
- [x] 出站端口接口定义（`TelemetryRepository` / `SnapshotRepository` / `AggregationRepository` / `EventPublisher`）
- [x] 应用层 CQRS：`IngestTelemetry` 命令 + 4 个查询 Handler，含 Metrics / Tracing 装饰器
- [x] gRPC 入站适配器（`Server` + `handlers.go` + `converter.go`）
- [x] TimescaleDB 适配器（原始写入 + 原始查询 + 聚合查询，含超表 Schema 自动迁移）
- [x] InfluxDB 备用适配器（完整实现，可通过修改 `server.go` 切换）
- [x] Redis 快照适配器（JSON 序列化，SCAN 支持全量查询）
- [x] Kafka SOE 发布适配器（stub，接口完整）
- [x] 服务组装根（`server.go` + `run.go` + `app.go`）
- [x] Proto 定义与代码生成（含 grpc-gateway HTTP 注解）
- [x] 配置系统（`options.go` + `config.go` + `telemetry.yaml`）
- [x] Docker Compose 集成（TimescaleDB 统一实例，init 脚本自动建库）
- [x] Makefile 快捷命令（`make run-telemetry` / `make infra-up`）

### 🚧 进行中 / 待完成

- [ ] **功能测试**：使用 grpcurl 逐一验证 5 个 RPC 接口
- [ ] **Kafka 生产实现**：用 `kafka-go` 或 `sarama` 替换 `kafka` stub
- [ ] **HTTP 网关**：实现 `adapter/inbound/http/gateway.go`（挂载 grpc-gateway）
- [ ] **数据保留策略**：配置 TimescaleDB `add_retention_policy`（如保留 90 天）
- [ ] **单元测试**：领域模型 + Application Handler 的 mock 测试
- [ ] **集成测试**：基于 testcontainers-go 的端到端接口测试
- [ ] **QualityBad 告警**：应用层策略：连续收到 N 次 bad-quality 时触发告警事件
- [ ] **聚合步长灵活化**：当前底层依赖 `telemetry_15m` 固定视图；可扩展为动态时间桶查询

### 📌 已知限制

| 限制 | 说明 |
|---|---|
| 聚合刷新延迟 | 连续聚合视图最多有 5 分钟延迟，不适合实时告警场景（用 GetSnapshot 代替） |
| 无批量写入 API | 当前 `IngestTelemetry` 每次处理一台 CU；批量多 CU 场景由调用方循环调用 |
| Kafka 未集成 | SOE 事件目前丢失，下游消费者暂时收不到状态变化通知 |
| 无认证 | gRPC 暂未配置 mTLS 或 token 鉴权 |
