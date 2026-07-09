# Dispatch v2 领域设计与实现规划

> **升级说明**：本文档在 `plan_v1.md` 基础上全面修订。核心修正：GatewayPort 层次归位、补全 Kafka 反馈链路、明确 Sequential Action 执行机制、引入 TimeoutScanner、对齐现有服务的模块约定，并补充了 proto 设计、DB migration 和开发阶段建议。**v2.1 补充**：新增细粒度持久化接口（解决写放大）、熔断不干预策略（解决顺控中断"半吊子"状态）。

---

## 一、v1 → v2 变更概要

| 问题点 | v1 状态 | v2 修正 |
|--------|---------|---------|
| `GatewayPort` 返回 `GatewayAcceptanceStatus` | 放在 `domain/port`（集成语义混入领域层） | **移到 `application/port`** |
| Gateway proto 无 `CommandID` | 缺失，幂等和事件关联无法实现 | **新增 `CommandID` 字段** |
| Gateway proto `Value` 类型为 `double` | 无法表达布尔/整型/字符串控制值 | **改为 `oneof`，对齐 `CommandValue`** |
| Gateway → Dispatch 结果反馈 | 仅提到"事件通知"，未设计 | **完整 Kafka 链路设计**（topic、payload、consumer） |
| Sequential Action 等待机制 | 未说明命令 N+1 如何等命令 N 完成 | **事件驱动 continuation**（HandleCommandResult 推进） |
| Timeout 触发机制 | 有 `Timeout` 状态，未说明谁触发 | **Application 层 TimeoutScanner**（ticker 定时扫描） |
| 目录结构 | 仅 `domain/` 层级 | **完整结构**（含 go.mod、config、server 等，对齐 gateway） |
| `dispatch.proto` | 仅提及存在 | **完整定义** |
| Gateway 侧变更 | 未提及 | **明确改动清单** |
| DB migration | 未提及 | **补充表结构草图** |
| **持久化写放大**：每次 Command 状态变动更新整个 Task 聚合根 | 单一 `DispatchTaskRepository` + 全量 `Update(task)` | **拆分为三个独立 Repository**（`TaskRepository` / `ActionRepository` / `CommandRepository`），写频率显式分层；顺带支持 `FailurePolicy` 枚举化中断策略 |
| **顺控中断"半吊子"状态**：Action 1 成功执行后 Action 2 失败，物理设备已动作 | 未定义中断策略 | **熔断不干预**：立即熔断、剩余 Action/Command → `Cancelled`、Task → `Failed`、`PublishTaskFailed` 强告警，v1 明确拒绝自动补偿回滚 |

---

## 二、设计原则

- **Dispatch 负责调度执行，不负责协议转换。**
- **Gateway 负责控制协议适配，不假设外部系统交互方式。**
- **Dispatch 不关心 Gateway 如何实现（同步、异步、轮询、Telemetry 校验）。**
- **Dispatch 与 Gateway 保持同步 gRPC 调用（指令下发侧）。**
- **最终执行结果无法同步获得时，由 Gateway 通过 Kafka 事件通知 Dispatch。**
- **领域模型只表达业务状态，不表达通信细节。**
- **`GatewayAcceptanceStatus` 属于集成语义，放在 Application 层 port，而非 Domain 层。**
- **完全对齐现有服务的模块结构（`gateway` 为参照）和 `platform/` 公共工具。**
- **三拆 Repository**：按写频率将 `DispatchTaskRepository` 拆分为 `TaskRepository`（极少更新）、`ActionRepository`（偶尔更新）、`CommandRepository`（高频更新），聚合的业务一致性仍由 Domain Service 保证，Repository 负责性能优化。
- **顺控熔断不干预（Fail-Safe）**：顺控链路中任一命令失败，立即熔断后续执行，剩余实体置 `Cancelled`，Task 置 `Failed` 并发布告警事件；v1 不做自动补偿回滚，人工介入或下一调度周期的算法负责覆盖。

---

## 三、完整目录结构

```
internal/dispatch/
├── go.mod
├── go.sum
├── app.go                          # 服务启动入口（对齐 gateway/app.go）
├── run.go                          # cobra command 定义
├── server.go                       # composition root（createServer / PrepareRun / Run）
│
├── cmd/
│   └── main.go
│
├── config/
│   └── config.go                   # Config struct + viper binding
│
├── options/
│   └── options.go
│
├── domain/
│   ├── errors.go
│   ├── model/
│   │   ├── dispatch_task.go        # DispatchTask aggregate root
│   │   ├── dispatch_action.go      # DispatchAction entity
│   │   ├── control_command.go      # ControlCommand entity
│   │   ├── command_value.go        # CommandValue VO
│   │   ├── command_result.go       # CommandResult VO
│   │   └── enums.go
│   ├── port/
│   │   ├── task_repository.go      # TaskRepository（极低频写）
│   │   ├── action_repository.go    # ActionRepository（中频写）
│   │   ├── command_repository.go   # CommandRepository（高频写）
│   │   └── event_publisher.go
│   └── service/
│       ├── dispatcher.go           # 状态推进 + 命令选择
│       └── validator.go
│
├── application/
│   ├── app.go
│   ├── port/
│   │   └── gateway.go              # GatewayPort + GatewayExecuteResult（集成关切在此层）
│   ├── command/
│   │   ├── submit_task.go
│   │   ├── handle_command_result.go # Kafka consumer 驱动的核心处理器
│   │   └── scan_timeouts.go        # TimeoutScanner（独立 goroutine）
│   └── query/
│       └── get_task.go
│
├── adapter/
│   ├── inbound/
│   │   ├── grpc/
│   │   │   ├── server.go
│   │   │   ├── handler.go
│   │   │   └── errors.go
│   │   └── kafka/
│   │       └── command_result_consumer.go  # 消费 vpp.command.events
│   └── outbound/
│       ├── gateway_grpc/
│       │   └── client.go           # 实现 application/port.GatewayPort（Phase 5）
│       ├── postgres/
│       │   ├── task_repository.go     # 实现 domain/port.TaskRepository
│       │   ├── action_repository.go   # 实现 domain/port.ActionRepository
│       │   ├── command_repository.go  # 实现 domain/port.CommandRepository
│       │   └── converter.go           # domain ↔ GORM model
│       └── kafka/
│           └── event_publisher.go  # 实现 domain/port.TaskEventPublisher（Phase 5）
│
└── infrastructure/
    └── persistent/postgres/
        ├── db.go
        ├── models.go               # GORM 模型：dispatch_tasks / dispatch_actions / control_commands
        ├── task_repo.go            # raw GORM（事务写树、按 ID/CommandID 加载）
        ├── action_repo.go
        └── command_repo.go
```

