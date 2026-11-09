package render

import (
	"encoding/json"
	"fmt"
	"html"
	"net/url"
	"regexp"
	"strings"
)

// Options controls the standalone BreezeWiki document surrounding article HTML.
// Content passed to Page is already rendered HTML; call Rewrite separately for
// wiki-specific transformations. SiteInfo accepts sitename and rightsinfo
// license fields from the MediaWiki siteinfo response.
type Options struct {
	Wiki, Title, SourceURL, CanonicalOrigin, ApplicationName, Theme, SiteName, Path, HeadHTML string
	SiteInfo                                                                                  map[string]any
	Notices                                                                                   []string
	CurrentURL                                                                                string
}

var (
	bodyClassRE  = regexp.MustCompile(`<body [^>]*class="([^"]*)"`)
	iconURLRE    = regexp.MustCompile(`<link rel="(?:shortcut )?icon" href="([^"]*)"`)
	wikiSuffixRE = regexp.MustCompile(` ?Wiki$`)
)

func escaped(s string) string { return html.EscapeString(s) }

func siteInfoString(info map[string]any, keys ...string) string {
	var cur any = info
	for _, k := range keys {
		dict, ok := cur.(map[string]any)
		if !ok {
			return ""
		}
		cur = dict[k]
	}
	s, _ := cur.(string)
	return s
}

func siteName(opts Options) string {
	if opts.SiteName != "" {
		return opts.SiteName
	}
	if name := siteInfoString(opts.SiteInfo, "sitename"); name != "" {
		return name
	}
	if name := siteInfoString(opts.SiteInfo, "query", "general", "sitename"); name != "" {
		return name
	}
	return "Unknown Wiki"
}

func licenseInfo(info map[string]any) (text, href string) {
	text, href = "CC-BY-SA", "https://www.fandom.com/licensing"
	if v := siteInfoString(info, "license_text"); v != "" {
		text = v
	}
	if v := siteInfoString(info, "license_url"); v != "" {
		href = v
	}
	if v := siteInfoString(info, "query", "rightsinfo", "text"); v != "" {
		text = v
	}
	if v := siteInfoString(info, "query", "rightsinfo", "url"); v != "" {
		href = v
	}
	if lic, ok := info["license"].(map[string]any); ok {
		if v, ok := lic["text"].(string); ok && v != "" {
			text = v
		}
		if v, ok := lic["url"].(string); ok && v != "" {
			href = v
		}
	}
	return
}

func pathForPage(opts Options) string {
	if opts.Path != "" {
		return opts.Path
	}
	return strings.ReplaceAll(opts.Title, " ", "_")
}

func themeLink(opts Options, theme string) string {
	path := opts.CurrentURL
	if path == "" {
		path = opts.Path
	}
	if !strings.HasPrefix(path, "/") || strings.HasPrefix(path, "//") {
		path = "/" + opts.Wiki + "/wiki/" + pathForPage(opts)
	}
	values := url.Values{}
	values.Set("next_location", path)
	values.Set("new_settings", "#s(user-cookies^ "+theme+" ("+strings.Join(validNotices(opts.Notices), " ")+"))")
	return "/set-user-settings?" + values.Encode()
}

