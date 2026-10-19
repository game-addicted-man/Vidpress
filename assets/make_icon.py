# Generates the VidPress icon (app.ico + PNG sizes). Requires Pillow: pip install pillow
# Run from this folder:  python make_icon.py
from PIL import Image, ImageDraw, ImageFilter
S = 1024
def bg(size):
    pad = int(size*0.04); rad = int(size*0.22)
    grad = Image.new("RGBA", (size, size))
    top = (139, 70, 245); bot = (30, 102, 235)
    px = grad.load()
    for y in range(size):
        for x in range(size):
            t = min(1, max(0, (x*0.3 + y*0.7) / size))
            px[x, y] = tuple(int(top[i] + (bot[i]-top[i])*t) for i in range(3)) + (255,)
    # soft top highlight (fades out)
    hl = Image.new("RGBA", (size, size), (255,255,255,0)); hp = hl.load()
    for y in range(size):
        a = int(34 * max(0, 1 - y/(size*0.55)))
        for x in range(size):
            hp[x, y] = (255,255,255,a)
    grad = Image.alpha_composite(grad, hl)
    mask = Image.new("L", (size, size), 0)
    ImageDraw.Draw(mask).rounded_rectangle([pad, pad, size-pad, size-pad], radius=rad, fill=255)
    out = Image.new("RGBA", (size, size), (0,0,0,0)); out.paste(grad, (0,0), mask)
    return out

def symbol_layer(size, simple):
    lay = Image.new("RGBA", (size, size), (0,0,0,0)); d = ImageDraw.Draw(lay)
    W = (255,255,255,255)
    pw = 0.62 if not simple else 0.68
    d.rounded_rectangle([size*(0.5-pw/2), size*0.21, size*(0.5+pw/2), size*(0.32 if not simple else 0.35)], radius=size*0.05, fill=W)
    ty0 = size*(0.43 if not simple else 0.46); ty1 = size*0.79
    d.polygon([(size*0.30, ty0), (size*0.75, (ty0+ty1)/2), (size*0.30, ty1)], fill=W)
    return lay

def compose(size, simple=False):
    b = bg(size)
    sym = symbol_layer(size, simple)
    if not simple:
        sh = Image.new("RGBA", (size,size), (0,0,0,0))
        sh.paste((20,10,60,90), (0, int(size*0.012)), sym)
        sh = sh.filter(ImageFilter.GaussianBlur(size*0.018))
        b = Image.alpha_composite(b, sh)
    return Image.alpha_composite(b, sym)

full = compose(S); simple = compose(S, True)
full.save("icon_1024.png")
sizes = [256, 128, 96, 64, 48, 40, 32, 24, 20, 16]
ims = []
for s in sizes:
    im = (full if s >= 32 else simple).resize((s, s), Image.LANCZOS)
    im.save(f"icon_{s}.png"); ims.append(im)
ims[0].save("app.ico", sizes=[(s,s) for s in sizes], append_images=ims[1:])
prev = Image.new('RGBA', (560, 300), (240,240,240,255)); x = 10
for s in [256,128,64,48,32,24,16]:
    im = Image.open(f'icon_{s}.png'); prev.paste(im, (x, 280 - s), im); x += s + 10
prev.save('icon_preview.png')
print("ok")
