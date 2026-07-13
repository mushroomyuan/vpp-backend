# vpp-simulator

长期运行的**虚拟设备运行时（Virtual Device Runtime）**。在没有真实 EMS / 设备时，为 Resource / Gateway / Telemetry / Dispatch 提供可重复的闭环测试与演示环境。

Simulator 扮演 Gateway 侧的外部系统：`ExternalSystem = "simulator"`。它可以模拟储能、PCS、光伏、电表等任意 CU，不限于能源管理系统。

---

## 职责边界

| 负责 | 不负责 |
|------|--------|
| 从 Resource 只读加载 Site→Asset→CU→Point | 资源 CRUD |
| 内存设备状态机 + Tick 自然演化 | 协议转换 / ID 映射（Gateway） |
| 经 Gateway 上报遥测、接收控制命令 | 时序存储（Telemetry） |
| Debug API / 故障注入 | 任务编排（Dispatch） |

---

## 闭环数据流

```
Resource ──List──▶ Simulator ──HTTP telemetry:ingest──▶ Gateway ──gRPC──▶ Telemetry
                                                              ▲
Dispatch ──gRPC ExecuteCommand──▶ Gateway ──HTTP /api/v1/commands──┘
                                      │
                                      └── Kafka command.completed ──▶ Dispatch
```

---

## 端口

| 端口 | 用途 |
|------|------|
| HTTP `:8084` | 命令入站 + Debug API |
| Metrics `:9106` | Prometheus |

配置：[`config/simulator.yaml`](../../config/simulator.yaml)

**联调步骤与用例见 [TESTING.md](./TESTING.md)。**

遥测默认每 **30s** 上报一次（`runtime.tick-interval`）；设 `runtime.publish-enabled: false` 可只做内存 Tick、不写 Telemetry。清理本地时序：`make clean-telemetry`。

---

## 启动

```bash
# 1) 基础设施 + 四服务已启动后，灌入 demo 资源与 mapping
./scripts/seed_simulator_demo.sh

# 2) 启动 Simulator（会按 Provider=simulator 从 Resource 加载 CU）
cd internal/simulator
go run ./cmd -c ../../config/simulator.yaml

# 3) 查看运行时
curl -s http://127.0.0.1:8084/api/v1/runtime | jq .
```

Gateway 需配置出站地址（已写入 `config/gateway.yaml`）：

```yaml
simulator:
  addr: http://127.0.0.1:8084
```

`ExternalSystem=simulator` 的命令走 Simulator；其它系统仍走 `ems_log`（或后续真实适配器）。

---

## HTTP API

### 命令（Gateway 调用）

```
POST /api/v1/commands
{ "command_id", "external_id", "point_key", "value" }
→ { "accepted": true }
```

### Debug

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/v1/runtime` | 全部设备摘要 |
| GET | `/api/v1/devices/:id` | 单设备（CUCode 或 ExternalID） |
| POST | `/api/v1/devices/:id/command` | 本地注入命令 |
| POST | `/api/v1/faults` | 注入故障：`offline` / `command_reject` / `telemetry_delay` / `clear` |
| GET | `/api/v1/faults` | 当前故障列表 |
| POST | `/api/v1/runtime/reset` | 重置设备初值 |
| POST | `/api/v1/runtime/reload` | 重新从 Resource 加载 |
| GET | `/healthz` | 健康检查 |

故障注入示例：

```bash
curl -s -X POST http://127.0.0.1:8084/api/v1/faults \
  -H 'Content-Type: application/json' \
  -d '{"key":"sim-battery-001","kind":"offline"}'
```

---

## 设备加载规则

启动时：

1. `ListSites` → `ListAssets` → `ListCUs` → `ListPoints`
2. 过滤：`Provider == runtime.require-provider`（默认 `simulator`）
3. 可选白名单：`runtime.site-ids` / `runtime.cu-ids`
4. 按 `CU.Type` 实例化：`Battery` / `PCS` / `PV` / `Meter`，未知类型用 Passthrough
5. Point 作为遥测/控制模板；`Snapshot()` 只输出 Resource 已声明的 PointKey

Onboarding 约定（与 architecture.md 一致）：

1. Resource `CreateCU`（`provider=simulator`）→ CU UUID  
2. Gateway `CreateMapping`：`external_system=simulator`，`cu_code=<CU UUID>`  
3. Simulator 启动后自动加载并 Tick

---

## 目录结构

```
internal/simulator/
  cmd/main.go
  api/                 # HTTP command + debug
  client/resource/     # gRPC → Resource
  client/gateway/      # HTTP → Gateway ingest
  device/              # Battery / PCS / PV / Meter / Passthrough
  domain/
  fault/
  runtime/             # Device manager
  telemetry/           # Publisher
  tick/                # Global tick loop
  config/ options/
```

---

## 与 Gateway 的适配

Gateway 出站包：`internal/gateway/adapter/outbound/simulator/`

- `Client`：HTTP 调用 Simulator 命令 API  
- `Router`：按 `ExternalSystem` 路由到 Simulator 或 default（`ems_log`）

命名刻意使用 **simulator** 而非 ems_*：Simulator 可模拟任意外部系统/单设备 CU，不假定对方是 EMS。
