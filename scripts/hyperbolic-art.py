"""Generates web/public/art/hyperbolic.svg: the (2,3,7) triangle tiling of the
hyperbolic plane in the Poincare disk, colored in the MACE logo's alternating
colorways. Edges are true geodesics (circular arcs), sampled as polylines.

The file holds two paths, #even and #odd, without fills, so a page can color
them with the active palette through <use>.

Run: python3 scripts/hyperbolic-art.py
"""
import cmath, math

P, Q = 7, 3
R = 280.0          # drawing radius in SVG units
LIMIT = 0.975      # stop where triangles become too small to see
MIN_AREA = 4.0    # in SVG units squared

def to_disk(d):
    return math.tanh(d / 2)

r_mid = to_disk(math.acosh(math.cos(math.pi / Q) / math.sin(math.pi / P)))
r_vert = to_disk(math.acosh(1 / (math.tan(math.pi / P) * math.tan(math.pi / Q))))
base = (0j, complex(r_mid, 0), cmath.rect(r_vert, math.pi / P))

def move(p):            # isometry taking p to the origin
    return lambda z: (z - p) / (1 - p.conjugate() * z)

def unmove(p):
    return lambda z: (z + p) / (1 + p.conjugate() * z)

def reflect(a, b, z):
    """Reflect z across the geodesic through a and b."""
    t, u = move(a), unmove(a)
    w = t(b)
    theta = cmath.phase(w)
    return u(cmath.exp(2j * theta) * t(z).conjugate())

def geodesic(a, b, n):
    t, u = move(a), unmove(a)
    w = t(b)
    return [u(w * k / n) for k in range(n + 1)]

def area(tri):
    (x1, y1), (x2, y2), (x3, y3) = [(z.real * R, z.imag * R) for z in tri]
    return abs((x2 - x1) * (y3 - y1) - (x3 - x1) * (y2 - y1)) / 2

seen = set()
def key(tri):
    c = sum(tri) / 3
    return (round(c.real, 6), round(c.imag, 6))

tiles = [[], []]
queue = [(base, 0)]
seen.add(key(base))
while queue:
    tri, parity = queue.pop(0)
    tiles[parity].append(tri)
    for i in range(3):
        a, b, c = tri[i], tri[(i + 1) % 3], tri[(i + 2) % 3]
        nxt = (a, b, reflect(a, b, c))
        k = key(nxt)
        if k in seen or max(abs(z) for z in nxt) > LIMIT or area(nxt) < MIN_AREA:
            continue
        seen.add(k)
        queue.append((nxt, 1 - parity))

def path(tris):
    out = []
    for tri in tris:
        pts = []
        for i in range(3):
            a, b = tri[i], tri[(i + 1) % 3]
            size = abs(a - b) * R
            n = 1 if size < 14 else min(5, int(size / 14) + 1)
            pts += geodesic(a, b, n)[:-1]
        out.append("M" + "L".join(f"{z.real * R:.1f} {z.imag * R:.1f}" for z in pts) + "Z")
    return "".join(out)

even, odd = path(tiles[0]), path(tiles[1])
svg = (f'<svg xmlns="http://www.w3.org/2000/svg" viewBox="{-R} {-R} {2 * R} {2 * R}">'
       f'<path id="even" d="{even}"/><path id="odd" d="{odd}"/></svg>\n')
open("web/public/art/hyperbolic.svg", "w").write(svg)
print(len(tiles[0]) + len(tiles[1]), "triangles,", len(svg) // 1024, "KiB")
