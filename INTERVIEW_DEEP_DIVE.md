# VPP Backend 深度拷问清单

> 基于代码实现细节、`docs/DECISIONS.md`（ADR）、`plan_v2.md` 和测试用例整理，不是通用面试题库。
> 建议先自己口头回答一遍，答不上来的地方回去翻对应代码文件，比直接看参考答案更有效。

---

## Dispatch（状态机 / 熔断 / 超时重试）

**Q1.** `OnCommandResult` **把 Task/Action/Command 拆成三个 Repository 分别更新,而不是整棵树 Save 一次——事务一致性怎么保证？**

写频率差异巨大：Command 几乎每次 Kafka 回调都要写,Action 偶尔写,Task 几乎不写,拆开是为了避免"改一个字段却重写整棵树"的写放大。Dispatcher 在内存里一次性算出完整的 `CommandResultOutcome`（哪些 Command/Action 变了、Task 是否变了）,再由 Application 层依次调用三个 `repo.Update`。要注意：这几次 Update 目前是否包在同一个数据库事务里,是可以被追问、也值得主动去代码里确认的点——如果没有事务包裹,理论上存在"Command 更新成功但 Action 更新失败"的中间态风险,是一个可以诚实指出的改进空间。

**Q2. Sequential Action 里"命令2必须等命令1完成",具体是谁在等？有没有阻塞或轮询？**

没有阻塞、没有轮询。靠的是 `OnCommandResult` 返回的 `NextCommands` 做"接力"（continuation）：命令1的结果一到（同步返回或 Kafka 回调）,Dispatcher 在同一次调用里就算出下一条要发的命令,Application 层拿到 `NextCommands` 立刻调用 `gatewayPort.ExecuteCommand`。本质是事件驱动的状态推进,不是某个 goroutine 挂起等待。

**Q3. FailFast 熔断时,已经处于 Sending 状态的命令怎么处理？为什么不直接 Cancel 掉？**

代码里 `ControlCommand.Cancel()` 的状态校验只允许从 Pending 转换,Sending 的命令不会被强行改成 Cancelled——因为它可能已经被物理设备执行,系统没法"撤回"一个已经在路上的指令,硬改状态只会让记录和物理现实脱节。正确做法是放着让它自然超时,交给 TimeoutScanner 处理。

**Q4. 为什么熔断路径完全不做反向补偿？Action1 让电池放电成功、Action2 失败了,电池不就一直放电吗？**

这是 ADR 里"熔断不干预（Fail-Safe）"策略的核心权衡：电力场景做自动反向操作的风险（二次冲击）比"保持现状"更大;而且 VPP 优化算法通常 5-15 分钟一个周期重新计算并下发全量指令,下一周期的正确指令会自然覆盖当前的失败状态,相当于"动态补偿"。`FailurePolicy` 被设计成枚举而不是硬编码逻辑,就是为了未来真的要做 Compensate 策略时可以直接加分支,不用改接口——这是典型的"预留扩展点但不过度设计"。

**Q5. TimeoutScanner 是全表扫描过期命令吗？命令量很大时会不会有性能问题？**

migration 里给 `control_commands` 建了一个 partial index：`WHERE status='sending'`,只覆盖当前处于 Sending 态的少数行,配合 `deadline_at` 排序,扫描范围是"正在等结果的命令"而不是全表。当前量级下没问题;如果要支撑千万级并发命令,还需要考虑按租户分区或者分片,这个是可以主动提的扩展方向。

**Q6. 命令的幂等检查具体怎么做的？Kafka 把** `command.completed` **重复投递两次会怎样？**

`HandleCommandResult` 先 `FindByCommandID` 拿到 Task,检查目标 Command 当前状态——如果已经是终态（Succeeded/Failed/Cancelled）就直接返回,不再调用 Dispatcher。领域模型本身的状态迁移方法也有前置校验（比如 `MarkSucceeded` 只允许从 Sending 迁移,`MarkSending` 重复调用会报错"double sending"）,双重保险下重复消息不会产生副作用。

**Q7. Sequential 和 Parallel 两种 ExecutionPolicy,具体怎么决定"这一刻该发哪些命令"？**

`CommandsToDispatch()`：Sequential 下只有当 Action 内没有正在 Sending 的命令时,才返回第一条 Pending 命令（一次只发一条）；Parallel 下一次性返回所有 Pending 命令。`dispatcher_test.go` 里的 `TestDispatcher_ParallelPartialSuccess` 专门验证了 parallel 场景下一条命令完成、另一条还在 Sending 时 Action 不会提前判定完成，必须等 `AllCommandsFinished()` 为真。

