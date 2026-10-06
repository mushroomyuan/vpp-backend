# Decision 重构后的配套改造

> 本文记录从 Decision 重构主计划中主动拆出的 Gateway 与 Alarm 配套工作，避免扩大当前重构范围，也避免后续遗忘。
>
> 当前阶段：Gateway 已用两个厂商的不同点名、单位和功率符号验证上行归一化与下行逆转换。Telemetry 只保留 canonical MetricID。改 external address 后策略仍按 canonical 指标规划，越界或 revision 不一致的命令不会下发。Dispatch 的 wire 字段仍叫 `PointKey`。`vpp.soe.events` 已是 schema v2：canonical `metric_id`，并带 observed_at、quality、value 和 CU。Alarm 规则只接受契约注册表里的 MetricID，fingerprint 为 `v2:` / `soe:v2:`。质量 BAD、UNCERTAIN、测点陈旧和恢复各自开单，不把陈旧的 good 值当成当前健康。

## 一、Gateway canonical binding 专项

### 目标

让 Gateway 成为设备协议与平台 canonical contract 之间的防腐层：

```text
设备 external_address/raw value
    ↔ Gateway binding cache + conversion
    ↔ canonical metric_id/canonical value
```

Resource 是 point binding 的权威源，但不进入遥测和命令热路径；Gateway 缓存绑定并执行转换。

### 待办

- 修改外部遥测入站契约：明确设备上报的是 `external_address`，不能把厂商点名冒充 canonical MetricID。
- Gateway 新增 Resource point-binding 只读客户端，按 `(tenant_id, cu_id)` 预加载和缓存：
  - `metric_id`
  - `external_address`
  - `access_mode`
  - `scale / offset`
  - `binding_revision`
  - safety constraints
- 上行执行 `canonical = raw * scale + offset`，只把 canonical MetricID/value 写入 Telemetry。
- 下行按 `(tenant, cu, metric_id)` 反查 binding，执行逆转换并向设备发送 external address。
- 缓存采用 last-known-good + 短 TTL；订阅 Resource point/CU 变更事件做失效。事件只负责失效，完整内容仍从 Resource 拉取。
- 命令执行前强校验 binding revision、可写权限、转换参数和 safety constraint；缓存过期、revision 不一致、重复/缺失绑定或转换不可逆时 fail closed。
- scale/offset 更新必须 bump binding revision；禁止使用过期转换静默下发。
- 遥测批次中已知点正常接收，未知点进入隔离/错误计数并标记接入健康异常；不得回退为原字符串，也不应让一个无关未知点阻断所有已知关键指标。
- 将 Gateway proto/domain 中遗留 `PointKey` 命名收敛为 `MetricID`，并明确 external address 只存在于 Gateway 边界。
- 补齐异构集成测试：至少两个厂商使用不同点名、单位和功率符号，验证上行归一化与下行逆转换。

### 完成标准

- Telemetry 中不再出现厂商点名。
- Decision/Dispatch 只处理 canonical MetricID。
- 修改 external address 不影响策略；修改 metric contract 必须走版本升级。
- 可疑、过期或越界 binding 宁可拒绝命令，也不能猜测或透传。

## 二、Alarm canonical SOE/规则专项

已完成。无生产数据，种子规则按契约注册表重建，不双读旧 `metric_name` 消息。

### 目标

让 Alarm 的 SOE 消费、规则选择和展示统一使用 canonical MetricID，同时保持告警去重与历史语义可解释。

### 待办

- 版本化 `vpp.soe.events` 契约：将 metric name 明确为 canonical `metric_id`，保留 observed_at、quality、value 和 CU identity。
- 审查并迁移 Alarm 基于 metric 字符串的规则匹配，规则写入时必须通过公共 contract registry 校验。
- 明确旧规则处理方式：项目当前无生产数据，采用重建种子规则，不做 alias 双读。
- 审查 fingerprint 输入：若旧 fingerprint 包含外部 metric name，切换前必须决定是否改为 canonical MetricID；任何变更都要显式版本化，不能静默改变已有告警聚合键。
- Alarm 展示需要友好名称时，从只读 contract descriptor 获取 display name/unit，不能使用 external address。
- 对 BAD/UNCERTAIN、metric 长时间陈旧和恢复事件定义独立规则语义；不要把“旧的 good value 仍在快照”误判为当前设备健康。
- 补齐 SOE consumer、规则匹配、fingerprint 稳定性和 canonical metric 展示测试。

### 完成标准

- Alarm 规则不再依赖厂商点名或历史自由文本 PointKey。
- 相同 canonical 事件在重放时保持幂等和 fingerprint 稳定。
- metric contract 升级对规则迁移有明确版本策略。

## 三、执行顺序

1. Decision 重构完成内部 canonical MetricID + Simulator 闭环。
2. 实施 Gateway binding/cache/conversion 专项，解除“设备必须直接上报 canonical ID”的临时限制。
3. 实施 Alarm canonical SOE/规则专项。已完成。
4. 两段都已落地，平台可以宣称真实异构点名和 canonical 告警。
