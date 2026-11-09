package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"
)

const redirectHTML = `<div class="mw-parser-output"><div class="redirectMsg"><a href="https://test.fandom.com/wiki/Target_Page">Target Page</a></div></div>`

func redirectAPI(w http.ResponseWriter, r *http.Request) {
	if r.URL.Query().Get("action") == "parse" {
		_ = json.NewEncoder(w).Encode(map[string]any{"parse": map[string]any{
			"title": "Alias", "text": redirectHTML,
		}})
		return
	}
	upstreamAPI(w, r)
}

func TestArticleRedirectUsesLocation(t *testing.T) {
	s, _ := mockServer(t, map[string]string{"feature_jsonp::enabled": "false"}, redirectAPI)
	got := response(s, "GET", "/test/wiki/Alias", nil, nil)
	if got.Code != 302 || got.Header().Get("Location") != "/test/wiki/Target_Page" || got.Header().Get("Refresh") != "" {
		t.Fatalf("status %d, Location %q, Refresh %q", got.Code, got.Header().Get("Location"), got.Header().Get("Refresh"))
	}
	no := response(s, "GET", "/test/wiki/Alias?redirect=no", nil, nil)
	if no.Code != 200 || no.Header().Get("Location") != "" || no.Header().Get("Refresh") != "" {
		t.Fatalf("redirect=no: status %d, headers %v", no.Code, no.Header())
	}
}

func TestJSONPPostDoesNotHTTPRedirect(t *testing.T) {
	s, _ := mockServer(t, nil, upstreamAPI)
	blob, err := json.Marshal(map[string]any{
		"wikiname": "test", "path": "Alias",
		"wikipage": map[string]any{"parse": map[string]any{"title": "Alias", "text": redirectHTML}},
	})
	if err != nil {
		t.Fatal(err)
	}
	got := response(s, "POST", "/test/wiki/Alias", bytes.NewReader(blob), map[string]string{"Origin": "https://example.com"})
	if got.Code != 200 || got.Header().Get("Location") != "" || !bytes.Contains([]byte(got.Body.String()), []byte("redirectMsg")) {
		t.Fatalf("POST: status %d, headers %v, body %q", got.Code, got.Header(), got.Body.String())
	}
}
