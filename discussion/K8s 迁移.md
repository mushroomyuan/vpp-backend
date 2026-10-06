应该迁，但**不是这一轮**。方案里把它们和 Postgres/Kafka 捆在一起说「数量级更大」，对 Kafka/Postgres 成立，对 Casdoor 说重了；APISIX 则确实是另一类工作。核心差别不是「该不该进 K8s」，而是**你拥有什么、状态在哪、谁当集群边缘**。

## 先把三类东西拆开

方案里「这次保留在 compose」的名单其实混了三类：

| 类型 | 例子 | 进 K8s 的本质 |
|---|---|---|
| 有状态数据面 | Postgres、Kafka、Redis | StatefulSet / PVC / Operator，丢数据就是事故 |
| 边缘 + 身份（成对） | **APISIX + etcd**、**Casdoor** | 可以进，但是产品级组件，不是你自己的无状态二进制 |
| 你自己的无状态服务 | resource / telemetry / gateway / dispatch / simulator | Deployment + Service，K8s 入门课本身 |

`ROADMAP` 写的是「现有 6 服务容器化 + 最小 manifests」。这一轮要练的，就是第三类。

## 你的 5 个服务进 K8s，实际在做什么

这是标准的「无状态应用上集群」：

- 镜像已经在 CI 里构建并推到 GHCR
- 探针已经有了（`GET /healthz`、`grpc.health.v1.Health`）
- 配置已经能用环境变量覆盖 YAML
- 进程本身**没有必须跟着 Pod 走的磁盘状态**（状态在外面的 Postgres/Kafka/Redis）
- 它们不是集群边缘：外面进不来，只让 APISIX 或 ClusterIP 打到它们

对应到 K8s，就是每份 `Deployment` + `ClusterIP Service` + `ConfigMap`/`Secret`。杀 Pod、滚动发布、`scale --replicas=2`，学的就是这些原语。工作量是「写 manifests + 把 APISIX upstream 从 `host.docker.internal:808x` 改成 kind NodePort」。

## Casdoor：比方案说的轻，但不是「再加一个 Deployment」

Casdoor 更像应用，不像 Kafka。容器本身相对无状态，用户/组织/权限/JWT 证书都在 Postgres 的 `casdoor` 库里。Postgres 继续留在 compose 的话，**单独把 Casdoor 放进 K8s 并不是 StatefulSet 工程**。

真正多出来的是 **OIDC 契约**，不是 YAML 行数：

1. **`origin` 就是 issuer。** 现在 `app.conf` 里是 `origin = http://host.docker.internal:8000`。JWT 的 `iss`、discovery、JWKS、登录跳转都绑在这个 URL 上。一改，已发 token 失效，APISIX 的 `openid-connect` discovery 要改，各服务的 `casdoor-url` 要改。
2. **APISIX 和 Casdoor 是一对。** APISIX 要从自己的网络里拉 `/.well-known/openid-configuration` 和 JWKS。现在两者都在 Docker 里，走 `host.docker.internal:8000`。只迁 Casdoor、APISIX 留在 compose，等于跨 kind/compose 边界做 OIDC；只迁 APISIX、Casdoor 留在外面，同样别扭。
3. **你不拥有它的配置模型。** 它吃 `app.conf` + 首次空库导入的 `init_data.json`（`initDataNewOnly = true`），不是你那套 `DATABASE_HOST` 覆盖。镜像是 `casbin/casdoor:3.125.0`，探针、就绪条件、启动时建表都是别人的语义。
4. **它还要给人打开 UI。** `:8000` 要 NodePort/Ingress；业务 5 个服务这轮故意不直接暴露。

所以 Casdoor 单独进 K8s（库仍在 compose）大概是「一个服务的 2～3 倍」，痛点是身份 URL 全链路，不是数量级。方案把它和 Kafka 写在同一句里，是 bundled 过头了。

## APISIX：这才是另一档工作

APISIX 不是「再写一份 Deployment」。它现在是：

```
APISIX 进程  +  etcd（路由/upstream/consumer 的真实状态）
监听 :9080 HTTP / :9081 gRPC h2c / :9181 Admin
make apisix-init 用 Admin API 灌路由
```

多出来的是三件你的 Go 服务完全没有的事。

