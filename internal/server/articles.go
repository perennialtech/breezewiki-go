package server

import (
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/perennialtech/breezewiki-go/internal/fandom"
)

type endpoint struct {
	name   string
	params []fandom.Param
}

func param(k, v string) fandom.Param { return fandom.Param{Key: k, Value: v} }
func parseEndpoint(title string, extra string) endpoint {
	props := "text|headhtml|langlinks"
	if extra != "" {
		props += "|" + extra
	}
	return endpoint{"wikipage", []fandom.Param{
		param("action", "parse"), param("page", title), param("prop", props),
		param("formatversion", "2"), param("format", "json")}}
}
func endpoints(kind, path, query string) []endpoint {
	switch kind {
	case "allpages":
		params := []fandom.Param{param("action", "query"), param("list", "allpages"),
			param("aplimit", "max"), param("apnamespace", "0"),
			param("formatversion", "2"), param("format", "json")}
		if query != "" {
			params = append(params, param("apcontinue", query))
		}
		return []endpoint{{"allpages", params}}
	case "wiki":
		return []endpoint{parseEndpoint(path, "categories")}
	case "category":
		return []endpoint{
			{"membersdata", []fandom.Param{param("action", "query"), param("list", "categorymembers"), param("cmtitle", path), param("cmlimit", "max"), param("formatversion", "2"), param("format", "json")}},
			{"pagedata", parseEndpoint(path, "").params},
		}
	case "file":
		params := []fandom.Param{param("action", "query"), param("titles", path),
			param("prop", "imageinfo|fileusage"), param("iiprop", "timestamp|user|canonicaltitle|url|size|mime|mediatype|metadata"),
			param("iilocalonly", "true"), param("fulimit", "max"), param("format", "json"), param("formatversion", "2")}
		if query != "" {
			params = append(params, param("fucontinue", query))
		}
		return []endpoint{parseEndpoint(path, "sections"), {"imageinfo", params}}
	case "search":
		return []endpoint{
			{"search", []fandom.Param{param("action", "query"), param("list", "search"), param("srsearch", query), param("srlimit", "50"), param("formatversion", "2"), param("format", "json")}},
		}
	}
	return nil
}
func (s *Server) article(w http.ResponseWriter, r *http.Request, wiki, lang, page, kind string) {
	q := requestParam(r, "q")
	if kind == "allpages" || kind == "file" {
		q = requestParam(r, "from")
	}
	source := sourceURL(wiki, lang, page)
	if kind == "search" {
		source = sourceURL(wiki, lang, "Special:Search") + "?query=" + url.QueryEscape(q) + "&search=internal"
	}
	if r.Method == "POST" {
		s.postArticle(w, r, wiki, lang, page, kind, source, q)
		return
	}
	if s.Config.True("feature_jsonp::enabled") {
		s.jsonp(w, r, wiki, lang, page, kind, source, q)
		return
	}
	ep := endpoints(kind, page, q)
	data := make(map[string]map[string]any, len(ep))
	for _, e := range ep {
		res, err := s.Fandom.API(r.Context(), wiki, lang, e.params, theme(r))
		if err != nil {
			s.internalError(w, wrapError("Fandom "+e.name, err))
			return
		}
		switch res.Status {
		case 200:
		case 404:
			s.notFound(w, r)
			return
		case 403, 406:
			s.fandomDenied(w, r, wiki, lang, page, source)
			return
		default:
			s.internalError(w, fmt.Errorf("Fandom returned %d on %s/%s: %s", res.Status, wiki, page, res.Body))
			return
		}
		js, err := res.JSON()
		if err != nil {
			s.internalError(w, wrapError("decode "+e.name, err))
			return
		}
		if fandom.ErrorCode(js) == "missingtitle" && (kind == "wiki" || kind == "file") {
			s.notFound(w, r)
			return
		}
		data[e.name] = js
	}
	info := s.Fandom.SiteInfo(r.Context(), wiki, lang)
	s.renderArticle(w, r, wiki, lang, page, kind, source, q, data, info)
}

