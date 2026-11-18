package server

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPastedFandomURLRedirect(t *testing.T) {
	s, _ := mockServer(t, map[string]string{"canonical_origin": "https://mirror.example"}, func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("unexpected upstream request %s", r.URL)
		w.WriteHeader(http.StatusInternalServerError)
	})
	for _, tc := range []struct {
		request, location string
	}{
		{"/https://undertale.fandom.com/wiki/Hot_Dog...%3F", "/undertale/wiki/Hot_Dog...%3F"},
		{"/HTTP://TEST.FANDOM.COM:80/wiki/Page", "/test/wiki/Page"},
		{"/https:/test.fandom.com/wiki/Page", "/test/wiki/Page"},
		{"/test.fandom.com/wiki/Page%2FSubpage%23Part%25?q=one%2Btwo&q=%3F", "/test/wiki/Page%2FSubpage%23Part%25?q=one%2Btwo&q=%3F"},
		{"///test.fandom.com/f/p/123", "/test/f/p/123"},
		{"/test.wikia.com/wiki/File:Example.png?", "/test/wiki/File:Example.png?"},
		{"/https://genshin-impact.fandom.com", "/genshin-impact"},
		{"/https://test.fandom.com/pt-br/?redirect=no", "/test/pt-br/wiki?redirect=no"},
		{"/https://test.fandom.com/es/wiki/Categor%C3%ADa:Personajes", "/test/es/wiki/Categor%C3%ADa:Personajes"},
		{"/https://test.fandom.com/es/search?q=one+two", "/test/es/search?q=one+two"},
		{"/https://test.fandom.com/wiki/index.php", "/test/wiki/index.php"},
		{"/https://test.fandom.com/Special:Search?query=example", "/test/wiki/Special:Search?query=example"},
		{"/https://test.fandom.com/w/index.php?title=Page+Name&oldid=123", "/test/wiki/Page_Name?oldid=123"},
		{"/https://test.fandom.com/es/index.php?title=Qu%C3%A9%3F&action=history", "/test/es/wiki/Qu%C3%A9%3F?action=history"},
		{"https://other.mirror.example/https://test.fandom.com/wiki/Page?oldid=123", "https://mirror.example/test/wiki/Page?oldid=123"},
	} {
		t.Run(tc.request, func(t *testing.T) {
			for _, method := range []string{http.MethodGet, http.MethodHead} {
				got := response(s, method, tc.request, nil, nil)
				if got.Code != http.StatusFound || got.Header().Get("Location") != tc.location {
					t.Errorf("%s: status %d, Location %q, want %q", method, got.Code, got.Header().Get("Location"), tc.location)
				}
			}
		})
	}
	for _, tc := range []struct{ method, request string }{
		{http.MethodGet, "/https://example.org/wiki/Page"},
		{http.MethodGet, "/https://test.fandom.com.example.org/wiki/Page"},
		{http.MethodGet, "/https://test.fandom.com@elsewhere.example/wiki/Page"},
		{http.MethodGet, "/ftp://test.fandom.com/wiki/Page"},
		{http.MethodPost, "/https://test.fandom.com/wiki/Page"},
	} {
		got := response(s, tc.method, tc.request, nil, nil)
		if got.Code != http.StatusNotFound || got.Header().Get("Location") != "" {
			t.Errorf("%s %s: status %d, Location %q, want 404 with no redirect", tc.method, tc.request, got.Code, got.Header().Get("Location"))
		}
	}
}

func TestPastedFandomURLResolvesThroughWikiRouter(t *testing.T) {
	s, _ := mockServer(t, nil, upstreamAPI)
	local := httptest.NewServer(s)
	defer local.Close()

	for _, tc := range []struct{ path, finalURI, content string }{
		{"/https://bfdi.fandom.com/es/wiki/Quand%3F%2FSubpage?redirect=no", "/battlefordreamisland/es/wiki/Quand%3F%2FSubpage?redirect=no", `BWData.path = "Quand?/Subpage"`},
		{"/https://test.fandom.com/es/?redirect=no", "/test/es/wiki/Start_Page?redirect=no", `BWData.path = "Start_Page"`},
	} {
		t.Run(tc.path, func(t *testing.T) {
			got, err := local.Client().Get(local.URL + tc.path)
			if err != nil {
				t.Fatal(err)
			}
			defer got.Body.Close()
			if got.StatusCode != http.StatusOK || got.Request.URL.RequestURI() != tc.finalURI {
				t.Fatalf("status %d, final URL %s, want %s", got.StatusCode, got.Request.URL, tc.finalURI)
			}
			body, err := io.ReadAll(got.Body)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(body), tc.content) {
				t.Errorf("page is missing %q", tc.content)
			}
		})
	}
}
