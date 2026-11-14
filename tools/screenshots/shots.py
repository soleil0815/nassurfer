# SPDX-License-Identifier: AGPL-3.0-only
"""Nimmt die README-Bilder auf (aufgerufen von tools/screenshots/run.sh)."""
import argparse
import os
import time

from PIL import Image
from playwright.sync_api import sync_playwright

DSM = "http://diskstation:5000"
VIEW = {"width": 1228, "height": 778}


def wait_title(page, text, timeout=20):
    end = time.time() + timeout
    while time.time() < end:
        title = page.evaluate("document.querySelector('.x-window-title').textContent")
        if text in title:
            return
        time.sleep(0.3)
    raise RuntimeError("Titel nicht erreicht: " + text)


def settle(page, sec=1.5):
    time.sleep(sec)


def shoot(page, out, name):
    path = os.path.join(out, name + ".png")
    page.screenshot(path=path)
    # 256 Farben reichen für Oberflächen und halten das Repo klein
    im = Image.open(path).convert("RGB")
    im.quantize(colors=256, method=Image.Quantize.MEDIANCUT, dither=Image.Dither.NONE).save(path, optimize=True)
    print("  ✔", name)


def run_lang(p, lang, out):
    de = lang == "ger"
    os.makedirs(out, exist_ok=True)
    browser = p.chromium.launch(channel="chrome", args=["--no-proxy-server"])
    ctx = browser.new_context(viewport=VIEW, device_scale_factor=1,
                              locale="de-DE" if de else "en-US", timezone_id="Europe/Berlin")
    page = ctx.new_page()
    page.goto(DSM + "/dsm.html?lang=" + lang)
    page.wait_for_function("window.win && window.win.urlField")

    # Ausgangslage: frischer Browser, drei Favoriten, zuletzt geöffnete Adressen
    page.evaluate("""async (de) => {
        const w = window.win;
        await w.api('reset', 'POST').catch(() => {});
        const cur = await w.api('bookmarks');
        for (const f of (cur.items || [])) await w.api('bookmarks', 'POST', {op: 'remove', url: f.url});
        const favs = [
            ['http://router.lan/', de ? 'Heimrouter – Übersicht' : 'Home router – Overview'],
            ['http://drucker.lan/', de ? 'Büro-Drucker – Status' : 'Office printer – Status'],
            ['http://container.lan/', de ? 'Container – Übersicht' : 'Containers – Overview'],
        ];
        let j;
        for (const [url, title] of favs) j = await w.api('bookmarks', 'POST', {op: 'add', url, title});
        localStorage.setItem('NASsurfer.recent', JSON.stringify(
            ['http://container.lan/', 'https://192.168.1.30:8006/', 'http://drucker.lan/', 'http://router.lan/']));
        w.setFavs(j.items);
        w.showStart(true);
    }""", de)
    settle(page)
    shoot(page, out, "start")

    # Router öffnen, zwei weitere Tabs
    page.evaluate("window.win.navigate('http://router.lan/')")
    wait_title(page, "Heimrouter" if de else "Home router")
    page.evaluate("window.win.client.tabNew('http://drucker.lan/')")
    settle(page, 2)
    page.evaluate("window.win.client.tabNew('http://container.lan/')")
    settle(page, 2)
    page.locator(".nas-tab").first.click()
    wait_title(page, "Heimrouter" if de else "Home router")
    page.mouse.move(5, 770)
    settle(page, 2)
    shoot(page, out, "browser-tabs")

    # Favoriten-Menü
    page.get_by_role("button", name="★ " + ("Favoriten" if de else "Favorites")).click()
    settle(page, 0.8)
    shoot(page, out, "favorites")
    page.mouse.click(600, 770)

    # Container-Tab
    page.locator(".nas-tab").nth(2).click()
    wait_title(page, "Container")
    settle(page, 2)
    shoot(page, out, "containers")

    # Drucker-Tab: Bericht herunterladen
    page.locator(".nas-tab").nth(1).click()
    wait_title(page, "Drucker" if de else "printer")
    settle(page, 1.5)
    page.evaluate("window.win.client.nav('http://drucker.lan/bericht.pdf')")
    page.wait_for_selector(".x-msg")
    settle(page, 1)
    shoot(page, out, "download")
    page.locator(".x-msg .x-btn").first.click()

    # ⋯ → Automatische Updates einrichten
    page.locator(".x-toolbar .x-btn", has_text="⋯").click()
    settle(page, 0.5)
    page.locator(".x-menu-item").first.click()
    page.wait_for_selector(".x-msg")
    settle(page, 0.5)
    shoot(page, out, "update-help")
    page.locator(".x-msg .x-btn").first.click()
    # Browser auf der "NAS" beenden, damit die nächste Sprache frisch startet
    page.evaluate("window.win.client.api('stop')")

    # Paket-Zentrum (vereinfacht nachgestellt, Daten aus der echten Paketquelle)
    for view, name in (("source", "pkg-source"), ("update", "pkg-update"), ("detail", "pkg-detail")):
        page.goto(DSM + "/pkgcenter.html?view=" + view + "&lang=" + lang)
        page.wait_for_function("window.__ready === true")
        settle(page, 0.5)
        shoot(page, out, name)
    browser.close()


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--out", required=True)
    a = ap.parse_args()
    with sync_playwright() as p:
        for lang, sub in (("ger", "de"), ("enu", "en")):
            print("Bilder", sub)
            run_lang(p, lang, os.path.join(a.out, sub))


if __name__ == "__main__":
    main()
