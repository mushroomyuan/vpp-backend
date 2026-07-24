# Architecture Decision Records (ADR)

> 记录已做出的关键设计决策、动机和取舍。接手时**先读这里**，避免重复争论已决定的问题。

---

## ADR-001 · 使用 DDD + Hexagonal 架构

**状态：** 已采纳

**背景：** VPP 是多服务分布式系统，resource 服务需要隔离业务逻辑、便于替换基础设施（如未来从 Postgres 迁移到 TimescaleDB）。

**决策：** 采用六边形架构（Ports & adapter）+ DDD 领域建模。
- `domain` 层不依赖任何框架
- 通过 `domain/port` 接口隔离基础设施
- `application` 层只编排领域对象，不含 SQL 逻辑

**取舍：**
- ✅ 测试友好（domain/application 层可纯 mock 测试）
- ✅ 基础设施可替换
- ❌ 新增实体需要跨 4 层添加文件（样板代码较多）

---

## ADR-002 · Worker 使用 DB 行锁而非 Redis 分布式锁

**状态：** 已采纳

**背景：** 异步 Job 需要在多 Pod 部署时保证同一 job 不被重复执行。

**决策：** 使用 PostgreSQL `SELECT FOR UPDATE SKIP LOCKED` 原子认领，不引入 Redis 或外部协调服务。

```sql
UPDATE import_jobs
SET status='running', attempts=attempts+1, started_at=now
WHERE id = (
    SELECT id FROM import_jobs
    WHERE ... FOR UPDATE SKIP LOCKED
)
RETURNING *
```

**动机：**
- MVP 阶段不想增加基础设施依赖
- PostgreSQL 行锁足以满足 VPP 当前规模
- 单条 UPDATE...RETURNING 避免两步 SELECT+UPDATE 的 race condition

**取舍：**
- ✅ 无额外中间件
- ✅ 事务一致性强
- ❌ 吞吐量受限于 PG 锁竞争（高并发场景需重新评估）
- ❌ 如迁移到非 PG 数据库需重写认领逻辑

**卡住的 worker 处理：** `started_at < now - 10min` 的 running job 视为 crashed，自动重新认领（`stuckJobTimeout = 10min`）。

---

## ADR-003 · ImportWorker 采用单 goroutine 设计

**状态：** 已采纳

**背景：** 是否需要 worker pool 来并发处理多个 import job。

**决策：** 单 goroutine，每次 tick 仅处理一个 job，job 内部分块（batchSize）。

**动机：**
- VPP MVP 导入频率低，单 goroutine 足够
- 避免 goroutine pool 的复杂性和资源泄漏风险
- 多节点水平扩展可自然增加并发度（每个 Pod 一个 goroutine）

**取舍：**
- ✅ 实现简单，易于 debug
- ❌ 单 Pod 内同时只能执行一个 job，高并发场景瓶颈明显

**未来扩展路径：** 如需提升，可在 worker 层引入 semaphore-based pool，无需修改 domain/port 层。

---

## ADR-004 · CQRS 分离 Command / Query

**状态：** 已采纳

**背景：** 读写混用容易导致 Handler 职责不清，且读侧可能需要独立优化（缓存、只读副本）。

**决策：** `application/command`（写）和 `application/query`（读）严格分目录。两侧各自定义输入/输出类型，不共用。

**取舍：**
- ✅ 读写职责清晰，可独立演进
- ✅ 未来可为读侧接入缓存层而不影响写侧
- ❌ 小服务初期样板代码略多

---

## ADR-005 · 批量写入使用应用层分块而非 DB 存储过程

**状态：** 已采纳

**决策：** `BatchCreateResource` 和 `ResourceImportExecutor` 都在 Go 层将 items 切分为 batchSize 大小的 chunk，再逐批调用 `CreateInBatches`。

**动机：**
- 避免单次超大 INSERT 导致 PG 内存压力
- Go 层分块便于逐块释放内存（GC 友好）
- 逐块可以记录进度（`UpdateProgress`）

**默认值：**
- `BatchCreateResource` command 默认 `defaultBatchSize = 100`
- `ResourceImportExecutor` 默认 `batchSize = 100`（来自 payload 或 default）
- GORM `CreateInBatches` 最大 500

**部分写入补偿：** 不用整 Job 大事务。后序 chunk / `onChunk` 失败时，`BatchCreate*` 对已成功 ID 调用 `BatchDelete` 补偿删除；现有 `RetryJob` 即可重试，不新增 Retry API。进程崩溃导致的脏数据不在此路径覆盖（需后续卡住 Job / 人工处理）。

---

## ADR-006 · SoftDelete 不级联（待最终决策）

**状态：** ⚠️ 待定

**背景：** 当 `SoftDelete` 一个 Site 时，其下的 Resource、CU、Point 应如何处理？

**当前实现：** 各实体独立软删除，无级联逻辑。`SiteRepository.SoftDelete` 只删 Site 行。

**待讨论选项：**
- A. 应用层事务中手动级联（在 `DeleteSiteHandler` 中顺序软删 Resource → CU → Point）
- B. DB 触发器级联（侵入基础设施，难以测试）
- C. 保持现状，由上层（调度服务）决定级联删除顺序

**影响文件：** `application/command/delete_site.go`，需在此决策后补充逻辑。

---

