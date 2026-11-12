// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"bufio"
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"mime"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

var timeNow = time.Now

const (
	idleTimeout = 15 * time.Minute
	maxSessions = 3
	pollWait    = 20 * time.Second
	maxUpload   = 4 << 30
)

// ---- Token-Prüfung -------------------------------------------------------
// Das Token stellt browser.cgi nach der DSM-Anmeldung aus. Läuft das CGI unter
// demselben Benutzer, prüfen wir direkt mit dessen Schlüssel; sonst fragen wir
// über den DSM-Port bei browser.cgi nach (a=verify) und merken uns das Ergebnis.

type verifier struct {
	mu    sync.Mutex
	cache map[string]cachedID
}

type cachedID struct {
	id  identity
	exp time.Time
}

func (v *verifier) check(tok string) (identity, bool) {
	if tok == "" || len(tok) > 512 {
		return identity{}, false
	}
	v.mu.Lock()
	if c, ok := v.cache[tok]; ok && timeNow().Before(c.exp) {
		v.mu.Unlock()
		return c.id, true
	}
	v.mu.Unlock()

	exp := tokenExpiry(tok)
	if exp.IsZero() || timeNow().After(exp) {
		return identity{}, false
	}
	var id identity
	ok := false
	if key, err := readKeyOnly(); err == nil {
		if got, err := checkToken(key, tok, timeNow()); err == nil {
			id, ok = got, true
		}
	}
	if !ok {
		id, ok = verifyViaDSM(tok)
	}
	if !ok || !id.Admin {
		return identity{}, false
	}
	v.mu.Lock()
	if len(v.cache) > 200 {
		v.cache = map[string]cachedID{}
	}
	v.cache[tok] = cachedID{id: id, exp: exp}
	v.mu.Unlock()
	return id, true
}

func tokenExpiry(tok string) time.Time {
	parts := strings.Split(tok, ".")
	if len(parts) != 4 {
		return time.Time{}
	}
	n, err := strconv.ParseInt(parts[2], 10, 64)
	if err != nil {
		return time.Time{}
	}
	return time.Unix(n, 0)
}

// readKeyOnly liest den Schlüssel des CGI, ohne einen neuen anzulegen.
func readKeyOnly() ([]byte, error) {
	dir, err := stateDir()
	if err != nil {
		return nil, err
	}
	k, err := os.ReadFile(filepath.Join(dir, "key"))
	if err != nil || len(k) != 32 {
		return nil, fmt.Errorf("no key")
	}
	return k, nil
}

var reSynoPort = regexp.MustCompile(`(?m)^(secure_admin_port|admin_port)\s*=\s*"?(\d+)"?`)

// dsmBaseURLs liefert die lokalen DSM-Adressen. Die Ports stammen aus der
// Systemkonfiguration – ein anderes Programm kann sich so nicht als DSM ausgeben.
func dsmBaseURLs() []string {
	if u := os.Getenv("BS_VERIFY_BASE"); u != "" {
		return []string{u}
	}
	https, http_ := "5001", "5000"
	if b, err := os.ReadFile("/etc/synoinfo.conf"); err == nil {
		for _, m := range reSynoPort.FindAllStringSubmatch(string(b), -1) {
			if m[1] == "secure_admin_port" {
				https = m[2]
			} else {
				http_ = m[2]
			}
		}
	}
	return []string{"https://127.0.0.1:" + https, "http://127.0.0.1:" + http_}
}

var verifyClient = &http.Client{
	Timeout: 10 * time.Second,
	Transport: &http.Transport{
		Proxy:           nil,
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, // eigene NAS über 127.0.0.1
	},
	CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
}

func verifyViaDSM(tok string) (identity, bool) {
	for _, base := range dsmBaseURLs() {
		resp, err := verifyClient.Get(base + "/webman/3rdparty/" + pkgName + "/browser.cgi?a=verify&t=" + url.QueryEscape(tok))
		if err != nil {
			continue
		}
		var r struct {
			Success bool   `json:"success"`
			User    string `json:"user"`
			Admin   bool   `json:"admin"`
		}
		err = json.NewDecoder(io.LimitReader(resp.Body, 4096)).Decode(&r)
		resp.Body.Close()
		if err == nil && resp.StatusCode == 200 && r.Success && r.User != "" {
			return identity{User: r.User, Admin: r.Admin}, true
		}
	}
	return identity{}, false
}

// ---- Sitzungen -----------------------------------------------------------

