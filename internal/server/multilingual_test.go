package server

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/perennialtech/breezewiki-go/internal/fandom"
)

func TestMultilingualRoutes(t *testing.T) {
	var paths []string
	s, _ := mockServer(t, map[string]string{
		"feature_jsonp::enabled": "false", "canonical_origin": "https://bw.example",
	}, func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		if r.URL.Query().Get("meta") == "siteinfo" {
			if r.URL.Path == "/es/api.php" {
				_, _ = io.WriteString(w, `{"query":{"general":{"sitename":"Spanish Wiki","base":"https://mlp.fandom.com/es/wiki/Inicio"}}}`)
			} else {
				_, _ = io.WriteString(w, `{"query":{"general":{"sitename":"English Wiki","base":"/wiki/Home"}}}`)
			}
			return
		}
		if r.URL.Query().Get("action") == "parse" && r.URL.Path == "/es/api.php" {
			if r.URL.Query().Get("page") == "Missing" {
				_, _ = io.WriteString(w, `{"error":{"code":"missingtitle"}}`)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"parse": map[string]any{
				"title": r.URL.Query().Get("page"),
				"text": `<a href="/es/wiki/Local">Local</a><a href="/wiki/Implicit">Implicit</a>` +
					`<a href="https://mlp.fandom.com/es/wiki/Another">Another</a>`,
			}})
			return
		}
		upstreamAPI(w, r)
	})

	page := response(s, "GET", "/mlp/es/wiki/Page_One", nil, nil)
	for _, want := range []string{
		`href="/mlp/es/wiki/Local"`, `href="/mlp/es/wiki/Implicit"`, `href="/mlp/es/wiki/Another"`,
		`action="/mlp/es/search"`, `href="https://bw.example/mlp/es/wiki/Page_One"`,
		`https://mlp.fandom.com/es/wiki/Page_One`, `"lang":"es"`, `BWData.path = "Page_One"`,
		`https://mlp.fandom.com/es/wikia.php?controller=ThemeApi`, `https://mlp.fandom.com/es/load.php?lang=es`,
	} {
		if page.Code != 200 || !strings.Contains(page.Body.String(), want) {
			t.Errorf("language article %d missing %q", page.Code, want)
		}
	}
	redirect := response(s, "GET", "/mlp/es/wiki", nil, nil)
	if redirect.Code != 302 || redirect.Header().Get("Location") != "/mlp/es/wiki/Inicio" {
		t.Fatalf("language homepage redirect: %d %s", redirect.Code, redirect.Header().Get("Location"))
	}
	category := response(s, "GET", "/mlp/es/wiki/Category:Friends", nil, nil)
	if category.Code != 200 || !strings.Contains(category.Body.String(), `href="/mlp/es/wiki/Leaf_Block"`) {
		t.Errorf("language category: %d", category.Code)
	}
	file := response(s, "GET", "/mlp/es/wiki/File:Example.png", nil, nil)
	if file.Code != 200 || !strings.Contains(file.Body.String(), `href="/mlp/es/wiki/User:Editor"`) {
		t.Errorf("language file: %d", file.Code)
	}
	search := response(s, "GET", "/mlp/es/search?q=spawner", nil, nil)
	if search.Code != 200 || !strings.Contains(search.Body.String(), `href="/mlp/es/wiki/Spawner"`) {
		t.Errorf("language search: %d", search.Code)
	}
	if missing := response(s, "GET", "/mlp/es/wiki/Missing", nil, nil); missing.Code != 404 {
		t.Errorf("language missingtitle: %d", missing.Code)
	}
	english := response(s, "GET", "/mlp/wiki/Page", nil, nil)
	if english.Code != 200 || !strings.Contains(english.Body.String(), `action="/mlp/search"`) {
		t.Errorf("standard route changed: %d", english.Code)
	}
	if unknown := response(s, "GET", "/mlp/unknown-route", nil, nil); unknown.Code != 404 {
		t.Errorf("unknown route: %d", unknown.Code)
	}
	seenEnglish, seenSpanish := false, false
	for _, path := range paths {
		seenEnglish = seenEnglish || path == "/api.php"
		seenSpanish = seenSpanish || path == "/es/api.php"
	}
	if !seenEnglish || !seenSpanish {
		t.Errorf("API paths: %v", paths)
	}
}