func (s *Server) fandomDenied(w http.ResponseWriter, r *http.Request, wiki, lang, page, source string) {
	s.common(w)
	w.Header().Set("Retry-After", "30")
	w.Header().Set("Cache-Control", "max-age=30, public")
	w.Header().Set("Refresh", "35")
	// Upstream responds 503 (including when Fandom returns 406).
	msg := "<p>Sorry! Fandom isn't allowing BreezeWiki to show pages right now.</p><p>We'll automatically try again in 30 seconds, so please stay on this page and be patient.</p><p><small>In a hurry? <a href=\"" + quote(source) + "\">Click here to read the page on Fandom.</a></small></p>"
	// Preserve the intended status even if render.Page fails.
	info := fandom.DefaultSiteInfo
	out, err := s.pageHTML(r, wiki, lang, strings.ReplaceAll(page, "_", " "), source, msg, "", info)
	if err != nil {
		s.internalError(w, err)
		return
	}
	s.html(w, 503, out)
}

func (s *Server) renderArticle(w http.ResponseWriter, r *http.Request, wiki, lang, page, kind, source, query string, data map[string]map[string]any, info map[string]any) {
	var content, title, head, filePlayer string
	switch kind {
	case "allpages":
		title = "All pages"
		pages := asList(fandom.Value(data["allpages"], "query", "allpages"))
		var b strings.Builder
		b.WriteString(`<div class="mw-parser-output"><ul>`)
		for _, item := range pages {
			name := fandom.String(item, "title")
			fmt.Fprintf(&b, `<li><a href="%s">%s</a></li>`, quote(linkTo(wiki, lang, name)), quote(name))
		}
		b.WriteString(`</ul>`)
		if next := fandom.String(data["allpages"], "continue", "apcontinue"); next != "" {
			fmt.Fprintf(&b, `<p><a href="%s?from=%s">Next page</a></p>`,
				quote(linkTo(wiki, lang, "Special:AllPages")), quote(url.QueryEscape(next)))
		}
		b.WriteString(`</div>`)
		content = b.String()
	case "wiki":
		p := data["wikipage"]
		title = fandom.String(p, "parse", "title")
		if title == "" {
			title = strings.ReplaceAll(page, "_", " ")
		}
		content = fandom.ParseText(p)
		head = fandom.String(p, "parse", "headhtml")
		if categories := asList(fandom.Value(p, "parse", "categories")); len(categories) > 0 {
			var b strings.Builder
			b.WriteString(`<nav aria-label="Categories"><p>Categories: `)
			for i, item := range categories {
				if i > 0 {
					b.WriteString(" · ")
				}
				name := fandom.String(item, "category")
				b.WriteString(`<a href="` + quote(linkTo(wiki, lang, "Category:"+name)) + `">` + quote(strings.ReplaceAll(name, "_", " ")) + `</a>`)
			}
			b.WriteString("</p></nav>")
			content += b.String()
		}
	case "category":
		p := data["pagedata"]
		title = fandom.String(p, "parse", "title")
		if title == "" {
			title = page
		}
		head = fandom.String(p, "parse", "headhtml")
		content = fandom.ParseText(p)
		members := asList(fandom.Value(data["membersdata"], "query", "categorymembers"))
		var b strings.Builder
		b.WriteString("<hr><h2>All Pages in " + quote(title) + "</h2><div class=\"mw-parser-output\"><ul class=\"my-category-list\">")
		for _, m := range members {
			t := fandom.String(m, "title")
			b.WriteString("<li><a href=\"" + quote(linkTo(wiki, lang, t)) + "\">" + quote(t) + "</a></li>")
		}
		b.WriteString("</ul></div>")
		content += b.String()
	case "file":
		p := data["wikipage"]
		title = fandom.String(p, "parse", "title")
		if title == "" {
			title = page
		}
		head = fandom.String(p, "parse", "headhtml")
		pages := asList(fandom.Value(data["imageinfo"], "query", "pages"))
		if len(pages) == 0 {
			s.notFound(w, r)
			return
		}
		first, _ := pages[0].(map[string]any)
		images := asList(first["imageinfo"])
		if len(images) == 0 {
			s.notFound(w, r)
			return
		}
		img, _ := images[0].(map[string]any)
		imageURL := fandom.String(img, "url")
		usage := asList(first["fileusage"])
		var b strings.Builder
		display := imageURL
		if s.Config.True("strict_proxy") && fandom.AllowedProxyURL(imageURL) {
			display = "/proxy?dest=" + url.QueryEscape(imageURL)
		}
		switch fandom.String(img, "mediatype") {
		case "BITMAP", "DRAWING":
			b.WriteString("<img src=\"" + quote(display) + "\"")
			for _, dim := range []string{"width", "height"} {
				if n, ok := img[dim].(float64); ok {
					b.WriteString(" " + dim + "=\"" + fmt.Sprintf("%.0f", n) + "\"")
				}
			}
			b.WriteString(">")
		case "AUDIO":
			b.WriteString("<audio src=\"" + quote(display) + "\" controls></audio>")
		case "VIDEO":
			if fandom.String(img, "mime") == "video/youtube" {
				var videoID string
				for _, item := range asList(img["metadata"]) {
					if fandom.String(item, "name") == "videoId" {
						videoID = fandom.String(item, "value")
						break
					}
				}
				if videoID != "" {
					filePlayer = `<iframe title="YouTube video" src="https://www.youtube-nocookie.com/embed/` +
						quote(url.PathEscape(videoID)) +
						`" width="560" height="315" style="max-width:100%" loading="lazy" referrerpolicy="strict-origin-when-cross-origin" allowfullscreen></iframe>`
					b.WriteString(`<p><a href="https://www.youtube.com/watch?v=` + quote(url.QueryEscape(videoID)) + `">Watch on YouTube</a></p>`)
				} else {
					b.WriteString(`<p><a href="` + quote(source) + `">Watch on Fandom</a></p>`)
				}
			} else {
				b.WriteString("<video src=\"" + quote(display) + "\" controls></video>")
			}
		}
		if fandom.String(img, "mime") != "video/youtube" {
			b.WriteString("<p><a href=\"" + quote(display) + "\">Download original file</a></p>")
		}
		description := fandom.ParseText(p)
		doc, err := goquery.NewDocumentFromReader(strings.NewReader(description))
		if len(asList(fandom.Value(p, "parse", "sections"))) > 0 ||
			err == nil && (strings.TrimSpace(doc.Find(".mw-parser-output").Text()) != "" ||
				doc.Find(".mw-parser-output img, .mw-parser-output video, .mw-parser-output audio, .mw-parser-output iframe").Length() > 0) {
			b.WriteString(description)
		} else {
			b.WriteString("<p>This file may be copyrighted. Consider licensing and fair use law before reusing it.</p>")
		}
		u := fandom.String(img, "user")
		b.WriteString("<p>Uploaded by <a href=\"" + quote(linkTo(wiki, lang, "User:"+u)) + "\">" + quote(u) + "</a>.</p>")
		if timestamp := fandom.String(img, "timestamp"); timestamp != "" {
			b.WriteString("<p>Latest upload: <time datetime=\"" + quote(timestamp) + "\">" + quote(strings.SplitN(timestamp, "T", 2)[0]) + "</time>.</p>")
		}
		next := fandom.String(data["imageinfo"], "continue", "fucontinue")
		if len(usage) > 0 {
			b.WriteString("<p>This file is used in ")
			for i, v := range usage {
				if i > 0 {
					b.WriteString(", ")
				}
				t := fandom.String(v, "title")
				b.WriteString("<a href=\"" + quote(linkTo(wiki, lang, t)) + "\">" + quote(t) + "</a>")
			}
			b.WriteString(".</p>")
		}
		if next != "" {
			b.WriteString(`<p><a href="` + quote(linkTo(wiki, lang, page)+"?from="+url.QueryEscape(next)) + `">More file uses</a></p>`)
		}
		content = b.String()
	case "search":
		title = query
		found := asList(fandom.Value(data["search"], "query", "search"))
		var b strings.Builder
		fmt.Fprintf(&b, "<div class=\"mw-parser-output\"><p>%d results found for <strong>%s</strong></p><ul>", len(found), quote(query))
		for _, item := range found {
			t := fandom.String(item, "title")
			timestamp := fandom.String(item, "timestamp")
			date := strings.SplitN(timestamp, "T", 2)[0]
			wordcount := fandom.Value(item, "wordcount")
			size, _ := fandom.Value(item, "size").(float64)
			fmt.Fprintf(&b, "<li class=\"my-result\"><a class=\"my-result__link\" href=\"%s\">%s</a><div class=\"my-result__info\">last edited <time datetime=\"%s\">%s</time>, %v words, %.1f kb</div></li>",
				quote(linkTo(wiki, lang, t)), quote(t), quote(timestamp), quote(date), wordcount, float64(int(size/100+0.5))/10)
		}
		b.WriteString("</ul></div>")
		content = b.String()
	}
	if kind != "search" {
		rewritten, err := rewriteContent(content, wiki, lang, s.Config.True("strict_proxy"))
		if err != nil {
			s.internalError(w, err)
			return
		}
		content = rewritten
	}
	// Generic article rewriting replaces iframes with links. The file player
	// uses a fixed provider URL assembled from the upstream video ID.
	if filePlayer != "" {
		content = filePlayer + content
	}
	if kind == "wiki" && !strings.HasPrefix(page, "Special:") {
		content += commentsPlaceholder(wiki, lang, title)
	}
	s.layout(w, r, wiki, lang, title, source, content, head, info)
}
func asList(v any) []any { a, _ := v.([]any); return a }

