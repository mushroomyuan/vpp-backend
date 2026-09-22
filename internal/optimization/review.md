我审查了设计方案（`plans/optimization_服务设计方案_71aae676.plan.md`）和实际代码（`internal/optimization/`）。结论：**方案和代码高度一致，没有出现"文档说一套、代码做另一套"的问题**；但代码里确实有 2 个值得认真对待的设计缺口、2 个低优先级的记录性问题。整体上 Optimization 没有 Forecast 那种"缓存了一个会过时的结论"式的时序 bug——原因是结构性的：Optimization 每次 tick 都直接问 Telemetry"现在"是什么状态，不存在"预先算好未来、再原样返回"这个模式，所以不会有那类 bug。下面按严重程度列出。

## 中高优先级

> **状态：#1、#2 已修复。** 见下方各条内的"已修复"说明；改动范围：`domain/model/{rule.go,target.go}`、`domain/service/{cooldown.go,evaluator.go}`、`application/command/run_decision_cycle.go`，及对应测试。#3/#4/#5 仍是记录性问题，未改代码。

### 1. 冷却期不分方向，可能压制合法的反向决策

- `internal/optimization/domain/service/cooldown.go:44` `cooldownKey(cuCode, ruleID)` 只用 `(RuleID, CUCode)` 做 key，不含方向（充电/放电）。
- 场景：SOC 跌破 `MinSOC` 触发充电，冷却期开始（键是 `"soc_threshold:cu-1"`）；如果冷却期内 SOC 又冲高突破 `MaxSOC`（需要放电），`evaluateSOCThreshold`（`evaluator.go`）用的是**同一个冷却键**，这个本该被响应的、方向相反的新状况会被当成"重复触发"直接压掉，直到冷却期结束才会重新评估。
- 冷却机制的设计初衷（讨论 §一 风险 #5）是"不要对同一份 stale 数据反复做同一个决策"，但现在实现的其实是更宽的规则——"这个 CU 在冷却期内不管什么决策都不做"，两者不是一回事。
- `evaluator_test.go`/`cooldown_test.go` 都没有覆盖这个"冷却期内方向反转"的场景，是一个真实的测试盲区。
- 影响面：如果电池 SOC 真的在冷却期内（默认 2 分钟）从低冲到高（模拟环境/快充/故障下并非不可能），Optimization 会在冷却期内对一个需要响应的超容状态"装作没看见"。

**已修复**：`domain/model.Direction`（`charge`/`discharge`）加入冷却键，`cooldownTracker` 现在按 `(CUCode, RuleID, Direction)` 三元组隔离。副作用：方向只有在读到 Telemetry 快照后才知道，`evaluateSOCThreshold` 因此改成"先读快照、算出方向，再查冷却"，v1 最初"冷却激活时直接跳过 Telemetry 读取"的小优化被移除——这个优化和这次修复在结构上互斥（正是要在冷却期内也能读到数据才能发现方向反转）。新增测试：`cooldown_test.go` 的 `TestCooldownTracker_IndependentPerDirection`、`evaluator_test.go` 的 `TestEvaluator_CooldownDoesNotSuppressOppositeDirection`。

### 2. `PointTarget` 不带具体 RuleID，metrics/日志的规则归因是硬编码猜出来的，会在加第二条规则时悄悄写错

- `internal/optimization/application/command/run_decision_cycle.go:187`（`ruleIDOf`）和 `:213`（`targetLogFields`）：只要 `PointTarget.Source() == SourceInternalRule`，就无条件把 `rule_id` 写成 `model.RuleSOCThreshold`。
- 根因：`domain/model/target.go` 的 `PointTarget` 只有 `Source()`（"internal_rule"/"external_dr"/"external_market" 这种粗分类），没有携带"具体是哪条规则实例产生的"这个信息。
- 现在为什么"凑巧是对的"：v1 只有一种规则（`RuleSOCThreshold`），硬编码不会被戳穿。
- 为什么这是个隐患：`evaluator.go` 自己的注释就写了"如果加第二种规则，重新考虑要不要上 registry"——这正是代码自己预告的演进方向。一旦真的加了第二条内部规则，它产生的 `PointTarget` 依然会被这两个函数打成 `soc_threshold`，`optimization_rules_fired_total{rule_id=...}` 这个 Prometheus 指标和结构化日志的 `rule_id` 字段会开始悄悄撒谎——编译通过、现有测试全过（因为测试也只测了一种规则），但可观测性数据是错的。和这次 Forecast 复盘里那类"今天凑巧对、不是从结构上保证对"的 bug 是同一类问题。
- 修复方向（不是现在就改）：给 `PointTarget` 加一个真正的 `RuleID`/`ProducedBy` 字段，由产生它的规则自己填，`ruleIDOf`/`targetLogFields` 直接读这个字段，不再靠 `Source()` 反推。

