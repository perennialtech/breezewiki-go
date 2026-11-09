package fandom

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/perennialtech/breezewiki-go/internal/config"
)

func TestProxyAllowlist(t *testing.T) {
	for _, u := range []string{
		"https://static.wikia.nocookie.net/test/images/A.png",
		"https://minecraft.fandom.com/wiki/Dirt",
		"https://a-b.fandom.com/",
	} {
		if !AllowedProxyURL(u) {
			t.Errorf("should allow %q", u)
		}
	}
	for _, u := range []string{
		"http://minecraft.fandom.com/wiki/Dirt",
		"https://minecraft.fandom.com.evil.test/wiki/Dirt",
		"https://evil.test/?url=https://minecraft.fandom.com/",
		"https://other.test/a",
		"https://minecraft.fandom.com@evil.test/wiki/A",
	} {
		if AllowedProxyURL(u) {
			t.Errorf("should reject %q", u)
		}
	}
}
func TestAPIAndSiteInfoCache(t *testing.T) {
	var count atomic.Int32
	var theme string
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count.Add(1)
		theme = r.Header.Get("Cookie")
		if r.URL.Query().Get("meta") == "siteinfo" {
			_, _ = io.WriteString(w, `{"query":{"general":{"sitename":"Example Wiki","base":"/wiki/Front_Page"},"rightsinfo":{"text":"CC","url":"https://example.test"}}}`)
			return
		}
		if r.URL.Query().Get("page") != "Foo & Bar" {
			t.Errorf("page query: %q", r.URL.Query().Get("page"))
		}
		_, _ = io.WriteString(w, `{"parse":{"title":"Foo & Bar","text":"hello"}}`)
	}))
	defer remote.Close()
	c := New(config.New(map[string]string{"log_outgoing": "false"}))
	c.HTTP = remote.Client()
	c.Origin = func(string) string { return remote.URL }
	ctx := context.Background()
	res, err := c.API(ctx, "test", "", []Param{{"action", "parse"}, {"page", "Foo & Bar"}}, "dark")
	if err != nil || res.Status != 200 || theme != "theme=dark" {
		t.Fatalf("API %v %+v theme=%q", err, res, theme)
	}
	info := c.SiteInfo(ctx, "test", "")
	name, base := General(info)
	if name != "Example Wiki" || base != "Front_Page" {
		t.Fatalf("siteinfo %s %s", name, base)
	}
	_ = c.SiteInfo(ctx, "test", "")
	if count.Load() != 2 {
		t.Fatalf("siteinfo wasn't cached: %d calls", count.Load())
	}
}
func TestLanguageAPIAndSiteInfoCache(t *testing.T) {
	var paths []string
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		if r.URL.Query().Get("meta") == "siteinfo" {
			_, _ = fmt.Fprintf(w, `{"query":{"general":{"sitename":%q,"base":%q}}}`, r.URL.Path, r.URL.Path)
			return
		}
		_, _ = io.WriteString(w, `{"parse":{"title":"Page"}}`)
	}))
	defer remote.Close()
	c := New(config.New(nil))
	c.HTTP = remote.Client()
	c.Origin = func(string) string { return remote.URL }
	ctx := context.Background()
	if _, err := c.API(ctx, "test", "es", []Param{{"action", "parse"}, {"page", "Page"}}, ""); err != nil {
		t.Fatal(err)
	}
	spanish := c.SiteInfo(ctx, "test", "es")
	english := c.SiteInfo(ctx, "test", "")
	_ = c.SiteInfo(ctx, "test", "es")
	if len(paths) != 3 || paths[0] != "/es/api.php" || paths[1] != "/es/api.php" || paths[2] != "/api.php" {
		t.Fatalf("API paths and cache separation: %v", paths)
	}
	if String(spanish, "query", "general", "sitename") == String(english, "query", "general", "sitename") {
		t.Fatal("language variants shared siteinfo cache")
	}
}

func TestRefusalBackoff(t *testing.T) {
	var count atomic.Int32
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count.Add(1)
		w.WriteHeader(http.StatusForbidden)
		_, _ = fmt.Fprint(w, "blocked")
	}))
	defer remote.Close()
	c := New(config.New(map[string]string{"log_outgoing": "false"}))
	c.HTTP = remote.Client()
	c.Origin = func(string) string { return remote.URL }
	ctx := context.Background()
	for _, wiki := range []string{"alpha", "beta"} {
		res, err := c.Get(ctx, wiki, "/wiki/Page", "")
		if err != nil || res.Status != 403 {
			t.Fatalf("refusal: %d %v", res.Status, err)
		}
	}
	if count.Load() != 1 {
		t.Errorf("expected cross-wiki 30-second refusal cache; got %d calls", count.Load())
	}
}
