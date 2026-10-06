#!/usr/bin/env python3
"""生成用于本演讲稿的 Excalidraw 图（纯 stdlib，无需联网/装包）。
运行一次后可删除本脚本，只保留生成的 .excalidraw 文件。
"""
import json
import os

SEED = [1000]


def next_id(prefix):
    SEED[0] += 1
    return f"{prefix}-{SEED[0]}"


def next_seed():
    SEED[0] += 1
    return SEED[0]


def rect(x, y, w, h, bg="#a5d8ff", stroke="#1e1e1e", rid=None, rough=1):
    rid = rid or next_id("rect")
    return rid, {
        "id": rid,
        "type": "rectangle",
        "x": x, "y": y, "width": w, "height": h,
        "angle": 0,
        "strokeColor": stroke,
        "backgroundColor": bg,
        "fillStyle": "solid",
        "strokeWidth": 2,
        "strokeStyle": "solid",
        "roughness": rough,
        "opacity": 100,
        "groupIds": [],
        "frameId": None,
        "roundness": {"type": 3},
        "seed": next_seed(),
        "version": 1,
        "versionNonce": next_seed(),
        "isDeleted": False,
        "boundElements": None,
        "updated": 1700000000000,
        "link": None,
        "locked": False,
    }


def text(x, y, w, h, s, size=16, align="center", color="#1e1e1e", bold_hint=False):
    tid = next_id("text")
    return tid, {
        "id": tid,
        "type": "text",
        "x": x, "y": y, "width": w, "height": h,
        "angle": 0,
        "strokeColor": color,
        "backgroundColor": "transparent",
        "fillStyle": "solid",
        "strokeWidth": 2,
        "strokeStyle": "solid",
        "roughness": 1,
        "opacity": 100,
        "groupIds": [],
        "frameId": None,
        "roundness": None,
        "seed": next_seed(),
        "version": 1,
        "versionNonce": next_seed(),
        "isDeleted": False,
        "boundElements": None,
        "updated": 1700000000000,
        "link": None,
        "locked": False,
        "text": s,
        "fontSize": size,
        "fontFamily": 1,
        "textAlign": align,
        "verticalAlign": "middle",
        "baseline": size + 2,
        "containerId": None,
        "originalText": s,
        "lineHeight": 1.25,
    }


def arrow(x1, y1, x2, y2, dashed=False, color="#1e1e1e", sw=2):
    aid = next_id("arrow")
    dx, dy = x2 - x1, y2 - y1
    return aid, {
        "id": aid,
        "type": "arrow",
        "x": x1, "y": y1, "width": abs(dx), "height": abs(dy),
        "angle": 0,
        "strokeColor": color,
        "backgroundColor": "transparent",
        "fillStyle": "solid",
        "strokeWidth": sw,
        "strokeStyle": "dashed" if dashed else "solid",
        "roughness": 1,
        "opacity": 100,
        "groupIds": [],
        "frameId": None,
        "roundness": {"type": 2},
        "seed": next_seed(),
        "version": 1,
        "versionNonce": next_seed(),
        "isDeleted": False,
        "boundElements": None,
        "updated": 1700000000000,
        "link": None,
        "locked": False,
        "points": [[0, 0], [dx, dy]],
        "lastCommittedPoint": None,
        "startBinding": None,
        "endBinding": None,
        "startArrowhead": None,
        "endArrowhead": "arrow",
    }


def box_with_label(x, y, w, h, label, bg="#a5d8ff", fontsize=16):
    elements = []
    _, r = rect(x, y, w, h, bg=bg)
    elements.append(r)
    _, t = text(x + 8, y + h / 2 - (fontsize * label.count(chr(10)) + fontsize) / 2,
                w - 16, fontsize * (label.count("\n") + 1) + 6, label, size=fontsize)
    elements.append(t)
    return elements, (x, y, w, h)


def scene(elements):
    return {
        "type": "excalidraw",
        "version": 2,
        "source": "https://excalidraw.com",
        "elements": elements,
        "appState": {
            "gridSize": 20,
            "viewBackgroundColor": "#ffffff"
        },
        "files": {}
    }


def save(path, elements):
    with open(path, "w", encoding="utf-8") as f:
        json.dump(scene(elements), f, ensure_ascii=False, indent=2)
    print("wrote", path, "elements:", len(elements))


OUT_DIR = os.path.join(os.path.dirname(__file__), "diagrams")
os.makedirs(OUT_DIR, exist_ok=True)

# ============================================================
# 图 1：VPP 中台服务全景图（业务视角，简化版）
# ============================================================
els = []

def add_box(x, y, w, h, label, bg="#a5d8ff", fontsize=15):
    box_els, geo = box_with_label(x, y, w, h, label, bg, fontsize)
    els.extend(box_els)
    return geo