**Q8.** `CommandValue` **为什么要求"恰好一个字段非 nil"，而不是简单地用一个** `interface{}` **或** `any` **表示控制值？**

用 `BoolValue/IntValue/FloatValue/StringValue` 四个指针字段而不是 `any`，是为了在领域层就能做强类型校验（`Validate()` 检查有且仅有一个字段非空）和序列化的确定性（JSON/Proto oneof 语义对齐）。用 `any` 的话，类型安全和校验都要推迟到运行时用反射判断，容易漏掉"同时传了两个值"这种非法输入。

---



## Gateway（路由 / 生命周期同步 / 幂等）

**Q1. Router 用字符串匹配 ExternalSystem 决定走 simulator 还是默认适配器,这样硬编码会不会有隐患？**

是有意的最小实现——目前只有 simulator 和其它（ems_log）两种,用 `EqualFold` 做大小写不敏感匹配。隐患是新增一个真实 EMS 类型时得在 Router 里再加一个 if 分支,严格说违反了开闭原则。更完整的做法是做一个 `map[string]port.EMSClient` 的注册表按 ExternalSystem 查表分发——这是可以主动指出的具体改进点,而不是空泛地说"可以更好"。

**Q2.** `NewRouter` **对** `defaultClient` **为 nil 直接 panic,这样是不是不太优雅？为什么这么设计？**

这是项目里反复出现的"快速失败"模式——必需依赖缺失应该在服务启动、组装依赖注入阶段就崩溃,而不是等某个请求进来才在运行时发现"没有默认出站适配器"。TimeoutScanner 的构造函数对 taskRepo/gateway 等必需依赖也是同样处理。

**Q3. resource 和 gateway 之间"非对称"生命周期同步——如果事件顺序乱了,比如 Mapping 还没建、CU 先被删了,会怎样？**

消费方按 CUCode 查 DeviceMapping 做 disable,如果 Mapping 不存在,`DisableMappingByCUCode` 是 no-op（查不到就什么都不做,不报错）——`lifecycle_consumer_test.go` 里对应的正是这种"温和幂等"路径。乱序不会导致报错或状态错乱,只是这次 disable 什么都没做而已。

**Q4. 事件里 TenantID 同时有 Envelope 级别和 Payload 级别两个来源,为什么要"Envelope 优先、Payload 兜底"？**

`TestHandleMessage_ResourceDeleted_TenantFallback` 验证的正是这条规则：Envelope 的 TenantID 为空时用 Payload 里的。这很可能是历史演进留下的兼容性设计——早期事件生产方没有稳定填充 Envelope 级别的 TenantID,消费方为兼容新旧格式做了降级读取。面试被问到"这是不是技术债"时,建议诚实承认这是兼容性妥协,而不是刻意的架构设计。

**Q5. "同步受理 + 异步终态"模式下,如果 Gateway 这次同步调用外部系统一直不返回（比如网络分区）,Dispatch 会一直卡住吗？**

不会。gRPC 客户端本身应该配置了超时（`DialGRPC`）；即便这层超时没触发,`ControlCommand` 自己也有独立的 Timeout 字段（默认 30s）,TimeoutScanner 会把它当成过期命令捞出来处理,不依赖 Gateway 这次调用一定要返回。

**Q6.** `NewDeviceMapping` **的校验里，**`ID / TenantID / ExternalSystem / ExternalID / CUCode` **五个字段都做了空值/空白校验，为什么这么严格？少一个会怎样？**

DeviceMapping 是整个 Gateway 路由体系的核心记录，CUCode 缺失会导致 lifecycle 事件反查不到对应 mapping（无法清理）；ExternalSystem/ExternalID 缺失会导致 Router 找不到该走哪个出站适配器。五个字段任何一个缺失都会让这条映射变成"半失效"状态，且大概率在运行期才会暴露成一个隐蔽 bug，所以在构造函数里就做强校验，把错误提前到创建阶段。

---



## Resource（分布式认领 / 补偿 / 资源树建模）

**Q1.** `SELECT FOR UPDATE SKIP LOCKED` **具体怎么工作？为什么不用乐观锁（version 字段）？**

`SKIP LOCKED` 让并发事务遇到已被锁住的行时直接跳过而不是阻塞等待,配合 `ORDER BY` + `LIMIT 1`,多个 Worker 实例同时轮询也能各自拿到不同的 Job,不会重复认领。乐观锁也能防重复处理,但冲突概率高时会有大量重试;`SKIP LOCKED` 是数据库原生的"排他式认领",一条 `UPDATE ... RETURNING` 就完成,不需要应用层重试循环——ADR-002 里写了这条取舍。

