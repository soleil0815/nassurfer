// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"net/url"
	"strings"
	"unicode/utf8"
)

// inputEvent kommt gebündelt vom DSM-Fenster.
type inputEvent struct {
	Type string   `json:"type"` // mouse, wheel, key, text, nav, back, forward, reload, stop, resize, dialog, files
	T    string   `json:"t"`    // mouse: down/up/move, key: down/up
	X    float64  `json:"x"`
	Y    float64  `json:"y"`
	DX   float64  `json:"dx"`
	DY   float64  `json:"dy"`
	Btn  string   `json:"btn"`
	Btns int      `json:"btns"`
	N    int      `json:"n"` // Klickanzahl
	Mods int      `json:"mods"`
	Key  string   `json:"key"`
	Code string   `json:"code"`
	KC   int      `json:"kc"`
	Text string   `json:"text"`
	URL  string   `json:"url"`
	W    int      `json:"w"`
	H    int      `json:"h"`
	OK   bool     `json:"ok"`
	IDs  []string `json:"ids"`
	ID   string   `json:"id"`
}

// Modifikatoren wie in CDP: Alt=1, Strg=2, Meta=4, Umschalt=8
const (
	modAlt   = 1
	modCtrl  = 2
	modMeta  = 4
	modShift = 8
)

// Bearbeitungsbefehle: Ohne sie reagieren Eingabefelder in Chromium ohne
// Bildschirm nicht auf Strg+A, Strg+Z usw.
func editCommands(e inputEvent) []string {
	if e.Mods&(modCtrl|modMeta) == 0 || e.Mods&modAlt != 0 {
		return nil
	}
	switch strings.ToLower(e.Key) {
	case "a":
		return []string{"selectAll"}
	case "z":
		if e.Mods&modShift != 0 {
			return []string{"redo"}
		}
		return []string{"undo"}
	case "y":
		return []string{"redo"}
	case "x":
		return []string{"cut"}
	case "c":
		return []string{"copy"}
	}
	return nil
}

// normalizeURL: nur http(s) und about:blank; ohne Schema wird http:// ergänzt.
func normalizeURL(raw string) (string, bool) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return "", false
	}
	if s == "about:blank" {
		return s, true
	}
	if !strings.Contains(s, "://") {
		s = "http://" + s
	}
	u, err := url.Parse(s)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil {
		return "", false
	}
	return u.String(), true
}

func (s *session) handleInput(evs []inputEvent) {
	s.mu.Lock()
	a := s.active()
	s.lastSeen = timeNow()
	s.mu.Unlock()
	if a == nil {
		return
	}
	sid := a.sessionID
	for _, e := range evs {
		switch e.Type {
		case "mouse":
			typ := map[string]string{"down": "mousePressed", "up": "mouseReleased", "move": "mouseMoved"}[e.T]
			if typ == "" {
				continue
			}
			btn := e.Btn
			if btn == "" || typ == "mouseMoved" && e.Btns == 0 {
				btn = "none"
			}
			p := map[string]any{"type": typ, "x": e.X, "y": e.Y, "button": btn, "buttons": e.Btns, "modifiers": e.Mods}
			if typ != "mouseMoved" {
				n := e.N
				if n < 1 {
					n = 1
				}
				p["clickCount"] = n
			}
			// Nicht auf Antwort warten: Ein Klick, der einen Dialog öffnet, kehrt
			// erst zurück, wenn der Dialog beantwortet ist. Die Reihenfolge bleibt erhalten.
			s.c.fire(sid, "Input.dispatchMouseEvent", p)
		case "wheel":
			s.c.fire(sid, "Input.dispatchMouseEvent", map[string]any{
				"type": "mouseWheel", "x": e.X, "y": e.Y, "deltaX": e.DX, "deltaY": e.DY, "modifiers": e.Mods,
			})
		case "key":
			p := map[string]any{
				"key": e.Key, "code": e.Code, "modifiers": e.Mods,
				"windowsVirtualKeyCode": e.KC, "nativeVirtualKeyCode": e.KC,
			}
			if e.T == "up" {
				p["type"] = "keyUp"
			} else {
				if e.Text != "" {
					p["type"] = "keyDown"
					p["text"] = e.Text
					p["unmodifiedText"] = e.Text
				} else {
					p["type"] = "rawKeyDown"
				}
				if c := editCommands(e); c != nil {
					p["commands"] = c
				}
				if strings.HasPrefix(e.Code, "Numpad") {
					p["location"] = 3
				}
			}
			s.c.fire(sid, "Input.dispatchKeyEvent", p)
		case "text":
			// Einzelne Zeichen als echte Tastendrücke (manche Seiten werten Tasten aus),
			// längere Texte (Einfügen, Eingabehilfen) am Stück
			if utf8.RuneCountInString(e.Text) == 1 {
				s.c.fire(sid, "Input.dispatchKeyEvent", map[string]any{"type": "keyDown", "key": e.Text, "text": e.Text, "unmodifiedText": e.Text})
				s.c.fire(sid, "Input.dispatchKeyEvent", map[string]any{"type": "keyUp", "key": e.Text})
			} else if e.Text != "" {
				s.c.fire(sid, "Input.insertText", map[string]any{"text": e.Text})
			}
		case "nav":
			if u, ok := normalizeURL(e.URL); ok {
				s.c.fire(sid, "Page.navigate", map[string]any{"url": u})
			}
		case "back":
			go s.go_(-1)
		case "forward":
			go s.go_(1)
		case "reload":
			s.c.fire(sid, "Page.reload", map[string]any{"ignoreCache": false})
		case "stop":
			s.c.fire(sid, "Page.stopLoading", nil)
		case "tab_new":
			go s.newTab(e.URL)
		case "tab_switch":
			go s.switchTab(e.ID)
		case "tab_close":
			go s.closeTab(e.ID)
		case "resize":
			go s.resize(e.W, e.H)
		case "dialog":
			s.mu.Lock()
			dsid := s.dialogSID
			s.dialogSID = ""
			s.mu.Unlock()
			if dsid != "" {
				s.c.fire(dsid, "Page.handleJavaScriptDialog", map[string]any{"accept": e.OK, "promptText": e.Text})
			}
		case "files":
			s.mu.Lock()
			ch := s.chooser
			s.chooser = nil
			var paths []string
			for _, id := range e.IDs {
				if p, ok := s.uploads[id]; ok {
					paths = append(paths, p)
				}
			}
			s.mu.Unlock()
			if ch != nil && len(paths) > 0 {
				s.c.fire(ch.sessionID, "DOM.setFileInputFiles", map[string]any{"files": paths, "backendNodeId": ch.nodeID})
			}
		}
	}
}
