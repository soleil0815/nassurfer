// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"encoding/json"
	"errors"
	"html"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Paketquelle für das DSM-Paket-Zentrum.
//
// Das Paket-Zentrum fragt browser.cgi?a=source nach Updates (mit Prozessor,
// DSM-Version und Sprache). NASsurfer schaut bei GitHub nach dem neuesten
// Release mit fertigem Paket und antwortet im Format des Paket-Zentrums.
// Die Datei lädt das Paket-Zentrum dann direkt von GitHub.

const (
	sourceRepo     = "soleil0815/nassurfer"
	sourceCacheTTL = time.Hour
)

var reAssetName = regexp.MustCompile(`^NASsurfer-x86_64-(\d+\.\d+\.\d+-\d{4})\.spk$`)

// DSM-Plattformen ohne Intel/AMD-Prozessor: Für sie gibt es kein Paket.
var nonX86Platforms = []string{
	"armada", "alpine", "comcerto", "monaco", "hi3535", "rtd1", "qoriq", "88f628",
	"evansport", "ppc", "arm", "aarch64",
}

type ghRelease struct {
	TagName    string `json:"tag_name"`
	Draft      bool   `json:"draft"`
	Prerelease bool   `json:"prerelease"`
	Body       string `json:"body"`
	HTMLURL    string `json:"html_url"`
	Assets     []struct {
		Name string `json:"name"`
		Size int64  `json:"size"`
		URL  string `json:"browser_download_url"`
	} `json:"assets"`
}

type sourceRelease struct {
	Version string `json:"version"`
	Link    string `json:"link"`
	Size    int64  `json:"size"`
	Body    string `json:"body"`
	Page    string `json:"page"`
	Fetched int64  `json:"fetched"`
}

func githubAPI() string {
	if u := os.Getenv("NASSURFER_GITHUB_API"); u != "" && os.Getenv("NASSURFER_TEST") == "1" {
		return strings.TrimRight(u, "/")
	}
	return "https://api.github.com"
}

// latestRelease liefert den neuesten veröffentlichten Release mit fertigem Paket.
// Ergebnis wird eine Stunde zwischengespeichert (GitHub erlaubt ohne Anmeldung
// nur 60 Abfragen pro Stunde).
func latestRelease() (*sourceRelease, error) {
	var cacheFile string
	if dir, err := stateDir(); err == nil {
		cacheFile = filepath.Join(dir, "source-cache.json")
		if b, err := os.ReadFile(cacheFile); err == nil {
			var c sourceRelease
			if json.Unmarshal(b, &c) == nil && time.Since(time.Unix(c.Fetched, 0)) < sourceCacheTTL && c.Version != "" {
				return &c, nil
			}
		}
	}
	client := &http.Client{Timeout: 15 * time.Second}
	req, _ := http.NewRequest("GET", githubAPI()+"/repos/"+sourceRepo+"/releases?per_page=10", nil)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "NASsurfer/"+version)
	resp, err := client.Do(req)
	if err != nil {
		return staleCache(cacheFile, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return staleCache(cacheFile, errors.New("github: "+resp.Status))
	}
	var rels []ghRelease
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&rels); err != nil {
		return staleCache(cacheFile, err)
	}
	r := pickRelease(rels)
	if r == nil {
		return nil, errors.New("kein Release mit Paket gefunden")
	}
	r.Fetched = time.Now().Unix()
	if cacheFile != "" {
		if b, err := json.Marshal(r); err == nil {
			os.WriteFile(cacheFile+".tmp", b, 0o600)
			os.Rename(cacheFile+".tmp", cacheFile)
		}
	}
	return r, nil
}

// staleCache: Ist GitHub nicht erreichbar, lieber die letzte bekannte Antwort.
func staleCache(cacheFile string, err error) (*sourceRelease, error) {
	if cacheFile != "" {
		if b, e := os.ReadFile(cacheFile); e == nil {
			var c sourceRelease
			if json.Unmarshal(b, &c) == nil && c.Version != "" {
				return &c, nil
			}
		}
	}
	return nil, err
}

// pickRelease: neuester veröffentlichter Release (kein Entwurf, keine Vorabversion),
// an dem das Paket schon hängt. Direkt nach dem Anlegen baut GitHub noch –
// dann gilt bis dahin der vorherige.
func pickRelease(rels []ghRelease) *sourceRelease {
	var best *sourceRelease
	for _, r := range rels {
		if r.Draft || r.Prerelease {
			continue
		}
		for _, a := range r.Assets {
			m := reAssetName.FindStringSubmatch(a.Name)
			if m == nil || !strings.HasPrefix(a.URL, "https://") {
				continue
			}
			c := &sourceRelease{Version: m[1], Link: a.URL, Size: a.Size, Body: r.Body, Page: r.HTMLURL}
			if best == nil || compareVersion(c.Version, best.Version) > 0 {
				best = c
			}
		}
	}
	return best
}

// compareVersion vergleicht "1.0.0-0004"-Versionen.
func compareVersion(a, b string) int {
	pa := strings.FieldsFunc(a, func(r rune) bool { return r == '.' || r == '-' })
	pb := strings.FieldsFunc(b, func(r rune) bool { return r == '.' || r == '-' })
	for i := 0; i < len(pa) || i < len(pb); i++ {
		var x, y int
		if i < len(pa) {
			x, _ = strconv.Atoi(pa[i])
		}
		if i < len(pb) {
			y, _ = strconv.Atoi(pb[i])
		}
		if x != y {
			if x < y {
				return -1
			}
			return 1
		}
	}
	return 0
}