type manager struct {
	mu       sync.Mutex
	sessions map[string]*session
	starting map[string]chan struct{}
}

func (m *manager) get(user string, pr prefs) (*session, error) {
	for {
		m.mu.Lock()
		if s := m.sessions[user]; s != nil {
			s.mu.Lock()
			dead := s.dead
			s.mu.Unlock()
			if !dead {
				m.mu.Unlock()
				s.setPrefs(pr) // vor weiteren Aktionen, damit die nächste Seite schon passt
				return s, nil
			}
			delete(m.sessions, user)
		}
		if ch := m.starting[user]; ch != nil {
			m.mu.Unlock()
			<-ch
			continue
		}
		if len(m.sessions) >= maxSessions {
			m.mu.Unlock()
			return nil, fmt.Errorf("zu viele Sitzungen")
		}
		ch := make(chan struct{})
		m.starting[user] = ch
		m.mu.Unlock()

		s := newSession(user, pr)
		err := s.start()
		m.mu.Lock()
		delete(m.starting, user)
		if err == nil {
			m.sessions[user] = s
		}
		m.mu.Unlock()
		close(ch)
		if err != nil {
			log.Printf("start für %s fehlgeschlagen: %v", user, err)
			return nil, err
		}
		log.Printf("chromium für %s gestartet", user)
		return s, nil
	}
}

func (m *manager) drop(user string, wipe bool) {
	m.mu.Lock()
	s := m.sessions[user]
	delete(m.sessions, user)
	m.mu.Unlock()
	if s != nil {
		s.kill()
	}
	k := userKey(user)
	os.RemoveAll(filepath.Join(varDir(), "sessions", k))
	if wipe {
		os.RemoveAll(filepath.Join(varDir(), "profiles", k))
	}
}

func (m *manager) reap() {
	for range time.Tick(30 * time.Second) {
		m.mu.Lock()
		var idle []string
		for u, s := range m.sessions {
			s.mu.Lock()
			if timeNow().Sub(s.lastSeen) > idleTimeout || s.dead {
				idle = append(idle, u)
			}
			s.mu.Unlock()
		}
		m.mu.Unlock()
		for _, u := range idle {
			log.Printf("beende ungenutzte Sitzung von %s", u)
			m.drop(u, false)
		}
	}
}

func (m *manager) stopAll() {
	m.mu.Lock()
	var all []*session
	for _, s := range m.sessions {
		all = append(all, s)
	}
	m.mu.Unlock()
	for _, s := range all {
		s.kill()
	}
}

// ---- HTTP ----------------------------------------------------------------

type daemon struct {
	v *verifier
	m *manager
}

func runDaemon() {
	log.SetFlags(log.LstdFlags)
	log.SetPrefix("nassurfer: ")
	os.MkdirAll(varDir(), 0o700)
	d := &daemon{v: &verifier{cache: map[string]cachedID{}}, m: &manager{sessions: map[string]*session{}, starting: map[string]chan struct{}{}}}
	go d.m.reap()

	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, "ok") })
	mux.HandleFunc("/poll", d.auth(d.poll))
	mux.HandleFunc("/input", d.auth(d.input))
	mux.HandleFunc("/copy", d.auth(d.copy))
	mux.HandleFunc("/download", d.auth(d.download))
	mux.HandleFunc("/upload", d.auth(d.upload))
	mux.HandleFunc("/bookmarks", d.authNoSession(d.bookmarks))
	mux.HandleFunc("/downloads", d.authNoSession(d.downloads))
	mux.HandleFunc("/reset", d.authNoSession(func(w http.ResponseWriter, r *http.Request, id identity) {
		d.m.drop(id.User, true)
		writeJSON(w, 200, map[string]any{"success": true})
	}))
	mux.HandleFunc("/stop", d.authNoSession(func(w http.ResponseWriter, r *http.Request, id identity) {
		d.m.drop(id.User, false)
		writeJSON(w, 200, map[string]any{"success": true})
	}))

	addr := os.Getenv("BS_LISTEN")
	if addr == "" {
		addr = daemonAddr
	}
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		log.Fatalf("kann %s nicht öffnen: %v", addr, err)
	}
	srv := &http.Server{Handler: mux, ReadHeaderTimeout: 10 * time.Second}

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGTERM, syscall.SIGINT)
	go func() {
		<-sig
		log.Print("beende …")
		d.m.stopAll()
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		srv.Shutdown(ctx)
	}()
	log.Printf("Dienst %s läuft auf %s", version, addr)
	if err := srv.Serve(ln); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func (d *daemon) authNoSession(h func(http.ResponseWriter, *http.Request, identity)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := d.v.check(r.Header.Get("X-BS-Token"))
		if !ok {
			writeJSON(w, 401, map[string]any{"success": false, "error": "token"})
			return
		}
		h(w, r, id)
	}
}

