// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"mime"
	"net/url"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"syscall"
	"time"
)

// Pfade (für Tests per Umgebung überschreibbar)
func chromiumBin() string {
	if p := os.Getenv("BS_CHROMIUM"); p != "" {
		return p
	}
	return "/var/packages/" + pkgName + "/target/chromium/chromium"
}

func varDir() string {
	if p := os.Getenv("BS_VAR"); p != "" {
		return p
	}
	return "/var/packages/" + pkgName + "/var"
}

func userKey(user string) string {
	sum := sha256.Sum256([]byte("bs-user|" + user))
	return hex.EncodeToString(sum[:10])
}

// prefs: Sprache und Zeitzone aus dem Browser des Benutzers
type prefs struct {
	locale string // z. B. de-DE
	accept string // Accept-Language, z. B. de-DE,de;q=0.9,en;q=0.8
	tz     string // z. B. Europe/Berlin
}

var (
	reLangTag = regexp.MustCompile(`^[A-Za-z]{2,3}(-[A-Za-z0-9]{2,8}){0,2}$`)
	reTZ      = regexp.MustCompile(`^[A-Za-z]+(?:[/_+-][A-Za-z0-9_+-]+){0,3}$`)
)

// parsePrefs prüft die Angaben streng: Sie landen in Chromium-Parametern.
func parsePrefs(langs, tz string) prefs {
	var tags []string
	for _, t := range strings.Split(langs, ",") {
		t = strings.TrimSpace(t)
		if reLangTag.MatchString(t) && len(tags) < 6 {
			tags = append(tags, t)
		}
	}
	if len(tags) == 0 {
		return prefs{}
	}
	parts := []string{tags[0]}
	q := 9
	for _, t := range tags[1:] {
		parts = append(parts, fmt.Sprintf("%s;q=0.%d", t, q))
		if q > 2 {
			q--
		}
	}
	p := prefs{locale: tags[0], accept: strings.Join(parts, ",")}
	if len(tz) < 64 && reTZ.MatchString(tz) {
		p.tz = tz
	}
	return p
}

func (p prefs) orDefault() prefs {
	if p.locale == "" {
		return prefs{locale: "en-US", accept: "en-US,en;q=0.9"}
	}
	return p
}

type tab struct {
	targetID  string
	sessionID string
	url       string
	title     string
}

type download struct {
	name string
	done bool
}

type chooser struct {
	sessionID string
	nodeID    int64
}

type session struct {
	user      string
	profile   string
	work      string // Downloads/Uploads dieser Sitzung
	mu        sync.Mutex
	cond      *sync.Cond
	cmd       *exec.Cmd
	c         *cdp
	tabs      []*tab // letzter Eintrag = sichtbarer Tab
	frame     []byte
	frameSeq  int64
	stateVer  int64
	loading   bool
	canBack   bool
	canFwd    bool
	events    []map[string]any
	lastSeen  time.Time
	w, h      int
	ua        string
	dl        map[string]*download
	uploads   map[string]string
	chooser   *chooser
	dialogSID string
	dlNames   map[string]string // URL → Dateiname aus Content-Disposition
	dead      bool
	starting  bool
	pr        prefs // Sprache und Zeitzone des Benutzers
}

func newSession(user string, pr prefs) *session {
	s := &session{user: user, pr: pr.orDefault(), w: 1280, h: 800, dl: map[string]*download{}, uploads: map[string]string{}, dlNames: map[string]string{}, lastSeen: time.Now()}
	s.cond = sync.NewCond(&s.mu)
	k := userKey(user)
	s.profile = filepath.Join(varDir(), "profiles", k)
	s.work = filepath.Join(varDir(), "sessions", k)
	return s
}

