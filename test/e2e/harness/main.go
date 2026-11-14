// SPDX-License-Identifier: AGPL-3.0-only

// Test-Aufbau für die Ende-zu-Ende-Tests: ein nachgebautes DSM (liefert
// browser.cgi und eine Testseite mit dem Browser-Kern) und ein "Gerät" im
// internen Netz mit Testseiten.
package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"net/http/cgi"
	"path/filepath"
)

func main() {
	cgiPath := flag.String("cgi", "/var/packages/NASsurfer/target/ui/browser.cgi", "installiertes browser.cgi")
	uiSrc := flag.String("uisrc", "package/ui/src", "Quellordner des Browser-Kerns (client.js)")
	groups := flag.String("groups", "administrators", "Gruppen des Test-Benutzers")
	dsmAddr := flag.String("dsm", "127.0.0.1:9100", "Adresse des nachgebauten DSM")
	devAddr := flag.String("dev", "127.0.0.1:9101", "Adresse des Test-Geräts")
	flag.Parse()

	dsm := http.NewServeMux()
	dsm.Handle("/webman/3rdparty/NASsurfer/browser.cgi", &cgi.Handler{
		Path: *cgiPath,
		Root: "/webman/3rdparty/NASsurfer/browser.cgi",
		Env: []string{"NASSURFER_TEST=1", "NASSURFER_TEST_USER=tester",
			"NASSURFER_TEST_GROUPS=" + *groups,
			"NASSURFER_GITHUB_API=http://" + *devAddr + "/gh"},
		InheritEnv: []string{"PATH"},
	})
	dsm.Handle("/webman/3rdparty/NASsurfer/src/", http.StripPrefix("/webman/3rdparty/NASsurfer/src/", http.FileServer(http.Dir(*uiSrc))))
	dsm.HandleFunc("/webman/3rdparty/NASsurfer/start.html", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, filepath.Join(*uiSrc, "..", "start.html"))
	})
	dsm.HandleFunc("/test.html", func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, testPage) })
	go func() { log.Fatal(http.ListenAndServe(*dsmAddr, dsm)) }()

	d := http.NewServeMux()
	page := func(path, html string) {
		d.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, html) })
	}
	// nachgebautes GitHub für die Paketquelle
	d.HandleFunc("/gh/repos/soleil0815/nassurfer/releases", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `[{"tag_name":"v9.9.9-0001","draft":false,"prerelease":false,"body":"## English\nTest\n## Deutsch\nTest-Release","assets":[{"name":"NASsurfer-x86_64-9.9.9-0001.spk","size":123,"browser_download_url":"https://github.com/soleil0815/nassurfer/releases/download/v9.9.9-0001/NASsurfer-x86_64-9.9.9-0001.spk"}]}]`)
	})
	page("/router/", `<html><head><title>Router</title></head><body><h1>Router</h1></body></html>`)
	page("/kb/", `<html><head><title>KB</title><style>body{margin:0}#i{position:absolute;left:10px;top:10px;width:300px;height:30px}#b{position:absolute;left:10px;top:60px;width:120px;height:30px}#p{position:absolute;left:150px;top:60px;width:120px;height:30px;display:block}#d{position:absolute;left:300px;top:60px;width:120px;height:30px;display:block}#f{position:absolute;left:10px;top:110px;width:250px;height:30px}</style></head><body>
<input id=i oninput="document.title='T:'+this.value">
<button id=b onclick="document.title='C:'+confirm('Sicher?')">confirm</button>
<a id=p href="/popup" target="_blank">popup</a>
<a id=d href="/dl">download</a>
<input id=f type=file onchange="document.title='F:'+this.files[0].name+':'+this.files[0].size">
</body></html>`)
	page("/popup", `<html><head><title>Popup-Seite</title></head><body>neu</body></html>`)
	page("/pw/", `<html><head><title>PW</title><style>body{margin:0}input,button{position:absolute;left:10px;width:250px;height:30px}#u{top:10px}#p{top:60px}#s{top:110px}</style></head><body>
<form method=post action="/pw/ok"><input id=u name=user autocomplete=username><input id=p name=pass type=password autocomplete=current-password><button id=s>Login</button></form></body></html>`)
	page("/pw/ok", `<html><head><title>PW-OK</title></head><body>angemeldet</body></html>`)
	d.HandleFunc("/dl", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Disposition", `attachment; filename="bericht.txt"; filename*=UTF-8''bericht%20%C3%A4.txt`)
		w.Header().Set("Content-Type", "text/plain")
		fmt.Fprint(w, "Inhalt der Datei")
	})
	d.HandleFunc("/lang/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `<html><head><title>L</title></head><body><script>
document.title = 'L:' + navigator.language + '|' + Intl.DateTimeFormat().resolvedOptions().timeZone + '|AL=%s';
</script></body></html>`, r.Header.Get("Accept-Language"))
	})
	log.Fatal(http.ListenAndServe(*devAddr, d))
}

const testPage = `<!doctype html><html><head><meta charset=utf-8></head><body style="margin:0">
<div id=host style="width:1000px;height:600px"></div>
<script src="/webman/3rdparty/NASsurfer/src/client.js"></script>
<script>
var tok = null; window.__ev = []; window.__st = null; window.__err = [];
function getToken() {
  if (tok) return Promise.resolve(tok);
  return fetch('/webman/3rdparty/NASsurfer/browser.cgi?a=token').then(function (r) { return r.json(); })
    .then(function (j) { tok = j.token; return tok; });
}
window.__c = new NASsurferClient({
  container: document.getElementById('host'), base: '/webman/3rdparty/NASsurfer/', getToken: getToken,
  onState: function (s) { window.__st = s; },
  onEvent: function (e) { window.__ev.push(e); if (e.ev === 'dialog') window.__c.answerDialog(true, ''); },
  onError: function (m) { if (m) window.__err.push(m); }
});
</script></body></html>`
