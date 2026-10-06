#!/usr/bin/env python3
"""只生成 Optimization 服务图，不覆盖已手改过的 01 / 02。"""
import json
import os

SEED = [8000]


def nid(prefix):
    SEED[0] += 1
    return f"{prefix}-{SEED[0]}"


def nseed():
    SEED[0] += 1
    return SEED[0]


def base(kind, x, y, w, h, **extra):
    el = {
        "id": extra.pop("id", nid(kind[:4])),
        "type": kind,
        "x": x, "y": y, "width": w, "height": h,
        "angle": 0,
        "strokeColor": extra.pop("stroke", "#1e1e1e"),
        "backgroundColor": extra.pop("bg", "transparent"),
        "fillStyle": "solid",
        "strokeWidth": extra.pop("sw", 2),
        "strokeStyle": extra.pop("stroke_style", "solid"),
        "roughness": 1,
        "opacity": 100,
        "groupIds": [],
        "frameId": None,
        "roundness": extra.pop("roundness", {"type": 3}),
        "seed": nseed(),
        "version": 1,
        "versionNonce": nseed(),
        "isDeleted": False,
        "boundElements": None,
        "updated": 1700000000000,
        "link": None,
        "locked": False,
    }
    el.update(extra)
    return el


def rect(x, y, w, h, bg, stroke="#1e1e1e", dashed=False):
    return base(
        "rectangle", x, y, w, h,
        bg=bg, stroke=stroke,
        stroke_style="dashed" if dashed else "solid",
    )


def label(x, y, w, h, s, size=16, color="#1e1e1e", align="center"):
    lines = s.count("\n") + 1
    block_h = size * 1.25 * lines
    return base(
        "text", x, y + (h - block_h) / 2, w, block_h,
        stroke=color, roundness=None,
        text=s, fontSize=size, fontFamily=1,
        textAlign=align, verticalAlign="middle",
        baseline=int(size + 2), containerId=None,
        originalText=s, lineHeight=1.25,
    )


def box(x, y, w, h, s, bg, size=16, stroke="#1e1e1e", dashed=False, color="#1e1e1e"):
    r = rect(x, y, w, h, bg, stroke=stroke, dashed=dashed)
    t = label(x + 10, y, w - 20, h, s, size=size, color=color)
    return [r, t], (x, y, w, h)


def side(geo, which):
    x, y, w, h = geo
    return {
        "top": (x + w / 2, y),
        "bottom": (x + w / 2, y + h),
        "left": (x, y + h / 2),
        "right": (x + w, y + h / 2),
    }[which]


def arrow(g1, g2, frm, to, dashed=False, color="#1e1e1e", start=None, end=None):
    x1, y1 = start or side(g1, frm)
    x2, y2 = end or side(g2, to)
    a = base(
        "arrow", x1, y1, abs(x2 - x1) or 1, abs(y2 - y1) or 1,
        stroke=color, roundness={"type": 2},
        stroke_style="dashed" if dashed else "solid",
        points=[[0, 0], [x2 - x1, y2 - y1]],
        lastCommittedPoint=None,
        startBinding=None, endBinding=None,
        startArrowhead=None, endArrowhead="arrow",
    )
    return a


def caption(x, y, w, s, size=14, color="#495057", align="center"):
    lines = s.count("\n") + 1
    h = size * 1.25 * lines
    return base(
        "text", x, y, w, h,
        stroke=color, roundness=None,
        text=s, fontSize=size, fontFamily=1,
        textAlign=align, verticalAlign="top",
        baseline=int(size + 2), containerId=None,
        originalText=s, lineHeight=1.25,
    )


def diamond(x, y, w, h, s, bg="#ffec99", size=16, stroke="#e67700", color="#1e1e1e"):
    r = base("diamond", x, y, w, h, bg=bg, stroke=stroke, roundness=None)
    t = label(x + 28, y, w - 56, h, s, size=size, color=color)
    return [r, t], (x, y, w, h)


els = []

els.append(caption(40, 12, 980, "Optimization：每 60 秒自己做一轮决策", size=24, color="#1e1e1e", align="left"))
els.append(caption(
    40, 48, 980,
    "主链是 v1 真正在走的。右侧虚线是本轮结束；底部虚线框是已接线但当前不用。",
    size=15, color="#868e96", align="left",
))

# 主链居中。菱形比矩形窄，中心线都落在 x=500。
spine_x, spine_w = 300, 400
cx = spine_x + spine_w / 2  # 500

loop, g_loop = box(
    spine_x, 100, spine_w, 64,
    "DecisionLoop\n每 60 秒一个 tick",
    "#d0bfff", size=16, stroke="#7048e8",
)
els.extend(loop)

tenants, g_tenants = box(
    spine_x, 200, spine_w, 56,
    "每个已配置租户各跑一轮",
    "#d0bfff", size=16, stroke="#7048e8",
)
els.extend(tenants)

