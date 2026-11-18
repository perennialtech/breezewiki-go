package server

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestGenshinNavigationAndLanguages(t *testing.T) {
	nav := map[string]any{"query": map[string]any{"allmessages": []any{map[string]any{
		"content": "*Character|Characters\n**Character/List|Character list\n**|Guides\n***Guide/Part #Details|A & B",
	}}}}
	article := map[string]any{"parse": map[string]any{
		"title": "Genshin Impact Wiki", "text": `<p>Home content</p>`,
		"langlinks": []map[string]string{
			{"autonym": "日本語", "url": "https://genshin-impact.fandom.com/ja/wiki/"},
			{"autonym": "Italiano", "url": "https://genshinimpact.fandom.com/it/wiki/"},
		},
	}}
	info := map[string]any{"query": map[string]any{"general": map[string]string{
		"base": "https://genshin-impact.fandom.com/wiki/Genshin_Impact_Wiki",
	}}}
	for _, jsonp := range []bool{false, true} {
		mode := "false"
		if jsonp {
			mode = "true"
		}
		t.Run(mode, func(t *testing.T) {
			s, _ := mockServer(t, map[string]string{"feature_jsonp::enabled": mode}, func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Query().Get("meta") {
				case "allmessages":
					_ = json.NewEncoder(w).Encode(nav)
				case "siteinfo":
					_ = json.NewEncoder(w).Encode(info)
				default:
					_ = json.NewEncoder(w).Encode(article)
				}
			})
			const path = "/genshin-impact/wiki/Genshin_Impact_Wiki"
			res := response(s, "GET", path, nil, nil)
			if jsonp {
				if !strings.Contains(res.Body.String(), "callback=proxy.navigation") {
					t.Fatal("navigation is missing from the JSONP dependencies")
				}
				post := jsonString(map[string]any{"wikiname": "genshin-impact", "path": "Genshin_Impact_Wiki",
					"wikipage": article, "siteinfo": info, "navigation": nav})
				res = response(s, "POST", path, strings.NewReader(post), map[string]string{"Origin": "https://example.com"})
			}
			for _, want := range []string{
				`/genshin-impact/wiki/Character/List`, `/genshin-impact/wiki/Guide/Part#Details`, `A &amp; B`,
				`/genshin-impact/ja/wiki/`, `/genshinimpact/it/wiki/`, `/genshin-impact/f`,
				`/genshin-impact/wiki/Genshin_Impact_Wiki?action=raw`, `Home content`,
			} {
				if res.Code != 200 || !strings.Contains(res.Body.String(), want) {
					t.Errorf("status %d missing %q", res.Code, want)
				}
			}
		})
	}
}

func TestHomepageReadViews(t *testing.T) {
	s, _ := mockServer(t, nil, func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		switch {
		case r.URL.Path == "/wiki/Special:NewPages":
			_, _ = io.WriteString(w, `<h1 class="page-header__title">New pages</h1><div id="mw-content-text">
			<form action="/wiki/Special:NewPages" method="get"><input name="limit"></form>
			<a href="/index.php?title=Special%3ANewPages&amp;offset=next">More</a>
			<a href="/wiki/Quest">Quest</a><script>excluded-script</script>
			<img src="https://static.wikia.nocookie.net/image.png" onerror="excluded-handler">
			<a href="javascript:excluded-url">Unsafe</a></div>`)
		case r.URL.Path == "/f":
			_, _ = io.WriteString(w, `<main class="home-page-desktop__main"><article class="standard-post">
				<section><a href="/f/p/123">A wiki discussion</a></section><footer>Excluded footer</footer>
			</article></main>`)
		case q.Get("action") == "compare":
			if q.Get("fromrev") != "100" || q.Get("torev") != "200" {
				t.Errorf("comparison received %s", q.Encode())
			}
			_, _ = io.WriteString(w, `{"compare":{"fromtitle":"Quest","totitle":"Quest","fromrevid":100,"torevid":200,"body":"<tr><td>Earlier</td><td>Later</td></tr>"}}`)
		case q.Get("list") == "recentchanges":
			_, _ = io.WriteString(w, `{"continue":{"rccontinue":"next|1"},"query":{"recentchanges":[{"title":"Quest","revid":200,"old_revid":100,"timestamp":"2026-11-18T08:00:00Z","user":"Editor"}]}}`)
		default:
			upstreamAPI(w, r)
		}
	})
	for _, tc := range []struct {
		path string
		want []string
	}{
		{"/wiki/Special:NewPages", []string{`action="/genshin-impact/wiki/Special:NewPages"`, `href="/genshin-impact/wiki/Special:NewPages?offset=next"`, `href="/genshin-impact/wiki/Quest"`}},
		{"/f", []string{`href="/genshin-impact/f/p/123"`, "A wiki discussion"}},
		{"/wiki/Quest?oldid=100&diff=200", []string{`Quest?oldid=100"`, `<tr><td>Earlier</td><td>Later</td></tr>`}},
		{"/wiki/Special:RecentChanges", []string{`Quest?oldid=100&amp;diff=200"`, `?rccontinue=next%7C1"`, `Special:Contributions/Editor"`}},
	} {
		res := response(s, "GET", "/genshin-impact"+tc.path, nil, nil)
		for _, text := range tc.want {
			if res.Code != 200 || !strings.Contains(res.Body.String(), text) {
				t.Errorf("%s status %d missing %q", tc.path, res.Code, text)
			}
		}
		for _, text := range []string{"excluded-script", "excluded-handler", "excluded-url", "Excluded footer"} {
			if strings.Contains(res.Body.String(), text) {
				t.Errorf("%s retained %q", tc.path, text)
			}
		}
	}
	res := response(s, "GET", "/genshin-impact/wiki/Quest?action=edit&section=2", nil, nil)
	if res.Code != http.StatusFound || res.Header().Get("Location") != "https://genshin-impact.fandom.com/wiki/Quest?action=edit&section=2" {
		t.Fatalf("edit handoff %d %s", res.Code, res.Header().Get("Location"))
	}
}
