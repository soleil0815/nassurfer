// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

const fakeReleases = `[
 {"tag_name":"v1.0.0-0005","draft":true,"prerelease":false,"assets":[{"name":"NASsurfer-x86_64-1.0.0-0005.spk","size":1,"browser_download_url":"https://example.com/5.spk"}]},
 {"tag_name":"v1.0.0-0004","draft":false,"prerelease":false,"assets":[]},
 {"tag_name":"v1.0.0-0003","draft":false,"prerelease":false,"html_url":"https://github.com/x/r/3",
  "body":"## English\nFixed **502**.\n\n## Deutsch\n**502** behoben.\n\n<!-- nassurfer:chromium=153.0.0 -->",
  "assets":[{"name":"NASsurfer-x86_64-1.0.0-0003.spk","size":106608640,"browser_download_url":"https://github.com/x/r/releases/download/v1.0.0-0003/NASsurfer-x86_64-1.0.0-0003.spk"}]},
 {"tag_name":"v1.0.0-0002","draft":false,"prerelease":false,"assets":[{"name":"NASsurfer-x86_64-1.0.0-0002.spk","size":1,"browser_download_url":"https://example.com/2.spk"}]}
]`

func runSource(t *testing.T, q url.Values) map[string]any {
	t.Helper()
	var buf bytes.Buffer
	handleSource(&buf, q)
	out := buf.String()
	i := strings.Index(out, "\r\n\r\n")
	var res map[string]any
	if err := json.Unmarshal([]byte(out[i+4:]), &res); err != nil {
		t.Fatalf("json: %v\n%s", err, out)
	}
	return res
}

func TestSource(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.Path != "/repos/"+sourceRepo+"/releases" {
			http.NotFound(w, r)
			return
		}
		fmt.Fprint(w, fakeReleases)
	}))
	defer srv.Close()
	t.Setenv("NASSURFER_TEST", "1")
	t.Setenv("NASSURFER_GITHUB_API", srv.URL)
	t.Setenv("NASSURFER_STATE", t.TempDir()+"/s")

	res := runSource(t, url.Values{"arch": {"geminilake"}, "build": {"72806"}, "language": {"ger"}})
	pk := res["packages"].([]any)
	if len(pk) != 1 {
		t.Fatalf("packages: %v", res)
	}
	p := pk[0].(map[string]any)
	if p["package"] != "NASsurfer" || p["version"] != "1.0.0-0003" || !strings.HasSuffix(p["link"].(string), "1.0.0-0003.spk") {
		t.Errorf("falscher Release: %v", p)
	}
	if p["changelog"] != "502 behoben." || !strings.Contains(p["desc"].(string), "Chromium-Browser") {
		t.Errorf("deutsch: %q %q", p["changelog"], p["desc"])
	}
	if p["qupgrade"] != true || p["startable"] != "yes" {
		t.Errorf("flags: %v", p)
	}

	// Zweiter Aufruf kommt aus dem Zwischenspeicher
	runSource(t, url.Values{"arch": {"apollolake"}, "language": {"enu"}})
	if calls != 1 {
		t.Errorf("GitHub %d-mal gefragt, erwartet 1", calls)
	}

	for _, arch := range []string{"rtd1296", "armada38x", "alpine4k", "evansport"} {
		if n := len(runSource(t, url.Values{"arch": {arch}})["packages"].([]any)); n != 0 {
			t.Errorf("%s bekam ein Paket", arch)
		}
	}
	if n := len(runSource(t, url.Values{"build": {"25556"}})["packages"].([]any)); n != 0 {
		t.Error("DSM 6 bekam ein Paket")
	}
}

func TestCompareVersion(t *testing.T) {
	if compareVersion("1.0.0-0010", "1.0.0-0009") <= 0 || compareVersion("1.2.0-0001", "1.10.0-0001") >= 0 || compareVersion("1.0.0-0003", "1.0.0-0003") != 0 {
		t.Error("compareVersion")
	}
}
