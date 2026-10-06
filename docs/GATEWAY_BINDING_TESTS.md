# Gateway 点绑定翻译：测试说明

这份说明只讲业务：测试在证明什么、喂进去的是什么、期望看到什么。对应代码是：

- 集成测试 [`tests/integration/gateway_hetero_test.go`](../tests/integration/gateway_hetero_test.go) 里的 `TestGateway_TwoVendorsCanonicalBinding`
- 下行纯计算 [`internal/gateway/domain/binding/downlink_test.go`](../internal/gateway/domain/binding/downlink_test.go)
- 下发用例 [`internal/gateway/application/command/command_test.go`](../internal/gateway/application/command/command_test.go) 里的 `TestExecuteCommand`

换算只有一条：

```text
平台值 = 设备原始值 × scale + offset
设备原始值 = (平台值 - offset) / scale
```

平台内部的有功功率单位是 kW，充电为负、放电为正。设备可以用瓦、千瓦，也可以把正负号反过来。`scale` 同时表达单位和符号。

## 1. 集成测试在讲一个什么故事

两个储能设备接在同一个租户下。策略和调度只认识平台指标。Gateway 负责把厂商点名换进换出。

| | 厂商 A | 厂商 B |
|---|---|---|
| 外部系统 / 设备 | `ems-a` / `device-a` | `ems-b` / `device-b` |
| 有功功率（只读） | `HOLDING_40001`，瓦，`scale = 0.001` | `inv.p.active`，千瓦且符号相反，`scale = -1` |
| SOC（只读） | `SOC_PERMILLE`，千分比，`scale = 0.1` | `bms.soc`，百分数，`scale = 1` |
| 功率设定值（可写） | `CMD_REG_40010`，瓦，`scale = 0.001` | `inv.p.set`，符号相反，`scale = -1` |
| 设定值安全范围 | 平台值 `-80` kW 到 `80` kW | 同左 |

两台设备的额定充放电能力都是 80 kW。

### 1.1 上行：设备上报后，库里只有平台量

输入是设备自己的点名和原始值，外加一个平台不认识的点。

| 设备 | 上报 | 含义 |
|---|---|---|
| A | `HOLDING_40001 = 25000` | 25000 W |
| A | `SOC_PERMILLE = 200` | 200‰ |
| A | `A_SPARE = 1` | 没有绑定 |
| B | `inv.p.active = -25` | 厂商把放电记成负数 |
| B | `bms.soc = 20` | 已经是百分数 |
| B | `B_SPARE = 1` | 没有绑定 |

期望：

| 观察点 | 结果 |
|---|---|
| 每台设备本批接收 / 隔离 | 接收 2 个，隔离 1 个（那个多余点） |
| A 的功率 | `electrical.active_power.v1 = 25` kW |
| A 的 SOC | `energy_storage.state_of_charge.v1 = 20` % |
| B 的功率 | 同样是 `25` kW |
| B 的 SOC | 同样是 `20` % |
| 快照里的指标个数 | 每台 2 个 |
| 厂商点名 | 快照里找不到 `HOLDING_40001`、`SOC_PERMILLE`、`inv.p.active`、`bms.soc` 这些名字 |

`25000 × 0.001 = 25`，`200 × 0.1 = 20`，`-25 × -1 = 25`。两个厂商的原始数不同，进 Telemetry 之后是同一个平台量。

### 1.2 下行：平台下发 -10 kW，两台设备收到的数不同

输入都是平台设定值 `-10` kW，指标是 `electrical.active_power_setpoint.v1`。调用方没有指定 revision。

| 设备 | 设备收到的点名 | 设备收到的原始值 | 怎么算的 |
|---|---|---|---|
| A | `CMD_REG_40010` | `-10000` | `-10 / 0.001`，单位换回瓦 |
| B | `inv.p.set` | `10` | `-10 / -1`，符号翻回去 |

A 发给 `ems-a` / `device-a`，B 发给 `ems-b` / `device-b`。

### 1.3 同一条命令再走一遍 Dispatch

对厂商 B 再经 Dispatch 提交一次 `-10` kW。任务里的 `PointKey` 仍然是 `electrical.active_power_setpoint.v1`，数值仍然是 `-10`。任务完成后，设备侧收到的还是 `inv.p.set = 10`。

这一步证明调度单上保持平台语言，翻译发生在 Gateway 出去的那一下。

### 1.4 越界就停住

对厂商 A 下发 `200` kW。安全上限是 `80` kW。

期望：命令被拒绝，设备侧没有这条命令。

### 1.5 改设定值点名，数值算法不变

把厂商 A 的设定值点名从 `CMD_REG_40010` 改成 `CMD_REG_40011`。`scale` 仍是 `0.001`，所以绑定 revision 从 1 变成 2。

