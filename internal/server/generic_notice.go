package server

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/PuerkitoBio/goquery"
)

// These three independent wikis publish MediaWiki:BreezeWikiRedirect on their
// own sites rather than being described by the bundled independent-wiki data.
// The table and introductory text are fetched on demand, like the Racket app.
type genericWiki struct {
	Name, API, Search string
}

var genericWikis = map[string]genericWiki{
	"gallowmere":  {"MediEvil Wiki", "https://medievil.wiki/api.php", "https://medievil.wiki/w/Special:Search"},
	"fallout":     {"Fallout Wiki", "https://fallout.wiki/api.php", "https://fallout.wiki/wiki/Special:Search"},
	"drawntolife": {"Wapopedia", "https://drawntolife.wiki/w/api.php", "https://drawntolife.wiki/en/Special:Search"},
}

type genericWikiContent struct {
	Paragraphs []string
	Links      [][2]string
	Logo       string
	Errors     []string
}

type genericCacheEntry struct {
	value  genericWikiContent
	loaded time.Time
}

type genericCache struct {
	mu      sync.Mutex
	entries map[string]genericCacheEntry
}

func (s *Server) genericNotice(ctx context.Context, wiki, title string) string {
	spec, ok := genericWikis[wiki]
	if !ok {
		return ""
	}
	cache := &s.genericCache
	cache.mu.Lock()
	if entry, ok := cache.entries[wiki]; ok && time.Since(entry.loaded) < time.Hour {
		cache.mu.Unlock()
		return renderGenericNotice(spec, title, entry.value)
	}
	cache.mu.Unlock()
	client := s.Fandom.HTTP
	if client == nil {
		client = http.DefaultClient
	}
	query := url.Values{}
	query.Set("action", "parse")
	query.Set("page", "MediaWiki:BreezeWikiRedirect")
	query.Set("prop", "text")
	query.Set("formatversion", "2")
	query.Set("format", "json")
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, spec.API+"?"+query.Encode(), nil)
	if err != nil {
		return ""
	}
	resp, err := client.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return ""
	}
	content, err := parseGenericWiki(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return ""
	}
	cache.mu.Lock()
	if cache.entries == nil {
		cache.entries = make(map[string]genericCacheEntry)
	}
	cache.entries[wiki] = genericCacheEntry{value: content, loaded: time.Now()}
	cache.mu.Unlock()
	return renderGenericNotice(spec, title, content)
}

func parseGenericWiki(reader io.Reader) (genericWikiContent, error) {
	var response struct {
		Parse struct {
			Text string `json:"text"`
		} `json:"parse"`
	}
	if err := json.NewDecoder(reader).Decode(&response); err != nil {
		return genericWikiContent{}, err
	}
	if response.Parse.Text == "" {
		return genericWikiContent{}, fmt.Errorf("missing independent-wiki notice")
	}
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(response.Parse.Text))
	if err != nil {
		return genericWikiContent{}, err
	}
	main := doc.Find(".mw-parser-output").First()
	if main.Length() == 0 {
		return genericWikiContent{}, fmt.Errorf("independent-wiki notice content missing")
	}
	var value genericWikiContent
	main.Find("p").Each(func(_ int, p *goquery.Selection) {
		if markup, err := goquery.OuterHtml(p); err == nil {
			value.Paragraphs = append(value.Paragraphs, markup)
		}
	})
	table := main.Find("table").First()
	if table.Length() == 0 {
		return value, nil
	}
	keys := make(map[string]int)
	table.Find("tr").First().Find("th").Each(func(i int, col *goquery.Selection) {
		keys[strings.ToLower(strings.TrimSpace(col.Text()))] = i
	})
	cells := table.Find("tr").Eq(1).Find("td")
	if idx, ok := keys["links"]; ok && idx < cells.Length() {
		cells.Eq(idx).Find("a[href]").Each(func(_ int, anchor *goquery.Selection) {
			link, _ := anchor.Attr("href")
			if safeExternalURL(link) {
				value.Links = append(value.Links, [2]string{link, strings.TrimSpace(anchor.Text())})
			}
		})
	}
	if len(value.Links) == 0 {
		value.Errors = append(value.Errors, "Links column must have at least one link")
	}
	if idx, ok := keys["logo"]; ok && idx < cells.Length() {
		logo := cells.Eq(idx).Find("a[href]").First()
		if logo.Length() > 0 {
			value.Logo, _ = logo.Attr("href")
		} else {
			value.Logo, _ = cells.Eq(idx).Find("img[src]").First().Attr("src")
		}
	}
	if !safeExternalURL(value.Logo) {
		value.Logo = ""
		value.Errors = append(value.Errors, "Logo table column must have a link")
	}
	return value, nil
}

func safeExternalURL(raw string) bool {
	u, err := url.Parse(raw)
	return err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != ""
}

func renderGenericNotice(spec genericWiki, title string, content genericWikiContent) string {
	query := url.Values{}
	query.Set("search", title)
	query.Set("go", "Go")
	search := spec.Search + "?" + query.Encode()
	var b strings.Builder
	b.WriteString(`<aside class="niwa__notice"><div class="niwa__summary"><strong class="niwa__header">`)
	b.WriteString(html.EscapeString(spec.Name))
	b.WriteString(` has its own website separate from Fandom.</strong><a class="niwa__go" href="`)
	b.WriteString(html.EscapeString(search))
	b.WriteString(`">Read `)
	b.WriteString(html.EscapeString(title))
	b.WriteString(` on `)
	b.WriteString(html.EscapeString(spec.Name))
	b.WriteString(` →</a></div><details class="niwa__details"><summary>More information</summary><div class="niwa__cols"><div class="niwa__left">`)
	for _, para := range content.Paragraphs {
		b.WriteString(para)
	}
	b.WriteString(`<p>This external wiki is a helpful alternative to Fandom. You should <a href="`)
	b.WriteString(html.EscapeString(search))
	b.WriteString(`">check it out now!</a></p></div>`)
	if content.Logo != "" {
		b.WriteString(`<div class="niwa__right"><img class="niwa__logo" src="`)
		b.WriteString(html.EscapeString(content.Logo))
		b.WriteString(`"></div>`)
	}
	b.WriteString(`</div>`)
	if len(content.Links) != 0 {
		b.WriteString(`<p class="niwa__feedback">`)
		for i, item := range content.Links {
			if i > 0 {
				b.WriteString(" / ")
			}
			b.WriteString(`<a href="`)
			b.WriteString(html.EscapeString(item[0]))
			b.WriteString(`">`)
			b.WriteString(html.EscapeString(item[1]))
			b.WriteString(`</a>`)
		}
		b.WriteString(`</p>`)
	}
	if len(content.Errors) != 0 {
		b.WriteString(`<ul>`)
		for _, message := range content.Errors {
			b.WriteString(`<li>`)
			b.WriteString(html.EscapeString(message))
			b.WriteString(`</li>`)
		}
		b.WriteString(`</ul>`)
	}
	b.WriteString(`</details></aside>`)
	return b.String()
}
