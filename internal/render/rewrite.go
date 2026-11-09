// Package render transforms Fandom article HTML and builds BreezeWiki pages.
package render

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

var (
	wikiLinkRE           = regexp.MustCompile(`^https://([a-zA-Z0-9-]{1,50})\.fandom\.com(/wiki/.*)$`)
	communityInterwikiRE = regexp.MustCompile(`^https://community\.fandom\.com/wiki/w:c:([a-zA-Z0-9-]{1,50})(?:(?::|%3[Aa])(.*))?$`)
	fandomResourceRE     = regexp.MustCompile(`^https://(?:static\.wikia\.nocookie\.net/|[a-zA-Z0-9-]{1,50}\.fandom\.com/)`)
	styleURLRE           = regexp.MustCompile(`url\(['"]?(.*?)['"]?\)`)
	hiddenStyleRE        = regexp.MustCompile(`display: *none`)
	toggleRE             = regexp.MustCompile(`^mw-customtoggle-[^ ]* button-c$`)
	preprocessRE         = regexp.MustCompile(`(<(?:td|figcaption)[^>]*?>\n?)(?:[ \t]*<a href="[^"]*" class="info-icon"><svg><use xlink:href="#wds-icons-info-small"></use></svg></a>)?(?:<li>|[ \t]*?<p class="caption">(.*?)</p>)`)
)

// Rewrite applies the upstream article transformations to an HTML fragment.
// It uses BreezeWiki's default (non-strict) proxy behavior.
func Rewrite(rawHTML, wiki string) (string, error) {
	return RewriteWithProxy(rawHTML, wiki, false)
}

// RewriteWithProxy also supports the upstream strict_proxy configuration:
// image resources and matching background-image URLs are fetched via /proxy.
func RewriteWithProxy(rawHTML, wiki string, strictProxy bool) (string, error) {
	root := &html.Node{Type: html.ElementNode, DataAtom: atom.Div, Data: "div"}
	nodes, err := html.ParseFragmentWithOptions(strings.NewReader(preprocessHTMLWiki(rawHTML)), root, html.ParseOptionEnableScripting(false))
	if err != nil {
		return "", fmt.Errorf("parse wiki HTML: %w", err)
	}
	var out strings.Builder
	for _, n := range nodes {
		if err := html.Render(&out, rewriteElement(n, wiki, strictProxy, false)); err != nil {
			return "", fmt.Errorf("render wiki HTML: %w", err)
		}
	}
	return out.String(), nil
}

func preprocessHTMLWiki(s string) string {
	matches := preprocessRE.FindAllStringSubmatchIndex(s, -1)
	if len(matches) == 0 {
		return s
	}
	var b strings.Builder
	last := 0
	for _, m := range matches {
		b.WriteString(s[last:m[0]])
		first := s[m[2]:m[3]]
		b.WriteString(first)
		if strings.HasPrefix(s[m[0]:], "<figcaption") {
			b.WriteString(`<span class="caption">`)
			if m[4] >= 0 {
				b.WriteString(s[m[4]:m[5]])
			}
			b.WriteString("</span>")
		} else {
			b.WriteString("<ul><li>")
		}
		last = m[1]
	}
	b.WriteString(s[last:])
	return b.String()
}

func copyNode(n *html.Node) *html.Node {
	return &html.Node{
		Type: n.Type, DataAtom: n.DataAtom, Data: n.Data,
		Namespace: n.Namespace, Attr: append([]html.Attribute(nil), n.Attr...),
	}
}

func element(tag string, attrs ...html.Attribute) *html.Node {
	return &html.Node{Type: html.ElementNode, Data: tag, Attr: attrs}
}

func attr(key, value string) html.Attribute {
	return html.Attribute{Key: key, Val: value}
}

func getAttr(n *html.Node, name string) (string, bool) {
	for _, a := range n.Attr {
		if a.Key == name {
			return a.Val, true
		}
	}
	return "", false
}

func setAttr(n *html.Node, name, value string) {
	for i := range n.Attr {
		if n.Attr[i].Key == name {
			n.Attr[i].Val = value
			return
		}
	}
	n.Attr = append(n.Attr, attr(name, value))
}

func removeAttr(n *html.Node, name string) {
	for i := 0; i < len(n.Attr); i++ {
		if n.Attr[i].Key == name {
			n.Attr = append(n.Attr[:i], n.Attr[i+1:]...)
			i--
		}
	}
}

func hasClass(n *html.Node, class string) bool {
	v, ok := getAttr(n, "class")
	if !ok {
		return false
	}
	for _, item := range strings.Fields(v) {
		if item == class {
			return true
		}
	}
	return false
}

func containsDescendant(n *html.Node, predicate func(*html.Node) bool) bool {
	if predicate(n) {
		return true
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if containsDescendant(c, predicate) {
			return true
		}
	}
	return false
}

