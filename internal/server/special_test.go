package server

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/perennialtech/breezewiki-go/internal/fandom"
)

func TestSpecialPages(t *testing.T) {
	var from string
	s, _ := mockServer(t, map[string]string{"feature_jsonp::enabled": "false"}, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Query().Get("list") {
		case "random":
			q := r.URL.Query()
			if q.Get("rnnamespace") != "0" || q.Get("rnlimit") != "1" {
				t.Errorf("random params: %s", r.URL.RawQuery)
			}
			_, _ = io.WriteString(w, `{"query":{"random":[{"title":"A & B/Page"}]}}`)
		case "allpages":
			q := r.URL.Query()
			if q.Get("apnamespace") != "0" || q.Get("aplimit") != "max" {
				t.Errorf("allpages params: %s", r.URL.RawQuery)
			}
			from = q.Get("apcontinue")
			if from == "" {
				_, _ = io.WriteString(w, `{"query":{"allpages":[{"title":"A & B"},{"title":"Category/One"}]},"continue":{"apcontinue":"Next Page"}}`)
			} else {
				_, _ = io.WriteString(w, `{"query":{"allpages":[{"title":"Next Page"}]}}`)
			}
		default:
			upstreamAPI(w, r)
		}
	})

	random := response(s, "GET", "/test/wiki/Special:Random", nil, nil)
	if random.Code != 302 || random.Header().Get("Location") != "/test/wiki/A_&_B/Page" {
		t.Fatalf("random redirect %d: %s", random.Code, random.Header().Get("Location"))
	}
	all := response(s, "GET", "/test/wiki/Special:AllPages", nil, nil)
	for _, want := range []string{`href="/test/wiki/A_&amp;_B"`, `href="/test/wiki/Category/One"`, `href="/test/wiki/Special:AllPages?from=Next+Page"`} {
		if all.Code != 200 || !strings.Contains(all.Body.String(), want) {
			t.Errorf("allpages %d missing %q", all.Code, want)
		}
	}
	second := response(s, "GET", "/test/wiki/Special:AllPages?from=Next+Page", nil, nil)
	if second.Code != 200 || from != "Next Page" || strings.Contains(second.Body.String(), "Next page</a>") {
		t.Errorf("allpages continuation %d, from %q", second.Code, from)
	}
	article := response(s, "GET", "/test/wiki/Example", nil, nil)
	if !strings.Contains(article.Body.String(), `href="/test/wiki/Special:AllPages">All pages</a>`) {
		t.Error("page-index link is not discoverable on articles")
	}
}

func TestRandomJSONP(t *testing.T) {
	s, _ := mockServer(t, nil, func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("random page should load through browser JSONP, got %s", r.URL)
	})
	page := response(s, "GET", "/test/wiki/Special:Random", nil, nil)
	for _, want := range []string{"showRandomPage", "callback=showRandomPage", "rnnamespace=0", "rnlimit=1", "location.replace"} {
		if page.Code != 200 || !strings.Contains(page.Body.String(), want) {
			t.Errorf("random JSONP %d missing %q", page.Code, want)
		}
	}
}

func TestAllPagesJSONP(t *testing.T) {
	s, _ := mockServer(t, nil, upstreamAPI)
	page := response(s, "GET", "/test/wiki/Special:AllPages?from=B", nil, nil)
	if page.Code != 200 || !strings.Contains(page.Body.String(), "proxy.allpages") || !strings.Contains(page.Body.String(), "apcontinue=B") {
		t.Fatalf("JSONP allpages %d", page.Code)
	}
	post := map[string]any{
		"wikiname": "test", "path": "Special:AllPages",
		"allpages": map[string]any{"query": map[string]any{"allpages": []any{map[string]any{"title": "Page B"}}}},
		"siteinfo": fandom.DefaultSiteInfo,
	}
	data, _ := json.Marshal(post)
	result := response(s, "POST", "/test/wiki/Special:AllPages?from=B", bytes.NewReader(data), map[string]string{"Origin": "https://example.com"})
	if result.Code != 200 || !strings.Contains(result.Body.String(), `href="/test/wiki/Page_B"`) {
		t.Fatalf("JSONP POST allpages %d: %s", result.Code, result.Body.String())
	}
}

func TestPercentInSpecialPageTitles(t *testing.T) {
	s, _ := mockServer(t, map[string]string{"feature_jsonp::enabled": "false"}, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Query().Get("list") {
		case "random":
			_, _ = io.WriteString(w, `{"query":{"random":[{"title":"100%"}]}}`)
		case "allpages":
			_, _ = io.WriteString(w, `{"query":{"allpages":[{"title":"100%"}]}}`)
		default:
			upstreamAPI(w, r)
		}
	})
	random := response(s, "GET", "/test/wiki/Special:Random", nil, nil)
	if random.Code != 302 || random.Header().Get("Location") != "/test/wiki/100%25" {
		t.Errorf("random title %% redirect: %d %q", random.Code, random.Header().Get("Location"))
	}
	all := response(s, "GET", "/test/wiki/Special:AllPages", nil, nil)
	if all.Code != 200 || !strings.Contains(all.Body.String(), `href="/test/wiki/100%25"`) {
		t.Errorf("allpages title %% link: %d", all.Code)
	}
}
