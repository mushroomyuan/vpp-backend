# Docker Desktop 与 Ubuntu dockerd 冲突排障

2026-10-02 这次本机环境起不来，根因不是 compose 文件写错，而是 **同一台 WSL 2 上有两套 Docker 引擎**，命令行和 Docker Desktop 各连一套。下面按「怎么判断」和「怎么恢复」记，避免下次再从零猜。

相关日常操作见 [`docs/K8S_DEPLOYMENT.md`](K8S_DEPLOYMENT.md)。

---

## 1. 两套引擎

Docker Desktop「基于 WSL 2」指的是它自己的引擎跑在一个独立发行版里，不会自动接管 Ubuntu 里另装的 Docker Engine。

| WSL 发行版 | 引擎 | 谁在用 |
|---|---|---|
| `docker-desktop` | Docker Desktop 的 dockerd | Docker Desktop 窗口、`docker.exe` |
| `Ubuntu-22.04` | 发行版里 systemd 启动的 `/usr/bin/dockerd` | 占住 `/var/run/docker.sock` 时，终端和 IDE 的 `docker` |

两边的镜像、容器、卷、网络互不相通。同名 `vpp-backend-*` 只是碰巧都叫这个名字。

本机该用的是 **Docker Desktop**：

- Kind 集群 `vpp-control-plane`、kubectl context `kind-vpp` 只在这一套里。
- APISIX 的 etcd 卷 `apisix_apisix_etcd_data` 只在这一套里。
- Postgres / Kafka / Grafana 的数据在仓库目录 `./data/`，本来就是 Desktop 那套容器在挂。

Ubuntu 里的 dockerd 是后装的第二套。在它上面 `make infra-up` 会挂载同一份 `./data/`，和 Desktop 抢目录、抢端口。

查看当前连的是哪一套：

```bash
wsl.exe -l -v
docker info --format 'OS={{.OperatingSystem}} Name={{.Name}}'
```

`OS=Docker Desktop (containerized)` 才是 Desktop。`OS=Ubuntu 22.04.5 LTS` 且 `Docker Root Dir=/var/lib/docker` 是 Ubuntu 自己的引擎。

Windows 侧可以对照：

```bash
"/mnt/c/Program Files/Docker/Docker/resources/bin/docker.exe" ps
```

---

## 2. 这次看到的现象

1. **`make infra-up` 拉镜像超时。**  
   直连 `registry-1.docker.io` 超时。本机代理 `127.0.0.1:7897` 可以通（未认证时返回 401）。当时命令打到 Ubuntu 的 dockerd，守护进程不继承 shell 的 `HTTP_PROXY`，所以只有终端里的 curl 能通。  
   Docker Desktop 的设置里已经配了同一代理（`OverrideProxyHTTP` / `OverrideProxyHTTPS`）。镜像拉取应走 Desktop，不要在 Ubuntu 引擎上再配一套代理。

2. **终端显示容器 Up，Docker Desktop 里仍是 Exited。**  
   终端的 `docker` 连的是 Ubuntu dockerd。Desktop 窗口只列出 `docker-desktop` 里的容器。Desktop 里那组 `vpp-backend-*` 是几周前引擎退出时留下的（退出码 255 表示进程被停掉，不是数据坏了）。

3. **IDE 右键 Compose Up 以前会进 Desktop，后来不行。**  
   Cursor / VS Code 的 Compose 用的是 `/var/run/docker.sock`。这个 socket 以前是 Desktop 的集成代理，所以右键创建的容器出现在 Desktop 里。Ubuntu 的 `docker.service` 被 enable 之后抢占了同一个 socket，右键就进了 Ubuntu 引擎。

4. **APISIX 启动失败：`not a directory`。**  
   报错容器名是 `f41fcc8807cb_apisix_apisix_1`，创建于 2026-07-28，compose 1.29.2，挂载源是  
   `deploy/apisix/config.yaml` → `/usr/local/apisix/conf/config.yaml`。  
   现行 compose 挂的是 `deploy/apisix/conf/config.yaml`。旧路径上已经没有这个文件，Docker 在缺失的文件路径上建了一个 **空目录**。再启动时，目录无法挂到镜像里的配置文件上，内核返回 `not a directory`。

5. **Desktop 上 Kafka 一直重启。**  
   日志是 `mkdir: cannot create directory '/bitnami/kafka/config': Permission denied`。容器记录的挂载源是 Ubuntu 路径 `/home/yfz/project/vpp-backend/data/kafka_data`。这个路径在 `docker-desktop` 发行版里不存在。没有集成代理时，Desktop 挂不到 Ubuntu 的文件，看到的是空目录，容器用户 `1001` 写不进去。真正的数据仍在仓库的 `./data/kafka_data`（属主 1001）。