func (s *Server) randomPage(w http.ResponseWriter, r *http.Request, wiki, lang string) {
	params := []fandom.Param{param("action", "query"), param("list", "random"),
		param("rnnamespace", "0"), param("rnlimit", "1"),
		param("formatversion", "2"), param("format", "json")}
	if s.Config.True("feature_jsonp::enabled") {
		s.randomPageJSONP(w, r, wiki, lang, params)
		return
	}
	res, err := s.Fandom.API(r.Context(), wiki, lang, params, theme(r))
	if err != nil {
		s.internalError(w, wrapError("Fandom random", err))
		return
	}
	switch res.Status {
	case 200:
	case 404:
		s.notFound(w, r)
		return
	case 403, 406:
		s.fandomDenied(w, r, wiki, lang, "Special:Random", sourceURL(wiki, lang, "Special:Random"))
		return
	default:
		s.internalError(w, fmt.Errorf("Fandom returned %d for random page: %s", res.Status, res.Body))
		return
	}
	data, err := res.JSON()
	if err != nil {
		s.internalError(w, wrapError("decode random", err))
		return
	}
	items := asList(fandom.Value(data, "query", "random"))
	if len(items) == 0 || fandom.String(items[0], "title") == "" {
		s.notFound(w, r)
		return
	}
	s.redirect(w, linkTo(wiki, lang, fandom.String(items[0], "title")))
}

