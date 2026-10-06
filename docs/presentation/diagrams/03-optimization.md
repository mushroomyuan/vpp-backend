# Optimization 一轮决策（已退役）

这张图描述已经删除的 optimization 循环，只作历史对照。当前决策主链见 [`internal/decision/OVERVIEW.md`](../../../internal/decision/OVERVIEW.md)。图和 [`03-optimization.excalidraw`](./03-optimization.excalidraw) 仍是同一张旧图。

```mermaid
flowchart TD
    A["DecisionLoop<br/>每 60 秒一个 tick"] --> B["每个已配置租户各跑一轮"]
    B --> C["RunDecisionCycle"]
    C --> D["读 Telemetry 当前快照<br/>过期则本轮跳过"]
    D --> E{"SOC 越限了？"}
    E -->|"否，在上下限之间"| Z["本轮不动作<br/>等下一个 tick"]
    E -->|"是，过低则充电 / 过高则放电"| F{"这个方向还在冷却？"}
    F -->|"是"| Y["这个方向先不再发<br/>反方向仍可以触发"]
    F -->|"否"| G["记下该方向的冷却<br/>产出一条 Target"]
    G --> H["Allocate<br/>PointTarget 一比一变成命令"]
    H --> I["Dispatch SubmitTask<br/>自动任务"]
    I --> Z

    E -.->|"v1 不调用"| FC["Forecast 只是占位"]
    H -.->|"仅按容量分摊时才读<br/>v1 不走"| RS["Resource 额定容量"]
```
