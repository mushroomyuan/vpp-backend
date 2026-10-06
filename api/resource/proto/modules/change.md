这次 `modules` 里的结构变化是一次语义重切：测点从“设备点位”变成“标准指标绑定”，控制单元上的能力标签变成独立的能力实例。

## 测点：自由点位变成指标绑定

`point.proto` 里原来的点位是一组松散字段：自己起的 `PointKey`、数据类型、扩展配置、描述、是否可控、是否虚拟、自由结构的安全阈值，以及嵌在点位上的最新运行值。

现在 `Point` 只保留绑定关系：


| 原来                                    | 现在                                                               |
| ------------------------------------- | ---------------------------------------------------------------- |
| `PointKey`                            | `MetricID`，指向 `contracts` 里的标准指标，例如 `electrical.active_power.v1` |
| `PointDataType`                       | 删掉。值类型跟指标定义走，不再写在点位上                                             |
| `ControlFlag`                         | `AccessMode`：`read` / `write` / `read_write`                     |
| `ExtConfig`、`Description`、`IsVirtual` | 删掉，换成 `Scale`、`Offset`、`Enabled`                                 |
| `SafetyThresholds`（`Struct`）          | `PointSafetyConstraint`：上下限、每秒最大变化、版本号                           |
| `CacheKeyAlias`、`Runtime`             | 字段号保留，不再出现在点位资源上                                                 |


`ExternalAddress` 还在，用来放厂商或设备侧地址。标准身份是 `MetricID`，设备名不再充当点位主键。

更新请求多了 `ExpectedRevision`，和响应里的 `Revision` 配对，做乐观锁。列表过滤也跟着换了：按 `MetricIDs`、`Enabled`、`AccessModes` 查，不再按点位键、是否虚拟、数据类型查。

`common.proto` 里的 `PointDataType` 枚举，以及 `runtime.proto` 里的 `PointRuntime`，都随这次删掉了。点位消息不再携带 Redis 里的最新采样值。随后 `Asset.Runtime`、`CU.Runtime` 和整个 `runtime.proto` 也删掉了：当前值只在 Telemetry 快照里，Resource 不再合并运行时缓存。

## 控制单元：能力标签拆成独立资源

`cu.proto` 里 `CU`、创建、更新、批量条目上的 `CapabilityTags` 都标成 `reserved`。列表请求的 `Capability` 改名为 `CapabilityIDs`。

能力不再是挂在控制单元上的一串字符串。新增的 `capability.proto` 把能力做成独立资源 `CUCapability`：

- `CUID`：挂在哪个控制单元上
- `CapabilityID`：契约 ID，例如 `energy.storage.v1`
- `SchemaVersion` + `Spec`：该实例的参数，`Spec` 就是前面那组储能、发电等 JSON
- `Enabled`、`Version`：是否启用，以及更新时的版本冲突检查

也就是说，控制单元只描述“这台设备是谁、怎么连”；它能储能还是能发电，以及容量、功率这些参数，改由能力实例单独保存和校验。