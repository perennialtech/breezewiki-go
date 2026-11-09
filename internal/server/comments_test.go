package server

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/perennialtech/breezewiki-go/internal/fandom"
)

func commentFixture() map[string]any {
	post := func(name, model string) map[string]any {
		return map[string]any{
			"createdBy":    map[string]any{"name": name},
			"creationDate": map[string]any{"epochSecond": 1731100000},
			"jsonModel":    model,
		}
	}
	return map[string]any{
		"totalCount": 21,
		"links":      map[string]any{"next": "https://test.fandom.com/wikia.php?page=2"},
		"threads": []any{map[string]any{
			"firstPost": post("Alice", `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"Hello <script>alert(1)</script>","marks":[{"type":"strong"}]}]}]}`),
			"posts":     []any{post("Bob", `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"Reply with link","marks":[{"type":"link","attrs":{"href":"javascript:alert(1)"}}]}]}]}`)},
		}},
	}
}

func TestPublicArticleCommentsAndPagination(t *testing.T) {
	calls := 0
	s, _ := mockServer(t, nil, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/wikia.php" {
			upstreamAPI(w, r)
			return
		}
		calls++
		q := r.URL.Query()
		if q.Get("controller") != "ArticleCommentsController" || q.Get("method") != "getComments" || q.Get("title") != "Page & Title" || q.Get("namespace") != "0" || q.Get("page") != "1" {
			t.Errorf("comment request: %s", r.URL)
		}
		_ = json.NewEncoder(w).Encode(commentFixture())
	})
	got := response(s, "GET", "/test/comments?title=Page+%26+Title", nil, nil)
	if got.Code != 200 {
		t.Fatalf("HTTP %d: %s", got.Code, got.Body.String())
	}
	for _, needle := range []string{"21 comments", "Alice", "Bob", "Reply with link", "<strong>Hello &lt;script&gt;alert(1)&lt;/script&gt;</strong>", `data-comments-page="2"`} {
		if !strings.Contains(got.Body.String(), needle) {
			t.Errorf("missing %q", needle)
		}
	}
	if strings.Contains(got.Body.String(), "href=\"javascript:") || calls != 1 {
		t.Fatalf("unsafe link or wrong call count: %d", calls)
	}
}

func TestArticleCommentsAppearAfterJSONPPost(t *testing.T) {
	s, _ := mockServer(t, nil, upstreamAPI)
	initial := response(s, "GET", "/test/wiki/Example", nil, nil)
	if !strings.Contains(initial.Body.String(), "comments.js") {
		t.Fatal("comments script not loaded on article page")
	}
	posted := map[string]any{"wikiname": "test", "path": "Example", "wikipage": map[string]any{"parse": map[string]any{"title": "Example", "text": "<p>Body</p>"}}, "siteinfo": fandom.DefaultSiteInfo}
	body, _ := json.Marshal(posted)
	got := response(s, "POST", "/test/wiki/Example", bytes.NewReader(body), map[string]string{"Origin": "https://example.com"})
	if got.Code != 200 || !strings.Contains(got.Body.String(), `id="comments"`) || !strings.Contains(got.Body.String(), `data-comments-url="/test/comments?title=Example"`) {
		t.Fatalf("JSONP POST comment placeholder: %d %s", got.Code, got.Body.String())
	}
	direct, _ := mockServer(t, map[string]string{"feature_jsonp::enabled": "false"}, upstreamAPI)
	shown := response(direct, "GET", "/test/wiki/Example", nil, nil)
	if shown.Code != 200 || !strings.Contains(shown.Body.String(), `id="comments"`) {
		t.Fatalf("direct page missing comments: %d", shown.Code)
	}
}

func TestCommentsFailureDoesNotBreakArticle(t *testing.T) {
	s, _ := mockServer(t, map[string]string{"feature_jsonp::enabled": "false"}, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/wikia.php" {
			w.WriteHeader(http.StatusForbidden)
			_, _ = io.WriteString(w, "blocked")
			return
		}
		upstreamAPI(w, r)
	})
	got := response(s, "GET", "/test/comments?title=Example", nil, nil)
	if got.Code == 200 {
		t.Fatalf("blocked comments reported success: %s", got.Body.String())
	}
	article := response(s, "GET", "/test/wiki/Example", nil, nil)
	if article.Code != 200 {
		t.Fatalf("comment fetch failure broke articles: HTTP %d", article.Code)
	}
}
