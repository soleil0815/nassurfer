// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"sort"
	"sync/atomic"
)

const maxTabs = 12

var tabCounter atomic.Int64

// tabList liefert die Tabs in der Reihenfolge, in der sie geöffnet wurden.
// Aufruf nur mit gehaltenem s.mu.
func (s *session) tabList() []map[string]any {
	a := s.active()
	ts := append([]*tab(nil), s.tabs...)
	sort.Slice(ts, func(i, j int) bool { return ts[i].n < ts[j].n })
	out := make([]map[string]any, 0, len(ts))
	for _, t := range ts {
		out = append(out, map[string]any{"id": t.targetID, "title": t.title, "url": t.url, "active": t == a})
	}
	return out
}

func (s *session) tabByID(id string) *tab {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, t := range s.tabs {
		if t.targetID == id {
			return t
		}
	}
	return nil
}

// newTab öffnet einen Tab (leer oder mit Adresse) und zeigt ihn an.
func (s *session) newTab(rawURL string) {
	s.mu.Lock()
	n := len(s.tabs)
	s.mu.Unlock()
	if n >= maxTabs {
		s.mu.Lock()
		s.pushEvent(map[string]any{"ev": "tablimit", "max": maxTabs})
		s.mu.Unlock()
		return
	}
	u := "about:blank"
	if rawURL != "" {
		if v, ok := normalizeURL(rawURL); ok {
			u = v
		}
	}
	var ct struct {
		TargetID string `json:"targetId"`
	}
	if s.c.call("", "Target.createTarget", map[string]any{"url": u}, &ct) == nil {
		s.attach(ct.TargetID)
	}
}

func (s *session) switchTab(id string) {
	if t := s.tabByID(id); t != nil {
		s.activate(t)
	}
}

// closeTab schließt einen Tab. Ist es der letzte, entsteht ein leerer neuer.
func (s *session) closeTab(id string) {
	if s.tabByID(id) == nil {
		return
	}
	s.c.call("", "Target.closeTarget", map[string]any{"targetId": id}, nil)
}
