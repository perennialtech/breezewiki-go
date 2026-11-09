package server

import (
	"encoding/json"
	"regexp"
	"strings"
	"testing"
)

func TestVersionedStaticAssetsMatchDocumentAndPreloads(t *testing.T) {
	s, _ := mockServer(t, map[string]string{"feature_jsonp::enabled": "false"}, upstreamAPI)
	got := response(s, "GET", "/test/wiki/Example", nil, nil)
	if got.Code != 200 {
		t.Fatalf("article status %d", got.Code)
	}
	page := got.Body.String()
	for _, name := range []string{"main.css", "preact.js", "tabs.js", "countdown.js", "search-suggestions.js", "breezewiki.svg", "breezewiki-favicon.svg"} {
		want := s.staticURL(name)
		if !strings.Contains(want, "?t=") {
			t.Fatalf("unversioned server asset: %s", want)
		}
		if !strings.Contains(page, want) {
			t.Errorf("article lacks versioned asset %s", want)
		}
	}
	if !strings.Contains(got.Header().Get("Link"), "<"+s.staticURL("main.css")+">") ||
		!strings.Contains(got.Header().Get("Link"), "<"+s.staticURL("preact.js")+">") ||
		!strings.Contains(got.Header().Get("Link"), "<"+s.staticURL("source-sans-pro-v21-vietnamese_latin-ext_latin_greek-ext_greek_cyrillic-ext_cyrillic-regular.woff2")+">") {
		t.Fatalf("preload and document URLs diverge: %s", got.Header().Get("Link"))
	}
	raw := regexp.MustCompile(`<script type="importmap">([^<]+)</script>`).FindStringSubmatch(page)
	if raw == nil {
		t.Fatal("missing import map")
	}
	var imports struct {
		Imports map[string]string `json:"imports"`
	}
	if err := json.Unmarshal([]byte(raw[1]), &imports); err != nil {
		t.Fatal(err)
	}
	for key, val := range imports.Imports {
		if !strings.HasPrefix(val, "/static/") || !strings.Contains(val, "?t=") {
			t.Errorf("unversioned module %s: %s", key, val)
		}
	}
	asset := response(s, "GET", s.staticURL("main.css"), nil, nil)
	if asset.Code != 200 || len(asset.Body.Bytes()) == 0 {
		t.Fatalf("versioned file unavailable: %d", asset.Code)
	}
	home := response(s, "GET", "/", nil, nil)
	if !strings.Contains(home.Body.String(), s.staticURL("main.css")) {
		t.Fatal("homepage uses stale CSS URL")
	}
}
