# vpp-simulator 全面测试指南

Phase 1 已具备闭环能力：Resource 加载 → Tick 遥测 → Gateway 入库 → 命令下发 → 状态反馈。  
本指南覆盖从环境启动到 Dispatch 全链路验证，以及 Debug / 故障注入。

---

## 0. 当前状态说明


| 项                                                                           | 状态                             |
| --------------------------------------------------------------------------- | ------------------------------ |
| Simulator 核心（Runtime / Device / Tick / Telemetry / Command / Fault / Debug） | ✅ Phase 1 完成                   |
| Gateway `adapter/outbound/simulator` 路由                                     | ✅ 完成                           |
| Scenario Engine / Kafka 动态增删设备                                              | ❌ Phase 2，未做                   |
| 单元测试套件                                                                      | ❌ 本指南以手工 / curl / grpcurl 联调为主 |


---

## 1. 前置条件

### 1.1 基础设施（docker compose）

```bash
cd /home/yfz/project/vpp-backend
docker compose up -d postgres redis kafka consul jaeger
```

确认端口：`5432` / `6379` / `9092` / `8500` / `4318`。

### 1.2 启动五个服务（各开一个终端）

建议启动顺序：resource → telemetry → gateway → dispatch → simulator。

```bash
# T1
cd internal/resource && go run ./cmd -c ../../config/resource.yaml

# T2
cd internal/telemetry && go run ./cmd -c ../../config/telemetry.yaml

# T3  （需已配置 simulator.addr）
cd internal/gateway && go run ./cmd -c ../../config/gateway.yaml

# T4
cd internal/dispatch && go run ./cmd -c ../../config/dispatch.yaml

# T5  （先 seed 再启；或启后 POST /runtime/reload）
cd internal/simulator && go run ./cmd -c ../../config/simulator.yaml
```

健康检查：

```bash
curl -s http://127.0.0.1:8082/api/tenants/default/sites | head -c 200; echo
curl -s http://127.0.0.1:8083/api/v1/tenants/default/mappings | head -c 200; echo
curl -s http://127.0.0.1:8084/healthz; echo
```


| 服务        | 端口                             |
| --------- | ------------------------------ |
| resource  | HTTP `:8082` / gRPC `:5002`    |
| telemetry | gRPC `:5003`                   |
| gateway   | HTTP `:8083` / gRPC `:5005`    |
| dispatch  | gRPC `:5006`                   |
| simulator | HTTP `:8084` / metrics `:9106` |




### 1.3 工具

```bash
# 可选但推荐
sudo apt install -y jq   # 或已有即可
# Dispatch 测试需要
go install github.com/fullstorydev/grpcurl/cmd/grpcurl@latest
```

---



## 2. 注入假数据（Onboarding）

Resource 的 grpc-gateway 使用 **PascalCase** JSON（`Name` / `CUID`）；Gateway 使用 **snake_case**。

### 2.1 一键脚本（推荐）

```bash
./scripts/seed_simulator_demo.sh
# IDs 写入 /tmp/vpp-simulator-seed.env
source /tmp/vpp-simulator-seed.env
```

脚本会创建：

- 1 Site + 1 Asset（ESS）
- 4 CU：`Battery` / `PCS` / `PV` / `Meter`（`Provider=simulator`）
- 对应 Point + Gateway mapping（`external_system=simulator`）



### 2.2 本次环境已注入的 ID（可直接用）

若你刚跑过注入且未清库，当前 demo 数据为：


| 角色      | CU UUID (`CUCode`)                     | ExternalID        |
| ------- | -------------------------------------- | ----------------- |
| Battery | `019f4b66-f221-78f4-90f1-7ece00068084` | `sim-battery-001` |
| PCS     | `019f4b66-f28b-7451-967c-830aed0ac444` | `sim-pcs-001`     |
| PV      | `019f4b66-f2cf-7d61-a159-8c2cc3c24ffc` | `sim-pv-001`      |
| Meter   | `019f4b66-f301-769a-a823-f7aa9a3dce13` | `sim-meter-001`   |