**Q2. ImportWorker 单 goroutine、5 秒轮询,同时提交 100 个大批量导入 Job 会怎样？**

单 Pod 内串行处理,两个 Job 之间最多有一次轮询间隔（5s）的空档；多 Pod 部署时,各自的 goroutine 都在轮询同一张 `import_jobs` 表,靠 SKIP LOCKED 互斥认领,吞吐大致等于 Pod 数量 × 单 Job 处理速度。ADR-003 写明了取舍：现在规模够用,扩展路径是加 Pod 而不是改代码加并发池。

**Q3.** `compensateCreated` **补偿删除是在什么场景触发的？如果补偿删除本身又失败了呢？**

场景：批量创建按 chunk 分片写入,前几个 chunk 已成功插入,某个后续 chunk 失败——这时要把已成功插入的 ID 删掉,避免 RetryJob 重跑时产生重复+孤儿数据。如果补偿删除本身失败,代码把 cause 和补偿失败包在一起返回（`%w (compensate delete ... failed: ...)`）,不会静默吞掉,方便运维知道"这次失败后残留了脏数据需要人工核查"。ADR-005 也明确写了：进程崩溃导致的脏数据不在这个补偿路径覆盖范围内。

**Q4. 为什么 SoftDelete 不做级联？删了一个 Site,下面的 Asset/CU/Point 不就成孤儿了？**

ADR-006 里这条明确写的是"待定"——当前每个实体独立软删除,没有级联,文档列了三个选项（应用层显式级联 / DB 触发器级联 / 交给上层调度服务决定）但没拍板。这是一个很好的"考你是否读过自己的 ADR、以及会不会拍板"的问题,建议提前想清楚自己的答案（倾向 A：应用层显式级联,因为触发器不透明、难测试）。

**Q5. 四级资源树为什么公共字段放在 nodes 表、类型专属字段各开扩展表,而不是每种类型一张完整表？**

这是单表继承思路的权衡：树遍历、面包屑、移动、软删这些公共操作只需要操作 nodes 表,不用为四种类型各写一遍相同逻辑；CU 的 Provider/Protocol、Point 的量测类型等专属字段放各自扩展表,避免 nodes 表字段爆炸。代价是查询一个完整 CU 要 JOIN 两张表。

---



## Telemetry（一致性分级 / SOE 判定 / 快照规则）

**Q1. "30 天查询窗口"限制为什么不在 domain 层做校验？测试里写的是 domain 不强制？**

`model_test.go` 明确注释"Domain does NOT enforce 30-day window"——这个限制是应用层/API 层的产品策略（防止一次查询扫太大范围拖垮 TimescaleDB）,不是数据本身合法性的领域规则。domain 层只保证 tenant/cu/metric/时间范围结构完整。故意把这类"业务策略性限制"留在更外层,是为了以后调整策略（比如给高级租户放宽到 90 天）不用碰 domain 代码。

**Q2. 写入三步（TimescaleDB → Redis 快照 → Kafka SOE）为什么第一步硬失败、后两步 best-effort？**

TimescaleDB 是历史真相的唯一来源,写失败必须让调用方知道并重试,否则数据永久丢失;Redis 快照只是"最新态缓存"给仪表盘用,丢一次不影响历史数据完整性,下次 Ingest 自然覆盖;SOE 是变位通知,丢一次只是少一条告警,不影响时序数据本身。本质是给每种存储分配匹配的一致性等级，而不是所有写入路径一刀切当成关键路径。

**Q3. SOE 判定规则是什么？为什么只有 DISCRETE 类型产生 SOE、ANALOG 只更新数值？**

DISCRETE（开关量,比如断路器分/合）的值变化本身就是有业务含义的事件，需要记录和告警;ANALOG（模拟量比如功率、SOC）是连续变化的数值,几乎每次采样都在变,如果也发 SOE 会产生海量无意义事件,所以只更新快照数值,不发事件。

**Q4. 快照更新规则"仅 QualityGood 覆盖旧值",如果设备一直上报 Bad 质量数据,快照会一直停留在很久以前吗？**

会。这是有意的保守策略——宁可展示旧的但确认为真的值,也不用不确定质量的坏数据污染当前展示。潜在问题是前端可能看到一个"数值正常但其实很久没更新"的假象,需要配合 LastSeenAt 或额外的"数据新鲜度"指标才能弥补——这是可以主动指出、需要产品侧配合的边界情况,不是纯技术就能解决的。

---



