# SPDX-License-Identifier: AGPL-3.0-only
"""Ende-zu-Ende-Tests für NASsurfer.

Ein Chrome auf dem Testrechner spielt den Benutzer: Er öffnet die Testseite des
nachgebauten DSM, die den echten Browser-Kern (client.js) lädt. Dieser spricht
über browser.cgi mit dem installierten Dienst und dem Chromium auf der "NAS".

Aufruf: python3 e2e.py [--chrome /pfad/zu/chrome] [--var /var/packages/NASsurfer/var]
Rückgabewert 0 = alles bestanden.
"""
import argparse
import asyncio
import os
import shutil
import sys
import time

from playwright.async_api import async_playwright

DSM = "http://127.0.0.1:9100"
DEV = "http://127.0.0.1:9101"

results = []


def check(name, ok, detail=""):
    results.append((name, bool(ok), detail))
    print(("  ✔ " if ok else "  ✘ ") + name + (f"  ({detail})" if detail and not ok else ""), flush=True)


async def wait_state(pg, pred, timeout=20):
    t = time.time()
    st = None
    while time.time() - t < timeout:
        st = await pg.evaluate("window.__st")
        if st and pred(st):
            return st
        await asyncio.sleep(0.15)
    return st or {}


async def nav(pg, url):
    await pg.evaluate("u => window.__c.nav(u)", url)


async def open_page(browser, locale="de-DE", tz="Europe/Berlin"):
    ctx = await browser.new_context(locale=locale, timezone_id=tz, viewport={"width": 1100, "height": 700})
    pg = await ctx.new_page()
    await pg.goto(DSM + "/test.html")
    return ctx, pg


async def canvas_origin(pg):
    return await pg.evaluate(
        "(()=>{var r=document.querySelector('#host canvas').getBoundingClientRect();return [r.left,r.top]})()")


async def api(pg, op, body=None):
    return await pg.evaluate("""async (a) => {
        const tok = await window.__c.o.getToken();
        const r = await fetch('/webman/3rdparty/NASsurfer/browser.cgi?a=api&op=' + a[0] + '&t=' + encodeURIComponent(tok),
            a[1] ? {method: 'POST', headers: {'Content-Type': 'application/json'}, body: JSON.stringify(a[1])} : {});
        return [r.status, await r.json()];
    }""", [op, body])


