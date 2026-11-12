// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"unicode/utf8"
)

// Favoriten liegen pro DSM-Benutzer auf der NAS – getrennt vom Browser-Profil,
// damit "Anmeldungen und Verlauf löschen" sie nicht mitnimmt.

const maxBookmarks = 200

type bookmark struct {
	URL   string `json:"url"`
	Title string `json:"title"`
}

var bmMu sync.Mutex

func bookmarkFile(user string) string {
	return filepath.Join(varDir(), "bookmarks", userKey(user)+".json")
}

func loadBookmarks(user string) []bookmark {
	var list []bookmark
	if b, err := os.ReadFile(bookmarkFile(user)); err == nil {
		json.Unmarshal(b, &list)
	}
	if list == nil {
		list = []bookmark{}
	}
	return list
}

func saveBookmarks(user string, list []bookmark) error {
	p := bookmarkFile(user)
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return err
	}
	b, err := json.Marshal(list)
	if err != nil {
		return err
	}
	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, p)
}

func cleanTitle(t, fallback string) string {
	t = strings.TrimSpace(strings.Map(func(r rune) rune {
		if r < 32 {
			return ' '
		}
		return r
	}, t))
	if t == "" {
		t = fallback
	}
	if utf8.RuneCountInString(t) > 120 {
		t = string([]rune(t)[:120]) + "…"
	}
	return t
}

func (d *daemon) bookmarks(w http.ResponseWriter, r *http.Request, id identity) {
	bmMu.Lock()
	defer bmMu.Unlock()
	list := loadBookmarks(id.User)
	if r.Method == http.MethodPost {
		var req struct {
			Op    string `json:"op"`
			URL   string `json:"url"`
			Title string `json:"title"`
		}
		if err := json.NewDecoder(io.LimitReader(r.Body, 16384)).Decode(&req); err != nil {
			writeJSON(w, 400, map[string]any{"success": false})
			return
		}
		u, ok := normalizeURL(req.URL)
		if !ok || u == "about:blank" {
			writeJSON(w, 400, map[string]any{"success": false, "error": "url"})
			return
		}
		keep := list[:0]
		for _, b := range list {
			if b.URL != u {
				keep = append(keep, b)
			}
		}
		list = keep
		if req.Op == "add" {
			if len(list) >= maxBookmarks {
				writeJSON(w, 400, map[string]any{"success": false, "error": "full"})
				return
			}
			list = append([]bookmark{{URL: u, Title: cleanTitle(req.Title, u)}}, list...)
		}
		if err := saveBookmarks(id.User, list); err != nil {
			writeJSON(w, 500, map[string]any{"success": false})
			return
		}
	}
	writeJSON(w, 200, map[string]any{"success": true, "items": list})
}

// downloads listet die fertigen Downloads der laufenden Sitzung.
func (d *daemon) downloads(w http.ResponseWriter, r *http.Request, id identity) {
	items := []map[string]any{}
	d.m.mu.Lock()
	s := d.m.sessions[id.User]
	d.m.mu.Unlock()
	if s != nil {
		s.mu.Lock()
		for _, g := range s.dlOrder {
			if dl := s.dl[g]; dl != nil && dl.done {
				items = append([]map[string]any{{"id": g, "name": dl.name}}, items...)
			}
		}
		s.mu.Unlock()
	}
	writeJSON(w, 200, map[string]any{"success": true, "items": items})
}
