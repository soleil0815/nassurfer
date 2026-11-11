// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

var b64 = base64.RawURLEncoding

// stateDir liefert ein privates Verzeichnis (0700) für Schlüssel und Cookies.
// Es liegt unter /tmp und wird beim Neustart der NAS geleert: Tokens und
// gespeicherte Anmeldungen bei Geräten verfallen dann automatisch.
func stateDir() (string, error) {
	base := os.Getenv("NASSURFER_STATE")
	if base == "" {
		base = filepath.Join(os.TempDir(), fmt.Sprintf("nassurfer-%d", os.Getuid()))
	}
	if err := os.Mkdir(base, 0o700); err != nil && !errors.Is(err, os.ErrExist) {
		return "", err
	}
	// Nicht auf einen untergeschobenen Symlink oder fremden Ordner hereinfallen.
	fi, err := os.Lstat(base)
	if err != nil {
		return "", err
	}
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !fi.IsDir() || !ok || int(st.Uid) != os.Getuid() || fi.Mode().Perm()&0o077 != 0 {
		return "", errors.New("state dir not private: " + base)
	}
	return base, nil
}

func secretKey() ([]byte, error) {
	dir, err := stateDir()
	if err != nil {
		return nil, err
	}
	p := filepath.Join(dir, "key")
	if k, err := os.ReadFile(p); err == nil && len(k) == 32 {
		return k, nil
	}
	k := make([]byte, 32)
	if _, err := rand.Read(k); err != nil {
		return nil, err
	}
	tmp := p + ".tmp" + strconv.Itoa(os.Getpid())
	if err := os.WriteFile(tmp, k, 0o600); err != nil {
		return nil, err
	}
	// Link statt Rename: gewinnt bei gleichzeitigen Aufrufen nur einer.
	if err := os.Link(tmp, p); err != nil && !errors.Is(err, os.ErrExist) {
		os.Remove(tmp)
		return nil, err
	}
	os.Remove(tmp)
	k, err = os.ReadFile(p)
	if err != nil || len(k) != 32 {
		return nil, errors.New("key unreadable")
	}
	return k, nil
}

type identity struct {
	User  string
	Admin bool
}

func sign(key []byte, payload string) string {
	m := hmac.New(sha256.New, key)
	m.Write([]byte("nassurfer/v1|" + payload))
	return b64.EncodeToString(m.Sum(nil))
}

// Token: base64(user) "." admin(0|1) "." ablauf "." signatur
func makeToken(key []byte, id identity, now time.Time) string {
	adm := "0"
	if id.Admin {
		adm = "1"
	}
	payload := b64.EncodeToString([]byte(id.User)) + "." + adm + "." + strconv.FormatInt(now.Unix()+tokenTTL, 10)
	return payload + "." + sign(key, payload)
}

func checkToken(key []byte, tok string, now time.Time) (identity, error) {
	parts := strings.Split(tok, ".")
	if len(parts) != 4 {
		return identity{}, errors.New("malformed")
	}
	payload := parts[0] + "." + parts[1] + "." + parts[2]
	if !hmac.Equal([]byte(sign(key, payload)), []byte(parts[3])) {
		return identity{}, errors.New("signature")
	}
	exp, err := strconv.ParseInt(parts[2], 10, 64)
	if err != nil || now.Unix() > exp {
		return identity{}, errors.New("expired")
	}
	u, err := b64.DecodeString(parts[0])
	if err != nil || len(u) == 0 {
		return identity{}, errors.New("user")
	}
	return identity{User: string(u), Admin: parts[1] == "1"}, nil
}

// dsmUser fragt DSM, wer angemeldet ist. authenticate.cgi liest dafür
// Cookie und SynoToken aus der CGI-Umgebung, die wir einfach weiterreichen.
func dsmUser() string {
	if u := os.Getenv("NASSURFER_TEST_USER"); u != "" && os.Getenv("NASSURFER_TEST") == "1" {
		return u
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "/usr/syno/synoman/webman/modules/authenticate.cgi")
	cmd.Stdin = nil
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	u := strings.TrimSpace(string(out))
	if strings.ContainsAny(u, "\n\r\x00") || len(u) > 256 {
		return ""
	}
	return u
}

func userGroups(user string) []string {
	if g := os.Getenv("NASSURFER_TEST_GROUPS"); os.Getenv("NASSURFER_TEST") == "1" {
		return strings.Fields(g)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	bin := "/usr/bin/id"
	if _, err := os.Stat(bin); err != nil {
		bin = "id"
	}
	out, err := exec.CommandContext(ctx, bin, "-Gn", "--", user).Output()
	if err != nil {
		return nil
	}
	return strings.Fields(string(out))
}

func handleToken(w io.Writer) {
	user := dsmUser()
	if user == "" {
		jsonOut(w, 401, map[string]any{"success": false, "error": "login"})
		return
	}
	// Ein echter Browser auf der NAS erreicht alles, was die NAS erreicht:
	// deshalb nur für Administratoren.
	admin := false
	for _, g := range userGroups(user) {
		if strings.EqualFold(g, adminGroup) {
			admin = true
		}
	}
	if !admin {
		jsonOut(w, 403, map[string]any{"success": false, "error": "forbidden"})
		return
	}
	key, err := secretKey()
	if err != nil {
		fmt.Fprintln(os.Stderr, "nassurfer:", err)
		jsonOut(w, 500, map[string]any{"success": false, "error": "state"})
		return
	}
	jsonOut(w, 200, map[string]any{
		"success": true,
		"token":   makeToken(key, identity{User: user, Admin: true}, time.Now()),
		"user":    user,
		"ttl":     tokenTTL,
	})
}

// handleVerify: Der Dienst fragt hier nach, ob ein Token echt ist.
func handleVerify(w io.Writer, q map[string][]string) {
	id, err := identityFromQuery(q)
	if err != nil || !id.Admin {
		jsonOut(w, 401, map[string]any{"success": false})
		return
	}
	jsonOut(w, 200, map[string]any{"success": true, "user": id.User, "admin": id.Admin})
}

func identityFromQuery(q map[string][]string) (identity, error) {
	t := ""
	if v := q["t"]; len(v) > 0 {
		t = v[0]
	}
	if t == "" {
		return identity{}, errors.New("missing")
	}
	key, err := secretKey()
	if err != nil {
		return identity{}, err
	}
	return checkToken(key, t, time.Now())
}

func jsonOut(w io.Writer, status int, v any) {
	var buf bytes.Buffer
	json.NewEncoder(&buf).Encode(v)
	h := http.Header{}
	h.Set("Content-Type", "application/json; charset=utf-8")
	h.Set("Cache-Control", "no-store")
	h.Set("X-Content-Type-Options", "nosniff")
	writeHead(w, status, h)
	w.Write(buf.Bytes())
}

func statusText(code int) string {
	if t := http.StatusText(code); t != "" {
		return t
	}
	return "Status"
}

func writeHead(w io.Writer, status int, h http.Header) {
	fmt.Fprintf(w, "Status: %d %s\r\n", status, statusText(status))
	for k, vs := range h {
		for _, v := range vs {
			v = strings.NewReplacer("\r", "", "\n", "").Replace(v)
			fmt.Fprintf(w, "%s: %s\r\n", k, v)
		}
	}
	fmt.Fprint(w, "\r\n")
}
