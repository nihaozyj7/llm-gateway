"""生成网关应用图标:渐变分流箭头(多渠道聚合转发)。
输出:assets/icon/app_icon.svg、app_icon_512.png、favicon.ico(多尺寸)
"""
import math
import os
from PIL import Image, ImageDraw

OUT = os.path.join(os.path.dirname(os.path.abspath(__file__)), '..', 'assets', 'icon')
os.makedirs(OUT, exist_ok=True)

S = 512  # 基础尺寸
SS = 4   # 超采样倍率,抗锯齿
W = S * SS
RADIUS = 110 * SS  # 圆角半径

# ---- 背景:垂直渐变(深紫 -> 近黑) ----
bg_top = (26, 15, 46)    # #1a0f2e
bg_bot = (11, 11, 16)    # #0b0b10

def lerp(a, b, t):
    return tuple(int(a[i] + (b[i] - a[i]) * t) for i in range(3))

img = Image.new('RGB', (W, W), (0, 0, 0))
d = ImageDraw.Draw(img)
for y in range(W):
    t = y / (W - 1)
    d.line([(0, y), (W, y)], fill=lerp(bg_top, bg_bot, t))

# 圆角裁剪
mask = Image.new('L', (W, W), 0)
md = ImageDraw.Draw(mask)
md.rounded_rectangle([0, 0, W - 1, W - 1], radius=RADIUS, fill=255)
img.putalpha(mask)

def scale(x):  # 坐标从 512 基础换算到超采样画布
    return int(x * SS)

def line_color(color, width, x0, y0, x1, y1, cap_arrow=None):
    """画一条线(可选末端箭头)。cap_arrow=(arrow_len, arrow_halfw)"""
    d.line([(scale(x0), scale(y0)), (scale(x1), scale(y1))], fill=color, width=scale(width))
    if cap_arrow:
        alen, ahw = cap_arrow
        # 方向向量
        dx, dy = x1 - x0, y1 - y0
        L = math.hypot(dx, dy) or 1
        ux, uy = dx / L, dy / L
        # 箭头尾(起点)在线上 x1 后退 alen 处
        bx, by = x1 - ux * alen, y1 - uy * alen
        # 垂直方向
        px, py = -uy, ux
        tip = (scale(x1), scale(y1))
        base1 = (scale(bx + px * ahw), scale(by + py * ahw))
        base2 = (scale(bx - px * ahw), scale(by - py * ahw))
        d.polygon([tip, base1, base2], fill=color)

# 汇聚点(右侧主箭头起点)
CX, CY = 268, 256
OUT_LEN = 148
OUT_W = 44

# 主输出箭头:分段渐变 蓝 -> 绿
c_from = (56, 189, 248)   # #38bdf8  sky-400
c_to = (52, 211, 153)     # #34d399  emerald-400
seg = 6  # 每段长度(512 单位)
n_seg = max(1, int(OUT_LEN / seg))
for i in range(n_seg):
    t0 = i / n_seg
    t1 = (i + 1) / n_seg
    x0 = CX + OUT_LEN * t0
    x1 = CX + OUT_LEN * t1
    c = lerp(c_from, c_to, (t0 + t1) / 2)
    line_color(c, OUT_W, x0, CY, x1, CY)
# 主箭头头(三角形,渐变绿)
head_len, head_hw = 56, 40
line_color(c_to, 1, CX + OUT_LEN, CY, CX + OUT_LEN + head_len, CY, cap_arrow=(head_len, head_hw))

# 三条输入线:青 / 靛蓝 / 紫,汇聚到 (CX, CY) 左侧
inputs = [
    ((56, 189, 248), (78, 118), (250, 222), 26),   # 上:青  #38bdf8
    ((129, 140, 248), (78, 256), (250, 256), 26),  # 中:靛蓝 #818cf8
    ((167, 139, 250), (78, 394), (250, 290), 26),  # 下:紫 #a78bfa
]
for color, (x0, y0), (x1, y1), w in inputs:
    line_color(color, w, x0, y0, x1, y1, cap_arrow=(26, 16))

# 缩小到 512(LANCZOS 抗锯齿)
img512 = img.resize((S, S), Image.LANCZOS).convert('RGBA')
png_path = os.path.join(OUT, 'app_icon_512.png')
img512.save(png_path)
print('saved', png_path)

# ---- favicon.ico:多尺寸 ----
sizes = [(16, 16), (32, 32), (48, 48), (64, 64), (128, 128), (256, 256)]
imgs = [img.resize(sz, Image.LANCZOS).convert('RGBA') for sz in sizes]
ico_path = os.path.join(OUT, 'favicon.ico')
imgs[0].save(ico_path, format='ICO', sizes=[(s[0], s[1]) for s in sizes], append_images=imgs[1:])
print('saved', ico_path)