Tenant：`default`  
Site：`019f4b66-b548-7c64-8b3f-e3e5089096ca`  
Asset：`019f4b66-f205-701b-98bd-51868b8453dc`

校验：

```bash
curl -s 'http://127.0.0.1:8082/api/tenants/default/cus' | jq '.CUs[] | {ID,Name,Type,Provider,ExternalID}'
curl -s 'http://127.0.0.1:8083/api/v1/tenants/default/mappings' | jq .
```



### 2.3 启动 / 重载 Simulator

```bash
# 若 Simulator 已在跑且 seed 在其后完成：
curl -s -X POST http://127.0.0.1:8084/api/v1/runtime/reload | jq .

# 查看加载结果（应有 4 台设备）
curl -s http://127.0.0.1:8084/api/v1/runtime | jq '{device_count, devices: [.devices[] | {name,type,cu_code,external_id,status}]}'
```

期望：`device_count == 4`，Battery 上能看到 `read_soc` 等点。

---



## 3. 测试用例

下面用环境变量简化命令（按你的实际 ID 改）：

```bash
export TENANT=default
export BAT_CU=019f4b66-f221-78f4-90f1-7ece00068084
export BAT_EXT=sim-battery-001
export SIM=http://127.0.0.1:8084
export GW=http://127.0.0.1:8083
```

---



### T1. Simulator 健康与 Runtime 可见

**目的：** 服务存活，设备已从 Resource 加载。

```bash
curl -s $SIM/healthz | jq .
curl -s $SIM/api/v1/runtime | jq .
curl -s $SIM/api/v1/devices/$BAT_EXT | jq .
```

**通过标准：**

- `healthz.status == ok`
- 4 台设备，`status` 多为 `online`
- Battery 有 `read_soc` / `read_active_power` / `write_power_setpoint` 等 points

---



### T2. Tick 自然演化（遥测变化）

**目的：** Tick 会改内存状态。

```bash
# 连续看两次 SOC / 功率（间隔 > tick-interval，默认 5s）
curl -s $SIM/api/v1/devices/$BAT_EXT | jq '.points'
sleep 6
curl -s $SIM/api/v1/devices/$BAT_EXT | jq '.points'
```

**通过标准：** PV 白天应有功率；Battery 在 setpoint≠0 时 SOC 会缓慢（可先做 T3 再观察）。

---



### T3. 本地命令注入（不经 Dispatch）

**目的：** Debug API 能改设备状态。

```bash
curl -s -X POST $SIM/api/v1/devices/$BAT_EXT/command \
  -H 'Content-Type: application/json' \
  -d '{"point_key":"write_power_setpoint","value":25}' | jq .

curl -s $SIM/api/v1/devices/$BAT_EXT | jq '.points'
```

**通过标准：** `write_power_setpoint` ≈ 25；随后 Tick 后 `read_active_power` 接近设定值，`read_soc` 缓慢下降。

负向：

```bash
# 只读点应失败
curl -s -X POST $SIM/api/v1/devices/$BAT_EXT/command \
  -H 'Content-Type: application/json' \
  -d '{"point_key":"read_soc","value":50}' | jq .
# 期望 error 含 not writable
```

---



### T4. 遥测上报链路（Simulator → Gateway → Telemetry）

**目的：** 周期 ingest 成功，Telemetry 能查到数据。

等至少 1 个 tick（约 30s，见 `runtime.tick-interval`）后：

```bash
# Gateway 日志应出现 ingest 成功（无 404 mapping）
# Telemetry 快照（gRPC）
grpcurl -plaintext -d "{
  \"TenantID\": \"$TENANT\",
  \"CUCode\": \"$BAT_CU\"
}" 127.0.0.1:5003 telemetrypb.TelemetryService/GetSnapshot
```

若 `GetSnapshot` 方法名不同，可先查：

```bash
grpcurl -plaintext 127.0.0.1:5003 list
grpcurl -plaintext 127.0.0.1:5003 describe telemetrypb.TelemetryService
```

**通过标准：**

