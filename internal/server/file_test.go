package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/perennialtech/breezewiki-go/internal/fandom"
)

func TestFileDescriptionWithoutSections(t *testing.T) {
	s, _ := mockServer(t, map[string]string{"feature_jsonp::enabled": "false"}, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("action") == "parse" {
			_ = json.NewEncoder(w).Encode(map[string]any{"parse": map[string]any{
				"title":    "File:Vena.png",
				"text":     `<div class="mw-parser-output"><p>Art for Vena</p></div>`,
				"sections": []any{},
			}})
			return
		}
		upstreamAPI(w, r)
	})
	got := response(s, "GET", "/test/wiki/File:Vena.png", nil, nil)
	if got.Code != http.StatusOK || !strings.Contains(got.Body.String(), "Art for Vena") {
		t.Fatalf("unheaded file description hidden: HTTP %d", got.Code)
	}
	if strings.Contains(got.Body.String(), "This file may be copyrighted") {
		t.Fatal("generic warning substituted for a real file description")
	}
}

func TestFileWithoutDescriptionKeepsCopyrightWarning(t *testing.T) {
	s, _ := mockServer(t, map[string]string{"feature_jsonp::enabled": "false"}, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("action") == "parse" {
			_ = json.NewEncoder(w).Encode(map[string]any{"parse": map[string]any{
				"title":    "File:Empty.png",
				"text":     `<div class="mw-parser-output"><!-- NewPP limit report --></div>`,
				"sections": []any{},
			}})
			return
		}
		upstreamAPI(w, r)
	})
	got := response(s, "GET", "/test/wiki/File:Empty.png", nil, nil)
	if got.Code != http.StatusOK || !strings.Contains(got.Body.String(), "This file may be copyrighted") {
		t.Fatalf("missing warning on empty description: HTTP %d", got.Code)
	}
}

func TestFileUsageContinuation(t *testing.T) {
	var usageFrom string
	s, _ := mockServer(t, map[string]string{"feature_jsonp::enabled": "false"}, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("prop") == "imageinfo|fileusage" {
			q := r.URL.Query()
			if q.Get("fulimit") != "max" {
				t.Errorf("fileusage limit: %q", q.Get("fulimit"))
			}
			usageFrom = q.Get("fucontinue")
			payload := map[string]any{"query": map[string]any{"pages": []any{map[string]any{
				"imageinfo": []any{map[string]any{"mediatype": "BITMAP", "url": "https://static.wikia.nocookie.net/test/images/a/a/File.png", "user": "Uploader"}},
				"fileusage": []any{map[string]any{"title": "First page"}},
			}}}}
			if usageFrom == "" {
				payload["continue"] = map[string]any{"fucontinue": "usage+next & 2", "iistart": "2010-03-14T12:25:43Z", "continue": "||"}
			} else {
				payload["query"].(map[string]any)["pages"].([]any)[0].(map[string]any)["fileusage"] = []any{map[string]any{"title": "Further page"}}
			}
			_ = json.NewEncoder(w).Encode(payload)
			return
		}
		upstreamAPI(w, r)
	})
	first := response(s, "GET", "/test/wiki/File:Example.png", nil, nil)
	nextURL := `/test/wiki/File:Example.png?from=usage%2Bnext+%26+2`
	if first.Code != http.StatusOK || !strings.Contains(first.Body.String(), `href="`+nextURL+`"`) {
		t.Fatalf("next usage page not linked: HTTP %d", first.Code)
	}
	next := response(s, "GET", nextURL, nil, nil)
	if next.Code != http.StatusOK || usageFrom != "usage+next & 2" || !strings.Contains(next.Body.String(), "Further page") {
		t.Fatalf("second usage page missing: HTTP %d from %q", next.Code, usageFrom)
	}
	if strings.Contains(next.Body.String(), "More file uses") {
		t.Fatal("continuation shown after final usage page")
	}
}

func TestFileUsageJSONPContinuation(t *testing.T) {
	s, _ := mockServer(t, nil, upstreamAPI)
	page := response(s, "GET", "/test/wiki/File:Example.png?from=Next+Token", nil, nil)
	for _, want := range []string{"proxy.imageinfo", "fucontinue=Next", "fulimit=max"} {
		if page.Code != 200 || !strings.Contains(page.Body.String(), want) {
			t.Errorf("JSONP query missing %q", want)
		}
	}
	payload := map[string]any{
		"wikiname": "test", "path": "File:Example.png",
		"wikipage": map[string]any{"parse": map[string]any{"title": "File:Example.png", "sections": []any{}, "text": `<div class="mw-parser-output"><p>A description</p></div>`}},
		"imageinfo": map[string]any{"query": map[string]any{"pages": []any{map[string]any{
			"imageinfo": []any{map[string]any{"mediatype": "BITMAP", "url": "https://static.wikia.nocookie.net/test/a.png", "user": "Uploader"}},
			"fileusage": []any{map[string]any{"title": "Next page usage"}},
		}}}},
		"siteinfo": fandom.DefaultSiteInfo,
	}
	body, _ := json.Marshal(payload)
	post := response(s, "POST", "/test/wiki/File:Example.png?from=Next+Token", bytes.NewReader(body), map[string]string{"Origin": "https://example.com"})
	if post.Code != 200 || !strings.Contains(post.Body.String(), "Next page usage") || !strings.Contains(post.Body.String(), "A description") {
		t.Fatalf("JSONP POST file content incomplete: HTTP %d", post.Code)
	}
}
