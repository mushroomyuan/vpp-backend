## `infrastructure` 层不只是"GORM 的包装"

表面上看`infrastructure/persistent/postgres/` 像是薄薄的 CRUD 封装，但它实际承担了几个有真实价值的职责，逐一分析：

### 1. 独立的基础设施数据模型

`models.go` 中的 `XxxModel` 是与数据库 schema 对齐的"物理模型"，与 domain model 有结构性差异：

```133:133:internal/resource/infrastructure/persistent/postgres/models.go

func (JobModel) TableName() string { return "import_jobs" }

```

例如 `Node` + `Asset` 是两张表，但 `domain/model.Asset` 是一个合并后的概念`infrastructure` 层保有数据库的物理视图`adapter/outbound` 的 converter 做合并/拆分。如果把这些 Model 放进 adapter，adapter 就成了一个既懂 domain 又懂 DB schema 的大杂烩。

### 2. 真正复杂的数据库逻辑

`NodeRepository` 远不止"wrap GORM"，它包含树型结构的基础设施原语：

```46:61:internal/resource/infrastructure/persistent/postgres/node_repo.go

// FindNodeByIDForUpdate is identical to FindNodeByID but acquires a FOR UPDATE

// row-level lock. Must be called inside a transaction (pass the *gorm.DB from

// the transaction callback).

func (r *NodeRepository) FindNodeByIDForUpdate(ctx context.Context, tx* gorm.DB, tenantID, id string) (result *NodeModel, err error) {

```

```253:268:internal/resource/infrastructure/persistent/postgres/node_repo.go

// SoftDeleteSubtree soft-deletes the root node (matched by id) and all

// descendants (matched by pathPrefix+"/%") in a single DELETE statement.

func (r *NodeRepository) SoftDeleteSubtree(ctx context.Context, tenantID, id, pathPrefix string) (err error) {

```

`FOR UPDATE` 锁、路径前缀的子树删除、跨事务的变体方法——这些是**数据库访问的技术策略**，不是 domain 知识，也不应该在 adapter 里出现。

### 3. Builder 作为独立的查询构建关注点

```36:63:internal/resource/infrastructure/persistent/postgres/builder/asset.go

func (a *Asset) Fill(db* gorm.DB) *gorm.DB {

	db = db.Table("assets").Order("assets.created_at DESC")

	// ...跨表 JOIN nodes，ILIKE 全名模糊搜索...

}

```

Builder 完全不感知 domain，只处理 SQL 拼接逻辑。它是 infrastructure 层内部可测试的独立单元，也可以被多个 infra repo 复用。

---



## `infrastructure` 层未来还可以承担什么

你们的架构还有几个自然演进方向，这些都应该落在 `infrastructure` 层：

### 最紧迫：Transactional Outbox

**这是当前架构的一个隐患**。目前的写入路径是：

```

command handler → Postgres 写入 → Kafka publish

```

如果 Postgres 写入成功但 Kafka 发布失败，事件永久丢失，但 DB 已变更。正确的做法是 **Outbox 模式**：

```

infrastructure/persistent/postgres/

  outbox_repo.go   ← 在同一个事务里写 outbox 表

  

infrastructure/runtime/

  outbox_relay.go  ← 独立 goroutine，轮询 outbox → 发布 Kafka → 删除记录

```

这样事件发布就具备了与 DB 写入相同的原子性。这是 `infrastructure` 层最典型的未来职责之一。

### 查询端（Read Model）扩展

当 `ExportResourceTree` 或统计查询变得复杂时，可以在 infrastructure 层引入：

```

infrastructure/persistent/postgres/

  view_repo.go       ← 访问 PostgreSQL 物化视图

  report_queries.go  ← 复杂的 raw SQL 报表查询（CTE、window functions）

```

这些查询没有对应的 domain model，是纯 DB 侧的读模型，放在 adapter 里不合适。

### 全文检索 / 高级搜索

当前 `NameLike` 用的是 `ILIKE`。如果未来要上 PostgreSQL `tsvector` 全文搜索：

```

infrastructure/persistent/postgres/

  search_repo.go     ← tsvector/tsquery，GIN 索引扫描

```

这是明显的 infrastructure 关注点，与 domain 无关。

---



## 总结：两层的价值在于"关注点分离的粒度"

```

infrastructure/persistent/postgres/

  职责：知道数据库 schema、SQL 方言、事务原语、GORM 技巧

  不知道：domain 错误、domain 业务规则

adapter/outbound/postgres/

  职责：翻译 domain 概念 ↔ 基础设施模型，实现 domain port 接口

  不知道：SQL、gorm.ErrRecordNotFound 的细节（只关心转换结果）

```

如果把两层合并，adapter 就需要同时懂 domain 语义、数据库 schema 和 SQL 细节。这在现在问题不大，但随着业务增长（更多 Join、物化视图、Outbox、Search），混合层会变得很难维护和测试。

**所以** `infrastructure` **层不是可以消除的冗余，而是在当前规模下略显"低调"，但架构上位置是正确的**。它的存在价值随着项目复杂度的增长会越来越明显。