**1. 它带着一个有状态的配置库。**  
路由不在 git 里生效，而在 etcd volume `apisix_etcd_data` 里。APISIX 进集群、etcd 留在 compose，网关和配置中心被拆开，比现在更差。etcd 一起进，就要 PVC、备份、重启丢路由——这才接近「有状态基础设施」。

**2. 要先选部署形态，这是架构决策，不是搬容器。**

| 形态 | 含义 | 对你意味着 |
|---|---|---|
| 传统模式（你现在这样） | APISIX + etcd，Admin API + `init.sh` | 要在 K8s 里复刻 etcd + 灌路由 Job，Gate 0 的 `:9081` h2c 还得自己挂端口 |
| APISIX Ingress Controller | 路由变成 `Ingress` / `ApisixRoute` CRD | `init.sh`、consumers.yaml、Admin API 工作流整段换掉；Helm chart、CRD、控制面/数据面 |

你已经在传统模式上做完 OIDC、`key-auth`、gRPC h2c Gate 0。迁 Ingress Controller 是换产品用法，不是 `kubectl apply` 换个镜像。

**3. 它是边缘，5 个服务不是。**  
现在外面只打 APISIX 的 `:9080/:9081`，再转到 host 上的业务端口。这轮方案是：业务进集群藏到 ClusterIP，APISIX 留在 host 边上看着 kind NodePort。若 APISIX 也进集群，就要回答「谁把 9080/9081 暴露到 Windows/WSL」——kind 没有云厂商 LoadBalancer，只能 NodePort / extraPortMappings / 再套一层。网关自己还要被网关出去，这是边缘组件特有的问题。

另外，你现有的 `deploy/apisix/init.sh`、`gate0/probe.sh`、OIDC discovery `http://host.docker.internal:8000`、upstream `host.docker.internal:808x`，全部假设「APISIX 在 Docker、业务在 host」。进集群后 upstream 反而更干净（`resource.default.svc.cluster.local`），但那是迁完的收益，不是迁的成本变小。

## 「数量级」到底指什么

更准确的说法是：

> 把 **Postgres + Kafka + Redis + APISIX/etcd + Casdoor + 可观测性栈** 整包迁进 K8s，才是数量级；其中 Kafka/Postgres 用 Operator、PVC、备份就能单独开一个项目。

不是「Casdoor 一个 Deployment ≈ 你五个服务加起来」。  
也不是「APISIX 不该进 K8s」——生产里 APISIX 当 Ingress、Casdoor 当 Deployment 才是常态。

这一轮刻意留下它们，是因为：

1. **学习顺序：** 先掌握无状态 Deployment/Service/探针/滚动发布；再学边缘、CRD、有状态。
2. **APISIX + Casdoor 应成对移动。** 拆开迁会在 kind 和 compose 之间再拆一条 OIDC/JWKS 路径，比现在更绕。
3. **你已经有一套能跑的边缘。** Gate 0、OIDC、`key-auth` 都在 compose 里验证过。这轮验收是「业务端口不再听在 host 上，只能经 APISIX 进 ClusterIP」，APISIX 留在外面正好当这个边界。
4. **混合拓扑是常见过渡：** 无状态应用进集群，IdP/网关/数据面先留在已经跑稳的地方。

## 以后若要迁，建议顺序

1. **先把 5 个服务在 kind 里跑稳**（当前方案）。APISIX 继续在 compose，upstream 改指向 NodePort。
2. **若只加一样：** 更合理的是 APISIX（和 etcd）进集群——upstream 改成 in-cluster DNS，边缘仍只有一处暴露。Casdoor 仍在 compose 时，让 APISIX Pod 去打 host 上的 `:8000`。
3. **Casdoor 跟着进**（Postgres 仍可在外）：改 `origin`/discovery，重灌 JWKS，回归 OIDC。
4. **Postgres/Kafka 最靠后**，那才是 StatefulSet/Operator 那一档。

一句话：**该进，而且生产里它们比业务服务更常住在 K8s 里；这一轮不进，是因为 APISIX 是带 etcd 的边缘产品、Casdoor 和它绑在 OIDC 上，两者应成对迁移，而当前目标是先学会部署你自己的无状态服务。**