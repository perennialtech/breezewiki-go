package render

import (
	"encoding/json"
	"regexp"
	"strings"
	"testing"

	xhtml "golang.org/x/net/html"
)

func mustRewrite(t *testing.T, source string) string {
	t.Helper()
	got, err := Rewrite(source, "test", "")
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func requireContains(t *testing.T, got string, fragments ...string) {
	t.Helper()
	for _, fragment := range fragments {
		if !strings.Contains(got, fragment) {
			t.Errorf("expected %q in HTML:\n%s", fragment, got)
		}
	}
}

func requireAbsent(t *testing.T, got string, fragments ...string) {
	t.Helper()
	for _, fragment := range fragments {
		if strings.Contains(got, fragment) {
			t.Errorf("unexpected %q in HTML:\n%s", fragment, got)
		}
	}
}

// Ported from lib/tree-updater.rkt's preprocess-html-wiki rackunit examples.
func TestPreprocessWiki(t *testing.T) {
	cases := []struct{ in, want string }{
		{
			`<td class="va-navbox-column" style="width: 33%">` + "\n<li>Hey</li>",
			`<td class="va-navbox-column" style="width: 33%">` + "\n<ul><li>Hey</li>",
		},
		{
			`<figure class="thumb tright" style="width: 150px"><a class="image"><img></a><noscript><a><img></a></noscript><figcaption class="thumbcaption"> ` + "\t" + `<p class="caption">Caption text.</p></figcaption></figure>`,
			`<figure class="thumb tright" style="width: 150px"><a class="image"><img></a><noscript><a><img></a></noscript><figcaption class="thumbcaption"><span class="caption">Caption text.</span></figcaption></figure>`,
		},
		{
			`<td><a href="/foo" class="info-icon"><svg><use xlink:href="#wds-icons-info-small"></use></svg></a><li>Hi`,
			`<td><ul><li>Hi`,
		},
	}
	for _, tc := range cases {
		if got := preprocessHTMLWiki(tc.in); got != tc.want {
			t.Errorf("preprocess\nwant %q\n got %q", tc.want, got)
		}
	}
}

// Ported from lib/tree-updater.rkt's attribute and element assertions.
func TestWikiLinkAndImages(t *testing.T) {
	got := mustRewrite(t, `<div class="mw-parser-output">
		<a data-test-wikilink href="https://test.fandom.com/wiki/Another_Page">Another</a>
		<a href="https://other-wiki.fandom.com/wiki/Home">Other</a>
		<a href="/wiki/Local">Local</a>
		<a class="image" href="https://static.wikia.nocookie.net/source.png"><img class="lazyload" src="data:image/gif;base64,abc" data-src="https://static.wikia.nocookie.net/thumb.png" srcset="x" width="80" height="500" loading="lazy"></a>
		<img loading="lazy" width="500" height="80" data-test-size="wide">
		<img loading="lazy" width="500" height="500" data-test-size="large">
	</div>`)
	requireContains(t, got, `href="/test/wiki/Another_Page"`, `href="/other-wiki/wiki/Home"`, `href="/test/wiki/Local"`)
	requireContains(t, got, `rel=" noreferrer"`, `src="https://static.wikia.nocookie.net/thumb.png"`)
	requireAbsent(t, got, "srcset=", `src="data:image/gif;base64,abc"`)
	if strings.Count(got, `loading="lazy"`) != 1 {
		t.Errorf("expected only large image to remain lazy-loaded: %s", got)
	}
}

func TestLanguageWikiLinks(t *testing.T) {
	input := `<a href="/es/wiki/Local">Local</a><a href="/wiki/Implicit">Implicit</a>` +
		`<a href="https://mlp.fandom.com/es/wiki/One">One</a>` +
		`<a href="https://other.fandom.com/fr/wiki/Deux">Deux</a>` +
		`<a href="/fr/wiki/Another">Another language</a>` +
		`<a href="/mlp/es/wiki/Already">Already local</a>` +
		`<a href="/wiki/wiki/Subpage">Nested wiki title</a>`
	got, err := Rewrite(input, "mlp", "es")
	if err != nil {
		t.Fatal(err)
	}
	requireContains(t, got, `href="/mlp/es/wiki/Local"`, `href="/mlp/es/wiki/Implicit"`,
		`href="/mlp/es/wiki/One"`, `href="/other/fr/wiki/Deux"`, `href="/mlp/fr/wiki/Another"`, `href="/mlp/es/wiki/Already"`, `href="/mlp/es/wiki/wiki/Subpage"`)
}

func TestCommunityInterwikiLinks(t *testing.T) {
	got := mustRewrite(t, `<div>
		<a href="https://community.fandom.com/wiki/w:c:zelda:Link">Article</a>
		<a href="https://community.fandom.com/wiki/w:c:battlefordreamisland:Battle_for_Dream_Island?oldid=42#Plot">Section</a>
		<a href="https://community.fandom.com/wiki/w:c:mlp%3ACherry_Berry">Wikidata article</a>
		<a href="https://community.fandom.com/wiki/w:c:naruto-fandom%3AMangeky%C5%8D_Sharingan">Encoded title</a>
		<a href="https://community.fandom.com/wiki/w:c:battlefordreamisland">Wiki</a>
		<a href="https://community.fandom.com/wiki/Main_Page">Community</a>
		<a href="https://example.org/wiki/w:c:zelda:Link">External</a>
	</div>`)
	requireContains(t, got,
		`href="/zelda/wiki/Link"`,
		`href="/battlefordreamisland/wiki/Battle_for_Dream_Island?oldid=42#Plot"`,
		`href="/mlp/wiki/Cherry_Berry"`,
		`href="/naruto-fandom/wiki/Mangeky%C5%8D_Sharingan"`,
		`href="/battlefordreamisland/wiki/"`,
		`href="/community/wiki/Main_Page"`,
		`href="https://example.org/wiki/w:c:zelda:Link"`)
}

func TestStrictProxyUpstreamExamples(t *testing.T) {
	got, err := RewriteWithProxy(`<div style="background-image:url('https://static.wikia.nocookie.net/bg.png')">
		<a class="image image-thumbnail" href="https://static.wikia.nocookie.net/original.png">
			<img src="data:image/gif;base64,abc" data-src="https://static.wikia.nocookie.net/thumb.png">
		</a>
		<img src="https://example.org/independent.png">
		<a class="image" href="https://test.fandom.com/wiki/Image:Test">file</a>
	</div>`, "test", "", true)
	if err != nil {
		t.Fatal(err)
	}
	requireContains(t, got,
		`href="/proxy?dest=https%3A%2F%2Fstatic.wikia.nocookie.net%2Foriginal.png"`,
		`src="/proxy?dest=https%3A%2F%2Fstatic.wikia.nocookie.net%2Fthumb.png"`,
		`url(/proxy?dest=https%3A%2F%2Fstatic.wikia.nocookie.net%2Fbg.png)`,
		`src="https://example.org/independent.png"`,
		`href="/proxy?dest=https%3A%2F%2Ftest.fandom.com%2Fwiki%2FImage%3ATest"`)
	requireAbsent(t, got, `src="/proxy?dest=data`)
}

func TestCollapsedAndAnimated(t *testing.T) {
	got := mustRewrite(t, `<div class="collapsible collapsetoggle-inline collapsed">
		<div class="mw-collapsible-content"><p style="display: none; color: red">Open</p></div>
		<div class="pi-collapse pi-collapse-closed"></div>
		<table class="navbox collapsed"></table>
		<div id="mw-customcollapsible-1" style="display:none">Visible</div>
		<div class="animated"><img src="a"><img src="b"></div>
		<audio class="ext-audiobutton" hidden>Sound</audio>
	</div>`)
	requireContains(t, got,
		`class="collapsible collapsetoggle-inline"`,
		`style="display:inline; color: red"`,
		`style="display:block"`,
		`class="animated-slot__outer" style="--steps: 2"`,
		`class="animated-slot__inner"`,
		`controls=""`)
	requireAbsent(t, got, `class="navbox collapsed"`, `hidden=""`, "pi-collapse-closed")
}

func TestWholeNodeReplacements(t *testing.T) {
	got := mustRewrite(t, `<div>
		<table class="wikitable"><tbody><tr><td>Data</td></tr></tbody></table>
		<table class="wikitable" data-scrolling><tr><td>Already wrapped</td></tr></table>
		<iframe src="https://example.com/iframe-src"></iframe>
		<figure><figcaption>Only text</figcaption><figcaption><span>Caption kept</span></figcaption></figure>
		<div class="reviews"><span>Ads</span></div>
		<div class="trfc161">AI quick answers</div>
		<span class="mw-editsection">Edit</span>
		<div class="mw-customtoggle-foo button-c">Toggle</div>
		<aside class="pi-item"><a class="video-thumbnail">Video</a></aside>
		<div class="wikia-gallery-item"><span class="video-thumbnail">Video</span></div>
		<noscript><a class="image"><img src="low.jpg"></a></noscript>
		<noscript><img src="low.jpg"></noscript>
		<a class="external" href="https://example.org">External</a>
		<a class="extiw" href="https://en.wikipedia.org/wiki/Test">Wikipedia</a>
	</div>`)
	requireContains(t, got, `<div class="table-scroller" role="region" aria-label="Scrollable table" tabindex="0"><table class="wikitable" data-scrolling=""`, `Already wrapped`,
		`<a class="iframe-alternative" href="https://example.com/iframe-src">Embedded media: https://example.com/iframe-src</a>`,
		`Caption kept`, `<template></template>`, `<svg fill="currentColor"`)
	requireAbsent(t, got, `<iframe`, `<noscript`, "Ads", "AI quick answers", "Edit", "Toggle", "Video", "Only text")
	if count := strings.Count(got, `class="table-scroller"`); count != 1 {
		t.Errorf("want one table scroller, got %d: %s", count, got)
	}
	if count := strings.Count(got, `<svg fill="currentColor"`); count != 1 {
		t.Errorf("want external link icon only on non-Wikipedia link, got %d: %s", count, got)
	}
}

func TestNoscriptWithOtherContentIsRetained(t *testing.T) {
	got := mustRewrite(t, `<noscript><img src="low.jpg"><p>An explanation</p></noscript>`)
	requireContains(t, got, `<noscript>`, `An explanation`)
}

func TestCardTable(t *testing.T) {
	got := mustRewrite(t, `<table class="cardtable"><tbody><tr><td class="cardtable-cardimage"><img src="card.png"></td><td>Statistics</td></tr></tbody></table>`)
	requireContains(t, got, `bw-updated-cardtable-cardimage`, `<td class="cardtable-cardimage"><img src="card.png"/></td>`, "Statistics")
	if count := strings.Count(got, `<img src="card.png"`); count != 1 {
		t.Errorf("card image should be moved once, got %d: %s", count, got)
	}
}

func TestCaptionAndEntities(t *testing.T) {
	got := mustRewrite(t, `<figure><figcaption><p class="caption">Words &amp; more</p></figcaption></figure><a href="https://example.org?a=1&amp;b=2">Link</a><p>A&nbsp;B</p>`)
	requireContains(t, got, `<span class="caption">Words &amp; more</span>`, `href="https://example.org?a=1&amp;b=2"`, "A")
	requireAbsent(t, got, `<p class="caption">`)
	// The HTML5 fragment parser preserves whitespace, entities, and mixed content.
	if _, err := xhtml.Parse(strings.NewReader("<html><body>" + got + "</body></html>")); err != nil {
		t.Fatal(err)
	}
}

// Cases from html-parsing/main.rkt: permissive markup and entity handling.
func TestHTMLParsingUpstreamCases(t *testing.T) {
	for _, tc := range []struct {
		name, source string
		want         []string
	}{
		{"mismatched emphasis", `<div><b><i>bold</b></i> after</div>`, []string{"bold", "after"}},
		{"unterminated paragraphs", `<p>foo<p>bar`, []string{"<p>foo</p><p>bar</p>"}},
		{"quoted entities", `<a href="https://example.org?a=1&amp;b=2">A &amp; B</a>`, []string{`href="https://example.org?a=1&amp;b=2"`, `A &amp; B`}},
		{"uppercase tags", `<IMG SRC="https://example.org/picture.jpg" WIDTH=80>`, []string{`<img src="https://example.org/picture.jpg" width="80"/>`}},
		{"script raw text", `<script>if (a < b) done()</script><p>After</p>`, []string{`if (a < b) done()`, `<p>After</p>`}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := mustRewrite(t, tc.source)
			requireContains(t, got, tc.want...)
		})
	}
}