---

## 四、领域模型

### 4.1 聚合关系

```
DispatchTask（Aggregate Root）
        │
        ▼
DispatchAction（Entity，嵌入聚合）
        │
        ▼
ControlCommand（Entity，嵌入聚合）
```

Action 不作为独立聚合，始终通过 Task 访问和持久化。

---

### 4.2 DispatchTask

```go
type DispatchTask struct {
    ID             string
    TenantID       string
    Name           string
    Description    string
    Type           TaskType
    TriggerType    TriggerType
    FailurePolicy  FailurePolicy   // v1 固定 FailFast；预留 Compensate / ManualIntervention
    Status         TaskStatus
    CreatedAt      time.Time
    StartedAt      *time.Time
    FinishedAt     *time.Time
    Actions        []*DispatchAction
}

// 状态转换方法
func (t *DispatchTask) Start(now time.Time) error
func (t *DispatchTask) Complete(now time.Time) error
func (t *DispatchTask) Fail(now time.Time) error
func (t *DispatchTask) Cancel(now time.Time) error

// 查询辅助
func (t *DispatchTask) NextPendingAction() *DispatchAction  // 按 Sequence 升序，返回第一个 Pending
func (t *DispatchTask) IsFinished() bool
func (t *DispatchTask) FindCommand(commandID string) (*DispatchAction, *ControlCommand)
```

---

### 4.3 DispatchAction

```go
type DispatchAction struct {
    ID              string
    TaskID          string
    TenantID        string
    Name            string
    Type            ActionType
    Sequence        int
    Status          ActionStatus
    ExecutionPolicy ExecutionPolicy
    Commands        []*ControlCommand
}

// 状态转换方法
func (a *DispatchAction) Start() error
func (a *DispatchAction) Complete() error
func (a *DispatchAction) Fail() error

// 执行调度辅助
func (a *DispatchAction) CommandsToDispatch() []*ControlCommand
// Sequential：返回第一条 Pending 命令（仅当无 Sending 命令时）
// Parallel：返回全部 Pending 命令

func (a *DispatchAction) AllCommandsFinished() bool
func (a *DispatchAction) AnyCommandFailed() bool
func (a *DispatchAction) Cancel() error                                        // 熔断用：Running/Pending → Cancelled
func (a *DispatchAction) CancelPendingCommands()                               // 熔断用：将所有 Pending 命令置 Cancelled
```

---

### 4.4 ControlCommand

```go
type ControlCommand struct {
    ID         string          // 全局唯一 CommandID（UUID v7），贯穿幂等/回调/追踪
    ActionID   string
    TenantID   string
    CUCode     string
    PointKey   string          // 对应 gateway proto ExecuteCommandRequest.PointKey

    Value      CommandValue

    Status     CommandStatus
    RetryCount int
    MaxRetries int             // 最大重试次数（默认 3）
    Timeout    time.Duration   // 等待结果超时（默认 30s）

    SentAt     *time.Time
    DeadlineAt *time.Time      // = SentAt + Timeout，供 TimeoutScanner 查询
    FinishedAt *time.Time

    Result     *CommandResult
}

// 状态转换方法
func (c *ControlCommand) MarkSending(sentAt time.Time) error    // Pending → Sending
func (c *ControlCommand) MarkSucceeded(result *CommandResult) error
func (c *ControlCommand) MarkFailed(result *CommandResult) error
func (c *ControlCommand) MarkTimeout() error                    // Sending → Timeout
func (c *ControlCommand) ResetForRetry() error                  // Timeout → Pending，RetryCount++

// 查询辅助
func (c *ControlCommand) IsExpired(now time.Time) bool          // DeadlineAt != nil && DeadlineAt.Before(now)
func (c *ControlCommand) CanRetry() bool                        // RetryCount < MaxRetries
func (c *ControlCommand) Cancel() error                         // Pending → Cancelled（熔断时由 dispatcher 调用）
```

> **为什么 CommandID 是 UUID v7？**
> 对齐项目现有约定（`platform/idgen`），UUID v7 单调递增，天然支持按时间排序，且全局唯一，满足幂等键要求。

---

### 4.5 CommandValue（VO）

```go
type CommandValue struct {
    BoolValue   *bool
    IntValue    *int64
    FloatValue  *float64
    StringValue *string
}

func (v CommandValue) Validate() error   // 恰好一个字段非 nil
func (v CommandValue) Kind() string      // "bool" / "int" / "float" / "string"
```

---

### 4.6 CommandResult（VO）

```go
type CommandResult struct {
    Success      bool
    ErrorCode    string
    ErrorMessage string
    AckAt        *time.Time
}
```

---

## 五、状态机

### 5.1 Task

```
Pending ──▶ Running ──▶ Completed
               │
               ├──▶ Failed
               │
               └──▶ Cancelled（任意时刻可触发）
```

### 5.2 Action

```
Pending ──▶ Running ──▶ Completed
    │          │
    │          └──▶ Failed
    │
    └──▶ Cancelled（熔断时由 Dispatcher 直接设置，跳过 Running）
```

说明：当上游 Action 失败触发熔断时，所有尚未开始的下游 Action 直接从 `Pending` 跳转到 `Cancelled`，不经过 `Running`。

### 5.3 Command

```
                      ┌─ CanRetry() ─▶ Pending（RetryCount++）
                      │
Pending ──▶ Sending ──┼──▶ Succeeded
               │      │
               │      └──▶ Failed（不可重试的 Timeout 或明确失败）
               │
               └──▶ Timeout（由 TimeoutScanner 触发）
```

注意：
- **没有 `Accepted` 状态**（这是通信状态，不是领域状态）
- **`Sending` = 已发给 Gateway，等待最终结果（同步或异步 Kafka 回调）**
- **`Timeout` 不是终态**：若 `CanRetry()`，重置为 `Pending` 并递增 `RetryCount`
- **`Cancelled` = 熔断时被动中止**：上游命令失败时，同 Action 内所有 `Pending` 命令及后续 Action 的全部命令均置 `Cancelled`，不再下发

