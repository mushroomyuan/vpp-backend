我已经通读了这份计划，并对照了当前代码库（`internal/optimization`、`resource`、`telemetry`、`gateway`、`dispatch`、`alarm` 以及相关 proto）逐条验证了计划里的技术假设。下面是我的审查结论。

---

## 总体判断

这份计划**方向是对的、问题诊断也基本准确**，作为学习项目的一次"把边界重新划清"的重构，它抓住了真正的痛点（PointKey 无契约、业务语义与设备点名混用、策略即配置、单 CU 绑定、Resource/Decision 边界不清）。目标边界的划分（Resource=资源与能力、Telemetry=运行值权威、Decision=Policy→Objective→Plan→Executor、Dispatch=可靠执行、Gateway=外部翻译）是行业内成熟且正确的分法。核心抽象——**能力组合（capability composition）取代设备类型字段/表**、**版本化 metric/capability ID 作为代码契约而非 DB 数据**、**Objective 作为多来源统一语言**——这几个都是经得起未来推敲的决策。

但它有几个**实质性的疏漏和一个结构性风险**，我按重要性排序说明。文中引用的代码我都实际核对过。

---

## 一、结构性风险：所谓"旁路 + optimization 作为行为对照"是自相矛盾的

计划的核心策略是：新建 `decision`，让旧 `optimization` 在开发期"继续作为行为对照，避免长期处于不可编译的半迁移状态"。

但这与它自己的步骤 1–3 冲突。步骤 3 要把 **Telemetry / Dispatch / Gateway 的 PointKey 链路统一改成 MetricID**（破坏性，无兼容层）。而现有 `optimization` 恰恰依赖这三条链路：

- `evaluator` 通过 telemetry client 读 `Snapshot.Metrics[metricName]`；
- `run_decision_cycle.go` 通过 dispatch 下发 `CommandSpec{PointKey, Value}`。

一旦步骤 1–3 落地（telemetry snapshot key、dispatch/gateway 语义改为 metric_id、删除 `point_key`），旧 `optimization` 就**无法再作为可运行的对照**了，它要么编译失败要么行为错误。也就是说"旁路期间旧服务仍可运行对照"这个前提，在跨服务契约破坏性变更之后是不成立的。

这不是致命问题，但计划应当明确承认：**真正能并存的窗口只到步骤 2 结束**；步骤 3 一旦动跨服务契约，就进入"只有 decision 一条路可跑通"的阶段。之前那份讨论（[decision 分阶段方案](2026-09-23-03)）反而更稳——它主张先保留 `DecisionLoop→Evaluate→Allocate→Dispatch`，只把"固定 CU"换成 Scope 展开，分四阶段推进。这份 plan 放弃了那条渐进路径，把 8 个大工作流压成一次大爆炸式切换。对学习项目可以接受，但**建议把"建议提交顺序"里的每一步都约束为"独立可编译、可测试、可单独合并"**，否则中间态无法验收。

---

## 二、明确遗漏的下游消费者：Alarm 服务

计划从头到尾**没有提到 `internal/alarm`**，但它是 metric 语义变更的直接受害者：

- Telemetry 的 `Snapshot.Apply` 在离散量变位时产生 `SOEEvent`，其中带 `m.Name`（metric 名）；
- `internal/alarm/adapter/inbound/kafka/soe_consumer.go` 消费 `vpp.soe.events`，且 alarm 有基于 metric 的规则（`domain/service/rules.go`、`evaluator.go`）。

一旦把 telemetry 的 `metric_name` 语义改成 `metric_id`，**`vpp.soe.events` 的契约和 alarm 的规则匹配都会受影响**。这属于计划"破坏性调整"清单里应当列出却漏掉的一项。至少要在计划里补一句 alarm 的 SOE 消费与规则如何随 metric_id 迁移。

---

## 三、被低估的工作量：Gateway 新增了一整层运行时职责

计划把"外部地址 ↔ canonical 值（`raw*scale+offset`）双向转换"放进 Gateway，这是**方向正确但被严重低估**的一块。核对现状：

