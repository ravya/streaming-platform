import sys, asyncio
from playwright.sync_api import sync_playwright
with sync_playwright() as p:
    b = p.chromium.launch(); 
    for svg in sys.argv[1:]:
        pg = b.new_page(device_scale_factor=1.6)
        pg.goto("file://" + __import__("os").path.abspath(svg))
        dim = pg.evaluate("[document.documentElement.getAttribute('width'), document.documentElement.getAttribute('height')]")
        pg.set_viewport_size({"width": int(dim[0]), "height": int(dim[1])})
        pg.screenshot(path=svg.replace(".svg", ".png"))
    b.close()
