# Decision / Attributes 架构设计笔记

记录 `Decision`、`Attributes`、`Evaluator`、`Fingerprint` 这几个类型为什么这么设计，
以及为新增业务类型预留了哪些扩展点、又刻意没做哪些事。代码见
`domain/model/`、`domain/service/`。

## 结论

把不同业务（dispatch / soe）收敛到一张 `alarms` 表、一个 `Decision` 是正确的选择，
不是 demo 阶段的权宜之计。所有业务共享完全相同的生命周期（open → acknowledged →
closed）、查询模式（按租户 + 状态 + 时间分页）、合单机制（fingerprint 部分唯一索引）、
精确一次语义（`alarm_event_dedup`）。这四件事一旦分表，就要在 N 张表上各自重复实现
一遍 ack/close 乐观锁和迁移，List 查询还要 UNION ALL 跨表分页——比 JSONB 多态属性更
复杂，收益却只是换来强类型字段。Prometheus Alertmanager（label map）、PagerDuty /
Opsgenie（`custom_details` JSON）、Grafana 统一告警都是同一张表 + 多态属性，这是
这类系统的常规架构。

真正会随业务增长变差的，是三处**实现细节**，不是收敛这件事本身：

1. `Attributes` 曾是一个所有业务字段拉平堆叠的 struct（字段跨业务混在一起，没有编译期
   约束）。
2. `Evaluator.Evaluate` 曾用 `switch(in.Source)` 硬编码分支。
3. fingerprint 版本前缀 `"v1:"` 曾是 dispatch 和 soe 共用的同一个 Go 常量。

三处均已重构，见下文。

## 已实施的改动

| 改动 | 文件 | 说明 |
|---|---|---|
| 放宽 `source` 约束 | `migrations/alarm/000001_init.up.sql` | `alarms_source_chk` 从枚举白名单改成 `source <> ''`，新增业务不再需要迁移，交给 `model.ParseSource` 兜底 |
| 按业务拆分 Attributes | `domain/model/attributes.go`（新增） | `AttributesPayload` 标记接口 + `DispatchAttributes` / `SOEAttributes`，指针接收者，字段不再跨业务堆叠 |
| Decision/Alarm 改用接口 | `decision.go`、`alarm.go` | `Attributes` 字段类型从具体 struct 改为 `AttributesPayload` |
| 按 RuleID 解码 | `adapter/outbound/postgres/row.go` | `attributePayloadFactories` 按 `rule_id` 查表选具体类型解码，新增业务只加一个 map 项 |
| Evaluator 改注册表 | `domain/service/evaluator.go` | 未导出的 `ruleHandler` 接口 + `map[model.Source]ruleHandler`；`NewEvaluator(rules Rules)` 签名不变 |
| 业务逻辑拆文件 | `dispatch_task_failed.go`、`soe_discrete_change.go`（新增） | 原来两段 `evalDispatch`/`evalSOE` 各自独立成文件 |
| fingerprint 常量拆分 | `fingerprint.go` | `fingerprintSchema` 拆成 `dispatchFingerprintSchema` / `soeFingerprintSchema` 两个独立常量，**值不变**（都还是 `"v1:"`） |

全部改动 `go build / go vet / go test ./...` 通过，未改变任何已有测试断言的观察行为。

## 新增第三种业务的成本（现状）

| 位置 | 是否要改 | 说明 |
|---|---|---|
| `enums.go`：新增 `Source` / `RuleID` 常量 | 要改 | 静态类型语言下的正常成本 |
| `rules.go`：新增一个具名 Rule 配置字段 | 要改 | 可选：字段形态差异大时具名字段比 `map[string]any` 更安全 |
| 新建一个 `xxxHandler` 文件实现 `ruleHandler` | 新增文件 | 不改 `evaluator.go` 本身 |
| `evaluator.go`：`NewEvaluator` 里加一行 map 项 | 改一行 | `Evaluate` 方法本身不用改 |
| `attributes.go`：新增一个 `AttributesPayload` 实现 | 新增类型 | 不改 `Decision`/`Alarm`/DB 列 |
| `row.go`：`attributePayloadFactories` 加一行 | 改一行 | |
| `fingerprint.go`：新增该业务自己的版本常量 + 函数 | 新增 | 不影响已有业务的常量 |
| 迁移 | 不需要 | `source` 约束已放宽 |
| Kafka consumer | 新增文件 | 接入新 topic 本来就是真实的新集成工作，不可消除 |
| `dto.go` / HTTP 响应 | 不需要改 | `Attributes` 按 JSON 通用序列化，新字段自动透传 |

