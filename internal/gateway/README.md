# vpp-gateway

VPP 平台的协议集成网关。负责外部 EMS/IoT 系统与内部微服务之间的 **ID 映射、协议转换与双向转发**：

- **入站 HTTP**：外部系统上报遥测、运维管理设备映射
- **入站 gRPC**：内部 dispatch 服务下发控制指令
- **出站 gRPC**：将标准化遥测转发至 `vpp-telemetry`
- **出站外部系统**：将控制指令翻译为外部设备 ID 后下发（`ExternalSystem=simulator` → Simulator HTTP；其它 → `ems_log`）

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
| **设备映射** | 维护 `(TenantID, ExternalSystem, ExternalID) → CUCode` 的映射关系，存于本地 Postgres |
| **遥测接入** | 接收 EMS 外部格式遥测，查映射后转换为标准格式，调用 telemetry 的 `IngestTelemetry` |
| **指令下发** | 接收 dispatch 的内部 `(TenantID, CUCode)` 控制请求，反查映射后转发至 EMS 适配器 |
| **映射治理** | 支持创建 / 查询 / 删除 / 禁用映射；禁用后拒绝遥测与指令（v1 stale mapping 兜底） |

---

## 架构设计

服务采用**六边形架构 + CQRS**，对外暴露 **HTTP + gRPC 双协议**：

```
外部 EMS / IoT                         dispatch（内部）
      │ HTTP POST telemetry                  │ gRPC ExecuteCommand
      ▼                                      ▼
┌─────────────────────────────────────────────────────────────┐
│                     Inbound Adapters                         │
│   adapter/inbound/http/          adapter/inbound/grpc/       │
│   · 遥测接入 · 映射 CRUD          · ExecuteCommand           │
└────────────────────────────┬────────────────────────────────┘
                             │ Command / Query
┌────────────────────────────▼────────────────────────────────┐
│                      Application Layer                       │
│  Commands                          Queries                   │
│  · ReceiveTelemetry                · ListMappings            │
│  · ExecuteCommand                                            │
│  · CreateMapping / DeleteMapping / DisableMapping            │
└──────┬──────────────────────────────┬───────────────────────┘
       │ port interfaces              │
┌──────▼──────────────────────────────▼───────────────────────┐
│                       Domain Layer                           │
│  model: DeviceMapping · ExternalTelemetry · StandardTelemetry│
│  port:  MappingRepository · TelemetryClient · EMSClient      │
└──────┬──────────────────────────────┬───────────────────────┘
       │ implements                   │
┌──────▼──────────────┐    ┌──────────▼───────────────────────┐
│ adapter/outbound/   │    │ adapter/outbound/                 │
│ postgres/           │    │ telemetry_grpc/  → vpp-telemetry  │
│ MappingRepository   │    │ simulator/       → vpp-simulator HTTP │
│                     │    │ ems_log/         → log-only default  │
└─────────────────────┘    └──────────────────────────────────┘
```

### 遥测上报流程（ReceiveTelemetry）

```
EMS HTTP POST
  { external_system, external_id, metrics[] }
       │
       ▼
[1] 查 device_mappings（tenant + external_system + external_id）
       │  mapping.status == active，否则 404 / 409
       ▼
[2] 转换为 StandardTelemetry（TenantID + CUCode + Metrics）
       │
       ▼
[3] gRPC IngestTelemetry → vpp-telemetry (:5003)
```

### 控制指令流程（ExecuteCommand）

```
dispatch gRPC ExecuteCommand
  { TenantID, CUCode, Command, Value }
       │
       ▼
[1] 反查 device_mappings（tenant + cu_code）
       │
       ▼
[2] EMSClient.SendCommand(external_system, external_id, ...)
       │  ExternalSystem=simulator → adapter/outbound/simulator
       │  其它 → ems_log（仅打日志）
       ▼
[3] 返回 { ExternalID, ExternalSystem }
```

---

## 数据存储

### Postgres — 设备映射表

Schema 定义见 `migrations/gateway/000001_init.up.sql`，由 compose init 脚本自动建库建表。