**已修复**：`Target` 接口加了 `RuleID() string` 方法；`PointTarget` 新增 `Rule RuleID` 字段，由 `evaluator.go` 在构造时填自己的 `model.RuleSOCThreshold`（产出方自报家门，不是消费方反推）；`AggregateTarget.RuleID()` 恒返回 `""`（它不是某条规则的产物）。`run_decision_cycle.go` 里的 `ruleIDOf` 辅助函数整个删掉，调用点直接改成 `target.RuleID()`；`targetLogFields` 同理不再对 `Source()` 做特判。新增测试：`target_test.go` 的 `TestPointTarget_RuleIDEmptyWhenUnset`/`TestAggregateTarget_Accessors` 的 `RuleID()` 断言。

## 低优先级 / 记录性问题

### 3.（休眠中，已部分自述）`AggregateTarget.Scope` 的"CUCode 或 AssetID"承诺，和 `resource_grpc.GetCapacityKW` 只认 AssetID 的实现对不上

- `domain/model/target.go`：`Scope []string // CUCode or AssetID list`。
- `adapter/outbound/resource_grpc/client.go` 的 `GetCapacityKW` 只会拿每个 `id` 去调 `GetAsset`，CUCode 传进去必然 `NotFound`，会被当成"未知容量"直接跳过（`splitByCapacity` 的"排除未知容量"分支）。
- 好消息：这条路径至今没有真实调用方（`AggregateTarget` 还没接 Market），且 `resource_grpc` 自己的注释已经写明这是"v1 简化，CU 场景要先解析成 Asset"，不是被藏起来的坑；真出现这种情况，`splitByCapacity` 大概率是整批都解析不出来 → 直接报错（"no resources with known capacity"），属于"失败得很响"而不是"悄悄算错"，严重度不高。
- 仍然值得记一笔：`Target` 类型的文档注释比适配器实际能力承诺得更宽，以后写 Market 的人如果只读 `target.go` 的注释，不会意识到这个限制。

### 4. 没有校验"同一个 CUCode 配了两条 SOCThresholdRule"这种误配置

- `options/options.go` 的 `Validate()` 逐条检查字段合法性，但不检查跨条目的 `cu-code` 重复。
- 因为冷却键只按 `(RuleID, CUCode)`，如果误配了两条指向同一个 CU 的规则（哪怕阈值不同），先触发的那条会在冷却期内连带压制另一条——不是运行时崩溃，是"配置上说了两条策略，实际只有一条在生效"的静默行为，没有任何报错提示运维。

### 5.（已知取舍，非缺陷）单 tick 内多租户严格串行

- `decision_loop.go` 的 `tick()` 对配置的每个租户同步串行跑一遍 `RunDecisionCycle`；`time.Ticker` 在处理未结束时会丢 tick（Go 标准行为）。如果某次 tick 因为 Telemetry/Dispatch 变慢（熔断器还没跳闸前）拖得比 `decision-interval` 还长，后续 tick 会被静默跳过，不会排队补上。这和"单实例、尽力而为"的既定设计定位一致（README/OVERVIEW 已经说明是单实例部署），不算缺陷，但目前 OVERVIEW.md"刻意未做"清单里没有明确写这一条，值得补一句说明，让"多租户会串行、慢下游会连带丢 tick"这件事白纸黑字。

## 确认没问题的部分（顺带说明，免得显得只挑刺）

- 同方向重复触发的冷却抑制、stale 快照跳过（且不误记冷却）、单个 CU/规则失败不阻断其它 CU——`evaluator_test.go` 都有对应用例覆盖，逻辑和测试都对。
- `Allocate` 的 unsupported-type 分支、`splitByCapacity` 的空 scope/零容量/未知容量分支——`allocate_test.go` 覆盖到位，行为符合文档描述。
- 三个 outbound gRPC client（telemetry/resource/dispatch）的超时+熔断拦截器接线、`createServer` 里任一个初始化失败时前面已建立的连接会被正确 `Close()`——检查过没有资源泄漏。
- `config/options` 的校验（`decision-interval > 30s`、`MinSOC < MaxSOC`、必填字段）都实际生效。
- `CommandValue`/`CommandSpec` 在 `dispatch_grpc` 边界的转换会先 `Validate()` 再转 proto，判别式处理完整。

---

第 3、4、5 条仍是记录性问题，未改代码：#3 是休眠路径（`AggregateTarget` 还没有真实调用方）、#4 是配置校验空白（可以在 `options.Validate()` 里加跨条目重复 `cu-code` 检查）、#5 是已知的单实例串行取舍（建议在 `OVERVIEW.md`"刻意未做"清单里补一句）。