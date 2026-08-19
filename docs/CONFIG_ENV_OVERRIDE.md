# 配置的环境变量覆盖（viper `AutomaticEnv`）

> 背景：在集成测试 / K8s 部署前置改造中曾错误记录过"配置外部化（env 覆盖 YAML）留给 K8s 阶段处理"。
> 实测（`internal/dispatch/app.go` 等 5 个服务的 `app.go:loadViperConfig`）证明这个能力其实**早就存在**，
> 是每个服务在最初接入 viper 时就带的默认行为，只是此前没有专门写文档说明规则和边界。
> 本文档补上这个空白，供 K8s manifests（Deployment env / ConfigMap / Secret）编写时参考。

## 1. 机制

`resource` / `telemetry` / `gateway` / `dispatch` / `simulator` 五个服务的 `app.go` 里都有等价的 `loadViperConfig()`：

```go
viper.SetConfigName("<service>")
viper.SetConfigType("yaml")
viper.AddConfigPath("./config")
// ... 其他候选路径

viper.AutomaticEnv()
viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_", "-", "_"))

if err := viper.ReadInConfig(); err != nil {
    logrus.Warnf("no config file loaded, using defaults and environment variables: %v", err)
}
```

`viper.AutomaticEnv()` 让 `viper.Unmarshal(opts)` 在读取每一个 `mapstructure` key 时，都会顺带检查是否存在对应的环境变量，
**环境变量优先于 YAML 文件里的值**。`SetEnvKeyReplacer` 把 key 路径里的 `.` 和 `-` 都换成 `_`，再整体转大写，
得到最终要查找的环境变量名。

## 2. 环境变量命名规则

对任意配置项，取它在 `Options` 结构体里从根到叶的完整 `mapstructure` 路径（用 `.` 连接各层级），
把 `.` 和 `-` 都替换成 `_`，再转大写，就是它的环境变量名。

```
<顶层>.<子层>....<叶子字段>  →  全部替换为 _  →  转大写
```

示例（以 dispatch 服务为例，取自 `internal/dispatch/options/options.go`）：

| YAML 路径 | 环境变量名 |
|---|---|
| `database.host` | `DATABASE_HOST` |
| `database.port` | `DATABASE_PORT` |
| `database.dbname` | `DATABASE_DBNAME` |
| `database.max-open-conns` | `DATABASE_MAX_OPEN_CONNS` |
| `kafka.brokers` | `KAFKA_BROKERS`（`[]string`，见下方「数组类型」说明） |
| `kafka.command-topic` | `KAFKA_COMMAND_TOPIC` |
| `gateway.grpc-addr` | `GATEWAY_GRPC_ADDR` |
| `dispatch.grpc-addr` | `DISPATCH_GRPC_ADDR` |
| `dispatch.auth.trust-proxy-headers` | `DISPATCH_AUTH_TRUST_PROXY_HEADERS` |
| `dispatch.auth.authz.casdoor-url` | `DISPATCH_AUTH_AUTHZ_CASDOOR_URL` |

其余四个服务（resource / telemetry / gateway / simulator）的 `options.go` 结构不完全相同，但规则一致：
直接对照各自的 `internal/<service>/options/options.go` 里的 `mapstructure` 标签推导即可。

## 3. 关键限制：key 必须已经存在于配置里

`viper.AutomaticEnv()` 不会凭空发明新 key——它只在 **`Unmarshal` 遍历到某个已知 key** 时才去查一次对应的环境变量。
"已知 key" 来自：

1. YAML 配置文件里已经出现过的 key（`config/<service>.yaml` 目前每个字段都给了默认值，天然满足这一条）；
2. 或者代码里显式 `viper.SetDefault(...)` / `viper.BindEnv(...)` 声明过的 key。

**实践含义**：只要继续使用仓库里现成的 `config/<service>.yaml` 作为镜像内置的基础配置（`deploy/docker/Dockerfile` 已经
`COPY config/${SERVICE}.yaml /etc/vpp/config.yaml`），YAML 里已有的每一项都可以用环境变量覆盖，**不需要改一行 Go 代码**。
如果未来要新增一个纯环境变量驱动、YAML 里完全没有的配置项，则需要额外加一行 `viper.SetDefault(...)`。

## 4. 数组类型（`[]string`）覆盖方式

`kafka.brokers` 这类 `[]string` 字段，viper 会尝试把环境变量的原始字符串按逗号切分。K8s manifests 里可以直接写：

```yaml
env:
  - name: KAFKA_BROKERS
    value: "kafka-0.kafka-headless:9092,kafka-1.kafka-headless:9092"
```

## 5. K8s Deployment 示例（仅示意，非最终 manifest）

```yaml
env:
  - name: DATABASE_HOST
    value: "postgres.vpp.svc.cluster.local"
  - name: DATABASE_PORT
    value: "5432"
  - name: DATABASE_DBNAME
    value: "dispatch"
  - name: DATABASE_PASSWORD
    valueFrom:
      secretKeyRef:
        name: vpp-postgres
        key: password
  - name: KAFKA_BROKERS
    value: "kafka.vpp.svc.cluster.local:9092"
  - name: GATEWAY_GRPC_ADDR
    value: "gateway.vpp.svc.cluster.local:5005"
```

容器仍然用镜像内置的 `/etc/vpp/config.yaml` 做基础配置（本机 host 网络地址等开发期默认值），
上面这些环境变量在 `viper.Unmarshal` 时逐项覆盖成集群内 Service 域名/密钥，两者不冲突。

## 6. 空字符串也会覆盖 YAML

viper 默认把「环境变量存在但值为空」当成未设置。各服务 `loadViperConfig` 已打开 `AllowEmptyEnv(true)`，ConfigMap 里写 `SOME_KEY: ""` 会盖掉 YAML 里的同名项。

## 7. 与本次集成测试改造的关系

`tests/integration` 模块没有用到这个机制（它是纯 Go 代码直接构造 `platformpostgres.Config{DSN: ...}` /
`kafka.Config{Brokers: ...}`，绕开了 viper），但排查这条链路时顺带验证了 env 覆盖对 5 个服务都成立，
遂将此前 `ROADMAP.md`「已知限制」表里"配置外部化留给 K8s 阶段"的记录更正为"已具备，K8s 阶段只需要写 manifests"。
