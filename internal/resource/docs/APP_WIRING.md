# Resource Service — App Wiring & Startup

本文件记录 `internal/resource` 的**应用组装（wiring）**与**启动（startup）**约定，目标是吸收 IAM 项目的 *thin main* 优势：入口极薄、依赖注入集中、启动分两阶段（`PrepareRun` → `Run`）、可持续演进。

---

## 1. 入口结构（Thin main）

- 可执行入口：`internal/resource/cmd/main.go`
- 入口只做一件事：调用 `resource.NewApp("vpp-resource").Run()`

目录与职责：

```
cmd/main.go        → thin main（不做 wiring）
app.go             → cobra + viper 配置加载（Options）
config/config.go   → Options → Config（内部配置）
run.go             → Run(cfg)：cross-cutting init + createServer
server.go          → createServer wiring + PrepareRun + Run（生命周期）
```

---

## 2. 配置分层：Options vs Config

### Options（外部输入）

`options.Options` 通过 `viper.Unmarshal` 填充，面向 CLI / 配置文件 / 环境变量，字段对齐 viper 的 key 层级：

- `resource.grpc-addr`
- `resource.http-addr`
- `resource.service-name`
- `resource.worker-poll-interval`
- `telemetry.url`
- `postgres.*`（仍由基础设施层 `postgres.NewPostgres()` 直接读取 viper）

### Config（内部可运行配置）

`config.Config` 是内部结构，专门传给 server wiring（`createServer`），用于启动监听、worker、tracing 等。

---

## 3. Wiring 顺序（createServer 内）

`internal/resource/server.go:createServer` 的 wiring 顺序固定为：

```
postgres.NewPostgres()
  ↓
postgres.NewXxxRepository(pg)          // infra repos（GORM CRUD）
  ↓
adapter.NewXxxRepositoryPostgres(...) // ports 实现（domain/port）
  ↓
application.NewApplication(deps)       // CQRS handlers + worker registry
  ↓
adapter/inbound/grpc.NewServer(app, repos...)    // gRPC server impl（复用 app handlers）
  ↓
adapter/inbound/http.MountGateway(...)           // gin + grpc-gateway
```

说明：

- gRPC 的 **batch** API（批量创建）出于性能与控制流需要，仍会直接调用 `resourceRepo/cuRepo/pointRepo` 的 `BatchCreate`；因此这些 repo 仍作为 `adapter/inbound/grpc.Server` 的字段保留。
- 其余 API 全部走 `application.Application` 中预先组装好的 `Commands/Queries` handlers，避免重复 wiring。

---

## 4. 生命周期：PrepareRun → Run

启动由 `Run(cfg)` 进入：

1. **可选初始化 tracing**：`cfg.TelemetryEndpoint != ""` 时初始化，否则跳过（本地开发可不开 OTEL collector）。
2. `createServer(cfg)`：完成所有 wiring，不启动 goroutine。
3. `PrepareRun()`：预留做 pre-flight（如未来 DB ping、readiness）。
4. `Run()`：
   - 启动 gRPC server
   - 启动 import worker（后台 goroutine）
   - 启动 HTTP server（gin + gateway）
   - 监听 `SIGINT/SIGTERM`，触发 graceful shutdown（30s deadline）

---

## 5. 启动方式

**推荐**：在 `internal/resource` 模块根目录执行（便于相对路径找到 `go.mod` 与 `./config` 下的 `resource.yaml`）：

```bash
cd internal/resource
go run ./cmd/ -c /path/to/resource.yaml
```

`--config` / `-c` 可选；若不指定，会在 `./config`、`../../config`、`.` 下查找 `resource.yaml`；仍找不到则仅用默认值 + 环境变量（见 `app.go:loadViperConfig`）。

也可进入 `cmd/` 后 `go run main.go`（Go 会向上解析到本模块的 `go.mod`），但传参时同样要带 `-c` 或依赖上述搜索路径，**更推荐始终在模块根用 `go run ./cmd/`**，与 CI/构建路径一致。

构建：

```bash
go build -o bin/vpp-resource ./cmd/
```

---

## 6. 后续改进点（非阻塞）

- ~~**健康检查**：`PrepareRun()` 中可以挂载 `/healthz`、`/readyz`。~~ **已解决**：`platform/server.NewGinEngine()` 现在统一注册 `GET /healthz`，resource 的 HTTP 引擎复用该工厂函数，无需在 resource 里单独实现。gRPC 侧同理，`NewGRPCServer()` 已注册标准 `grpc.health.v1.Health` 服务。
- **配置 key 统一**：`database.*` 已通过 `options.Database` → `infrastructure/db` 注入；若仍有个别路径直读 viper，可再收敛到 composition root。
- **示例配置**：仓库内补充 `config/resource.yaml.example`，与 `HANDOFF.md` 测试阶段 checklist 对齐。

