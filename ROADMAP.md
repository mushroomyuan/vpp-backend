# VPP Backend 开发路线图

> 本文档整理自与 AI 的多轮架构讨论（详见 `discussion.md`）和 2026-07 的架构复审。
> 用途：跟踪阶段性目标，按阶段打勾推进，不是一次性写死的计划——排期和范围可以随时调整。

---

## 当前状态（2026-07 复审后）

6 个核心服务（resource / telemetry / gateway / dispatch / simulator + platform 共享库）已跑通端到端主链路，均可编译、单测通过。

**北向接入已完成，且深度超出原计划：**

- APISIX 作为基础设施网关：`/gateway/*` key-auth（EMS 机机）+ `/resource/*`、`/gateway/.../mappings*` Casdoor OIDC（人管）
- Casdoor 作为 IdP + PAP：签发 JWT，托管 Role/Permission
- 本地 Casbin 作为 PDP：各服务本地决策，不依赖 Casdoor 请求热路径可用性
- 分级 fail-closed 降级（健康/过期/失效三档）+ 冷启动安全网 + 磁盘快照，dispatch 控制面阈值比 resource 管理面更严格
- 已推广到 resource（C7）、dispatch（C10a）、gateway mappings（C10b）、telemetry（C10c）

详见 `docs/CASDOOR.md`、`docs/APISIX.md`、`docs/AUTHZ_CENTRALIZATION_PLAN.md`。

**2026-07 架构复审结论：无阻塞性问题。** 已识别的后续关注点见下方「已知限制 / 待处理」。

---

## 已知限制 / 待处理（架构复审产出）

| 项 | 说明 | 处理方式 |
|---|---|---|
| dispatch/telemetry 的 gRPC PEP 缺少可信接入点 | 曾缺等价于 APISIX OIDC 的验签注入 | **已解决（2026-08 Gate 0+）**：`:9081` h2c + Path C OIDC；dispatch 全方法 + telemetry 读 RPC；Ingest 仍机机直连。默认 `trust-proxy-headers: false`；联调用 `make run-*-secured`。勿用 `proxy-rewrite` 删 `x-userinfo`。详见 `docs/APISIX.md` §11.7、`AUTHZ_RUNBOOK.md` §6 |
| 是否要把 HTTP+gRPC 一起升级为服务自验签 JWT 的零信任模型 | 复审时讨论过「gRPC 单独自验签、HTTP 不变」会破坏架构一致性（两条协议对"后端信任什么"给出不同答案），因此否决了这个局部方案 | 列为独立的未来架构决策，如果要做必须 HTTP + gRPC 一起升级，不在当前 gRPC 补洞范围内 |
| APISIX 是否覆盖客户端伪造的 `X-Userinfo` | 理论上 `ngx.req.set_header` 是覆盖语义，未实测验证 | 建议做一次实验：合法 Bearer + 伪造 `X-Userinfo` 同时发送，确认后者被覆盖 |
| 业务端口直接监听在 host 网络 | 旧 `:8082/:8083/:500x` 已离开 host（不再 `make run-*`）。kind extraPortMappings 仍把 NodePort `:30082/:30083/:30003/:30006` 映到 WSL，供 compose APISIX hairpin；直连这些口仍绕过 APISIX。simulator 仅 ClusterIP。 | **本轮已收口旧 host 口**（`make k8s-exposure-check`）。真正只经 APISIX 要等 APISIX 进集群后拿掉 extraPortMappings。 |
| 内部服务间 gRPC（dispatch→gateway、gateway→telemetry）无鉴权/mTLS | 依赖同网络可信假设，perimeter security 模型 | 暂不处理，作为已知设计取舍；长期可在 K8s + service mesh 阶段一并解决东西向身份问题 |
| 角色模型（admin/operator/viewer）为占位 | 真实业务角色/权限模型尚未确定 | 不阻塞现有架构，等业务角色明确后在 Casdoor 侧调整 Permission 绑定即可，不需要改代码 |
| CI 已构建并推送镜像到 GHCR，但镜像本身尚不能直接部署 | manifests、探针、env 覆盖均已落地（本机 kind）。 | **已解决（2026-08）**：见 [`docs/K8S_DEPLOYMENT.md`](docs/K8S_DEPLOYMENT.md) |
| Consul 运行时 | compose 已删除；`consul-addr` 默认空，跳过注册。封装留在 `platform/discovery`。 | **已从运行时移除（2026-08）** |

---

## Phase A · 基建 + 补缺口