```sql
device_mappings (
    id              VARCHAR(36)  PRIMARY KEY,
    tenant_id       VARCHAR(64)  NOT NULL,
    external_system VARCHAR(64)  NOT NULL,   -- 如 "ems-sg"
    external_id     VARCHAR(128) NOT NULL,   -- EMS 侧设备 ID，如 "SG001"
    cu_code         VARCHAR(128) NOT NULL,   -- 内部 CU 标识，telemetry 寻址键
    status          VARCHAR(16)  NOT NULL,   -- active / disabled
    created_at      TIMESTAMPTZ,
    updated_at      TIMESTAMPTZ,
    UNIQUE (tenant_id, external_system, external_id)
)
```

> v1 **不调用 resource 服务**：CU 存在性不在 gateway 侧校验；映射由运维/API 手动维护。

---

## API 接口

### HTTP（外部 EMS 接入）— `:8083`

| 方法 | 路由 | 说明 |
|---|---|---|
| `POST` | `/api/v1/tenants/:tenant_id/telemetry:ingest` | EMS 上报遥测（外部 ID 格式） |
| `POST` | `/api/v1/tenants/:tenant_id/mappings` | 创建设备映射 |
| `GET` | `/api/v1/tenants/:tenant_id/mappings` | 查询映射列表 |
| `DELETE` | `/api/v1/tenants/:tenant_id/mappings/:id` | 删除映射 |
| `PATCH` | `/api/v1/tenants/:tenant_id/mappings/:id/disable` | 禁用映射 |

### gRPC（内部 dispatch 接入）— `:5005`

proto 定义见 `api/gateway/proto/gateway.proto`。

| RPC | 方向 | 说明 |
|---|---|---|
| `ExecuteCommand` | Write | 内部 CUCode → 外部设备 ID，下发控制指令 |

---

## 目录结构

```
internal/gateway/
├── cmd/
│   └── main.go                          # 进程入口
├── app.go                               # cobra 命令 + viper 配置加载
├── run.go                               # tracing + 启动
├── server.go                            # 组装根：HTTP + gRPC 双服务器
├── config/
│   └── config.go
├── options/
│   └── options.go
├── application/
│   ├── app.go
│   ├── command/
│   │   ├── receive_telemetry.go         # EMS 遥测 → telemetry gRPC
│   │   ├── execute_command.go           # dispatch 指令 → EMS
│   │   ├── create_mapping.go
│   │   ├── delete_mapping.go
│   │   └── disable_mapping.go
│   └── query/
│       └── list_mappings.go
├── domain/
│   ├── errors.go
│   ├── model/
│   │   ├── device_mapping.go
│   │   ├── external_telemetry.go
│   │   └── standard_telemetry.go
│   └── port/
│       ├── mapping_repository.go
│       ├── telemetry_client.go
│       └── ems_client.go
├── adapter/
│   ├── inbound/
│   │   ├── http/                        # Gin 路由（无 grpc-gateway）
│   │   └── grpc/                        # GatewayService
│   └── outbound/
│       ├── postgres/                    # MappingRepository 适配
│       ├── telemetry_grpc/              # → vpp-telemetry
│       ├── simulator/                   # vpp-simulator HTTP 出站
│       └── ems_log/                     # log-only default
└── infrastructure/persistent/postgres/  # GORM 仓储实现
```

---

## 依赖组件

| 组件 | 用途 | 端口 |
|---|---|---|
| Postgres | 设备映射持久化（`gateway` 库） | 5432 |
| vpp-telemetry | 遥测写入（gRPC `IngestTelemetry`） | 5003 |
| Jaeger | 分布式链路追踪 | 4318 |
| Prometheus | 指标采集 | 9104（本服务 `/metrics`） |

### 服务端口一览

| 服务 | gRPC | HTTP | Metrics |
|---|---|---|---|
| resource | `:5002` | — | `:9102` |
| telemetry | `:5003` | — | `:9103` |
| **gateway** | **`:5005`** | **`:8083`** | **`:9104`** |

> gateway gRPC 使用 **5005** 而非 5004。部分 WSL2 环境下 5004 端口会出现「能 bind 但 connect 被拒绝」的异常，已在本地验证；若遇类似问题请换端口。

---

## 启动方式

### 1. 启动基础设施

```bash
# 在项目根目录执行
make infra-up
```

> **首次初始化 Postgres**：init 脚本仅在 `data/postgres_data` 为空时执行。若库表缺失，需清空数据卷后重建：
> ```bash
> docker compose down
> rm -rf data/postgres_data
> make infra-up
> ```