cycle, g_cycle = box(
    spine_x, 292, spine_w, 56,
    "RunDecisionCycle",
    "#e5dbff", size=16, stroke="#7048e8",
)
els.extend(cycle)

tel, g_tel = box(
    spine_x, 384, spine_w, 72,
    "读 Telemetry 当前快照\n过期则本轮跳过",
    "#a5d8ff", size=16,
)
els.extend(tel)

soc, g_soc = box(
    spine_x, 492, spine_w, 72,
    "SOC 规则\n过低则充电，过高则放电",
    "#d0bfff", size=16, stroke="#7048e8",
)
els.extend(soc)

d_breach, g_breach = diamond(cx - 120, 604, 240, 140, "越限了？", size=18)
els.extend(d_breach)

d_cool, g_cool = diamond(cx - 120, 820, 240, 140, "这个方向\n还在冷却？", size=16)
els.extend(d_cool)

target, g_target = box(
    spine_x, 1000, spine_w, 64,
    "记下该方向的冷却\n产出一条 Target",
    "#d0bfff", size=16, stroke="#7048e8",
)
els.extend(target)

alloc, g_alloc = box(
    spine_x, 1100, spine_w, 72,
    "Allocate\nPointTarget 一比一变成命令",
    "#d0bfff", size=16, stroke="#7048e8",
)
els.extend(alloc)

dis, g_dis = box(
    spine_x, 1208, spine_w, 64,
    "Dispatch  SubmitTask\n自动任务，人工任务不经过这里",
    "#a5d8ff", size=16,
)
els.extend(dis)

skip, g_skip = box(
    760, 640, 250, 88,
    "本轮不动作\n等下一个 tick",
    "#f1f3f5", size=16, stroke="#868e96", dashed=True, color="#495057",
)
els.extend(skip)

cool_skip, g_cool_skip = box(
    760, 846, 250, 88,
    "这个方向先不再发\n反方向仍可以触发",
    "#f1f3f5", size=15, stroke="#868e96", dashed=True, color="#495057",
)
els.extend(cool_skip)

els.append(arrow(g_loop, g_tenants, "bottom", "top", color="#7048e8"))
els.append(arrow(g_tenants, g_cycle, "bottom", "top", color="#7048e8"))
els.append(arrow(g_cycle, g_tel, "bottom", "top", color="#7048e8"))
els.append(arrow(g_tel, g_soc, "bottom", "top", color="#7048e8"))
els.append(arrow(g_soc, g_breach, "bottom", "top", color="#7048e8"))
els.append(arrow(g_breach, g_cool, "bottom", "top", color="#2f9e44"))
els.append(caption(cx + 8, 748, 48, "是", size=15, color="#2f9e44", align="left"))
els.append(arrow(g_cool, g_target, "bottom", "top", color="#2f9e44"))
els.append(caption(cx + 8, 968, 48, "否", size=15, color="#2f9e44", align="left"))
els.append(arrow(g_target, g_alloc, "bottom", "top", color="#7048e8"))
els.append(arrow(g_alloc, g_dis, "bottom", "top", color="#7048e8"))

els.append(arrow(g_breach, g_skip, "right", "left", dashed=True, color="#868e96"))
els.append(caption(668, 612, 80, "否", size=15, color="#868e96", align="left"))
els.append(arrow(g_cool, g_cool_skip, "right", "left", dashed=True, color="#868e96"))
els.append(caption(668, 828, 80, "是", size=15, color="#868e96", align="left"))

fc, _ = box(
    40, 1360, 300, 100,
    "Forecast\n预测接口只是占位\nv1 规则不调用",
    "#f1f3f5", size=15, stroke="#868e96", dashed=True, color="#495057",
)
els.extend(fc)
res, _ = box(
    370, 1360, 320, 100,
    "Resource\n只在按额定容量分摊时才读\nv1 的 PointTarget 不调用",
    "#f1f3f5", size=15, stroke="#868e96", dashed=True, color="#495057",
)
els.extend(res)
note, _ = box(
    720, 1360, 300, 100,
    "不直连 Gateway / Alarm\n执行仍经 Dispatch\n冷却记在进程内存里",
    "#fff4e6", size=15, stroke="#e67700",
)
els.extend(note)

els.append(caption(
    40, 1480, 980,
    "没有入站业务接口，也没有自己的数据库。租户或阈值没配时，循环空转，进程照样能起来。",
    size=15, color="#495057", align="left",
))

out = os.path.join(os.path.dirname(__file__), "diagrams", "03-optimization.excalidraw")
doc = {
    "type": "excalidraw",
    "version": 2,
    "source": "https://excalidraw.com",
    "elements": els,
    "appState": {"gridSize": 20, "viewBackgroundColor": "#ffffff"},
    "files": {},
}
with open(out, "w", encoding="utf-8") as f:
    json.dump(doc, f, ensure_ascii=False, indent=2)
print("wrote", out, "elements", len(els))
