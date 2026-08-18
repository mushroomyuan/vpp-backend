# vpp-dispatch

VPP 平台的**调度执行服务**。负责将上层下发的控制意图编排为可执行的 Task → Action → Command 状态机，通过 Gateway 完成设备侧下发，并消费命令结果事件推进后续步骤。

- **入站 gRPC**：管理端 / 算法侧提交调度任务、查询任务状态
- **出站 gRPC**：调用 `vpp-gateway.ExecuteCommand` 下发控制指令
- **入站 Kafka**：消费 `vpp.command.events`，驱动异步 continuation
- **出站 Kafka**：发布任务生命周期事件到 `vpp.dispatch.events`（告警/监控可选消费）

本服务**不负责**：协议转换、CUCode → ExternalID 映射、与 EMS 直连（均属 gateway）。

设计细节见 [plan_v2.md](./plan_v2.md)。

---

## 目录

- [服务职责](#服务职责)
- [架构设计](#架构设计)
- [核心执行链路](#核心执行链路)
- [数据存储](#数据存储)
- [API 接口](#api-接口)
- [目录结构](#目录结构)
- [依赖组件](#依赖组件)
- [启动方式](#启动方式)
- [基础连通测试](#基础连通测试)
- [关键设计约定](#关键设计约定)
- [当前开发进度](#当前开发进度)

---

## 服务职责

| 职责 | 说明 |
|---|---|
| **任务编排** | 维护 `DispatchTask` → `DispatchAction` → `ControlCommand` 聚合与状态机 |
| **顺控 / 并发** | Action 间按 `Sequence` 顺序执行；Action 内按 `ExecutionPolicy`（sequential / parallel）调度 Command |
| **指令下发** | 通过 `GatewayPort` 同步调用 Gateway；接受结果为 `Accepted` 后等待 Kafka 终态回调 |
| **结果推进** | 消费 `command.completed` 事件，幂等推进 Command / Action / Task，并触发 Sequential continuation |
| **超时重试** | `TimeoutScanner` 扫描 `Sending` 且过期的命令，可重试则重置为 Pending 再下发，否则 FailFast 熔断 |
| **熔断不干预** | v1 固定 `FailurePolicy=fail_fast`：失败后取消剩余 Pending，Task → Failed，**不做自动补偿回滚** |

---

## 架构设计

服务采用**六边形架构 + CQRS**，对外暴露 **gRPC**；异步路径由 Kafka consumer 与 TimeoutScanner 驱动。

```
管理端 / 算法
      │ gRPC SubmitTask / GetTask
      ▼
┌─────────────────────────────────────────────────────────────┐
│                     Inbound Adapters                         │
│   adapter/inbound/grpc/          adapter/inbound/kafka/      │
│   · SubmitTask · GetTask         · CommandResultConsumer     │
│   · CancelTask (Unimplemented)     (vpp.command.events)      │
└────────────────────────────┬────────────────────────────────┘
                             │ Command / Query
┌────────────────────────────▼────────────────────────────────┐
│                      Application Layer                       │
│  Commands                          Queries                   │
│  · SubmitTask                      · GetTask                 │
│  · HandleCommandResult                                       │
│  · TimeoutScanner (goroutine)                                │
│  port: GatewayPort（集成语义，非 domain）                      │
└──────┬──────────────────────────────┬───────────────────────┘
       │ domain ports                 │
┌──────▼──────────────────────────────▼───────────────────────┐
│                       Domain Layer                           │
│  model: DispatchTask · DispatchAction · ControlCommand       │
│  service: Dispatcher · Validator                             │
│  port: TaskRepository · ActionRepository · CommandRepository │
│        TaskEventPublisher                                    │
└──────┬──────────────────────────────┬───────────────────────┘
       │ implements                   │
┌──────▼──────────────┐    ┌──────────▼───────────────────────┐
│ adapter/outbound/   │    │ adapter/outbound/                 │
│ postgres/           │    │ gateway_grpc/ → vpp-gateway       │
│ (三 Repository)     │    │ kafka/        → vpp.dispatch.events│
└──────────┬──────────┘    └──────────────────────────────────┘
           ▼
┌──────────────────────┐
│  infrastructure/     │
│  persistent/postgres │  ← raw GORM（仅 *Model）
└──────────────────────┘
```

### 分层依赖规则

```
domain ← application ← adapter ← infrastructure
```

- `domain`：实体、状态机、`Dispatcher`、业务 port；不依赖 application / adapter
- `application`：用例编排；依赖 domain port + `application/port.GatewayPort`
- `adapter`：实现 port（Postgres / Gateway gRPC / Kafka）
- `infrastructure`：GORM model 与原始 SQL/CRUD，不引用 domain model

### 三 Repository（写频率分层）

| Repository | 表 | 写频率 | 典型场景 |
|---|---|---|---|
| `TaskRepository` | `dispatch_tasks` | 极低 | 创建、开始、完成/失败 |
| `ActionRepository` | `dispatch_actions` | 中 | Action 状态推进 |
| `CommandRepository` | `control_commands` | 极高 | 每次 Kafka 回调 / 下发 Sending |

Application 按 `CommandResultOutcome` **只更新真正变化的行**，避免整棵聚合写放大。

---

## 核心执行链路

### 全异步路径（Gateway 返回 Accepted）

```
管理端 ──gRPC SubmitTask──▶ Dispatch
  ├─ 构建 Task（FailFast）+ idgen 分配 ID
  ├─ validator → taskRepo.Save（Pending 整树）
  ├─ task.Start / first Action.Start
  ├─ gateway.ExecuteCommand(Command1)
  │     └─ GatewayAccepted → Command1 = Sending
  └─ 返回 TaskID (status=running)

Gateway（ems_log v1 同步成功）
  └─ Publish CommandCompleted → vpp.command.events

Dispatch CommandResultConsumer
  └─ HandleCommandResult
        ├─ 幂等：终态则忽略
        ├─ Dispatcher.OnCommandResult → Succeeded
        ├─ Sequential：NextCommands 继续下发
        └─ 全部完成 → Task = Completed + PublishTaskCompleted
```

### 超时路径

```
TimeoutScanner（默认每 10s）
  → FindExpiredSending(now)
  → OnCommandTimeout
       ├─ CanRetry → Pending + 重新 ExecuteCommand
       └─ 耗尽重试 → FailFast 熔断 → Task = Failed
```

### 与周边服务关系

```
管理端 ──gRPC──▶ Dispatch (:5006)
Dispatch ──gRPC ExecuteCommand──▶ Gateway (:5005)
Gateway  ──Kafka vpp.command.events──▶ Dispatch
Dispatch ──Kafka vpp.dispatch.events──▶ （告警/监控，可选）
```

---

## 数据存储

### Postgres — `dispatch` 库

Schema：`migrations/dispatch/000001_init.up.sql`  
Compose 首次初始化：`migrations/initdb/50-dispatch-db.sh` + `51-dispatch-schema.sql`

> 若 Postgres 数据卷在加入 init 脚本**之前**已创建，需手动建库并执行 up migration（见下方连通测试）。

| 表 | 说明 |
|---|---|
| `dispatch_tasks` | 任务根：类型、触发方式、`failure_policy`、状态、时间戳 |
| `dispatch_actions` | Action：`sequence`、`execution_policy`、状态 |
| `control_commands` | Command：`sequence`、CUCode、PointKey、`value`/`result` JSONB、超时字段 |

TimeoutScanner 依赖 partial index：

```sql
CREATE INDEX idx_control_commands_timeout_scan
    ON control_commands (deadline_at)
    WHERE status = 'sending';
```

### Kafka Topics

| Topic | 方向 | 说明 |
|---|---|---|
| `vpp.command.events` | 消费 | Gateway 发布的 `command.completed`（`platform/event/gateway`） |
| `vpp.dispatch.events` | 生产 | Task started / completed / failed（`platform/event/dispatch`） |

`kafka.brokers` 为空时：consumer / publisher 降级为 no-op，服务仍可启动（无法走异步闭环）。

---

## API 接口

### gRPC — `:5006`

proto：`api/dispatch/proto/dispatch.proto`  
包名：`dispatchpb.DispatchService`

| RPC | 说明 | v1 状态 |
|---|---|---|
| `SubmitTask` | 创建任务并同步下发第一批命令 | ✅ |
| `GetTask` | 按 TenantID + TaskID 查询完整快照 | ✅ |
| `CancelTask` | 取消任务 | ❌ Unimplemented |

字段命名与仓库其他服务一致，采用 **PascalCase**（如 `TenantID`、`CUCode`）。

#### SubmitTask（摘要）

```text
SubmitTaskRequest
  TenantID, Name, Description
  TaskType      // "control"
  TriggerType   // "manual" | "scheduled" | "automatic"
  Actions[]
    Name, ActionType, Sequence
    ExecutionPolicy  // "sequential" | "parallel"
    Commands[]
      CUCode, PointKey
      Value oneof { BoolValue | IntValue | FloatValue | StringValue }
      TimeoutSeconds  // 0 → 默认 30s
      MaxRetries      // 0 → 默认 3

SubmitTaskResponse
  TaskID, Status  // 通常为 "running"
```

#### GetTask（摘要）

返回 Task 状态、`FailurePolicy`，以及嵌套的 Action / Command 状态（含失败时的 `ErrorCode` / `ErrorMessage`）。

---

## 目录结构

```
internal/dispatch/
├── cmd/main.go
├── app.go / run.go / server.go          # composition root
├── config/ · options/
├── application/
│   ├── app.go
│   ├── port/gateway.go                  # GatewayPort（集成语义）
│   ├── command/
│   │   ├── submit_task.go
│   │   ├── handle_command_result.go
│   │   ├── scan_timeouts.go
│   │   └── dispatch_helper.go           # 下发 + 三 repo 精确持久化
│   └── query/get_task.go
├── domain/
│   ├── model/ · port/ · service/
├── adapter/
│   ├── inbound/grpc/ · inbound/kafka/
│   └── outbound/postgres/ · gateway_grpc/ · kafka/
└── infrastructure/persistent/postgres/
```

---

## 依赖组件

| 组件 | 用途 | 默认地址 |
|---|---|---|
| PostgreSQL | 任务持久化（库名 `dispatch`） | `127.0.0.1:5432` |
| Kafka | 命令结果消费 + 任务事件发布 | `127.0.0.1:9092` |
| vpp-gateway | `ExecuteCommand` | gRPC `127.0.0.1:5005` |
| Jaeger OTLP | Tracing（可选） | `127.0.0.1:4318` |
| Prometheus | Metrics | `127.0.0.1:9105` |

配置文件：仓库根目录 [`config/dispatch.yaml`](../../config/dispatch.yaml)。

---

## 启动方式

### 1. 基础设施

```bash
# 仓库根目录
make infra-up
# 或
docker compose up -d postgres kafka jaeger
```

确认 `dispatch` 库存在（已有数据卷时可能需手动初始化）：

```bash
docker exec -i vpp-backend-postgres-1 \
  psql -U postgres -c "SELECT 'CREATE DATABASE dispatch' WHERE NOT EXISTS (SELECT FROM pg_database WHERE datname = 'dispatch')\gexec"

docker exec -i vpp-backend-postgres-1 \
  psql -U postgres -d dispatch -v ON_ERROR_STOP=1 \
  < migrations/dispatch/000001_init.up.sql
```

建议预先创建 Kafka topic（也可依赖 auto-create）：

```bash
docker exec vpp-backend-kafka-1 kafka-topics.sh \
  --bootstrap-server localhost:9092 --create --if-not-exists \
  --topic vpp.command.events --partitions 1 --replication-factor 1
```

### 2. 启动服务

需先启动 **gateway**（dispatch 启动时会拨号 Gateway）：

```bash
make run-gateway     # :5005 / HTTP :8083
make run-dispatch    # :5006
```

或：

```bash
cd internal/gateway  && go run ./cmd/main.go -c ../../config/gateway.yaml
cd internal/dispatch && go run ./cmd/main.go -c ../../config/dispatch.yaml
```

---

## 基础连通测试

以下流程验证：**SubmitTask → Gateway ExecuteCommand → Kafka CommandCompleted → HandleCommandResult → GetTask=completed**。

前置：gateway + dispatch 已启动，`config/*.yaml` 中 `kafka.brokers` 指向 `127.0.0.1:9092`。

### 1. 在 Gateway 创建 Mapping

Dispatch 只认 `CUCode`；Gateway 需有 active mapping，否则 `ExecuteCommand` 返回 Rejected。

```bash
curl --noproxy '*' -sS -X POST \
  'http://127.0.0.1:8083/api/v1/tenants/tenant-e2e/mappings' \
  -H 'Content-Type: application/json' \
  -d '{
    "external_system": "ems-test",
    "external_id": "dev-001",
    "cu_code": "cu-e2e-001"
  }'
```

### 2. 提交调度任务

```bash
grpcurl -plaintext -d '{
  "TenantID": "tenant-e2e",
  "Name": "e2e-control-task",
  "Description": "smoke test",
  "TaskType": "control",
  "TriggerType": "manual",
  "Actions": [{
    "Name": "set-power",
    "ActionType": "control",
    "Sequence": 1,
    "ExecutionPolicy": "sequential",
    "Commands": [{
      "CUCode": "cu-e2e-001",
      "PointKey": "active_power",
      "FloatValue": 100.5,
      "TimeoutSeconds": 30,
      "MaxRetries": 3
    }]
  }]
}' 127.0.0.1:5006 dispatchpb.DispatchService/SubmitTask
```

期望响应：

```json
{
  "TaskID": "<uuid-v7>",
  "Status": "running"
}
```

### 3. 查询任务终态

将上一步 `TaskID` 代入（等待约 1–3 秒，等 Kafka 回调）：

```bash
grpcurl -plaintext -d '{
  "TenantID": "tenant-e2e",
  "TaskID": "<uuid-v7>"
}' 127.0.0.1:5006 dispatchpb.DispatchService/GetTask
```

期望：

```json
{
  "TaskID": "...",
  "Status": "completed",
  "FailurePolicy": "fail_fast",
  "Actions": [{
    "Status": "completed",
    "Commands": [{
      "CUCode": "cu-e2e-001",
      "PointKey": "active_power",
      "Status": "succeeded"
    }]
  }]
}
```

### 4. 旁路校验

```bash
# Postgres
docker exec vpp-backend-postgres-1 psql -U postgres -d dispatch \
  -c "SELECT id, status FROM dispatch_tasks ORDER BY created_at DESC LIMIT 3;"
docker exec vpp-backend-postgres-1 psql -U postgres -d dispatch \
  -c "SELECT id, status, cu_code, point_key FROM control_commands ORDER BY id DESC LIMIT 5;"

# 日志关键字
# gateway:  ems_log: command dispatched
# dispatch: HandleCommandResult ... Success:true
```

### 一次实测结果（Phase 7）

| 步骤 | 结果 |
|---|---|
| CreateMapping | `status=active` |
| SubmitTask | `Status=running`，返回 TaskID |
| Gateway | `ems_log` 收到 `command_id` + `active_power=100.5` |
| Kafka → HandleCommandResult | 约 1s 内处理成功 |
| GetTask | `Task/Action/Command` 均为 completed / succeeded |
| DB | `dispatch_tasks.status=completed`，`control_commands.status=succeeded` |

---

## 关键设计约定

1. **Dispatch 编排，Gateway 适配**  
   Dispatch 只知道 `CUCode` + `PointKey` + `CommandValue`；外部系统 ID / 协议细节在 Gateway。

2. **同步接受 ≠ 业务成功**  
   Gateway gRPC 成功映射为 `GatewayAccepted`；设备终态以 Kafka `command.completed` 为准（v1 `ems_log` 在同步成功后立即发事件，便于闭环联调）。

3. **FailFast 熔断不干预**  
   任一命令失败：取消剩余 Pending，Task → Failed，发布失败事件；**不自动下发反向补偿**。

4. **幂等**  
   `HandleCommandResult` 对已终态 Command 直接忽略，适配 Kafka at-least-once。

5. **Kafka 瞬时故障不拖垮进程**  
   Consumer `FetchMessage` 失败时 warn + 退避重试，避免 errgroup 整服务退出。

6. **StringValue**  
   Gateway EMS v1 仅支持数值（bool→0/1）；`StringValue` 在 Gateway 侧会 Rejected。

---

## 当前开发进度

| 阶段 | 内容 | 状态 |
|---|---|---|
| Phase 1 | Gateway proto（CommandID / PointKey / oneof Value）+ handler | ✅ |
| Phase 2 | Dispatch domain | ✅ |
| Phase 3 | infrastructure + postgres adapter | ✅ |
| Phase 4 | application（Submit / HandleResult / TimeoutScanner / GetTask） | ✅ |
| Phase 5 | adapters + composition root | ✅ |
| Phase 6 | Gateway Kafka `CommandEventPublisher` | ✅ |
| Phase 7 | 端到端连通（Submit → Gateway → Kafka → completed） | ✅ |

### 已知未做 / 后续

- `CancelTask` 业务实现
- 真实 EMS 适配器（替换 `ems_log`）与真正异步设备回执
- 多 Action 顺控、Parallel、超时重试、熔断路径的专项用例
- 告警服务消费 `vpp.dispatch.events`