---

## 六、Domain Port（纯业务端口）

v2 将 `GatewayPort` 移出 domain/port，此层只保留纯业务关切。

> **v2.1 Repository 重构**：按写频率拆分为三个独立接口。聚合的业务一致性仍由 Domain Service（`Dispatcher`）在内存中保证，Repository 仅负责精确持久化已发生变化的层级。
>
> | Repository | 对应表 | 写频率 | 典型场景 |
> |---|---|---|---|
> | `TaskRepository` | `dispatch_tasks` | **极低** | 任务创建、开始、完成/失败 |
> | `ActionRepository` | `dispatch_actions` | **中** | Action 状态推进（Running/Completed/Failed/Cancelled） |
> | `CommandRepository` | `control_commands` | **极高** | 每条 Kafka 回调触发一次 Update |

### 6.1 TaskRepository

```go
// domain/port/task_repository.go
type TaskRepository interface {
    // Save 一次性持久化完整 Task 树（含所有 Action/Command），仅在任务创建时调用一次
    Save(ctx context.Context, task *model.DispatchTask) error

    // Update 仅更新 dispatch_tasks 表本身的字段（status / started_at / finished_at），
    // 不触及 Actions/Commands
    Update(ctx context.Context, task *model.DispatchTask) error

    // FindByID 加载完整 Task 树（含 Actions/Commands），供 Domain Service 使用
    FindByID(ctx context.Context, id string) (*model.DispatchTask, error)

    // FindByCommandID 通过任意 CommandID 反查其宿主 Task（完整树），
    // 供 HandleCommandResult / TimeoutScanner 使用
    FindByCommandID(ctx context.Context, commandID string) (*model.DispatchTask, error)
}
```

### 6.2 ActionRepository

```go
// domain/port/action_repository.go
type ActionRepository interface {
    // Update 仅更新 dispatch_actions 表的 status 字段
    Update(ctx context.Context, action *model.DispatchAction) error
}
```

### 6.3 CommandRepository

```go
// domain/port/command_repository.go
type CommandRepository interface {
    // Update 更新 control_commands 表的运行时字段：
    // status / retry_count / sent_at / deadline_at / finished_at / result
    // 这是整个 Dispatch 系统中写频率最高的操作
    Update(ctx context.Context, cmd *model.ControlCommand) error

    // FindExpiredSending 供 TimeoutScanner 使用，仅返回轻量 Command 对象（无需完整 Task 树）
    // 等价于: SELECT * FROM control_commands WHERE status='sending' AND deadline_at < before
    FindExpiredSending(ctx context.Context, before time.Time) ([]*model.ControlCommand, error)
}
```

### 6.4 TaskEventPublisher

```go
type TaskEventPublisher interface {
    PublishTaskStarted(ctx context.Context, task *model.DispatchTask) error
    PublishTaskCompleted(ctx context.Context, task *model.DispatchTask) error
    PublishTaskFailed(ctx context.Context, task *model.DispatchTask) error
}
```

> **说明**：`Clock` 和 `IDGenerator` 不作为 `domain/port` 接口。  
> 对齐项目现有约定（gateway / resource 均如此）：
> - 时间戳：domain model 方法内直接调用 `time.Now()`
> - ID 生成：Application 层调用 `platform/idgen.Must()` 生成后传入 domain model

---

## 七、Application Port（集成关切）

> **v2 核心修正**：`GatewayPort` 及其关联类型放在 `application/port/`，而非 `domain/port/`。
>
> 原因：`Accepted / Completed / Rejected` 描述的是与 Gateway 通信的结果，是集成层语义，
> 领域模型只需要关心 `ControlCommand` 当前是 `Sending`、`Succeeded` 还是 `Failed`。

```go
// application/port/gateway.go

type GatewayAcceptanceStatus int

const (
    GatewayAccepted  GatewayAcceptanceStatus = iota // 已接收，最终结果待 Kafka 回调
    GatewayCompleted                                 // 同步完成，结果已知
    GatewayRejected                                  // 拒绝（CU 不存在 / 参数非法 / 映射禁用）
)

type GatewayExecuteResult struct {
    Status  GatewayAcceptanceStatus
    Success bool    // 当 Status == GatewayCompleted 时有效
    Message string
}

type GatewayPort interface {
    ExecuteCommand(ctx context.Context, cmd *model.ControlCommand) (*GatewayExecuteResult, error)
}
```

Application 层（`execute_task`、`handle_command_result`）在调用 `GatewayPort` 后，
根据 `GatewayAcceptanceStatus` 驱动领域状态机：

| GatewayAcceptanceStatus | 推进动作 |
|-------------------------|---------|
| `GatewayCompleted` | `cmd.MarkSucceeded(result)` 或 `cmd.MarkFailed(result)` |
| `GatewayAccepted` | `cmd.MarkSending(now)`，等待 Kafka 回调 |
| `GatewayRejected` | `cmd.MarkFailed(result)`，终止 Action/Task |

---

## 八、Domain Service

### 8.1 Dispatcher