def center_top(geo):
    x, y, w, h = geo
    return x + w / 2, y


def center_bottom(geo):
    x, y, w, h = geo
    return x + w / 2, y + h


def center_left(geo):
    x, y, w, h = geo
    return x, y + h / 2


def center_right(geo):
    x, y, w, h = geo
    return x + w, y + h / 2


def add_arrow(g1, g2, label=None, dashed=False, color="#1e1e1e", from_side="bottom", to_side="top", label_size=13):
    fn = {"top": center_top, "bottom": center_bottom, "left": center_left, "right": center_right}
    x1, y1 = fn[from_side](g1)
    x2, y2 = fn[to_side](g2)
    _, a = arrow(x1, y1, x2, y2, dashed=dashed, color=color)
    els.append(a)
    if label:
        mx, my = (x1 + x2) / 2, (y1 + y2) / 2
        _, t = text(mx - 60, my - 10, 120, 18, label, size=label_size, color="#495057")
        els.append(t)

# 标题
_, title = text(40, 0, 900, 40, "VPP 中台服务全景图（业务视角 · 简化版）", size=24)
els.append(title)

# Row 0：外部世界
g_ems = add_box(40, 70, 220, 70, "现场设备 / EMS", bg="#ffd8a8")
g_admin = add_box(320, 70, 220, 70, "运维 / 管理端", bg="#ffd8a8")
g_casdoor = add_box(600, 70, 220, 70, "Casdoor\n身份认证中心", bg="#eebefa")

# Row 1：统一网关
g_apisix = add_box(160, 200, 320, 70, "APISIX 统一北向网关\n（认证 · 路由 · 限流）", bg="#ffec99")
g_sim = add_box(600, 200, 220, 70, "Simulator\n虚拟设备沙盘", bg="#b2f2bb")

# Row 2：核心业务服务
g_res = add_box(40, 340, 200, 80, "Resource\n资产台账（有什么设备）", bg="#a5d8ff")
g_gw = add_box(280, 340, 200, 80, "Gateway\n协议网关（内外身份翻译）", bg="#a5d8ff")
g_tel = add_box(520, 340, 200, 80, "Telemetry\n遥测中心（数据存哪/怎么查）", bg="#a5d8ff")
g_dis = add_box(760, 340, 200, 80, "Dispatch\n调度执行（指令怎么下发）", bg="#a5d8ff")

# Row 3：智能 + 告警
g_opt = add_box(150, 480, 200, 80, "Optimization\n自动决策（该不该动/动多少）", bg="#d0bfff")
g_fc = add_box(400, 480, 200, 80, "Forecast\n预测分析（接下来会怎样）", bg="#d0bfff")
g_alarm = add_box(650, 480, 200, 80, "Alarm\n告警中心（出问题怎么办）", bg="#ffc9c9")

# Row 4：事件总线
g_kafka = add_box(150, 620, 700, 60, "Kafka 事件总线（资源事件 / 命令终态 / 任务事件 / SOE 变位）", bg="#e9ecef", fontsize=15)

# Row 5：存储
g_db = add_box(150, 720, 700, 55, "Postgres（各服务独立库）  ·  Redis（快照 / 缓存）", bg="#e9ecef", fontsize=15)

# 箭头：外部 -> 网关
add_arrow(g_ems, g_apisix, "X-API-KEY")
add_arrow(g_admin, g_apisix, "Bearer JWT")
add_arrow(g_casdoor, g_apisix, "签发/验签", dashed=True)

# 网关 -> 核心服务
add_arrow(g_apisix, g_res, "/resource/*")
add_arrow(g_apisix, g_gw, "/gateway/*")
add_arrow(g_apisix, g_dis, "SubmitTask", from_side="right", to_side="top")

# Gateway <-> Telemetry / Simulator
add_arrow(g_gw, g_tel, "上报遥测")
add_arrow(g_sim, g_gw, "遥测/命令回执", from_side="left", to_side="right")
add_arrow(g_dis, g_gw, "下发指令", from_side="left", to_side="right")

# Optimization / Forecast
add_arrow(g_tel, g_opt, "读快照", from_side="bottom", to_side="top")
add_arrow(g_opt, g_dis, "自动下发", from_side="right", to_side="bottom", label_size=12)
add_arrow(g_tel, g_fc, "读历史", from_side="bottom", to_side="top")

# 告警来源
add_arrow(g_dis, g_alarm, "任务失败", dashed=True, from_side="bottom", to_side="top")
add_arrow(g_tel, g_alarm, "SOE 变位", dashed=True, from_side="bottom", to_side="left")

