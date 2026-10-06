---
name: Proto-first gateway
overview: 迁移到 proto-first，以 gRPC 为单一事实来源，同时用 grpc-gateway 自动提供 REST 镜像与 OpenAPI（不再维护手写/拆分 OpenAPI spec）。
todos:
  - id: proto-http-annotations
    content: 为 ResourceService RPC 增加 google.api.http 注解，路径对齐现有 REST 设计
    status: completed
  - id: gateway-openapi-gen
    content: 补齐生成链路：protoc-gen-grpc-gateway + protoc-gen-openapiv2（输出到固定目录）
    status: completed
  - id: ports-wireup
    content: 实现 resource 服务的 gRPC server 注册与 grpc-gateway HTTP 入口
    status: in_progress
  - id: make-gen
    content: 更新 Makefile 的 genopenapi 目标为 proto→openapi 生成，弃用 oapi-codegen
    status: pending
isProject: false
---

## 目标
- 以 `api/resource/proto/root.proto` 及 `modules/*.proto` 作为唯一 API Source of Truth
- 用 `grpc-gateway` 自动生成：HTTP 反向代理（REST）+ OpenAPI（Swagger）
- 移除/冻结当前手写 OpenAPI 拆分体系（`api/resource/openapi/*`）带来的工具链摩擦（external refs、bundled.yml 等）

## 现状要点（基于仓库）
- gRPC service 已存在：`api/resource/proto/root.proto` 定义了 `ResourceService` 的所有 RPC
- HTTP/GRPC ports 仍是空壳：`internal/resource/ports/http.go` 为空，`internal/resource/ports/grpc.go` 仅 `package ports`
- Go module 已间接依赖 `grpc-gateway/v2`（`internal/resource/go.mod`），说明引入成本不高

## 推荐方案（你选择了“gRPC + gateway”且无历史包袱）
- **抛弃手写 OpenAPI 作为主链路**，改为：
  - proto 定义 + `google.api.http` 注解 → gateway 生成 REST 路由
  - 同一套 proto → 生成 OpenAPI（通常是 `protoc-gen-openapiv2`）
  - 如需更好的人类可读性：用 README/注释 + 自动生成文档站点（后续再加）

## 实施步骤（最小闭环）
1. **在 proto 中加 HTTP 映射注解**
   - 在 `[api/resource/proto/root.proto](/home/yfz/project/vpp/api/resource/proto/root.proto)` 引入 `google/api/annotations.proto`（以及所需的 `protoc-gen-openapiv2` options）
   - 给每个 RPC 添加 `option (google.api.http) = { ... }`，路径尽量对齐你现有 `api/resource/openapi/root.yml` 的 REST 设计（例如 `/tenants/{tenant_id}/sites` 等）

2. **生成代码（3类）**
   - **gRPC Go**：你已有 `scripts/genproto.sh` 输出到 `api/resource/proto/gen/`
   - **gateway 反向代理**：新增生成步骤（通常输出到某个 `internal/resource/ports/http/gen` 或 `api/resource/proto/gen` 下的 gateway 文件，按你项目习惯定）
   - **OpenAPI/Swagger**：由 proto 直接生成单一 swagger/openapi 文件（避免拆分 external refs）

3. **落地服务启动形态**
   - 实现 `internal/resource/ports/grpc.go`：注册 `ResourceService` 的 server（把 application handlers 接进去）
   - 实现 `internal/resource/ports/http.go`：启动 `grpc-gateway` mux，将 HTTP 请求转发到本地 gRPC（同进程或同主机）
   - 在 `internal/resource` 的 main/启动入口（如果已有）把两者组合起来

4. **逐步下线手写 OpenAPI**
   - 先把 `Makefile genopenapi` 从 `oapi-codegen` 切换到 “proto → openapi” 的生成链路
   - `api/resource/openapi/*` 可以先保留只读（作为对照/过渡），等 proto 注解覆盖完整后再删除

## 关键约束/取舍
- **优点**：单一事实来源、工具链稳定、不会再遇到拆分 `$ref` + import-mapping 的复杂度爆炸
- **代价**：需要在 proto 里维护 HTTP 映射与部分 OpenAPI 细节（错误模型、分页参数等）
- **注意**：REST 语义（query 参数、筛选数组、default response 模型等）在 proto 注解里表达能力有限，最好尽早统一一套错误响应/分页规范

## 验收标准
- `make gen`：proto 生成 + gateway/openapi 生成均成功
- 能通过 HTTP 访问至少 1-2 个代表性接口（如 CreateSite/ListSites），并正确路由到 gRPC handler
- 自动生成的 OpenAPI 文件可被常见工具打开（Swagger UI / openapi linter）