- [x] 认证鉴权 / 用户中心（Casdoor + APISIX + 本地 Casbin，深度超出原计划的 scoped 版本）
- [x] CI/CD（GitHub Actions：`.github/workflows/ci.yml` 三个 job——`lint`/`test` 按 6 模块矩阵跑，`docker` 构建 5 个服务镜像并推送 GHCR；`Makefile` 补齐 `test`/`vet`/`docker-build`，修复 `make lint`）
- [x] 集成测试（`tests/integration`：testcontainers 起 Postgres×2 + Kafka，bufconn 打通 dispatch↔gateway，覆盖 SubmitTask→ExecuteCommand→Kafka 回调主链路的正向 + mapping 缺失异常路径；`.github/workflows/ci.yml` 新增 `integration-test` job，`docker` job 依赖它；顺带补上 gRPC health server + HTTP `/healthz`，见上表）
- [ ] Alarm 服务（消费 `vpp.dispatch.events` / `vpp.soe.events`，工作量小、闭环价值高，可随时插入）
- [ ] `CancelTask`（dispatch，独立功能补齐，可随时插入）
- [x] K8s 基础部署（本机 kind：5 个无状态服务 GHCR manifests + NodePort 接 APISIX；旧 host `:808x/:500x` 已收口。Consul 已从运行时移除，发现走 Service DNS。用法见 [`docs/K8S_DEPLOYMENT.md`](docs/K8S_DEPLOYMENT.md)）

**建议顺序：** CI/CD → 集成测试（补进 CI）→ **K8s 基础部署（已完成）** → Alarm / CancelTask（可随时插入）。

---

## Phase B · 业务闭环

- [ ] **gateway 定位澄清（复审新增）**：当前管理端对 resource 的资源管理、对 dispatch 的任务提交都是直连（经 APISIX 统一认证），完全不经过 gateway；gateway 实际定位一直是"EMS/外部系统适配器"，不是"所有对外流量的统一业务网关"。这不是 APISIX 上线后才产生的问题，是既有设计边界，只是现在更显眼了。需要决定：①是否要在文档里把 gateway 的措辞改得更精确（如 "EMS Integration Adapter"），避免望文生义；②Onboarding 这类跨服务编排逻辑目前下放给"管理端显式两步调用"，等 Optimization/Forecast 落地、需要组合调用多个服务时，是否需要一个真正的业务编排层（可以是重新定位 gateway，也可以是新建 BFF/orchestration 服务）——现在讨论还太早，缺乏真实编排复杂度做参照，留到本阶段结合新服务的实际接入方式一起判断
- [x] APISIX gRPC 网关（`:9081` Path C OIDC；dispatch + telemetry 读；Gate 0 已通过）
- [ ] Forecast v1（故意简单的占位算法，如同比/移动平均，纯粹是为了让 Optimization 有输入）
- [ ] Optimization v1（规则/阈值策略，整合 Telemetry 读状态 + Forecast 读预测 + Dispatch.SubmitTask 下发决策）

---

## Phase C · 事件溯源

- [ ] 事件溯源/审计存储服务：消费 `vpp.resource.events` / `vpp.command.events` / `vpp.dispatch.events` / `vpp.soe.events` 全部落库，对外提供统一查询（"某个 CommandID/TaskID 完整生命周期"类审计问题）。纯新增消费者，不改动现有生产者，风险低
- [ ] 压测（放在 K8s + HPA 环境下做，比测本地进程更有说服力）

---

## Phase D · 加分项 / 靠后

- [ ] Market 服务（"薄壳"：出价/清算/结算记录，复用已存在的 `Asset.MarketEnabled` 字段，不建模真实市场规则）
- [ ] AI 能力见缝插针：最自然的落点是 Forecast——用真实小模型（Prophet/线性回归/调一次 LLM 做异常解释）替换 naive 算法，不需要动其他服务接口

---

## 保持空白（明确不做）

- 电价预测的复杂算法（超出当前技能范围）
- 真实 EMS 对接（无实际设备/EMS 可用）

---

## 框架/工具箱引入原则（讨论结论，非强制）

不做整体框架迁移（go-zero / Kratos / Kitex 等）。当前手写的 `platform/decorator`（泛型 Handler/Middleware/Chain）、`platform/server`、`platform/discovery` 已经是同等抽象层次的轻量框架骨架，且更能体现对六边形架构/CQRS/DDD 的主动理解，替换成框架反而会稀释这个信号。

如果确实想体验某个框架，推荐方式：在全新服务（如 Forecast）里从零尝试 Kratos，与其他手写服务并存对比，而不是引入到现有六个服务中。限流/熔断可以针对性引入 `golang.org/x/time/rate` 或 `sony/gobreaker`，作为 `decorator.Chain` 的新增 Middleware，不需要引入整框架。