```go
// domain/service/dispatcher.go
//
// Dispatcher 编排 Task → Action → Command 的状态推进和命令选择。
// 不持有 GatewayPort（在 Application 层持有），不做 I/O，纯领域逻辑。

type Dispatcher struct{}

// PrepareTask 将 Task 推进到 Running。ID 已由 Application 层在构建 DispatchTask 时通过
// platform/idgen.Must() 分配完毕，此方法只做状态校验和 StartedAt 设置。
func (d *Dispatcher) PrepareTask(task *model.DispatchTask) error

// CommandsToDispatch 返回当前 Action 中应立即下发的命令：
//   Sequential：仅当无 Sending 命令时，返回第一条 Pending 命令
//   Parallel：返回全部 Pending 命令
func (d *Dispatcher) CommandsToDispatch(action *model.DispatchAction) []*model.ControlCommand

// OnCommandResult 处理命令结果，推进 Command / Action / Task 状态。
//
// 成功路径（FailurePolicy 不影响此路径）：
//   Command → Succeeded
//   若 Sequential：nextCommands = 下一条 Pending 命令
//   若 Action 全部完成：Action → Completed，推进下一个 Action
//   若所有 Action Completed：Task → Completed，outcome.TaskFinished = true
//
// 失败/熔断路径（v1 FailurePolicy = FailFast）：
//   Command → Failed
//   同 Action 内所有 Pending 命令 → Cancelled
//   当前 Action → Failed
//   后续所有 Pending Action（及其命令）→ Cancelled
//   Task → Failed，outcome.TaskFinished = true
//
// 返回 CommandResultOutcome（结构体，比多返回值更易扩展）
func (d *Dispatcher) OnCommandResult(
    task *model.DispatchTask,
    commandID string,
    result *model.CommandResult,
) (*CommandResultOutcome, error)

// CommandResultOutcome 封装 OnCommandResult / OnCommandTimeout 的所有输出，
// 供 Application 层精确驱动三个 Repository 的更新。
type CommandResultOutcome struct {
    // 状态已变化的 Command 列表（Application → commandRepo.Update 每一条）
    ChangedCommands []*model.ControlCommand
    // 状态已变化的 Action 列表（Application → actionRepo.Update 每一条）
    ChangedActions  []*model.DispatchAction
    // Task 状态是否发生变化（Application → taskRepo.Update）
    TaskChanged     bool
    // Sequential continuation：需要立即下发的下一批命令（失败路径下为 nil）
    NextCommands    []*model.ControlCommand
    // Task 是否已终结（Completed 或 Failed）
    TaskFinished    bool
}

// OnCommandTimeout 处理命令超时：
//   可重试（RetryCount < MaxRetries）→ Command 重置为 Pending，outcome.NextCommands = [cmd]，不触发熔断
//   不可重试 → 同 OnCommandResult 失败路径，触发熔断
func (d *Dispatcher) OnCommandTimeout(
    task *model.DispatchTask,
    commandID string,
) (*CommandResultOutcome, error)
```

### 8.2 Validator

```go
// domain/service/validator.go
func (v *Validator) ValidateTask(task *model.DispatchTask) error
func (v *Validator) ValidateAction(action *model.DispatchAction) error
func (v *Validator) ValidateCommand(cmd *model.ControlCommand) error
```

---

## 八·补、熔断不干预策略（Fail-Safe）

> 这是 Dispatch v2 在能源行业场景下的核心安全约定，直接影响 `dispatcher.go` 和 `handle_command_result.go` 的实现。

### 问题背景

顺控链路（Sequential Actions）中，物理操作具有惯性：

```
Task
  Action 1（储能A放电 200kW）  → 执行成功，设备已动作
  Action 2（储能B充电 100kW）  → Rejected / Timeout / Failed
```

此时系统进入"半吊子"状态：储能A已在放电，储能B未响应，控制意图只完成了一半。

### v1 决策：挂起不干预（策略一）

**强制约定：v1 不编写任何自动补偿（reverse compensation）或回滚（rollback）代码。**

理由：
1. **物理安全风险**：能源/电力场景中，错误的自动反向指令可能造成二次冲击。工控首要原则 —— 不确定安全时，保持现状比盲目动作更安全。
2. **算法周期覆盖**：VPP 优化算法通常每 5~15 分钟重新计算一次控制基线。当前周期失败后，下一周期算法会基于最新 Telemetry 数据重新下发全量控制指令，自然完成"动态补偿"。
3. **实现复杂度**：补偿逻辑需要知道"如何逆操作每种设备控制指令"，这属于业务语义，v1 不具备此能力。

### 熔断行为规范（Dispatcher 实现约定）

当 `OnCommandResult` 或 `OnCommandTimeout` 进入失败路径时，`Dispatcher` **必须**：

```
1. 将失败的 Command 标记 Failed
2. 将同 Action 内所有状态为 Pending 的命令 → Cancelled
   （已处于 Sending 的命令不干预，等其自然超时）
3. 将当前 Action 标记 Failed
4. 将 Task 中所有后续 Pending Action → Cancelled（依次调用 action.Cancel() + action.CancelPendingCommands()）
5. 将 Task 标记 Failed
6. 返回 taskFinished = true
```

**不做的事**：
- 不下发任何反向控制命令
- 不修改已成功执行的 Action/Command 状态（保留现场）
- 不重试失败的 Action（重试只限于单条 Command 级别，且受 MaxRetries 约束）

### Application 层响应（handle_command_result.go）

熔断后，Application 层通过 `PublishTaskFailed` 将事件推送到 `vpp.dispatch.events`（或直接 `vpp.task.events`），由下游告警服务（Alarm Service，v1 规划中）消费，向运维人员发送高优先级告警：

```
租户 X / 站点 Y
调度任务 [task-id] 执行中断
已完成：Action 1（储能A放电 200kW）✅
失败：Action 2（储能B充电 100kW）— Gateway Timeout
已取消：Action 3 ~ N
请人工核查设备状态并决策。
```

### enums.go 补充

```go
type ActionStatus int
const (
    ActionPending   ActionStatus = iota
    ActionRunning
    ActionCompleted
    ActionFailed
    ActionCancelled  // 熔断时，未执行的 Action 置此状态
)

type CommandStatus int
const (
    CommandPending   CommandStatus = iota
    CommandSending
    CommandSucceeded
    CommandFailed
    CommandTimeout
    CommandCancelled  // 熔断时，未发出的 Command 置此状态
)

// FailurePolicy 决定顺控链路中断时的行为策略（显式枚举，预留扩展点）
type FailurePolicy int
const (
    // FailFast（v1 唯一策略）：任一 Command/Action 失败立即熔断，剩余置 Cancelled，
    // Task → Failed，发布告警事件，不做任何自动补偿。
    FailFast FailurePolicy = iota

    // 以下为未来预留，v1 不实现：
    // Compensate  — 自动执行反向控制命令（需要业务语义支持"逆操作"）
    // ManualIntervention — 暂停任务，等待人工确认后继续或终止
)
```

> **为什么要显式定义 FailurePolicy？**
> 即使 v1 只有 `FailFast`，明确定义枚举有两个好处：
> 1. `DispatchTask.FailurePolicy` 字段使运维人员（通过 GetTask API）能直观看到"该任务采用何种中断策略"；
> 2. 当业务真的需要 Saga 补偿时，只需在 `Dispatcher.OnCommandResult` 的 switch 分支里增加 `case Compensate` 即可，不需要重构接口。

