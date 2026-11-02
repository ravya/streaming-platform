"""Build Word files from the Markdown docs: python3 scripts/build_docs.py (needs pandoc + python-docx)."""
import subprocess, sys
from docx import Document
from docx.shared import Inches, Pt
from docx.enum.section import WD_ORIENT
from docx.oxml.ns import qn
from docx.oxml import OxmlElement

PAGE_W = 10.4  # usable width in inches (landscape A4, 0.6in margins)
IMG_W = 8.6

def build(md, out):
    raw = out + ".raw.docx"
    subprocess.run(["pandoc", md, "-o", raw, "--resource-path=docs"], check=True)
    d = Document(raw)
    for s in d.sections:
        s.orientation = WD_ORIENT.LANDSCAPE
        s.page_width, s.page_height = Inches(11.69), Inches(8.27)
        for m in ("left_margin", "right_margin", "top_margin", "bottom_margin"):
            setattr(s, m, Inches(0.6))
    for st in ("Normal", "Body Text", "First Paragraph", "Compact"):
        try:
            d.styles[st].font.size = Pt(10); d.styles[st].font.name = "Calibri"
        except KeyError:
            pass
    for sh in d.inline_shapes:
        r = sh.height / sh.width
        sh.width = Inches(IMG_W); sh.height = int(Inches(IMG_W) * r)
    for t in d.tables:
        n = len(t.columns)
        w = [max(6, min(60, sum(len(r.cells[c].text) for r in t.rows) / len(t.rows))) for c in range(n)]
        widths = [Inches(PAGE_W * x / sum(w)) for x in w]
        t.autofit = False
        pr = t._tbl.tblPr
        for e in pr.findall(qn("w:tblW")): pr.remove(e)
        tw = OxmlElement("w:tblW"); tw.set(qn("w:w"), str(int(PAGE_W * 1440))); tw.set(qn("w:type"), "dxa"); pr.append(tw)
        lay = OxmlElement("w:tblLayout"); lay.set(qn("w:type"), "fixed"); pr.append(lay)
        b = OxmlElement("w:tblBorders")
        for e in ("top", "left", "bottom", "right", "insideH", "insideV"):
            x = OxmlElement(f"w:{e}"); x.set(qn("w:val"), "single"); x.set(qn("w:sz"), "4"); x.set(qn("w:color"), "A0A0A0"); b.append(x)
        pr.append(b)
        for gc, wd in zip(t._tbl.tblGrid.findall(qn("w:gridCol")), widths): gc.set(qn("w:w"), str(int(wd / 635)))
        for row in t.rows:
            for cell, wd in zip(row.cells, widths): cell.width = wd
        for cell in t.rows[0].cells:
            sh = OxmlElement("w:shd"); sh.set(qn("w:val"), "clear"); sh.set(qn("w:fill"), "E8EEF4"); cell._tc.get_or_add_tcPr().append(sh)
            for p in cell.paragraphs:
                for r in p.runs: r.bold = True
    d.save(out)

if __name__ == "__main__":
    build("docs/DESIGN.md", "docs/Design_Proposal.docx")
    build("docs/ASSUMPTIONS.md", "docs/Assumptions_and_Sizing.docx")
