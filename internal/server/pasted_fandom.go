package server

import (
	"net/url"
	"regexp"
	"strings"
)

const fandomLocale = `[a-zA-Z]{2,3}(?:-[a-zA-Z0-9]+)*`

var pastedFandomPath = regexp.MustCompile(`(?i)^/(?:https?:/{1,2}|/{1,2})?([a-z0-9-]{1,50})\.(?:fandom|wikia)\.com\.?(?::(?:80|443))?(/.*)?$`)
var fandomLocaleRoot = regexp.MustCompile(`^/` + fandomLocale + `/?$`)
var fandomIndexPath = regexp.MustCompile(`^(?:/(` + fandomLocale + `))?(?:/w)?/index\.php$`)

func pastedFandomTarget(u *url.URL) string {
	// A reverse proxy may merge the two slashes after the embedded scheme.
	m := pastedFandomPath.FindStringSubmatch(u.EscapedPath())
	if m == nil {
		return ""
	}
	wiki, path := strings.ToLower(m[1]), m[2]
	switch {
	case fandomLocaleRoot.MatchString(path):
		path = strings.TrimSuffix(path, "/") + "/wiki"
	case path == "/Blog:Recent_posts" || path == "/Special:Search":
		path = "/wiki" + path
	default:
		if index := fandomIndexPath.FindStringSubmatch(path); index != nil {
			params, err := url.ParseQuery(u.RawQuery)
			if title := params.Get("title"); err == nil && title != "" {
				params.Del("title")
				return linkTo(wiki, index[1], title) + querySuffix(&url.URL{RawQuery: params.Encode()})
			}
		}
	}
	// Browsers inherit the original fragment when Location has none.
	return "/" + wiki + path + querySuffix(u)
}

func querySuffix(u *url.URL) string {
	if u.RawQuery != "" || u.ForceQuery {
		return "?" + u.RawQuery
	}
	return ""
}
