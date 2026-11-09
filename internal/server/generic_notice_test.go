package server

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestGenericIndependentWikiNotice(t *testing.T) {
	const markup = `<div class="mw-parser-output"><p>A better wiki with <a href="https://example.org/help">help</a>.</p><table><tr><th>Links</th><th>Logo</th></tr><tr><td><a href="https://example.org/">Official homepage</a></td><td><a href="https://example.org/logo.svg">Logo</a></td></tr></table></div>`
	contents, err := parseGenericWiki(strings.NewReader(fmt.Sprintf(`{"parse":{"text":%q}}`, markup)))
	if err != nil {
		t.Fatal(err)
	}
	if len(contents.Paragraphs) != 1 || len(contents.Links) != 1 || contents.Logo != "https://example.org/logo.svg" {
		t.Fatalf("content %+v", contents)
	}
	page := renderGenericNotice(genericWiki{"Example Wiki", "", "https://example.org/wiki/Special:Search"}, "Bricks & Sand", contents)
	for _, want := range []string{"Bricks &amp; Sand", "Bricks+%26+Sand", "Official homepage", `https://example.org/logo.svg`, "has its own website"} {
		if !strings.Contains(page, want) {
			t.Errorf("notice missing %q", want)
		}
	}
}

func TestGenericNoticePageData(t *testing.T) {
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"parse":{"text":"<div class=\"mw-parser-output\"><p>Independent!</p></div>"}}`)
	}))
	defer remote.Close()
	s, _ := mockServer(t, nil, upstreamAPI)
	s.Fandom.HTTP = remote.Client()
	original := genericWikis["fallout"]
	genericWikis["fallout"] = genericWiki{Name: original.Name, API: remote.URL, Search: original.Search}
	defer func() { genericWikis["fallout"] = original }()
	got := s.genericNotice(context.Background(), "fallout", "Pip-Boy")
	if !strings.Contains(got, "Independent!") {
		t.Errorf("missing live notice: %q", got)
	}
	got = s.genericNotice(context.Background(), "fallout", "Vault")
	if !strings.Contains(got, "Vault") {
		t.Errorf("cache missed title: %q", got)
	}
}