// Page creates the complete HTML document using upstream BreezeWiki's
// stylesheet, importmap, navigation, search, theme, content, and footer hooks.
func Page(content string, opts Options) (string, error) {
	if opts.Wiki == "" {
		return "", fmt.Errorf("render page: wiki is required")
	}
	app := opts.ApplicationName
	if app == "" {
		app = "BreezeWiki"
	}
	theme := opts.Theme
	if theme != "light" && theme != "dark" {
		theme = "default"
	}
	origin := "https://" + opts.Wiki + ".fandom.com"
	site := wikiSuffixRE.ReplaceAllString(siteName(opts), "")
	bodyClass := "skin-fandomdesktop"
	if m := bodyClassRE.FindStringSubmatch(opts.HeadHTML); m != nil {
		bodyClass = m[1]
	}
	favicon := "/static/breezewiki-favicon.svg"
	if m := iconURLRE.FindStringSubmatch(opts.HeadHTML); m != nil {
		favicon = m[1]
	}
	licenseText, licenseURL := licenseInfo(opts.SiteInfo)
	styles := []string{
		origin + "/wikia.php?controller=ThemeApi&method=themeVariables&variant=" + url.QueryEscape(theme),
		origin + "/load.php?lang=en&modules=site.styles%7Cskin.fandomdesktop.styles%7Cext.fandom.PortableInfoboxFandomDesktop.css%7Cext.fandom.GlobalComponents.CommunityHeaderBackground.css%7Cext.fandom.photoGallery.gallery.css%7Cext.gadget.site-styles%2Csound-styles&only=styles&skin=fandomdesktop",
	}
	jsData, err := json.Marshal(map[string]any{"wikiname": opts.Wiki, "strict_proxy": false, "path": pathForPage(opts), "jsonp": nil})
	if err != nil {
		return "", err
	}
	imports, err := json.Marshal(map[string]any{"imports": map[string]string{
		"captcha":            "/static/captcha.js",
		"countdown":          "/static/countdown.js",
		"jsonp":              "/static/jsonp.js",
		"preact":             "/static/preact.js",
		"search-suggestions": "/static/search-suggestions.js",
		"tabs":               "/static/tabs.js",
	}})
	if err != nil {
		return "", err
	}
	var b strings.Builder
	b.WriteString("<!DOCTYPE html><html><head><meta name=\"viewport\" content=\"width=device-width, initial-scale=1\"><title>")
	b.WriteString(escaped(opts.Title + " | " + site + "+" + app))
	b.WriteString("</title>")
	for _, style := range styles {
		b.WriteString(`<link rel="stylesheet" type="text/css" href="`)
		b.WriteString(escaped(style))
		b.WriteString(`">`)
	}
	b.WriteString(`<link rel="stylesheet" type="text/css" href="/static/main.css">`)
	b.WriteString("<script>const BWData = ")
	b.Write(jsData)
	b.WriteString("</script><script type=\"importmap\">")
	b.Write(imports)
	b.WriteString("</script>")
	for _, script := range []string{"search-suggestions.js", "countdown.js", "tabs.js"} {
		b.WriteString(`<script type="module" src="/static/`)
		b.WriteString(script)
		b.WriteString(`"></script>`)
	}
	b.WriteString(`<link rel="icon" href="`)
	b.WriteString(escaped(favicon))
	b.WriteString(`">`)
	if opts.CanonicalOrigin != "" {
		b.WriteString(`<link rel="canonical" href="`)
		b.WriteString(escaped(strings.TrimRight(opts.CanonicalOrigin, "/") + "/" + opts.Wiki + "/wiki/" + strings.TrimPrefix(pathForPage(opts), "/")))
		b.WriteString(`">`)
	}
	b.WriteString(`</head><body class="`)
	b.WriteString(escaped(bodyClass))
	b.WriteString(` bw-tabs-nojs"><div class="main-container"><div class="fandom-community-header__background tileHorizontally header"></div><div class="page"><main class="page__main">`)
	b.WriteString(externalWikiNotice(opts))
	b.WriteString(`<div class="custom-top"><h1 class="page-title">`)
	titleBreadcrumbs(&b, opts.Title)
	b.WriteString(`</h1><nav class="sitesearch"><form action="/`)
	b.WriteString(escaped(opts.Wiki))
	b.WriteString(`/search" class="bw-search-form" id="bw-pr-search-form"><label for="bw-search-input">Search </label><div id="bw-pr-search-input"><input type="text" name="q" id="bw-search-input" autocomplete="off"></div><div class="bw-ss__container" id="bw-pr-search-suggestions"></div></form><div class="bw-theme__select"><span class="bw-theme__main-label">Page theme</span><span class="bw-theme__items">`)
	for _, t := range []string{"default", "light", "dark"} {
		class := "bw-theme__item"
		if t == theme {
			class += " bw-theme__item--selected"
		}
		b.WriteString(`<a rel="nofollow" href="`)
		b.WriteString(escaped(themeLink(opts, t)))
		b.WriteString(`" class="`)
		b.WriteString(class)
		b.WriteString(`"><span class="bw-theme__icon-container">`)
		b.WriteString(themeIcons[t])
		b.WriteString("</span>")
		b.WriteString(t)
		b.WriteString("</a>")
	}
	b.WriteString(`</span></div></nav></div><div id="content"><div id="mw-content-text">`)
	b.WriteString(content)
	b.WriteString("</div></div>")
	footer(&b, opts.SourceURL, app, licenseText, licenseURL)
	b.WriteString("</main></div></div></body></html>")
	return b.String(), nil
}

func titleBreadcrumbs(b *strings.Builder, title string) {
	parts := strings.Split(title, "/")
	for i, part := range parts {
		if i > 0 {
			b.WriteByte('/')
		}
		if remaining := len(parts) - i - 1; remaining > 0 {
			b.WriteString(`<a href="`)
			b.WriteString(escaped(strings.Repeat("../", remaining-1) + "../" + part))
			b.WriteString(`">`)
			b.WriteString(escaped(part))
			b.WriteString("</a>")
		} else {
			b.WriteString(escaped(part))
		}
	}
}

func footer(b *strings.Builder, source, app, license, licenseURL string) {
	b.WriteString(`<footer class="custom-footer"><div class="`)
	if source != "" {
		b.WriteString("custom-footer__cols")
	} else {
		b.WriteString("internal-footer")
	}
	b.WriteString(`"><div><p><img class="my-logo" src="/static/breezewiki.svg"></p><p><a href="https://gitdab.com/cadence/breezewiki">`)
	b.WriteString(escaped(app))
	b.WriteString(` source code</a></p><p><a href="https://docs.breezewiki.com">Documentation and more information</a></p><p><a href="https://lists.sr.ht/~cadence/breezewiki-discuss">Chat / Bug reports / Feature requests</a></p><p>This unofficial instance is based off the `)
	b.WriteString(escaped(app))
	b.WriteString(` source code, but is not controlled by the code developer.</p></div><div><p>`)
	if source != "" {
		b.WriteString(`This page displays proxied content from <a href="`)
		b.WriteString(escaped(source))
		b.WriteString(`" rel="nofollow noreferrer">`)
		b.WriteString(escaped(source))
		b.WriteString("</a>. Text content is available under the ")
		b.WriteString(escaped(license))
		b.WriteString(` license, <a href="`)
		b.WriteString(escaped(licenseURL))
		b.WriteString(`" rel="nofollow">see license info.</a> Media files may have different copying restrictions.`)
	} else {
		b.WriteString(`Text content on wikis run by Fandom is available under the Creative Commons Attribution-Share Alike License 3.0 (Unported), <a href="https://www.fandom.com/licensing" rel="nofollow">see license info.</a> Media files and official Fandom documents have different copying restrictions.`)
	}
	b.WriteString("</p><p>Fandom is a trademark of Fandom, Inc. ")
	b.WriteString(escaped(app))
	b.WriteString(" is not affiliated with Fandom.</p></div></div></footer>")
}
