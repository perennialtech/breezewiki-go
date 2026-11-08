package server

import (
	"context"
	"html"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"github.com/perennialtech/breezewiki-go/internal/fandom"
)

type archivedPageKey struct{}

var styleLink = regexp.MustCompile(`<link\s+rel="stylesheet"[^>]*href="([^"]+)"[^>]*>`)
var iconLink = regexp.MustCompile(`<link\s+rel="icon"[^>]*href="([^"]+)"[^>]*>`)

// Renderer options deliberately do not expose online/offline stylesheet
// selection or the strict_proxy preference. These are HTTP-layer choices,
// so adjust only stylesheet/icon URLs in the finished document.
func (s *Server) adjustPageAssets(document string, r *http.Request, wiki string) string {
	offline, _ := r.Context().Value(archivedPageKey{}).(bool)
	styleIndex := 0
	document = styleLink.ReplaceAllStringFunc(document, func(tag string) string {
		m := styleLink.FindStringSubmatch(tag)
		if len(m) != 2 {
			return tag
		}
		old := html.UnescapeString(m[1])
		if !fandom.AllowedProxyURL(old) {
			return tag
		}
		var replacement string
		if offline {
			if styleIndex == 0 {
				replacement = "/archive/" + wiki + "/styles/themeVariables-" + theme(r) + ".css"
			} else {
				replacement = "/archive/" + wiki + "/styles/site.css"
			}
			styleIndex++
		} else if s.Config.True("strict_proxy") {
			replacement = "/proxy?dest=" + url.QueryEscape(old)
		}
		if replacement == "" {
			return tag
		}
		return strings.Replace(tag, m[1], html.EscapeString(replacement), 1)
	})
	if s.Config.True("strict_proxy") {
		document = iconLink.ReplaceAllStringFunc(document, func(tag string) string {
			m := iconLink.FindStringSubmatch(tag)
			if len(m) != 2 {
				return tag
			}
			old := html.UnescapeString(m[1])
			if !fandom.AllowedProxyURL(old) {
				return tag
			}
			return strings.Replace(tag, m[1], html.EscapeString("/proxy?dest="+url.QueryEscape(old)), 1)
		})
	}
	// Keep BWData's path and proxy flag compatible with the original frontend.
	// render.Options.Path is the local path used by theme links; the JS payload
	// instead requires the title/path sent to Fandom.
	var remotePath string
	prefix := "/" + wiki + "/wiki/"
	if strings.HasPrefix(r.URL.Path, prefix) {
		remotePath = strings.TrimPrefix(r.URL.Path, prefix)
	} else if strings.HasSuffix(r.URL.Path, "/search") {
		values := url.Values{}
		values.Set("query", r.URL.Query().Get("q"))
		values.Set("search", "internal")
		remotePath = "Special:Search?" + values.Encode()
	}
	script := "<script>BWData.path = " + jsonString(remotePath) +
		"; BWData.strict_proxy = " + jsonString(s.Config.True("strict_proxy")) + ";</script>"
	return strings.Replace(document, "</head>", script+"</head>", 1)
}
func markArchived(r *http.Request) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), archivedPageKey{}, true))
}
