# APISIX 部署与配置指南

> 本文记录 vpp-backend 北向 APISIX 网关的**实际部署过程**、配置说明与踩坑经验。  
> 当前阶段：**Phase 0 — 透明反代**（无认证插件）。  
> 业务网关职责见 [`internal/gateway/OVERVIEW.md`](../internal/gateway/OVERVIEW.md)。

---

## 1. 为什么需要 APISIX

项目里已有 **vpp-gateway**（业务网关），负责设备映射、协议转换、指令转发。  
APISIX 是**另一层**，做基础设施级治理，两者不冲突：

| 层次 | 组件 | 职责 |
|------|------|------|
| 基础设施网关 | **APISIX** `:9080` / `:9081` | TLS、认证、限流、HTTP + gRPC 统一入口 |
| 业务网关 | **vpp-gateway** `:8083` | ID 映射、遥测归一、ExecuteCommand |

```
外部客户端
    │
    ├─ HTTP ──────────────────────────▶ APISIX :9080
    │                                      ├── /gateway/*  → kind NodePort :30083 (gateway HTTP)
    │                                      └── /resource/* → kind NodePort :30082 (resource HTTP)
    │
    └─ gRPC (h2c, localhost) ─────────▶ APISIX :9081
                                           ├── /dispatchpb.DispatchService/* → :30006
                                           └── TelemetryService 读 RPC → :30003
                                               （Ingest 仍集群内直连 telemetry Service，不经 APISIX）

内部 gRPC（dispatch→gateway、gateway→telemetry Ingest）不经过 APISIX，走 ClusterIP。
```
---

## 2. 前置条件

| 项 | 要求 |
|----|------|
| Docker | 已安装并运行 |
| docker-compose | v1（`docker-compose`）或 v2（`docker compose`）；Makefile 自动检测 |
| 业务服务 | 默认：kind 里跑，APISIX 打 NodePort `:30082` / `:30083` / `:30003` / `:30006`；本机 `make run-*` 需覆盖 `*_UPSTREAM` 再 `make apisix-init` |
| 网络 | WSL2 / Docker Desktop 需支持 `host.docker.internal` |

APISIX 与主 infra（Postgres/Kafka 等）**独立 compose**，生命周期分开：

```bash
make infra-up      # 主基础设施（compose.yaml）
make apisix-up     # APISIX 边缘网关（deploy/apisix/docker-compose.apisix.yaml）
```

---

## 3. 快速部署（Phase 0）

```bash
# 1. 主基础设施
make infra-up

# 2. 业务服务（kind NodePort；本机进程则覆盖 *_UPSTREAM 见 init.sh）
make k8s-apply

# 3. APISIX
make apisix-up
make apisix-init      # 灌入 routes / upstreams（幂等，可重复执行）

# 4. 检查
make apisix-status
```

停止：

```bash
make apisix-down     # 只停 APISIX
make stop-all        # 停业务服务
```

---

## 4. 目录与文件说明

```
deploy/apisix/
├── docker-compose.apisix.yaml   # APISIX + etcd 容器编排
├── conf/
│   └── config.yaml              # APISIX 引导配置（只挂载此文件）
├── init.sh                      # Admin API 初始化脚本（routes/upstreams）
├── gate0/
│   └── probe.sh                 # Gate 0：grpcurl + grpc-go 验收
└── routes/
    ├── gateway.yaml             # 路由参考文档（权威来源是 init.sh）
    ├── resource.yaml
    ├── dispatch-grpc.yaml
    └── telemetry-grpc.yaml
```

| 文件 | 作用 |
|------|------|
| `docker-compose.apisix.yaml` | 起 APISIX 3.11 + etcd；配置端口映射、healthcheck |
| `conf/config.yaml` | APISIX 进程引导：监听端口、Admin Key、etcd 地址、`enable_http2` |
| `init.sh` | 通过 Admin API 创建 upstream 和 route（存在则覆盖） |
| `routes/*.yaml` | 人类可读的参考，**不参与自动加载** |
| `gate0/probe.sh` | Gate 0 可行性探针 |

Makefile 命令：

