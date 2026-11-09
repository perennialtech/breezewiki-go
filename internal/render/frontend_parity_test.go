package render

import (
	"strings"
	"testing"
)

func TestConfiguredThemeOverridesArchivedHeadHTML(t *testing.T) {
	for _, tc := range []struct {
		theme, head, want, unwanted string
	}{
		{"dark", `<body class="skin-fandomdesktop theme-fandomdesktop-light other">`, `theme-fandomdesktop-dark`, `theme-fandomdesktop-light`},
		{"light", `<body class="skin-fandomdesktop theme-fandomdesktop-dark">`, `theme-fandomdesktop-light`, `theme-fandomdesktop-dark`},
		{"dark", "", `theme-fandomdesktop-dark`, `theme-fandomdesktop-light`},
	} {
		out, err := Page("<p>Archive</p>", Options{Wiki: "test", Theme: tc.theme, HeadHTML: tc.head})
		if err != nil {
			t.Fatal(err)
		}
		body := strings.Split(out, `<body class="`)[1]
		body = strings.Split(body, `"`)[0]
		if !strings.Contains(body, tc.want) || strings.Contains(body, tc.unwanted) {
			t.Errorf("theme=%q head=%q: body classes %q", tc.theme, tc.head, body)
		}
	}
}

func TestSearchSuggestionsSettingAndSourceNotice(t *testing.T) {
	defaults, err := Page("<p>Article</p>", Options{Wiki: "test"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(defaults, `src="/static/search-suggestions.js"`) {
		t.Fatal("suggestions missing by default")
	}
	off, err := Page("<p>Article</p>", Options{Wiki: "test", DisableSearchSuggestions: true})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(off, `src="/static/search-suggestions.js"`) {
		t.Fatal("disabled suggestions script loaded")
	}
	if !strings.Contains(off, `src="/static/countdown.js"`) || !strings.Contains(off, `src="/static/tabs.js"`) {
		t.Fatal("other scripts missing")
	}
	if !strings.Contains(off, `https://github.com/perennialtech/breezewiki-go`) {
		t.Fatal("source code footer points away from published Go source")
	}
}
