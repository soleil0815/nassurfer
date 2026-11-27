// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBookmarks(t *testing.T) {
	t.Setenv("BS_VAR", t.TempDir())
	d := &daemon{m: &manager{sessions: map[string]*session{}}}
	id := identity{User: "michael", Admin: true}
	call := func(method, body string) (int, []bookmark) {
		r := httptest.NewRequest(method, "/bookmarks", strings.NewReader(body))
		w := httptest.NewRecorder()
		d.bookmarks(w, r, id)
		var res struct {
			Items []bookmark `json:"items"`
		}
		json.Unmarshal(w.Body.Bytes(), &res)
		return w.Code, res.Items
	}
	if c, items := call("GET", ""); c != 200 || len(items) != 0 {
		t.Fatalf("empty: %d %v", c, items)
	}
	call("POST", `{"op":"add","url":"192.168.0.1","title":"FRITZ!Box"}`)
	_, items := call("POST", `{"op":"add","url":"https://pve:8006/","title":"Proxmox\n"}`)
	if len(items) != 2 || items[0].URL != "https://pve:8006/" || items[0].Title != "Proxmox" || items[1].URL != "http://192.168.0.1" {
		t.Errorf("add: %+v", items)
	}
	_, items = call("POST", `{"op":"add","url":"192.168.0.1","title":"neu"}`) // kein Duplikat
	if len(items) != 2 || items[0].Title != "neu" {
		t.Errorf("dedupe: %+v", items)
	}
	if c, _ := call("POST", `{"op":"add","url":"file:///etc/passwd"}`); c != 400 {
		t.Errorf("file url accepted: %d", c)
	}
	_, items = call("POST", `{"op":"remove","url":"https://pve:8006/"}`)
	if len(items) != 1 {
		t.Errorf("remove: %+v", items)
	}
	fi, err := os.Stat(filepath.Join(os.Getenv("BS_VAR"), "bookmarks", userKey("michael")+".json"))
	if err != nil || fi.Mode().Perm() != 0o600 {
		t.Errorf("file perms: %v %v", err, fi)
	}
}

func TestHardenProfile(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "Default"), 0o700)
	os.WriteFile(filepath.Join(dir, "Default", "Login Data"), []byte("x"), 0o600)
	os.WriteFile(filepath.Join(dir, "Default", "History"), []byte("x"), 0o600)
	os.MkdirAll(filepath.Join(dir, "Default", "Sessions"), 0o700)
	os.WriteFile(filepath.Join(dir, "Default", "Cookies"), []byte("x"), 0o600)
	os.WriteFile(filepath.Join(dir, "Default", "Preferences"), []byte(`{"homepage":"x","profile":{"name":"a"}}`), 0o600)
	if err := hardenProfile(dir); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"Login Data", "History", "Sessions"} {
		if _, err := os.Stat(filepath.Join(dir, "Default", f)); err == nil {
			t.Errorf("%s not removed", f)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "Default", "Cookies")); err != nil {
		t.Error("Cookies must be kept (sign-ins)")
	}
	var p map[string]any
	b, _ := os.ReadFile(filepath.Join(dir, "Default", "Preferences"))
	json.Unmarshal(b, &p)
	pr := p["profile"].(map[string]any)
	if hi, _ := p["history"].(map[string]any); hi["saving_disabled"] != true {
		t.Errorf("history saving not disabled: %s", b)
	}
	if p["credentials_enable_service"] != false || pr["password_manager_enabled"] != false || pr["name"] != "a" || p["homepage"] != "x" {
		t.Errorf("prefs: %s", b)
	}
}
