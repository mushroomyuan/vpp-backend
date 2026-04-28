# Architecture & Coding Hard Rules

> 违反这些规则会破坏架构边界或引入静默 bug，PR 时必须检查。

---

## 1. 分层依赖方向（不可违反）

```
domain  ←  application  ←  adapters  ←  infrastructure
```

- `domain` **不得** import 任何其他本模块包
- `application` 只能 import `domain`（model / port / errors）
- `adapters` 只能 import `domain` + `infrastructure`
- `infrastructure` 只能 import 外部库（gorm、logrus 等）

> 违反示例：在 `domain/model` 里 import `adapters` 或 `infrastructure` → 立即拒绝。

---

## 2. Port 接口规则

- 所有仓储接口定义在 `domain/port/`，**不得** 在其他包中定义接口后再注入
- 每个 adapter 实现文件顶部**必须**有编译期断言：

```go
var _ port.SiteRepository = (*SitePostgresRepository)(nil)
```

---

## 3. Handler 规则

- 每个 Handler 的 `Handle` 方法**第一行**必须是：

```go
ctx, span := telemetry.Start(ctx, "snake_case_operation_name")
defer span.End()
```

- 所有 Handler 通过 `decorator.ApplyCommandDecorators` / `ApplyQueryDecorators` 包装后对外暴露
- 构造函数对 nil 仓储**必须** `panic`（快速失败，避免运行时 nil dereference）

---

## 4. 错误处理规则

| 场景 | 规则 |
|------|------|
| 基础设施层检测 `gorm.ErrRecordNotFound` | 转换为对应领域哨兵（`domain.ErrXxxNotFound`） |
| 应用层 / 适配器层比较哨兵 | **必须用** `errors.Is(err, domain.ErrXxx)`，禁止 `==` |
| 向上传递包装错误 | `fmt.Errorf("context: %w", err)` |
| 领域哨兵定义位置 | **只能**在 `domain/errors.go` |

---

## 5. 领域模型规则

- 实体字段**不得**是可选指针（除时间戳 `*time.Time`），零值代表"未设置"
- 构造函数（`NewXxx`）负责全部合法性验证并返回 `error`，**禁止**在外部直接赋值创建实体
- 状态机转换（如 `Job.Start / Complete / Fail`）**只能**在领域模型方法中进行，应用层不得直接修改 `Status` 字段

---

## 6. 数据库访问规则

- GORM 模型（`*Model`）**只存在于** `infrastructure/persistent/postgres/`
- 每个数据库方法**必须**使用 `logging.WhenDB` 模式：

```go
func (r *XxxRepository) SomeMethod(ctx context.Context, ...) (result *XxxModel, err error) {
    _, deferLog := logging.WhenDB(ctx, "XxxRepository.SomeMethod", input)
    defer func() { deferLog(result, &err) }()
    // ...
}
```

- 复杂多表过滤逻辑**必须**提取到 `infrastructure/persistent/postgres/builder/` 中
- 批量写入**必须**使用 `CreateInBatches(..., 500)` 或应用层分块（chunk size ≤ 500）

---

## 7. 并发与 Worker 规则

- `ImportWorker` 设计为**单 goroutine**，不得在 `processNext` 内部开新 goroutine
- 多节点安全性通过 `SELECT FOR UPDATE SKIP LOCKED` 保证，**禁止**引入 Redis 分布式锁替代（架构决策 ADR-002）
- 新增 `Executor` 必须注册到 `ExecutorRegistry`，**禁止** switch-case 硬编码在 worker 主流程中

---

## 8. 命名约定

| 类型 | 命名格式 | 示例 |
|------|---------|------|
| 用例 Handler 接口 | `XxxHandler` (type alias) | `CreateSiteHandler` |
| 用例 Handler 实现 | `xxxHandler`（小写） | `createSiteHandler` |
| Port 接口 | `XxxRepository` | `SiteRepository` |
| Adapter 实现 | `XxxPostgresRepository` | `SitePostgresRepository` |
| Infra 实现 | `XxxRepository`（在 postgres 包下） | `postgres.SiteRepository` |
| GORM 模型 | `XxxModel` | `SiteModel` |
| 查询构建器 | 包名 `builder`，函数 `BuildXxxQuery` | `builder.BuildSiteQuery` |
| 命令/查询 | `XxxCommand` / `XxxQuery` | `CreateSiteCommand` |

---

## 9. 测试规则（待落地）

- 领域模型：纯单元测试，无任何外部依赖
- 应用层：Mock Port 接口，禁止真实 DB
- 基础设施层：集成测试，需 Docker PostgreSQL