### 2. 启动依赖服务

gateway 依赖 **telemetry** 服务（出站 gRPC）。测试前需先启动：

```bash
make run-telemetry   # 终端 1
make run-gateway     # 终端 2
```

### 3. 配置文件

配置文件位于 `config/gateway.yaml`：

```yaml
gateway:
  grpc-addr: 127.0.0.1:5005      # 内部 gRPC（dispatch 调用）
  http-addr: 127.0.0.1:8083      # 外部 EMS HTTP
  metrics-addr: 127.0.0.1:9104
  consul-addr: ""

database:
  host: 127.0.0.1
  port: 5432
  dbname: gateway
  user: postgres
  password: postgres123

telemetry-grpc:
  addr: 127.0.0.1:5003           # vpp-telemetry gRPC 地址
```

### 4. 启动成功日志特征

```
telemetry_grpc: connected to 127.0.0.1:5003
HTTP server listening on 127.0.0.1:8083
gRPC server listening on 127.0.0.1:5005
```

---

## 功能测试

端到端测试需要 **telemetry + gateway** 同时运行。HTTP 测试用 `curl`，gRPC 测试推荐 [`grpcurl`](https://github.com/fullstorydev/grpcurl)：

```bash
go install github.com/fullstorydev/grpcurl/cmd/grpcurl@latest
```

以下示例统一使用租户 `001`：

```bash
export TENANT=001
export BASE=http://127.0.0.1:8083/api/v1/tenants/${TENANT}
```

---

### 0. 前置检查

```bash
# telemetry gRPC 可达
grpcurl -plaintext 127.0.0.1:5003 list telemetrypb.TelemetryService

# gateway gRPC 可达（注意端口 5005）
grpcurl -plaintext 127.0.0.1:5005 list gatewaypb.GatewayService

# gateway HTTP 可达
curl -s -o /dev/null -w '%{http_code}\n' "${BASE}/mappings"
```

---

### 1. CreateMapping — 创建设备映射

将 EMS 外部设备 `ems-sg / SG001` 映射到内部 CU `cu-001`：

```bash
curl -s -X POST "${BASE}/mappings" \
  -H "Content-Type: application/json" \
  -d '{
    "external_system": "ems-sg",
    "external_id": "SG001",
    "cu_code": "cu-001"
  }' | jq .
```

预期响应（示例）：

```json
{
  "id": "019f1d3f-1e02-744e-8f97-a48c643b6ae3",
  "tenant_id": "001",
  "external_system": "ems-sg",
  "external_id": "SG001",
  "cu_code": "cu-001",
  "status": "active"
}
```

记录返回的 `id`，后续禁用/删除测试会用到：

```bash
export MAPPING_ID="<上一步返回的 id>"
```

---

### 2. ListMappings — 查询映射列表

```bash
curl -s "${BASE}/mappings" | jq .
```

---

### 3. ReceiveTelemetry — EMS 上报遥测（HTTP）

```bash
curl -v -X POST "${BASE}/telemetry:ingest" \
  -H "Content-Type: application/json" \
  -d '{
    "external_system": "ems-sg",
    "external_id": "SG001",
    "timestamp": "2026-06-29T10:00:00Z",
    "metrics": [
      {"name": "p_act", "value": 100.5},
      {"name": "q_act", "value": 20.0}
    ]
  }'
```

预期：**HTTP 204 No Content**（gateway 查映射 → 转发 telemetry gRPC）。

---

### 4. GetSnapshot — 验证 telemetry 侧已写入

在 **telemetry** 服务上查询快照。**TenantID 必须与映射一致**（本例为 `001`，不要用 `t-demo`）：

```bash
grpcurl -plaintext -d '{
  "TenantID": "001",
  "CUCode":   "cu-001"
}' 127.0.0.1:5003 telemetrypb.TelemetryService/GetSnapshot
```

预期响应（示例）：

```json
{
  "TenantID": "001",
  "CUCode": "cu-001",
  "Metrics": {
    "p_act": 100.5,
    "q_act": 20
  },
  "UpdatedAt": "2026-06-29T10:00:00Z",
  "Stale": true
}
```

> `Stale: true` 表示快照时间距当前超过默认阈值，测试数据时间戳较旧时会出现，属正常现象。

---

### 5. DisableMapping — 禁用映射

```bash
curl -v -X PATCH "${BASE}/mappings/${MAPPING_ID}/disable"
```

预期：**HTTP 204**。

禁用后再次上报遥测，应返回 **409 Conflict**：

```bash
curl -v -X POST "${BASE}/telemetry:ingest" \
  -H "Content-Type: application/json" \
  -d '{
    "external_system": "ems-sg",
    "external_id": "SG001",
    "metrics": [{"name": "p_act", "value": 100}]
  }'
```

预期响应：

```json
{"error":"device mapping is disabled"}
```

---

### 6. DeleteMapping + 重建映射

```bash
curl -v -X DELETE "${BASE}/mappings/${MAPPING_ID}"

curl -s -X POST "${BASE}/mappings" \
  -H "Content-Type: application/json" \
  -d '{
    "external_system": "ems-sg",
    "external_id": "SG001",
    "cu_code": "cu-001"
  }'
```

---

### 7. ExecuteCommand — dispatch 下发控制指令（gRPC）

```bash
grpcurl -plaintext -d '{
  "TenantID": "001",
  "CUCode":   "cu-001",
  "Command":  "set_power",
  "Value":    500
}' 127.0.0.1:5005 gatewaypb.GatewayService/ExecuteCommand
```

预期响应：

```json
{
  "ExternalID": "SG001",
  "ExternalSystem": "ems-sg"
}
```

gateway 日志中可见 EMS log-only 输出：

```
ems_log: command dispatched (log-only, no real EMS connection)
  external_system=ems-sg external_id=SG001 command=set_power value=500
```

---

### Prometheus 指标

```bash
curl http://127.0.0.1:9104/metrics | grep app_
```

---

## 当前开发进度

### ✅ 已完成

- [x] 领域模型：`DeviceMapping`、`ExternalTelemetry`、`StandardTelemetry`
- [x] 出站端口接口（`MappingRepository` / `TelemetryClient` / `EMSClient`）
- [x] 应用层 CQRS：遥测接入、指令下发、映射 CRUD + 禁用
- [x] HTTP 入站适配器（Gin，纯 REST，无 grpc-gateway）
- [x] gRPC 入站适配器（`GatewayService.ExecuteCommand`）
- [x] Postgres 映射仓储（GORM + 迁移脚本）
- [x] Telemetry gRPC 出站客户端（`IngestTelemetry`）
- [x] EMS log-only 出站客户端（v1 占位）
- [x] 双服务器生命周期（gRPC GracefulStop + HTTP Shutdown）
- [x] Proto 定义与代码生成（`api/gateway/proto/gateway.proto`）
- [x] 配置系统 + `config/gateway.yaml`
- [x] Makefile 快捷命令（`make run-gateway`）
- [x] 端到端功能测试流程（映射 → 遥测上报 → snapshot 验证 → 指令下发）

### 🚧 进行中 / 待完成

- [ ] **真实 EMS 适配器**：替换 `ems_log`，对接具体厂商协议
- [ ] **Kafka 消费者**：订阅 resource CU 生命周期事件，自动禁用/清理 stale mapping（v2）
- [ ] **Resource 存在性校验**：创建映射时可选调用 resource gRPC 验证 CUCode（v2）
- [ ] **单元 / 集成测试**：Application Handler mock 测试 + testcontainers 端到端
- [x] **APISIX key-auth（Phase 1）**：外部 EMS 经 APISIX `:9080/gateway/*` 需 `X-API-KEY`；Gateway 应用内不重复校验
- [x] **APISIX + Casdoor OIDC（Phase 2）**：管理端经 `:9080/resource/*`（Resource 侧 RBAC；与 Gateway EMS 线独立）
- [ ] **mTLS**（真实 EMS 对接，APISIX `mutual-tls` 插件）

### 📌 已知限制

| 限制 | 说明 |
|---|---|
| EMS 为 log-only | v1 不连接真实 EMS，仅验证 dispatch → mapping → adapter 链路 |
| 映射手动维护 | v1 无 Kafka 自动同步；CU 删除后需运维手动 `DisableMapping` |
| 无应用内 HTTP 鉴权 | 认证由 APISIX key-auth 承担（Phase 1）；直连 `:8083` 仅用于本地调试 |
| WSL 5004 端口 | 部分 WSL2 环境 TCP 5004 不可用，已改用 5005 |
