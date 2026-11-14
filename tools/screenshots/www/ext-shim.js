// SPDX-License-Identifier: AGPL-3.0-only
/* Nur für die README-Bilder: ein winziger Ersatz für die Teile von ExtJS und
 * DSM, die window.js benutzt. So läuft das echte Fenster-Skript außerhalb
 * von DSM. Kein Nachbau von DSM – nur genug, um das Fenster darzustellen. */
(function () {
    "use strict";

    function ns(path) {
        return path.split(".").reduce(function (o, k) { return (o[k] = o[k] || {}); }, window);
    }
    function el(tag, cls, text) {
        var e = document.createElement(tag);
        if (cls) { e.className = cls; }
        if (text !== undefined) { e.textContent = text; }
        return e;
    }

    var Ext = window.Ext = {
        ns: ns,
        apply: function (dst, src) { Object.keys(src || {}).forEach(function (k) { dst[k] = src[k]; }); return dst; },
        util: { Format: { htmlEncode: function (s) {
            return String(s).replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;").replace(/"/g, "&quot;");
        } } },
        // Klassen mit extend und callParent
        define: function (name, cfg) {
            var parent = cfg.extend ? cfg.extend.split(".").reduce(function (o, k) { return o[k]; }, window) : Object;
            var proto = Object.create(parent.prototype);
            Object.keys(cfg).forEach(function (k) {
                if (k === "extend") { return; }
                var v = cfg[k];
                if (typeof v === "function") {
                    var sup = k === "constructor" ? parent : parent.prototype[k];
                    proto[k] = function () {
                        var prev = this.callParent;
                        this.callParent = function (args) { return sup ? sup.apply(this, args || []) : undefined; };
                        try { return v.apply(this, arguments); } finally { this.callParent = prev; }
                    };
                } else {
                    proto[k] = v;
                }
            });
            var cls = function () { return proto.constructor.apply(this, arguments); };
            if (!Object.prototype.hasOwnProperty.call(cfg, "constructor")) {
                proto.constructor = function () { return parent.apply(this, arguments); };
            }
            cls.prototype = proto;
            var parts = name.split(".");
            ns(parts.slice(0, -1).join("."))[parts[parts.length - 1]] = cls;
            return cls;
        }
    };

    // ---- Bausteine -----------------------------------------------------------
    function Button(cfg) {
        var me = this;
        me.cfg = cfg;
        me.text = cfg.text || "";
        me.dom = el("button", "x-btn" + (cfg.btnStyle === "blue" ? " x-btn-blue" : ""), me.text);
        if (cfg.tooltip) { me.dom.title = cfg.tooltip; }
        me.setDisabled(!!cfg.disabled);
        if (cfg.menu) { me.menu = cfg.menu instanceof Menu ? cfg.menu : new Menu(cfg.menu); }
        me.dom.addEventListener("click", function (e) {
            e.stopPropagation();
            if (me.disabled) { return; }
            if (me.menu) { me.menu.showBy(me.dom); }
            if (cfg.handler) { cfg.handler.call(me, me, e); }
        });
    }
    Button.prototype.setDisabled = function (d) { this.disabled = d; this.dom.disabled = d; };
    Button.prototype.setText = function (t) { this.text = t; this.dom.textContent = t; };
    Button.prototype.setTooltip = function (t) { this.dom.title = t; };

    function TbText(cfg) { this.text = cfg.text || ""; this.dom = el("span", "x-tbtext"); this.dom.innerHTML = this.text; }
    TbText.prototype.setText = function (t) { this.text = t; this.dom.innerHTML = t; };

    function TextField(cfg) {
        var me = this;
        me.dom = el("input", "x-field");
        me.dom.type = "text";
        me.dom.placeholder = cfg.emptyText || "";
        me.hasFocus = false;
        if (cfg.width) { me.setWidth(cfg.width); }
        me.dom.addEventListener("focus", function () { me.hasFocus = true; if (cfg.selectOnFocus) { me.dom.select(); } });
        me.dom.addEventListener("blur", function () { me.hasFocus = false; });
        me.dom.addEventListener("keydown", function (e) {
            var l = cfg.listeners && cfg.listeners.specialkey;
            if (l && e.key === "Enter") { l(me, { ENTER: 13, getKey: function () { return 13; } }); }
        });
    }
    TextField.prototype.setValue = function (v) { this.dom.value = v; };
    TextField.prototype.getValue = function () { return this.dom.value; };
    TextField.prototype.setWidth = function (w) { this.dom.style.width = w + "px"; };
    TextField.prototype.focus = function () { this.dom.focus(); };

    var openMenu = null;
    document.addEventListener("click", function () { if (openMenu) { openMenu.hide(); } });

    function Menu(cfg) {
        cfg = cfg || {};
        this.cfg = cfg;
        this.dom = el("div", "x-menu");
        this.items = [];
        (cfg.items || []).forEach(this.add, this);
    }
    Menu.prototype.add = function (it) {
        var row = el("div", "x-menu-item" + (it.disabled ? " x-menu-disabled" : ""));
        row.innerHTML = it.text;
        if (it.tooltip) { row.title = it.tooltip; }
        row.addEventListener("click", function (e) {
            e.stopPropagation();
            if (it.disabled) { return; }
            if (openMenu) { openMenu.hide(); }
            if (it.handler) { it.handler(); }
        });
        this.dom.appendChild(row);
    };
    Menu.prototype.removeAll = function () { this.dom.textContent = ""; };
    Menu.prototype.showBy = function (anchor) {
        if (openMenu) { openMenu.hide(); }
        var l = this.cfg.listeners && this.cfg.listeners.beforeshow;
        if (l) { l(this); }
        document.body.appendChild(this.dom);
        var r = anchor.getBoundingClientRect();
        this.dom.style.top = (r.bottom + 4) + "px";
        var left = Math.min(r.left, window.innerWidth - this.dom.offsetWidth - 12);
        this.dom.style.left = left + "px";
        openMenu = this;
    };
    Menu.prototype.hide = function () { if (this.dom.parentNode) { this.dom.parentNode.removeChild(this.dom); } openMenu = null; };
    ns("Ext.menu").Menu = Menu;

    Ext.ComponentMgr = { create: function (cfg) {
        if (cfg.xtype === "tbtext") { return new TbText(cfg); }
        return new Button(cfg);
    } };
    ns("SYNO.ux").TextField = TextField;

    // ---- Meldungsfenster ----------------------------------------------------
    function MsgBox(win) { this.win = win; }
    MsgBox.prototype.show = function (title, html, buttons, cb) {
        var mask = el("div", "x-mask");
        var box = el("div", "x-msg");
        box.appendChild(el("div", "x-msg-title", title));
        var body = el("div", "x-msg-body");
        body.innerHTML = html;
        box.appendChild(body);
        var bar = el("div", "x-msg-buttons");
        buttons.forEach(function (b) {
            var btn = el("button", "x-btn" + (b.blue ? " x-btn-blue" : ""), b.text);
            btn.addEventListener("click", function () { mask.remove(); if (cb) { cb(b.id); } });
            bar.appendChild(btn);
        });
        box.appendChild(bar);
        mask.appendChild(box);
        this.win.frame.appendChild(mask);
    };
    function de() { return /^(ger|deu|de)/.test(window._S("lang")); }
    MsgBox.prototype.alert = function (title, html, cb) { this.show(title, html, [{ id: "ok", text: "OK", blue: true }], cb); };
    MsgBox.prototype.confirm = function (title, html, cb) {
        this.show(title, html, [{ id: "no", text: de() ? "Nein" : "No" }, { id: "yes", text: de() ? "Ja" : "Yes", blue: true }], cb);
    };

    // ---- App-Fenster ----------------------------------------------------------
    function AppInstance() {}
    function AppWindow(cfg) {
        var me = this;
        me.cfg = cfg;
        me.frame = el("div", "x-window");
        me.frame.style.width = cfg.width + "px";
        me.frame.style.height = cfg.height + "px";
        var head = el("div", "x-window-header");
        var icon = el("img", "x-window-icon");
        icon.src = "/webman/3rdparty/NASsurfer/images/icon_64.png";
        head.appendChild(icon);
        me.titleEl = el("span", "x-window-title", cfg.title);
        head.appendChild(me.titleEl);
        var ctl = el("span", "x-window-tools");
        ["–", "□", "✕"].forEach(function (c) { ctl.appendChild(el("span", "x-tool", c)); });
        head.appendChild(ctl);
        me.frame.appendChild(head);
        var tb = el("div", "x-toolbar");
        (cfg.tbar || []).forEach(function (it) {
            if (it === " ") { tb.appendChild(el("span", "x-spacer")); return; }
            if (it === "->") { tb.appendChild(el("span", "x-fill")); return; }
            if (!it.dom) { it = Ext.ComponentMgr.create(it); }
            tb.appendChild(it.dom);
        });
        me.frame.appendChild(tb);
        var body = el("div", "x-window-body");
        me.frame.appendChild(body);
        me.body = { dom: body };
        document.getElementById("desktop").appendChild(me.frame);
        me.msgBox = new MsgBox(me);
        if (cfg.listeners && cfg.listeners.afterrender) { cfg.listeners.afterrender.call(me); }
    }
    AppWindow.prototype.getWidth = function () { return this.cfg.width; };
    AppWindow.prototype.setTitle = function (t) { this.titleEl.textContent = t; };
    AppWindow.prototype.getMsgBox = function () { return this.msgBox; };
    ns("SYNO.SDS").AppInstance = AppInstance;
    ns("SYNO.SDS").AppWindow = AppWindow;
})();