var (
	reHTMLComment = regexp.MustCompile(`(?s)<!--.*?-->`)
	reMdMarks     = regexp.MustCompile("[*`]")
	reInstallHint = regexp.MustCompile(`^(Install / update|Installation / Update):|^DSM 7\.`)
)

// changelog: passender Sprachabschnitt der Release-Notizen als schlichter Text.
func changelog(body string, german bool) string {
	body = reHTMLComment.ReplaceAllString(body, "")
	head := "## English"
	if german {
		head = "## Deutsch"
	}
	if i := strings.Index(body, head); i >= 0 {
		body = body[i+len(head):]
		if j := strings.Index(body, "\n## "); j >= 0 {
			body = body[:j]
		}
	}
	body = strings.ReplaceAll(body, "\r\n", "\n")
	body = reMdMarks.ReplaceAllString(body, "")
	// Das Paket-Zentrum zeigt die Notizen als HTML: Zeilen behalten, Text
	// maskieren, Installationshinweise (dort überflüssig) weglassen.
	var lines []string
	for _, l := range strings.Split(body, "\n") {
		l = strings.TrimSpace(l)
		if l == "" || reInstallHint.MatchString(l) {
			continue
		}
		lines = append(lines, l)
	}
	text := strings.Join(lines, "\n")
	if r := []rune(text); len(r) > 1500 {
		text = string(r[:1500]) + " …"
	}
	return strings.ReplaceAll(html.EscapeString(text), "\n", "<br>")
}

func isX86(platform string) bool {
	p := strings.ToLower(platform)
	for _, n := range nonX86Platforms {
		if strings.HasPrefix(p, n) {
			return false
		}
	}
	return true
}

// handleSource beantwortet die Anfrage des Paket-Zentrums. Ohne Anmeldung:
// Es werden nur öffentliche Angaben über den neuesten Release ausgeliefert.
func handleSource(w io.Writer, q url.Values) {
	empty := map[string]any{"packages": []any{}}
	if arch := q.Get("arch"); arch != "" && !isX86(arch) {
		jsonOut(w, 200, empty)
		return
	}
	if b, err := strconv.Atoi(q.Get("build")); err == nil && b > 0 && b < 40000 {
		jsonOut(w, 200, empty) // vor DSM 7
		return
	}
	r, err := latestRelease()
	if err != nil {
		jsonOut(w, 200, empty)
		return
	}
	german := strings.HasPrefix(q.Get("language"), "ger")
	desc := "NASsurfer – a real Chromium browser for DSM. Reach the web interfaces of internal devices (router, printer, Proxmox, Docker tools) through your NAS, without VPN. Administrators only."
	if german {
		desc = "NASsurfer – ein echter Chromium-Browser für DSM. Erreicht die Weboberflächen interner Geräte (Router, Drucker, Proxmox, Docker-Werkzeuge) über die NAS, ohne VPN. Nur für Administratoren."
	}
	raw := "https://raw.githubusercontent.com/" + sourceRepo + "/main/package/"
	pkg := map[string]any{
		"package":               pkgName,
		"version":               r.Version,
		"dname":                 "NASsurfer",
		"desc":                  desc,
		"link":                  r.Link,
		"size":                  r.Size,
		"thumbnail":             []string{raw + "PACKAGE_ICON.PNG", raw + "PACKAGE_ICON_256.PNG"},
		"thumbnail_retina":      []string{raw + "PACKAGE_ICON_256.PNG", raw + "PACKAGE_ICON_256.PNG"},
		"snapshot":              []string{},
		"qinst":                 true,
		"qupgrade":              true,
		"qstart":                true,
		"startable":             "yes",
		"deppkgs":               nil,
		"conflictpkgs":          nil,
		"download_count":        0,
		"recent_download_count": 0,
		"maintainer":            "soleil0815",
		"maintainer_url":        "https://github.com/" + sourceRepo,
		"distributor":           "soleil0815",
		"distributor_url":       "https://github.com/" + sourceRepo,
		"support_url":           "https://github.com/" + sourceRepo + "/issues",
		"changelog":             changelog(r.Body, german),
	}
	jsonOut(w, 200, map[string]any{"packages": []any{pkg}})
}

// sourceParams: Das Paket-Zentrum schickt seine Angaben per GET oder POST.
func sourceParams(q url.Values) url.Values {
	out := url.Values{}
	for k, v := range q {
		out[k] = v
	}
	if strings.HasPrefix(os.Getenv("CONTENT_TYPE"), "application/x-www-form-urlencoded") {
		if n, err := strconv.ParseInt(os.Getenv("CONTENT_LENGTH"), 10, 64); err == nil && n > 0 && n < 65536 {
			b, _ := io.ReadAll(io.LimitReader(os.Stdin, n))
			if f, err := url.ParseQuery(string(b)); err == nil {
				for k, v := range f {
					if _, ok := out[k]; !ok {
						out[k] = v
					}
				}
			}
		}
	}
	return out
}
