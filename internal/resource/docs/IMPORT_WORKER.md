## **主循环（**`ImportWorker`**）**

1. `ClaimPending`：从库里「抢」一条可执行任务（pending，或实现里还会处理卡住的 running 等，见 infra 的 SQL）；没有任务则静默返回。
2. `executors.Get(job.Type)`：找不到类型 → `Fail`，任务标记失败。
3. `executor.Execute(ctx, job)`：真正干导入（解析 payload、分批写库、中间可 `UpdateProgress`）。
4. 成功 → `Complete`（带上 total/succeeded/failedCount 和 `resultJSON`）；失败 → `Fail`（错误信息进库）。

对应代码在 `47:100:internal/resource/application/worker/import_worker.go`。

## **领域模型里的状态（**`model.Job`**）**

- 状态：`pending` → `running` → `success` / `failed`。
- 默认 `MaxAttempts = 3`；领域上有 `CanRetry` **/** `ResetForRetry`，配合仓储的 `ResetForRetry` 可把失败任务拉回 `pending` 再被 worker 捡起（具体何时调用由命令/API 层决定，不在 worker 循环里自动重试每一条）。

## **两种已接好的任务类型**

- `JobTypeResource`：`ResourceImportExecutor` 解析 `ResourceImportPayload`，调用 `BatchCreateResources`，结果序列化为 `ResourceImportResult`（含 `resource_ids`、可选 `failed_items`）。
- `JobTypeCU`：`CUImportExecutor` 解析 `CUImportPayload`，批量插入 CU，结果为 `CUImportResult`。

注册关系在 server 组装时把 `ExecutorRegistry` 建好并传给 `NewImportWorker`（你之前文档里写过）。

## **和接口注释的对应关系**

- `ClaimPending`：就是 worker 每轮的第一步，保证多节点下同一 job 只被一个 worker 执行。
- `UpdateProgress`：给长时间批量导入用，让外部能查到中间进度（executor 里在分批写入后会调）。
- `Complete` **/** `Fail`：对应一次执行的成功收尾或失败收尾。
- `ResetForRetry`：业务上「手动/策略性重试」时用，把失败任务重新变成 pending，下一轮 `ClaimPending` 才能再捡到。