---

## 九、Application Layer

### 9.1 SubmitTask

```go
type SubmitTask struct {
    TenantID    string
    Name        string
    Description string
    Type        model.TaskType
    TriggerType model.TriggerType
    Actions     []SubmitActionDTO
}

type SubmitActionDTO struct {
    Name            string
    Type            model.ActionType
    Sequence        int
    ExecutionPolicy model.ExecutionPolicy
    Commands        []SubmitCommandDTO
}

type SubmitCommandDTO struct {
    CUCode     string
    PointKey   string
    Value      model.CommandValue
    Timeout    time.Duration  // 0 → 使用配置默认值
    MaxRetries int            // 0 → 使用配置默认值
}
```

Handler 职责：
1. 构建 `DispatchTask`（含 `FailurePolicy = FailFast`），在 Application 层调用 `platform/idgen.Must()` 为 Task/Action/Command 分配 ID
2. `validator.ValidateTask(task)`
3. `taskRepo.Save(task)`（一次性写入全部 Actions/Commands，状态 Pending）
4. 同步驱动 `executeFirstBatch(task)`（Pending → Running，发出第一批命令）
5. 持久化初始下发状态（`commandRepo.Update` 更新 Sending 的命令，`actionRepo.Update` 更新 Running 的 Action，`taskRepo.Update` 更新 Running 的 Task）
6. 返回 `TaskID`

### 9.2 HandleCommandResult（核心处理器）

此 handler 是整个异步执行路径的核心，由 Kafka consumer 驱动。

```go
type HandleCommandResult struct {
    CommandID string
    Result    *model.CommandResult
}
```

Handler 职责：
1. `taskRepo.FindByCommandID(ctx, cmd.CommandID)` → 加载完整 `DispatchTask`（含 Actions/Commands）
2. **幂等检查**：若 Command 已处于终态（`Succeeded` / `Failed` / `Cancelled`），直接返回
3. `outcome, _ := dispatcher.OnCommandResult(task, cmd.CommandID, result)`
4. **按三 Repository 精确持久化**（outcome 驱动，只更新真正变化的行）：
   ```go
   // CommandRepository：最高频，每次 Kafka 回调必调
   for _, cmd := range outcome.ChangedCommands {
       commandRepo.Update(ctx, cmd)
   }
   // ActionRepository：仅 Action 状态实际变化时调用
   for _, action := range outcome.ChangedActions {
       actionRepo.Update(ctx, action)
   }
   // TaskRepository：仅 Task 状态实际变化时调用（极少）
   if outcome.TaskChanged {
       taskRepo.Update(ctx, task)
   }
   ```
5. 对于 `outcome.NextCommands`（Sequential continuation）：
   - `gatewayPort.ExecuteCommand(ctx, c)` 并按返回状态推进 Command
   - `commandRepo.Update(ctx, c)` 持久化新的 `Sending` 状态
6. 若 `outcome.TaskFinished`：
   - `eventPublisher.PublishTaskCompleted(ctx, task)` 或 `PublishTaskFailed(ctx, task)`

> **正常路径的真实写开销**（绝大多数情况）：
> ```
> 1 × UPDATE control_commands SET status='succeeded', finished_at=... WHERE id='cmd-xxx'
> ```
> Action/Task 表完全不动。仅在 Action/Task 状态确实改变时，才额外各写一行。

### 9.3 TimeoutScanner（独立 goroutine）

```go
// application/command/scan_timeouts.go
type TimeoutScanner struct {
    taskRepo    port.TaskRepository
    actionRepo  port.ActionRepository
    commandRepo port.CommandRepository
    dispatcher  *service.Dispatcher
    gateway     appport.GatewayPort
    publisher   port.TaskEventPublisher
    interval    time.Duration
}

func (s *TimeoutScanner) Run(ctx context.Context) error {
    ticker := time.NewTicker(s.interval)
    defer ticker.Stop()
    for {
        select {
        case <-ticker.C:
            s.scanOnce(ctx)
        case <-ctx.Done():
            return nil
        }
    }
}

func (s *TimeoutScanner) scanOnce(ctx context.Context) {
    expired, _ := s.commandRepo.FindExpiredSending(ctx, time.Now())
    for _, cmd := range expired {
        task, _ := s.taskRepo.FindByCommandID(ctx, cmd.ID)
        outcome, _ := s.dispatcher.OnCommandTimeout(task, cmd.ID)
        // 按三 Repository 精确持久化
        for _, c := range outcome.ChangedCommands {
            s.commandRepo.Update(ctx, c)
        }
        for _, a := range outcome.ChangedActions {
            s.actionRepo.Update(ctx, a)
        }
        if outcome.TaskChanged {
            s.taskRepo.Update(ctx, task)
        }
        if outcome.TaskFinished {
            s.publisher.PublishTaskFailed(ctx, task)
        }
        // 重试路径（CanRetry）：发送下一条命令并持久化 Sending 状态
        for _, c := range outcome.NextCommands {
            s.gateway.ExecuteCommand(ctx, c)
            s.commandRepo.Update(ctx, c)
        }
    }
}
```

`server.go` 中与 gRPC / Kafka consumer 并列启动：

```go
eg.Go(func() error {
    return s.timeoutScanner.Run(egCtx)
})
```

### 9.4 GetTask（Query）

```go
type GetTask struct {
    TenantID string
    TaskID   string
}
// 返回 DispatchTask 含 Actions/Commands 快照，供管理端查询
```

---

## 十、Gateway → Dispatch 反馈链路（Kafka）

### 10.1 新 Kafka topic

```
vpp.command.events
```

| 属性 | 值 |
|------|----|
| 生产方 | gateway |
| 消费方 | dispatch |
| 格式 | `platform/event.Envelope[CommandCompletedPayload]` |
| Group ID | `vpp-dispatch-command-events` |

### 10.2 CommandCompletedPayload

新增到 `platform/event/gateway/events.go`（对齐现有 `platform/event/resource/` 结构）：