func TestMultilingualJSONP(t *testing.T) {
	s, _ := mockServer(t, nil, upstreamAPI)
	page := response(s, "GET", "/mlp/es/wiki/Page", nil, nil)
	if page.Code != 200 || !strings.Contains(page.Body.String(), "mlp.fandom.com/es/api.php?") ||
		!strings.Contains(page.Body.String(), `"lang":"es"`) || !strings.Contains(page.Body.String(), `BWData.path = "Page"`) {
		t.Fatalf("language JSONP GET: %d", page.Code)
	}
	post := map[string]any{
		"wikiname": "mlp", "path": "Page",
		"wikipage": map[string]any{"parse": map[string]any{"title": "Page", "text": `<a href="/es/wiki/Friend">Friend</a>`}},
		"siteinfo": fandom.DefaultSiteInfo,
	}
	data, _ := json.Marshal(post)
	result := response(s, "POST", "/mlp/es/wiki/Page", bytes.NewReader(data), map[string]string{"Origin": "https://example.com"})
	if result.Code != 200 || !strings.Contains(result.Body.String(), `href="/mlp/es/wiki/Friend"`) {
		t.Fatalf("language JSONP POST: %d", result.Code)
	}
}

func TestLocalizedCanonicalURLPreservesEncodedTitle(t *testing.T) {
	s, _ := mockServer(t, map[string]string{"canonical_origin": "https://mirror.example", "feature_jsonp::enabled": "false"}, upstreamAPI)
	got := response(s, "GET", "/mlp/es/wiki/What%3F", nil, nil)
	if got.Code != 200 || !strings.Contains(got.Body.String(), `rel="canonical" href="https://mirror.example/mlp/es/wiki/What%3F"`) {
		t.Fatalf("canonical title encoding missing: %d %s", got.Code, got.Body.String())
	}
}

func TestLocalizedCategoryAndFileNamespaces(t *testing.T) {
	s, _ := mockServer(t, map[string]string{"feature_jsonp::enabled": "false"}, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("meta") == "siteinfo" {
			_, _ = io.WriteString(w, `{"query":{"general":{"sitename":"Spanish Wiki","base":"/es/wiki/Inicio"},"namespaces":{"6":{"id":6,"name":"Archivo","canonical":"File"},"14":{"id":14,"name":"Categoría","canonical":"Category"}}}}`)
			return
		}
		upstreamAPI(w, r)
	})
	category := response(s, "GET", "/mlp/es/wiki/Categor%C3%ADa:Alicornio", nil, nil)
	if category.Code != 200 || !strings.Contains(category.Body.String(), "All Pages in") || !strings.Contains(category.Body.String(), "Leaf_Block") {
		t.Fatalf("localized category did not show members: %d", category.Code)
	}
	file := response(s, "GET", "/mlp/es/wiki/Archivo:File.png", nil, nil)
	if file.Code != 200 || !strings.Contains(file.Body.String(), "Download original file") {
		t.Fatalf("localized file omitted metadata: %d", file.Code)
	}
}