func firstDescendant(n *html.Node, predicate func(*html.Node) bool) *html.Node {
	if predicate(n) {
		return n
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if match := firstDescendant(c, predicate); match != nil {
			return match
		}
	}
	return nil
}

func removed() *html.Node { return element("template") }

func rewriteElement(n *html.Node, wiki string, strict, withoutCard bool) *html.Node {
	if n.Type != html.ElementNode {
		out := copyNode(n)
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			out.AppendChild(rewriteElement(c, wiki, strict, withoutCard))
		}
		return out
	}
	if withoutCard && n.Data == "td" && hasClass(n, "cardtable-cardimage") {
		return removed()
	}
	switch {
	case n.Data == "figcaption":
		hasElement := false
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			hasElement = hasElement || c.Type == html.ElementNode
		}
		if !hasElement || containsDescendant(n, func(c *html.Node) bool { return c.Type == html.ElementNode && c.Data == "use" }) {
			return removed()
		}
	case (hasClass(n, "pi-item") || hasClass(n, "wikia-gallery-item")) &&
		containsDescendant(n, func(c *html.Node) bool { return hasClass(c, "video-thumbnail") }):
		return removed()
	case n.Data == "span" && hasClass(n, "mw-editsection"):
		return removed()
	case n.Data == "iframe":
		src, _ := getAttr(n, "src")
		out := element("a", attr("class", "iframe-alternative"), attr("href", src))
		out.AppendChild(&html.Node{Type: html.TextNode, Data: "Embedded media: " + src})
		return out
	case n.Data == "noscript" && noscriptImage(n):
		return removed()
	case hasClass(n, "reviews"), hasClass(n, "trfc161"):
		return removed()
	case toggleClass(n):
		return removed()
	}
	if n.Data == "table" && hasClass(n, "cardtable") && !hasClass(n, "bw-updated-cardtable-cardimage") && !withoutCard {
		card := firstDescendant(n, func(c *html.Node) bool {
			return c.Type == html.ElementNode && c.Data == "td" && hasClass(c, "cardtable-cardimage")
		})
		if card != nil {
			wrap := element("div")
			wrap.AppendChild(rewriteElement(card, wiki, strict, false))
			table := copyNode(n)
			cls, _ := getAttr(table, "class")
			setAttr(table, "class", cls+" bw-updated-cardtable-cardimage")
			for c := n.FirstChild; c != nil; c = c.NextSibling {
				table.AppendChild(rewriteElement(c, wiki, strict, true))
			}
			rewriteAttributes(table, wiki, strict)
			wrap.AppendChild(table)
			return wrap
		}
	}
	out := copyNode(n)
	if n.Data == "table" && hasClass(n, "wikitable") {
		if _, ok := getAttr(n, "data-scrolling"); !ok {
			setAttr(out, "data-scrolling", "")
		}
	}
	rewriteAttributes(out, wiki, strict)
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		child := c
		if hasClass(n, "mw-collapsible-content") && c.Type == html.ElementNode {
			child = copyNode(c)
			for cc := c.FirstChild; cc != nil; cc = cc.NextSibling {
				child.AppendChild(cloneTree(cc))
			}
			if v, ok := getAttr(child, "style"); ok {
				setAttr(child, "style", hiddenStyleRE.ReplaceAllString(v, "display:inline"))
			}
		}
		out.AppendChild(rewriteElement(child, wiki, strict, withoutCard))
	}
	if hasClass(n, "animated") && countChildren(n) > 1 {
		outer := element("span", attr("class", "animated-slot__outer"), attr("style", fmt.Sprintf("--steps: %d", countChildren(n))))
		inner := element("span", attr("class", "animated-slot__inner"))
		for c := out.FirstChild; c != nil; {
			next := c.NextSibling
			out.RemoveChild(c)
			inner.AppendChild(c)
			c = next
		}
		outer.AppendChild(inner)
		out.AppendChild(outer)
	}
	if n.Data == "a" && (hasClass(n, "external") || hasClass(n, "extiw")) {
		href, ok := getAttr(n, "href")
		if ok && !strings.Contains(href, "wikipedia.org") {
			out.AppendChild(externalLinkIcon())
		}
	}
	if n.Data == "table" && hasClass(n, "wikitable") {
		if _, ok := getAttr(n, "data-scrolling"); !ok {
			wrap := element("div", attr("class", "table-scroller"))
			wrap.AppendChild(out)
			return wrap
		}
	}
	return out
}

func countChildren(n *html.Node) int {
	count := 0
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if c.Type == html.TextNode || c.Type == html.ElementNode {
			count++
		}
	}
	return count
}