- Simulator 日志无持续 `telemetry publish failed`
- Gateway 无 mapping not found
- Telemetry 快照中有 `read_soc` / `read_active_power` 等

手动补发一条（排查用）：

```bash
curl -s -o /dev/null -w '%{http_code}\n' -X POST \
  "$GW/api/v1/tenants/$TENANT/telemetry:ingest" \
  -H 'Content-Type: application/json' \
  -d "{\"external_system\":\"simulator\",\"external_id\":\"$BAT_EXT\",\"metrics\":[{\"name\":\"read_soc\",\"value\":61.2}]}"
# 期望 204
```

---



### T5. Gateway → Simulator 命令（绕过 Dispatch）

**目的：** Gateway 出站 `simulator` 适配器可用。

```bash
# 直接打 Simulator 命令 API（模拟 Gateway 行为）
curl -s -X POST $SIM/api/v1/commands \
  -H 'Content-Type: application/json' \
  -d "{\"command_id\":\"cmd-manual-1\",\"external_id\":\"$BAT_EXT\",\"point_key\":\"write_power_setpoint\",\"value\":-15}" | jq .

curl -s $SIM/api/v1/devices/$BAT_EXT | jq '.points'
```

再经 Gateway gRPC（需 mapping）：

```bash
grpcurl -plaintext -d "{
  \"CommandID\": \"cmd-gw-1\",
  \"TenantID\": \"$TENANT\",
  \"CUCode\": \"$BAT_CU\",
  \"PointKey\": \"write_power_setpoint\",
  \"FloatValue\": 10
}" 127.0.0.1:5005 gatewaypb.GatewayService/ExecuteCommand
```

**通过标准：** 返回 `ExternalSystem=simulator`、`ExternalID=sim-battery-001`；设备 setpoint 更新。

---



### T6. 全链路 Dispatch 闭环（核心）

**目的：**  
`SubmitTask → Gateway.ExecuteCommand → Simulator → Kafka command.completed → Task completed`

```bash
grpcurl -plaintext -d "{
  \"TenantID\": \"$TENANT\",
  \"Name\": \"sim-battery-setpoint\",
  \"Description\": \"simulator e2e\",
  \"TaskType\": \"control\",
  \"TriggerType\": \"manual\",
  \"Actions\": [{
    \"Name\": \"set-power\",
    \"ActionType\": \"control\",
    \"Sequence\": 1,
    \"ExecutionPolicy\": \"sequential\",
    \"Commands\": [{
      \"CUCode\": \"$BAT_CU\",
      \"PointKey\": \"write_power_setpoint\",
      \"FloatValue\": 30,
      \"TimeoutSeconds\": 30,
      \"MaxRetries\": 1
    }]
  }]
}" 127.0.0.1:5006 dispatchpb.DispatchService/SubmitTask
```

记下 `TaskID`，约 1–3 秒后：

```bash
TASK_ID=<粘贴 TaskID>
grpcurl -plaintext -d "{
  \"TenantID\": \"$TENANT\",
  \"TaskID\": \"$TASK_ID\"
}" 127.0.0.1:5006 dispatchpb.DispatchService/GetTask
```

**通过标准：**


| 检查点        | 期望                                          |
| ---------- | ------------------------------------------- |
| SubmitTask | `Status=running`（或很快 completed）             |
| GetTask    | `Status=completed`，Command `succeeded`      |
| Simulator  | `write_power_setpoint≈30`                   |
| Gateway 日志 | `simulator: command delivered`（不是仅 ems_log） |
| Kafka      | `vpp.command.events` 有成功事件（可选消费验证）          |


旁路 SQL：

```bash
docker exec vpp-backend-postgres-1 psql -U postgres -d dispatch \
  -c "SELECT id, status FROM dispatch_tasks ORDER BY created_at DESC LIMIT 3;"
```

---



### T7. 故障注入：offline

