# Resource Service — Project Context

> VPP (Virtual Power Plant) · `internal/resource` 微服务
> 架构: DDD + Hexagonal (Ports & adapter) + CQRS · 语言: Go 1.26.2

---

## 1. 技术栈

| 层面 | 选型 |
|------|------|
| 语言 | Go 1.26.2 |
| 模块名 | `github.com/mushroomyuan/vpp-backend/resource` |
| 数据库 | PostgreSQL + GORM v1.31.1 |
| 配置 | spf13/viper：`resource.*`、`tracing.*`、`database.*`（见 `options/options.go`） |
| 日志 | logrus + `pkg/logging.WhenDB` |
| 跨切面 | `vpp/platform`（decorator、telemetry、**Prometheus `/metrics`**、**Consul 注册**） |
| ID 生成 | `vpp/platform/idgen` |

---

## 2. 文件夹结构

```
internal/resource/
├── cmd/
│   └── main.go                    # 可执行入口（thin main）
├── app.go                         # cobra + viper（Options）
├── run.go                         # Run(cfg)：init tracing + createServer
├── server.go                      # wiring + PrepareRun/Run + graceful shutdown
├── config/
│   └── config.go                  # Options → Config
├── options/
│   └── options.go                 # 外部输入 Options（viper.Unmarshal）
├── go.mod / go.sum
├── domain/
│   ├── errors.go                  # 哨兵错误 ErrXxxNotFound
│   ├── model/                     # 实体 & 值对象
│   │   ├── site.go  resource.go  cu.go  point.go  job.go
│   └── port/                      # 仓储接口 + 过滤器
│       ├── repository.go
│       └── import_job_repository.go
├── application/
│   ├── command/                   # 写侧用例 (*Handler)
│   ├── query/                     # 读侧用例 (*Handler)
│   └── worker/                    # 异步任务 (导入 Worker)
├── adapter/                      # Port 实现 + 领域↔DB 映射
│   ├── converter.go
│   └── *_postgres_repository.go
└── infrastructure/
    └── persistent/postgres/
        ├── db.go                  # GORM 初始化 & 连接池
        ├── models.go              # GORM 模型 (TableName)
        ├── *_repo.go              # 低层 CRUD / 原生 SQL
        └── builder/               # 流式查询构建器
```

---

## 3. 架构模式

```
[application] ──uses──> [domain/port] <──implements── [adapter] ──wraps──> [infrastructure]
```

- **Hexagonal**: `domain/port` 定义接口；`adapter` 实现并注入。
- **CQRS**: `application/command`（写）与 `application/query`（读）严格分离。
- **Decorator**: Handler 通过 `decorator.ApplyCommandDecorators / ApplyQueryDecorators` 加 tracing & metrics。
- **Worker**: 单 goroutine + DB 行级锁（`FOR UPDATE SKIP LOCKED`）保证多节点安全认领。

---

## 4. 编码规范

- **命名**: `*Handler`（用例）、`*Repository`（端口/基础设施）、`*Model`（GORM 模型）、`builder.*`（查询构建器）
- **错误**: 基础设施层用 `errors.Is(err, gorm.ErrRecordNotFound)` → 转换为领域哨兵；调用方用 `errors.Is`（**勿用 `==`**，见已知问题 #4）
- **错误包装**: `fmt.Errorf("...: %w", err)`
- **Tracing**: 每个 Handler 首行 `telemetry.Start(ctx, "operation_name")`
- **Nil guard**: 构造函数对空仓储执行 `panic`（快速失败）
- **批量**: DB 批量写用 `CreateInBatches(..., 500)`

---

## 5. 关键模块

| 模块 | 职责 |
|------|------|
| `domain/model/site.go` | Site 实体，状态枚举，地理位置值对象 |
| `domain/model/resource.go` | Resource 实体，租户/站点归属，元数据 map |
| `domain/model/cu.go` | CU（控制单元），父子层级，能力标签 |
| `domain/model/point.go` | Point（测点），数据类型，安全阈值，反范式 ResourceID |
| `domain/model/import_job.go` | 导入任务状态机：Pending→Running→Completed/Failed，重试逻辑 |
| `application/worker/import_worker.go` | 轮询 + 认领 + 执行导入，多节点安全 |
| `infrastructure/.../builder/` | 多表 JOIN 过滤构建器（CU/Point 需 JOIN resources） |

---

## 6. 重要依赖

```
github.com/mushroomyuan/vpp-backend/platform  → replace ../pkg
github.com/mushroomyuan/vpp-backend/platform     → replace ../../pkg
gorm.io/gorm                        v1.31.1
gorm.io/driver/postgres             v1.6.0
github.com/sirupsen/logrus          v1.9.3
github.com/spf13/viper              v1.21.0
```

---

## 7. 已知问题

| # | 位置 | 描述 |
|---|------|------|

---

## 8. 如何按现有风格添加新模块（以 `Schedule` 为例）

### Step 1 — 领域层

```go
// domain/model/schedule.go
type Schedule struct { ID, TenantID, ResourceID string; ... }
func NewSchedule(...) (*Schedule, error) { /* 校验 + 构造 */ }

// domain/errors.go
var ErrScheduleNotFound = errors.New("schedule not found")

// domain/port/repository.go（追加接口）
type ScheduleRepository interface {
    Create(ctx, *model.Schedule) error
    GetByID(ctx, id string) (*model.Schedule, error)
    List(ctx, ScheduleFilter) ([]*model.Schedule, error)
}
```

### Step 2 — 基础设施层

```go
// infrastructure/persistent/postgres/models.go（追加）
type ScheduleModel struct { ... }
func (ScheduleModel) TableName() string { return "schedules" }

// infrastructure/persistent/postgres/schedule_repo.go
type ScheduleRepository struct { db *gorm.DB }
// 实现接口方法，用 logging.WhenDB 记录，gorm.ErrRecordNotFound → domain sentinel
```

### Step 3 — 适配器层

```go
// adapter/schedule_postgres_repository.go
type SchedulePostgresRepository struct { repo *postgres.ScheduleRepository }
var _ port.ScheduleRepository = (*SchedulePostgresRepository)(nil) // 编译期检查

// adapter/convertor.go（追加转换函数）
func scheduleToModel(s *model.Schedule) *postgres.ScheduleModel { ... }
func scheduleFromModel(m *postgres.ScheduleModel) (*model.Schedule, error) { ... }
```

### Step 4 — 应用层

```go
// application/command/create_schedule.go
type CreateScheduleCommand struct { TenantID, ResourceID string; ... }
type CreateScheduleHandler struct { repo port.ScheduleRepository }
func (h *CreateScheduleHandler) Handle(ctx context.Context, cmd CreateScheduleCommand) error {
    ctx, span := telemetry.Start(ctx, "CreateSchedule")
    defer span.End()
    id := idgen.Must()
    s, err := model.NewSchedule(id, cmd.TenantID, ...)
    if err != nil { return err }
    return h.repo.Create(ctx, s)
}
// 用 decorator.ApplyCommandDecorators 包装

// application/query/get_schedule.go — 同理
```

### 关键检查清单

- [ ] 领域哨兵错误加入 `domain/errors.go`
- [ ] Port 接口加入 `domain/port/repository.go`
- [ ] GORM 模型加入 `models.go`，实现 `TableName()`
- [ ] 适配器加 `var _ port.X = (*Y)(nil)` 编译期断言
- [ ] Handler 首行 `telemetry.Start`
- [ ] 构造函数对空仓储 `panic`
- [ ] 列表查询通过 `builder/` 构建，勿在 repo 中硬编码过滤条件
