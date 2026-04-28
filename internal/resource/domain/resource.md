### 1️⃣ Site 表

| 字段名      | 类型     | 说明                                   |
| ----------- | -------- | -------------------------------------- |
| id          | UUID     | 站点唯一标识                           |
| tenant_id   | UUID     | 租户 ID，用于权限隔离                  |
| name        | String   | 站点名称                               |
| location    | JSONB    | 经纬度/地址                            |
| description | String   | 备注                                   |
| status      | Enum/Int | 站点状态（建设中、运行中、故障、离线） |

> **补充点**：tenant_id 支持多租户隔离，status 支持快速屏蔽故障站点。

------

### 2️⃣ Resource 表

| 字段名       | 类型   | 说明                         |
| ------------ | ------ | ---------------------------- |
| id           | UUID   | 资源唯一标识                 |
| site_id      | UUID   | 所属站点 (FK)                |
| name         | String | 资源名称                     |
| type         | String | 类型（变压器、光伏逆变器等） |
| capacity     | Float  | 额定功率                     |
| manufacturer | String | 厂商                         |
| model        | String | 型号                         |
| metadata     | JSONB  | 拓扑、协议类型等扩展信息     |

------

### 3️⃣ CU（Control Unit）表

| 字段名          | 类型        | 说明                                                       |
| --------------- | ----------- | ---------------------------------------------------------- |
| id              | UUID        | CU 唯一标识                                                |
| resource_id     | UUID        | 所属 Resource (FK)                                         |
| parent_cu_id    | UUID, 可选  | 支持嵌套 CU 聚合                                           |
| name            | String      | CU 名称                                                    |
| type            | String      | 类型/功能分类                                              |
| capability_tags | JSONB/Array | 调度能力标签，例如 ["frequency_regulation","peak_shaving"] |
| metadata        | JSONB       | 可扩展属性（模式、厂商型号等）                             |

> **补充点**：parent_cu_id 支持嵌套聚合，capability_tags 支持快速筛选调节能力，方便控制面调度算法使用。

------

### 4️⃣ Point 表（测点 / 控制点）

| 字段名            | 类型    | 说明                                             |
| ----------------- | ------- | ------------------------------------------------ |
| id                | UUID    | 点位唯一标识                                     |
| cu_id             | UUID    | 所属 CU (FK)                                     |
| point_key         | String  | 业务标准键，如 read_p, write_v, read_soc         |
| external_address  | String  | EMS/IoT 系统原始标识（寄存器地址或 Topic）       |
| data_type         | String  | 数据类型: Float / Int / Bool / Enum              |
| ext_config        | JSONB   | 协议相关配置（系数、偏置、读写权限、采样频率等） |
| description       | String  | 点位说明                                         |
| control_flag      | Boolean | 是否可控点（True = 控制面，False = 遥测面）      |
| is_virtual        | Boolean | 是否虚拟点（算法计算，不对应物理采集）           |
| safety_thresholds | JSONB   | 控制面安全红线，例如 {"max_power":500}           |
| cache_key_alias   | String  | Redis 快速访问的 Key 别名                        |