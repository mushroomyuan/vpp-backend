# vpp-forecast

VPP 平台的**内部批算式预测服务**。按固定周期拉 Telemetry 历史，用朴素算法算出资产出力/负荷的未来点，写入 Redis 热点缓存和 Postgres 权威历史，再通过只读 gRPC 回答"现在往后看，下一步是多少"和"这段窗口里预测过什么"。

- **只读 gRPC**：`GetLatestPrediction` / `QueryForecastHistory`；没有写 RPC
- **出站**：Telemetry `QueryAggregation`（内部直连，不经 APISIX）、Redis db=2、Postgres `forecast` 库
- **不做**：电价/天气等外部数据预测；不查 Resource；本轮不接 Decision（那边的 `ForecastProvider` stub 保持不动）

能力简介见 [OVERVIEW.md](./OVERVIEW.md)。设计讨论见 [`discussion/2026-09-04.md`](../../discussion/2026-09-04.md)。

---

## 目录

- [服务职责](#服务职责)
- [架构设计](#架构设计)
- [批算循环](#批算循环)
- [GetLatestPrediction 与选点](#getlatestprediction-与选点)
- [算法](#算法)
- [存储](#存储)
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
| **批算循环** | `ForecastLoop` ticker 驱动，每轮对每个 enabled `ForecastTarget` 跑一次 `RunForecastCycle` |
| **朴素预测** | v1：`moving_average`（最近 N 个 bucket 均值填满 horizon）/ `same_period_prior`（过去 N 天同一时刻均值） |
| **权威历史** | 每批 `horizon-steps` 个点写入 `forecast_history`（先 Postgres） |
| **热点缓存** | Postgres 成功后再把整批点写入 Redis；TTL 只负责"删掉的 target 别永远占着" |
| **只读查询** | `GetLatestPrediction`（下一步还没到的点）/ `QueryForecastHistory`（按 `target_timestamp` 窗口） |

**不负责**

- 写 RPC / 外部推送预测
- 电价、天气、政策类预测
- 查 Resource、自动发现"该预测哪些点"
- APISIX 北向 / grpc-gateway HTTP
- 多副本协调（单实例）

---

## 架构设计

六边形分层，和 decision 同一套手写骨架。主路径是独立 goroutine（对照 `DecisionLoop` / dispatch `TimeoutScanner`），另有 inbound gRPC 给人/服务以后来读。

```
Telemetry :5003                 Redis db=2                 Postgres forecast
 QueryAggregation               最新整批点                  forecast_history
      ▲                              ▲                           ▲
      │ gRPC 只读                    │ 整批 JSON                  │ pgx.Batch
┌─────┴──────────────────────────────┴───────────────────────────┴─────┐
│                     Outbound Adapters                                 │
│   telemetry_grpc / redis_cache / postgres_history                     │
└────────────────────────────┬─────────────────────────────────────────┘
                             │ ports
┌────────────────────────────▼─────────────────────────────────────────┐
│                      Application Layer                                │
│  Commands: RunForecastCycle · ForecastLoop                            │
│  Queries:  GetLatestPrediction · QueryForecastHistory                 │
└──────┬───────────────────────────────────────────────────────────────┘
       │
┌──────▼───────────────────────────────────────────────────────────────┐
│                       Domain Layer                                    │
│  model: Prediction · ForecastTarget · AlgorithmID                     │
│  service: Predictor · Registry · SelectNextPoint                      │
│  port: TelemetryPort · CachePort · HistoryPort · Observer             │
└──────────────────────────────────────────────────────────────────────┘
                              ▲
                              │ gRPC 只读
                    ForecastService :5007
                    HTTP :8089 /healthz only
```

`server.go` 用 errgroup 拉起：gRPC、ForecastLoop、HTTP `/healthz`、metrics。无 Kafka、无 authz。

---

## 批算循环

```
ticker（cycle-interval，默认 15m）
  → 每个 enabled ForecastTarget：
      1. 对齐到 step-seconds 时钟桶、严格晚于 now 的 horizon-steps 个未来点
      2. QueryAggregation(now-history-window, now)
      3. Registry.Get(algorithm).Predict
      4. HistoryPort.SaveBatch（失败：记日志，跳过 Redis，处理下一个 target）
      5. CachePort.SetLatestBatch（失败：只记日志；Postgres 已是权威源）
```

单个 target 失败不中断本轮其它 target。`cycle-interval` **不是**正确性约束（没有副作用指令），默认 15m 对齐 Telemetry 15 分钟聚合视图。

`targets: []` 时 loop 空转，进程仍能起来。

---

## GetLatestPrediction 与选点

回答的是"现在往后看，下一个还没到的时间点"，不是"上次批算写进去的第一个点"。

1. Redis `GetLatestBatch` → `SelectNextPoint(batch, now)`
2. 挑不出点（miss，或整批都已落在过去）→ Postgres `GetLatestBatch`（`MAX(generated_at)` 那一批）再跑一次同一函数
3. 两次都挑不出 → `NOT_FOUND`

Redis 和 Postgres 共用 `SelectNextPoint`，同一个 `now` 问两次不会因为走了哪条路径而答案不同。缓存的是整批 horizon，不是"写入时刻的下一步"（那个点会过时）。

`QueryForecastHistory`：`GeneratedAt` 空 = 每个 `target_timestamp` 只返回最近一次生成的点；填值 = 只看那一次批算。

---

## 算法

`Predictor` 是 v1 唯一需要长期稳定的接口。Phase D 用真实模型替换 naive 算法时，不改调用方和落库形状。

| AlgorithmID | 行为 |
|---|---|
| `moving_average` | 最近 `MovingAverageWindow` 个 bucket 的 Avg，同一均值填满所有 horizon 点 |
| `same_period_prior` | 每个未来点取过去 `SamePeriodLookbackDays` 天同一时刻 Last 的算术均值（不排除事件日） |

`AlgorithmVersion()` 直接写入 `forecast_history.algorithm_version`。

`ForecastTarget` 是扁平 struct：`TenantID` 挂在 target 上（不是服务级 `tenant-ids` 列表），每条显式绑定 CU / MetricName。

---

## 存储

| 层 | 职责 | 细节 |
|---|---|---|
| **Postgres** | 权威、不覆盖 | 独立逻辑库 `forecast`；`UNIQUE (tenant, cu, metric, generated_at, target_timestamp)`；`ON CONFLICT DO NOTHING` |
| **Redis db=2** | 可重算热点 | key `forecast:latest:{tenant}:{cu}:{metric}` → JSON 整批点；TTL = `horizon-steps × step-seconds + cycle-interval`（默认 75m） |

v1 **不做** retention/清理。TimescaleDB 超表不需要（双时间轴、低频写入、要带 `algorithm_version`）。

---

## 可观测性

挂在进程 `/metrics`（`:9109`），与 `platform/metrics` 的 `app_requests_*` 共用 endpoint。

| 指标 | 含义 |
|---|---|
| `forecast_cycle_duration_seconds` | 一轮批算墙钟时间 |
| `forecast_cycle_total{result}` | `ok` \| `error`（任一 target 失败也记 error） |
| `forecast_targets_total{result}` | 成功/失败 target 数（Redis 写失败仍算 success） |
| `forecast_predict_duration_seconds` / `forecast_predict_total` | 按 `algorithm_version` |
| `forecast_telemetry_query_total{result}` | QueryAggregation |
| `forecast_postgres_write_total{result}` | SaveBatch |
| `forecast_redis_write_total{result}` | SetLatestBatch（仅 Postgres 成功后才计） |

结构化日志 `component: "ForecastLoop"`，字段含 `cu_code`、`metric_name`、`algorithm`、`generated_at`。

---

## 目录结构

```
internal/forecast/
  cmd/main.go
  app.go  run.go  server.go
  application/{command,query}/
  domain/{model,port,service}/
  adapter/inbound/grpc/
  adapter/outbound/{telemetry_grpc,redis_cache,postgres_history}/
  metrics/
  config/  options/
```

---

## 依赖组件

| 组件 | 用途 |
|---|---|
| telemetry `:5003` | `QueryAggregation` AVG + LAST（熔断默认开） |
| Postgres `forecast` 库 | `forecast_history` |
| Redis db=2 | 最新一批点 |
| Prometheus `:9109` | 指标 |

不查 Resource、不调 Dispatch、不接 Casdoor。Decision 的 `ForecastProvider` **本轮不接**。

---

## 启动方式

需要 Postgres（`forecast` 库，`migrations/initdb/80-forecast-db.sh`）、Redis、以及 tick 时可达的 telemetry。`make run-all` 会一并拉起。

```bash
make run-forecast
# 或
cd internal/forecast && go run ./cmd/main.go -c ../../config/forecast.yaml
```

端口：gRPC `:5007`，HTTP `:8089`（仅 `/healthz`），metrics `:9109`。

kind：ClusterIP，无 extraPortMappings。批算循环无跨副本协调，**不要** `scale --replicas=2`。

```bash
make k8s-apply
kubectl -n vpp port-forward svc/forecast 5007:5007
```

真正批算前，在 YAML 里填至少一条 `forecast.targets`。

---

## 关键设计约定

1. **先 Postgres 后 Redis。** 权威源落库失败就不更新缓存。
2. **缓存整批点，读取时现算下一步。** `SelectNextPoint` 是 Redis 命中和 Postgres 回退的同一段逻辑。
3. **TTL 不判断新鲜度。** 新鲜度看 `target_timestamp` 相对请求时刻。
4. **horizon 对齐时钟桶。** `step-seconds=900` 时对齐 :00/:15/:30/:45，才能对上 Telemetry 15 分钟视图和同比查找。
5. **TenantID 在 target 上。** 避免服务级租户列表 × 已绑定 CU 的无意义交叉调用。
6. **内部直连 Telemetry。** 不经 APISIX；v1 不挂北向路由。

---

## 已知技术债（v1 故意不做）

- **不接 Decision。** `ForecastProvider` 仍是 stub；接线是独立后续工作。
- **不做电价/天气/政策预测。** 讨论定性为另一类服务。
- **`same_period_prior` 不排除事件日。** 没有 Market 日历；`BaselinePredictor` 以后再特化。
- **不做目标自动发现。** 显式 `targets` 列表。
- **不做写 RPC / APISIX / grpc-gateway。** 无真实调用方前不打通北向鉴权。
- **不做 `forecast_history` retention。** 数据量远小于原始遥测。
- **固定 `replicas: 1`。**
