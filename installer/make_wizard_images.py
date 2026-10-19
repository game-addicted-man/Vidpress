# Generates the installer wizard images from ../assets/icon_1024.png.
# Requires Pillow (pip install pillow) and a font with Korean glyphs.
# Run from this folder:  python make_wizard_images.py
import os
from PIL import Image, ImageDraw, ImageFont

HERE = os.path.dirname(os.path.abspath(__file__))
ICON = os.path.join(HERE, "..", "assets", "icon_1024.png")
FONT_CANDIDATES = [
    ("/usr/share/fonts/opentype/noto/NotoSansCJK-Bold.ttc", 1),
    ("C:/Windows/Fonts/malgunbd.ttf", 0),
]
FONT_REG_CANDIDATES = [
    ("/usr/share/fonts/opentype/noto/NotoSansCJK-Regular.ttc", 1),
    ("C:/Windows/Fonts/malgun.ttf", 0),
]

def font(cands, size):
    for path, idx in cands:
        if os.path.exists(path):
            return ImageFont.truetype(path, size, index=idx)
    return ImageFont.load_default()

TOP, BOT = (139, 70, 245), (30, 102, 235)

def gradient(w, h):
    img = Image.new("RGB", (w, h))
    px = img.load()
    for y in range(h):
        for x in range(w):
            t = min(1.0, max(0.0, (x * 0.25 + y) / (h + w * 0.25)))
            px[x, y] = tuple(int(TOP[i] + (BOT[i] - TOP[i]) * t) for i in range(3))
    return img

def glyph(draw, cx, cy, s, fill):
    # the VidPress mark: a "press plate" above a wide play triangle
    draw.rounded_rectangle([cx - s * 0.62, cy - s * 0.55, cx + s * 0.62, cy - s * 0.33], radius=s * 0.1, fill=fill)
    draw.polygon([(cx - s * 0.42, cy - s * 0.18), (cx + s * 0.5, cy + 0.17 * s), (cx - s * 0.42, cy + s * 0.52)], fill=fill)

def large(scale):
    w, h = int(164 * scale), int(314 * scale)
    img = gradient(w, h).convert("RGBA")
    d = ImageDraw.Draw(img)
    glyph(d, w / 2, h * 0.30, 46 * scale, (255, 255, 255, 255))
    f1 = font(FONT_CANDIDATES, int(23 * scale))
    f2 = font(FONT_REG_CANDIDATES, int(11.5 * scale))
    def center(text, y, f, fill):
        tw = d.textlength(text, font=f)
        d.text(((w - tw) / 2, y), text, font=f, fill=fill)
    center("VidPress", h * 0.47, f1, (255, 255, 255, 255))
    center("동영상 용량 줄이기", h * 0.585, f2, (255, 255, 255, 225))
    center("형식 바꾸기", h * 0.585 + 17 * scale, f2, (255, 255, 255, 225))
    return img.convert("RGB")

def small(size):
    return Image.open(ICON).convert("RGBA").resize((size, size), Image.LANCZOS)

for scale, tag in [(1, "100"), (1.5, "150"), (2, "200")]:
    large(scale).save(os.path.join(HERE, "wizard-large-%s.png" % tag), optimize=True)
    small(int(round(55 * scale))).save(os.path.join(HERE, "wizard-small-%s.png" % tag), optimize=True)
print("ok")
