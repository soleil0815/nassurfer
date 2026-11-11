package main

import (
	"strings"
	"testing"
	"time"
)

func TestToken(t *testing.T) {
	key := []byte("0123456789abcdef0123456789abcdef")
	now := time.Now()
	tok := makeToken(key, identity{User: "michael", Admin: true}, now)
	id, err := checkToken(key, tok, now)
	if err != nil || id.User != "michael" || !id.Admin {
		t.Fatalf("valid token rejected: %v %+v", err, id)
	}
	if _, err := checkToken(key, tok, now.Add(tokenTTL*time.Second+time.Second)); err == nil {
		t.Error("expired token accepted")
	}
	fp := strings.Split(makeToken(key, identity{User: "gast"}, now), ".")
	fp[1] = "1"
	if _, err := checkToken(key, strings.Join(fp, "."), now); err == nil {
		t.Error("forged admin flag accepted")
	}
	if _, err := checkToken([]byte("other-key-other-key-other-key-00"), tok, now); err == nil {
		t.Error("foreign key accepted")
	}
	if exp := tokenExpiry(tok); exp.Before(now) {
		t.Error("expiry parse")
	}
}

func TestNormalizeURL(t *testing.T) {
	ok := map[string]string{"192.168.0.1": "http://192.168.0.1", "https://pve:8006/": "https://pve:8006/", "about:blank": "about:blank"}
	for in, want := range ok {
		if got, good := normalizeURL(in); !good || got != want {
			t.Errorf("%q → %q %v", in, got, good)
		}
	}
	for _, bad := range []string{"file:///etc/passwd", "chrome://settings", "javascript:alert(1)", "http://user:pw@host/", ""} {
		if _, good := normalizeURL(bad); good {
			t.Errorf("%q accepted", bad)
		}
	}
}

func TestEditCommands(t *testing.T) {
	if c := editCommands(inputEvent{Key: "a", Mods: modCtrl}); len(c) != 1 || c[0] != "selectAll" {
		t.Errorf("ctrl+a: %v", c)
	}
	if c := editCommands(inputEvent{Key: "q", Mods: modCtrl | modAlt}); c != nil {
		t.Errorf("AltGr must not map: %v", c)
	}
}

func TestParsePrefs(t *testing.T) {
	p := parsePrefs("de-DE,de,en-US,en", "Europe/Berlin")
	if p.locale != "de-DE" || p.accept != "de-DE,de;q=0.9,en-US;q=0.8,en;q=0.7" || p.tz != "Europe/Berlin" {
		t.Errorf("%+v", p)
	}
	bad := parsePrefs("de-DE,--no-sandbox,x\" y", "../../etc;rm")
	if bad.locale != "de-DE" || strings.Contains(bad.accept, "sandbox") || bad.tz != "" {
		t.Errorf("unsafe input passed: %+v", bad)
	}
	if parsePrefs("", "").orDefault().locale != "en-US" {
		t.Error("default")
	}
}