- 现在 Gateway **只做 CU 级映射**（`external_system + external_id → CUCode`，见 `execute_command.go` / `receive_telemetry.go`），metric 名是**直接透传**的，没有任何点级转换；
- `ReceiveTelemetry` 的入站模型只有 `Name + Value`，**并不携带 per-point external_address**。

这意味着计划隐含了两个未写明的前提：

1. **入站遥测契约要改**：外部设备/simulator 必须按 external_address 上报，Gateway 才能 `(external_address → metric_id)` 反查绑定。这是一个 gateway ingest 边界的数据模型变更，计划只字未提。
2. **Gateway 要新增对 Resource point binding 的缓存与失效**，即引入 `gateway → resource` 的新运行时耦合。计划提了 TTL + 事件失效 + revision 校验（很好），但没讨论一个关键的安全语义：**scale/offset 错误是静默的**——一个过期的 scale 会让 setpoint 悄悄发错而不会报错。控制系统里这比"读失败"危险得多。建议在计划里把"绑定 revision 强校验 + 越界 fail-closed"提到和 canonical 转换同等的地位，并明确"宁可拒发也不用可疑绑定"。

另外值得在计划里补一句权衡：转换逻辑放 Gateway（外部防腐层）是合理的，但绑定的**权威源在 Resource**。是否考虑过让 Resource 暴露一个翻译能力、Gateway 只做协议 I/O？计划直接选了 Gateway 缓存方案却没交代为什么不走另一条，属于架构决策缺少 rationale。

---

## 四、Telemetry 侧的两个具体缺口

计划大量依赖"扩展后的 Telemetry fleet/batch snapshot"和 StateCollector 的"per-metric freshness/quality 检查"，但对照现状有两个缺口：

1. **值类型只有 float64。** `Snapshot.Metrics` 是 `map[string]float64`。而 metric 契约要定义 `Float/Int/Bool/Enum` 和 canonical unit。SOC 场景够用，但契约一旦声明支持多类型，telemetry 的存储/快照模型就对不上。这是未来兼容性的一个隐性债务，计划应至少说明"本轮 metric 契约的 dtype 只在 Resource/Gateway 校验层生效，telemetry 值域暂仍 float64"。

2. **快照没有 per-metric 质量与时间戳。** `Snapshot` 只有整体 `UpdatedAt`，且 `Apply` 会丢弃非 good 样本（好的读数覆盖、坏的跳过）。但 StateCollector 计划"逐 metric 检查 freshness、quality、必需 metric"并按严格模式跳过——**当前快照模型无法提供 per-metric 的时间戳和质量**。这要么扩展 Snapshot 模型，要么改走时序查询。这是一个未列入"破坏性调整"的 Telemetry 工作流。

3. 顺带：`GetFleetSnapshot` 已存在，但它是"整租户所有 CU"。计划说 StateCollector"一次获取**所需** CU"，需要一个**按 CU 列表过滤的批量快照 RPC**，现有 proto 没有。这属于要新增的 API，计划里没显式列。

---

## 五、几处 rationale 缺失 / 语义未定，容易固化错误假设

这几条不影响方向，但按你"抽象与架构合理性优先"的要求，属于该在计划阶段定清楚、否则实现时会拍脑袋的点：

1. **`resource_revision` 的确定性算法未定义。** 计划说它"由 scope 内 node/capability/point version 确定性计算，不另建快照表"。但把多个实体 version 归并成一个 revision 需要明确算法（排序后 hash？还是 max？）和明确的输入集合与排序，否则分页/顺序差异会让"同一状态算出不同 revision"，执行前的 revision 复校验会变成偶发拒绝。**这是 PlanExecutor 可靠性的地基，必须先定义死并单测。**

2. **Allocator 的不可行处理未定义。** `HeuristicAllocator` 按"可用功率上限 + SOC 余量"分配 scope 总功率。StateCollector 处理了"数据缺失/陈旧"，但**当各 CU 上限之和 < objective 目标功率时怎么办**（截断并告警？整体失败？）没写。这是分配算法的核心域语义，应在计划里定调。