```go
const TypeCommandCompleted = "command.completed"

type CommandCompletedPayload struct {
    TenantID     string     `json:"tenant_id"`
    CommandID    string     `json:"command_id"`   // 与 ControlCommand.ID 一一对应
    CUCode       string     `json:"cu_code"`
    Success      bool       `json:"success"`
    ErrorCode    string     `json:"error_code,omitempty"`
    ErrorMessage string     `json:"error_message,omitempty"`
    AckAt        *time.Time `json:"ack_at,omitempty"`
}
```

### 10.3 Dispatch Kafka Consumer

```go
// adapter/inbound/kafka/command_result_consumer.go
type CommandResultConsumer struct {
    cfg     CommandResultConsumerConfig
    reader  *kafka.Reader
    handler command.HandleCommandResultHandler
}

// 消费 vpp.command.events
// 解析 Envelope[CommandCompletedPayload]
// 调用 handler.Handle(ctx, HandleCommandResult{...})
// at-least-once：handler 通过幂等检查保证安全重试
```

---

## 十一、Gateway 侧变更清单

为支持 Dispatch v2，Gateway 服务需要同步修改：

| 修改位置 | 内容 |
|----------|------|
| `api/gateway/proto/gateway.proto` | 新增 `CommandID` 字段；`Command` 重命名为 `PointKey`；`double Value` 升级为 `oneof value` |
| `domain/port/ems_client.go` | `SendCommand` 签名加入 `commandID string` 参数 |
| `application/command/execute_command.go` | 将 `CommandID` 传给 `emsClient.SendCommand` |
| `domain/port/` | 新增 `CommandEventPublisher` port |
| `adapter/outbound/kafka/` | 新增 `CommandEventPublisher` 实现，发布到 `vpp.command.events` |
| `config/config.go` | 新增 Kafka producer 配置 |
| `server.go` | 注入并启动 `CommandEventPublisher` |

> **开发策略**：gateway + dispatch 初版在同一迭代内完成，避免 proto breaking change 导致中间态不可用。

---

## 十二、Proto 设计

### 12.1 api/gateway/proto/gateway.proto（更新）

```protobuf
syntax = "proto3";
package gatewaypb;
option go_package = "github.com/mushroomyuan/vpp-backend/api/gateway/proto/gen;gatewaypb";

service GatewayService {
  rpc ExecuteCommand(ExecuteCommandRequest) returns (ExecuteCommandResponse);
}

message ExecuteCommandRequest {
  string command_id  = 1;  // dispatch 分配，用于幂等和 Kafka 回调关联
  string tenant_id   = 2;
  string cu_code     = 3;
  string point_key   = 4;  // 原 Command 字段重命名
  oneof value {
    bool   bool_value   = 5;
    int64  int_value    = 6;
    double float_value  = 7;
    string string_value = 8;
  }
}

message ExecuteCommandResponse {
  string external_id     = 1;
  string external_system = 2;
  // 同步返回仅表示"已接受/已完成"，最终结果通过 vpp.command.events 回调
  // 不在 proto 里暴露 GatewayAcceptanceStatus（避免内部 DTO 泄漏到 proto 契约）
}
```

### 12.2 api/dispatch/proto/dispatch.proto（新增）

```protobuf
syntax = "proto3";
package dispatchpb;
option go_package = "github.com/mushroomyuan/vpp-backend/api/dispatch/proto/gen;dispatchpb";

service DispatchService {
  rpc SubmitTask(SubmitTaskRequest) returns (SubmitTaskResponse);
  rpc GetTask(GetTaskRequest)       returns (GetTaskResponse);
  rpc CancelTask(CancelTaskRequest) returns (CancelTaskResponse);
}

// SubmitTask
message SubmitTaskRequest {
  string tenant_id    = 1;
  string name         = 2;
  string description  = 3;
  string task_type    = 4;    // "control"
  string trigger_type = 5;    // "manual" / "scheduled" / "automatic"
  repeated ActionSpec actions = 6;
}
message ActionSpec {
  string name             = 1;
  string action_type      = 2;
  int32  sequence         = 3;
  string execution_policy = 4;  // "sequential" / "parallel"
  repeated CommandSpec commands = 5;
}
message CommandSpec {
  string cu_code          = 1;
  string point_key        = 2;
  oneof value {
    bool   bool_value   = 3;
    int64  int_value    = 4;
    double float_value  = 5;
    string string_value = 6;
  }
  int32 timeout_seconds = 7;  // 0 → 使用服务默认值（30s）
  int32 max_retries     = 8;  // 0 → 使用服务默认值（3）
}
message SubmitTaskResponse {
  string task_id = 1;
  string status  = 2;  // "running"
}

// GetTask
message GetTaskRequest {
  string tenant_id = 1;
  string task_id   = 2;
}
message GetTaskResponse {
  string task_id  = 1;
  string status   = 2;
  repeated ActionStatusMsg actions = 3;
}
message ActionStatusMsg {
  string action_id = 1;
  string name      = 2;
  string status    = 3;
  repeated CommandStatusMsg commands = 4;
}
message CommandStatusMsg {
  string command_id = 1;
  string cu_code    = 2;
  string point_key  = 3;
  string status     = 4;
  string error_code = 5;
}

// CancelTask
message CancelTaskRequest {
  string tenant_id = 1;
  string task_id   = 2;
}
message CancelTaskResponse {
  bool success = 1;
}
```

---

## 十三、Sequential Action 执行机制

Sequential Action 的核心挑战：命令 N+1 必须等命令 N 完成后才能发送。

### 路径一：Gateway 同步返回 Completed

```
SubmitTask
  → CommandsToDispatch(action) = [Command1]（Sequential，无 Sending）
  → gatewayPort.ExecuteCommand(Command1) → GatewayCompleted
  → dispatcher.OnCommandResult(task, Command1.ID, success)
      → Command1 = Succeeded
      → nextCommands = [Command2]
  → gatewayPort.ExecuteCommand(Command2)
  → ...（同一调用栈内完成，整个 Action 同步结束）
```

### 路径二：Gateway 返回 Accepted（异步路径）

