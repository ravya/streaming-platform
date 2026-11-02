"""Tiny SVG builder so the diagrams are code (reviewable, diff-able, regenerable)."""
from html import escape

PAL = {  # fill, stroke
    "src": ("#EEF2F7", "#64748B"), "edge": ("#FDEBD3", "#C2410C"), "ingest": ("#DCEBFE", "#1D4ED8"),
    "kafka": ("#E9E5FF", "#6D28D9"), "stream": ("#F3E1FA", "#A21CAF"), "store": ("#DDF5E4", "#15803D"),
    "serve": ("#FFE8CC", "#EA580C"), "user": ("#EEF2F7", "#475569"), "batch": ("#FEF3C7", "#B45309"),
    "xc": ("#E0F2F1", "#0F766E"), "warn": ("#FEE2E2", "#B91C1C"), "ext": ("#F1F5F9", "#475569"),
}

class SVG:
    def __init__(self, w, h, title):
        self.w, self.h, self.o = w, h, []
        self.o.append(f'<svg xmlns="http://www.w3.org/2000/svg" width="{w}" height="{h}" viewBox="0 0 {w} {h}" '
                      f'font-family="Helvetica, Arial, DejaVu Sans, sans-serif">')
        self.o.append('<defs>'
            '<marker id="a" viewBox="0 0 10 10" refX="9" refY="5" markerWidth="7" markerHeight="7" orient="auto-start-reverse"><path d="M0,0 L10,5 L0,10 z" fill="#334155"/></marker>'
            '<marker id="ar" viewBox="0 0 10 10" refX="9" refY="5" markerWidth="7" markerHeight="7" orient="auto-start-reverse"><path d="M0,0 L10,5 L0,10 z" fill="#B91C1C"/></marker>'
            '<marker id="ag" viewBox="0 0 10 10" refX="9" refY="5" markerWidth="7" markerHeight="7" orient="auto-start-reverse"><path d="M0,0 L10,5 L0,10 z" fill="#B45309"/></marker>'
            '</defs>')
        self.o.append(f'<rect width="{w}" height="{h}" fill="#FFFFFF"/>')
        self.text(24, 34, title, 22, "#0F172A", bold=True)

    def text(self, x, y, s, size=12, fill="#0F172A", bold=False, anchor="start", italic=False):
        wt = ' font-weight="700"' if bold else ""
        it = ' font-style="italic"' if italic else ""
        self.o.append(f'<text x="{x}" y="{y}" font-size="{size}" fill="{fill}" text-anchor="{anchor}"{wt}{it}>{escape(s)}</text>')

    def lane(self, x, y, w, h, label, fill="#F8FAFC", stroke="#CBD5E1", lab_color="#475569"):
        self.o.append(f'<rect x="{x}" y="{y}" width="{w}" height="{h}" rx="10" fill="{fill}" stroke="{stroke}" stroke-dasharray="5 4"/>')
        self.text(x + 10, y + 18, label, 12, lab_color, bold=True)

    def box(self, x, y, w, h, title, lines=(), kind="ingest", size=11):
        f, s = PAL[kind]
        self.o.append(f'<rect x="{x}" y="{y}" width="{w}" height="{h}" rx="7" fill="{f}" stroke="{s}" stroke-width="1.4"/>')
        self.text(x + w / 2, y + 17, title, 12.5, s, bold=True, anchor="middle")
        for i, ln in enumerate(lines):
            self.text(x + w / 2, y + 34 + i * 14.5, ln, size, "#1E293B", anchor="middle")
        return (x, y, w, h)

    def arrow(self, pts, label="", kind="n", dash=False, lx=None, ly=None, lanchor="middle"):
        mk = {"n": "a", "r": "ar", "g": "ag"}[kind]
        col = {"n": "#334155", "r": "#B91C1C", "g": "#B45309"}[kind]
        d = "M" + " L".join(f"{x},{y}" for x, y in pts)
        da = ' stroke-dasharray="6 4"' if dash else ""
        self.o.append(f'<path d="{d}" fill="none" stroke="{col}" stroke-width="1.6"{da} marker-end="url(#{mk})"/>')
        if label:
            (x1, y1), (x2, y2) = pts[0], pts[-1]
            tx = lx if lx is not None else (x1 + x2) / 2
            ty = ly if ly is not None else (y1 + y2) / 2 - 5
            self.o.append(f'<text x="{tx}" y="{ty}" font-size="10.5" fill="{col}" text-anchor="{lanchor}" paint-order="stroke" stroke="#fff" stroke-width="3">{escape(label)}</text>')

    def badge(self, cx, cy, n, color="#0F172A"):
        self.o.append(f'<circle cx="{cx}" cy="{cy}" r="11" fill="{color}"/>')
        self.text(cx, cy + 4, str(n), 11.5, "#fff", bold=True, anchor="middle")

    def save(self, path):
        self.o.append("</svg>")
        open(path, "w").write("\n".join(self.o))
