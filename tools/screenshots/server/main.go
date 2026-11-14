// SPDX-License-Identifier: AGPL-3.0-only

// Bühne für die README-Bilder: ein nachgestelltes DSM (liefert das echte
// browser.cgi und die echte Oberfläche), ausgedachte Geräte im "internen
// Netz" (router.lan, drucker.lan, container.lan) und ein nachgebautes GitHub
// für die Paketquelle.
package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/cgi"
	"path/filepath"
	"strings"
)

const releaseBody = `## English
**Patchday update** – built and end-to-end tested automatically.

- Chromium **154.0.0** (previously 153.0.0) with the latest security fixes

**Changes since v1.0.0-0006**
- Tabs: middle click closes a tab

**Install / update:** Package Center → Manual Install → ` + "`NASsurfer-x86_64-1.0.0-0007.spk`" + `
DSM 7.0–7.2 · x86_64 · License: AGPL-3.0

## Deutsch
**Patchday-Update** – automatisch gebaut und Ende-zu-Ende getestet.

- Chromium **154.0.0** (bisher 153.0.0) mit den aktuellen Sicherheitskorrekturen

**Änderungen seit v1.0.0-0006**
- Tabs: Mittelklick schließt einen Tab

**Installation / Update:** Paket-Zentrum → Manuelle Installation → ` + "`NASsurfer-x86_64-1.0.0-0007.spk`" + `
DSM 7.0–7.2 · x86_64 · Lizenz: AGPL-3.0

<!-- nassurfer:chromium=154.0.0 -->`

// kleinste gültige PDF-Datei für den Download
const pdf = "%PDF-1.4\n1 0 obj<</Type/Catalog/Pages 2 0 R>>endobj 2 0 obj<</Type/Pages/Kids[3 0 R]/Count 1>>endobj 3 0 obj<</Type/Page/Parent 2 0 R/MediaBox[0 0 595 842]>>endobj\ntrailer<</Root 1 0 R>>\n%%EOF\n"

func main() {
	root := flag.String("root", ".", "Wurzel des Repos")
	cgiPath := flag.String("cgi", "/var/packages/NASsurfer/target/ui/browser.cgi", "installiertes browser.cgi")
	flag.Parse()
	shots := filepath.Join(*root, "tools/screenshots")

	gh := http.NewServeMux()
	gh.HandleFunc("/gh/repos/soleil0815/nassurfer/releases", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		body := strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", `\n`).Replace(releaseBody)
		fmt.Fprintf(w, `[{"tag_name":"v1.0.0-0007","draft":false,"prerelease":false,"body":"%s","assets":[{"name":"NASsurfer-x86_64-1.0.0-0007.spk","size":106700000,"browser_download_url":"https://github.com/soleil0815/nassurfer/releases/download/v1.0.0-0007/NASsurfer-x86_64-1.0.0-0007.spk"}]}]`, body)
	})
	go func() { log.Fatal(http.ListenAndServe("127.0.0.1:9201", gh)) }()

	dsm := http.NewServeMux()
	dsm.Handle("/webman/3rdparty/NASsurfer/browser.cgi", &cgi.Handler{
		Path: *cgiPath,
		Root: "/webman/3rdparty/NASsurfer/browser.cgi",
		Env: []string{"NASSURFER_TEST=1", "NASSURFER_TEST_USER=admin", "NASSURFER_TEST_GROUPS=administrators",
			"NASSURFER_GITHUB_API=http://127.0.0.1:9201/gh"},
		InheritEnv: []string{"PATH"},
	})
	dsm.Handle("/webman/3rdparty/NASsurfer/", http.StripPrefix("/webman/3rdparty/NASsurfer/",
		http.FileServer(http.Dir(filepath.Join(*root, "package/ui")))))
	dsm.Handle("/", http.FileServer(http.Dir(filepath.Join(shots, "www"))))
	go func() { log.Fatal(http.ListenAndServe("127.0.0.1:5000", dsm)) }()

	devices := filepath.Join(shots, "devices")
	files := http.FileServer(http.Dir(devices))
	dev := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host, _, err := net.SplitHostPort(r.Host)
		if err != nil {
			host = r.Host
		}
		switch {
		case r.URL.Path == "/common.css" || r.URL.Path == "/i18n.js":
			files.ServeHTTP(w, r)
		case host == "drucker.lan" && r.URL.Path == "/bericht.pdf":
			w.Header().Set("Content-Type", "application/pdf")
			w.Header().Set("Content-Disposition", `attachment; filename="Statusbericht.pdf"`)
			io.WriteString(w, pdf)
		case strings.HasSuffix(host, ".lan") && !strings.Contains(host, "/") && !strings.Contains(host, ".."):
			r.URL.Path = "/" + host + r.URL.Path
			files.ServeHTTP(w, r)
		default:
			http.NotFound(w, r)
		}
	})
	log.Fatal(http.ListenAndServe("127.0.0.1:80", dev))
}
