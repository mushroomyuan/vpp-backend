
结合你的项目（**DDD + 微服务 + Resource 四级模型 Site → Asset → CU → Point**）以及前面四个服务的设计，我给出的 Simulator 方案如下。

------

# vpp-simulator 服务设计方案

## 1. 服务定位

**vpp-simulator** 是一个长期运行的**虚拟设备运行时（Virtual Device Runtime）**。

职责：

- 从 Resource 加载需要模拟的资源树（Site→Asset→CU→Point）
- 在内存中实例化 CU 对象
- 周期更新设备状态
- 产生遥测数据
- 响应 Gateway 下发的控制命令
- 提供故障注入能力
- 为整个 VPP 提供闭环测试环境

**不负责：**

- 资源管理（Resource）
- 协议转换（Gateway）
- 数据存储（Telemetry）
- 调度决策（Dispatch）

------

# 2. 整体架构

```text
                     Resource
             (Site / Asset / CU / Point)
                      │
                 Query Resource
                      │
                      ▼
               Resource Loader
                      │
                      ▼
             Runtime Manager
                      │
     ┌────────────┬────────────┐
     │            │            │
     ▼            ▼            ▼
 Battery CU     PCS CU       PV CU
     │            │            │
     └────────────┴────────────┘
                 │
              Tick Engine
                 │
     ┌───────────┴────────────┐
     ▼                        ▼
Telemetry Publisher     Command Server
     │                        ▲
     ▼                        │
  Gateway                Dispatch
```

------

# 3. Runtime 模型

Simulator 不维护数据库。

启动以后：

```text
Site
 ├── Asset
 │      ├── CU
 │      │      ├── Point
 │      │      └── Point
 │      │
 │      └── CU
 │
 └── Asset
```

转换为：

```text
Runtime

SiteRuntime
    │
    ├── AssetRuntime
    │
    ├── AssetRuntime
    │
    └── ...
```

AssetRuntime：

```text
AssetRuntime

↓

CU Runtime

↓

Device Object
```

Point 不实例化对象。

Point 是 Device 的输出。

------

# 4. Runtime Manager

职责：

```text
维护所有 Device

启动 Tick

停止 Tick

注册 Device

注销 Device

查找 Device

状态查询
```

数据结构：

```go
type RuntimeManager struct {

    Devices map[string]Device

}
```

key：

```text
CUCode
```

------

# 5. Device 抽象

```go
type Device interface {

    Tick()

    Execute(cmd ControlCommand)

    Snapshot() []TelemetryPoint

    Status() DeviceStatus

}
```

实现：

```text
Battery

PCS

PV

Load

Breaker

Meter
```

以后：

```text
Wind

EV

Generator

HeatPump
```

无需修改 Runtime。

------

# 6. Tick Engine

统一 Tick。

例如：

```text
Every 3 Seconds

↓

RuntimeManager

↓

foreach Device

↓

Device.Tick()

↓

更新内部状态
```

例如：

Battery：

```text
SOC

Power

Temperature
```

发生自然变化。

------

# 7. Telemetry Publisher

每轮 Tick：

```text
Device

↓

Snapshot()

↓

Point Value

↓

Telemetry Message

↓

Gateway
```

Point 来源：

Resource。

Value 来源：

Device。

例如：

```text
Point：

BAT001.SOC

↓

Value：

63.4%
```

组合后：

```text
PointTelemetry
```

发送 Gateway。

------

# 8. Command Server

提供：

```text
ExecuteCommand()
```

流程：

```text
Gateway

↓

CUCode

↓

RuntimeManager

↓

Device.Execute()

↓

ACK
```

Device：

修改运行状态。

例如：

```text
SetPower

↓

Power=100

↓

后续 Tick

↓

SOC 持续下降
```

------

# 9. Resource Loader

启动：

```text
Query Resource

↓

Site

↓

Asset

↓

CU

↓

Point
```

创建：

```text
Runtime

↓

Device
```

建议：

增加查询条件：

```text
SimulationEnabled=true
```

或者：

```text
Environment=Simulator
```

避免加载真实设备。

------

# 10. Fault Engine

负责模拟异常。

支持：

```text
Device Offline

Telemetry Lost

Telemetry Delay

Command Reject

Command Timeout

Random Failure

High Latency

Network Partition
```

可动态开启。

------

# 11. Scenario Engine

负责模拟业务场景。

例如：

```text
白天光伏发电

↓

Battery 充电

↓

晚上放电
```

或者：

```text
SOC 低于20%

↓

拒绝充电命令
```

或者：

```text
14:00

PV 输出下降80%
```

Scenario：

修改 Device。

Device：

自然演化。

------

# 12. Debug API

提供：

```text
GET /runtime

查看所有 Device

GET /device/{cu}

查看状态

POST /device/{cu}/command

模拟命令

POST /scenario/start

启动场景

POST /fault

注入故障

POST /runtime/reset

恢复初始状态
```

------

# 13. 服务内部结构

```text
vpp-simulator/

cmd/
    simulator/

internal/

    config/

    runtime/
        manager.go
        loader.go

    device/
        battery.go
        pcs.go
        pv.go
        meter.go
        breaker.go
        load.go

    telemetry/
        publisher.go

    command/
        grpc_server.go

    scenario/
        engine.go

    fault/
        engine.go

    api/
        http_server.go

    client/
        resource/
        gateway/
```

------

# 14. 与现有服务关系

```text
Resource
    │
    │ 查询资源树
    ▼
Simulator
    │
    │ Telemetry
    ▼
Gateway
    │
    ▼
Telemetry

Dispatch
    │
    ▼
Gateway
    │
    ▼
Simulator
```

------

# 15. Resource 四级模型映射

| Resource 模型 | Simulator 中角色                                             |
| ------------- | ------------------------------------------------------------ |
| Site          | Runtime 根节点，仅用于组织资源                               |
| Asset         | Device 容器（储能站、光伏阵列等），用于组织 CU               |
| CU            | Runtime Device，对应一个运行中的设备对象，是 Simulator 的核心实体 |
| Point         | Telemetry 模板，不实例化对象，由 Device 在 Snapshot() 时生成对应 Point 的实时值 |

------

## 与 Grok 方案相比，我建议增加两点

### 1. Simulator 应完整理解 Resource 的四级资源树

Simulator 不应直接以 CU 为输入，而应加载 **Site → Asset → CU → Point** 的完整资源树，保持与真实系统一致，只是在 Runtime 中最终实例化 CU，Point 作为遥测模板。

### 2. Telemetry 应基于 Point 生成，而不是 DeviceState

不建议设计统一的 `DeviceState` 包含 `SOC、Power、Voltage...` 等所有字段。不同 CU 类型暴露的 Point 本身就不同，应由各 Device 根据 Resource 中定义的 Point 动态生成遥测数据，这样 Simulator 与 Resource 解耦程度更高，也更容易支持新增设备类型。