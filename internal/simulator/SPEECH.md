# vpp-simulator 团队分享演讲稿

> 建议时长：**8–12 分钟**（不含 Demo）。  
> 配合 [`OVERVIEW.md`](./OVERVIEW.md) 中的架构图使用；括号内为可选备注，不必照读。

---

## 开场（约 1 分钟）

大家好，今天想跟大家介绍一下我最近做的 **vpp-simulator**——虚拟设备运行时。

先用一句话说明它是什么：

> **在没有真实 EMS、没有现场设备的时候，让整个 VPP 平台仍然能跑出一条完整、可重复、可观测的业务闭环。**

它不是一个 Mock API，也不是写完就扔的测试脚本。  
它是一组**长期跑着的、有内部状态的活设备**：会随时间变化，会响应控制命令，也可以人为注入故障。

在平台里，它扮演的是 Gateway 对面的外部系统，标识是 `ExternalSystem = "simulator"`。

---

## 为什么要做它（约 1.5 分钟）

大家应该都遇到过这些问题：

- 没有真实 EMS，联调就卡住；
- 遥测和控制不好复现，昨天测过的场景今天对不上；
- 异常路径很难造——总不能真去拔设备；
- Resource、Gateway、Telemetry、Dispatch 各自 Mock，最后合起来发现链路根本不通。

所以 Simulator 的核心目标很明确：

**让 Resource → Gateway → Telemetry → Dispatch，走真实协议路径闭环运转，而不是各服务自己骗自己。**

有了它之后：

- 本地就能跑完整业务；
- 状态可以重置，场景可以重复；
- 故障可以按设备注入；
- 从「下发」到「反馈」是一条真链路。

---

## 它到底做了什么：六个亮点（约 4 分钟）

### 亮点一：状态驱动的设备运行时

设备是内存里的活跃对象，不是数据库里的一行记录。

每个设备都实现同一套契约：

- **Tick**：每隔一段时间自然演化——比如储能 SOC 变化、功率抖动、光伏按日照曲线出力；
- **Execute**：接收控制点写入；
- **Snapshot**：按 Resource 声明的测点，输出当前遥测。

目前内置了 Battery、PCS、PV、Meter；未知类型走 Passthrough，照样能读写、有轻微扰动。

（可强调）**它像真设备一样「活着」，而不只是返回固定 JSON。**

### 亮点二：Resource 是配置权威

Simulator **不自己维护一套设备目录**。

启动或 reload 时，它从 Resource 只读拉取：Site → Asset → CU → Point，过滤 `provider = simulator` 的 CU，再实例化成运行时对象。

这样做的好处很直接：**资源模型改一处，模拟侧跟着变**，不会出现「平台一套台账、模拟器另一套」的漂移。

### 亮点三：闭环走真路径，不是旁路

这一点非常重要。

- 遥测：Simulator → Gateway ingest → Telemetry  
- 控制：Dispatch → Gateway → Simulator 命令 API → 设备状态变化 → 下一拍遥测反馈  

和真实 EMS 的路径是同构的。

Dispatch **完全不用知道** Simulator 的存在——它只跟 Gateway 说话。  
到底打到模拟设备还是真实外部系统，由 Gateway 的 Mapping（`ExternalSystem`）决定。

### 亮点四：故障注入是一等能力

调试异常场景时，不用改代码、不用拔线。Debug API 可以对某个设备注入：

- **offline**：离线——停演化、拒命令、不上报；
- **command_reject**：命令被拒——验证失败路径；
- **telemetry_delay**：上报延迟——验证超时与时序；
- **clear**：清除故障。

这样我们可以讲两套故事：一套「链路一切正常」，一套「设备侧出问题了」。

### 亮点五：可观测、可干预

有 Debug API，可以看全部设备实时状态、本地注入命令、reset、reload。  
也接了 Prometheus 和 OpenTelemetry，跟其它微服务同一套可观测体系。

需要的时候，还可以关掉上报，只做内存 Tick，方便单独调运行时。

### 亮点六：协议中立的「外部系统」角色

命名我刻意用了 `simulator`，而不是 `ems_xxx`。

因为它模拟的是 Gateway 对面的**任意外部系统 / CU**，不假定对方一定是 EMS。  
Gateway 按 `ExternalSystem` 路由：`simulator` 走我们的 HTTP 适配器，其它继续走现有的 `ems_log` 或后续真实适配器。

---

## 架构怎么理解（约 2.5 分钟）

（建议此时切到 OVERVIEW 里的架构图）

内部可以看成几层：

1. **从 Resource 加载**设备规格；
2. **Runtime Manager** 管住所有 Device；
3. **Global Ticker** 周期驱动 Tick；
4. **Telemetry Publisher** 把快照推给 Gateway；
5. **Command API** 接收 Gateway 下发的控制；
6. **Fault Engine** 横切地挂故障，不污染设备模型本身。

职责边界也很清楚——Simulator **只回答一件事：设备此刻怎么跑**。

它**不做**：

- 资源 CRUD → Resource  
- ID 映射 / 协议转换 → Gateway  
- 时序存储 → Telemetry  
- 任务编排 → Dispatch  