3. **Cooldown 键的粒度变化改变了语义。** 现有代码用 `(CUCode, RuleID, Direction)`，`rule.go` 里有很详细的注释解释"为什么 Direction 必须进 key"。计划改成 `(policy_id, direction)`（scope 级）。对 Asset 策略这意味着**整个 asset 一个冷却，失去 per-CU 防抖**。这大概率是有意的（scope 级决策），但计划没点明这个取舍——建议明确"asset 内单 CU 的振荡在本轮可接受"。

4. **公共契约模块的落点没定。** 计划说"新增公共契约模块"定义 metric/capability ID，但没说放哪。由于每个服务是**独立 Go module**（我确认了 `internal/*/go.mod` 各自独立），而 resource/gateway/telemetry/dispatch/decision 都要引用同一批 ID，这个模块的位置直接决定 module 依赖图。合理落点是 `platform`（已是共享依赖）或新建一个共享 contract module。这是个会影响全局 module graph 的决策，不该留空。

5. **现有 `CU.CapabilityTags []string` 的去向没交代。** CU 模型已有一个非结构化的 `CapabilityTags`。计划新增强类型 `cu_capabilities` 表，但没说旧的 `CapabilityTags` 是废弃、并存还是迁移。既然是破坏性重构，应明确删除或收敛，避免两套"能力"概念并存。

---

## 六、抽象数量 vs 单一用例：YAGNI 的边界

计划保留了大量扩展点（`Solver`、`ForecastProvider`、`ApprovalPolicy`、`SubmitObjectiveHandler`、`decision_outbox`、`PlanExecution` 的 claim/lease 崩溃恢复），并新增约 11 张表，而**唯一真正跑通的业务只有一条 SOC 阈值**。

我的评估是**大部分是合理的**，且计划自己划了很好的线（"市场/预测只写文档不建空表"），这点值得肯定。但有两处值得你留意：

- **outbox 是对的**（事务提交与 gRPC Dispatch 副作用解耦，是标准的可靠投递模式，不算过度设计）。
- **但存在一处并发模型的内在不一致**：cooldown 那节说要保证"重启和**单副本**切换后不重复触发"，而 PlanExecution 又要建"数据库 claim/lease 防止**多实例**重复执行"。到底目标是单副本还是多副本？这决定了要不要 claim/lease 这套复杂度。建议先定死并发假设，再决定这块的深度。

对学习项目而言，"把正确的模式练一遍"本身是价值，所以我不建议砍抽象；但**强烈建议实现顺序上先打通最薄的一条垂直切片**（一个 CU、一个立即 step、内存 cooldown、无 outbox），端到端跑通后再逐层加持久化/可靠性。否则 11 张表和一堆 port 会淹没那条唯一真实的 SOC 逻辑，反而不利于"看清架构"。

---

## 小结与建议

方向、边界、核心抽象都**站得住**，可以按这个大方向做。落地前建议补齐/明确以下几点（按优先级）：

1. 修正"optimization 作为可运行对照"的表述——明确并存窗口只到步骤 2；每个提交步骤约束为独立可编译可测。
2. 把 **alarm** 的 SOE/规则迁移补进破坏性调整清单。
3. 把 **Gateway 入站遥测契约变更**（携带 external_address）和 **scale/offset 静默错误的安全语义**写进计划，并补上不选"Resource 侧翻译"的 rationale。
4. 把 **Telemetry** 的三项（值类型策略、per-metric quality/时间戳、按 CU 列表过滤的批量快照 RPC）列为独立工作流。
5. 先定义死 **`resource_revision` 算法**、**Allocator 不可行处理**、**公共契约模块落点**、**并发模型（单/多副本）**。
6. 实现上先做最薄垂直切片再加可靠性层。

需要的话，我可以基于上面这些点，直接帮你把这份 plan 的 todos 和"破坏性调整/提交顺序"改写成修订版（补上 alarm、Telemetry 工作流、并存窗口澄清、以及把每步约束为可独立验收）。

要我直接改写 plan 文件吗？