## Simulator（故障注入 / 配置同步）

**Q1. FaultEngine.Lookup 对多个 key 做 OR 合并,这是什么场景需要的？**

同一个虚拟设备在故障注入时可能同时被 cu_code（内部身份）和 external_id（Gateway 用的外部身份）两种 key 标注故障,OR 合并保证不管故障打在哪个 key 上,只要命中任意一个,Offline / CommandReject / TelemetryDelay 效果都要生效,不会因为查询用错了 key 而漏判。`engine_test.go` 里 `TestEngine_ApplyClearAndLookup` 就是在验证这条合并逻辑。

**Q2. Simulator 不自建设备目录,从 Resource 只读拉取——如果 Resource 把某个 CU 删了,Simulator 怎么知道要销毁对应虚拟设备？**

目前靠的是 reload 机制（重新从 Resource 全量拉取并重建实例）,不是订阅 Kafka 做增量同步。这是文档里明确写的"刻意未做"的 Phase 2 能力（经 Kafka 动态增删设备）,v1 阶段存在一个时间窗口：CU 已经在 Resource 删除,但 Simulator 还在 Tick 一个幽灵设备,直到下次手动/定时 reload。这是已知限制,不是遗漏。

**Q3. Debug API 的三种故障注入（offline / command_reject / telemetry_delay）分别在链路的哪个环节生效？**

offline 作用最外层——设备完全停 Tick、拒绝所有命令、不上报;command_reject 只影响 Execute 路径,设备仍正常 Tick 和上报遥测,只是命令会被拒;telemetry_delay 只延迟 Publish 阶段（快照生成后、HTTP 上报前人为 sleep）,不影响设备内部状态演化。三者分层次生效,方便组合出不同故障场景做演示。

**Q4. 未知设备类型走** `Passthrough`**，这个兜底类型具体能做到什么程度？为什么不直接报错拒绝创建？**

`device_test.go` 里验证了 `New()` 对未注册类型（如 `"other"`）会返回 `*passthrough` 而不是报错。Passthrough 仍然支持读写点位和轻量扰动，只是没有电池/PCS/PV 那样的物理语义（SOC 变化曲线等）。这样设计是为了让 Resource 侧新增一种设备类型时，Simulator 不需要同步跟着改代码就能先跑起来（哪怕行为简单），避免两个服务的迭代节奏被绑定。

---



## Platform / 跨服务（泛型装饰器 / 事件信封 / 测试策略）

**Q1. decorator 包用 Go 泛型实现 Chain,Command 和 Query 输入输出类型完全不同,为什么能共用同一套 Middleware？**

`Handler[C, R any]` 用类型参数抽象输入 C 和输出 R,Middleware 本身不关心 C/R 的业务含义,只是在调用 `next.Handle` 前后插入日志/指标/追踪这类通用逻辑；Command 和 Query 的 Handler 签名形状相同（`Handle(ctx, in) (out, err)`）,只是语义不同,泛型消除了"为 Command 和 Query 各写一套几乎一样的装饰器"的重复代码。

**Q2.** `event.Envelope[T]` **用泛型做统一信封,相比每种事件自己定义一个包含公共字段的 struct,好在哪？**

信封字段（event_id/event_type/version/tenant_id/occurred_at）只需维护一份定义和序列化逻辑,新增一种事件只需要定义 Payload 本身;消费方可以先用一个轻量 `peekEventType` 只解析 event_type 做路由,再按需完整反序列化成 `Envelope[SpecificPayload]`,避免拿到不认识的事件类型时也要完整解析失败。

**Q3. 目前 domain/application 层测试覆盖率不错,但 outbound adapter（postgres/kafka/grpc client）基本是 0%,这是刻意的分层策略还是没来得及做？**

建议诚实回答两层意思：domain 层是纯函数、易 mock、ROI 最高,优先覆盖是合理的优先级排序;但 outbound adapter 目前确实是空白,不是"不需要测"而是"还没做",要补的话思路是用 testcontainers-go 起一次性 Postgres/Kafka 容器做集成测试，而不是继续加纯 mock 单测（mock 数据库连接测不出真实 SQL 问题）。

---



## 使用建议

1. 先不看参考答案，自己口头复述一遍；卡壳的地方回代码里找答案，而不是回这份文档找答案。
2. 每次被新问题卡住，就是一个"这里我其实没有真正搞懂"的信号——去代码里追一遍调用链，而不是死记这份文档的表述。
3. 这份清单会随代码演进过时（比如 `CancelTask` 一旦实现，Dispatch Q3/Q4 的答案需要更新），定期回来核对。