| 命令 | 说明 |
|------|------|
| `make apisix-up` | 启动 APISIX stack |
| `make apisix-down` | 停止并移除容器 |
| `make apisix-init` | 执行 `init.sh` 灌配置（含 Gate 0 gRPC 路由） |
| `make apisix-gate0-probe` | 跑 Gate 0 验收（需 Casdoor + secured dispatch） |
| `make run-dispatch-secured` | 以 `trust-proxy-headers: true` 启动 dispatch |
| `make apisix-status` | 检查端口与 routes 数量 |
| `make apisix-logs` | 跟踪 APISIX 容器日志 |

---

## 5. 端口一览

| 宿主机端口 | 容器端口 | 用途 |
|-----------|---------|------|
| **9080** | 9080 | HTTP 北向入口（外部请求打这里） |
| **9081** | 9081 | **明文 HTTP/2（h2c）gRPC 北向**（Gate 0+；仅限 localhost） |
| **9181** | 9180 | Admin API（管理 routes/consumers） |
| 9091 | 9091 | Prometheus metrics（Phase 3 接入） |
| 9443 | 9443 | HTTPS / gRPCS（跨机联调；不要在明文 :9081 上跨机器传 Bearer） |

> Admin API 故意映射到宿主机 **9181** 而非 9180，避免与本机其他服务或 HTTP 代理冲突。

---

## 6. 核心配置详解

### 6.1 `conf/config.yaml` — APISIX 引导配置

APISIX 3.x 的配置结构与 2.x 不同，`admin_key` 必须放在 `deployment.admin` 下：

```yaml
apisix:
  node_listen:
    - port: 9080
    - port: 9081
  enable_http2: true              # :9081 明文 gRPC（h2c）需要
  id: vpp-dev-apisix-1          # 固定 node id，避免回写 host 挂载文件

deployment:
  role: traditional
  role_traditional:
    config_provider: etcd       # 动态配置存 etcd
  admin:
    admin_listen:
      ip: 0.0.0.0
      port: 9180                # 容器内 Admin 端口
    admin_key:
      - name: admin
        key: edd1c9f034335f136f87ad84b625c8f1   # 本地 dev only
        role: admin
  etcd:
    host:
      - "http://etcd:2379"
    prefix: "/apisix"
```

**注意：** `admin_key` 放在 `apisix:` 下会被忽略，Admin API 会报 "using empty Admin API"。

### 6.2 `docker-compose.apisix.yaml` — 容器编排

关键设计决策：

```yaml
services:
  etcd:
    image: bitnamilegacy/etcd:3.5.11   # bitnami 官方镜像已下架，用 legacy

  apisix:
    image: apache/apisix:3.11.0-debian
    volumes:
      # ✅ 只挂载 config.yaml；nginx.conf 等留给镜像内生成
      - ./conf/config.yaml:/usr/local/apisix/conf/config.yaml:ro
      - apisix_logs:/usr/local/apisix/logs
    ports:
      - "9181:9180"    # Admin：宿主机 9181 → 容器 9180
    extra_hosts:
      - "host.docker.internal:host-gateway"   # Casdoor :8000 + kind NodePort :300xx
```

### 6.3 `init.sh` — 路由初始化

通过 Admin API 创建 2 个 upstream + 2 个 route：

| Route ID | 对外路径 | 剥离前缀后 | Upstream |
|----------|---------|-----------|----------|
| `gateway-proxy` | `/gateway/*` | `/*` | `host.docker.internal:30083`（kind NodePort） |
| `resource-proxy` | `/resource/*` | `/*` | `host.docker.internal:30082`（kind NodePort） |

`proxy-rewrite` 插件负责剥离前缀：

```
GET /gateway/api/v1/tenants/default/mappings
  → proxy-rewrite: ^/gateway/(.*) → /$1
  → GET /api/v1/tenants/default/mappings  @ gateway:8083
```

脚本特点：

- **幂等**：重复执行 `make apisix-init` 安全
- **`curl --noproxy '*'`**：绕过 WSL 上常见的 HTTP 代理（如 Clash `:7897`）
- **健康检查**：等待 Admin API 返回 HTTP 200 再灌配置

---

## 7. APISIX 基础概念（5 分钟入门）

| 概念 | 说明 | 本项目对应 |
|------|------|-----------|
| **Route** | 匹配 URI，决定转发规则 | `/gateway/*` → gateway upstream |
| **Upstream** | 后端服务节点 | `host.docker.internal:30083`（kind NodePort；本机 `make run-*` 可覆盖） |
| **Consumer** | 调用方身份（API Key 持有者） | Phase 1 为 EMS 厂商创建 |
| **Plugin** | 横切能力链 | Phase 0 仅 `proxy-rewrite`；Phase 1+ 加 `key-auth` 等 |
| **Admin API** | 动态管理 routes/consumers | 宿主机 `:9181`，Header `X-API-KEY` |

