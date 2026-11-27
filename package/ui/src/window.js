// SPDX-License-Identifier: AGPL-3.0-only
/* NASsurfer – DSM-Fenster (Adressleiste, Dialoge) um den Browser-Kern. */
Ext.ns("SYNO.SDS.NASsurfer");

(function () {
    var BASE = "/webman/3rdparty/NASsurfer/";
    var RECENT_KEY = "NASsurfer.recent";

    var L = {
        ger: {
            back: "Zurück", forward: "Vor", reload: "Neu laden", stop: "Laden abbrechen", home: "Startseite",
            go: "Öffnen", address: "Adresse eingeben, z. B. 192.168.1.1 oder https://pve.lan:8006",
            more: "Mehr", reset: "Anmeldungen löschen", quit: "Browser beenden (Speicher freigeben)",
            resetq: "Alle Cookies und Anmeldungen dieses Browsers löschen?", resetdone: "Gelöscht.",
            error: "NASsurfer", starting: "Browser startet …", loading: "Lädt …", offline: "Verbindung gestört",
            dlrun: "Download läuft: ", dldone: "Download fertig", save: "Speichern",
            upload: "Datei hochladen", uploadmsg: "Die Seite möchte eine Datei von deinem Rechner.", choose: "Dateien wählen …",
            uploading: "Lade hoch …", crashed: "Die Seite ist abgestürzt und wird neu geladen.",
            e_login: "Deine DSM-Anmeldung ist abgelaufen. Bitte melde dich neu an.",
            e_forbidden: "NASsurfer dürfen nur Administratoren benutzen.",
            e_state: "NASsurfer konnte keinen privaten Arbeitsordner anlegen.",
            e_stopped: "NASsurfer ist im Paket-Zentrum gestoppt.",
            e_net: "Keine Antwort von der NAS.", e_url: "Diese Adresse ist ungültig. Erlaubt sind http:// und https://.",
            fav: "Favoriten", favadd: "Zu Favoriten hinzufügen", favdel: "Aus Favoriten entfernen", favnone: "Noch keine Favoriten – mit ☆ hinzufügen.",
            dls: "Downloads", dlnone: "Noch keine Downloads in dieser Sitzung.", newtab: "Neuer Tab", closetab: "Tab schließen",
            tablimit: "Mehr als {0} Tabs gehen nicht. Schließe einen Tab.",
            upd: "Automatische Updates einrichten",
            updtext: "So bekommt NASsurfer Updates direkt aus GitHub über das Paket-Zentrum:<ol style=\"margin:8px 0 8px 18px;padding:0\"><li><b>Paket-Zentrum → Einstellungen → Paketquellen → Hinzufügen</b></li><li>Name: <b>NASsurfer</b>, Ort: eine der Adressen unten (zuerst die erste probieren)</li><li><b>Einstellungen → Automatische Updates:</b> NASsurfer einschließen</li></ol>"
        },
        enu: {
            back: "Back", forward: "Forward", reload: "Reload", stop: "Stop", home: "Start page",
            go: "Go", address: "Enter an address, e.g. 192.168.1.1 or https://pve.lan:8006",
            more: "More", reset: "Delete sign-ins", quit: "Quit browser (free memory)",
            resetq: "Delete all cookies and sign-ins of this browser?", resetdone: "Deleted.",
            error: "NASsurfer", starting: "Starting browser …", loading: "Loading …", offline: "Connection problem",
            dlrun: "Downloading: ", dldone: "Download finished", save: "Save",
            upload: "Upload file", uploadmsg: "The page wants a file from your computer.", choose: "Choose files …",
            uploading: "Uploading …", crashed: "The page crashed and is being reloaded.",
            e_login: "Your DSM session has expired. Please sign in again.",
            e_forbidden: "Only administrators may use NASsurfer.",
            e_state: "NASsurfer could not create its private working folder.",
            e_stopped: "NASsurfer is stopped in Package Center.",
            e_net: "The NAS did not respond.", e_url: "This address is not valid. Only http:// and https:// are allowed.",
            fav: "Favorites", favadd: "Add to favorites", favdel: "Remove from favorites", favnone: "No favorites yet – add one with ☆.",
            dls: "Downloads", dlnone: "No downloads in this session yet.", newtab: "New tab", closetab: "Close tab",
            tablimit: "No more than {0} tabs. Please close a tab.",
            upd: "Set up automatic updates",
            updtext: "This way NASsurfer gets updates from GitHub through Package Center:<ol style=\"margin:8px 0 8px 18px;padding:0\"><li><b>Package Center → Settings → Package Sources → Add</b></li><li>Name: <b>NASsurfer</b>, Location: one of the addresses below (try the first one first)</li><li><b>Settings → Auto-update:</b> include NASsurfer</li></ol>"
        }
    };
    var TABBAR_H = 30;

    // Tab-Leiste im DSM-Stil (einmal pro Seite eingefügt)
    function injectStyles() {
        if (document.getElementById("nas-styles")) { return; }
        var st = document.createElement("style");
        st.id = "nas-styles";
        st.textContent =
            ".nas-tabbar{position:absolute;left:0;right:0;top:0;height:" + TABBAR_H + "px;display:flex;align-items:flex-end;gap:2px;padding:0 6px;background:#eef1f5;border-bottom:1px solid #d7dde5;overflow:hidden;box-sizing:border-box;font:12px/1 -apple-system,'Segoe UI',Roboto,sans-serif}" +
            ".nas-tab{display:flex;align-items:center;gap:6px;max-width:200px;min-width:60px;height:25px;padding:0 6px 0 10px;background:#dde3ea;color:#3a4553;border-radius:6px 6px 0 0;cursor:pointer;user-select:none}" +
            ".nas-tab:hover{background:#e7ecf2}" +
            ".nas-tab-active{background:#fff;color:#1f2a37;box-shadow:0 -1px 0 #d7dde5 inset}" +
            ".nas-tab-label{flex:1;overflow:hidden;white-space:nowrap;text-overflow:ellipsis}" +
            ".nas-tab-x{opacity:.55;padding:2px 3px;border-radius:3px}" +
            ".nas-tab-x:hover{opacity:1;background:rgba(0,0,0,.08)}" +
            ".nas-tab-plus{height:25px;line-height:25px;padding:0 10px;font-size:15px;color:#3a4553;cursor:pointer;border-radius:6px 6px 0 0;user-select:none}" +
            ".nas-tab-plus:hover{background:#dde3ea}";
        document.head.appendChild(st);
    }

    function lang() { return (window._S && _S("lang")) || "enu"; }
    function T(k) {
        var d = L[lang()] || (/^(ger|deu|de)/.test(lang()) ? L.ger : L.enu);
        return d[k] || L.enu[k] || k;
    }
    function esc(s) { return Ext.util.Format.htmlEncode(String(s)); }

    // Früher gab es eine Liste "Zuletzt geöffnet" im Browser des Benutzers.
    // NASsurfer merkt sich keinen Verlauf mehr: alte Einträge entfernen.
    try { localStorage.removeItem(RECENT_KEY); } catch (e) {}

    function normalize(input) {
        var s = String(input || "").trim();
        if (!s) return null;
        if (!/^[a-z][a-z0-9+.\-]*:\/\//i.test(s)) s = "http://" + s;
        try {
            var u = new URL(s);
            if ((u.protocol !== "http:" && u.protocol !== "https:") || !u.hostname || u.username) return null;
            return u.href;
        } catch (e) { return null; }
    }

    Ext.define("SYNO.SDS.NASsurfer.Instance", {
        extend: "SYNO.SDS.AppInstance",
        appWindowName: "SYNO.SDS.NASsurfer.Window",
        constructor: function () { this.callParent(arguments); }
    });

    Ext.define("SYNO.SDS.NASsurfer.Window", {
        extend: "SYNO.SDS.AppWindow",

        constructor: function (config) {
            var me = this;
            me.token = null;
            me.tokenExp = 0;
            me.tokenWaiters = null;

            me.urlField = new SYNO.ux.TextField({
                width: 420, emptyText: T("address"), enableKeyEvents: true, selectOnFocus: true,
                listeners: { specialkey: function (f, e) { if (e.getKey() === e.ENTER) { me.navigate(f.getValue()); } } }
            });
            me.btnBack = Ext.ComponentMgr.create({ xtype: "syno_button",  text: "‹", tooltip: T("back"), disabled: true, handler: function () { me.client && me.client.back(); } });
            me.btnFwd = Ext.ComponentMgr.create({ xtype: "syno_button",  text: "›", tooltip: T("forward"), disabled: true, handler: function () { me.client && me.client.forward(); } });
            me.btnReload = Ext.ComponentMgr.create({ xtype: "syno_button",  text: "↻", tooltip: T("reload"), handler: function () {
                if (!me.client) { return; }
                if (me.loading) { me.client.stop(); } else { me.client.reload(); }
            } });
            me.status = Ext.ComponentMgr.create({ xtype: "tbtext", text: "" });
            me.favs = [];
            me.btnStar = Ext.ComponentMgr.create({ xtype: "syno_button", text: "☆", tooltip: T("favadd"), handler: function () { me.toggleFav(); } });
            me.favMenu = new Ext.menu.Menu({ listeners: { beforeshow: function (m) { me.fillFavMenu(m); } } });
            me.dlMenu = new Ext.menu.Menu({ listeners: { beforeshow: function (m) { me.fillDlMenu(m); } } });

            config = Ext.apply({
                title: "NASsurfer",
                width: 1180, height: 760, minWidth: 560, minHeight: 380,
                layout: "fit",
                tbar: [
                    me.btnBack, me.btnFwd, me.btnReload,
                    { xtype: "syno_button", text: "⌂", tooltip: T("home"), handler: function () { me.showStart(true); } },
                    " ", me.urlField, " ",
                    { xtype: "syno_button", btnStyle: "blue", text: T("go"), handler: function () { me.navigate(me.urlField.getValue()); } },
                    me.btnStar,
                    " ", me.status, "->",
                    { xtype: "syno_button", text: "★ " + T("fav"), menu: me.favMenu },
                    { xtype: "syno_button", text: "⬇", tooltip: T("dls"), menu: me.dlMenu },
                    {
                        xtype: "syno_button", text: "⋯", tooltip: T("more"),
                        menu: { items: [
                            { text: T("upd"), handler: function () { me.showUpdateHelp(); } },
                            { text: T("reset"), handler: function () { me.resetBrowser(); } },
                            { text: T("quit"), handler: function () { me.quitBrowser(); } }
                        ] }
                    }
                ],
                html: "",
                listeners: {
                    afterrender: function () { me.setup(); },
                    resize: function (w, width) { me.fitAddressBar(width); },
                    destroy: function () { me.teardown(); }
                }
            }, config);
            me.callParent([config]);
        },

        setup: function () {
            var me = this, body = me.body.dom;
            body.style.position = "relative";
            injectStyles();
            me.tabbar = document.createElement("div");
            me.tabbar.className = "nas-tabbar";
            body.appendChild(me.tabbar);
            me.host = document.createElement("div");
            me.host.style.cssText = "position:absolute;left:0;right:0;bottom:0;top:" + TABBAR_H + "px";
            body.appendChild(me.host);
            me.renderTabs([]);

            me.start = document.createElement("iframe");
            me.start.setAttribute("sandbox", "allow-scripts");
            me.start.style.cssText = "position:absolute;left:0;right:0;bottom:0;top:" + TABBAR_H + "px;width:100%;height:calc(100% - " + TABBAR_H + "px);border:0;background:#f5f7fa;z-index:2";
            body.appendChild(me.start);

            me.onMsg = function (e) {
                if (e.source !== me.start.contentWindow || !e.data || e.data.__bs !== 1) { return; }
                if (e.data.type === "go" && typeof e.data.url === "string") { me.navigate(e.data.url); }
                if (e.data.type === "unfav" && typeof e.data.url === "string") { me.removeFav(e.data.url); }
            };
            window.addEventListener("message", me.onMsg);
            me.fitAddressBar(me.getWidth());
            me.showStart(false);
            me.loadFavs();
        },

        teardown: function () {
            if (this.onMsg) { window.removeEventListener("message", this.onMsg); }
            if (this.client) { this.client.destroy(); this.client = null; }
        },

        fitAddressBar: function (width) {
            if (this.urlField && width) { this.urlField.setWidth(Math.max(160, width - 600)); }
        },

        showStart: function (keepBrowser) {
            var data = { lang: lang(), favs: this.favs || [] };
            this.start.src = BASE + "start.html#" + encodeURIComponent(JSON.stringify(data));
            this.start.style.display = "block";
            if (!keepBrowser) { this.urlField.setValue(""); }
        },

        hideStart: function () { this.start.style.display = "none"; },
        startVisible: function () { return this.start.style.display !== "none"; },

        // Der Browser auf der NAS startet erst, wenn er gebraucht wird
        ensureClient: function () {
            var me = this;
            if (me.client) { return me.client; }
            me.setStatus(T("starting"));
            me.client = new NASsurferClient({
                container: me.host,
                base: BASE,
                getToken: function () { return me.getToken(); },
                onTokenInvalid: function () { me.token = null; },
                onState: function (st) { me.onState(st); },
                onEvent: function (ev) { me.onEvent(ev); },
                onError: function (msg) { me.setStatus(msg ? T("offline") + " (" + msg + ")" : ""); }
            });
            return me.client;
        },

        setStatus: function (text) { this.status.setText(text ? esc(text) : ""); },

        onState: function (st) {
            this.loading = !!st.loading;
            this.btnBack.setDisabled(!st.back);
            this.btnFwd.setDisabled(!st.fwd);
            this.btnReload.setText(this.loading ? "✕" : "↻");
            this.btnReload.setTooltip(this.loading ? T("stop") : T("reload"));
            if (this.status.text === esc(T("starting")) || !this.status.text || this.status.text === esc(T("loading"))) {
                this.setStatus(this.loading ? T("loading") : "");
            }
            if (st.url && st.url !== "about:blank" && !this.urlField.hasFocus && !this.startVisible()) { this.urlField.setValue(st.url); }
            this.currentUrl = st.url && st.url !== "about:blank" ? st.url : "";
            this.currentTitle = st.title || "";
            if (Array.isArray(st.tabs)) { this.renderTabs(st.tabs); }
            this.updateStar();
            this.setTitle("NASsurfer" + (st.title && st.title !== st.url ? " – " + String(st.title).slice(0, 80) : ""));
        },

        onEvent: function (ev) {
            var me = this, mb = me.getMsgBox();
            switch (ev.ev) {
            case "dialog":
                if (ev.kind === "alert") {
                    mb.alert(T("error"), esc(ev.message), function () { me.client.answerDialog(true); });
                } else if (ev.kind === "prompt" && mb.prompt) {
                    mb.prompt(T("error"), esc(ev.message), function (btn, text) {
                        me.client.answerDialog(btn === "ok" || btn === "yes", text);
                    }, me, false, ev.def || "");
                } else if (ev.kind === "prompt") {
                    var v = window.prompt(ev.message, ev.def || "");
                    me.client.answerDialog(v !== null, v || "");
                } else {
                    mb.confirm(T("error"), esc(ev.message), function (btn) {
                        me.client.answerDialog(btn === "yes" || btn === "ok");
                    });
                }
                break;
            case "file":
                me.askFiles(!!ev.multiple);
                break;
            case "dlstart":
                me.setStatus(T("dlrun") + ev.name);
                break;
            case "download":
                me.setStatus("");
                mb.confirm(T("dldone"), esc(ev.name) + "<br><br>" + esc(T("save")) + "?", function (btn) {
                    if (btn === "yes" || btn === "ok") { me.saveDownload(ev.id, ev.name); }
                });
                break;
            case "crashed":
                me.setStatus(T("crashed"));
                break;
            case "tablimit":
                mb.alert(T("error"), esc(T("tablimit").replace("{0}", ev.max)));
                break;
            }
        },

        // ---- Tabs ----------------------------------------------------------
        renderTabs: function (tabs) {
            var me = this, bar = me.tabbar;
            bar.textContent = "";
            tabs.forEach(function (t) {
                var el = document.createElement("div");
                el.className = "nas-tab" + (t.active ? " nas-tab-active" : "");
                el.title = t.url || "";
                var label = document.createElement("span");
                label.className = "nas-tab-label";
                label.textContent = t.title && t.title !== t.url ? t.title : (t.url && t.url !== "about:blank" ? t.url.replace(/^https?:\/\//, "") : T("newtab"));
                var x = document.createElement("span");
                x.className = "nas-tab-x";
                x.textContent = "✕";
                x.title = T("closetab");
                x.addEventListener("click", function (e) { e.stopPropagation(); me.client && me.client.tabClose(t.id); });
                el.addEventListener("click", function () {
                    if (!me.client) { return; }
                    me.hideStart();
                    if (!t.active) { me.client.tabSwitch(t.id); }
                });
                el.addEventListener("auxclick", function (e) { if (e.button === 1 && me.client) { me.client.tabClose(t.id); } });
                el.appendChild(label);
                el.appendChild(x);
                bar.appendChild(el);
            });
            var plus = document.createElement("div");
            plus.className = "nas-tab-plus";
            plus.textContent = "+";
            plus.title = T("newtab");
            plus.addEventListener("click", function () {
                if (me.client) { me.client.tabNew(""); }
                me.showStart(false);
                me.urlField.focus();
            });
            bar.appendChild(plus);
        },

        // ---- Favoriten (auf der NAS gespeichert) ---------------------------
        api: function (op, method, body) {
            return this.getToken().then(function (tok) {
                return fetch(BASE + "browser.cgi?a=api&op=" + op + "&t=" + encodeURIComponent(tok), {
                    method: method || "GET", credentials: "same-origin", cache: "no-store",
                    headers: body ? { "Content-Type": "application/json" } : {},
                    body: body ? JSON.stringify(body) : undefined
                }).then(function (r) { return r.json(); });
            });
        },

        loadFavs: function () {
            var me = this;
            me.api("bookmarks").then(function (j) {
                if (j && j.success) { me.setFavs(j.items); }
            }, function () {});
        },

        setFavs: function (items) {
            this.favs = Array.isArray(items) ? items : [];
            this.updateStar();
            if (this.startVisible()) { this.showStart(true); }
        },

        isFav: function (url) {
            return !!url && (this.favs || []).some(function (f) { return f.url === url; });
        },

        updateStar: function () {
            var on = this.isFav(this.currentUrl);
            this.btnStar.setText(on ? "★" : "☆");
            this.btnStar.setTooltip(on ? T("favdel") : T("favadd"));
            this.btnStar.setDisabled(!this.currentUrl || this.startVisible());
        },

        toggleFav: function () {
            var me = this, url = me.currentUrl;
            if (!url) { return; }
            me.api("bookmarks", "POST", { op: me.isFav(url) ? "remove" : "add", url: url, title: me.currentTitle })
                .then(function (j) { if (j && j.success) { me.setFavs(j.items); } }, function () {});
        },

        removeFav: function (url) {
            var me = this;
            me.api("bookmarks", "POST", { op: "remove", url: url })
                .then(function (j) { if (j && j.success) { me.setFavs(j.items); } }, function () {});
        },

        fillFavMenu: function (m) {
            var me = this;
            m.removeAll();
            if (!me.favs.length) {
                m.add({ text: esc(T("favnone")), disabled: true });
                return;
            }
            me.favs.forEach(function (f) {
                m.add({ text: esc(f.title || f.url), tooltip: f.url, handler: function () { me.navigate(f.url); } });
            });
        },

        // ---- Downloads -----------------------------------------------------
        fillDlMenu: function (m) {
            var me = this;
            m.removeAll();
            m.add({ text: "…", disabled: true });
            me.api("downloads").then(function (j) {
                m.removeAll();
                var items = j && j.items || [];
                if (!items.length) { m.add({ text: esc(T("dlnone")), disabled: true }); return; }
                items.forEach(function (d) {
                    m.add({ text: "⬇ " + esc(d.name), handler: function () { me.saveDownload(d.id, d.name); } });
                });
            }, function () { m.removeAll(); m.add({ text: esc(T("e_net")), disabled: true }); });
        },

        saveDownload: function (id, name) {
            this.getToken().then(function (tok) {
                var a = document.createElement("a");
                a.href = BASE + "browser.cgi?a=api&op=download&id=" + encodeURIComponent(id) + "&t=" + encodeURIComponent(tok);
                a.download = name || "download";
                a.style.display = "none";
                document.body.appendChild(a); a.click(); document.body.removeChild(a);
            }, function () {});
        },

        // Datei-Auswahl braucht einen echten Klick: deshalb über eine Rückfrage
        askFiles: function (multiple) {
            var me = this;
            me.getMsgBox().confirm(T("upload"), esc(T("uploadmsg")) + "<br><br>" + esc(T("choose")), function (btn) {
                if (btn !== "yes" && btn !== "ok") { return; }
                var inp = document.createElement("input");
                inp.type = "file";
                inp.multiple = multiple;
                inp.style.display = "none";
                inp.addEventListener("change", function () {
                    if (inp.files && inp.files.length) {
                        me.setStatus(T("uploading"));
                        me.client.uploadFiles(inp.files).then(function () { me.setStatus(""); });
                    }
                    document.body.removeChild(inp);
                });
                document.body.appendChild(inp);
                inp.click();
            });
        },

        navigate: function (input) {
            var url = normalize(input);
            if (!url) { this.getMsgBox().alert(T("error"), esc(T("e_url"))); return; }
            var me = this;
            me.getToken().then(function () {
                me.urlField.setValue(url);
                me.hideStart();
                me.ensureClient().nav(url);
            }, function () {});
        },

        // Adressen für die Paketquelle: so, wie DSM gerade erreicht wird, und lokal
        showUpdateHelp: function () {
            var path = BASE + "browser.cgi?a=source";
            var urls = [window.location.origin + path];
            var local = "http://127.0.0.1:5000" + path;
            if (urls.indexOf(local) < 0) { urls.push(local); }
            var html = T("updtext") + urls.map(function (u) {
                return "<input type=\"text\" readonly value=\"" + esc(u) + "\" onclick=\"this.select()\" " +
                    "style=\"width:100%;box-sizing:border-box;margin:4px 0;padding:5px;font:12px monospace\">";
            }).join("");
            this.getMsgBox().alert(T("upd"), html);
        },

        resetBrowser: function () {
            var me = this;
            me.getMsgBox().confirm(T("error"), esc(T("resetq")), function (btn) {
                if (btn !== "yes" && btn !== "ok") { return; }
                var c = me.ensureClient();
                c.reset().then(function () { me.setStatus(T("resetdone")); me.showStart(false); });
            });
        },

        quitBrowser: function () {
            var me = this;
            if (!me.client) { return; }
            me.client.api("stop").then(function () {
                me.client.destroy();
                me.client = null;
                me.showStart(false);
                me.setStatus("");
            });
        },

        // ---- Anmeldung bei der NAS ------------------------------------------
        // Natives fetch per GET: DSMs Ajax-Schicht und POST-Inhalte haben hier
        // früher zu Verbindungsabbrüchen geführt.
        getToken: function () {
            var me = this;
            if (me.token && Date.now() < me.tokenExp - 10 * 60 * 1000) { return Promise.resolve(me.token); }
            if (me.tokenWaiters) { return me.tokenWaiters; }
            var syno = (window._S && _S("SynoToken")) || "";
            me.tokenWaiters = fetch(BASE + "browser.cgi?a=token&SynoToken=" + encodeURIComponent(syno), {
                method: "GET", credentials: "same-origin", cache: "no-store", headers: { "X-SYNO-TOKEN": syno }
            }).then(function (r) {
                return r.text().then(function (t) {
                    var j = null;
                    try { j = JSON.parse(t); } catch (e) {}
                    if (j && j.success && j.token) {
                        me.token = j.token;
                        me.tokenExp = Date.now() + (j.ttl || 3600) * 1000;
                        return j.token;
                    }
                    var msg = T("e_net");
                    if (r.status === 503) { msg = T("e_stopped"); }
                    else if (j && j.error === "login") { msg = T("e_login"); }
                    else if (j && j.error === "forbidden") { msg = T("e_forbidden"); }
                    else if (j && j.error === "state") { msg = T("e_state"); }
                    else { msg += " (HTTP " + r.status + ")"; }
                    throw new Error(msg);
                });
            }).then(function (tok) { me.tokenWaiters = null; return tok; }, function (e) {
                me.tokenWaiters = null;
                var msg = e && e.message || T("e_net");
                if (!me.tokenAlert) {
                    me.tokenAlert = true;
                    me.getMsgBox().alert(T("error"), esc(msg), function () { me.tokenAlert = false; });
                }
                throw e;
            });
            return me.tokenWaiters;
        }
    });
})();