```
SubmitTask
  → CommandsToDispatch(action) = [Command1]
  → gatewayPort.ExecuteCommand(Command1) → GatewayAccepted
  → Command1 = Sending，DeadlineAt = now + 30s
  → repo.Save(task)
  → 返回 TaskID 给调用方（此时 Task = Running）

[稍后，Gateway 发布 Kafka 事件]
vpp.command.events → CommandResultConsumer
  → HandleCommandResult{CommandID: Command1.ID, Result: success}
      → dispatcher.OnCommandResult(task, Command1.ID, success)
          → Command1 = Succeeded
          → nextCommands = [Command2]（Sequential continuation）
      → gatewayPort.ExecuteCommand(Command2)
      → ...

  [Action1 全部完成]
      → Action1 = Completed
      → NextPendingAction() = Action[seq=2]
      → executeFirstBatch(Action2)

  [全部 Action 完成]
      → Task = Completed
      → PublishTaskCompleted(task)
      → repo.Update(task)
```

### Parallel Action

```
SubmitTask
  → CommandsToDispatch(action) = [Cmd1, Cmd2, Cmd3]（全部 Pending）
  → 并发调用 gatewayPort.ExecuteCommand × 3
  → 所有命令 = Sending

[3 个 Kafka 事件依次到来]
  → 每次 HandleCommandResult：
      → 更新对应命令状态
      → AllCommandsFinished()? → 最后一条完成时 → Action = Completed
```

---

## 十四、数据库 Migration 草图

```sql
-- migrations/dispatch/000001_init.up.sql
-- initdb: migrations/initdb/50-dispatch-db.sh + 51-dispatch-schema.sql

CREATE TABLE dispatch_tasks (
    id             TEXT        PRIMARY KEY,
    tenant_id      TEXT        NOT NULL,
    name           TEXT        NOT NULL,
    description    TEXT,
    type           TEXT        NOT NULL,
    trigger_type   TEXT        NOT NULL,
    failure_policy TEXT        NOT NULL DEFAULT 'fail_fast',
    status         TEXT        NOT NULL DEFAULT 'pending',
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    started_at     TIMESTAMPTZ,
    finished_at    TIMESTAMPTZ
);

CREATE INDEX idx_dispatch_tasks_tenant_status ON dispatch_tasks (tenant_id, status);

-- ---

CREATE TABLE dispatch_actions (
    id               TEXT PRIMARY KEY,
    task_id          TEXT NOT NULL REFERENCES dispatch_tasks(id),
    tenant_id        TEXT NOT NULL,
    name             TEXT NOT NULL,
    type             TEXT NOT NULL,
    sequence         INT  NOT NULL,
    status           TEXT NOT NULL DEFAULT 'pending',
    execution_policy TEXT NOT NULL DEFAULT 'sequential'
);

CREATE INDEX idx_dispatch_actions_task_id ON dispatch_actions (task_id);

-- ---

CREATE TABLE control_commands (
    id          TEXT        PRIMARY KEY,
    action_id   TEXT        NOT NULL REFERENCES dispatch_actions(id),
    tenant_id   TEXT        NOT NULL,
    sequence    INT         NOT NULL,       -- 同 Action 内稳定顺序（Sequential 依赖）
    cu_code     TEXT        NOT NULL,
    point_key   TEXT        NOT NULL,
    value       JSONB       NOT NULL,       -- {"kind":"bool|int|float|string", ...}
    status      TEXT        NOT NULL DEFAULT 'pending',
    retry_count INT         NOT NULL DEFAULT 0,
    max_retries INT         NOT NULL DEFAULT 3,
    timeout_ms  BIGINT      NOT NULL DEFAULT 30000,
    sent_at     TIMESTAMPTZ,
    deadline_at TIMESTAMPTZ,                -- TimeoutScanner 扫描字段
    finished_at TIMESTAMPTZ,
    result      JSONB                       -- {"success":bool,"error_code":"","error_message":"","ack_at":"..."}
);

CREATE INDEX idx_control_commands_action_id ON control_commands (action_id);

-- TimeoutScanner 专用：仅扫描 Sending 状态的命令，partial index 减少扫描范围
CREATE INDEX idx_control_commands_timeout_scan
    ON control_commands (deadline_at)
    WHERE status = 'sending';
```

Postgres 数据库命名：对齐现有约定，新建 `dispatch` 库（参考 `migrations/initdb/50-dispatch-db.sh`）。

---

## 十五、Module 结构（go.mod）

```
module github.com/mushroomyuan/vpp-backend/dispatch

go 1.26.4

replace (
    github.com/mushroomyuan/vpp-backend/api/dispatch/proto/gen => ../../api/dispatch/proto/gen
    github.com/mushroomyuan/vpp-backend/api/gateway/proto/gen  => ../../api/gateway/proto/gen
    github.com/mushroomyuan/vpp-backend/platform               => ../platform
)

require (
    github.com/mushroomyuan/vpp-backend/api/dispatch/proto/gen v0.0.0-00010101000000-000000000000
    github.com/mushroomyuan/vpp-backend/api/gateway/proto/gen  v0.0.0-00010101000000-000000000000
    github.com/mushroomyuan/vpp-backend/platform               v0.0.0-00010101000000-000000000000
    github.com/jackc/pgx/v5          v5.6.0
    github.com/segmentio/kafka-go    ...
    github.com/sirupsen/logrus       v1.9.3
    github.com/spf13/cobra           v1.10.2
    github.com/spf13/viper           v1.21.0
    golang.org/x/sync                v0.20.0
    google.golang.org/grpc           v1.80.0
    google.golang.org/protobuf       v1.36.11
    gorm.io/gorm                     v1.31.1
    gorm.io/driver/postgres          ...
    // OpenTelemetry、Consul、Prometheus 对齐 gateway go.mod 的间接依赖
)
```

---

## 十六、Config 结构

```go
// config/config.go
type Config struct {
    ServiceName string `mapstructure:"service-name"`
    GRPCAddr    string `mapstructure:"grpc-addr"`
    MetricsAddr string `mapstructure:"metrics-addr"`

    Database DatabaseConfig `mapstructure:"database"`
    Gateway  GatewayConfig  `mapstructure:"gateway"`
    Kafka    KafkaConfig    `mapstructure:"kafka"`
    Dispatch DispatchConfig `mapstructure:"dispatch"`
}

type GatewayConfig struct {
    GRPCAddr string `mapstructure:"grpc-addr"`  // 127.0.0.1:5005
}

type KafkaConfig struct {
    Brokers        []string `mapstructure:"brokers"`
    CommandTopic   string   `mapstructure:"command-topic"`  // vpp.command.events（消费）
    GroupID        string   `mapstructure:"group-id"`
}

type DispatchConfig struct {
    TimeoutScanInterval    time.Duration `mapstructure:"timeout-scan-interval"`     // 默认 10s
    DefaultCommandTimeout  time.Duration `mapstructure:"default-command-timeout"`   // 默认 30s
    DefaultMaxRetries      int           `mapstructure:"default-max-retries"`       // 默认 3
}
```