APISIX 底层是 OpenResty/nginx，但路由和插件通过 Admin API 或 etcd 管理，无需手写 nginx.conf。

---

## 8. 验证与 Smoke Test

### 8.1 检查 APISIX 本身

```bash
make apisix-status
# 期望：9080 UP, 9181 UP, routes-configured: 2
```

### 8.2 检查 Admin API

```bash
curl --noproxy '*' -s http://127.0.0.1:9181/apisix/admin/routes \
  -H "X-API-KEY: edd1c9f034335f136f87ad84b625c8f1" | jq '.total'
# 期望：2
```

### 8.3 透明反代对比（业务服务需运行）

```bash
# 经 APISIX
curl --noproxy '*' -s http://127.0.0.1:9080/gateway/api/v1/tenants/default/mappings
curl --noproxy '*' -s http://127.0.0.1:9080/resource/api/tenants/default/sites

# 直连 kind NodePort（绕过 APISIX；resource/gateway /healthz 不需鉴权）
curl --noproxy '*' -s http://127.0.0.1:30083/healthz
curl --noproxy '*' -s http://127.0.0.1:30082/healthz
```

Phase 0 无认证，两者 status/body 应一致。

| 响应码 | 含义 |
|--------|------|
| 200 | 正常 |
| 502 | APISIX 正常，但 kind NodePort 未就绪 → `make k8s-apply`；确认 `:30082` / `:30083` |
| 404 | route 未配置 → `make apisix-init` |

---

## 9. 部署踩坑记录

以下是 Phase 0 实际部署中遇到的问题与最终解法，供后续参考。

### 9.1 挂载整个 `conf/` 目录 → Permission denied

**现象：**

```
failed to open file: /usr/local/apisix/conf/nginx.conf, Permission denied
```

**原因：** 把宿主机 `./conf/` 挂载到容器 `/usr/local/apisix/conf/`，覆盖了镜像内的 `nginx.conf`、`mime.types`。APISIX 启动时需要**在容器内生成/更新**这些文件，但 bind-mount 目录权限属于宿主机用户，容器进程写不了。

**解法：** 只挂载单个 `config.yaml`，其余 conf 文件留给镜像：

```yaml
# ✅
- ./conf/config.yaml:/usr/local/apisix/conf/config.yaml:ro

# ❌
- ./conf:/usr/local/apisix/conf
```

同时在 `config.yaml` 中设置固定 `apisix.id`，避免 APISIX 尝试回写 host 上的 config 文件。

---

### 9.2 `admin_key` 配置位置错误 → Admin API 空 key

**现象：**

```
WARNING: using empty Admin API.
This will trigger APISIX to automatically generate a random Admin API token.
```

**原因：** APISIX 3.x 的 `admin_key` 必须在 `deployment.admin.admin_key` 下，放在 `apisix.admin_key` 会被忽略。