func (d *daemon) auth(h func(http.ResponseWriter, *http.Request, *session)) http.HandlerFunc {
	return d.authNoSession(func(w http.ResponseWriter, r *http.Request, id identity) {
		q := r.URL.Query()
		s, err := d.m.get(id.User, parsePrefs(q.Get("lang"), q.Get("tz")))
		if err != nil {
			writeJSON(w, 503, map[string]any{"success": false, "error": "start", "detail": err.Error()})
			return
		}
		h(w, r, s)
	})
}

func (d *daemon) poll(w http.ResponseWriter, r *http.Request, s *session) {
	since, _ := strconv.ParseInt(r.URL.Query().Get("f"), 10, 64)
	sinceState, _ := strconv.ParseInt(r.URL.Query().Get("s"), 10, 64)
	res := s.poll(since, sinceState, pollWait)
	st, _ := json.Marshal(res.state)
	w.Header().Set("X-BS-State", base64.RawURLEncoding.EncodeToString(st))
	if len(res.frame) > 0 {
		w.Header().Set("Content-Type", "image/jpeg")
	} else {
		w.Header().Set("Content-Type", "application/octet-stream")
	}
	w.Header().Set("Content-Length", strconv.Itoa(len(res.frame)))
	w.WriteHeader(200)
	w.Write(res.frame)
}

func (d *daemon) input(w http.ResponseWriter, r *http.Request, s *session) {
	var evs []inputEvent
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&evs); err != nil {
		writeJSON(w, 400, map[string]any{"success": false})
		return
	}
	if len(evs) > 500 {
		evs = evs[:500]
	}
	s.handleInput(evs)
	writeJSON(w, 200, map[string]any{"success": true})
}

func (d *daemon) copy(w http.ResponseWriter, r *http.Request, s *session) {
	writeJSON(w, 200, map[string]any{"success": true, "text": s.copyText()})
}

var reSafeID = regexp.MustCompile(`^[A-Za-z0-9-]{8,64}$`)

func (d *daemon) download(w http.ResponseWriter, r *http.Request, s *session) {
	id := r.URL.Query().Get("id")
	if !reSafeID.MatchString(id) {
		http.Error(w, "bad id", 400)
		return
	}
	s.mu.Lock()
	dl := s.dl[id]
	s.mu.Unlock()
	if dl == nil || !dl.done {
		http.Error(w, "not found", 404)
		return
	}
	f, err := os.Open(filepath.Join(s.work, "downloads", id))
	if err != nil {
		http.Error(w, "not found", 404)
		return
	}
	defer f.Close()
	fi, _ := f.Stat()
	name := dl.name
	if name == "" {
		name = "download"
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": name}))
	if fi != nil {
		w.Header().Set("Content-Length", strconv.FormatInt(fi.Size(), 10))
	}
	io.Copy(w, bufio.NewReader(f))
}

var reBadName = regexp.MustCompile(`[^\p{L}\p{N}._ -]+`)

func (d *daemon) upload(w http.ResponseWriter, r *http.Request, s *session) {
	name := reBadName.ReplaceAllString(filepath.Base(r.URL.Query().Get("name")), "_")
	if name == "" || name == "." || name == ".." {
		name = "upload"
	}
	b := make([]byte, 8)
	rand.Read(b)
	id := hex.EncodeToString(b)
	dir := filepath.Join(s.work, "uploads", id)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		writeJSON(w, 500, map[string]any{"success": false})
		return
	}
	p := filepath.Join(dir, name)
	f, err := os.OpenFile(p, os.O_CREATE|os.O_WRONLY|os.O_EXCL, 0o600)
	if err != nil {
		writeJSON(w, 500, map[string]any{"success": false})
		return
	}
	_, err = io.Copy(f, io.LimitReader(r.Body, maxUpload))
	f.Close()
	if err != nil {
		os.RemoveAll(dir)
		writeJSON(w, 500, map[string]any{"success": false})
		return
	}
	s.mu.Lock()
	s.uploads[id] = p
	s.mu.Unlock()
	writeJSON(w, 200, map[string]any{"success": true, "id": id})
}