func (s *Server) randomPageJSONP(w http.ResponseWriter, r *http.Request, wiki, lang string, params []fandom.Param) {
	q := url.Values{}
	for _, p := range params {
		q.Add(p.Key, p.Value)
	}
	q.Set("callback", "showRandomPage")
	var b strings.Builder
	b.WriteString(`<p id="random-status">Finding a random page…</p><script>function showRandomPage(data) {`)
	b.WriteString(`const title = data.query?.random?.[0]?.title; if (!title) { document.getElementById("random-status").textContent = "No random page was found."; return; } `)
	b.WriteString(`location.replace(` + jsonString("/"+wiki+func() string {
		if lang != "" {
			return "/" + lang
		}
		return ""
	}()+"/wiki/") + ` + title.replaceAll(" ", "_").split("/").map(encodeURIComponent).join("/"));`)
	resourceOrigin := "https://" + wiki + ".fandom.com"
	if lang != "" {
		resourceOrigin += "/" + lang
	}
	b.WriteString(`}</script><script async src="` + resourceOrigin + `/api.php?` + quote(q.Encode()))
	b.WriteString(`" onerror="document.getElementById('random-status').textContent='Unable to load a random page.'"></script>`)
	s.layout(w, r, wiki, lang, "Random page", sourceURL(wiki, lang, "Special:Random"), b.String(), "", fandom.DefaultSiteInfo)
}

