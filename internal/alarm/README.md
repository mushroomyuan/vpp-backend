# vpp-alarm

VPP 平台的**告警中心**。消费已有 Kafka topic，按规则开单 / 合单，提供租户内查询、确认、关闭。告警是充血聚合，**不是事件副本**。

- **入站 Kafka**：`vpp.dispatch.events`（仅 `task.failed`）+ `vpp.soe.events`（全部离散量变位）
- **入站 HTTP**：管理端 List / Get / Ack / Close（纯 Gin，无 proto / gRPC）
- **出站**：Postgres `alarm` 库；通知口先打日志（`Notifier`），不接邮件 / 短信

本服务**不负责**：全量事件归档、消费 `vpp.command.events` / `vpp.resource.events`、规则 DSL、告警抑制风暴、SOE 自动恢复、APISIX 北向。不改 dispatch / telemetry 生产者。

SQL CTE 细节见 [plan_v1.md](./plan_v1.md)；`Decision`/`Attributes`/`Evaluator`/
fingerprint 版本的设计取舍见 [DECISION_DESIGN.md](./DECISION_DESIGN.md)。

---

## 目录

- [服务职责](#服务职责)
- [架构设计](#架构设计)
- [Fingerprint 与去重](#fingerprint-与去重)
- [规则](#规则)
- [HTTP 与鉴权](#http-与鉴权)
- [可观测性](#可观测性)
- [数据存储](#数据存储)
- [目录结构](#目录结构)
- [依赖组件](#依赖组件)
- [启动方式](#启动方式)
- [关键设计约定](#关键设计约定)
- [已知技术债](#已知技术债v1-故意不做)

---

## 服务职责

| 职责 | 说明 |
|---|---|
| **任务失败开单** | 消费 `task.failed` Envelope，默认 `severity=critical`，一次失败一张单 |
| **SOE 合单** | 消费全部离散量变位；同一测点在 open 期间合并，`count` 累加 |
| **精确一次** | `alarm_event_dedup` PK `(tenant_id, event_id)`；Kafka at-least-once 重投不重复写 |
| **人管面** | 租户内 List / Get / Ack / Close；ack/close 乐观锁，冲突 409 |
| **通知口** | `Notifier` port，v1 只打日志 |

**不负责**

- Phase C 审计仓库；按 CommandID / TaskID 查生命周期不在这里
- `task.started` / `task.completed`（噪声）
- 消费时调 `GetTask` 拼失败原因（热路径不去耦合 dispatch）
- 多副本消费者（kind `replicas: 1`）

---

## 架构设计

服务采用**六边形架构 + CQRS**，对外只暴露 **HTTP**；ingest 由两个 Kafka consumer 驱动。

```
telemetry / dispatch（不改生产者）
      │ Kafka  vpp.soe.events / vpp.dispatch.events
      ▼
┌─────────────────────────────────────────────────────────────┐
│                     Inbound Adapters                         │
│   adapter/inbound/kafka/         adapter/inbound/http/       │
│   · DispatchConsumer             · List / Get / Ack / Close  │
│   · SOEConsumer                  · Path C PEP + catalog      │
└────────────────────────────┬────────────────────────────────┘
                             │ Command / Query
┌────────────────────────────▼────────────────────────────────┐
│                      Application Layer                       │
│  Commands                          Queries                   │
│  · IngestEvent                     · ListAlarms              │
│  · Acknowledge                     · GetAlarm                │
│  · Close                                                     │
└──────┬──────────────────────────────┬───────────────────────┘
       │ domain ports                 │
┌──────▼──────────────────────────────▼───────────────────────┐
│                       Domain Layer                           │
│  model: Alarm · IncomingEvent · Fingerprint                  │
│  service: RuleEvaluator                                      │
│  port: AlarmRepository · Notifier · Observer                 │
└──────┬──────────────────────────────┬───────────────────────┘
       │ implements                   │
┌──────▼──────────────┐    ┌──────────▼───────────────────────┐
│ adapter/outbound/   │    │ adapter/outbound/notify/          │
│ postgres/           │    │ log.go                            │
│ (dedup + upsert)    │    │                                   │
└──────────┬──────────┘    └──────────────────────────────────┘
           ▼
      Postgres 库 alarm
```

`server.go` 用 errgroup 拉起：HTTP、两个 consumer、authz Syncer、metrics。健康检查只用 `GET /healthz`。Kafka brokers 为空时 consumer no-op，进程仍能起来。

---

## Fingerprint 与去重

这是两件事，不要混用。

| 机制 | 约束 | 负责 |
|---|---|---|
| Fingerprint + 部分唯一索引 | `alarms_open_fingerprint_uidx`：`(tenant_id, fingerprint) WHERE status <> 'closed'` | 和哪条 **open** 告警聚合 |
| `alarm_event_dedup` | PK `(tenant_id, event_id)`，约束名 `alarm_event_dedup_pkey` | Kafka 精确一次 |

`LastEventID` 是展示字段，最近一次真正更新了本行的 event_id，**不是**唯一键。

**Dispatch：** fingerprint 含 `event_id`，一次 `task.failed` 一张单。部分唯一索引不会把两次失败合成一条。去重完全交给 dedup 表。

**SOE：** fingerprint **不含** 单次变位的时间 / 新旧值。同一断路器连跳：dedup 未命中则 `count+1`；dedup 命中则整笔成功返回、不 bump count。关闭后再变位：部分唯一索引不再命中已 closed 行，INSERT 新开一条。

### Fingerprint 是 v1 稳定契约

不要用 `|` 裸拼接（`cu_code` / `metric_name` / `task_id` 可能含分隔符）。

```
fingerprint = "v1:" + hex(sha256(canonical UTF-8))
```

字段之间用 `\x1f`（unit separator），顺序固定：

- dispatch：`dispatch` + tenant_id + task_id + event_id
- soe：`soe` + tenant_id + cu_code + metric_name

前缀 `v1:` 标 schema。**聚合粒度一旦落库即持久化契约**：以后若改成「整 CU 一条告警」，必须新 fingerprint 版本 + 迁移，不能默默改哈希输入。

### Event ID

哈希输入与 fingerprint **同一套规范**：`\x1f` 分隔，禁止裸拼接。否则 `cu="AB", metric="C"` 与 `cu="A", metric="BC"` 会得到同一个 id，去重表会把第二次真实变位吞掉。

- dispatch：用 Envelope 已有 `event_id`
- SOE：`soe:v1:` + hex(sha256(canonical))

```
tenant_id \x1f cu_code \x1f metric_name \x1f RFC3339Nano(occurred_at) \x1f FormatFloat(old) \x1f FormatFloat(new)
```

`FormatFloat` = `strconv.FormatFloat(x, 'g', 17, 64)`。同一次变位重投 → 同一 id；时间或值不同 → 不同 id。

### 写入语义

一条 SQL，**dedup INSERT 必须在 alarms upsert 之前**。隔离级别用 Postgres 默认 **READ COMMITTED**（不是 SERIALIZABLE）。23505 按 **ConstraintName** 分流：

| 约束名 | ingest `result` / `reason` | 含义 |
|---|---|---|
| `alarm_event_dedup_pkey` | `dedup_hit` / `none` | 精确一次命中，整笔成功 |
| `alarms_open_fingerprint_uidx` | `poison` / `fingerprint_collision` | 数据完整性问题，**不要**和 decode poison 绑同一条告警规则 |
| 其它 unique | `poison` / `unique` | 未预期的唯一冲突 |

ingest 侧不要先 load 再 `Touch()` 写回。乱序：较新的 SOE 先写、较旧的后到（不同 event_id）→ `count+1`，但 `last_*` 不回退。

---

## 规则

静态 YAML，无 DSL。未命中或禁用 → **dropped**（commit offset，**不写** dedup）。

| 规则 id | 默认 | 说明 |
|---|---|---|
| `dispatch-task-failed` | enabled，`critical` | 仅 `task.failed` |
| `soe-discrete-change` | enabled，`warning` | `metric-names` 空 = 全部 SOE |

配置见 [`config/alarm.yaml`](../../config/alarm.yaml)。

---

## HTTP 与鉴权

基路径 `/api/v1/tenants/:tenant_id/alarms`。**没有** `POST /alarms`（只从 Kafka ingest）。

| 方法 | 路径 | 权限 |
|---|---|---|
| `GET` | `/` | `read` |
| `GET` | `/:id` | `read` |
| `POST` | `/:id/ack` | `ack` |
| `POST` | `/:id/close` | `close` |

Catalog：`alarm:alerts` + `read` / `ack` / `close`。占位角色：viewer=`read`，operator=`read+ack+close`，admin=全部。Actor 来自 PEP，不从 body 取。路径 `tenant_id` 必须等于 `X-Userinfo` 的 owner。

ack / close 带期望 `version`，冲突 → **409** 并返回当前 `version`。

`auth.trust-proxy-headers` 默认 **false**（本机直连 `:8087` 调试）。APISIX `/alarm/*` **v1 不接**；代码侧 Path C PEP 已写好。kind 联调：

```bash
kubectl -n vpp port-forward svc/alarm 8087:8087
```

---

## 可观测性

ingest **只有这一条** Counter，禁止再拆 `alarm_ingest_poison_total` / `dropped_total` / `retry_total`：

```
alarm_ingest_total{source,result,reason}
```

- `source`：`dispatch` \| `soe`
- `result`：`ok` \| `dedup_hit` \| `dropped` \| `poison` \| `retry`
- `reason`：`none`（ok / dedup_hit）\| `rule` \| `decode` \| `db` \| `unique` \| `fingerprint_collision` \| `transient`

另有 `alarm_ingest_duration_seconds{source}`、`alarm_ack_conflict_total`、`alarm_close_conflict_total`、`alarm_consumer_messages_total`、`alarm_consumer_handler_errors_total`。

**Open gauge（进程内）：** `alarm_open_alarms{source}`。新开 +1，close −1，SOE 合单不变。启动时 `CalibrateOpenAlarms` 扫一次 DB 校准；**校准查询不是每 scrape 都跑**，不要周期性 `SELECT COUNT(*) FROM alarms WHERE status <> 'closed'`。

**Lag：** kafka-go `ReadLag()` 不能和 consumer group 一起用。v1 用 `Reader.Stats().Lag`（最近一次 fetch 的 high-watermark 差），不是 group committed lag。真 group lag 是下一刀。

毒消息看 `result="poison"`；数据完整性单独看 `reason="fingerprint_collision"`，不要和 `reason="decode"` 绑同一条阈值。

---

## 数据存储

Postgres 独立库 `alarm`（`migrations/alarm/000001_init.up.sql`）。不出站查其它业务库，不用 Redis。

| 表 | 说明 |
|---|---|
| `alarms` | 告警聚合；部分唯一索引 `alarms_open_fingerprint_uidx` |
| `alarm_event_dedup` | 只追加；PK 即 `alarm_event_dedup_pkey`；已有 `ingested_at` 索引给以后 retention |

---

## 目录结构

```
internal/alarm/
  cmd/main.go
  app.go  run.go  server.go
  application/{command,query}/
  domain/{model,port,service}/
  adapter/inbound/{http,kafka}/
  adapter/outbound/{postgres,notify}/
  metrics/
  config/  options/
```

---

## 依赖组件

| 组件 | 用途 |
|---|---|
| Postgres `alarm` 库 | 告警 + 去重表 |
| Kafka | `vpp.dispatch.events` / `vpp.soe.events` |
| Casdoor（可选） | `trust-proxy-headers=true` 时 Path C PEP |
| Prometheus `:9107` | 指标 |

---

## 启动方式

需要 Postgres 已建 `alarm` 库（compose `migrations/initdb/70-alarm-db.sh` 会建库并跑 migration）。

```bash
make run-alarm
# 或
cd internal/alarm && go run ./cmd/main.go -c ../../config/alarm.yaml
```

端口：HTTP `:8087`，metrics `:9107`。kind 为 ClusterIP，无 extraPortMappings。

---

## 关键设计约定

1. **Fingerprint 管合单，dedup 表管精确一次。** `LastEventID` 不是唯一键。
2. **哈希契约不可默默改。** 分隔符是 `\x1f`，不是 `|`；改粒度必须新版本 + 迁移。
3. **一条 SQL，dedup 先行。** READ COMMITTED；23505 按约束名分流。
4. **无 POST /alarms。** Actor 来自 PEP。
5. **ingest 只用 `alarm_ingest_total`。** open gauge 进程内计数，启动校准一次。
6. **单副本。** 水平扩展靠分区 + 去重表，不要靠再加 replicas。

---

## 已知技术债（v1 故意不做）

- **closed 行与 `alarm_event_dedup` 无限堆积。** 正确性不受影响。retention 按 `ingested_at` 删即可，v1 不跑归档 job。
- SOE 无 Envelope；合成 event_id 依赖 `occurred_at` 精度。
- 单副本消费者；表已经为分区扩展预留。
- 无 closed 自动恢复、无 webhook、无 APISIX `/alarm/*`。
- `alarm_consumer_lag` 是 Stats 水位，不是 group committed lag。
