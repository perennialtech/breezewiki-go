package server

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/perennialtech/breezewiki-go/internal/fandom"
)

// readView handles destinations that an ordinary action=parse cannot render.
func (s *Server) readView(w http.ResponseWriter, r *http.Request, wiki, lang, page string) bool {
	q := r.URL.Query()
	source := sourceURL(wiki, lang, page)
	if r.URL.RawQuery != "" {
		source += "?" + r.URL.RawQuery
	}
	special, _, _ := strings.Cut(page, "/")
	switch {
	case q.Get("diff") != "":
		s.revisionDiff(w, r, wiki, lang, page, source)
	case q.Get("action") == "history":
		s.revisionList(w, r, wiki, lang, page, source, "revisions")
	case q.Get("action") == "raw" || q.Get("oldid") != "" && (q.Get("action") == "" || q.Get("action") == "view"):
		s.revisionView(w, r, wiki, lang, page, source)
	case q.Get("action") != "" && q.Get("action") != "view",
		q.Has("commentId"), strings.HasPrefix(page, "Message_Wall:"),
		strings.HasPrefix(page, "Map:"), special == "Special:UserLogin",
		special == "Special:CreateAccount", special == "Special:Preferences",
		special == "Special:CreatePage":
		s.redirect(w, source)
	case special == "Special:RecentChanges":
		s.revisionList(w, r, wiki, lang, page, source, "recentchanges")
	case strings.HasPrefix(page, "Special:Contributions/"):
		s.revisionList(w, r, wiki, lang, page, source, "usercontribs")
	case special == "Special:Search":
		query := q.Get("query")
		if query == "" {
			query = q.Get("search")
		}
		s.redirect(w, wikiPrefix(wiki, lang)+"/search?q="+url.QueryEscape(query))
	case strings.HasPrefix(page, "Special:") && special != "Special:AllPages" && special != "Special:Random":
		// Localized special-page aliases must still be resolved by the usual router.
		if lang != "" {
			return false
		}
		s.specialContent(w, r, wiki, lang, page, source)
	default:
		return false
	}
	return true
}

func wikiPrefix(wiki, lang string) string {
	prefix := "/" + wiki
	if lang != "" {
		prefix += "/" + lang
	}
	return prefix
}

func (s *Server) readAPI(r *http.Request, wiki, lang string, params ...fandom.Param) map[string]any {
	params = append(params, param("format", "json"), param("formatversion", "2"))
	result, err := s.Fandom.API(r.Context(), wiki, lang, params, theme(r))
	if err != nil || result.Status != http.StatusOK {
		return nil
	}
	data, err := result.JSON()
	if err != nil || fandom.ErrorCode(data) != "" {
		return nil
	}
	return data
}

func (s *Server) readUnavailable(w http.ResponseWriter, r *http.Request, wiki, lang, page, source string) {
	content := `<p>This view is available <a href="` + quote(source) + `">on Fandom</a>.</p>`
	out, err := s.pageHTML(r, wiki, lang, strings.ReplaceAll(page, "_", " "), source, content, "", nil)
	if err != nil {
		s.internalError(w, err)
		return
	}
	s.html(w, http.StatusBadGateway, out)
}

func (s *Server) revisionView(w http.ResponseWriter, r *http.Request, wiki, lang, page, source string) {
	oldid := requestParam(r, "oldid")
	if requestParam(r, "action") == "raw" {
		params := []fandom.Param{param("action", "query"), param("prop", "revisions"),
			param("rvprop", "content"), param("rvslots", "main")}
		if oldid != "" {
			params = append(params, param("revids", oldid))
		} else {
			params = append(params, param("titles", page))
		}
		data := s.readAPI(r, wiki, lang, params...)
		revs := revisions(data)
		if len(revs) == 0 {
			s.readUnavailable(w, r, wiki, lang, page, source)
			return
		}
		code := fandom.String(revs[0], "slots", "main", "content")
		s.page(w, r, wiki, lang, "Source for "+strings.ReplaceAll(page, "_", " "), source,
			`<pre class="bw-page-source">`+quote(code)+`</pre>`,
			"", s.Fandom.SiteInfo(r.Context(), wiki, lang))
		return
	}
	data := s.readAPI(r, wiki, lang, param("action", "parse"), param("oldid", oldid),
		param("prop", "text|headhtml"))
	title := fandom.String(data, "parse", "title")
	if title == "" {
		s.readUnavailable(w, r, wiki, lang, page, source)
		return
	}
	content := `<p>Revision ` + quote(oldid) + ` · <a href="` + quote(linkTo(wiki, lang, title)) +
		`">Current version</a></p>` + fandom.ParseText(data)
	s.page(w, r, wiki, lang, title, source, content, fandom.String(data, "parse", "headhtml"),
		s.Fandom.SiteInfo(r.Context(), wiki, lang))
}