func TestLocalizedJSONPUsesBrowserSiteInfoForNamespaces(t *testing.T) {
	calls := 0
	s, _ := mockServer(t, nil, func(w http.ResponseWriter, r *http.Request) { calls++; w.WriteHeader(http.StatusServiceUnavailable) })
	for _, route := range []string{"/mlp/es/wiki/Archivo:Example.png", "/mlp/es/wiki/Categor%C3%ADa:Alicornio"} {
		got := response(s, "GET", route, nil, nil)
		if got.Code != 200 || calls != 0 {
			t.Fatalf("GET %s: HTTP %d, server-side API calls %d", route, got.Code, calls)
		}
		if !strings.Contains(got.Body.String(), "localizedExtraEndpoints") {
			t.Errorf("missing browser-side namespace lookup: %s", route)
		}
	}
	siteinfo := map[string]any{"query": map[string]any{"namespaces": map[string]any{
		"6":  map[string]any{"id": 6, "name": "Archivo", "canonical": "File"},
		"14": map[string]any{"id": 14, "name": "Categoría", "canonical": "Category"},
	}}}
	for _, tc := range []struct {
		title, extra string
		payload      map[string]any
	}{
		{"Archivo:Example.png", "Download original file", map[string]any{"imageinfo": map[string]any{"query": map[string]any{"pages": []any{map[string]any{"imageinfo": []any{map[string]any{"url": "https://static.wikia.nocookie.net/test/images/a/aa/P.png", "mediatype": "BITMAP", "user": "Editor"}}}}}}}},
		{"Categoría:Alicornio", "All Pages in", map[string]any{"membersdata": map[string]any{"query": map[string]any{"categorymembers": []any{map[string]any{"title": "Leaf Block"}}}}}},
	} {
		post := map[string]any{"wikiname": "mlp", "path": tc.title, "wikipage": map[string]any{"parse": map[string]any{"title": tc.title, "text": "<p>Example</p>"}}, "siteinfo": siteinfo}
		for name, value := range tc.payload {
			post[name] = value
		}
		blob, _ := json.Marshal(post)
		got := response(s, "POST", "/mlp/es/wiki/"+url.PathEscape(tc.title), bytes.NewReader(blob), map[string]string{"Origin": "https://example.com"})
		if got.Code != 200 || !strings.Contains(got.Body.String(), tc.extra) {
			t.Errorf("localized JSONP %s: HTTP %d missing %q", tc.title, got.Code, tc.extra)
		}
	}
}

func TestLocalizedCanonicalNamespacesAndEscapedSourceLink(t *testing.T) {
	siteinfo := map[string]any{"query": map[string]any{"namespaces": map[string]any{"6": map[string]any{"id": 6, "name": "Archivo", "canonical": "File"}, "14": map[string]any{"id": 14, "name": "Categoría", "canonical": "Category"}}}}
	if got := namespaceKind(siteinfo, "file:Example.png"); got != "file" {
		t.Errorf("lowercase canonical file namespace: %q", got)
	}
	if got := sourceURL("mlp", "es", "Hot_Dog...?"); got != "https://mlp.fandom.com/es/wiki/Hot_Dog...%3F" {
		t.Errorf("source URL: %q", got)
	}
}

func TestLocalizedHomepageEscapedTitle(t *testing.T) {
	s, _ := mockServer(t, nil, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("meta") == "siteinfo" {
			_, _ = io.WriteString(w, `{"query":{"general":{"sitename":"Test Wiki","base":"https://mlp.fandom.com/es/wiki/P%C3%A1gina_principal"}}}`)
			return
		}
		upstreamAPI(w, r)
	})
	got := response(s, "GET", "/mlp/es/wiki", nil, nil)
	want := "/mlp/es/wiki/P%C3%A1gina_principal"
	if got.Code != 302 || got.Header().Get("Location") != want {
		t.Fatalf("localized home redirect: %d %q; want %q", got.Code, got.Header().Get("Location"), want)
	}
}

func TestLocalizedCommentsUseLanguageEndpoint(t *testing.T) {
	var commentsPath string
	s, _ := mockServer(t, map[string]string{"feature_jsonp::enabled": "false"}, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("controller") == "ArticleCommentsController" {
			commentsPath = r.URL.Path
			_ = json.NewEncoder(w).Encode(commentFixture())
			return
		}
		upstreamAPI(w, r)
	})
	article := response(s, "GET", "/mlp/es/wiki/Twilight_Sparkle", nil, nil)
	if article.Code != 200 || !strings.Contains(article.Body.String(), `data-comments-url="/mlp/es/comments?title=Twilight_Sparkle"`) {
		t.Fatalf("localized article missing matching comments endpoint: HTTP %d; has local endpoint=%v", article.Code, strings.Contains(article.Body.String(), `/mlp/es/comments`))
	}
	result := response(s, "GET", "/mlp/es/comments?title=Twilight_Sparkle", nil, nil)
	if result.Code != 200 || commentsPath != "/es/wikia.php" || !strings.Contains(result.Body.String(), "21 comments") {
		t.Fatalf("localized comment request: HTTP %d upstream %q", result.Code, commentsPath)
	}
}
