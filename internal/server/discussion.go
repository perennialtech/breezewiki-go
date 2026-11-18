package server

import (
	"net/http"
	"net/url"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

func (s *Server) discussion(w http.ResponseWriter, r *http.Request, wiki, route string) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	path := (&url.URL{Path: "/" + route, RawQuery: r.URL.RawQuery}).RequestURI()
	source := "https://" + wiki + ".fandom.com" + path
	result, err := s.Fandom.GetWithoutDenyCache(r.Context(), wiki, path, theme(r))
	if err != nil || result.Status != http.StatusOK {
		s.readUnavailable(w, r, wiki, "", "Discussions", source)
		return
	}
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(string(result.Body)))
	if err != nil {
		s.readUnavailable(w, r, wiki, "", "Discussions", source)
		return
	}
	body := doc.Find(".post-details-desktop__main").First()
	var content string
	title := "Discussions"
	if body.Length() > 0 {
		body.Find(".desktop-reply-create,.post-footer,button,input,textarea").Remove()
		cleanReadView(body, "/"+wiki)
		content, err = body.Html()
	} else {
		posts := doc.Find("article.standard-post")
		if posts.Length() == 0 && doc.Find(".home-page-desktop__main,.user-posts-desktop__main,.tags-page").Length() == 0 {
			s.readUnavailable(w, r, wiki, "", title, source)
			return
		}
		if heading := strings.TrimSpace(doc.Find("h1").First().Text()); heading != "" && route != "f" {
			title = heading
		}
		var b strings.Builder
		posts.Each(func(_ int, post *goquery.Selection) {
			post.ChildrenFiltered("footer").Remove()
			post.Find("button,input,textarea").Remove()
			cleanReadView(post, "/"+wiki)
			if text, err := post.Html(); err == nil {
				b.WriteString("<article>" + text + "</article>")
			}
		})
		content = b.String()
		if posts.Length() == 0 {
			content = "<p>No discussions to show.</p>"
		}
	}
	if err != nil {
		s.readUnavailable(w, r, wiki, "", title, source)
		return
	}
	// Account features and the upstream feed's additional pages are accessible
	// through the original discussion URL.
	header := `<p><a href="/` + quote(wiki) + `/f">Discussion home</a> · <a href="` + quote(source) + `">Open on Fandom</a></p>`
	s.page(w, r, wiki, "", title, source, header+`<div class="bw-discussion">`+content+`</div>`, "",
		s.Fandom.SiteInfo(r.Context(), wiki, ""))
}