和其它服务的关系，可以记一句口诀：

> **Resource 定义「有什么设备」；  
> Simulator 定义「设备此刻怎么跑」；  
> Gateway 负责「内外身份与通道」；  
> Telemetry 负责「记下来」；  
> Dispatch 负责「让它去做」。**

再展开一点：

- **对 Resource**：只读配置源；
- **对 Gateway**：唯一双向对接面——上报走它，命令也从它来；
- **对 Telemetry**：间接消费方，Simulator 不直连写库；
- **对 Dispatch**：完全透明，Mapping 指到 simulator，同一套任务引擎就能驱动虚拟设备。

---

## 设计原则（约 1 分钟）

为什么这样设计，我总结六条：

1. **Runtime 第一** —— 设备是状态机，不是 CRUD 记录；  
2. **配置外置到 Resource** —— 单一真相源；  
3. **自然行为 + 可控干预** —— 默认像真设备，需要时再 Debug / Fault；  
4. **最小化持久化** —— Phase 1 以内存为主，可 reload、可 reset；  
5. **服务自治** —— 不越权；  
6. **与真实路径同构** —— 测到的是真链路，不是测试替身。

---

## 当前做到哪、还没做什么（约 1 分钟）

**Phase 1 已经具备：**

从 Resource 加载 → Tick 演化 → Gateway 上报 → 命令回写 → Debug / 故障注入，以及 Gateway 侧的 simulator 出站路由。

**刻意留给 Phase 2 的：**

- Scenario Engine（脚本化场景编排）  
- 经 Kafka 动态增删设备  
- 模拟侧多协议（比如 MQTT）

但不影响今天就能用它做闭环演示和回归——主路径和主要异常路径已经覆盖。

---

## 收尾：它带来什么（约 1 分钟）

最后总结一下。有了 Simulator，团队在没有现场设备的前提下，仍然可以：

1. 用**真实服务边界**验证闭环；  
2. 用**有物理语义的状态机**产生可信遥测；  
3. 用**故障注入**覆盖异常故事；  
4. 让四个核心服务的协作，能被反复演示和回归。

它既是开发沙箱、联调底座，也是对外演示「虚拟电厂在软件侧如何跑起来」的活标本。

我的介绍就到这里。如果大家有兴趣，后面可以现场走一条幸福路径，或者注入一个 offline，看 Dispatch / Gateway 怎么表现。  
谢谢大家，欢迎提问。

---

## 附录 A：可选 Demo 串词（+3–5 分钟）

> 若时间紧，可跳过；若做 Demo，按此顺序讲即可。

**故事 1：幸福路径**

「我们先 seed 好资源和 mapping，启动 Simulator。  
大家看，它在 Tick，Telemetry 里已经能查到功率和 SOC。  
现在从 Dispatch 下发一个功率设定——下一拍遥测就会反映这个设定。  
这条链路没有旁路，走的就是生产同构路径。」

**故事 2：异常路径**

「我对这台设备注入 offline。  
再下发命令——会看到拒绝或超时。  
这就是我们验证失败路径的方式：不用改代码，也不用拔线。」

**故事 3：模型一致性（可选）**

「如果在 Resource 里改了 Point 或 CU，Simulator reload 之后，上报点表会跟着变。  
说明模拟侧没有第二套台账。」

---

## 附录 B：可能的提问与简短回答

**Q：为什么不直接 Mock Gateway / Telemetry？**  
A：Mock 测的是替身。我们要验证的是真实服务边界是否通，所以模拟放在 Gateway 外侧，路径同构。

**Q：为什么设备定义不放在 Simulator 自己配置里？**  
A：避免双份台账。Resource 是配置权威，Simulator 只负责「怎么跑」。

**Q：Dispatch 要不要改？**  
A：不用。Mapping 指到 `simulator` 即可，Dispatch 无感。

**Q：重启后状态还在吗？**  
A：Phase 1 以内存为主。重启后从 Resource reload；演示场景也可以 reset。持久化快照是后续可选优化。

**Q：能模拟多少设备？**  
A：当前按 Resource 中 `provider=simulator` 的 CU 加载，规模取决于机器与 Tick/上报频率；演示和联调场景足够。大规模压测不是 Phase 1 目标。

**Q：和真实 EMS 切换怎么切？**  
A：改 Gateway Mapping 的 `ExternalSystem`（以及对应外部 ID）。Simulator 与真实适配器通过路由共存，不必改 Dispatch。

---

## 附录 C：时间不够时的「3 分钟精简版」

各位好，一句话介绍 Simulator：  
**没有真设备时，让 VPP 平台仍能跑通真实闭环。**

它是有状态的虚拟设备运行时，不是 Mock。  
设备从 Resource 加载，遥测和控制都走 Gateway，Dispatch 无感。  
支持故障注入，方便验证异常路径。

记住这句分工：  
**Resource 管有什么，Simulator 管怎么跑，Gateway 管通道，Telemetry 管记录，Dispatch 管调度。**

Phase 1 主路径和故障注入已可用。谢谢。
