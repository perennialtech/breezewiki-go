package server

import (
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/perennialtech/breezewiki-go/internal/fandom"
)

// searchSolr implements feature_offline::search=solr, using the same indexed
// fields and filters as the original BreezeWiki search provider.
func (s *Server) searchSolr(w http.ResponseWriter, r *http.Request, wiki string) {
	if s.Config.Get("feature_offline::search") != "solr" {
		s.internalError(w, fmt.Errorf("unknown search provider configured: %q", s.Config.Get("feature_offline::search")))
		return
	}
	op := "AND"
	if r.URL.Query().Get("op") == "or" {
		op = "OR"
	}
	sort := "score desc"
	if r.URL.Query().Get("sort") == "len" {
		sort = "len desc"
	}
	q := r.URL.Query().Get("q")
	qs := url.Values{}
	for k, v := range map[string]string{
		"defType": "edismax", "q": q, "q.op": op,
		"qf": "title_prefix title^2.0 body table^0.3",
		"hl": "true", "hl.method": "unified", "hl.defaultSummary": "true",
		"hl.fl": "title,body", "fl": "id,len,title",
		"hl.encoder": "html", "hl.tag.pre": "<mark>", "hl.tag.post": "</mark>",
		"sort": sort,
	} {
		qs.Set(k, v)
	}
	base := s.SolrBaseURL
	if base == "" {
		base = "http://localhost:8983"
	}
	u := strings.TrimRight(base, "/") + "/solr/" + wiki + "/select?" + qs.Encode()
	request, err := http.NewRequestWithContext(r.Context(), "GET", u, nil)
	if err != nil {
		s.internalError(w, err)
		return
	}
	client := s.Fandom.HTTP
	if client == nil {
		client = http.DefaultClient
	}
	res, err := client.Do(request)
	if err != nil {
		s.internalError(w, err)
		return
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		body, _ := io.ReadAll(res.Body)
		s.internalError(w, fmt.Errorf("Solr returned %d: %s", res.StatusCode, body))
		return
	}
	var data map[string]any
	if err := json.NewDecoder(res.Body).Decode(&data); err != nil {
		s.internalError(w, err)
		return
	}
	header, _ := data["responseHeader"].(map[string]any)
	result, _ := data["response"].(map[string]any)
	highlighting, _ := data["highlighting"].(map[string]any)
	docs := asList(result["docs"])
	numFound := result["numFound"]
	timing := floatNumber(header["QTime"])
	qtime := float64(int(timing/10+0.5)) / 100
	var b strings.Builder
	b.WriteString("<div class=\"mw-parser-output\"><form class=\"my-result__filter\">")
	b.WriteString("<input type=\"hidden\" name=\"q\" value=\"" + html.EscapeString(q) + "\">")
	b.WriteString("<select name=\"op\">")
	b.WriteString(option("and", "All words must match", op == "AND"))
	b.WriteString(option("or", "Some words must match", op == "OR"))
	b.WriteString("</select><select name=\"sort\">")
	b.WriteString(option("relevance", "Relevant articles", sort == "score desc"))
	b.WriteString(option("len", "Wordiest articles", sort == "len desc"))
	b.WriteString("</select><button>Filter results</button></form>")
	fmt.Fprintf(&b, "<p>%v results (%.2f seconds) found for <strong>%s</strong></p><ul>", numFound, qtime, html.EscapeString(q))
	for _, doc := range docs {
		d, _ := doc.(map[string]any)
		id := fandom.String(d, "id")
		length := floatNumber(d["len"])
		title := fandom.String(d, "title")
		if titles, ok := d["title"].([]any); ok && len(titles) > 0 {
			title, _ = titles[0].(string)
		}
		h, _ := highlighting[id].(map[string]any)
		hTitle := firstString(h["title"])
		if hTitle == "" {
			hTitle = html.EscapeString(title)
		}
		hBody := firstString(h["body"])
		kb := float64(int(length/100+0.5)) / 10
		words := int(length/60+0.5) * 10
		fmt.Fprintf(&b, "<li class=\"my-result\"><a class=\"my-result__link\" href=\"%s\">%s</a><p class=\"my-result__description\">%s</p><div class=\"my-result__info\">%d words, %.1f kb of readable stuff</div></li>",
			html.EscapeString(linkTo(wiki, title)), hTitle, hBody, words, kb)
	}
	b.WriteString("</ul></div>")
	source := sourceURL(wiki, "Special:Search?query="+url.QueryEscape(q)+"&search=internal")
	s.layout(w, r, wiki, q, source, b.String(), "", s.siteInfo(r.Context(), wiki))
}
func option(value, label string, selected bool) string {
	out := "<option value=\"" + value + "\""
	if selected {
		out += " selected"
	}
	return out + ">" + label + "</option>"
}
func firstString(v any) string {
	if a, ok := v.([]any); ok && len(a) > 0 {
		if s, ok := a[0].(string); ok {
			return s
		}
	}
	return ""
}
func floatNumber(v any) float64 {
	switch x := v.(type) {
	case float64:
		return x
	case json.Number:
		f, _ := x.Float64()
		return f
	}
	return 0
}
