# VPP 中台演讲稿 & 架构图

这个目录是为"向他人介绍这套 VPP 中台服务"准备的材料，聚焦**宏观业务逻辑与架构设计理念**，不深入技术细节（技术细节和亮点计划后续单独补充）。

## 目录内容

| 文件 | 说明 |
|---|---|
| [`SPEECH.md`](./SPEECH.md) | 完整演讲稿，分 15 个章节，每章标注建议时长；附录里有配套的 PPT 分页建议 |
| [`diagrams/01-service-overview.excalidraw`](./diagrams/01-service-overview.excalidraw) | 服务全景图（业务视角，比 `../architecture.md` 里的技术向 Mermaid 图简化很多） |
| [`diagrams/02-business-loop.excalidraw`](./diagrams/02-business-loop.excalidraw) | 核心业务闭环图（数据 → 决策 → 执行 → 反馈 → 告警） |
| [`diagrams/03-optimization.excalidraw`](./diagrams/03-optimization.excalidraw) | 已退役的 optimization 一轮决策。当前主链见 [`internal/decision/OVERVIEW.md`](../../internal/decision/OVERVIEW.md) |
| `_gen_excalidraw.py` | 生成上述两张图的脚本，纯 Python 标准库、无外部依赖；想调整布局改这个脚本重跑即可，不用手改 JSON |

## 怎么用

1. 先读 `SPEECH.md`，按章节顺序过一遍，确认逻辑和措辞是否符合你想讲的方式（技术细节故意讲得很浅，可以按需要自己加深）。
2. 在 Cursor 里用已安装的 Excalidraw 插件直接打开两个 `.excalidraw` 文件查看/编辑；也可以拖进 [excalidraw.com](https://excalidraw.com) 导入。
3. 如果要做成正式 PPT：可以把 `SPEECH.md` 的章节标题当分页依据，手动誊入 PPT 工具，并把 Excalidraw 图截图贴进对应页；或者用 [Marp](https://marp.app/) / [Slidev](https://sli.dev/) 之类的工具直接把 Markdown 转成幻灯片。`SPEECH.md` 附录 B 里也提到，如果需要我直接生成 `.pptx` 文件，可以让我安装 `python-pptx`（需要一次性联网权限）来做。

## 这份材料没有覆盖什么

刻意没有涉及：具体端口号、RPC 方法签名、数据库表结构、Kafka topic 内部字段等实现细节。这些内容在仓库根目录的 `architecture.md`、`ARCHITECTURE_PATTERNS.md`、`AUTHN_AUTHZ.md`、`observability.md` 以及各服务的 `OVERVIEW.md` / `README.md` 里已经写得很细，本材料的目标是"先建立整体心智模型"，细节按需回头查那些文档即可。