func cloneTree(n *html.Node) *html.Node {
	c := copyNode(n)
	for child := n.FirstChild; child != nil; child = child.NextSibling {
		c.AppendChild(cloneTree(child))
	}
	return c
}

func toggleClass(n *html.Node) bool {
	v, ok := getAttr(n, "class")
	return ok && toggleRE.MatchString(v)
}

func noscriptImage(n *html.Node) bool {
	var only *html.Node
	for child := n.FirstChild; child != nil; child = child.NextSibling {
		if child.Type == html.CommentNode || (child.Type == html.TextNode && strings.TrimSpace(child.Data) == "") {
			continue
		}
		if only != nil {
			return false
		}
		only = child
	}
	return only != nil && only.Type == html.ElementNode &&
		(only.Data == "img" || (only.Data == "a" && hasClass(only, "image")))
}

func rewriteAttributes(n *html.Node, wiki string, strict bool) {
	if class, ok := getAttr(n, "class"); ok {
		class = strings.ReplaceAll(class, "pi-collapse-closed", "")
		class = strings.ReplaceAll(class, "pi-collapse", "")
		class = strings.ReplaceAll(class, " collapsed", "")
		setAttr(n, "class", class)
	}
	if hasClass(n, "ext-audiobutton") {
		removeAttr(n, "hidden")
		setAttr(n, "controls", "")
	}
	if id, ok := getAttr(n, "id"); ok && strings.HasPrefix(id, "mw-customcollapsible") {
		setAttr(n, "style", "display:block")
	}
	if dataSrc, ok := getAttr(n, "data-src"); ok {
		setAttr(n, "src", dataSrc)
	}
	if strict {
		if style, ok := getAttr(n, "style"); ok {
			if indices := styleURLRE.FindStringSubmatchIndex(style); len(indices) >= 4 {
				url := style[indices[2]:indices[3]]
				style = style[:indices[0]] + "url(" + proxyURL(url) + ")" + style[indices[1]:]
				setAttr(n, "style", style)
			}
		}
		if hasClass(n, "image-thumbnail") || hasClass(n, "image") {
			if href, ok := getAttr(n, "href"); ok {
				setAttr(n, "href", proxyURL(href))
			}
		}
		if src, ok := getAttr(n, "src"); ok {
			setAttr(n, "src", proxyURL(src))
		}
	}
	if href, ok := getAttr(n, "href"); ok {
		if parts := communityInterwikiRE.FindStringSubmatch(href); parts != nil {
			href = "/" + parts[1] + "/wiki/" + parts[2]
		} else if parts := wikiLinkRE.FindStringSubmatch(href); parts != nil {
			href = "/" + parts[1] + parts[2]
		} else if strings.HasPrefix(href, "/wiki/") {
			href = "/" + wiki + href
		}
		setAttr(n, "href", href)
	}
	if hasClass(n, "image") {
		rel, _ := getAttr(n, "rel")
		setAttr(n, "rel", rel+" noreferrer")
	}
	if loading, ok := getAttr(n, "loading"); ok && loading == "lazy" {
		for _, k := range []string{"height", "width"} {
			if size, ok := getAttr(n, k); ok {
				if dimension, err := strconv.ParseFloat(size, 64); err == nil && dimension <= 200 {
					removeAttr(n, "loading")
					break
				}
			}
		}
	}
	removeAttr(n, "srcset")
}

func proxyURL(raw string) string {
	if !fandomResourceRE.MatchString(raw) {
		return raw
	}
	return "/proxy?dest=" + percentEncodeQuery(raw)
}

// Mirrors lib/url-utils.rkt's urlencoded-set for values in params->query.
func percentEncodeQuery(s string) string {
	const extra = "!'()~$%&+,:=@[\\]^|;?`{} \"#<>/"
	const upper = "0123456789ABCDEF"
	var b strings.Builder
	for _, c := range []byte(s) {
		switch {
		case c == ' ':
			b.WriteByte('+')
		case c <= 0x1f || c > 0x7e || strings.IndexByte(extra, c) >= 0:
			b.WriteByte('%')
			b.WriteByte(upper[c>>4])
			b.WriteByte(upper[c&15])
		default:
			b.WriteByte(c)
		}
	}
	return b.String()
}

func externalLinkIcon() *html.Node {
	icon := element("svg", attr("fill", "currentColor"), attr("width", "12"), attr("height", "12"), attr("viewBox", "0 0 12 12"), attr("class", "external"))
	icon.AppendChild(element("path", attr("d", "M6 1h5v5L8.86 3.85 4.7 8 4 7.3l4.15-4.16zM2 3h2v1H2v6h6V8h1v2a1 1 0 0 1-1 1H2a1 1 0 0 1-1-1V4a1 1 0 0 1 1-1")))
	return icon
}
