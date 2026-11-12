// SPDX-License-Identifier: AGPL-3.0-only
/* NASsurfer – Browser-Kern im DSM-Fenster.
 * Zeigt das Bild des Chromium auf der NAS und schickt Maus, Tastatur und
 * Zwischenablage zurück. Unabhängig von DSM/ExtJS, damit er sich testen lässt.
 *
 *   var c = new NASsurferClient({
 *     container: el, base: "/webman/3rdparty/NASsurfer/",
 *     getToken: function () { return Promise<string> },
 *     onState: function (st) {}, onEvent: function (ev) {}, onError: function (msg) {}
 *   });
 */
(function (global) {
    "use strict";

    var MOD_ALT = 1, MOD_CTRL = 2, MOD_META = 4, MOD_SHIFT = 8;
    var IS_MAC = /Mac|iPhone|iPad/.test(navigator.platform || "");

    function mods(e) {
        var m = 0;
        var altGr = e.getModifierState && e.getModifierState("AltGraph");
        if (e.altKey && !altGr) m |= MOD_ALT;
        if (e.ctrlKey && !altGr) m |= MOD_CTRL;
        if (e.shiftKey) m |= MOD_SHIFT;
        // Auf dem Mac ist Cmd das Strg der Linux-Seite
        if (e.metaKey) m |= IS_MAC ? MOD_CTRL : MOD_META;
        return m;
    }

    // Antwort des Dienstes: "NSF1" | Länge (uint32) | Zustand (JSON) | Bild (JPEG, optional)
    function parsePoll(buf) {
        var v = new DataView(buf);
        if (buf.byteLength < 8 || v.getUint32(0) !== 0x4E534631) return null;
        var n = v.getUint32(4);
        if (8 + n > buf.byteLength) return null;
        var st = JSON.parse(new TextDecoder().decode(new Uint8Array(buf, 8, n)));
        var img = buf.byteLength > 8 + n ? new Blob([new Uint8Array(buf, 8 + n)], { type: "image/jpeg" }) : null;
        return { st: st, img: img };
    }

    function sleep(ms) { return new Promise(function (r) { setTimeout(r, ms); }); }

    function NASsurferClient(o) {
        var me = this;
        me.o = o;
        me.base = o.base;
        me.f = 0;            // letztes Bild
        me.s = 0;            // letzter Zustand
        me.vw = 0; me.vh = 0;
        me.queue = [];
        me.sending = false;
        me.closed = false;
        me.pressed = {};
        me.lastUrl = "";
        me.composing = false;
        // Sprache und Zeitzone des eigenen Browsers an Chromium weitergeben
        var langs = (navigator.languages && navigator.languages.length ? navigator.languages : [navigator.language || "en-US"]).slice(0, 6);
        var tz = "";
        try { tz = Intl.DateTimeFormat().resolvedOptions().timeZone || ""; } catch (e) {}
        me.prefQuery = "&lang=" + encodeURIComponent(langs.join(",")) + "&tz=" + encodeURIComponent(tz);

        var root = me.root = document.createElement("div");
        root.style.cssText = "position:relative;width:100%;height:100%;overflow:hidden;background:#fff;outline:none";
        var canvas = me.canvas = document.createElement("canvas");
        canvas.style.cssText = "position:absolute;left:0;top:0;display:block;cursor:default";
        canvas.width = 1; canvas.height = 1;
        var ta = me.ta = document.createElement("textarea");
        ta.setAttribute("autocomplete", "off");
        ta.setAttribute("autocorrect", "off");
        ta.setAttribute("autocapitalize", "off");
        ta.setAttribute("spellcheck", "false");
        ta.setAttribute("aria-label", "Eingabe");
        ta.style.cssText = "position:absolute;left:0;top:0;width:2px;height:2px;opacity:0;border:0;padding:0;resize:none;pointer-events:none;z-index:-1";
        root.appendChild(canvas);
        root.appendChild(ta);
        o.container.appendChild(root);
        me.ctx = canvas.getContext("2d");

        me.bind();
        me.ro = new ResizeObserver(function () { me.scheduleResize(); });
        me.ro.observe(root);
        me.scheduleResize();
        me.loop();
    }

    var P = NASsurferClient.prototype;

    P.url = function (op, extra, tok) {
        return this.base + "browser.cgi?a=api&op=" + op + (extra || "") + "&t=" + encodeURIComponent(tok);
    };

    // ---- Bild und Zustand ---------------------------------------------------
    P.loop = async function () {
        var me = this, fails = 0;
        while (!me.closed) {
            try {
                var tok = await me.o.getToken();
                var r = await fetch(me.url("poll", "&f=" + me.f + "&s=" + me.s + me.prefQuery, tok), { credentials: "same-origin", cache: "no-store" });
                if (me.closed) return;
                if (r.status === 401 && me.o.onTokenInvalid) { me.o.onTokenInvalid(); await sleep(500); continue; }
                if (!r.ok) {
                    var j = null;
                    try { j = await r.json(); } catch (e) {}
                    throw new Error("HTTP " + r.status + (j && j.error ? " " + j.error + (j.detail ? ": " + j.detail : "") : ""));
                }
                var parsed = parsePoll(await r.arrayBuffer());
                if (!parsed) throw new Error("Antwort unlesbar");
                var st = parsed.st, blob = parsed.img;
                if (st) {
                    // Sitzung neu gestartet (z. B. nach Leerlauf): letzte Seite wieder öffnen
                    if (st.s < me.s || st.f < me.f) {
                        me.f = 0;
                        if (me.lastUrl && (!st.url || st.url === "about:blank")) me.nav(me.lastUrl);
                        me.sendResize(true);
                    }
                    me.f = st.f; me.s = st.s;
                }
                if (blob) await me.draw(blob);
                if (st) {
                    if (st.url && st.url !== "about:blank") me.lastUrl = st.url;
                    me.o.onState && me.o.onState(st);
                    (st.ev || []).forEach(function (ev) { me.o.onEvent && me.o.onEvent(ev); });
                }
                if (fails >= 3) { me.o.onError && me.o.onError(null); }
                fails = 0;
            } catch (e) {
                if (me.closed) return;
                fails++;
                // Einzelne Aussetzer (DSM-Webserver, Netz) nicht anzeigen, nur echte Störungen
                if (fails >= 3) me.o.onError && me.o.onError(e && e.message || String(e));
                await sleep(fails === 1 ? 150 : Math.min(8000, 500 * fails));
            }
        }
    };

    P.draw = async function (blob) {
        var bmp = await createImageBitmap(blob);
        var c = this.canvas;
        if (c.width !== bmp.width || c.height !== bmp.height) {
            c.width = bmp.width; c.height = bmp.height;
            c.style.width = bmp.width + "px"; c.style.height = bmp.height + "px";
        }
        this.vw = bmp.width; this.vh = bmp.height;
        this.ctx.drawImage(bmp, 0, 0);
        if (bmp.close) bmp.close();
    };

    // ---- Senden -------------------------------------------------------------
    P.send = function (ev, now) {
        var q = this.queue;
        // Mausbewegungen zusammenfassen: nur die letzte zählt
        if (ev.type === "mouse" && ev.t === "move" && q.length && q[q.length - 1].type === "mouse" && q[q.length - 1].t === "move") {
            q[q.length - 1] = ev;
        } else {
            q.push(ev);
        }
        var me = this;
        if (now) { me.flush(); return; }
        if (!me.flushTimer) me.flushTimer = setTimeout(function () { me.flushTimer = null; me.flush(); }, 25);
    };

    P.flush = async function () {
        var me = this;
        if (me.sending || !me.queue.length || me.closed) return;
        me.sending = true;
        var batch = me.queue;
        me.queue = [];
        try {
            var tok = await me.o.getToken();
            await fetch(me.url("input", "", tok), {
                method: "POST", credentials: "same-origin", cache: "no-store",
                headers: { "Content-Type": "application/json" }, body: JSON.stringify(batch)
            });
        } catch (e) {
            me.o.onError && me.o.onError(e && e.message || String(e));
        }
        me.sending = false;
        if (me.queue.length) me.flush();
    };

    P.api = async function (op, opts, extra) {
        var tok = await this.o.getToken();
        return fetch(this.url(op, extra || "", tok), Object.assign({ method: "POST", credentials: "same-origin", cache: "no-store" }, opts || {}));
    };

    P.scheduleResize = function () {
        var me = this;
        clearTimeout(me.resizeTimer);
        me.resizeTimer = setTimeout(function () { me.sendResize(false); }, 200);
    };

    P.sendResize = function (force) {
        var w = Math.floor(this.root.clientWidth), h = Math.floor(this.root.clientHeight);
        if (w < 50 || h < 50) return;
        if (!force && w === this.sentW && h === this.sentH) return;
        this.sentW = w; this.sentH = h;
        this.send({ type: "resize", w: w, h: h }, true);
    };

    // ---- Maus ---------------------------------------------------------------
    P.pos = function (e) {
        var r = this.canvas.getBoundingClientRect();
        return { x: Math.round(e.clientX - r.left), y: Math.round(e.clientY - r.top) };
    };

    P.bind = function () {
        var me = this, c = me.canvas, ta = me.ta;
        var btnName = ["left", "middle", "right", "back", "forward"];

        me.h = {
            down: function (e) {
                e.preventDefault();
                ta.focus({ preventScroll: true });
                me.mouseDown = true;
                var p = me.pos(e);
                me.send({ type: "mouse", t: "down", x: p.x, y: p.y, btn: btnName[e.button] || "left", btns: e.buttons, n: e.detail || 1, mods: mods(e) }, true);
            },
            up: function (e) {
                var p = me.pos(e);
                me.send({ type: "mouse", t: "up", x: p.x, y: p.y, btn: btnName[e.button] || "left", btns: e.buttons, n: e.detail || 1, mods: mods(e) }, true);
            },
            move: function (e) {
                var p = me.pos(e);
                me.send({ type: "mouse", t: "move", x: p.x, y: p.y, btns: e.buttons, btn: e.buttons & 1 ? "left" : e.buttons & 2 ? "right" : e.buttons & 4 ? "middle" : "", mods: mods(e) });
            },
            wheel: function (e) {
                e.preventDefault();
                var p = me.pos(e), k = e.deltaMode === 1 ? 40 : e.deltaMode === 2 ? 800 : 1;
                me.send({ type: "wheel", x: p.x, y: p.y, dx: e.deltaX * k, dy: e.deltaY * k, mods: mods(e) });
            },
            ctx: function (e) { e.preventDefault(); },
            keydown: function (e) { me.keydown(e); },
            keyup: function (e) { me.keyup(e); },
            input: function () { if (!me.composing) me.takeText(); },
            compStart: function () { me.composing = true; },
            compEnd: function () { me.composing = false; me.takeText(); },
            paste: function (e) {
                e.preventDefault();
                var t = e.clipboardData && e.clipboardData.getData("text/plain");
                if (t) me.send({ type: "text", text: t }, true);
            }
        };
        // Pointer statt Maus: liefert auch Bewegungen außerhalb, solange gedrückt
        c.addEventListener("mousedown", me.h.down);
        c.addEventListener("mousemove", me.h.move);
        window.addEventListener("mouseup", me.h.upGuard = function (e) { if (me.mouseDown) { me.mouseDown = !!e.buttons; me.h.up(e); } });
        c.addEventListener("wheel", me.h.wheel, { passive: false });
        c.addEventListener("contextmenu", me.h.ctx);
        ta.addEventListener("keydown", me.h.keydown);
        ta.addEventListener("keyup", me.h.keyup);
        ta.addEventListener("input", me.h.input);
        ta.addEventListener("compositionstart", me.h.compStart);
        ta.addEventListener("compositionend", me.h.compEnd);
        ta.addEventListener("paste", me.h.paste);
    };

    P.takeText = function () {
        var t = this.ta.value;
        this.ta.value = "";
        if (t) this.send({ type: "text", text: t }, true);
    };

    // ---- Tastatur -----------------------------------------------------------
    P.keydown = function (e) {
        var me = this;
        if (e.isComposing || e.key === "Dead" || e.key === "Process" || e.keyCode === 229) return; // Eingabehilfen/Akzente: Text kommt über "input"
        var m = mods(e);
        var shortcut = (m & (MOD_CTRL | MOD_META)) !== 0;
        var k = (e.key || "").toLowerCase();
        if (shortcut && k === "v") return; // Einfügen läuft über das paste-Ereignis
        if (e.key && e.key.length === 1 && !shortcut && !(m & MOD_ALT)) return; // Zeichen kommen über "input"
        e.preventDefault();
        var ev = { type: "key", t: "down", key: e.key, code: e.code, kc: e.keyCode, mods: m };
        if (e.key === "Enter") ev.text = "\r";
        me.pressed[e.code] = { key: e.key, kc: e.keyCode };
        me.send(ev, true);
        if (shortcut && (k === "c" || k === "x")) me.copyToClipboard();
    };

    P.keyup = function (e) {
        var p = this.pressed[e.code];
        if (!p) return;
        delete this.pressed[e.code];
        e.preventDefault();
        this.send({ type: "key", t: "up", key: p.key, code: e.code, kc: p.kc, mods: mods(e) }, true);
    };

    P.copyToClipboard = async function () {
        try {
            await sleep(60);
            var r = await this.api("copy");
            var j = await r.json();
            if (!j.text) return;
            if (navigator.clipboard && window.isSecureContext) {
                await navigator.clipboard.writeText(j.text);
            } else {
                var t = document.createElement("textarea");
                t.value = j.text; t.style.cssText = "position:fixed;opacity:0";
                document.body.appendChild(t); t.select();
                document.execCommand("copy");
                document.body.removeChild(t);
                this.ta.focus({ preventScroll: true });
            }
        } catch (e) {}
    };

    // ---- Steuerung ----------------------------------------------------------
    P.nav = function (url) { this.lastUrl = url; this.send({ type: "nav", url: url }, true); this.ta.focus({ preventScroll: true }); };
    P.back = function () { this.send({ type: "back" }, true); };
    P.forward = function () { this.send({ type: "forward" }, true); };
    P.reload = function () { this.send({ type: "reload" }, true); };
    P.stop = function () { this.send({ type: "stop" }, true); };
    P.focus = function () { this.ta.focus({ preventScroll: true }); };
    P.answerDialog = function (ok, text) { this.send({ type: "dialog", ok: !!ok, text: text || "" }, true); };

    P.uploadFiles = async function (files) {
        var ids = [];
        for (var i = 0; i < files.length; i++) {
            var r = await this.api("upload", { body: files[i], headers: { "Content-Type": "application/octet-stream" } }, "&name=" + encodeURIComponent(files[i].name));
            var j = await r.json();
            if (j.success) ids.push(j.id);
        }
        this.send({ type: "files", ids: ids }, true);
    };

    P.downloadUrl = async function (id) {
        var tok = await this.o.getToken();
        return this.base + "browser.cgi?a=api&op=download&id=" + encodeURIComponent(id) + "&t=" + encodeURIComponent(tok);
    };

    P.tabNew = function (url) { this.send({ type: "tab_new", url: url || "" }, true); };
    P.tabSwitch = function (id) { this.send({ type: "tab_switch", id: id }, true); this.focus(); };
    P.tabClose = function (id) { this.send({ type: "tab_close", id: id }, true); };

    P.reset = async function () { this.lastUrl = ""; await this.api("reset"); this.f = 0; this.s = 0; };

    P.destroy = function () {
        var me = this, c = me.canvas, ta = me.ta;
        me.closed = true;
        me.ro.disconnect();
        window.removeEventListener("mouseup", me.h.upGuard);
        c.removeEventListener("mousedown", me.h.down);
        c.removeEventListener("mousemove", me.h.move);
        c.removeEventListener("wheel", me.h.wheel);
        c.removeEventListener("contextmenu", me.h.ctx);
        if (me.root.parentNode) me.root.parentNode.removeChild(me.root);
    };

    global.NASsurferClient = NASsurferClient;
})(window);