对应 `config/dispatch.yaml`：

```yaml
service-name: vpp-dispatch
grpc-addr:    "127.0.0.1:5006"
metrics-addr: "127.0.0.1:9105"

database:
  dsn: "host=127.0.0.1 user=postgres password=postgres dbname=dispatch port=5432 sslmode=disable"

gateway:
  grpc-addr: "127.0.0.1:5005"

kafka:
  brokers: []               # 空 = no-op（对齐 gateway 现有降级设计）
  command-topic: "vpp.command.events"
  group-id: "vpp-dispatch-command-events"

dispatch:
  timeout-scan-interval:   10s
  default-command-timeout: 30s
  default-max-retries:     3
```

---

## 十七、职责边界总表

|  | Dispatch | Gateway |
|---|---|---|
| 调度任务生命周期（Task / Action / Command 状态机） | ✅ | ❌ |
| Action 顺控 / 并发编排 | ✅ | ❌ |
| Retry 策略 + Timeout 扫描 | ✅ | ❌ |
| Task 完成 / 失败判定 | ✅ | ❌ |
| 协议转换（PointKey → 外部格式） | ❌ | ✅ |
| CUCode → ExternalID 映射 | ❌ | ✅ |
| 与 EMS / 设备通信 | ❌ | ✅ |
| 适配同步 / 异步 / 轮询等外部交互 | ❌ | ✅ |
| 发布命令执行事件（`vpp.command.events`） | ❌ | ✅ |
| 发布任务完成事件（`vpp.dispatch.events`，可选） | ✅ | ❌ |

---

## 十八、完整端到端数据流

> **说明**：下列伪代码中的 `repo.Save` / `repo.Update` 为简写；实际持久化以 §六 三 Repository（`TaskRepository` / `ActionRepository` / `CommandRepository`）为准，按 outcome 精确更新变化的行。

### 18.1 全异步路径（Gateway 返回 Accepted）

```
管理端
  ──gRPC SubmitTask──▶ Dispatch
    ├─ 构建 DispatchTask（Pending）→ dispatcher.PrepareTask → Running
    ├─ Action[seq=1, Sequential]
    │    └─ CommandsToDispatch = [Command1]（Pending → Sending）
    │         ──gRPC ExecuteCommand(commandID=cmd-xxx, ...)──▶ Gateway
    │              ├─ 查 DeviceMapping（CUCode → ExternalID）
    │              ├─ emsClient.SendCommand(commandID, externalID, ...)
    │              └─ 返回 GatewayAccepted
    ├─ Command1: Sending，DeadlineAt = now + 30s
    ├─ repo.Save(task)
    └─ 返回 TaskID

[Gateway 内部异步执行完毕]
  → gateway.adapter.outbound.kafka 发布：
    vpp.command.events ← Envelope[CommandCompletedPayload{
      command_id: "cmd-xxx",
      success: true,
      ack_at: ...
    }]

Dispatch CommandResultConsumer
  → HandleCommandResult{CommandID: "cmd-xxx", Success: true}
      ├─ repo.FindByCommandID → DispatchTask
      ├─ 幂等检查：Command1 仍为 Sending，继续处理
      ├─ dispatcher.OnCommandResult
      │    ├─ Command1 = Succeeded
      │    └─ nextCommands = [Command2]（Sequential continuation）
      ├─ gatewayPort.ExecuteCommand(Command2)
      └─ ...（递归推进直到 Action1 全部完成）

  [Action1 = Completed，NextPendingAction = Action2]
      → executeFirstBatch(Action2)

  [所有 Action 完成]
      → Task = Completed
      → PublishTaskCompleted(task)
      → repo.Update(task)
```

### 18.2 Timeout 路径

```
TimeoutScanner（每 10s 扫描）
  → commandRepo.FindExpiredSending(now)
  → [Command1，deadline 已过]

  → task = repo.FindByCommandID(Command1.ID)
  → dispatcher.OnCommandTimeout(task, Command1.ID)
      ├─ CanRetry()? → Command1: Timeout → Pending（RetryCount++）
      │                → nextCommands = [Command1]（重试）
      └─ !CanRetry()  → Command1: Failed
                        → Action: Failed → Task: Failed
                        → PublishTaskFailed(task)
  → repo.Update(task)
```

### 18.3 与其他服务的关系

```
管理端 ──gRPC──▶ Dispatch                         (新)
Dispatch ──gRPC ExecuteCommand──▶ Gateway         (已有接口，需更新 proto)
Gateway  ──Kafka vpp.command.events──▶ Dispatch   (新链路，gateway 需新增生产者)
Dispatch ──Kafka vpp.dispatch.events──▶ （告警/监控，可选，v1 不实现）
```

---

## 十九、开发阶段建议

| 阶段 | 内容 | 说明 |
|------|------|------|
| **Phase 1** | 更新 `gateway.proto`（CommandID、PointKey、oneof value）+ 同步更新 gateway handler | Breaking change，需先完成 |
| **Phase 2** | Dispatch `domain/` 层（models、ports、service/dispatcher、service/validator）| 纯 Go，无外部依赖，可单测 |
| **Phase 3** | Dispatch 持久化：DB migration、`infrastructure/` raw GORM、`adapter/outbound/postgres`（三 Repository）| 依赖 Phase 2 |
| **Phase 4** | Dispatch `application/`（SubmitTask、HandleCommandResult、TimeoutScanner、GetTask）| 依赖 Phase 2、3 |
| **Phase 5** | Dispatch 其余 `adapter/`（gateway_grpc client、inbound gRPC server、kafka consumer、event publisher）| 依赖 Phase 4 |
| **Phase 6** | Gateway 新增 Kafka producer（CommandEventPublisher）| 依赖 Phase 1 |
| **Phase 7** | 端到端联调（dispatch → gateway → Kafka → dispatch consumer） | 依赖 Phase 5、6 |