func writeFontsConf() (string, error) {
	p := filepath.Join(varDir(), "fonts.conf")
	fontDir := filepath.Join(filepath.Dir(chromiumBin()), "fonts")
	cache := filepath.Join(varDir(), "fontcache")
	conf := `<?xml version="1.0"?>
<!DOCTYPE fontconfig SYSTEM "fonts.dtd">
<fontconfig>
  <dir>` + fontDir + `</dir>
  <cachedir>` + cache + `</cachedir>
  <alias><family>sans-serif</family><prefer><family>Liberation Sans</family><family>DejaVu Sans</family></prefer></alias>
  <alias><family>serif</family><prefer><family>Liberation Serif</family><family>DejaVu Serif</family></prefer></alias>
  <alias><family>monospace</family><prefer><family>Liberation Mono</family></prefer></alias>
  <alias><family>Arial</family><prefer><family>Liberation Sans</family></prefer></alias>
  <alias><family>Helvetica</family><prefer><family>Liberation Sans</family></prefer></alias>
  <alias><family>Times New Roman</family><prefer><family>Liberation Serif</family></prefer></alias>
  <alias><family>Courier New</family><prefer><family>Liberation Mono</family></prefer></alias>
</fontconfig>
`
	return p, os.WriteFile(p, []byte(conf), 0o600)
}

func (s *session) start() error {
	for _, d := range []string{s.profile, s.work} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			return err
		}
	}
	os.RemoveAll(filepath.Join(s.work, "downloads"))
	os.RemoveAll(filepath.Join(s.work, "uploads"))
	os.MkdirAll(filepath.Join(s.work, "downloads"), 0o700)
	os.MkdirAll(filepath.Join(s.work, "uploads"), 0o700)
	// Absturz-Marker aus dem Profil entfernen, sonst meckert Chromium beim Start
	os.Remove(filepath.Join(s.profile, "SingletonLock"))

	fc, err := writeFontsConf()
	if err != nil {
		return err
	}
	inR, inW, err := os.Pipe() // wir schreiben → Chromium liest (fd 3)
	if err != nil {
		return err
	}
	outR, outW, err := os.Pipe() // Chromium schreibt (fd 4) → wir lesen
	if err != nil {
		return err
	}
	args := []string{
		"--headless=shell",
		"--remote-debugging-pipe",
		"--user-data-dir=" + s.profile,
		"--no-sandbox", // DSM erlaubt keine Namensräume für Pakete; Chromium läuft als eigener Paket-Benutzer
		"--no-zygote",
		"--disable-dev-shm-usage",
		"--disable-gpu",
		"--ignore-certificate-errors", // interne Geräte haben fast immer selbst signierte Zertifikate
		"--no-first-run",
		"--no-default-browser-check",
		"--disable-background-networking",
		"--disable-component-update",
		"--disable-sync",
		"--disable-breakpad",
		"--disable-domain-reliability",
		"--disable-features=Translate,MediaRouter,OptimizationHints,DialMediaRouteProvider",
		"--password-store=basic",
		"--mute-audio",
		"--lang=" + s.pr.locale,
		"--font-render-hinting=none",
		"--renderer-process-limit=6",
		"--disk-cache-size=67108864",
		"--window-size=1280,800",
		"about:blank",
	}
	cmd := exec.Command(chromiumBin(), args...)
	cmd.ExtraFiles = []*os.File{inR, outW}
	cmd.Env = []string{
		"HOME=" + s.work,
		"FONTCONFIG_FILE=" + fc,
		"TMPDIR=" + s.work,
		"LANG=de_DE.UTF-8",
		"TZ=" + os.Getenv("TZ"),
	}
	cmd.Dir = s.work
	logf, _ := os.OpenFile(filepath.Join(s.work, "chromium.log"), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	cmd.Stdout, cmd.Stderr = logf, logf
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true, Pdeathsig: syscall.SIGKILL}
	if err := cmd.Start(); err != nil {
		inR.Close()
		inW.Close()
		outR.Close()
		outW.Close()
		return fmt.Errorf("chromium start: %w", err)
	}
	inR.Close()
	outW.Close()
	s.cmd = cmd
	s.c = newCDP(outR, inW, s.onEvent)
	go func() {
		cmd.Wait()
		if logf != nil {
			logf.Close()
		}
		s.mu.Lock()
		s.dead = true
		s.cond.Broadcast()
		s.mu.Unlock()
		log.Printf("chromium für %s beendet", s.user)
	}()

	var ver struct {
		UserAgent string `json:"userAgent"`
	}
	if err := s.c.call("", "Browser.getVersion", nil, &ver); err != nil {
		s.kill()
		return err
	}
	s.ua = strings.Replace(ver.UserAgent, "HeadlessChrome", "Chrome", 1)
	s.c.call("", "Target.setDiscoverTargets", map[string]any{"discover": true}, nil)
	s.c.call("", "Browser.setDownloadBehavior", map[string]any{
		"behavior": "allowAndName", "downloadPath": filepath.Join(s.work, "downloads"), "eventsEnabled": true,
	}, nil)

	var tg struct {
		TargetInfos []struct {
			TargetID string `json:"targetId"`
			Type     string `json:"type"`
			URL      string `json:"url"`
			Title    string `json:"title"`
		} `json:"targetInfos"`
	}
	if err := s.c.call("", "Target.getTargets", nil, &tg); err != nil {
		s.kill()
		return err
	}
	first := ""
	for _, t := range tg.TargetInfos {
		if t.Type == "page" {
			first = t.TargetID
			break
		}
	}
	if first == "" {
		var ct struct {
			TargetID string `json:"targetId"`
		}
		if err := s.c.call("", "Target.createTarget", map[string]any{"url": "about:blank"}, &ct); err != nil {
			s.kill()
			return err
		}
		first = ct.TargetID
	}
	go s.watchTitle()
	return s.attach(first)
}