```bash
curl -s -X POST $SIM/api/v1/faults \
  -H 'Content-Type: application/json' \
  -d "{\"key\":\"$BAT_EXT\",\"kind\":\"offline\"}" | jq .

curl -s $SIM/api/v1/devices/$BAT_EXT | jq '{status,fault}'

# 命令应被拒
curl -s -X POST $SIM/api/v1/commands \
  -H 'Content-Type: application/json' \
  -d "{\"command_id\":\"x\",\"external_id\":\"$BAT_EXT\",\"point_key\":\"write_power_setpoint\",\"value\":1}" | jq .
# 期望 accepted=false / 409

# 清除
curl -s -X POST $SIM/api/v1/faults \
  -H 'Content-Type: application/json' \
  -d "{\"key\":\"$BAT_EXT\",\"kind\":\"clear\"}" | jq .
```

**通过标准：** offline 期间不报遥测（或跳过）、命令失败；clear 后恢复。

---



### T8. 故障注入：command_reject

```bash
curl -s -X POST $SIM/api/v1/faults \
  -H 'Content-Type: application/json' \
  -d "{\"key\":\"$BAT_EXT\",\"kind\":\"command_reject\"}" | jq .

# 经 Dispatch 再 SubmitTask（同 T6）→ 期望 Gateway 失败 / Task failed 或命令未成功
# 清故障
curl -s -X POST $SIM/api/v1/faults -H 'Content-Type: application/json' \
  -d "{\"key\":\"$BAT_EXT\",\"kind\":\"clear\"}" | jq .
```

---



### T9. 故障注入：telemetry_delay

```bash
curl -s -X POST $SIM/api/v1/faults \
  -H 'Content-Type: application/json' \
  -d "{\"key\":\"$BAT_EXT\",\"kind\":\"telemetry_delay\",\"delay_ms\":3000}" | jq .
```

**通过标准：** Tick 周期内该设备上报明显变慢；`clear` 后恢复。

---



### T10. Reset / Reload

```bash
curl -s -X POST $SIM/api/v1/runtime/reset | jq .
curl -s $SIM/api/v1/devices/$BAT_EXT | jq '.points'   # setpoint 等回到初值

curl -s -X POST $SIM/api/v1/runtime/reload | jq .
# device_count 仍为 4（Resource 未删）
```

---



### T11. Mapping 缺失负向

```bash
curl -s -o /dev/null -w '%{http_code}\n' -X POST \
  "$GW/api/v1/tenants/$TENANT/telemetry:ingest" \
  -H 'Content-Type: application/json' \
  -d '{"external_system":"simulator","external_id":"no-such-device","metrics":[{"name":"read_soc","value":1}]}'
# 期望 404
```

---



### T12. Provider 过滤

Simulator 默认只加载 `Provider=simulator`。在 Resource 建一个 `Provider=other` 的 CU 后 reload，不应出现在 `/runtime`。

---



## 4. 推荐联调顺序（最短路径）

```text
1. compose 起基础设施
2. 起 resource / telemetry / gateway
3. ./scripts/seed_simulator_demo.sh
4. 起 simulator → 确认 /runtime 有 4 台
5. T4 确认遥测进 Telemetry
6. 起 dispatch → T6 全链路
7. T7/T8 故障注入
```

---



## 5. 常见问题


| 现象                                    | 可能原因                                       | 处理                                     |
| ------------------------------------- | ------------------------------------------ | -------------------------------------- |
| Simulator `loaded 0 devices`          | 未 seed / Provider 不匹配 / seed 在启动后未 reload  | seed 后 `POST /runtime/reload`          |
| 遥测 404                                | Gateway 无 mapping                          | 检查 mappings；ExternalID 是否一致            |
| Dispatch 立刻 Rejected                  | CUCode 不是 Resource UUID 或 mapping disabled | 用 seed 输出的 CU UUID                     |
| 命令走 ems_log 而非 simulator              | Gateway 未配 `simulator.addr` 或未重启           | 检查 `config/gateway.yaml` 并重启 Gateway   |
| CreateSite `display_name is required` | JSON 用了 camelCase `name`                   | Resource 用 PascalCase：`{"Name":"..."}` |
| Tick 无变化                              | 间隔未到 / 设备 offline                          | 等 5s+；检查 fault                         |


---