func TestPreprocessingThroughRewrite(t *testing.T) {
	got := mustRewrite(t, `<table><tr><td class="va-navbox-column"><li>First</li><li>Second</li></td></tr></table>`)
	requireContains(t, got, `<td class="va-navbox-column"><ul><li>First</li><li>Second</li></ul></td>`)

	got = mustRewrite(t, `<figure><figcaption class="thumbcaption"> <p class="caption">A &amp; B</p></figcaption></figure>`)
	requireContains(t, got, `<figcaption class="thumbcaption"><span class="caption">A &amp; B</span></figcaption>`)
}

func TestSVGAndEntityPreservation(t *testing.T) {
	got := mustRewrite(t, `<div><svg viewBox="0 0 10 10"><use xlink:href="#icon"></use></svg><span title="A &amp; B">R &ndash; T</span></div>`)
	requireContains(t, got, `<svg viewBox="0 0 10 10">`, `xlink:href="#icon"`, `title="A &amp; B"`)
	if _, err := xhtml.Parse(strings.NewReader(got)); err != nil {
		t.Fatal(err)
	}
}

func TestPercentEncodeQueryUpstreamCases(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"hello world''", "hello+world%27%27"},
		{"✓", "%E2%9C%93"},
		{"https://test.fandom.com/a?x=1&y=2", "https%3A%2F%2Ftest.fandom.com%2Fa%3Fx%3D1%26y%3D2"},
	} {
		if got := percentEncodeQuery(tc.in); got != tc.want {
			t.Errorf("encode %q: got %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestPageLayoutAndScripts(t *testing.T) {
	body := `<article id="article">A &amp; B</article>`
	opts := Options{
		Wiki: "test", Title: "Level 1/Level 2/Level 3", Path: "Level_1/Level_2/Level_3",
		SourceURL:       "https://test.fandom.com/wiki/Level_1",
		CanonicalOrigin: "https://bw.example/",
		ApplicationName: "SampleWiki", Theme: "dark",
		HeadHTML: `<body class="skin-fandomdesktop custom-skin"><link rel="shortcut icon" href="https://example.org/favicon.ico">`,
		SiteInfo: map[string]any{"query": map[string]any{
			"general":    map[string]any{"sitename": "Cool Wiki"},
			"rightsinfo": map[string]any{"text": "CC-BY-SA-4.0", "url": "https://example.org/license"},
		}},
	}
	got, err := Page(body, opts)
	if err != nil {
		t.Fatal(err)
	}
	requireContains(t, got,
		"<!DOCTYPE html>", "Level 1/Level 2/Level 3 | Cool+SampleWiki",
		`class="skin-fandomdesktop custom-skin theme-fandomdesktop-dark bw-tabs-nojs"`,
		`href="https://example.org/favicon.ico"`,
		`href="https://bw.example/test/wiki/Level_1/Level_2/Level_3"`,
		`variant=dark`, `href="/static/main.css"`, `src="/static/tabs.js"`,
		`src="/static/search-suggestions.js"`, `src="/static/countdown.js"`,
		`id="bw-pr-search-form"`, `id="bw-pr-search-input"`, `id="bw-pr-search-suggestions"`,
		`class="bw-theme__item bw-theme__item--selected"`,
		`href="../../Level 1"`, `href="../Level 2"`, body,
		`CC-BY-SA-4.0 license`, `href="https://example.org/license"`,
		`custom-footer__cols`, `SampleWiki source code`)
	for _, js := range []string{"jsonp", "preact", "tabs"} {
		requireContains(t, got, `"`+js+`":"/static/`+js+`.js"`)
	}
	js := regexp.MustCompile(`const BWData = (\{.*?\})</script>`).FindStringSubmatch(got)
	if js == nil {
		t.Fatal("missing BWData")
	}
	var bw map[string]any
	if err := json.Unmarshal([]byte(js[1]), &bw); err != nil {
		t.Fatal(err)
	}
	if bw["wikiname"] != "test" || bw["path"] != opts.Path || bw["strict_proxy"] != false {
		t.Errorf("unexpected BWData: %#v", bw)
	}
	if _, err := xhtml.Parse(strings.NewReader(got)); err != nil {
		t.Fatalf("invalid page HTML: %v", err)
	}
}

func TestPageDefaultsAndNotices(t *testing.T) {
	if _, err := Page("", Options{}); err == nil {
		t.Error("missing wiki should report an error")
	}
	for _, tc := range []struct {
		wiki, notice string
	}{
		{"minecraft", "The Minecraft Wiki has its own website separate from Fandom."},
		{"xenoblade", "See also"},
		{"calamitymod", "You will be redirected to Official Calamity Mod Wiki"},
		{"unknown", ""},
	} {
		got, err := Page(`<p>Content</p>`, Options{Wiki: tc.wiki, Title: "Some Page"})
		if err != nil {
			t.Fatal(err)
		}
		requireContains(t, got, `<title>Some Page | Unknown+BreezeWiki</title>`, `/static/breezewiki-favicon.svg`, `internal-footer`)
		if tc.notice != "" {
			requireContains(t, got, tc.notice, "Special:Search?search=Some+Page&amp;go=Go")
		} else {
			requireAbsent(t, got, "niwa__notice", "niwa__parallel")
		}
	}
	if got, _ := Page("", Options{Wiki: "test", SiteName: "My Wiki"}); !strings.Contains(got, "My+BreezeWiki") {
		t.Fatal("SiteName override should be used")
	}
}

func TestIndependentWikiDataset(t *testing.T) {
	if len(independentWikis) != 123 {
		t.Fatalf("expected 123 wiki aliases, got %d", len(independentWikis))
	}
	for _, wiki := range []string{"pokemon", "terraria", "runescape", "zelda", "undertale", "deltarune", "rain-web-comic", "teamfortress", "warframe"} {
		if _, ok := independentWikis[wiki]; !ok {
			t.Fatalf("missing upstream independent wiki %q", wiki)
		}
	}
}

func TestWarframeMigrationNotice(t *testing.T) {
	got, err := Page("<p>Content</p>", Options{Wiki: "warframe", Title: "Excalibur"})
	if err != nil {
		t.Fatal(err)
	}
	requireContains(t, got, "Official WARFRAME Wiki", "wiki.warframe.com/w/Special:Search?search=Excalibur", "forums.warframe.com/topic/1439599")
}

func TestPageEscapesMetadata(t *testing.T) {
	got, err := Page("<div>raw</div>", Options{
		Wiki: "test", Title: `<script>alert(1)</script>`,
		ApplicationName: `App & "friends"`,
		HeadHTML:        `<body class="skin-test" ><link rel="icon" href="https://example.org/favicon?a=1&amp;b=2">`,
	})
	if err != nil {
		t.Fatal(err)
	}
	requireContains(t, got, `&lt;script&gt;alert(1)&lt;/script&gt;`, `App &amp; &#34;friends&#34;`, `<div>raw</div>`)
	requireAbsent(t, got, `<title><script>`)
}

func TestSortableTablesScriptIsBundled(t *testing.T) {
	out, err := Page(`<table class="wikitable sortable"><tr><th>Name</th></tr><tr><td>Example</td></tr></table>`, Options{Wiki: "test", Title: "Tables"})
	if err != nil {
		t.Fatal(err)
	}
	requireContains(t, out, `src="/static/sortable.js"`, `"sortable":"/static/sortable.js"`, `class="wikitable sortable"`)
}

func TestArticleHistoryLink(t *testing.T) {
	out, err := Page("<p>Article</p>", Options{Wiki: "example", Title: "Article", Path: "/example/wiki/Article", SourceURL: "https://example.fandom.com/wiki/Article"})
	if err != nil {
		t.Fatal(err)
	}
	requireContains(t, out, `href="https://example.fandom.com/wiki/Article?action=history">History</a>`)
	search, err := Page("Search", Options{Wiki: "example", Title: "Search", Path: "/example/search", SourceURL: "https://example.fandom.com/wiki/Special:Search"})
	if err != nil {
		t.Fatal(err)
	}
	requireAbsent(t, search, `>History</a>`)
}

func TestHistoryURLReservedArticleTitle(t *testing.T) {
	got, err := Page("<p>Test</p>", Options{Wiki: "spongebob", Title: "Are You Happy Now?", Path: "/spongebob/wiki/Are_You_Happy_Now?", SourceURL: "https://spongebob.fandom.com/wiki/Are_You_Happy_Now?"})
	if err != nil {
		t.Fatal(err)
	}
	requireContains(t, got, `href="https://spongebob.fandom.com/wiki/Are_You_Happy_Now%3F?action=history"`)
}

func TestWikiHomeNavigation(t *testing.T) {
	out, err := Page("<p>Article</p>", Options{Wiki: "example", Title: "Article", Path: "/example/wiki/Article"})
	if err != nil {
		t.Fatal(err)
	}
	requireContains(t, out, `<a href="/example/">Wiki home</a>`)
}

func TestLocalizedWikiHomeNavigation(t *testing.T) {
	out, err := Page("<p>Artículo</p>", Options{Wiki: "mlp", Lang: "es", Title: "Artículo", Path: "/mlp/es/wiki/Artículo"})
	if err != nil {
		t.Fatal(err)
	}
	requireContains(t, out, `<a href="/mlp/es/wiki">Wiki home</a>`)
}

func TestWideTableKeyboardScrolling(t *testing.T) {
	got := mustRewrite(t, `<table class="va-table"><tr><th>Long heading</th></tr><tr><td>Large table</td></tr></table>`)
	requireContains(t, got, `<div class="table-scroller" role="region" aria-label="Scrollable table" tabindex="0"><table class="va-table"`)
}