// watchTitle: Chromium meldet per Skript geänderte Titel nicht von selbst.
func (s *session) watchTitle() {
	t := time.NewTicker(time.Second)
	defer t.Stop()
	for range t.C {
		s.mu.Lock()
		dead, a, recent := s.dead, s.active(), time.Since(s.lastSeen) < 30*time.Second
		s.mu.Unlock()
		if dead {
			return
		}
		if a == nil || !recent {
			continue
		}
		var r struct {
			Result struct {
				Value string `json:"value"`
			} `json:"result"`
		}
		if s.c.call(a.sessionID, "Runtime.evaluate", map[string]any{"expression": "document.title", "returnByValue": true}, &r) != nil {
			continue
		}
		s.mu.Lock()
		if r.Result.Value != "" && r.Result.Value != a.title {
			a.title = r.Result.Value
			if a == s.active() {
				s.stateVer++
				s.cond.Broadcast()
			}
		}
		s.mu.Unlock()
	}
}

// attach hängt sich an einen Tab und macht ihn sichtbar.
func (s *session) attach(targetID string) error {
	s.mu.Lock()
	for _, t := range s.tabs {
		if t.targetID == targetID {
			s.mu.Unlock()
			return s.activate(t)
		}
	}
	s.mu.Unlock()
	var at struct {
		SessionID string `json:"sessionId"`
	}
	if err := s.c.call("", "Target.attachToTarget", map[string]any{"targetId": targetID, "flatten": true}, &at); err != nil {
		return err
	}
	sid := at.SessionID
	s.c.call(sid, "Page.enable", nil, nil)
	s.c.call(sid, "Inspector.enable", nil, nil)
	// Nur für Download-Namen: Chromium ohne Bildschirm nennt alle Downloads "download"
	s.c.call(sid, "Network.enable", map[string]any{"maxTotalBufferSize": 0, "maxResourceBufferSize": 0}, nil)
	s.applyPrefsTo(sid)
	s.c.call(sid, "Page.setInterceptFileChooserDialog", map[string]any{"enabled": true}, nil)
	t := &tab{targetID: targetID, sessionID: sid}
	s.mu.Lock()
	s.tabs = append(s.tabs, t)
	s.mu.Unlock()
	return s.activate(t)
}

