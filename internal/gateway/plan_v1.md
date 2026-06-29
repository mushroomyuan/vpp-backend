Gateway 服务设计方案
一、服务定位

gateway 是 VPP 与外部系统（EMS / IoT Platform）的集成边界。

职责：

接收外部系统遥测数据
将外部模型转换为内部标准模型
转发到 telemetry 服务
接收内部控制指令
将内部资源 ID 转换为外部资源 ID，并发送给外部系统

不负责：

资源管理（resource）
时序存储（telemetry）
调度算法（dispatch）
二、核心领域模型
1. DeviceMapping

负责外部资源和内部资源的映射。

位置：

gateway/domain/model

代码：

type DeviceMapping struct {
	ID string

	ExternalSystem string

	ExternalID string

	ResourceID string
}

示例：

EMS
 |
plantCode=SG001
 |
gateway mapping
 |
resource_id=bat-001

说明：

ExternalID 是外部系统认识的 ID
ResourceID 是 resource 服务内部 ID
2. ExternalTelemetry

外部输入模型。

不要直接复用 telemetry 模型。

type ExternalTelemetry struct {

	ExternalID string

	Metric string

	Value float64

	Timestamp int64

}

例如：

EMS:

{
 "meterId":"SG001",
 "p_act":100
}

gateway 接收：

ExternalTelemetry{
 ExternalID:"SG001",
 Metric:"p_act",
 Value:100,
}
3. StandardTelemetry

转换后的内部模型。

对应 telemetry 服务接口。

type StandardTelemetry struct {

	ResourceID string

	Metric string

	Value float64

	Timestamp int64

}

转换：

ExternalTelemetry

        ↓

DeviceMapping

        ↓

StandardTelemetry
4. ExternalCommand

外部控制命令模型。

type ExternalCommand struct {

	ExternalID string

	Command string

	Value float64

}

例如：

set_power
500kw
三、依赖接口（Port）

gateway 不直接依赖具体服务。

ResourcePort

查询资源映射。

type ResourcePort interface {

	GetMappingByExternalID(
		ctx context.Context,
		externalID string,
	) (*DeviceMapping,error)


	GetExternalID(
		ctx context.Context,
		resourceID string,
	) (string,error)

}

实现：

调用 resource gRPC。

TelemetryPort

发送标准遥测。

type TelemetryPort interface {

	SaveTelemetry(
		ctx context.Context,
		data StandardTelemetry,
	) error

}

实现：

调用 telemetry gRPC。

EMSPort

发送控制命令。

初期 mock：

type EMSPort interface {

	SendCommand(
		ctx context.Context,
		cmd ExternalCommand,
	) error

}

实现：

先打印日志：

send command:
external_id=SG001
power=500

未来替换真实 EMS adapter。

四、Application Use Case
1. ReceiveTelemetry

入口：

POST /api/v1/telemetry

流程：

ExternalTelemetry

↓

查询 DeviceMapping

↓

转换 StandardTelemetry

↓

调用 telemetry.SaveTelemetry

异常：

找不到映射：

返回：

device not registered
2. ExecuteCommand

入口：

POST /api/v1/command

输入：

内部 resource_id：

{
 "resource_id":"bat-001",
 "power":500
}

流程：

resource_id

↓

查询 DeviceMapping

↓

转换 ExternalCommand

↓

调用 EMSPort
3. CreateMapping

入口：

POST /api/v1/mappings

作用：

模拟 EMS 注册。

请求：

{
 "external_id":"SG001",
 "resource_id":"bat-001"
}

保存：

gateway 自己数据库。

初期：

PostgreSQL 即可。

五、接口设计
HTTP API（外部）
接收遥测
POST /api/v1/telemetry

请求：

{
 "external_id":"SG001",
 "metric":"power",
 "value":100,
 "timestamp":123456
}
控制命令
POST /api/v1/command

请求：

{
 "resource_id":"bat-001",
 "command":"set_power",
 "value":500
}
映射管理
POST /api/v1/mappings
DELETE /api/v1/mappings/{id}
六、服务调用关系

最终：

                EMS

                 |
                 |
             gateway

          /             \

     telemetry          resource


                 
             dispatch

                 |

             gateway

                 |

                EMS
七、当前实现范围（避免过度设计）

第一版：

实现：

DeviceMapping
HTTP 接收 telemetry
resource mock 查询
telemetry gRPC 调用
command log 输出
mapping CRUD

不要实现：

多 EMS adapter
协议转换框架
动态插件
复杂字段映射引擎

以后真实 EMS 接入：

增加：

adapter/ems_xxx

而不是修改核心模型。