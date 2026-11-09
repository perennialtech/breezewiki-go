package server

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/perennialtech/breezewiki-go/internal/config"
	"github.com/perennialtech/breezewiki-go/internal/fandom"
)

func mockServer(t *testing.T, settings map[string]string, upstream http.HandlerFunc) (*Server, *httptest.Server) {
	t.Helper()
	c := config.New(settings)
	remote := httptest.NewServer(upstream)
	f := fandom.New(c)
	f.HTTP = remote.Client()
	f.Origin = func(string) string { return remote.URL }
	s := New(c, f)
	t.Cleanup(remote.Close)
	return s, remote
}
func response(s *Server, method, target string, body io.Reader, headers map[string]string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, target, body)
	for k, v := range headers {
		r.Header.Set(k, v)
	}
	out := httptest.NewRecorder()
	s.ServeHTTP(out, r)
	return out
}
func upstreamAPI(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	w.Header().Set("Content-Type", "application/json")
	if q.Get("meta") == "siteinfo" {
		_, _ = io.WriteString(w, `{"query":{"general":{"sitename":"Test Wiki","base":"/wiki/Start_Page"},"rightsinfo":{"text":"CC BY-SA","url":"https://license.invalid/"}}}`)
		return
	}
	if q.Get("action") == "parse" {
		if q.Get("page") == "Missing" {
			_, _ = io.WriteString(w, `{"error":{"code":"missingtitle"}}`)
			return
		}
		if q.Get("page") == "Forbidden" {
			w.WriteHeader(403)
			_, _ = io.WriteString(w, "blocked")
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"parse": map[string]any{
			"title": q.Get("page"), "headhtml": `<body class="theme-fandomdesktop-dark"/>`,
			"text":     `<div class="mw-parser-output"><a href="https://other.fandom.com/wiki/Other">Hello</a></div>`,
			"sections": []any{map[string]any{"line": "Info"}},
		}})
		return
	}
	if q.Get("list") == "categorymembers" {
		_, _ = io.WriteString(w, `{"query":{"categorymembers":[{"title":"Leaf Block"},{"title":"Stone/Brick"}]}}`)
		return
	}
	if q.Get("prop") == "imageinfo|fileusage" {
		_, _ = io.WriteString(w, `{"query":{"pages":[{"imageinfo":[{"mediatype":"BITMAP","url":"https://static.wikia.nocookie.net/test/images/f/File.png","width":50,"height":25,"user":"Editor"}],"fileusage":[{"title":"Uses File"}]}]}}`)
		return
	}
	if q.Get("list") == "search" {
		_, _ = io.WriteString(w, `{"query":{"search":[{"title":"Spawner","timestamp":"2024-04-01T12:00:00Z","wordcount":55,"size":1200}]}}`)
		return
	}
	w.WriteHeader(500)
}
func TestPageRoutesStandard(t *testing.T) {
	s, _ := mockServer(t, map[string]string{"feature_jsonp::enabled": "false"}, upstreamAPI)
	for _, tc := range []struct{ url, contains string }{
		{"/test/wiki/Sample_Page", "Sample_Page"},
		{"/test/wiki/Category:Blocks", "All Pages in Category:Blocks"},
		{"/test/wiki/File:File.png", "Download original file"},
		{"/test/search?q=spawner", "Spawner"},
	} {
		t.Run(tc.url, func(t *testing.T) {
			got := response(s, "GET", tc.url, nil, nil)
			if got.Code != 200 {
				t.Fatalf("status %d: %s", got.Code, got.Body.String())
			}
			if !strings.Contains(got.Body.String(), tc.contains) {
				t.Fatalf("missing %q in %s", tc.contains, got.Body.String())
			}
			if got.Header().Get("Referrer-Policy") != "same-origin" {
				t.Error("referrer policy")
			}
		})
	}
	if got := response(s, "GET", "/test/wiki/Missing", nil, nil); got.Code != 404 {
		t.Fatalf("missingtitle %d", got.Code)
	}
	if got := response(s, "GET", "/test/wiki/Forbidden", nil, nil); got.Code != 503 || got.Header().Get("Retry-After") != "30" {
		t.Fatalf("Fandom denial %d %v", got.Code, got.Header())
	}
}
func TestClientThemeAndRedirects(t *testing.T) {
	var cookie string
	s, _ := mockServer(t, map[string]string{"feature_jsonp::enabled": "false"}, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("meta") == "siteinfo" {
			upstreamAPI(w, r)
			return
		}
		cookie = r.Header.Get("Cookie")
		upstreamAPI(w, r)
	})
	got := response(s, "GET", "/test/wiki/Page", nil, map[string]string{"Cookie": "theme=dark"})
	if got.Code != 200 || cookie != "theme=dark" {
		t.Fatalf("theme cookie %s, status %d", cookie, got.Code)
	}
	redir := response(s, "GET", "/test", nil, nil)
	if redir.Code != 302 || redir.Header().Get("Location") != "/test/wiki/Start_Page" {
		t.Fatalf("wiki redirect: %v", redir.Header())
	}
	global := response(s, "GET", "/search?wikiname=test&q=some+search", nil, nil)
	if global.Code != 302 || global.Header().Get("Location") != "/test/search?q=some+search" {
		t.Fatalf("search redirect %v", global.Header())
	}
	if got := response(s, "GET", "/search", nil, nil); got.Code != 400 {
		t.Fatalf("missing wiki %d", got.Code)
	}
}
func TestJSONPAndPost(t *testing.T) {
	s, _ := mockServer(t, nil, upstreamAPI)
	got := response(s, "GET", "/test/wiki/Example", nil, nil)
	if got.Code != 200 {
		t.Fatal(got.Code)
	}
	for _, text := range []string{"proxy.wikipage", "proxy.siteinfo", "data-jsonp-var", "jsonp.js", "BWData.path"} {
		if !strings.Contains(got.Body.String(), text) {
			t.Errorf("missing JSONP %q", text)
		}
	}
	post := map[string]any{"wikiname": "test", "path": "Example",
		"wikipage": map[string]any{"parse": map[string]any{"title": "Example", "text": "<p>Uploaded</p>"}},
		"siteinfo": fandom.DefaultSiteInfo}
	body, _ := json.Marshal(post)
	bad := response(s, "POST", "/test/wiki/Example", bytes.NewReader(body), nil)
	if bad.Code != 400 || !strings.Contains(bad.Body.String(), "cross-origin") {
		t.Fatalf("POST origin %d: %s", bad.Code, bad.Body.String())
	}
	ok := response(s, "POST", "/test/wiki/Example", bytes.NewReader(body), map[string]string{"Origin": "https://example.com"})
	if ok.Code != 200 || !strings.Contains(ok.Body.String(), "Uploaded") {
		t.Fatalf("POST page %d: %s", ok.Code, ok.Body.String())
	}
	s.Config = config.New(map[string]string{"canonical_origin": "https://breezewiki.example"})
	wrong := response(s, "POST", "/test/wiki/Example", bytes.NewReader(body), map[string]string{"Origin": "https://attacker.example"})
	if wrong.Code != 400 {
		t.Fatalf("cross-origin allowed %d", wrong.Code)
	}
	for _, forged := range []string{"https://breezewiki.example.evil", "https://breezewiki.example@evil.test", "http://breezewiki.example", "null"} {
		wrong := response(s, "POST", "/test/wiki/Example", bytes.NewReader(body), map[string]string{"Origin": forged})
		if wrong.Code != 400 {
			t.Fatalf("cross-origin %q allowed: %d", forged, wrong.Code)
		}
	}
	canonical := response(s, "POST", "/test/wiki/Example", bytes.NewReader(body), map[string]string{"Origin": "https://breezewiki.example"})
	if canonical.Code != 200 {
		t.Fatalf("canonical POST denied: %d", canonical.Code)
	}
}
func TestStatusStaticSettingsSubdomain(t *testing.T) {
	s, _ := mockServer(t, nil, upstreamAPI)
	s.Config = config.New(map[string]string{"canonical_origin": "https://mirror.example"})
	got := response(s, "GET", "http://test.mirror.example/wiki/Hello", nil, nil)
	if got.Code != 302 || got.Header().Get("Location") != "https://mirror.example/test/wiki/Hello" {
		t.Fatalf("subdomain %d %v", got.Code, got.Header())
	}
	got = response(s, "GET", "/set-user-settings?next_location=%2Ftest%2Fwiki%2FPage&new_settings=%23s%28user-cookies%5E+dark+%28minecraft+zelda%29%29", nil, nil)
	if got.Code != 302 || got.Header().Get("Location") != "/test/wiki/Page" {
		t.Fatalf("settings redirect %d: %s", got.Code, got.Body.String())
	}
	if cookies := got.Result().Cookies(); len(cookies) != 2 || cookies[0].Value != "dark" || cookies[1].Value != "minecraft|zelda" {
		t.Fatalf("settings cookies %v", cookies)
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "main.css"), []byte("css"), 0644); err != nil {
		t.Fatal(err)
	}
	s.StaticDir = dir
	got = response(s, "GET", "/static/main.css", nil, nil)
	if got.Code != 200 || got.Body.String() != "css" || got.Header().Get("Access-Control-Allow-Origin") != "*" {
		t.Fatalf("static %d %v", got.Code, got.Header())
	}
	if got := response(s, "GET", "/test/unknown-route", nil, nil); got.Code != 404 {
		t.Fatalf("unknown path %d", got.Code)
	}
}
func TestStrictProxyPage(t *testing.T) {
	s, _ := mockServer(t, map[string]string{"strict_proxy": "true", "feature_jsonp::enabled": "false"}, upstreamAPI)
	got := response(s, "GET", "/test/wiki/Page", nil, nil)
	if got.Code != 200 || !strings.Contains(got.Body.String(), `/proxy?dest=https%3A%2F%2Ftest.fandom.com`) ||
		!strings.Contains(got.Body.String(), `BWData.strict_proxy = true`) {
		t.Fatalf("strict proxy page %d: %s", got.Code, got.Body.String())
	}
}
