package render

import (
	"html"
	"net/url"
	"strings"
	"testing"
)

func TestWikiNoticeDismissalAndThemePreserveCookies(t *testing.T) {
	opts := Options{Wiki: "minecraft", Title: "Bricks", Theme: "dark", CurrentURL: "/minecraft/wiki/Bricks?redirect=no", Notices: []string{"pokemon"}}
	content := externalWikiNotice(opts)
	if !strings.Contains(content, "OK, got it") || strings.Contains(content, "niwa--seen") {
		t.Fatal("missing initial dismissal link or marked seen")
	}
	urlPart := strings.Split(strings.Split(content, `class="niwa__got-it" href="`)[1], `"`)[0]
	u, err := url.Parse(html.UnescapeString(urlPart))
	if err != nil {
		t.Fatal(err)
	}
	if got := u.Query().Get("next_location"); got != opts.CurrentURL {
		t.Fatalf("url %q", got)
	}
	if got := u.Query().Get("new_settings"); got != "#s(user-cookies^ dark (minecraft pokemon))" {
		t.Fatalf("settings %q", got)
	}
	opts.Notices = []string{"minecraft", "pokemon"}
	if !strings.Contains(externalWikiNotice(opts), "niwa--seen") {
		t.Fatal("dismissed notice should be visually marked")
	}
	u, err = url.Parse(themeLink(opts, "light"))
	if err != nil {
		t.Fatal(err)
	}
	if got := u.Query().Get("new_settings"); got != "#s(user-cookies^ light (minecraft pokemon))" {
		t.Fatalf("theme lost notices: %q", got)
	}
}

func TestIndependentWikiNoticeStartsCompact(t *testing.T) {
	opts := Options{Wiki: "minecraft", Title: "Bricks"}
	got := externalWikiNotice(opts)
	for _, want := range []string{`class="niwa__summary"`, `<details class="niwa__details">`, `<summary>More information</summary>`, `class="niwa__go"`, `class="niwa__got-it"`} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q", want)
		}
	}
	if strings.Contains(got, `<details class="niwa__details" open`) {
		t.Fatal("notice details should start collapsed")
	}
}

func TestParallelAndRedirectNoticesRemainCompact(t *testing.T) {
	parallel := externalWikiNotice(Options{Wiki: "xenoblade", Title: "Shulk"})
	for _, want := range []string{`class="niwa__summary"`, `<details class="niwa__details">`, `<summary>More information</summary>`, "See also", "multiple communities"} {
		if !strings.Contains(parallel, want) {
			t.Errorf("parallel missing %q", want)
		}
	}
	redirect := externalWikiNotice(Options{Wiki: "calamitymod", Title: "Item"})
	for _, want := range []string{`class="niwa__go"`, `<details class="niwa__details">`, "Take me there!", `niwa__notice--alt`} {
		if !strings.Contains(redirect, want) {
			t.Errorf("redirect missing %q", want)
		}
	}
}