// applyPrefsTo setzt Sprache und Zeitzone für einen Tab.
func (s *session) applyPrefsTo(sid string) {
	s.mu.Lock()
	p := s.pr
	s.mu.Unlock()
	err := s.c.call(sid, "Emulation.setUserAgentOverride", map[string]any{
		"userAgent": s.ua, "acceptLanguage": p.accept, "platform": "Linux",
	}, nil)
	if debugCDP {
		log.Printf("prefs %s → %+v (%v)", sid, p, err)
	}
	s.c.call(sid, "Emulation.setLocaleOverride", map[string]any{}, nil) // vorherige Einstellung lösen
	s.c.call(sid, "Emulation.setLocaleOverride", map[string]any{"locale": p.locale}, nil)
	if p.tz != "" {
		s.c.call(sid, "Emulation.setTimezoneOverride", map[string]any{"timezoneId": ""}, nil) // vorherige lösen
		s.c.call(sid, "Emulation.setTimezoneOverride", map[string]any{"timezoneId": p.tz}, nil)
	}
	// Sprache gilt auch für Anfragen ans Netz (Accept-Language)
	s.c.call(sid, "Network.setUserAgentOverride", map[string]any{
		"userAgent": s.ua, "acceptLanguage": p.accept, "platform": "Linux",
	}, nil)
}

// setPrefs übernimmt geänderte Sprache/Zeitzone in alle offenen Tabs.
func (s *session) setPrefs(p prefs) {
	if p.locale == "" {
		return
	}
	s.mu.Lock()
	if p == s.pr {
		s.mu.Unlock()
		return
	}
	s.pr = p
	var sids []string
	for _, t := range s.tabs {
		sids = append(sids, t.sessionID)
	}
	s.mu.Unlock()
	for _, sid := range sids {
		s.applyPrefsTo(sid)
	}
}

func (s *session) active() *tab {
	if len(s.tabs) == 0 {
		return nil
	}
	return s.tabs[len(s.tabs)-1]
}

func (s *session) activate(t *tab) error {
	s.mu.Lock()
	old := s.active()
	if old != nil && old != t {
		s.c.fire(old.sessionID, "Page.stopScreencast", nil)
	}
	// nach hinten schieben = sichtbar
	for i, x := range s.tabs {
		if x == t {
			s.tabs = append(s.tabs[:i], s.tabs[i+1:]...)
			break
		}
	}
	s.tabs = append(s.tabs, t)
	w, h := s.w, s.h
	s.stateVer++
	s.cond.Broadcast()
	s.mu.Unlock()
	s.c.call(t.sessionID, "Emulation.setDeviceMetricsOverride", map[string]any{
		"width": w, "height": h, "deviceScaleFactor": 1, "mobile": false,
	}, nil)
	s.c.call("", "Target.activateTarget", map[string]any{"targetId": t.targetID}, nil)
	err := s.c.call(t.sessionID, "Page.startScreencast", map[string]any{
		"format": "jpeg", "quality": 80, "maxWidth": w, "maxHeight": h, "everyNthFrame": 1,
	}, nil)
	go s.refreshHistory()
	return err
}

func (s *session) tabBySession(sid string) *tab {
	for _, t := range s.tabs {
		if t.sessionID == sid {
			return t
		}
	}
	return nil
}

func (s *session) pushEvent(ev map[string]any) {
	s.events = append(s.events, ev)
	if len(s.events) > 50 {
		s.events = s.events[len(s.events)-50:]
	}
	s.cond.Broadcast()
}

var debugCDP = os.Getenv("BS_DEBUG") == "1"

