package archive

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestTitleAndURLMapping(t *testing.T) {
	cases := []struct{ title, basename, query string }{
		{"Main Page", "Main_Page", "Main_Page"},
		{"A/B: C?#~&", "A#B%3A_C%3F%23%7E%26", "A/B:_C?#~&"},
		{"Über & Co/Story", "%C3%9Cber_%26_Co#Story", "Über_&_Co/Story"},
		{"Percent%3F", "Percent%3F", "Percent?"},
		{"Something;Original", "Something;Original", "Something;Original"},
	}
	for _, tc := range cases {
		if got := TitleBasename(tc.title); got != tc.basename {
			t.Errorf("%q: basename %q, want %q", tc.title, got, tc.basename)
		}
		got, err := BasenameTitle(tc.basename)
		if err != nil || got != tc.query {
			t.Errorf("%q: query %q, %v, want %q", tc.basename, got, err, tc.query)
		}
	}
	got, err := LocalURLBasename("/wiki/Page%20title/Sub%23part")
	if err != nil || got != "Page title#Sub%23part" {
		t.Errorf("local URL mapping = %q, %v", got, err)
	}
	name := strings.Repeat("X", 241)
	key := PageKey(name)
	if len(key) != 40 || key != sha1Hex(name) {
		t.Fatalf("long page key = %q", key)
	}
	if PageKey(strings.Repeat("X", 240)) != strings.Repeat("X", 240) {
		t.Fatal("short page should not be hashed")
	}
	if got := PagePath("storage", "demo", name, ".json"); got != filepath.Join("storage", "archive", "demo", key+".json") {
		t.Errorf("page path = %q", got)
	}
}

func TestImageURLHashMapping(t *testing.T) {
	cases := []struct{ input, output string }{
		{"https://example.org/a.png?cb=123", "https://example.org/a.png"},
		{"https://example.org/a.png?cb=123&width=20", "https://example.org/a.png?width=20"},
		{"https://example.org/a.png?width=20&cb=123", "https://example.org/a.png?width=20"},
		{"https://example.org/a.png?width=20&cb=123&height=5", "https://example.org/a.png?width=20&height=5"},
		{"https://example.org/a.png?cb=x&foo=1", "https://example.org/a.png?cb=x&foo=1"},
		{"https://example.org/a.png?cb=1&cb=2&size=5", "https://example.org/a.png?size=5"},
	}
	for _, tc := range cases {
		clean, hash := ImageKey(tc.input)
		if clean != tc.output || hash != sha1Hex(tc.output) {
			t.Errorf("ImageKey(%q) = %q, %q", tc.input, clean, hash)
		}
		if got := ImageDir("root", "demo", hash); got != filepath.Join("root", "archive", "demo", "images", hash[:1], hash[:2]) {
			t.Errorf("image path = %q", got)
		}
	}
}

func TestCSSImageURLs(t *testing.T) {
	content := `a{background:url("//static.wikia.nocookie.net/a.png")} b{background:url('/images/b.png')} c{background:url(data:image/gif;base64,AA)} d{background:url('/fonts/font.woff2')} e{background:url(http://example.net/c.png)} f{background:url("https://example.net/a b.png")}`
	got, err := CSSImageURLs("demo", content)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"https://static.wikia.nocookie.net/a.png", "https://demo.fandom.com/images/b.png", "https://example.net/c.png", "https://example.net/a b.png"}
	if len(got) != len(want) {
		t.Fatalf("urls: got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("url %d: got %q, want %q", i, got[i], want[i])
		}
	}
	if _, err := CSSImageURLs("demo", "a{background:url(../image.png)}"); err == nil {
		t.Fatal("unknown relative URL must be reported")
	}
}
