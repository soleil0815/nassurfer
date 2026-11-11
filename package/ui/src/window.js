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
            more: "Mehr", reset: "Anmeldungen und Verlauf löschen", quit: "Browser beenden (Speicher freigeben)",
            resetq: "Alle Cookies, Anmeldungen und den Verlauf dieses Browsers löschen?", resetdone: "Gelöscht.",
            error: "NASsurfer", starting: "Browser startet …", loading: "Lädt …", offline: "Verbindung gestört",
            dlrun: "Download läuft: ", dldone: "Download fertig", save: "Speichern",
            upload: "Datei hochladen", uploadmsg: "Die Seite möchte eine Datei von deinem Rechner.", choose: "Dateien wählen …",
            uploading: "Lade hoch …", crashed: "Die Seite ist abgestürzt und wird neu geladen.",
            e_login: "Deine DSM-Anmeldung ist abgelaufen. Bitte melde dich neu an.",
            e_forbidden: "NASsurfer dürfen nur Administratoren benutzen.",
            e_state: "NASsurfer konnte keinen privaten Arbeitsordner anlegen.",
            e_stopped: "NASsurfer ist im Paket-Zentrum gestoppt.",
            e_net: "Keine Antwort von der NAS.", e_url: "Diese Adresse ist ungültig. Erlaubt sind http:// und https://."
        },
        enu: {
            back: "Back", forward: "Forward", reload: "Reload", stop: "Stop", home: "Start page",
            go: "Go", address: "Enter an address, e.g. 192.168.1.1 or https://pve.lan:8006",
            more: "More", reset: "Delete sign-ins and history", quit: "Quit browser (free memory)",
            resetq: "Delete all cookies, sign-ins and history of this browser?", resetdone: "Deleted.",
            error: "NASsurfer", starting: "Starting browser …", loading: "Loading …", offline: "Connection problem",
            dlrun: "Downloading: ", dldone: "Download finished", save: "Save",
            upload: "Upload file", uploadmsg: "The page wants a file from your computer.", choose: "Choose files …",
            uploading: "Uploading …", crashed: "The page crashed and is being reloaded.",
            e_login: "Your DSM session has expired. Please sign in again.",
            e_forbidden: "Only administrators may use NASsurfer.",
            e_state: "NASsurfer could not create its private working folder.",
            e_stopped: "NASsurfer is stopped in Package Center.",
            e_net: "The NAS did not respond.", e_url: "This address is not valid. Only http:// and https:// are allowed."
        }
    };
    function lang() { return (window._S && _S("lang")) || "enu"; }
    function T(k) {
        var d = L[lang()] || (/^(ger|deu|de)/.test(lang()) ? L.ger : L.enu);
        return d[k] || L.enu[k] || k;
    }
    function esc(s) { return Ext.util.Format.htmlEncode(String(s)); }

    function loadRecent() {
        try { return JSON.parse(localStorage.getItem(RECENT_KEY) || "[]").slice(0, 12); } catch (e) { return []; }
    }
    function saveRecent(url) {
        try {
            var r = loadRecent().filter(function (u) { return u !== url; });
            r.unshift(url);
            localStorage.setItem(RECENT_KEY, JSON.stringify(r.slice(0, 12)));
        } catch (e) {}
    }

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

            config = Ext.apply({
                title: "NASsurfer",
                width: 1180, height: 760, minWidth: 560, minHeight: 380,
                layout: "fit",
                tbar: [
                    me.btnBack, me.btnFwd, me.btnReload,
                    { xtype: "syno_button", text: "⌂", tooltip: T("home"), handler: function () { me.showStart(true); } },
                    " ", me.urlField, " ",
                    { xtype: "syno_button", btnStyle: "blue", text: T("go"), handler: function () { me.navigate(me.urlField.getValue()); } },
                    " ", me.status, "->",
                    {
                        xtype: "syno_button", text: "⋯", tooltip: T("more"),
                        menu: { items: [
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
            me.host = document.createElement("div");
            me.host.style.cssText = "position:absolute;inset:0";
            body.appendChild(me.host);

            me.start = document.createElement("iframe");
            me.start.setAttribute("sandbox", "allow-scripts");
            me.start.style.cssText = "position:absolute;inset:0;width:100%;height:100%;border:0;background:#f5f7fa;z-index:2";
            body.appendChild(me.start);

            me.onMsg = function (e) {
                if (e.source !== me.start.contentWindow || !e.data || e.data.__bs !== 1) { return; }
                if (e.data.type === "go" && typeof e.data.url === "string") { me.navigate(e.data.url); }
            };
            window.addEventListener("message", me.onMsg);
            me.fitAddressBar(me.getWidth());
            me.showStart(false);
        },

        teardown: function () {
            if (this.onMsg) { window.removeEventListener("message", this.onMsg); }
            if (this.client) { this.client.destroy(); this.client = null; }
        },

        fitAddressBar: function (width) {
            if (this.urlField && width) { this.urlField.setWidth(Math.max(160, width - 420)); }
        },

        showStart: function (keepBrowser) {
            var data = { lang: lang(), recent: loadRecent() };
            this.start.src = BASE + "start.html#" + encodeURIComponent(JSON.stringify(data));
            this.start.style.display = "block";
            if (!keepBrowser) { this.urlField.setValue(""); }
        },

        hideStart: function () { this.start.style.display = "none"; },

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
            if (st.url && st.url !== "about:blank" && !this.urlField.hasFocus) { this.urlField.setValue(st.url); }
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
                me.client.downloadUrl(ev.id).then(function (url) {
                    mb.confirm(T("dldone"), esc(ev.name) + "<br><br>" + esc(T("save")) + "?", function (btn) {
                        if (btn !== "yes" && btn !== "ok") { return; }
                        var a = document.createElement("a");
                        a.href = url; a.download = ev.name || "download"; a.style.display = "none";
                        document.body.appendChild(a); a.click(); document.body.removeChild(a);
                    });
                });
                break;
            case "crashed":
                me.setStatus(T("crashed"));
                break;
            }
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
                saveRecent(url);
                me.hideStart();
                me.ensureClient().nav(url);
            }, function () {});
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
