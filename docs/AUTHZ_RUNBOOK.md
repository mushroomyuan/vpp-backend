# Authz 运维 Runbook（C8）

策略同步失败或档位降级时，按本页处理。架构见 [`AUTHZ_CENTRALIZATION_PLAN.md`](AUTHZ_CENTRALIZATION_PLAN.md) §6；联调见 [`AUTHZ_TEST.md`](AUTHZ_TEST.md)。

---

## 1. 关键指标（resource `:9102/metrics`；dispatch `:9105/metrics`）

| 指标 | 含义 |
|------|------|
| `authz_policy_sync_last_success_timestamp{service=}` | 上次成功 sync 的 Unix 秒 |
| `authz_policy_sync_failures_total{service=}` | sync 失败累计 |
| `authz_policy_sync_successes_total{service=}` | sync 成功累计 |
| `authz_policy_stale_seconds{service=}` | 距上次成功秒数；从未成功为 `-1` |
| `authz_policy_loaded{service=}` | 本地是否已有 p 规则（`1`/`0`） |
| `authz_policy_tier{service=,tier=healthy\|stale\|invalid}` | 当前档位（活跃档为 `1`） |
| `authz_decision_total{service=,result=allow\|deny\|degraded_allow\|degraded_deny}` | PEP 决策计数 |

快速查看：

```bash
curl -s http://127.0.0.1:9102/metrics | grep '^authz_'
```

Prometheus 告警规则：[`config/prometheus-authz-alerts.yaml`](../config/prometheus-authz-alerts.yaml)（由 `config/prometheus.yaml` 的 `rule_files` 加载）。

默认档位阈值：

- **resource**（`auth.authz`）：健康 &lt; **5m** ≤ 过期 &lt; **30m** ≤ 失效  
- **dispatch**（控制类，C10a）：健康 &lt; **1m** ≤ 过期 &lt; **5m** ≤ 失效；且 `deny-writes-when-stale: true`（过期档拒绝 `submit`/`cancel`）

---

## 2. 告警与处置

### AuthzPolicySyncFailing（warning）

**含义：** 近 10 分钟 sync 失败多次。

**排查：**

1. Casdoor 是否存活：`curl -s http://127.0.0.1:8000/api/health`；`make casdoor-status`
2. resource 日志：`authz policy sync failed` / `authz initial policy sync failed`
3. Syncer 凭据是否为 **built-in admin / 123**（`config/resource.yaml` → `auth.authz.casdoor-*`），不是业务用户 `vpp-*-dev`
4. 网络：resource 能否访问 `auth.authz.casdoor-url`

**处置：** 恢复 Casdoor 后等待下一轮 sync（默认 30s）；确认 `authz_policy_sync_successes_total` 增加、`failures` 不再上升。

### AuthzPolicyStale（warning）

**含义：** 仍在用旧缓存（过期档），尚未到硬阈值。

**处置：** 同「SyncFailing」排查链路；业务仍可按旧策略运行，但权限回收可能延迟。优先恢复 sync，避免滑入 invalid。

### AuthzPolicyInvalid（critical）

**含义：** `staleness ≥ stale-after` 或从未成功同步后进入失效档；**写操作 fail-closed**（默认只读也不放行，除非 `allow-read-when-invalid: true`）。

**处置：**

1. 立即恢复 Casdoor + 确认 sync 成功  
2. 若长时间无法恢复：评估是否暂停北向变更类操作；**不要**为图省事把 `trust-proxy-headers` 设回 `false` 当生产旁路  
3. 客户端会看到 403，文案可能含 `authorization unavailable or policy stale`

### AuthzPolicyNotLoaded（critical）

**含义：** 无策略规则 + invalid → **冷启动安全网**（默认仅 `admin` 放行）。

**处置：**

1. 确认 `./data/resource-authz-snapshot.json`（或配置的 `snapshot-path`）是否存在且可读  
2. Casdoor Permission 是否仍有 C5 种子（`vpp-resource-*`）；缺失则按 [`CASDOOR.md`](CASDOOR.md) / 计划书重灌  
3. Syncer 登录与 `get-permissions` 是否返回非空  

---

## 3. 日志信号

| 日志 | 级别 | 含义 |
|------|------|------|
| `authz policy sync ok` | INFO | 本轮拉取并刷新成功 |
| `authz policy sync failed` | ERROR | 本轮失败（已计 `failures_total`） |
| `authz policy sync tier degraded` | ERROR | 档位迁入 `stale` / `invalid` |
| `authz decision made in degraded mode` | WARN | PEP 在非健康档放行了请求 |

---

## 4. 常用命令

```bash
# 指标
curl -s http://127.0.0.1:9102/metrics | grep authz_

# Casdoor
make casdoor-status
make casdoor-logs

# 重灌种子（会清空 casdoor 库，仅本地）
make casdoor-down
docker exec vpp-backend-postgres-1 \
  psql -U postgres -c 'DROP DATABASE IF EXISTS casdoor WITH (FORCE);'
docker exec vpp-backend-postgres-1 \
  psql -U postgres -c 'CREATE DATABASE casdoor;'
make casdoor-up && make casdoor-init

# 看快照
ls -la ./data/resource-authz-snapshot.json
```

Prometheus UI：http://127.0.0.1:9090 → Status → Rules / Alerts（需 compose 已挂载告警文件并重启 prometheus）。

---

## 5. 升级 Prometheus 配置后

```bash
docker compose up -d prometheus
# 或
curl -X POST http://127.0.0.1:9090/-/reload   # 若开启了 lifecycle
```

确认容器内存在 `/etc/prometheus/prometheus-authz-alerts.yaml`，且 Rules 页能看到 `vpp-authz` 组。