6. **Kind 业务 Pod CrashLoopBackOff。**  
   节点 `vpp-control-plane` 本身是 Ready 的。`resource` 等 Pod 连 `host.docker.internal:5432`（`192.168.65.254`）被拒绝，因为 Postgres 还没起来。基础设施起来之后，退避中的 Pod 不会马上重试，需要删掉 Pod 让它们立刻重建。

---

## 3. 恢复时做了什么

### 3.1 停掉 Ubuntu 的引擎

```bash
sudo systemctl disable --now docker.service docker.socket
```

`docker.socket` 必须一起 disable。只停 service 的话，下次有人访问 socket 会把它再拉起来。

确认：

```bash
systemctl is-enabled docker.service docker.socket   # disabled
systemctl is-active docker.service                  # inactive
```

### 3.2 把 socket 交回 Docker Desktop

Desktop 要挂 Ubuntu 里的文件，依赖 Ubuntu 里跑着的集成代理。代理起来之后，`docker-desktop` 里才会有：

```text
/run/guest-services/distro-services/ubuntu-22-04.sock
```

没有这个 socket 时，`docker compose up` 会失败：

```text
accessing specified distro mount service: stat .../ubuntu-22-04.sock: no such file or directory
```

代理进程是 `/mnt/wsl/docker-desktop/docker-desktop-user-distro`。它要在 Ubuntu 里以 root 运行，并且 Docker Desktop 已经启动（`/mnt/wsl/docker-desktop` 已挂上）。位置参数必须是 **没有空格的 Windows 路径**，否则它执行 `mount -t drvfs` 时会把路径拆碎并退出。本机可用 8.3 短名：

```text
C:\PROGRA~1\Docker\Docker\RESOUR~1
```

短名可以用下面的命令查：

```powershell
(New-Object -ComObject Scripting.FileSystemObject).GetFolder('C:\Program Files\Docker\Docker\resources').ShortPath
```

已写入并 enable 的单元：`/etc/systemd/system/docker-desktop-wsl.service`。开机后如果 Desktop 还没起来，单元会按 `Restart=always` 重试。

恢复后确认：

```bash
docker info --format 'OS={{.OperatingSystem}} Name={{.Name}}'
# OS=Docker Desktop (containerized) Name=docker-desktop
```

### 3.3 删掉 APISIX 的错误挂载目录

`deploy/apisix/config.yaml` 如果是目录（这次属主是 root），删掉。不要在这个路径上新建文件。配置只在 `deploy/apisix/conf/config.yaml`。

### 3.4 在 Desktop 上按当前 compose 重建

必须通过已经接回 Desktop 的 `docker`（或能把路径写成 `\\wsl.localhost\Ubuntu-22.04\...` 的 `docker.exe compose`）重建。旧容器上的 `/home/yfz/...` 挂载不会自动改成跨发行版挂载。

```bash
docker compose -f compose.yaml up -d --force-recreate
docker compose -f deploy/apisix/docker-compose.apisix.yaml up -d --force-recreate
docker compose -f deploy/casdoor/docker-compose.casdoor.yaml up -d --force-recreate
```

Postgres 日志里应出现 `database system was shut down` / `Skipping initialization`，而不是初始化一个空库。Casdoor 连的是已有库 `casdoor`。

Kind 节点若是 Exited，`docker start vpp-control-plane` 即可，集群状态在节点容器里，不要 `kind create` 重做。Postgres 起来之后，把仍在崩溃的业务 Pod 删掉，让 Deployment 立刻新建：

```bash
kubectl --context kind-vpp -n vpp delete pod -l 'app.kubernetes.io/name in (resource,telemetry,gateway,dispatch,simulator)'
# 若标签对不上，按 kubectl get pods -n vpp 的名字删
```

---

## 4. 以后怎么避免

- 日常只用 Docker Desktop。Ubuntu 里的 `docker.service` / `docker.socket` 保持 disabled。
- 右键 Compose、`make infra-up` 之前先看 `docker info` 的 OS。不是 `Docker Desktop` 就不要 up，否则会再写一份 `./data/`。
- `./data/postgres_data`、`./data/kafka_data`、`./data/grafana` 是 bind mount。两套引擎同时挂它们会改权限、互相覆盖。
- APISIX 只挂 `deploy/apisix/conf/config.yaml`。若 `deploy/apisix/config.yaml` 变成了目录，先删目录再启动。
- APISIX 容器可能显示 unhealthy：健康检查调用了镜像里没有的 `curl`。管理接口 `curl -H "X-API-KEY: ..." http://127.0.0.1:9181/apisix/admin/routes` 返回 200 就说明进程是好的。
- Kind 业务 Pod 依赖 Compose 里的 Postgres（`host.docker.internal:5432`）。先 `make infra-up`，再看 `kubectl --context kind-vpp -n vpp get pods`。连接拒绝导致的 CrashLoop 删 Pod 即可，不必重建集群。
- 拉镜像走 Desktop 已配置的 `127.0.0.1:7897`。一次拉很多大镜像时代理可能回 `EOF`，逐个 `docker pull` 再 `compose up`。
