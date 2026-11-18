package server

import (
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/perennialtech/breezewiki-go/internal/fandom"
)

var navigationEndpoint = endpoint{"navigation", []fandom.Param{
	param("action", "query"), param("meta", "allmessages"), param("ammessages", "wiki-navigation"),
	param("format", "json"), param("formatversion", "2"),
}}

func hasWikiNavigation(wiki, lang string) bool {
	return wiki == "genshin-impact" && lang == ""
}

type navigationItem struct {
	label, href string
	children    []*navigationItem
}

// MediaWiki:Wiki-navigation uses asterisks for levels and target|label for an
// item. An empty target names a group without linking to a page.
func wikiNavigation(wiki, lang, message string) []*navigationItem {
	root := &navigationItem{}
	parents := []*navigationItem{root}
	for _, line := range strings.Split(message, "\n") {
		line = strings.TrimSpace(line)
		target := strings.TrimLeft(line, "*")
		depth := len(line) - len(target)
		if depth < 1 || depth > len(parents) || depth > 8 {
			continue
		}
		target, label, labeled := strings.Cut(target, "|")
		target = strings.TrimSpace(target)
		if !labeled {
			label = target
		}
		label = strings.TrimSpace(label)
		if label == "" {
			continue
		}
		item := &navigationItem{label: label, href: navigationTarget(wiki, lang, target)}
		parent := parents[depth-1]
		parent.children = append(parent.children, item)
		parents = append(parents[:depth], item)
	}
	return root.children
}

func navigationTarget(wiki, lang, target string) string {
	if target == "" {
		return ""
	}
	if u, err := url.Parse(target); err == nil {
		switch {
		case u.Scheme == "https" && u.Host != "" && u.User == nil:
			return u.String()
		case u.Host == "" && strings.HasPrefix(target, "/wiki/"):
			return "/" + wiki + target
		case u.IsAbs() && strings.Contains(target, "://"), strings.HasPrefix(target, "/"):
			return ""
		}
	}
	title, fragment, hasFragment := strings.Cut(target, "#")
	href := linkTo(wiki, lang, strings.TrimSpace(title))
	if hasFragment {
		href += "#" + (&url.URL{Fragment: fragment}).EscapedFragment()
	}
	return href
}

func writeNavigationItems(b *strings.Builder, items []*navigationItem) {
	for _, item := range items {
		b.WriteString("<li>")
		if len(item.children) > 0 {
			b.WriteString(`<details><summary>` + quote(item.label) + `</summary><ul>`)
			if item.href != "" {
				b.WriteString(`<li><a href="` + quote(item.href) + `">` + quote(item.label) + `</a></li>`)
			}
			writeNavigationItems(b, item.children)
			b.WriteString("</ul></details>")
		} else if item.href != "" {
			b.WriteString(`<a href="` + quote(item.href) + `">` + quote(item.label) + "</a>")
		} else {
			b.WriteString(quote(item.label))
		}
		b.WriteString("</li>")
	}
}

func articleNavigation(wiki, lang, title string, info, navigation, parsed map[string]any) string {
	var b strings.Builder
	messages := asList(fandom.Value(navigation, "query", "allmessages"))
	var items []*navigationItem
	if len(messages) > 0 {
		items = wikiNavigation(wiki, lang, fandom.String(messages[0], "content"))
	}
	if hasWikiNavigation(wiki, lang) {
		_, home := fandom.General(info)
		items = append([]*navigationItem{{label: "Explore", children: []*navigationItem{
			{label: "Interactive maps", href: linkTo(wiki, lang, "Special:AllMaps")},
			{label: "Community", href: linkTo(wiki, lang, "Special:Community")},
			{label: "Recent blog posts", href: linkTo(wiki, lang, "Blog:Recent_posts")},
			{label: "Discuss", href: wikiPrefix(wiki, lang) + "/f"},
		}}}, items...)
		if strings.ReplaceAll(title, " ", "_") == strings.ReplaceAll(home, " ", "_") {
			items = append(items, &navigationItem{label: "Page tools", children: []*navigationItem{
				{label: "View source", href: linkTo(wiki, lang, home) + "?action=raw"},
				{label: "Talk", href: linkTo(wiki, lang, "Talk:"+home)},
				{label: "Categories", href: linkTo(wiki, lang, "Special:Categories")},
			}})
		}
	}
	var languages []*navigationItem
	for _, link := range asList(fandom.Value(parsed, "parse", "langlinks")) {
		u, err := url.Parse(fandom.String(link, "url"))
		label := fandom.String(link, "autonym")
		if err != nil || label == "" || u.Scheme != "https" || u.User != nil || !fandom.AllowedProxyURL(u.String()) {
			continue
		}
		languages = append(languages, &navigationItem{label: label, href: u.String()})
	}
	if len(languages) > 0 {
		items = append(items, &navigationItem{label: "Languages", children: languages})
	}
	if len(items) > 0 {
		b.WriteString(`<ul class="bw-wiki-navigation">`)
		writeNavigationItems(&b, items)
		b.WriteString("</ul>")
	}
	return b.String()
}

type cachedNavigation struct {
	data    map[string]any
	expires time.Time
}

func (s *Server) loadNavigation(r *http.Request, wiki string) map[string]any {
	value, _ := s.navigation.Load(wiki)
	cached, _ := value.(cachedNavigation)
	if time.Now().Before(cached.expires) {
		return cached.data
	}
	q := url.Values{}
	for _, p := range navigationEndpoint.params {
		q.Add(p.Key, p.Value)
	}
	res, err := s.Fandom.GetWithoutDenyCache(r.Context(), wiki, "/api.php?"+q.Encode(), "")
	if err == nil && res.Status == http.StatusOK {
		if data, err := res.JSON(); err == nil && fandom.ErrorCode(data) == "" {
			s.navigation.Store(wiki, cachedNavigation{data, time.Now().Add(10 * time.Minute)})
			return data
		}
	}
	if cached.data != nil {
		s.navigation.Store(wiki, cachedNavigation{cached.data, time.Now().Add(time.Minute)})
	}
	return cached.data
}