## 两个不同的"版本"概念，不要混在一起

讨论中出现过"要不要取消版本机制"，实际上指的是两件完全不同的事：

**1. `AttributesPayload` 的 Go 类型命名版本（如 `DispatchAttributesV1`）—— 已取消**

这是为 JSON 形状变更预留的版本类型命名，项目还没上线、业务频繁变动，提前建
`V1/V2/V3` 只会堆出没人敢删的废弃类型。**结论：不做**。现在的类型就叫
`DispatchAttributes` / `SOEAttributes`，不带版本后缀。`attributes_schema`
这一列继续保留、继续写值（固定写 1），当"惰性戳记"用，但不建分支解码逻辑——
上线后第一次发生破坏性字段变更时，再把当时的旧类型补上 `V1` 后缀、新类型继续用
不带后缀的名字（一次性回填 + 切换，不长期共存多版本）。

**2. fingerprint 前缀 `"v1:"`（`dispatchFingerprintSchema` / `soeFingerprintSchema`）—— 从未打算取消，现在依然存在**

这不是同一件事：它是从最初设计起就写入数据库、参与哈希计算的**持久化命名空间前缀**，
不是 Go 结构体命名习惯。它标记的是"用哪种规则算 fingerprint"（例如以后要把 SOE
的聚合粒度从"单点"改成"整 CU"，就必须换成 `"soe:v2:"`，否则新旧两种算法算出的
fingerprint 会互相碰撞、把语义完全不同的告警错误合并）。这类底层聚合规则变更极其
罕见，和会随业务迭代频繁变化的 JSON 属性形状完全是两种变化频率，所以不适用
"提前建版本会堆积废弃代码"这条顾虑。

阶段 2 对它做的唯一改动是：把原来 dispatch 和 soe **共用同一个 Go 常量**
`fingerprintSchema = "v1:"`，拆成 `dispatchFingerprintSchema` /
`soeFingerprintSchema` 两个**独立**常量——**值完全没变**，都还是 `"v1:"`，不影响
任何已产生的 fingerprint，也不用改 `fingerprint_test.go`。目的只是解除两个独立
业务之间的耦合：以后只想把 SOE 的聚合算法升到 v2，不会因为常量共享而被迫牵连
dispatch 的版本号。

## 有意没做的事

- **没有导出 `RuleHandler` 类型，也没有把 `NewEvaluator` 改成 `NewEvaluator(handlers ...RuleHandler)` 的 variadic 构造函数。**
  这样做可以让调用方（如 `server.go`）从外部显式组装、注入自定义 handler，扩展性更强，
  但代价是要改 `application/app.go`、`config` 等多处签名。权衡后选择改动面更小的写法：
  `NewEvaluator(rules Rules)` 签名不变，内部把两个业务分支拆成未导出的 `ruleHandler`
  实现，注册进内部 map。现有调用方零改动，新增业务只需要在 `NewEvaluator` 内部加一行。
  如果未来真的需要跨包组装 handler（比如从另一个模块注入规则），再导出接口不迟。

- **没有引入完整的 `FingerprintStrategy` 接口**（形如
  `interface { Fingerprint(in) string; EventID(in) string }`，每个版本各自实现）。
  只做了拆常量这一步最小改动。当前每种业务的 fingerprint 计算只有 1-2 个调用点
  （对应的 `xxxHandler.evaluate` 方法），为此建一整套接口 + 注册表是没有实际消费者的
  抽象，属于过度设计；等真的出现第二个版本要与第一个版本并存（而不是直接替换）时，
  再引入接口更有依据。

- **没有做"新增第三种业务"的演练**（比如假想一个"拓扑越限"业务）。
  已有的重构 + 测试通过已经足以证明改动面变小，没必要为验证而新增一条不会真正上线
  的业务线。

## 参考

- 整体架构、职责边界、可观测性约定见 [`README.md`](./README.md)
- SQL CTE 与 ingest 语义细节见 [`plan_v1.md`](./plan_v1.md)
