# 授权联调测试指南（Casdoor ↔ Resource）

> 对应阶段：**C5–C9**（权限目录 / 本地 Casbin PDP / resource PEP / 观测 / 目录自动注册）。  
> 架构说明见 [`AUTHZ_CENTRALIZATION_PLAN.md`](AUTHZ_CENTRALIZATION_PLAN.md)；Casdoor 部署见 [`CASDOOR.md`](CASDOOR.md)。

---

## 1. 先分清交互链路

| 链路 | 谁连 Casdoor | 干什么 | 何时发生 |
|------|--------------|--------|----------|
| **A. 认证（OIDC）** | APISIX | 校验 Bearer JWT，注入 `X-Userinfo` | **每个**业务请求 |
| **B. 授权同步（B1）** | resource 的 `Syncer` | session 登录 + `get-permissions`，刷新本地 Casbin | 启动时 + 默认每 **30s** |
| **C. 目录注册（C9）** | resource 启动 | `add-permission` / `update-permission` upsert 目录条目 | **启动一次**（先于 sync） |

要点：

- resource **请求热路径不直连 Casdoor**；只读本地 Enforcer。
- 在 Casdoor UI 改权限后，要等一次 sync（最多约一个 `sync-interval`）才会反映到服务。
- 启动日志应出现 `authz catalog registered: added=…`（可在 Casdoor Permissions 看到 `catalog-resource-sites-read` 等；**Roles 默认为空**，不影响现有 `vpp-resource-*` 绑定）。
- 直连 `:8082` 且 `trust-proxy-headers: false` 时，**整段鉴权旁路**，测不到上述链路。

```text
客户端
  │  Authorization: Bearer <token>
  ▼
APISIX :9080 /resource/*     ← 链路 A（OIDC）
  │  X-Userinfo
  ▼
Resource :8082               ← 本地 Casbin（策略来自链路 B）
  │
  ├── 启动 RegisterCatalog ──► Casdoor add/update-permission（链路 C）
  └── Syncer ──定时──► Casdoor :8000 /api/get-permissions
```

---

## 2. 环境准备

### 2.1 拉起依赖

```bash
cd /home/yfz/project/vpp-backend   # 或你的仓库根目录

make infra-up                      # Postgres 等（已起可跳过）
make casdoor-up && make casdoor-init
make apisix-up && make apisix-init
```

自检：

```bash
curl -s -o /dev/null -w '%{http_code}\n' http://127.0.0.1:8000/api/health   # 期望 200
make casdoor-status
make apisix-status
```

Casdoor 控制台：http://127.0.0.1:8000  

| 用途 | 账号 |
|------|------|
| 控制台 / Syncer 管理 API | org `built-in`，用户 `admin` / `123` |
| 业务 Password Grant（拿 token） | org `default`：`admin` / `operator` / `viewer`（密码见 [`deploy/casdoor/conf/credentials.yaml`](../deploy/casdoor/conf/credentials.yaml)） |

### 2.2 确认 C5 种子（Model + Permission）

UI：Models 可见 `vpp-rbac`；Permissions 可见：

- `vpp-resource-read`
- `vpp-resource-write`
- `vpp-resource-admin`

若缺失（旧库 `initDataNewOnly` 不会重灌），本地重灌：

```bash
make casdoor-down
docker exec vpp-backend-postgres-1 \
  psql -U postgres -c 'DROP DATABASE IF EXISTS casdoor WITH (FORCE);'
docker exec vpp-backend-postgres-1 \
  psql -U postgres -c 'CREATE DATABASE casdoor;'
make casdoor-up && make casdoor-init
```

### 2.3 打开 Resource 鉴权并启动

编辑 [`config/resource.yaml`](../config/resource.yaml)：

```yaml
auth:
  trust-proxy-headers: true   # 必须为 true，否则测不到鉴权 / 同步
  authz:
    casdoor-url: http://127.0.0.1:8000
    # …其余见该文件；Syncer 使用 built-in admin/123
```

```bash
make run-resource
```

期望日志：

```text
authz syncer configured (casdoor=http://127.0.0.1:8000 ...)
authz policy sync ok
```

成功同步后应出现快照文件（默认）：

```text
./data/resource-authz-snapshot.json
```

---

## 3. 单元测试（不依赖 Casdoor 进程）

```bash
cd internal/platform && go test ./authz/ ./middleware/ -count=1
cd ../resource && go test ./adapter/inbound/http/ -count=1
```

覆盖：C3 等价矩阵、`resourceOf`/`actionOf`、冷启动安全网、失效 fail-closed 等。

---

## 4. 端到端：APISIX → Resource（链路 A + 本地 PDP）

**入口统一用** `http://127.0.0.1:9080/resource/...`（经 APISIX）。  
`--noproxy '*'` 避免本机代理干扰。

### 4.1 门卫与角色矩阵