func revisions(data map[string]any) []any {
	pages := asList(fandom.Value(data, "query", "pages"))
	if len(pages) == 0 {
		return nil
	}
	return asList(fandom.Value(pages[0], "revisions"))
}

func revisionID(item any, key string) string {
	if n, ok := fandom.Value(item, key).(float64); ok && n > 0 {
		return fmt.Sprintf("%.0f", n)
	}
	return ""
}

func (s *Server) revisionDiff(w http.ResponseWriter, r *http.Request, wiki, lang, page, source string) {
	from, to := requestParam(r, "oldid"), requestParam(r, "diff")
	params := []fandom.Param{param("action", "compare"), param("prop", "title|ids|diff")}
	if from == "" {
		params = append(params, param("fromrev", to), param("torelative", "prev"))
	} else {
		params = append(params, param("fromrev", from))
		switch to {
		case "prev", "next", "cur":
			params = append(params, param("torelative", to))
		default:
			params = append(params, param("torev", to))
		}
	}
	data := s.readAPI(r, wiki, lang, params...)
	diff, ok := fandom.Value(data, "compare", "body").(string)
	if !ok {
		s.readUnavailable(w, r, wiki, lang, page, source)
		return
	}
	comparison := fandom.Value(data, "compare")
	fromLink := linkTo(wiki, lang, fandom.String(comparison, "fromtitle")) + "?oldid=" + revisionID(comparison, "fromrevid")
	toLink := linkTo(wiki, lang, fandom.String(comparison, "totitle")) + "?oldid=" + revisionID(comparison, "torevid")
	// The MediaWiki API returns table rows. A table wrapper preserves their structure.
	content := `<div class="table-scroller" tabindex="0"><table class="diff wikitable"><thead><tr><th colspan="2"><a href="` +
		quote(fromLink) + `">Earlier revision</a></th><th colspan="2"><a href="` + quote(toLink) +
		`">Later revision</a></th></tr></thead><tbody>` + diff + `</tbody></table></div>`
	if diff == "" {
		content += "<p>No changes between these revisions.</p>"
	}
	s.page(w, r, wiki, lang, "Changes to "+strings.ReplaceAll(page, "_", " "), source, content, "",
		s.Fandom.SiteInfo(r.Context(), wiki, lang))
}

func (s *Server) revisionList(w http.ResponseWriter, r *http.Request, wiki, lang, page, source, mode string) {
	title := "Recent changes"
	cursor, parentKey := "rccontinue", "old_revid"
	params := []fandom.Param{param("action", "query")}
	switch mode {
	case "revisions":
		title = "History of " + strings.ReplaceAll(page, "_", " ")
		cursor, parentKey = "rvcontinue", "parentid"
		params = append(params, param("titles", page), param("prop", mode),
			param("rvlimit", "50"), param("rvprop", "ids|timestamp|user|comment"))
	case "usercontribs":
		user := strings.ReplaceAll(strings.TrimPrefix(page, "Special:Contributions/"), "_", " ")
		title = "Contributions by " + user
		cursor, parentKey = "uccontinue", "parentid"
		params = append(params, param("list", mode), param("ucuser", user), param("uclimit", "50"),
			param("ucprop", "ids|title|timestamp|comment"))
	default:
		params = append(params, param("list", mode), param("rclimit", "50"), param("rctype", "edit|new"),
			param("rcprop", "ids|title|timestamp|user|comment"))
	}
	if next := requestParam(r, cursor); next != "" {
		params = append(params, param(cursor, next))
	}
	data := s.readAPI(r, wiki, lang, params...)
	if data == nil {
		s.readUnavailable(w, r, wiki, lang, page, source)
		return
	}
	list := asList(fandom.Value(data, "query", mode))
	if mode == "revisions" {
		list = revisions(data)
	}
	var b strings.Builder
	b.WriteString(`<ul class="bw-revisions">`)
	for _, item := range list {
		name := fandom.String(item, "title")
		if name == "" {
			name = strings.ReplaceAll(page, "_", " ")
		}
		href := linkTo(wiki, lang, name)
		id, prev := revisionID(item, "revid"), revisionID(item, parentKey)
		fmt.Fprintf(&b, `<li><a href="%s">%s</a>`, quote(href), quote(name))
		if stamp := fandom.String(item, "timestamp"); id != "" && stamp != "" {
			fmt.Fprintf(&b, ` · <a href="%s"><time datetime="%s">%s</time></a>`,
				quote(href+"?oldid="+id), quote(stamp), quote(strings.ReplaceAll(strings.TrimSuffix(stamp, "Z"), "T", " ")+" UTC"))
		}
		if prev != "" && id != "" {
			fmt.Fprintf(&b, ` · <a href="%s">diff</a>`, quote(href+"?oldid="+prev+"&diff="+id))
		}
		if user := fandom.String(item, "user"); user != "" {
			fmt.Fprintf(&b, ` · <a href="%s">%s</a> (<a href="%s">contributions</a>)`,
				quote(linkTo(wiki, lang, "User:"+user)), quote(user), quote(linkTo(wiki, lang, "Special:Contributions/"+user)))
		}
		if comment := fandom.String(item, "comment"); comment != "" {
			b.WriteString(" · " + quote(comment))
		}
		b.WriteString("</li>")
	}
	b.WriteString("</ul>")
	if len(list) == 0 {
		b.WriteString("<p>No revisions found.</p>")
	}
	if next := fandom.String(data, "continue", cursor); next != "" {
		query := r.URL.Query()
		query.Set(cursor, next)
		fmt.Fprintf(&b, `<p><a href="?%s">Older entries</a></p>`, quote(query.Encode()))
	}
	s.page(w, r, wiki, lang, title, source, b.String(), "", s.Fandom.SiteInfo(r.Context(), wiki, lang))
}

