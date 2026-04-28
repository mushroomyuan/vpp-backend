# Development Handoff — Resource Service

> 最后更新：2026-04-23
> 目的：让接手的 agent / 开发者快速了解当前进度、下一步任务、以及不可踩的坑。

---

## 当前整体状态

**阶段：核心功能与横切能力已就绪；✅ 进入测试阶段（部署测试库、依赖服务、补自动化测试）。**

上一阶段已完成：Site/Resource/CU/Point 建模与 CQRS、异步导入、HTTP/gRPC（grpc-gateway）、thin main + `PrepareRun`/`Run`、**配置文件（viper + Options/Config）**、**Prometheus 指标（独立 `/metrics` 监听 + DB 池指标）**、**OpenTelemetry（`tracing.endpoint` 可关）**、**Consul 服务发现（`resource.consul-addr` 可关）**。

| 子系统 | 状态 |
|--------|------|
| Site CRUD | ✅ 完成（command + query + adapter + infra） |
| Resource CRUD + 批量创建 | ✅ 完成 |
| CU CRUD + 批量 | ✅ 完成（含批量创建/批量删除） |
| Point CRUD + 批量 | ✅ 完成（含批量创建） |
| 异步导入 (Job) | ✅ 完成（worker + executor + 重试；已扩展 CU job type；submit / get / retry 已收口 HTTP/gRPC） |
| HTTP / gRPC 入口 | ✅ 已接入（`ports/http` 挂载 grpc-gateway；`ports/grpc` 实现 ResourceServiceServer） |
| 应用组装 / 启动入口 | ✅ 已完成（见 `docs/APP_WIRING.md`） |
| 配置（YAML / 环境变量） | ✅ 已梳理（`options` + `config`；无文件时默认值 + `Viper` 环境覆盖） |
| Prometheus 指标 | ✅ `platform/metrics`：`resource.metrics-addr` |
| OpenTelemetry | ✅ `run.go`：`tracing.endpoint` 为空则跳过 |
| 服务发现（Consul） | ✅ `run.go`：`resource.consul-addr` 为空则跳过 |
| 数据库迁移脚本 | ✅ `migrations/resource/000001_init.{up,down}.sql`（PostgreSQL，与 `models.go` 对齐） |
| 单元 / 集成测试 | ⚠️ 测试阶段优先补（`_test.go` 仍少；建议冒烟 + repo 集成） |

---

## 已完成的模块清单

### Domain Layer
- `domain/model/`: Site, Resource, CU, Point, Job（含完整状态机方法）
- `domain/port/`: 所有 Repository 接口 + Filter 类型
- `domain/errors.go`: 5 个哨兵错误

### Application Layer
**Commands（写侧）：**
- `create_site`, `update_site`, `delete_site`
- `create_resource`, `update_resource`, `delete_resource`
- `batch_create_resource`（分块写入，默认 100/批）
- `create_cu`, `update_cu`, `delete_cu`, `batch_delete_cu`
- `batch_create_cu`（分块写入）
- `create_point`, `update_point`, `delete_point`
- `batch_create_point`（分块写入）
- `submit_resource_import`（提交异步任务，校验后写入 import_jobs）
- `retry_import_job`

**Queries（读侧）：**
- `get_site`, `list_sites`
- `get_resource`, `list_resources`
- `get_cu`, `list_cus`
- `get_point`, `list_points`
- `get_import_job`

**Workers：**
- `ImportWorker`：单 goroutine 轮询，5s 间隔，ctx 可取消
- `ResourceImportExecutor`：按 batchSize 分块插入，逐块更新进度
- `CUImportExecutor`：按 batchSize 分块插入，逐块更新进度

### Adapters Layer
- 所有 5 个实体的 Postgres 仓储实现
- `convertor.go`：领域对象 ↔ GORM 模型的双向转换

### Infrastructure Layer
- `db.go`：Viper 配置 DSN + GORM 连接池
- `models.go`：所有 GORM 模型定义
- `*_repo.go`：低层 CRUD 实现
- `builder/`：Site, Resource, CU, Point 的查询构建器
- `import_job_repo.go`：含 `ClaimPendingJob`（FOR UPDATE SKIP LOCKED）

---

## 待处理的已知 Bug（接手时优先修复）

| 优先级 | 位置 | 描述 |
|--------|------|------|

---

## 下一步工作建议

### 近期（测试阶段）
- 1. **部署测试环境**：起 PostgreSQL（与 `database.*` 或 `database.dsn` 一致）、按需起 OTEL Collector、Prometheus 抓取、`Consul`（若需注册）；建表/迁移与种子数据按团队约定执行。
- 2. **补测试（优先冒烟）**：覆盖 gRPC/HTTP（grpc-gateway）主要路径的基本 CRUD/LIST（建议先做只读 GET/LIST + 1~2 个写侧用例）；可选 `testcontainers` 做 repo 集成测试。
- 3. **配置与文档**：在仓库中保留一份可提交的 `config/resource.yaml.example`（或等价示例），避免新人只靠默认值猜 key。
- 4. **导入任务 API**：✅ 已完成（`SubmitResourceImport` / `GetJob` / `RetryJob` 均已收口 HTTP/gRPC）。
- 5. **权限/租户边界校验**：补齐 tenantID/siteID/resourceID 的跨层校验策略（目前部分 repo 方法不带 tenantID）。

### 已完成（无需重复排期）
- **依赖注入 / 入口组织**：thin main + `PrepareRun().Run()`（见 `docs/APP_WIRING.md`）。
- **可观测性与发现**：Metrics、Tracing、Consul 已在 `run.go` / `server.go` 接线。

### 中期

- **数据库迁移**：补充建表脚本（sites / resources / cus / points / import_jobs）
- **补充导入能力**：如需要，增加 CU/Point 的 submit handler（写入 import_jobs.payload）并注册 executor
- **写单元测试**：先覆盖领域模型（Job 状态机、Site 构造函数）
- **Worker 重试延迟**：当前 `retryDelay = 5min` 硬编码在 infra 层，考虑提升到配置

### 长期

- **事件发布**：Site/Resource 变更后向消息队列发布领域事件（目前无 event bus 集成）
- **级联软删除**：删除 Site 时是否级联删除 Resource/CU/Point，需要业务决策后落实（见 DECISIONS.md）

---

## 依赖注入顺序参考（接入入口时）

```
postgres.NewPostgres(cfg)
  ↓
postgres.NewXxxRepository(pg)
  ↓
adapters.NewXxxPostgresRepository(pgRepo)
  ↓
command.NewXxxHandler(repo, metricsClient)
```

Worker 启动：
```
registry := worker.ExecutorRegistry{
    model.JobTypeResource: worker.NewResourceImportExecutor(resourceRepo, jobRepo),
    model.JobTypeCU:       worker.NewCUImportExecutor(cuRepo, jobRepo),
}
w := worker.NewImportWorker(jobRepo, registry, cfg)
go w.Start(ctx)
```