func (s *session) onEvent(method, sid string, raw json.RawMessage) {
	if debugCDP && method != "Page.screencastFrame" {
		r := string(raw)
		if len(r) > 300 {
			r = r[:300]
		}
		log.Printf("cdp %s [%s] %s", method, sid, r)
	}
	switch method {
	case "Page.screencastFrame":
		var p struct {
			Data      string `json:"data"`
			SessionID int64  `json:"sessionId"`
		}
		if json.Unmarshal(raw, &p) != nil {
			return
		}
		s.c.fire(sid, "Page.screencastFrameAck", map[string]any{"sessionId": p.SessionID})
		img, err := base64.StdEncoding.DecodeString(p.Data)
		if err != nil {
			return
		}
		s.mu.Lock()
		if a := s.active(); a != nil && a.sessionID == sid {
			s.frame = img
			s.frameSeq++
			s.cond.Broadcast()
		}
		s.mu.Unlock()

	case "Target.targetInfoChanged":
		var p struct {
			TargetInfo struct {
				TargetID string `json:"targetId"`
				URL      string `json:"url"`
				Title    string `json:"title"`
			} `json:"targetInfo"`
		}
		if json.Unmarshal(raw, &p) != nil {
			return
		}
		s.mu.Lock()
		for _, t := range s.tabs {
			if t.targetID == p.TargetInfo.TargetID {
				if t.url != p.TargetInfo.URL || t.title == "" {
					t.title = p.TargetInfo.Title
				}
				t.url = p.TargetInfo.URL
				if t == s.active() {
					s.stateVer++
					s.cond.Broadcast()
				}
			}
		}
		s.mu.Unlock()
		go s.refreshHistory()

	case "Target.targetCreated":
		var p struct {
			TargetInfo struct {
				TargetID string `json:"targetId"`
				Type     string `json:"type"`
				OpenerID string `json:"openerId"`
			} `json:"targetInfo"`
		}
		if json.Unmarshal(raw, &p) != nil {
			return
		}
		if p.TargetInfo.Type == "page" && p.TargetInfo.OpenerID != "" {
			// Neues Fenster/Tab der Seite (target=_blank, window.open): anzeigen
			go s.attach(p.TargetInfo.TargetID)
		}

	case "Target.targetDestroyed", "Target.detachedFromTarget":
		var p struct {
			TargetID  string `json:"targetId"`
			SessionID string `json:"sessionId"`
		}
		if json.Unmarshal(raw, &p) != nil {
			return
		}
		s.mu.Lock()
		wasActive := false
		for i, t := range s.tabs {
			if (p.TargetID != "" && t.targetID == p.TargetID) || (p.SessionID != "" && t.sessionID == p.SessionID) {
				wasActive = i == len(s.tabs)-1
				s.tabs = append(s.tabs[:i], s.tabs[i+1:]...)
				break
			}
		}
		next := s.active()
		s.mu.Unlock()
		if wasActive {
			if next != nil {
				go s.activate(next)
			} else {
				go func() {
					var ct struct {
						TargetID string `json:"targetId"`
					}
					if s.c.call("", "Target.createTarget", map[string]any{"url": "about:blank"}, &ct) == nil {
						s.attach(ct.TargetID)
					}
				}()
			}
		}

	case "Page.frameStartedLoading", "Page.frameStoppedLoading":
		var p struct {
			FrameID string `json:"frameId"`
		}
		json.Unmarshal(raw, &p)
		s.mu.Lock()
		if a := s.active(); a != nil && a.sessionID == sid && p.FrameID == a.targetID {
			s.loading = method == "Page.frameStartedLoading"
			s.stateVer++
			s.cond.Broadcast()
		}
		s.mu.Unlock()

	case "Page.javascriptDialogOpening":
		var p struct {
			Message       string `json:"message"`
			Type          string `json:"type"`
			DefaultPrompt string `json:"defaultPrompt"`
		}
		json.Unmarshal(raw, &p)
		s.mu.Lock()
		s.dialogSID = sid
		s.pushEvent(map[string]any{"ev": "dialog", "kind": p.Type, "message": p.Message, "def": p.DefaultPrompt})
		s.mu.Unlock()

	case "Page.fileChooserOpened":
		var p struct {
			Mode          string `json:"mode"`
			BackendNodeID int64  `json:"backendNodeId"`
		}
		json.Unmarshal(raw, &p)
		s.mu.Lock()
		s.chooser = &chooser{sessionID: sid, nodeID: p.BackendNodeID}
		s.pushEvent(map[string]any{"ev": "file", "multiple": p.Mode == "selectMultiple"})
		s.mu.Unlock()

	case "Network.responseReceived":
		var p struct {
			Response struct {
				URL     string            `json:"url"`
				Headers map[string]string `json:"headers"`
			} `json:"response"`
		}
		if json.Unmarshal(raw, &p) != nil {
			return
		}
		for k, v := range p.Response.Headers {
			if strings.EqualFold(k, "content-disposition") {
				if _, params, err := mime.ParseMediaType(v); err == nil && params["filename"] != "" {
					s.mu.Lock()
					if len(s.dlNames) > 100 {
						s.dlNames = map[string]string{}
					}
					s.dlNames[p.Response.URL] = filepath.Base(params["filename"])
					s.mu.Unlock()
				}
			}
		}

	case "Browser.downloadWillBegin":
		var p struct {
			GUID string `json:"guid"`
			Name string `json:"suggestedFilename"`
			URL  string `json:"url"`
		}
		json.Unmarshal(raw, &p)
		s.mu.Lock()
		if p.Name == "" || p.Name == "download" {
			if n := s.dlNames[p.URL]; n != "" {
				p.Name = n
			} else if u, err := url.Parse(p.URL); err == nil && path.Base(u.Path) != "/" && path.Base(u.Path) != "." {
				p.Name = path.Base(u.Path)
			}
		}
		s.dl[p.GUID] = &download{name: p.Name}
		s.pushEvent(map[string]any{"ev": "dlstart", "name": p.Name})
		s.mu.Unlock()

	case "Browser.downloadProgress":
		var p struct {
			GUID  string `json:"guid"`
			State string `json:"state"`
		}
		json.Unmarshal(raw, &p)
		if p.State != "completed" {
			return
		}
		s.mu.Lock()
		if d := s.dl[p.GUID]; d != nil && !d.done {
			d.done = true
			s.pushEvent(map[string]any{"ev": "download", "id": p.GUID, "name": d.name})
		}
		s.mu.Unlock()

	case "Inspector.targetCrashed":
		s.mu.Lock()
		s.pushEvent(map[string]any{"ev": "crashed"})
		s.mu.Unlock()
		s.c.fire(sid, "Page.reload", nil)
	}
}

