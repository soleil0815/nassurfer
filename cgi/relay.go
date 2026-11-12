// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

// Erlaubte Dienst-Aufrufe und ihre HTTP-Methoden.
var relayOps = map[string]string{
	"poll":      "GET",
	"input":     "POST",
	"copy":      "POST",
	"download":  "GET",
	"downloads": "GET",
	"bookmarks": "GET POST",
	"upload":    "POST",
	"reset":     "POST",
	"stop":      "POST",
}

var relayClient = &http.Client{Timeout: 10 * time.Minute}

// handleRelay reicht eine Anfrage des DSM-Fensters an den Dienst weiter.
// Das Token wird hier geprüft und zusätzlich vom Dienst selbst.
func handleRelay(w io.Writer, q url.Values, method string) {
	id, err := identityFromQuery(q)
	if err != nil || !id.Admin {
		jsonOut(w, 401, map[string]any{"success": false, "error": "token"})
		return
	}
	op := q.Get("op")
	want, ok := relayOps[op]
	if !ok {
		jsonOut(w, 400, map[string]any{"success": false, "error": "op"})
		return
	}
	if !strings.Contains(" "+want+" ", " "+method+" ") {
		jsonOut(w, 405, map[string]any{"success": false, "error": "method"})
		return
	}
	fwd := url.Values{}
	for k, v := range q {
		switch k {
		case "a", "t", "op", "SynoToken":
		default:
			fwd[k] = v
		}
	}
	var body io.Reader
	var clen int64
	if n, err := strconv.ParseInt(os.Getenv("CONTENT_LENGTH"), 10, 64); err == nil && n > 0 {
		clen, body = n, io.LimitReader(os.Stdin, n)
	}
	req, err := http.NewRequest(method, "http://"+daemonAddr+"/"+op+"?"+fwd.Encode(), body)
	if err != nil {
		jsonOut(w, 400, map[string]any{"success": false})
		return
	}
	req.ContentLength = clen
	req.Header.Set("X-BS-Token", q.Get("t"))
	if ct := os.Getenv("CONTENT_TYPE"); ct != "" {
		req.Header.Set("Content-Type", ct)
	}
	resp, err := relayClient.Do(req)
	if err != nil {
		jsonOut(w, 502, map[string]any{"success": false, "error": "daemon"})
		return
	}
	defer resp.Body.Close()
	h := http.Header{}
	for _, k := range []string{"Content-Type", "Content-Disposition", "Content-Length"} {
		if v := resp.Header.Get(k); v != "" {
			h.Set(k, v)
		}
	}
	h.Set("Cache-Control", "no-store")
	h.Set("X-Content-Type-Options", "nosniff")
	writeHead(w, resp.StatusCode, h)
	if f, ok := w.(interface{ Flush() error }); ok {
		f.Flush()
	}
	io.Copy(w, resp.Body)
}

func daemonGet(path string) ([]byte, error) {
	c := &http.Client{Timeout: 3 * time.Second}
	resp, err := c.Get("http://" + daemonAddr + path)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	return io.ReadAll(io.LimitReader(resp.Body, 4096))
}