# Kafka 总线（示意性连接，代表事件解耦而非直接调用）
add_arrow(g_res, g_kafka, "资源事件", dashed=True, color="#868e96")
add_arrow(g_opt, g_kafka, "", dashed=True, color="#868e96")
add_arrow(g_alarm, g_kafka, "", dashed=True, color="#868e96")
add_arrow(g_kafka, g_db, "", dashed=True, color="#868e96")

save(os.path.join(OUT_DIR, "01-service-overview.excalidraw"), els)

# ============================================================
# 图 2：核心业务闭环（数据 -> 决策 -> 执行 -> 反馈 -> 告警）
# ============================================================
els2 = []
SEED[0] = 5000


def add_box2(x, y, w, h, label, bg="#a5d8ff", fontsize=15):
    box_els, geo = box_with_label(x, y, w, h, label, bg, fontsize)
    els2.extend(box_els)
    return geo


def add_arrow2(g1, g2, label=None, dashed=False, color="#1e1e1e", from_side="right", to_side="left", label_size=13, bend=None):
    fn = {"top": center_top, "bottom": center_bottom, "left": center_left, "right": center_right}
    x1, y1 = fn[from_side](g1)
    x2, y2 = fn[to_side](g2)
    if bend:
        aid = next_id("arrow")
        p0 = (0, 0)
        p_mid = (bend[0] - x1, bend[1] - y1)
        p_end = (x2 - x1, y2 - y1)
        a = {
            "id": aid, "type": "arrow", "x": x1, "y": y1,
            "width": abs(x2 - x1), "height": abs(y2 - y1), "angle": 0,
            "strokeColor": color, "backgroundColor": "transparent", "fillStyle": "solid",
            "strokeWidth": 2, "strokeStyle": "dashed" if dashed else "solid", "roughness": 1,
            "opacity": 100, "groupIds": [], "frameId": None, "roundness": {"type": 2},
            "seed": next_seed(), "version": 1, "versionNonce": next_seed(), "isDeleted": False,
            "boundElements": None, "updated": 1700000000000, "link": None, "locked": False,
            "points": [list(p0), list(p_mid), list(p_end)],
            "lastCommittedPoint": None, "startBinding": None, "endBinding": None,
            "startArrowhead": None, "endArrowhead": "arrow",
        }
        els2.append(a)
    else:
        _, a = arrow(x1, y1, x2, y2, dashed=dashed, color=color)
        els2.append(a)
    if label:
        mx, my = (x1 + x2) / 2, (y1 + y2) / 2
        _, t = text(mx - 70, my - 22, 140, 18, label, size=label_size, color="#495057")
        els2.append(t)

_, title2 = text(40, 0, 1100, 40, "核心业务闭环：数据 → 决策 → 执行 → 反馈 → 告警", size=24)
els2.append(title2)

g1 = add_box2(40, 140, 200, 90, "① 现场设备\n/ Simulator\n持续产生数据", bg="#ffd8a8")
g2 = add_box2(320, 140, 200, 90, "② Gateway\n协议归一\n内外身份翻译", bg="#a5d8ff")
g3 = add_box2(600, 140, 200, 90, "③ Telemetry\n落库 + 快照\n+ 变位事件(SOE)", bg="#a5d8ff")
g4 = add_box2(880, 140, 220, 90, "④ Optimization\n/ Forecast\n决策与预测", bg="#d0bfff")
g5 = add_box2(880, 320, 220, 90, "⑤ Dispatch\n任务编排\n下发控制指令", bg="#a5d8ff")
g6 = add_box2(560, 460, 260, 80, "⑥ Alarm 告警中心\n合单 / 通知 / 处理跟踪", bg="#ffc9c9")

add_arrow2(g1, g2, "遥测上报")
add_arrow2(g2, g3, "IngestTelemetry")
add_arrow2(g3, g4, "读快照/历史")
add_arrow2(g4, g5, "自动下发\nSubmitTask", from_side="bottom", to_side="top")
add_arrow2(g5, g2, "ExecuteCommand", from_side="left", to_side="bottom", bend=(320, 400))
add_arrow2(g2, g1, "命令回执→设备执行", from_side="left", to_side="top", bend=(140, 400))

add_arrow2(g3, g6, "SOE 变位", dashed=True, from_side="bottom", to_side="top")
add_arrow2(g5, g6, "任务失败", dashed=True, from_side="bottom", to_side="right")

# 说明文字
_, note = text(40, 560, 1060, 60,
               "闭环说明：①②③是「遥测采集」主链路；④⑤是「智能决策 + 控制下发」主链路（人工也可跳过④直接从管理端下发）；\n"
               "⑥是「异常反馈」支线，任何一次任务失败或测点异常变位都会汇总成一张告警工单，供人工介入处理。",
               size=15, align="left", color="#495057")
els2.append(note)

save(os.path.join(OUT_DIR, "02-business-loop.excalidraw"), els2)

print("done")