func (s *session) refreshHistory() {
	s.mu.Lock()
	a := s.active()
	s.mu.Unlock()
	if a == nil {
		return
	}
	var h struct {
		CurrentIndex int `json:"currentIndex"`
		Entries      []struct {
			ID int64 `json:"id"`
		} `json:"entries"`
	}
	if s.c.call(a.sessionID, "Page.getNavigationHistory", nil, &h) != nil {
		return
	}
	s.mu.Lock()
	back, fwd := h.CurrentIndex > 0, h.CurrentIndex < len(h.Entries)-1
	if back != s.canBack || fwd != s.canFwd {
		s.canBack, s.canFwd = back, fwd
		s.stateVer++
		s.cond.Broadcast()
	}
	s.mu.Unlock()
}

func (s *session) go_(delta int) {
	s.mu.Lock()
	a := s.active()
	s.mu.Unlock()
	if a == nil {
		return
	}
	var h struct {
		CurrentIndex int `json:"currentIndex"`
		Entries      []struct {
			ID int64 `json:"id"`
		} `json:"entries"`
	}
	if s.c.call(a.sessionID, "Page.getNavigationHistory", nil, &h) != nil {
		return
	}
	i := h.CurrentIndex + delta
	if i < 0 || i >= len(h.Entries) {
		return
	}
	s.c.call(a.sessionID, "Page.navigateToHistoryEntry", map[string]any{"entryId": h.Entries[i].ID}, nil)
}