缓存刷新之后，再下发平台值 `-10` kW，并声明 revision 必须是 2。

期望：设备收到 `CMD_REG_40011 = -10000`。

紧接着用旧 revision `1` 再下发同一次 `-10` kW。期望：拒绝，设备侧没有这条命令。旧点名不能继续拿来发。

### 1.6 改 SOC 点名，策略还是按平台 SOC 规划

先给厂商 A 挂一条 SOC 策略：SOC 低于 `80%` 就充电，充电功率 `40` kW。此时快照里的 SOC 是 `20%`。

期望的第一份计划：

| 字段 | 值 |
|---|---|
| 控制单元 | 厂商 A |
| 指标 | `electrical.active_power_setpoint.v1` |
| 设定值 | `-40` kW（充电为负；40 kW 没有超过 80 kW 能力） |
| 计划状态 | `ready` |

然后把 SOC 点名从 `SOC_PERMILLE` 改成 `SOC_PM_V2`，`scale` 仍是 `0.1`。

| 上报 | 期望 |
|---|---|
| 新点名 `SOC_PM_V2 = 200` | 接收 1 个，隔离 0 个 |
| 旧点名 `SOC_PERMILLE = 200` | 接收 0 个，隔离 1 个 |
| 快照 SOC | 仍是 `20%`，点名不是 `SOC_PM_V2` 也不是 `SOC_PERMILLE` |

冷却只有 1 毫秒，所以同一条策略可以马上再跑一次。第二份计划仍是对厂商 A 下发 `-40` kW，而且是一份新计划。策略正文里没有厂商点名；点名改了，充放电判断不变。

## 2. 单元测试：把一种换算和每一种拒绝拆开

集成测试走的是真实服务。下面两组不启动数据库，只核对 Gateway 自己的判断。

### 2.1 符号和偏移能对上

绑定：点名 `REG_SET_P`，`scale = -0.5`，`offset = 1`，revision `4`，可写。

输入：平台值 `-8` kW。绑定可以是刚从 Resource 拉到的，也可以是还在有效期内的缓存。调用方可以不带 revision，也可以带上 `4`。

期望：设备点名 `REG_SET_P`，原始值 `18`。

```text
(-8 - 1) / -0.5 = 18
18 × -0.5 + 1 = -8
```

边界值也算合法。安全范围是 `-30` 到 `30`，`scale = -1`。输入正好是 `-30`。期望放行，原始值是 `30`。

### 2.2 这些输入都不会产生设备命令

基准绑定：设定值点 `REG_SET_P`，`scale = -1`，revision `3`，安全范围 `-30` 到 `30`。输入平台值如果没有另写，就是 `-10`。

| 情况 | 输入相对基准的差别 | 拒绝原因 |
|---|---|---|
| 缓存是 Resource 失败后的旧副本 | 绑定来源是回退缓存 | 过期，不能拿来下发 |
| 说不清这份绑定是不是刚确认过 | 来源为空 | 同上 |
| 这个控制单元没有该指标 | 绑定列表是空的 | 缺失 |
| `PointKey` 写成 `switch` | 不是平台指标 | 不能用 |
| 调用方以为 revision 还是 2 | 实际是 3 | revision 不一致 |
| 绑定自己的 revision 是 0 | | revision 无效 |
| 点是只读的 | | 不可写 |
| 点被停用 | | 已停用 |
| 同一个指标有两个可写点 | 另一个点名是 `REG_SET_P_B` | 重复，无法决定发哪一个 |
| 平台值 `40` | 高于上限 30 | 越界 |
| 平台值 `-40` | 低于下限 -30 | 越界 |
| 平台值是 NaN | | 不是有限数 |
| `scale = 0` | | 除数为 0，换不回去 |
| 安全下限 20、上限 10 | 上下限写反了 | 绑定本身不可用 |

下发用例再补一层：上面这类拒绝发生时，设备适配器收不到命令，也不会发出「命令已完成」。正常下发则发给外部系统 `ems-sg`、设备 `dev-1`，点名和原始值就是上一节的 `REG_SET_P = 18`。租户、控制单元、指标、命令号缺了任何一个，在查绑定之前就失败。

## 3. 怎么跑

集成测试要 Docker。它和本包里的其他测试共用一套临时 Postgres、Kafka、Redis。

```bash
cd tests/integration
go test -count=1 -timeout=8m -run TestGateway_TwoVendorsCanonicalBinding .
```

只跑 Gateway 自己的换算和下发判断：

```bash
cd internal/gateway
go test ./domain/binding/ ./application/command/ -count=1 -run 'Downlink|ExecuteCommand'
```
