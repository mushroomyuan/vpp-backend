# 本机 kind 部署与基础操作

> 5 个无状态业务服务跑在 kind 单节点里；Postgres / Kafka / Redis / APISIX / Casdoor / 可观测性栈仍在 Docker Compose。  
> 网络心智模型见 [`discussion/k8s网络拓扑.md`](../discussion/k8s网络拓扑.md)。  
> 配置如何被环境变量盖掉见 [`docs/CONFIG_ENV_OVERRIDE.md`](CONFIG_ENV_OVERRIDE.md)。

---

## 1. 范围

**在 kind 里：** `resource` / `telemetry` / `gateway` / `dispatch` / `simulator`  
镜像来自 CI：`ghcr.io/mushroomyuan/vpp-backend/<service>:latest`。

**仍在 compose：** Postgres/TimescaleDB、Kafka、Redis、APISIX、Casdoor、Prometheus/Grafana/Loki/Jaeger。

这不是偷懒。ROADMAP 这一项本来就是「业务服务最小 manifests」，不是把有状态基础设施一并迁进集群。

```
浏览器 / grpcurl
        │
        ▼
APISIX :9080 / :9081          ← 还在 Compose
        │  host.docker.internal:300xx（hairpin）
        ▼
kind NodePort → ClusterIP → Pod
```

集群内互调走 `*.vpp.svc.cluster.local`。Pod 出集群打基础设施走 `host.docker.internal`（`host-aliases-patch.yaml` 写死 `192.168.65.254`）。

---

## 2. 集群与部署

```bash
# WSL 若开了 HTTP_PROXY=127.0.0.1:7897，建集群时改写成 Docker Desktop 宿主机别名：
HTTP_PROXY=http://host.docker.internal:7897 \
HTTPS_PROXY=http://host.docker.internal:7897 \
kind create cluster --config deploy/k8s/kind-cluster.yaml
# kubectl 上下文：kind-vpp

make infra-up
make casdoor-up && make casdoor-init
make apisix-up && make apisix-init
make k8s-apply                 # kubectl apply -k deploy/k8s/base
kubectl -n vpp get pods,svc
make k8s-exposure-check        # 旧 :808x/:500x 应已离开 host
```

销毁（会删掉集群里所有业务 Pod，compose 不受影响）：

```bash
kind delete cluster --name vpp
# 或只撤业务：make k8s-delete
```

`imagePullPolicy: Always`，`make k8s-apply` 之后若要吃到新的 GHCR `:latest`：

```bash
kubectl -n vpp rollout restart deploy/resource deploy/telemetry deploy/gateway deploy/dispatch deploy/simulator
```

---

## 3. Manifest 结构

```
deploy/k8s/
  kind-cluster.yaml              # 单节点 + extraPortMappings 30082/30083/30003/30006
  base/
    kustomization.yaml           # namespace: vpp
    namespace.yaml
    secret.yaml                  # postgres / casdoor / simulator API key
    configmap-infra.yaml         # DATABASE_HOST / KAFKA_BROKERS=host.docker.internal:29092 …
    host-aliases-patch.yaml      # 所有 Deployment：host.docker.internal → 192.168.65.254
    resource/ telemetry/ gateway/ dispatch/ simulator/
      deployment.yaml
      service.yaml
      configmap.yaml             # 该服务互调地址、Casdoor URL
      kustomization.yaml
```

没有 Helm。一套本机环境，kustomize 足够。

| 地址该写成 | 例子 |
| ---------- | ---- |
| 自己监听 | `RESOURCE_HTTP_ADDR=$(POD_IP):8082`（探针打 Pod IP，不能只绑 `127.0.0.1`） |
| 调别的业务 | `telemetry.vpp.svc.cluster.local:5003` |
| 调 compose 基础设施 | `host.docker.internal:5432` / `:29092` / `:8000` |

simulator 绑 `0.0.0.0:8084`。Service 仍是 ClusterIP，不映射到 host。

### 端口

| 角色 | 地址 |
| ---- | ---- |
| 用户入口 | APISIX `:9080` HTTP、`:9081` gRPC h2c |
| APISIX → kind | `host.docker.internal:30082` resource HTTP、`:30083` gateway HTTP、`:30003` telemetry gRPC、`:30006` dispatch gRPC |
| 集群内 | ClusterIP + 上表端口号（8082/8083/5003/5006/8084） |
| 本机 `make run-*`（可选） | 仍是 `:8082` 等；覆盖 `GATEWAY_UPSTREAM` 等再 `make apisix-init` |

host `:30082/healthz` 能通，但**绕过 APISIX**。验收脚本：`make k8s-exposure-check`。

Docker Desktop 升级后若 `host.docker.internal` 不再是 `192.168.65.254`：

```bash
docker exec vpp-control-plane getent hosts host.docker.internal
# 改 deploy/k8s/base/host-aliases-patch.yaml 再 make k8s-apply
```

---

## 4. 基础操作指南

下面都在 namespace `vpp`。演示用 **simulator**：无 Kafka 消费组，scale 最安全。  
`resource` / `gateway` / `dispatch` 先保持 `replicas: 1`（Kafka 消费者语义还没按多副本设计）。

