// Package wikinames downloads Fandom's public directory of newly created wikis.
package wikinames

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

// Wiki is one entry in the NewWikis directory.
type Wiki struct {
	Title string `json:"title"`
	Link  string `json:"link"`
	Lang  string `json:"lang"`
}

// ParsePage extracts the wiki directory and continuation offset from HTML.
func ParsePage(r io.Reader) ([]Wiki, string, error) {
	doc, err := goquery.NewDocumentFromReader(r)
	if err != nil {
		return nil, "", err
	}
	content := doc.Find(".mw-spcontent").First()
	if content.Length() == 0 {
		return nil, "", fmt.Errorf("wiki directory content not found")
	}
	items := make([]Wiki, 0)
	content.Find("li").Each(func(_ int, li *goquery.Selection) {
		a := li.Find("a[href]").First()
		if a.Length() == 0 {
			return
		}
		link, ok := a.Attr("href")
		if !ok || link == "" {
			return
		}
		lang := strings.TrimSpace(li.Clone().ChildrenFiltered("a").Remove().End().Text())
		items = append(items, Wiki{Title: strings.TrimSpace(a.Text()), Link: link, Lang: lang})
	})
	nextHref, exists := content.Find("a.mw-nextlink[href]").First().Attr("href")
	if !exists {
		return items, "", nil
	}
	next, err := url.Parse(nextHref)
	if err != nil {
		return nil, "", fmt.Errorf("invalid next-page URL: %w", err)
	}
	return items, next.Query().Get("offset"), nil
}

// Download enumerates pages until the source provides no continuation link.
func Download(ctx context.Context, client *http.Client, endpoint string, progress func(string, int, int)) ([]Wiki, error) {
	if client == nil {
		client = http.DefaultClient
	}
	all := make([]Wiki, 0)
	offset := ""
	seen := map[string]bool{}
	for {
		if seen[offset] {
			return nil, fmt.Errorf("wiki directory repeated offset %q", offset)
		}
		seen[offset] = true
		u, err := url.Parse(endpoint)
		if err != nil {
			return nil, err
		}
		q := u.Query()
		q.Set("offset", offset)
		q.Set("limit", "5000")
		u.RawQuery = q.Encode()
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("User-Agent", "BreezeWiki/1.0 (https://github.com/perennialtech/breezewiki-go)")
		res, err := client.Do(req)
		if err != nil {
			return nil, err
		}
		if res.StatusCode != http.StatusOK {
			res.Body.Close()
			return nil, fmt.Errorf("wiki directory request: HTTP %d", res.StatusCode)
		}
		items, next, err := ParsePage(res.Body)
		closeErr := res.Body.Close()
		if err != nil {
			return nil, err
		}
		if closeErr != nil {
			return nil, closeErr
		}
		all = append(all, items...)
		if progress != nil {
			progress(offset, len(items), len(all))
		}
		if next == "" {
			return all, nil
		}
		offset = next
	}
}