// specialContent keeps Fandom's public list and form markup with the platform
// shell and executable markup removed. GET forms and pagination stay local.
func (s *Server) specialContent(w http.ResponseWriter, r *http.Request, wiki, lang, page, source string) {
	path := strings.TrimPrefix(source, "https://"+wiki+".fandom.com")
	result, err := s.Fandom.GetWithoutDenyCache(r.Context(), wiki, path, theme(r))
	if err != nil || result.Status != http.StatusOK {
		s.readUnavailable(w, r, wiki, lang, page, source)
		return
	}
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(string(result.Body)))
	if err != nil || doc.Find("#mw-content-text").Length() != 1 {
		s.readUnavailable(w, r, wiki, lang, page, source)
		return
	}
	body := doc.Find("#mw-content-text")
	cleanReadView(body, wikiPrefix(wiki, lang))
	content, err := body.Html()
	if err != nil {
		s.readUnavailable(w, r, wiki, lang, page, source)
		return
	}
	title := strings.TrimSpace(doc.Find("h1.page-header__title").First().Text())
	if title == "" {
		title = strings.ReplaceAll(page, "_", " ")
	}
	head, _ := doc.Find("body").First().Attr("class")
	s.page(w, r, wiki, lang, title, source, `<div class="mw-parser-output bw-special-content">`+content+`</div>`,
		`<body class="`+quote(head)+`">`, s.Fandom.SiteInfo(r.Context(), wiki, lang))
}

// cleanReadView retains only inert content and local GET forms.
func cleanReadView(body *goquery.Selection, prefix string) {
	body.Find("script,style,link,meta,base,noscript,iframe,object,embed,svg,.fandom-ad-placeholder").Remove()
	body.Find("form").Each(func(_ int, form *goquery.Selection) {
		method, _ := form.Attr("method")
		action, _ := form.Attr("action")
		if method != "" && !strings.EqualFold(method, "get") || action != "" && !strings.HasPrefix(action, "/wiki/Special:") {
			form.Remove()
		} else if action != "" {
			form.SetAttr("action", prefix+action)
		}
	})
	body.Find("*").Each(func(_ int, sel *goquery.Selection) {
		for _, attribute := range sel.Nodes[0].Attr {
			key := attribute.Key
			if strings.HasPrefix(key, "on") || key == "srcdoc" || key == "contenteditable" || strings.HasPrefix(key, "form") {
				sel.RemoveAttr(key)
				continue
			}
			if key == "href" || key == "src" || key == "poster" {
				u, err := url.Parse(strings.TrimSpace(attribute.Val))
				if err != nil || u.Scheme != "" && u.Scheme != "https" && u.Scheme != "http" && u.Scheme != "mailto" {
					sel.RemoveAttr(key)
				}
			}
		}
	})
}