常用查看：

```bash
kubectl -n vpp get pods,svc,deploy
kubectl -n vpp describe pod -l app.kubernetes.io/name=simulator
kubectl -n vpp logs -l app.kubernetes.io/name=simulator --tail=50
kubectl -n vpp get events --sort-by=.lastTimestamp | tail
```

### 4.1 滚动重启 `rollout restart`

改的是 Pod 模板的一次「重新拉起」，Deployment 会起新 Pod、等 Ready、再停旧 Pod。本机已实测 simulator 大约十几秒内完成。

```bash
kubectl -n vpp rollout restart deploy/simulator
kubectl -n vpp rollout status deploy/simulator --timeout=90s
kubectl -n vpp get pods -l app.kubernetes.io/name=simulator
# 名字里的 ReplicaSet hash 会变，例如 7897599c5b → 5d9b875986
```

吃新镜像、或只想无痛重启某一个服务时用这个，不要先 `delete deploy`。

### 4.2 删 Pod：自愈

Deployment 的职责是「始终有 `replicas` 个 Ready」。你删掉当前 Pod，ReplicaSet 会再造一个，**名字不同、IP 通常也不同**。Service 不需要改，kube-proxy 转到新 Endpoints。

```bash
kubectl -n vpp get pod -l app.kubernetes.io/name=simulator -o name
kubectl -n vpp delete pod -l app.kubernetes.io/name=simulator
kubectl -n vpp rollout status deploy/simulator --timeout=90s
kubectl -n vpp get pods -l app.kubernetes.io/name=simulator
```

本机一次实测：`simulator-5d9b875986-x68rf` 被删后，几秒内出现 `…-q298t` 并 Ready。这就是「自愈」——不是进程自己拉起来，是控制器补副本。

对比：`make run-*` 挂了要自己再启；这里删了控制器会补。

### 4.3 扩缩容 `scale`

```bash
kubectl -n vpp scale deploy/simulator --replicas=2
kubectl -n vpp rollout status deploy/simulator --timeout=90s
kubectl -n vpp get pods -l app.kubernetes.io/name=simulator
# 两个 Running 时，gateway 调 simulator.vpp.svc.cluster.local:8084 会在两者间负载均衡

kubectl -n vpp scale deploy/simulator --replicas=1
```

git 里的 `replicas: 1` 不会因为这次 scale 自动改回去。下次 `make k8s-apply` 会把副本数扳回 YAML。若要长期 2 副本，改 `deployment.yaml` 再 apply。

不要对 `resource` / `dispatch` 随手 `--replicas=2` 然后走开。

### 4.4 面试时能讲清楚的三句

1. **Pod 是易逝的**，不要记 Pod IP；客户端记 Service 名。
2. **Deployment 管副本数和滚动更新**；你杀 Pod 它会补，你 rollout 它会先起新再停旧。
3. **Service 把稳定 DNS/ClusterIP 指到当前 Endpoints**；扩成 2 副本时流量自然分开，不用改调用方配置。

---

## 5. 故障速查

| 现象 | 先看 |
| ---- | ---- |
| ImagePullBackOff | GHCR 能否拉；WSL 代理要写成 `host.docker.internal:7897` 再重建集群 |
| CrashLoop / 探针失败 | `kubectl -n vpp logs <pod>`；常见是打不到 Postgres/Kafka，或 `hostAliases` IP 过期 |
| APISIX 502 | `make k8s-exposure-check`；`:30082/healthz` 应为 200 |
| APISIX `/resource` 401 | 预期（无 Bearer）；带 token 仍 401 见 [`CASDOOR.md`](CASDOOR.md) |
| `400 Request Header Or Cookie Too Large` | `make casdoor-init`（JWT-Custom）后再拿 token |
| Pod 里连不上 `host.docker.internal` | `docker exec vpp-control-plane getent hosts host.docker.internal` |

---

## 6. Consul：运行时已摘掉

集群内互调走 K8s Service DNS；`consul-addr` 默认空，进程不注册。compose 已无 Consul 服务。

当前 GHCR `:latest` 仍烤着 `127.0.0.1:8500`，而且那版 viper **忽略空环境变量**，所以 ConfigMap 里的 `*_CONSUL_ADDR: ""` 盖不掉。四个 Deployment 启动时用 `sed` 把该字段写成空（写到 `/tmp/config.yaml`）。下次镜像带上空 YAML + `AllowEmptyEnv` 之后，这段 `command` 可以删。

`platform/discovery` 的 Consul 封装还在，`run.go` 里 `if ConsulAddr != ""` 仍保留，填地址即可再连。配置中心如果以后要做，是 ConfigMap/Secret（以及现有 env 覆盖），不是把 Consul KV 请回来。

---

## 7. 明确还没做

- APISIX / Casdoor / Postgres / Kafka 进集群
- Helm、Ingress Controller、HPA、多节点
- 用 NetworkPolicy 挡住 host 上的 NodePort 旁路

相关：[`docs/APISIX.md`](APISIX.md)、[`docs/CASDOOR.md`](CASDOOR.md)、[`architecture.md`](../architecture.md)、[`ROADMAP.md`](../ROADMAP.md)。