func (s *session) resize(w, h int) {
	if w < 320 {
		w = 320
	}
	if h < 200 {
		h = 200
	}
	if w > 3840 {
		w = 3840
	}
	if h > 2400 {
		h = 2400
	}
	s.mu.Lock()
	if w == s.w && h == s.h {
		s.mu.Unlock()
		return
	}
	s.w, s.h = w, h
	a := s.active()
	s.mu.Unlock()
	if a == nil {
		return
	}
	s.c.call(a.sessionID, "Emulation.setDeviceMetricsOverride", map[string]any{
		"width": w, "height": h, "deviceScaleFactor": 1, "mobile": false,
	}, nil)
	s.c.call(a.sessionID, "Page.stopScreencast", nil, nil)
	s.c.call(a.sessionID, "Page.startScreencast", map[string]any{
		"format": "jpeg", "quality": 80, "maxWidth": w, "maxHeight": h, "everyNthFrame": 1,
	}, nil)
}

type pollResult struct {
	frame []byte
	state map[string]any
}

// poll wartet, bis sich Bild oder Zustand ändern (oder die Zeit abläuft).
func (s *session) poll(since, sinceState int64, wait time.Duration) pollResult {
	deadline := time.Now().Add(wait)
	timer := time.AfterFunc(wait, func() {
		s.mu.Lock()
		s.cond.Broadcast()
		s.mu.Unlock()
	})
	defer timer.Stop()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastSeen = time.Now()
	for s.frameSeq <= since && s.stateVer <= sinceState && len(s.events) == 0 && !s.dead && time.Now().Before(deadline) {
		s.cond.Wait()
	}
	s.lastSeen = time.Now()
	r := pollResult{state: map[string]any{
		"f": s.frameSeq, "s": s.stateVer, "loading": s.loading,
		"back": s.canBack, "fwd": s.canFwd, "tabs": len(s.tabs), "dead": s.dead,
		"w": s.w, "h": s.h,
	}}
	if a := s.active(); a != nil {
		r.state["url"], r.state["title"] = a.url, a.title
	}
	if len(s.events) > 0 {
		r.state["ev"] = s.events
		s.events = nil
	}
	if s.frameSeq > since {
		r.frame = s.frame
	}
	return r
}

func (s *session) kill() {
	if s.cmd != nil && s.cmd.Process != nil {
		syscall.Kill(-s.cmd.Process.Pid, syscall.SIGKILL)
		s.cmd.Process.Kill()
	}
	s.mu.Lock()
	s.dead = true
	s.cond.Broadcast()
	s.mu.Unlock()
}

// copyText liefert die aktuelle Auswahl der Seite (für die Zwischenablage).
func (s *session) copyText() string {
	s.mu.Lock()
	a := s.active()
	s.mu.Unlock()
	if a == nil {
		return ""
	}
	var r struct {
		Result struct {
			Value string `json:"value"`
		} `json:"result"`
	}
	expr := `(function(){function f(d){var a=d.activeElement;if(a&&a.tagName==="IFRAME"){try{return f(a.contentDocument)}catch(e){}}
if(a&&(a.tagName==="INPUT"||a.tagName==="TEXTAREA")&&typeof a.selectionStart==="number"){return a.value.substring(a.selectionStart,a.selectionEnd)}
return String(d.getSelection?d.getSelection():"")}return f(document)})()`
	if s.c.call(a.sessionID, "Runtime.evaluate", map[string]any{"expression": expr, "returnByValue": true}, &r) != nil {
		return ""
	}
	return r.Result.Value
}

var errNoTab = errors.New("no tab")
