// NASsurfer – ein echter Browser (Chromium) als Synology-DSM-App.
//
// Ein Programm, zwei Rollen:
//
//	nassurfer daemon   Dienst: startet Chromium pro DSM-Benutzer und steuert ihn
//	browser.cgi             DSM-CGI: prüft die DSM-Anmeldung und vermittelt zum Dienst
//
// CGI-Aufrufe:
//
//	browser.cgi?a=token          Zugangs-Token (verlangt DSM-Anmeldung als Administrator)
//	browser.cgi?a=verify&t=…     Token prüfen (für den Dienst)
//	browser.cgi?a=api&op=…&t=…   an den Dienst weiterreichen (Bild, Eingaben, Downloads …)
//	browser.cgi?a=diag           Diagnose (nur Ja/Nein-Angaben)
package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/url"
	"os"
	"strconv"
	"strings"
)

const (
	pkgName    = "NASsurfer"
	version    = "1.0.0"
	tokenTTL   = 8 * 60 * 60 // Sekunden
	adminGroup = "administrators"
	daemonAddr = "127.0.0.1:47391"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "daemon" {
		runDaemon()
		return
	}
	runCGI()
}

func runCGI() {
	out := bufio.NewWriterSize(os.Stdout, 64*1024)
	defer out.Flush()

	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintf(os.Stderr, "nassurfer: panic: %v\n", r)
			jsonOut(out, 500, map[string]any{"success": false, "error": "internal"})
		}
	}()

	q, _ := url.ParseQuery(os.Getenv("QUERY_STRING"))
	method := requestMethod()
	a := q.Get("a")

	// Nicht gelesener Anfrage-Inhalt lässt DSM die Verbindung abbrechen
	// ("Failed to fetch"). Aktionen ohne Inhalt lesen ihn deshalb vorab leer.
	if a != "api" {
		drainBody()
	}
	defer drainBody()

	if a == "diag" {
		handleDiag(out, method)
		return
	}
	if !packageRunning() {
		jsonOut(out, 503, map[string]any{"success": false, "error": "stopped"})
		return
	}
	switch a {
	case "token":
		handleToken(out)
	case "verify":
		handleVerify(out, q)
	case "api":
		handleRelay(out, q, method)
	default:
		jsonOut(out, 400, map[string]any{"success": false, "error": "action"})
	}
}

func drainBody() {
	n, err := strconv.ParseInt(os.Getenv("CONTENT_LENGTH"), 10, 64)
	if err != nil || n <= 0 {
		return
	}
	io.Copy(io.Discard, io.LimitReader(os.Stdin, n))
}

// requestMethod: Manche Server reichen REQUEST_METHOD nicht durch.
func requestMethod() string {
	m := strings.ToUpper(strings.TrimSpace(os.Getenv("REQUEST_METHOD")))
	if m != "" {
		return m
	}
	if n, err := strconv.ParseInt(os.Getenv("CONTENT_LENGTH"), 10, 64); err == nil && n > 0 {
		return "POST"
	}
	return "GET"
}

func handleDiag(w io.Writer, method string) {
	exists := func(p string) bool { _, err := os.Stat(p); return err == nil }
	_, stateErr := stateDir()
	_, daemonErr := daemonGet("/health")
	jsonOut(w, 200, map[string]any{
		"success":      true,
		"version":      version,
		"method":       method,
		"running":      packageRunning(),
		"daemon":       daemonErr == nil,
		"authenticate": exists("/usr/syno/synoman/webman/modules/authenticate.cgi"),
		"cookie":       os.Getenv("HTTP_COOKIE") != "",
		"loggedIn":     dsmUser() != "",
		"stateDir":     stateErr == nil,
		"uid":          os.Getuid(),
	})
}

// packageRunning prüft, ob das Paket im Paket-Zentrum gestartet ist.
func packageRunning() bool {
	if os.Getenv("NASSURFER_TEST") == "1" {
		return true
	}
	varDir := "/var/packages/" + pkgName + "/var"
	_, err := os.Stat(varDir + "/running")
	if err == nil {
		return true
	}
	if errors.Is(err, fs.ErrNotExist) {
		if _, err := os.Stat(varDir); err == nil {
			return false // Ordner lesbar, Markierung fehlt: Paket ist gestoppt
		}
	}
	_, err = os.Stat("/var/packages/" + pkgName + "/enabled")
	return err == nil
}
