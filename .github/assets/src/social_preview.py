"""Builds .github/assets/social-preview.png (1280x640) from a real frame of
docs/assets/demos/chat.gif, used as captured (companion panel included), then
framed with the logo and tagline.

    python .github/assets/src/social_preview.py
"""
from pathlib import Path
from PIL import Image, ImageDraw, ImageFilter, ImageFont

ROOT = Path(__file__).resolve().parents[3]
OUT = ROOT / ".github/assets/social-preview.png"
GIF = ROOT / "docs/assets/demos/chat.gif"
FONTS = Path("C:/Windows/Fonts")

BG = (19, 17, 28)
LAV, PINK = (167, 139, 250), (244, 114, 182)



def font(name, size):
    return ImageFont.truetype(str(FONTS / name), size)


def tui_frame():
    gif = Image.open(GIF)
    gif.seek(gif.n_frames - 1)
    return gif.convert("RGB")


def gradient_text(img, xy, text, fnt):
    mask = Image.new("L", img.size)
    ImageDraw.Draw(mask).text(xy, text, font=fnt, fill=255)
    box = mask.getbbox()
    grad = Image.new("RGB", img.size)
    gd = ImageDraw.Draw(grad)
    for x in range(box[0], box[2] + 1):
        t = (x - box[0]) / max(1, box[2] - box[0])
        gd.line((x, 0, x, img.height),
                fill=tuple(round(a + (b - a) * t) for a, b in zip(LAV, PINK)))
    img.paste(grad, mask=mask)


def logo(size):
    s = size / 128
    img = Image.new("RGBA", (size, size))
    d = ImageDraw.Draw(img)
    d.rounded_rectangle((0, 0, size - 1, size - 1), radius=28 * s, fill=BG)
    bubble = Image.new("L", (size, size))
    bd = ImageDraw.Draw(bubble)
    bd.rounded_rectangle((20 * s, 22 * s, 108 * s, 88 * s), radius=22 * s, fill=255)
    bd.polygon([(36 * s, 80 * s), (58 * s, 88 * s), (36 * s, 106 * s)], fill=255)
    grad = Image.new("RGBA", (size, size))
    gd = ImageDraw.Draw(grad)
    for i in range(size * 2):
        t = i / (size * 2)
        gd.line((i, 0, 0, i), fill=tuple(round(a + (b - a) * t) for a, b in zip(LAV, PINK)))
    img.paste(grad, mask=bubble)
    d.line([(43 * s, 44 * s), (56 * s, 55 * s), (43 * s, 66 * s)], fill=BG,
           width=round(8 * s), joint="curve")
    d.rounded_rectangle((64 * s, 59 * s, 86 * s, 67 * s), radius=3 * s, fill=BG)
    return img


def main():
    W, H = 1280, 640
    img = Image.new("RGB", (W, H), BG)
    glow = Image.new("RGB", (W, H), BG)
    gd = ImageDraw.Draw(glow)
    gd.ellipse((-260, -300, 420, 300), fill=(62, 46, 104))
    gd.ellipse((900, 420, 1500, 900), fill=(78, 36, 72))
    img = Image.blend(img, glow.filter(ImageFilter.GaussianBlur(140)), 1.0)

    # Real TUI window, scaled, with a soft shadow and a thin border.
    tui = tui_frame()
    tw = 860
    th = round(tui.height * tw / tui.width)
    tui = tui.resize((tw, th), Image.LANCZOS)
    tx, ty = W - tw - 40, (H - th) // 2 + 14
    shadow = Image.new("L", (W, H))
    ImageDraw.Draw(shadow).rounded_rectangle((tx, ty + 12, tx + tw, ty + th + 12), 14, fill=170)
    img.paste((5, 4, 10), mask=shadow.filter(ImageFilter.GaussianBlur(22)))
    m = Image.new("L", (tw, th))
    ImageDraw.Draw(m).rounded_rectangle((0, 0, tw - 1, th - 1), 12, fill=255)
    img.paste(tui, (tx, ty), m)
    d = ImageDraw.Draw(img)
    d.rounded_rectangle((tx, ty, tx + tw - 1, ty + th - 1), 12, outline=(68, 60, 100), width=2)

    # Left column: logo, name, tagline, tags.
    lx = 48
    img.paste(logo(64), (lx, 76), logo(64))
    gradient_text(img, (lx + 78, 74), "Visper", font("CascadiaMono.ttf", 52))
    tag = font("CascadiaMono.ttf", 25)
    y = 210
    for line, accent in [("Anonymous,", False), ("end-to-end", True),
                         ("encrypted chat", True), ("rooms, right", False),
                         ("from your", False), ("terminal.", False)]:
        d.text((lx, y), line, font=tag, fill=PINK if accent else (240, 238, 248))
        y += 36
    chip = font("CascadiaMono.ttf", 15)
    cx, cy = lx, 450
    for t in ["no sign-up", "E2EE", "TUI + web", "self-hosted"]:
        w = d.textlength(t, font=chip) + 22
        if cx + w > lx + 290:
            cx, cy = lx, cy + 40
        d.rounded_rectangle((cx, cy, cx + w, cy + 30), 7, fill=(36, 32, 54), outline=(72, 64, 104))
        d.text((cx + 11, cy + 6), t, font=chip, fill=(205, 200, 230))
        cx += w + 10
    img.save(OUT, optimize=True)
    print("wrote", OUT)


if __name__ == "__main__":
    main()