```bash
# ① 无 token → 401（APISIX）
curl --noproxy '*' -s -o /dev/null -w '%{http_code}\n' \
  http://127.0.0.1:9080/resource/api/tenants/default/sites

# ② admin 读 → 200（或业务 JSON，非 401/403）
curl --noproxy '*' -s -o /dev/null -w '%{http_code}\n' \
  -H "Authorization: Bearer $(make -s casdoor-token USER=admin)" \
  http://127.0.0.1:9080/resource/api/tenants/default/sites

# ③ 跨租户 path → 403（token owner=default）
curl --noproxy '*' -s -o /dev/null -w '%{http_code}\n' \
  -H "Authorization: Bearer $(make -s casdoor-token USER=admin)" \
  http://127.0.0.1:9080/resource/api/tenants/other/sites

# ④ viewer 写 → 403（本地 Casbin）
curl --noproxy '*' -s -o /dev/null -w '%{http_code}\n' \
  -H "Authorization: Bearer $(make -s casdoor-token USER=viewer)" \
  -H 'Content-Type: application/json' \
  -d '{"name":"x"}' \
  http://127.0.0.1:9080/resource/api/tenants/default/sites

# ⑤ operator 写 → 非 403（可能 200 或业务校验错误）
curl --noproxy '*' -s -o /dev/null -w '%{http_code}\n' \
  -H "Authorization: Bearer $(make -s casdoor-token USER=operator)" \
  -H 'Content-Type: application/json' \
  -d '{"name":"demo-site"}' \
  http://127.0.0.1:9080/resource/api/tenants/default/sites

# ⑥ operator 删 → 403
curl --noproxy '*' -s -o /dev/null -w '%{http_code}\n' \
  -H "Authorization: Bearer $(make -s casdoor-token USER=operator)" \
  -X DELETE \
  http://127.0.0.1:9080/resource/api/tenants/default/resources/dummy
```

种子策略对齐的期望矩阵：

| 角色 | GET | POST/PUT/PATCH | DELETE / `:changeLifecycle` |
|------|-----|----------------|------------------------------|
| viewer | 允许 | 拒绝 | 拒绝 |
| operator | 允许 | 允许 | 拒绝 |
| admin | 允许 | 允许 | 允许 |

### 4.2 Claim 自检

```bash
make -s casdoor-token USER=admin DECODE=1
```

确认 `owner`→租户、`roles[].name`→角色，与 [`CASDOOR.md`](CASDOOR.md) §7.5 一致。

### 4.3 直连调试（旁路）

保持 `trust-proxy-headers: false`，访问：

```text
http://127.0.0.1:8082/api/tenants/...
```

**不要**伪造 `X-Userinfo` 当正式鉴权。

---

## 5. 专测链路 B：改 Casdoor 是否影响服务（集中管理）

验证「改权限不改 resource 代码」。

1. 浏览器登录 Casdoor → **Permissions** → 编辑 `vpp-resource-write`  
   - 暂时从 Roles 中去掉 `default/operator`（只留 admin）。
2. 等待 ≤ `sync-interval`（默认 30s），或看 resource 日志再次出现 `authz policy sync ok`。
3. 复测 operator 写：

```bash
curl --noproxy '*' -s -o /dev/null -w '%{http_code}\n' \
  -H "Authorization: Bearer $(make -s casdoor-token USER=operator)" \
  -H 'Content-Type: application/json' \
  -d '{"name":"should-deny"}' \
  http://127.0.0.1:9080/resource/api/tenants/default/sites
# 期望：403
```

4. 在 Casdoor 把 `default/operator` 加回 → 再等 sync → 应变回允许。

可选：对比 `./data/resource-authz-snapshot.json` 在 sync 前后是否更新。

---

## 6. 降级 / 失联（可选）

| 场景 | 做法 | 期望 |
|------|------|------|
| Syncer 拉不到策略 | `make casdoor-down`；测试可将 `stale-after` 临时改为 `1m` | 超过硬阈值后写操作 fail-closed（403）；日志有 sync failed |
| 冷启动无快照 | 删除 snapshot，Casdoor 不可用时再启 resource | 仅 `admin` 安全网放行；viewer 等 403 |
| 有快照重启 | Casdoor 短时不可用但 snapshot 仍在 | 按快照时间戳走健康/过期档，短时仍可用旧策略 |

降级语义详见计划书 §6 与 `platform/authz` 单测。

---

## 7. 最小验收清单

- [ ] `casdoor-init` 通过；UI 可见 `vpp-rbac` 与三条 Permission  
- [ ] `trust-proxy-headers: true` + `run-resource`；日志有 `policy sync ok`  
- [ ] §4.1 ①–⑥ 结果符合矩阵  
- [ ] §5 改 Casdoor 绑定后，不重启代码即可改变 operator 写权限  
- [ ] （可选）§6 失联 / 冷启动行为符合 fail-closed  
- [ ] `curl -s http://127.0.0.1:9102/metrics | grep authz_` 可见 sync / tier / decision 指标（**C8**）  

运维告警与处置见 [`AUTHZ_RUNBOOK.md`](AUTHZ_RUNBOOK.md)。

---

## 8. 常见问题

| 现象 | 原因 / 处理 |
|------|-------------|
| 直连 `:8082` 像没鉴权 | `trust-proxy-headers: false`（预期） |
| 改了 Casdoor 立刻不生效 | 仍在 sync 周期内；等 `policy sync ok` |
| sync `rules=0` / Permission 空 | 旧库无 C5 种子；按 §2.2 重灌 |
| APISIX 401，token 看似正确 | discovery/JWKS；`make casdoor-init`，必要时重启 APISIX |
| Syncer 登录失败 | `auth.authz` 用 **built-in admin / 123**，不是业务用户 `vpp-*-dev` |
| 403 `authorization unavailable or policy stale` | 策略档位失效且 fail-closed；查 sync 与 `stale-after` |

---

## 9. 相关路径

| 项 | 路径 |
|----|------|
| 本指南 | `docs/AUTHZ_TEST.md` |
| 改造计划 | [`AUTHZ_CENTRALIZATION_PLAN.md`](AUTHZ_CENTRALIZATION_PLAN.md) |
| Casdoor 部署 / C4 清单 | [`CASDOOR.md`](CASDOOR.md) §11 |
| Resource 配置 | [`config/resource.yaml`](../config/resource.yaml) |
| PEP + 目录映射 | `internal/resource/adapter/inbound/http/{auth,catalog}.go` |
| 本地 PDP / Syncer | `internal/platform/authz/` |
| 策略快照（运行时） | `./data/resource-authz-snapshot.json` |
