# vpp-optimization

VPP 平台的**内部闭环决策服务**。按固定周期读 Telemetry 当前状态，用阈值规则决定要不要动、动多少，经 Dispatch 下发自动任务。

- **无入站业务 API**：没有人/服务主动调用 Optimization；只保留 `GET /healthz` 和 `/metrics`
- **出站 gRPC（内部直连，不经 APISIX）**：Telemetry `GetSnapshot`、Resource `GetAsset`（仅 Aggregate 分摊）、Dispatch `SubmitTask`（`TriggerType=automatic`）
- **Forecast**：只留 `ForecastPort` + stub，v1 规则不调用

本服务**不负责**：真实预测算法、需求响应/市场分摊（`AggregateTarget` 的产线调用方）、多级任务分解、Resource Runtime 缓存、人工任务提交。

能力简介见 [OVERVIEW.md](./OVERVIEW.md)。设计讨论见 [`discussion/2026-09-04.md`](../../discussion/2026-09-04.md)。

---

## 目录

- [服务职责](#服务职责)
- [架构设计](#架构设计)
- [决策周期与冷却期](#决策周期与冷却期)
- [规则](#规则)
- [Target / Allocate](#target--allocate)
- [可观测性](#可观测性)
- [目录结构](#目录结构)
- [依赖组件](#依赖组件)
- [启动方式](#启动方式)
- [关键设计约定](#关键设计约定)
- [已知技术债](#已知技术债v1-故意不做)

---

## 服务职责

| 职责 | 说明 |
|---|---|
| **决策循环** | `DecisionLoop` ticker 驱动，每租户一轮 `RunDecisionCycle` |
| **阈值评估** | v1 仅 `SOCThresholdRule`：显式绑定 CU + 读/写 PointKey |
| **防抖** | 进程内 `(CUCode, RuleID)` 冷却期；过期/stale 快照不决策 |
| **下发** | 一条 Target → 一个 Dispatch Task（parallel Action） |
| **来源标记** | `TriggerType=automatic`，经 `vpp.dispatch.events.trigger_type` 进 alarm 工单属性 |

**不负责**

- 入站 gRPC / 业务 HTTP
- 查 Telemetry 历史、依赖预测
- 按 Resource Runtime 的 `MaxDischargePowerKW` 分摊（该字段从未被写入）
- 多副本 / Redis 冷却态（单实例，重启丢冷却可接受）

---

## 架构设计

服务采用**六边形架构**。与 alarm / dispatch 的差别：没有 inbound 业务适配器，主路径是独立 goroutine（对照 dispatch 的 `TimeoutScanner`）。

```
Telemetry :5003          Resource :5002           Dispatch :5006
 GetSnapshot              GetAsset (容量)          SubmitTask automatic
      ▲                        ▲                        ▲
      │ gRPC 只读              │ gRPC 只读              │ gRPC 写
┌─────┴────────────────────────┴────────────────────────┴─────┐
│                     Outbound Adapters                        │
│   telemetry_grpc / resource_grpc / dispatch_grpc /           │
│   forecast_stub（恒 not_implemented）                         │
└────────────────────────────┬────────────────────────────────┘
                             │ ports
┌────────────────────────────▼────────────────────────────────┐
│                      Application Layer                       │
│  Commands                                                    │
│  · RunDecisionCycle                                          │
│  · DecisionLoop (goroutine + ticker)                         │
└──────┬──────────────────────────────────────────────────────┘
       │
┌──────▼──────────────────────────────────────────────────────┐
│                       Domain Layer                           │
│  model: Target · PointTarget · AggregateTarget · Rule        │
│  service: Evaluator · Allocate / splitByCapacity             │
│  port: TelemetryPort · ResourcePort · ForecastPort · Observer│
│  application/port: DispatchPort                              │
└─────────────────────────────────────────────────────────────┘
```

`server.go` 用 errgroup 拉起：decision loop、HTTP `/healthz`、metrics。无 Kafka、无 Postgres、无 authz。

---

## 决策周期与冷却期

这是两件事，都是为了避免"同一份临界数据反复决策"造成震荡。

| 机制 | 默认 | 负责 |
|---|---|---|
| Decision interval | `60s`（YAML 可调） | 必须**严格大于** Telemetry 采集周期（Simulator 默认 30s） |
| Cooldown | `2× interval`（`2m`） | 同一 `(CUCode, RuleID)` 触发后，窗口内即使仍越限也不再下发 |

冷却态是进程内 map，不是 Redis。单实例部署；重启后最坏情况是多一次决策，不是正确性 bug。

规则自己的 `cooldown: 0s` 表示"用默认值"，**不是**"关闭冷却"。stale snapshot 静默跳过，不算错误。

---

## 规则

静态 YAML，无 DSL。`DefaultRules()` 为空：具体 CU / 阈值是部署相关的，不在代码里写死假数据。

| 规则 id | 说明 |
|---|---|
| `soc_threshold` | SOC ≤ `min-soc` → 下发充电功率；SOC ≥ `max-soc` → 下发放电功率 |

每条实例必须显式绑定 `cu-code` + `read-point-key` + `write-point-key`。PointKey 是自由字符串，v1 不做自动发现。

配置见 [`config/optimization.yaml`](../../config/optimization.yaml)。`tenant-ids` 与 `soc-thresholds` 默认空，填上才会真正决策。

依赖预测的规则类型预留了接口位置，`DefaultRules()` / YAML **不启用**。

---

## Target / Allocate

```
Evaluate → []Target → Allocate → []CommandSpec → SubmitTask
```

- **PointTarget**：规则已算好点位，Allocate 1:1。
- **AggregateTarget**：按 Resource 静态 `RatedCapacityKW` 比例拆 `DeltaValue`。v1 无产线调用方（等 Market）；未知/零容量的 CU 不发空命令。

---

## 可观测性

挂在进程 `/metrics`（`:9108`），与 `platform/metrics` 的 `app_requests_*` 共用 endpoint。

| 指标 | 含义 |
|---|---|
| `optimization_decision_cycle_duration_seconds` | 每租户一轮耗时 |
| `optimization_decision_cycle_total{result}` | `ok` \| `error`（部分失败也记 error） |
| `optimization_rules_fired_total{rule_id}` | 规则产出的 Target 数 |
| `optimization_submit_task_total{result}` | `success` \| `failure` |
| `optimization_forecast_calls_total{result}` | `ok` \| `error` \| `not_implemented` |

结构化日志 `component: "DecisionLoop"`，字段包括 `rule_id`、`cu_code`、`cooldown_skipped`、`tenant_id`、`targets_fired`、`tasks_submitted`。

---

## 目录结构

```
internal/optimization/
  cmd/main.go
  app.go  run.go  server.go
  application/{command,port}/
  domain/{model,port,service}/
  adapter/outbound/{telemetry_grpc,resource_grpc,dispatch_grpc,forecast_stub}/
  metrics/
  config/  options/
```

---

## 依赖组件

| 组件 | 用途 |
|---|---|
| telemetry `:5003` | `GetSnapshot`（熔断默认开） |
| resource `:5002` | `GetAsset` 额定容量（熔断默认开；v1 规则不用） |
| dispatch `:5006` | `SubmitTask`（熔断默认开；服务端 `submit-task` 限流 `rps=20 burst=40`） |
| Prometheus `:9108` | 指标 |
| Forecast | **无**；stub 占位 |

无 Postgres / Kafka / Redis / Casdoor。

---

## 启动方式

上游 telemetry / resource / dispatch 需要已起来（gRPC dial 是懒连接，进程能先起；tick 时才会报错）。`make run-all` 会一并拉起。

```bash
make run-optimization
# 或
cd internal/optimization && go run ./cmd/main.go -c ../../config/optimization.yaml
```

端口：HTTP `:8088`（仅 `/healthz`），metrics `:9108`。

kind：ClusterIP，无 extraPortMappings。冷却态在进程内，**不要** `scale --replicas=2`。

```bash
make k8s-apply
kubectl -n vpp port-forward svc/optimization 8088:8088
```

真正决策前，在 YAML 里填 `tenant-ids` 和至少一条 `rules.soc-thresholds`。

---

## 关键设计约定

1. **周期 > 采集周期。** `decision-interval` 校验必须大于 30s。
2. **冷却 0 不是关闭。** 规则 `cooldown: 0` = 用 `2× interval`。
3. **绕开 Runtime 缓存。** 决策读 Telemetry，容量读 Resource 静态配置。
4. **内部直连。** 不经 APISIX；人/算法区分靠 Dispatch 已有的 `TriggerType`，不新增 proto 字段。
5. **fingerprint 不含 TriggerType。** alarm 只把 `trigger_type` 写进属性做展示。
6. **一条 Target 一个 Task。** 多 CU 无顺序要求 → `ExecutionPolicy=parallel`。

---

## 已知技术债（v1 故意不做）

- **真实 Forecast。** 只有 Port + stub；预测型规则不启用。
- **`AggregateTarget` 无产线调用方。** 等 Market / 需求响应。
- **多级任务分解。** `Scope` 单层；代理用户链推迟。
- **冷却态不持久化。** 重启可能多一次下发；kind `replicas: 1`。
- **无滞回 / 状态机。** 冷却是防抖的一半；边界震荡的另一半留给以后。