**解法：** 见本文 [6.1 节](#61-confconfigyaml--apisix-引导配置) 的正确结构。

---

### 9.3 etcd 镜像不可用

**现象：**

```
manifest for bitnami/etcd:3.5.18 not found: manifest unknown
```

**原因：** Bitnami 2025 年起将大量镜像移出 Docker Hub 免费层（与项目中 Kafka 镜像情况相同）。

**解法：** 使用 `bitnamilegacy/etcd:3.5.11`（与 [`compose.yaml`](../compose.yaml) 里 Kafka 用 `bitnamilegacy/kafka` 是同一策略）。

---

### 9.4 Admin API 从宿主机连不上

**现象：** `make apisix-init` 等待 30 次超时；`docker port` 显示 9180 未映射到宿主机。

**原因（两个叠加）：**

1. 早期 compose 端口映射未生效，需 `docker-compose down && up` 重建
2. WSL 上 curl 默认走 HTTP 代理（`http_proxy=127.0.0.1:7897`），请求被代理截获返回 404

**解法：**

- Admin 映射改为 **`9181:9180`**（宿主机 9181 → 容器 9180）
- `init.sh` 所有 curl 加 **`--noproxy '*'`**
- 健康检查改为判断 HTTP status code 200，而非 `curl -sf`（4xx 也会 fail）

---

### 9.5 bash `$1` 被误解析 → proxy-rewrite 规则错误

**现象：** 路由创建成功，但转发路径错误；Admin API 返回：

```json
"regex_uri": ["^/gateway/(.*)", "/gateway-proxy"]
```

**原因：** `init.sh` 在双引号字符串中写 `"/$1"`，bash 把 `$1` 解析为函数第一个参数（route id），而非 APISIX 的 regex 反向引用。

**解法：** 用单引号 heredoc 拼接 JSON，让 `$1` 保持字面量：

```bash
admin_curl -X PUT "..." -d '{
  "plugins": {
    "proxy-rewrite": {
      "regex_uri": ["^'"${uri_prefix}"'/(.*)", "/$1"]
    }
  }
}'
```

---

### 9.6 docker-compose v1 vs v2

**现象：**

```
unknown shorthand flag: 'f' in -f
```

**原因：** 部分 WSL 环境只有 `docker-compose`（v1），没有 `docker compose`（v2 插件）。

**解法：** Makefile 自动检测：

```makefile
DOCKER_COMPOSE := $(shell command -v docker-compose >/dev/null 2>&1 && echo docker-compose || echo "docker compose")
```

---

## 10. 日常运维

### 重启 APISIX

```bash
make apisix-down
make apisix-up
make apisix-init    # etcd 数据持久化，但首次或清空后需重新 init
```

### 查看日志

```bash
make apisix-logs
# 或
docker-compose -f deploy/apisix/docker-compose.apisix.yaml logs -f apisix
```

### 手动管理 Route（Admin API）

```bash
# 列出所有 routes
curl --noproxy '*' -s http://127.0.0.1:9181/apisix/admin/routes \
  -H "X-API-KEY: edd1c9f034335f136f87ad84b625c8f1" | jq .

# 删除某个 route
curl --noproxy '*' -X DELETE http://127.0.0.1:9181/apisix/admin/routes/gateway-proxy \
  -H "X-API-KEY: edd1c9f034335f136f87ad84b625c8f1"

# 重新应用 Phase 0 全套配置
make apisix-init
```

### 清空 etcd 数据（完全重置）

```bash
make apisix-down
docker volume rm apisix_apisix_etcd_data
make apisix-up && make apisix-init
```

---

## 11. Phase 1：Gateway API Key 认证（EMS 线）

Phase 1 在 `/gateway/*` 路由上启用 `key-auth` + `limit-req`（**EMS `telemetry:ingest` 等机机路径**）。

**C10b：** `/gateway/api/v1/tenants/*/mappings*` 另有更高优先级的 **OIDC** 路由（`gateway-mappings`，priority 100），人管映射走 Casdoor，不再用 API Key。`make apisix-init` 会同时安装两条路由。详见 [`AUTHZ_TEST.md`](AUTHZ_TEST.md) §7.6。

### 11.1 配置内容

| 项 | 值 |
|----|-----|
| Consumer | `simulator_default` |
| Dev API Key | `vpp-dev-simulator-key`（见 [`conf/consumers.yaml`](../deploy/apisix/conf/consumers.yaml)） |
| 限流 | 100 req/s，burst 200（按 `remote_addr`） |
| 无 Key 访问 ingest 等 | HTTP **401** |
| 有 Key 访问 ingest | 正常转发到 gateway `:8083` |
| mappings | 需 **Bearer**（OIDC），不是 API Key |

### 11.2 应用配置

```bash
make apisix-init    # 幂等，含 Phase 0 + Phase 1 + mappings OIDC
```

### 11.3 验证

```bash
# ingest：401 — 无 Key
curl --noproxy '*' -s -o /dev/null -w '%{http_code}\n' \
  -X POST http://127.0.0.1:9080/gateway/api/v1/tenants/default/telemetry:ingest

# ingest：带 Key（需 gateway 在跑）
curl --noproxy '*' -s -o /dev/null -w '%{http_code}\n' \
  -H 'X-API-KEY: vpp-dev-simulator-key' \
  -X POST http://127.0.0.1:9080/gateway/api/v1/tenants/default/telemetry:ingest

# mappings：401 — 无 Bearer（OIDC，不再接受仅 API Key）
curl --noproxy '*' -s -o /dev/null -w '%{http_code}\n' \
  http://127.0.0.1:9080/gateway/api/v1/tenants/default/mappings

# mappings：Bearer（Casdoor）
curl --noproxy '*' -s -o /dev/null -w '%{http_code}\n' \
  -H "Authorization: Bearer $(make -s casdoor-token)" \
  http://127.0.0.1:9080/gateway/api/v1/tenants/default/mappings

# resource 需 Bearer（Phase 2 Casdoor OIDC）
curl --noproxy '*' -s -o /dev/null -w '%{http_code}\n' \
  http://127.0.0.1:9080/resource/api/tenants/default/sites
# → 401
```

### 11.4 Simulator 适配

[`config/simulator.yaml`](../config/simulator.yaml) 已改为经 APISIX 上报遥测：

```yaml
gateway:
  http-addr: http://127.0.0.1:9080/gateway
  api-key: vpp-dev-simulator-key
```

Simulator HTTP client 自动携带 `X-API-KEY` header。本地调试 Gateway 业务逻辑时仍可直连 `:8083`（绕过 APISIX）。

### 11.5 新增 EMS 厂商

通过 Admin API 创建新 consumer（每个厂商/tenant 独立 Key）：

```bash
curl --noproxy '*' -X PUT http://127.0.0.1:9181/apisix/admin/consumers/tenant-acme-ems \
  -H "X-API-KEY: <admin-key>" \
  -H "Content-Type: application/json" \
  -d '{
    "username": "tenant-acme-ems",
    "plugins": { "key-auth": { "key": "<generate-random-key>" } }
  }'
```

---

## 11.6 Phase 2：Resource Casdoor OIDC

`/resource/*` 使用 APISIX `openid-connect`（**bearer_only** + **use_jwks** + **set_userinfo_header**）。  
IdP 部署、claim 映射、Resource RBAC、端到端验收见 [`docs/CASDOOR.md`](CASDOOR.md)。

| 项 | 值 |
|----|-----|
| Discovery（容器内） | `http://host.docker.internal:8000/.well-known/openid-configuration` |
| Client | `vpp-resource-dev-client` / `vpp-resource-dev-secret` |
| 限流 | 30 req/s，burst 50 |
| 无 / 无效 Bearer | HTTP **401**（APISIX） |
| 有效 Bearer | 转发到 resource `:8082`，注入 **`X-Userinfo`** |
| Resource C3 | `auth.trust-proxy-headers: true` 时校验租户 + RBAC（缺 header → 401；越权 → 403） |

```bash
make casdoor-up && make casdoor-init
make apisix-up && make apisix-init   # 含 Phase 2 OIDC
# config/resource.yaml → auth.trust-proxy-headers: true ，再启动 Resource

# 401 — 无 Bearer
curl --noproxy '*' -s -o /dev/null -w '%{http_code}\n' \
  http://127.0.0.1:9080/resource/api/tenants/default/sites

# 200 — admin token（Resource 需在跑）
curl --noproxy '*' -s -o /dev/null -w '%{http_code}\n' \
  -H "Authorization: Bearer $(make -s casdoor-token USER=admin)" \
  http://127.0.0.1:9080/resource/api/tenants/default/sites

# 403 — 跨租户 / viewer 写操作：见 docs/CASDOOR.md §11
```

Casdoor `origin` 必须是 `host.docker.internal:8000`，否则 APISIX 容器无法拉 JWKS / userinfo（`127.0.0.1` 指向容器自身）。

---

## 11.7 gRPC + OIDC（dispatch + telemetry 读，明文 HTTP/2）

Gate 0（2026-08）已实测通过：`openid-connect` 可在 gRPC-over-HTTP/2 上验 Bearer、注入 `x-userinfo`，且客户端伪造的 `x-userinfo` 会被覆盖。

| 项 | 值 |
|----|-----|
| 入口 | `:9081` plaintext HTTP/2（h2c） |
| Dispatch | `/dispatchpb.DispatchService/*` → `:5006` |
| Telemetry 读 | `QueryTelemetry` / `GetSnapshot` / `GetFleetSnapshot` / `QueryAggregation` → `:5003` |
| Telemetry 写 | **`IngestTelemetry` 不配用户 OIDC**（gateway 本机直连） |
| 插件 | `openid-connect`（与 `/resource/*` 同参）；**不要** `proxy-rewrite` 删 `x-userinfo` |
| 限制 | Bearer 明文；**仅 localhost**；跨机请用 `:9443` TLS |

```bash
make casdoor-up && make casdoor-init
make apisix-up && make apisix-init
make run-dispatch-secured          # 另开终端可再 make run-telemetry-secured
make apisix-gate0-probe            # grpcurl + grpc-go
```

参考：[`deploy/apisix/routes/dispatch-grpc.yaml`](../deploy/apisix/routes/dispatch-grpc.yaml)、[`telemetry-grpc.yaml`](../deploy/apisix/routes/telemetry-grpc.yaml)、[`deploy/apisix/gate0/probe.sh`](../deploy/apisix/gate0/probe.sh)。启用开关前读 [`AUTHZ_RUNBOOK.md`](AUTHZ_RUNBOOK.md) §6。

---

## 12. 后续阶段规划

| 阶段 | 内容 | 依赖 |
|------|------|------|
| **Phase 0** ✅ | 透明反代 gateway/resource | 无 |
| **Phase 1** ✅ | `/gateway/*` 启用 `key-auth`（EMS API Key） | Simulator client 带 Key |
| **Phase 2** ✅ | `/resource/*` 启用 Casdoor OIDC + Resource RBAC（C3） | [docs/CASDOOR.md](CASDOOR.md) |
| **Phase 3** | APISIX metrics → Prometheus；access log → Loki | prometheus.yaml |
| **Phase 4** | K8s APISIX Ingress Controller | K8s manifests |

当前混合拓扑：APISIX 仍在 compose，upstream 已是 kind NodePort（`host.docker.internal:300xx`）。APISIX 自己进集群后，再改成 `resource.vpp.svc.cluster.local:8082` 这类 Service DNS，NodePort 可以拿掉。

---

## 12. 故障排查速查

| 现象 | 可能原因 | 处理 |
|------|---------|------|
| 容器反复 Restart | 挂载整个 conf/ 目录 | 只挂载 `conf/config.yaml` |
| `empty Admin API` | admin_key 位置错误 | 移到 `deployment.admin.admin_key` |
| `make apisix-init` 超时 | 代理拦截 / 9180 未映射 | 用 `--noproxy '*'`；确认 9181 监听 |
| 502 Bad Gateway | kind NodePort 未通 / 业务 Pod 未 Ready | `kubectl -n vpp get pods,svc`；`curl :30082/healthz` |
| 404（经 APISIX） | routes 未灌入 | `make apisix-init` |
| 转发路径不对 | proxy-rewrite `$1` 被 bash 吃掉 | 检查 init.sh 单引号拼接 |
| etcd 镜像 pull 失败 | bitnami 下架 | 用 `bitnamilegacy/etcd` |
| 401 经 APISIX `/resource` | 未带 / 无效 Bearer | `make -s casdoor-token`；检查 Casdoor origin 是否 `host.docker.internal` |
| `400 Request Header Or Cookie Too Large` | admin JWT 仍是默认整包 User | `make casdoor-init`（JWT-Custom）后重新拿 token |
| 有效 token 仍 401 | JWKS 不可达 / iss 不匹配 | APISIX 容器需能访问 `host.docker.internal:8000`；重启 Casdoor 后重拿 token |
| 有效 token 502 | Resource 未 Ready | `kubectl -n vpp get pods`；`curl :30082/healthz` |
| 403 经 APISIX `/resource` | 跨租户 path 或角色不够 | 见 [`docs/CASDOOR.md`](CASDOOR.md) §6.1 / §11 |
| Resource 经 APISIX 仍无应用层鉴权 | `trust-proxy-headers: false` | 改为 `true` 并重启 Resource |

---

## 13. 相关文档

- [`docs/K8S_DEPLOYMENT.md`](K8S_DEPLOYMENT.md) — 本机 kind 部署、暴露面、rollout / 自愈 / scale
- [`docs/CASDOOR.md`](CASDOOR.md) — Casdoor IdP + Phase 2 OIDC / Resource RBAC 联调
- [`internal/gateway/OVERVIEW.md`](../internal/gateway/OVERVIEW.md) — 业务网关职责
- [`internal/gateway/README.md`](../internal/gateway/README.md) — Gateway HTTP API
- [`architecture.md`](../architecture.md) — 全局服务拓扑（含北向 APISIX）
- [APISIX 官方 Docker 文档](https://apisix.apache.org/docs/docker/apisix-3.11.0/manual/)