async def run(chrome, var_dir):
    async with async_playwright() as p:
        browser = await p.chromium.launch(executable_path=chrome, args=["--no-sandbox"])

        print("Grundfunktionen")
        ctx, pg = await open_page(browser)
        t0 = time.time()
        await nav(pg, DEV + "/kb/")
        st = await wait_state(pg, lambda s: s.get("title") == "KB" and s.get("f", 0) > 0)
        check("Seite lädt und Bild kommt an", st.get("title") == "KB", st)
        check("Laden in unter 10 s", time.time() - t0 < 10, f"{time.time() - t0:.1f}s")
        ox, oy = await canvas_origin(pg)

        await pg.mouse.click(ox + 50, oy + 25)
        await asyncio.sleep(0.3)
        await pg.keyboard.type("Grüße ä@€ 1+1")
        st = await wait_state(pg, lambda s: (s.get("title") or "").startswith("T:Grüße"))
        check("Tippen mit Umlauten, @ und €", st.get("title") == "T:Grüße ä@€ 1+1", st.get("title"))

        await pg.keyboard.press("Control+a")
        await pg.keyboard.press("Backspace")
        st = await wait_state(pg, lambda s: s.get("title") == "T:")
        check("Strg+A und Löschen", st.get("title") == "T:", st.get("title"))

        await pg.keyboard.insert_text("eingefügter Text")
        st = await wait_state(pg, lambda s: "eingef" in (s.get("title") or ""))
        check("Einfügen", st.get("title") == "T:eingefügter Text", st.get("title"))

        await pg.keyboard.press("Control+a")
        await asyncio.sleep(0.3)
        cp = await pg.evaluate("window.__c.api('copy').then(r => r.json())")
        check("Kopieren (Auswahl auslesen)", cp.get("text") == "eingefügter Text", cp)

        await pg.mouse.click(ox + 60, oy + 75)
        st = await wait_state(pg, lambda s: (s.get("title") or "").startswith("C:"))
        check("Bestätigungsdialog", st.get("title") == "C:true", st.get("title"))

        print("Tabs")
        await pg.mouse.click(ox + 160, oy + 75)
        st = await wait_state(pg, lambda s: s.get("title") == "Popup-Seite")
        check("Link mit neuem Fenster öffnet Tab", len(st.get("tabs", [])) == 2, st.get("tabs"))
        await pg.evaluate("u => window.__c.tabNew(u)", DEV + "/router/")
        st = await wait_state(pg, lambda s: len(s.get("tabs", [])) == 3 and s.get("title") == "Router")
        check("Neuer Tab mit Adresse", st.get("title") == "Router" and st["tabs"][-1].get("url", "").endswith("/router/"), st.get("tabs"))
        first = st["tabs"][0]["id"]
        await pg.evaluate("id => window.__c.tabSwitch(id)", first)
        st = await wait_state(pg, lambda s: s.get("title", "").startswith("C:"))
        check("Tab wechseln", st.get("title", "").startswith("C:"), st.get("title"))
        active = [t for t in st["tabs"] if t["active"]][0]["id"]
        await pg.evaluate("id => window.__c.tabClose(id)", active)
        st = await wait_state(pg, lambda s: len(s.get("tabs", [])) == 2)
        check("Tab schließen", len(st.get("tabs", [])) == 2, st.get("tabs"))

        print("Downloads und Uploads")
        await nav(pg, DEV + "/kb/")
        await wait_state(pg, lambda s: s.get("title") == "KB")
        await pg.evaluate("window.__ev = []")
        await pg.mouse.click(ox + 310, oy + 75)
        dl = []
        for _ in range(60):
            dl = [e for e in await pg.evaluate("window.__ev") if e["ev"] == "download"]
            if dl:
                break
            await asyncio.sleep(0.2)
        check("Download mit richtigem Namen", dl and dl[0]["name"] == "bericht ä.txt", dl)
        if dl:
            url = await pg.evaluate("id => window.__c.downloadUrl(id)", dl[0]["id"])
            r = await pg.evaluate("u => fetch(u).then(async r => [r.status, await r.text()])", url)
            check("Download-Inhalt", r == [200, "Inhalt der Datei"], r)
        r = await api(pg, "downloads")
        check("Download-Liste", r[0] == 200 and len(r[1].get("items", [])) >= 1, r)

        await pg.mouse.click(ox + 60, oy + 125)
        fe = []
        for _ in range(60):
            fe = [e for e in await pg.evaluate("window.__ev") if e["ev"] == "file"]
            if fe:
                break
            await asyncio.sleep(0.2)
        check("Datei-Auswahl wird gemeldet", bool(fe), fe)
        await pg.evaluate("window.__c.uploadFiles([new File(['hallo welt'], 'notiz.txt')])")
        st = await wait_state(pg, lambda s: (s.get("title") or "").startswith("F:"))
        check("Upload", st.get("title") == "F:notiz.txt:10", st.get("title"))

        print("Favoriten")
        r = await api(pg, "bookmarks", {"op": "add", "url": DEV + "/kb/", "title": "KB-Test"})
        check("Favorit anlegen", r[0] == 200 and r[1]["items"][0]["title"] == "KB-Test", r)
        r = await api(pg, "bookmarks")
        check("Favoriten abrufen", any(b["url"] == DEV + "/kb/" for b in r[1].get("items", [])), r)
        r = await api(pg, "bookmarks", {"op": "remove", "url": DEV + "/kb/"})
        check("Favorit entfernen", r[0] == 200 and not r[1]["items"], r)

        print("Paketquelle")
        src = await pg.evaluate("""() => fetch('/webman/3rdparty/NASsurfer/browser.cgi?a=source', {method: 'POST',
            headers: {'Content-Type': 'application/x-www-form-urlencoded'},
            body: 'language=ger&arch=geminilake&build=72806&major=7&minor=2'}).then(r => r.json())""")
        pk = (src.get("packages") or [{}])[0]
        check("Paketquelle liefert neuesten Release", pk.get("package") == "NASsurfer" and pk.get("version") == "9.9.9-0001"
              and pk.get("link", "").endswith("9.9.9-0001.spk") and pk.get("changelog") == "Test-Release", src)
        arm = await pg.evaluate("() => fetch('/webman/3rdparty/NASsurfer/browser.cgi?a=source&arch=rtd1296&build=72806').then(r => r.json())")
        check("Paketquelle: kein Paket für ARM-NAS", arm.get("packages") == [], arm)

        print("Passwörter")
        await nav(pg, DEV + "/pw/")
        await wait_state(pg, lambda s: s.get("title") == "PW")
        await pg.mouse.click(ox + 50, oy + 25)
        await pg.keyboard.type("tester")
        await pg.mouse.click(ox + 50, oy + 75)
        await pg.keyboard.type("GeheimesPasswort123")
        await pg.keyboard.press("Enter")
        st = await wait_state(pg, lambda s: s.get("title") == "PW-OK")
        check("Anmeldung mit Passwort", st.get("title") == "PW-OK", st.get("title"))
        check("Keine Verbindungsfehler im Fenster", not await pg.evaluate("window.__err"), await pg.evaluate("window.__err"))
        await ctx.close()

        print("Sprache und Zeitzone")
        for loc, tz, al in (("en-US", "America/New_York", "en-US"), ("de-DE", "Europe/Berlin", "de-DE")):
            ctx, pg = await open_page(browser, loc, tz)
            await asyncio.sleep(1.5)
            await nav(pg, DEV + "/lang/?" + loc)
            st = await wait_state(pg, lambda s: (s.get("url") or "").endswith(loc) and (s.get("title") or "").startswith("L:"))
            want = f"L:{loc}|{tz}|AL={al}"
            check(f"Sprache {loc}, Zeitzone {tz}", (st.get("title") or "").startswith(want), st.get("title"))
            await ctx.close()

        await browser.close()

    # Passwort darf nirgends im Profil stehen (Dienst vorher beenden, damit alles geschrieben ist)
    return var_dir


def password_leaked(var_dir):
    needle = b"GeheimesPasswort123"
    for root, _, files in os.walk(os.path.join(var_dir, "profiles")):
        for f in files:
            try:
                with open(os.path.join(root, f), "rb") as fh:
                    if needle in fh.read():
                        return os.path.join(root, f)
            except OSError:
                pass
    return None


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--chrome", default=shutil.which("google-chrome") or "/usr/bin/google-chrome")
    ap.add_argument("--var", default="/var/packages/NASsurfer/var")
    ap.add_argument("--after-stop", action="store_true", help="nur die Prüfung nach dem Stoppen ausführen")
    a = ap.parse_args()
    if a.after_stop:
        leak = password_leaked(a.var)
        check("Passwort nicht im Browser-Profil gespeichert", leak is None, leak)
    else:
        asyncio.run(run(a.chrome, a.var))
    failed = [r for r in results if not r[1]]
    print(f"\n{len(results) - len(failed)}/{len(results)} bestanden")
    sys.exit(1 if failed else 0)


if __name__ == "__main__":
    main()