func (s *Server) jsonp(w http.ResponseWriter, r *http.Request, wiki, lang, page, kind, source, query string) {
	ep := endpoints(kind, page, query)
	extras := map[string]string{}
	if lang != "" && kind == "wiki" && strings.Contains(page, ":") {
		ep[0] = parseEndpoint(page, "categories|sections")
		for _, extra := range []endpoint{endpoints("category", page, query)[0], endpoints("file", page, requestParam(r, "from"))[1]} {
			q := url.Values{}
			for _, param := range extra.params {
				q.Add(param.Key, param.Value)
			}
			q.Set("callback", "proxy."+extra.name)
			extras[extra.name] = "https://" + wiki + ".fandom.com/" + lang + "/api.php?" + q.Encode()
		}
	}
	ep = append(ep, endpoint{"siteinfo", fandom.SiteInfoParams})
	// A JSONP page is a real HTML page, not JSON. The browser fetches each
	// endpoint with a callback, then POSTs the results back to this URL.
	var b strings.Builder
	b.WriteString("<noscript>You have to enable JavaScript to load wiki pages. Sorry!</noscript><div id=\"loading\"></div><div id=\"progress-bar\" style=\"margin-bottom: 50vh\"><progress></progress></div>")
	b.WriteString("<script>var jsonpData = {}; var proxy = new Proxy(jsonpData, {get(obj, prop) { return value => obj[prop] = value }});</script>")
	for _, e := range ep {
		q := url.Values{}
		for _, p := range e.params {
			q.Add(p.Key, p.Value)
		}
		q.Set("callback", "proxy."+e.name)
		u := "https://" + wiki + ".fandom.com"
		if lang != "" {
			u += "/" + lang
		}
		u += "/api.php?" + q.Encode()
		fmt.Fprintf(&b, "<script async src=\"%s\" data-jsonp-var=\"%s\" onerror=\"proxy.%s({error:{code:'disconnected',info:'Fandom connection failed or was blocked by your browser. Check any browser extensions that may block third-party scripts, then reload the page.'}})\"></script>", html.EscapeString(u), e.name, e.name)
	}
	b.WriteString("<script>BWData.path = " + jsonString(page) + "; BWData.jsonp = true; BWData.strict_proxy = " + jsonString(s.Config.True("strict_proxy")) + ";")
	if len(extras) > 0 {
		b.WriteString("BWData.localizedExtraEndpoints = " + jsonString(extras) + ";")
	}
	b.WriteString("</script>")
	b.WriteString("<script type=\"module\" src=\"" + s.staticURL("jsonp.js") + "\"></script>")
	title := strings.ReplaceAll(page, "_", " ")
	if kind == "search" {
		title = "Search"
	}
	s.layout(w, r, wiki, lang, title, source, b.String(), "", fandom.DefaultSiteInfo)
}
func (s *Server) postArticle(w http.ResponseWriter, r *http.Request, wiki, lang, page, kind, source, query string) {
	origin := r.Header.Get("Origin")
	if origin == "" {
		origin = r.Header.Get("Referer")
	}
	if !s.allowedPOSTOrigin(r, origin) {
		s.postError(w, "Origin/Referer header failed validation - cross-origin requests are not allowed here")
		return
	}
	if r.Body == nil {
		s.postError(w, "POST requests only, please.")
		return
	}
	blob, err := io.ReadAll(io.LimitReader(r.Body, 8*1024*1024+1))
	if err != nil || len(blob) > 8*1024*1024 {
		s.postError(w, "Invalid POST data.")
		return
	}
	var input map[string]any
	if err := json.Unmarshal(blob, &input); err != nil || input == nil {
		s.postError(w, "Invalid POST JSON.")
		return
	}
	postWiki, _ := input["wikiname"].(string)
	postPage, _ := input["path"].(string)
	if !fandom.ValidWiki(postWiki) || postPage == "" && kind != "search" {
		s.postError(w, "Missing wikiname or path.")
		return
	}
	source = sourceURL(postWiki, lang, postPage)
	if kind == "search" {
		source = sourceURL(postWiki, lang, "Special:Search") + "?query=" + url.QueryEscape(query) + "&search=internal"
	}
	info := fandom.DefaultSiteInfo
	if v, ok := input["siteinfo"].(map[string]any); ok {
		info = v
	}
	if lang != "" && kind == "wiki" && strings.Contains(postPage, ":") {
		kind = namespaceKind(info, postPage)
		if kind == "category" {
			input["pagedata"] = input["wikipage"]
		}
	}
	data := map[string]map[string]any{}
	for _, e := range endpoints(kind, postPage, query) {
		d, ok := input[e.name].(map[string]any)
		if !ok {
			s.postError(w, "Missing "+e.name+" data.")
			return
		}
		data[e.name] = d
	}
	s.renderArticle(w, r, postWiki, lang, postPage, kind, source, query, data, info)
}
func (s *Server) postError(w http.ResponseWriter, message string) {
	s.common(w)
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(400)
	_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{"code": "breezewiki", "info": message}})
}
