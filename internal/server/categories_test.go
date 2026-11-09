package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/perennialtech/breezewiki-go/internal/fandom"
)

func TestWikiArticleCategories(t *testing.T) {
	article := map[string]any{"parse": map[string]any{
		"title": "Nightbrother (Gauntlet fighter)",
		"text":  "<p>Article content</p>",
		"categories": []any{
			map[string]any{"sortkey": "", "category": "Captured_starships"},
			map[string]any{"sortkey": "", "category": "Kom'rk-class_fighter/transports"},
			map[string]any{"sortkey": "", "category": "Questions_&_answers?#Test"},
		},
	}}
	var requestedProps string
	s, _ := mockServer(t, map[string]string{"feature_jsonp::enabled": "false"}, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("action") != "parse" || r.URL.Query().Get("page") != "Nightbrother_(Gauntlet_fighter)" {
			upstreamAPI(w, r)
			return
		}
		requestedProps = r.URL.Query().Get("prop")
		_ = json.NewEncoder(w).Encode(article)
	})

	res := response(s, "GET", "/starwars/wiki/Nightbrother_(Gauntlet_fighter)", nil, nil)
	if res.Code != 200 {
		t.Fatalf("status %d: %s", res.Code, res.Body.String())
	}
	if requestedProps != "text|headhtml|langlinks|categories" {
		t.Errorf("requested parse properties %q", requestedProps)
	}
	for _, want := range []string{
		`<nav aria-label="Categories">`,
		`<a href="/starwars/wiki/Category:Captured_starships">Captured starships</a>`,
		`<a href="/starwars/wiki/Category:Kom&#39;rk-class_fighter/transports">Kom&#39;rk-class fighter/transports</a>`,
		`<a href="/starwars/wiki/Category:Questions_&amp;_answers%3F%23Test">Questions &amp; answers?#Test</a>`,
	} {
		if !strings.Contains(res.Body.String(), want) {
			t.Errorf("missing %q in article HTML", want)
		}
	}

	categoryPage := response(s, "GET", "/starwars/wiki/Category:Captured_starships", nil, nil)
	if categoryPage.Code != 200 || strings.Contains(categoryPage.Body.String(), `<nav aria-label="Categories">`) {
		t.Errorf("category route changed: status %d", categoryPage.Code)
	}
	noCategories := response(s, "GET", "/starwars/wiki/Sample_Page", nil, nil)
	if noCategories.Code != 200 || strings.Contains(noCategories.Body.String(), `<nav aria-label="Categories">`) {
		t.Errorf("article without categories changed: status %d", noCategories.Code)
	}

	jsonp, _ := mockServer(t, nil, upstreamAPI)
	page := response(jsonp, "GET", "/starwars/wiki/Nightbrother_(Gauntlet_fighter)", nil, nil)
	if page.Code != 200 || !strings.Contains(page.Body.String(), "prop=text%7Cheadhtml%7Clanglinks%7Ccategories") {
		t.Errorf("JSONP request does not include categories: status %d", page.Code)
	}
	postData, err := json.Marshal(map[string]any{
		"wikiname": "starwars", "path": "Nightbrother_(Gauntlet_fighter)",
		"wikipage": article, "siteinfo": fandom.DefaultSiteInfo,
	})
	if err != nil {
		t.Fatal(err)
	}
	post := response(jsonp, "POST", "/starwars/wiki/Nightbrother_(Gauntlet_fighter)", bytes.NewReader(postData), map[string]string{"Origin": "https://example.com"})
	if post.Code != 200 || !strings.Contains(post.Body.String(), `<nav aria-label="Categories">`) {
		t.Errorf("JSONP POST categories missing: status %d", post.Code)
	}
}
