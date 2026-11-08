package archive

import (
	"bufio"
	"context"
	"database/sql"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

//go:embed mime.types
var mimeFiles embed.FS

type HTTPDoer interface {
	Do(*http.Request) (*http.Response, error)
}
type Progress func(done, total int, name string)
type StageProgress func(stage, done, total int, name string)

var StageNames = [...]string{"List pages", "Download pages and redirects", "Download styles", "Download style images"}

func (a *Archive) Run(ctx context.Context, wiki string, progress StageProgress) error {
	for stage := range StageNames {
		if err := a.RunStage(ctx, wiki, stage, func(done, total int, name string) {
			if progress != nil {
				progress(stage, done, total, name)
			}
		}); err != nil {
			return fmt.Errorf("stage %d (%s): %w", stage+1, StageNames[stage], err)
		}
	}
	return nil
}

func (a *Archive) RunStage(ctx context.Context, wiki string, stage int, progress Progress) error {
	if !wikiName.MatchString(wiki) {
		return fmt.Errorf("invalid Fandom wiki name %q", wiki)
	}
	if progress == nil {
		progress = func(int, int, string) {}
	}
	switch stage {
	case 0:
		return a.listPages(ctx, wiki, progress)
	case 1:
		return a.savePages(ctx, wiki, progress)
	case 2:
		return a.saveStyles(ctx, wiki, progress)
	case 3:
		return a.saveImages(ctx, wiki, progress)
	default:
		return fmt.Errorf("invalid stage %d", stage)
	}
}

var wikiName = regexp.MustCompile(`^[a-zA-Z0-9-]{1,50}$`)

func ValidWiki(wiki string) bool { return wikiName.MatchString(wiki) }

func (a *Archive) origin(wiki string) string {
	if a.Origin != nil {
		return strings.TrimRight(a.Origin(wiki), "/")
	}
	return "https://" + wiki + ".fandom.com"
}

func (a *Archive) get(ctx context.Context, raw string) ([]byte, http.Header, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, raw, nil)
	if err != nil {
		return nil, nil, err
	}
	req.Header.Set("User-Agent", "BreezeWiki-Go-Archiver/1.0")
	client := a.Client
	if client == nil {
		client = http.DefaultClient
	}
	res, err := client.Do(req)
	if err != nil {
		return nil, nil, err
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return nil, nil, fmt.Errorf("GET %s: %s", raw, res.Status)
	}
	body, err := io.ReadAll(res.Body)
	return body, res.Header, err
}

func (a *Archive) api(ctx context.Context, wiki string, pairs ...string) ([]byte, error) {
	q := url.Values{}
	for i := 0; i < len(pairs); i += 2 {
		q.Set(pairs[i], pairs[i+1])
	}
	q.Set("format", "json")
	q.Set("formatversion", "2")
	body, _, err := a.get(ctx, a.origin(wiki)+"/api.php?"+q.Encode())
	if err != nil {
		return nil, err
	}
	var document struct {
		Error json.RawMessage `json:"error"`
	}
	if err := json.Unmarshal(body, &document); err != nil {
		return nil, err
	}
	if len(document.Error) > 0 {
		return nil, fmt.Errorf("MediaWiki API error: %s", document.Error)
	}
	return body, nil
}

func progressOfWiki(a *Archive, wiki string) (int, error) {
	var progress int
	err := a.DB.QueryRow(`select progress from wiki where wikiname = ?`, wiki).Scan(&progress)
	if errors.Is(err, sql.ErrNoRows) {
		return -1, nil
	}
	return progress, err
}

func (a *Archive) listPages(ctx context.Context, wiki string, callback Progress) error {
	p, err := progressOfWiki(a, wiki)
	if err != nil || p >= 1 {
		return err
	}
	data, err := a.api(ctx, wiki, "action", "query", "meta", "siteinfo", "siprop", "general|rightsinfo|statistics|namespaces")
	if err != nil {
		return err
	}
	var site struct {
		Query struct {
			General struct {
				Sitename string `json:"sitename"`
				Base     string `json:"base"`
			} `json:"general"`
			Rightsinfo struct {
				Text string `json:"text"`
				URL  string `json:"url"`
			} `json:"rightsinfo"`
			Statistics struct {
				Articles int `json:"articles"`
			} `json:"statistics"`
			Namespaces map[string]struct {
				ID      int  `json:"id"`
				Content bool `json:"content"`
			} `json:"namespaces"`
		} `json:"query"`
	}
	if err := json.Unmarshal(data, &site); err != nil {
		return err
	}
	basePage := site.Query.General.Base
	if i := strings.Index(basePage, "/wiki/"); i >= 0 {
		basePage = basePage[i+6:]
	} else {
		return fmt.Errorf("siteinfo base has no /wiki/: %q", basePage)
	}
	_, err = a.DB.Exec(`insert into wiki (wikiname, progress, sitename, basepage, license_text, license_url) values (?, 0, ?, ?, ?, ?) on conflict(wikiname) do update set sitename=excluded.sitename, basepage=excluded.basepage, license_text=excluded.license_text, license_url=excluded.license_url`, wiki, site.Query.General.Sitename, basePage, site.Query.Rightsinfo.Text, site.Query.Rightsinfo.URL)
	if err != nil {
		return err
	}
	var namespaces []int
	for _, ns := range site.Query.Namespaces {
		if ns.Content && ns.ID < 2900 {
			namespaces = append(namespaces, ns.ID)
		}
	}
	sort.Ints(namespaces)
	total := 0
	for _, ns := range namespaces {
		for _, filter := range []string{"nonredirects", "redirects"} {
			var basenames []string
			continuation := ""
			for {
				if err := ctx.Err(); err != nil {
					return err
				}
				pages, err := a.api(ctx, wiki, "action", "query", "list", "allpages", "apnamespace", strconv.Itoa(ns), "apfilterredir", filter, "aplimit", "500", "apcontinue", continuation)
				if err != nil {
					return err
				}
				var result struct {
					Query struct {
						Allpages []struct {
							Title string `json:"title"`
						} `json:"allpages"`
					} `json:"query"`
					Continue struct {
						APContinue string `json:"apcontinue"`
					} `json:"continue"`
				}
				if err := json.Unmarshal(pages, &result); err != nil {
					return err
				}
				for _, page := range result.Query.Allpages {
					basenames = append(basenames, TitleBasename(page.Title))
				}
				if len(result.Query.Allpages) > 0 {
					n := total
					if filter == "nonredirects" {
						n += len(basenames)
					}
					callback(n, site.Query.Statistics.Articles, basenames[len(basenames)-1])
				}
				if result.Continue.APContinue == "" {
					break
				}
				continuation = result.Continue.APContinue
			}
			if filter == "nonredirects" {
				total += len(basenames)
			}
			if err := a.insertPages(ctx, wiki, basenames, filter == "redirects"); err != nil {
				return err
			}
		}
	}
	_, err = a.DB.ExecContext(ctx, `update wiki set progress = 1 where wikiname = ?`, wiki)
	return err
}

func (a *Archive) insertPages(ctx context.Context, wiki string, basenames []string, redirect bool) error {
	tx, err := a.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	stmt, err := tx.PrepareContext(ctx, `insert or ignore into page (wikiname, redirect, basename, progress) values (?, ?, ?, 0)`)
	if err != nil {
		tx.Rollback()
		return err
	}
	var marker any
	if redirect {
		marker = 1
	}
	for _, name := range basenames {
		if _, err := stmt.ExecContext(ctx, wiki, marker, name); err != nil {
			stmt.Close()
			tx.Rollback()
			return err
		}
	}
	if err := stmt.Close(); err != nil {
		tx.Rollback()
		return err
	}
	return tx.Commit()
}

func (a *Archive) savePages(ctx context.Context, wiki string, callback Progress) error {
	root := filepath.Join(a.Storage, "archive", wiki)
	if err := os.MkdirAll(root, 0755); err != nil {
		return err
	}
	var already, remaining int
	if err := a.DB.QueryRow(`select count(*) from page where wikiname = ? and progress = 1`, wiki).Scan(&already); err != nil {
		return err
	}
	if err := a.DB.QueryRow(`select count(*) from page where wikiname = ? and progress < 1`, wiki).Scan(&remaining); err != nil {
		return err
	}
	total := already + remaining
	callback(already, total, "")
	rows, err := a.DB.QueryContext(ctx, `select basename from page where wikiname = ? and progress < 1 and redirect is null`, wiki)
	if err != nil {
		return err
	}
	var names []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			rows.Close()
			return err
		}
		names = append(names, name)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	for i, basename := range names {
		name, err := BasenameTitle(basename)
		if err != nil {
			return err
		}
		q := url.Values{"action": {"parse"}, "page": {name}, "prop": {"text|headhtml|langlinks"}, "formatversion": {"2"}, "format": {"json"}}
		body, _, err := a.get(ctx, a.origin(wiki)+"/api.php?"+q.Encode())
		if err != nil {
			return err
		}
		if err := json.Valid(body); !err {
			return fmt.Errorf("invalid JSON for %s", basename)
		}
		if len(basename) > 240 {
			_, err = a.DB.ExecContext(ctx, `insert or ignore into special_page (wikiname, key, basename) values (?, ?, ?)`, wiki, PageKey(basename), basename)
			if err != nil {
				return err
			}
		}
		if err := os.WriteFile(PagePath(a.Storage, wiki, basename, ".json"), body, 0644); err != nil {
			return err
		}
		if _, err := a.DB.ExecContext(ctx, `update page set progress = 1 where wikiname = ? and basename = ?`, wiki, basename); err != nil {
			return err
		}
		callback(already+i+1, total, basename)
	}
	nonRedirectCount := len(names)
	rows, err = a.DB.QueryContext(ctx, `select basename from page where wikiname = ? and progress < 1 and redirect = 1`, wiki)
	if err != nil {
		return err
	}
	names = nil
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			rows.Close()
			return err
		}
		names = append(names, name)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	for i, basename := range names {
		name, err := BasenameTitle(basename)
		if err != nil {
			return err
		}
		pages, err := a.api(ctx, wiki, "action", "query", "prop", "links", "titles", name)
		if err != nil {
			return err
		}
		var result struct {
			Query struct {
				Pages []struct {
					Links []struct {
						Title string `json:"title"`
					} `json:"links"`
				} `json:"pages"`
			} `json:"query"`
		}
		if err := json.Unmarshal(pages, &result); err != nil {
			return err
		}
		if len(result.Query.Pages) > 0 && len(result.Query.Pages[0].Links) > 0 {
			_, err = a.DB.ExecContext(ctx, `update page set progress = 1, redirect = ? where wikiname = ? and basename = ?`, TitleBasename(result.Query.Pages[0].Links[0].Title), wiki, basename)
		} else {
			_, err = a.DB.ExecContext(ctx, `delete from page where wikiname = ? and basename = ?`, wiki, basename)
		}
		if err != nil {
			return err
		}
		callback(already+nonRedirectCount+i+1, total, basename)
	}
	_, err = a.DB.ExecContext(ctx, `update wiki set progress = 2 where wikiname = ? and progress <= 2`, wiki)
	return err
}

var cssURL = regexp.MustCompile(`url\(\s*['"]?([^)'"]*?)['"]?\s*\)`)
var ignoredCSSURLs = []string{"/resources-ucp/", "/fonts/", "/drm_fonts/", "//db.onlinewebfonts.com/", "//bits.wikimedia.org/", "mygamercard.net/", "dropbox", "only=styles", "https://https://"}

func CSSImageURLs(wiki, css string) ([]string, error) {
	var urls []string
	for _, match := range cssURL.FindAllStringSubmatch(css, -1) {
		raw := match[1]
		if raw == "" || raw == "'" || strings.HasPrefix(raw, "%20") || strings.HasPrefix(raw, "data:") || strings.HasPrefix(raw, "file:") {
			continue
		}
		skip := false
		for _, ignored := range ignoredCSSURLs {
			if strings.Contains(raw, ignored) {
				skip = true
				break
			}
		}
		if skip {
			continue
		}
		switch {
		case strings.HasPrefix(raw, "https://"):
		case strings.HasPrefix(raw, "http://"):
			raw = "https:" + strings.TrimPrefix(raw, "http:")
		case strings.HasPrefix(raw, "httpshttps://"):
			raw = "https://" + strings.TrimPrefix(raw, "httpshttps://")
		case strings.HasPrefix(raw, "//"):
			raw = "https:" + raw
		case strings.HasPrefix(raw, "/"):
			raw = "https://" + wiki + ".fandom.com" + raw
		default:
			return nil, fmt.Errorf("unknown CSS image URL %q", raw)
		}
		urls = append(urls, raw)
	}
	return urls, nil
}

func (a *Archive) saveStyles(ctx context.Context, wiki string, callback Progress) error {
	p, err := progressOfWiki(a, wiki)
	if err != nil || p >= 3 {
		return err
	}
	dir := filepath.Join(a.Storage, "archive", wiki, "styles")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	styles := [][2]string{}
	for _, theme := range []string{"default", "light", "dark"} {
		styles = append(styles, [2]string{a.origin(wiki) + "/wikia.php?controller=ThemeApi&method=themeVariables&variant=" + theme, "themeVariables-" + theme + ".css"})
	}
	styles = append(styles, [2]string{a.origin(wiki) + "/load.php?lang=en&modules=site.styles%7Cskin.fandomdesktop.styles%7Cext.fandom.PortableInfoboxFandomDesktop.css%7Cext.fandom.GlobalComponents.CommunityHeaderBackground.css%7Cext.gadget.site-styles%2Csound-styles&only=styles&skin=fandomdesktop", "site.css"})
	seen := map[string]struct{}{}
	for i, pair := range styles {
		callback(i, len(styles), "styles...")
		body, _, err := a.get(ctx, pair[0])
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(dir, pair[1]), body, 0644); err != nil {
			return err
		}
		urls, err := CSSImageURLs(wiki, string(body))
		if err != nil {
			return fmt.Errorf("%s: %w", pair[1], err)
		}
		for _, raw := range urls {
			clean, hash := ImageKey(raw)
			if _, ok := seen[hash]; ok {
				continue
			}
			seen[hash] = struct{}{}
			if _, err := a.DB.ExecContext(ctx, `insert or ignore into image (wikiname, url, hash, ext, source, progress) values (?, ?, ?, NULL, 1, 0)`, wiki, clean, hash); err != nil {
				return err
			}
		}
	}
	callback(len(styles), len(styles), "styles...")
	_, err = a.DB.ExecContext(ctx, `update wiki set progress = 3 where wikiname = ?`, wiki)
	return err
}

func imageExtension(contentType string, body []byte) (string, error) {
	if contentType == "application/octet-stream" {
		contentType = http.DetectContentType(body)
	}
	parsed, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		return "", err
	}
	file, err := mimeFiles.Open("mime.types")
	if err != nil {
		return "", err
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	var ext string
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) >= 2 && fields[0] == parsed {
			ext = fields[1]
		}
	}
	if err := scanner.Err(); err != nil {
		return "", err
	}
	if ext == "" {
		return "", fmt.Errorf("no extension for MIME type %q", contentType)
	}
	return ext, nil
}

func (a *Archive) saveImages(ctx context.Context, wiki string, callback Progress) error {
	var already, remaining int
	if err := a.DB.QueryRow(`select count(*) from image where wikiname = ? and source <= 1 and progress = 1`, wiki).Scan(&already); err != nil {
		return err
	}
	if err := a.DB.QueryRow(`select count(*) from image where wikiname = ? and source <= 1 and progress < 1`, wiki).Scan(&remaining); err != nil {
		return err
	}
	total := already + remaining
	callback(already, total, "")
	rows, err := a.DB.QueryContext(ctx, `select url, hash from image where wikiname = ? and source <= 1 and progress < 1`, wiki)
	if err != nil {
		return err
	}
	type imageRecord struct{ URL, Hash string }
	var images []imageRecord
	for rows.Next() {
		var item imageRecord
		if err := rows.Scan(&item.URL, &item.Hash); err != nil {
			rows.Close()
			return err
		}
		images = append(images, item)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	for i, item := range images {
		body, headers, err := a.get(ctx, item.URL)
		if err != nil {
			return err
		}
		ext, err := imageExtension(headers.Get("Content-Type"), body)
		if err != nil {
			return fmt.Errorf("%s: %w", item.URL, err)
		}
		dir := ImageDir(a.Storage, wiki, item.Hash)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(dir, item.Hash+"."+ext), body, 0644); err != nil {
			return err
		}
		if _, err := a.DB.ExecContext(ctx, `update image set progress = 1, ext = ? where wikiname = ? and hash = ?`, ext, wiki, item.Hash); err != nil {
			return err
		}
		callback(already+i+1, total, item.Hash[:6]+"..."+ext)
	}
	_, err = a.DB.ExecContext(ctx, `update wiki set progress = 4 where wikiname = ?`, wiki)
	return err
}

func (a *Archive) Reset(wiki string, deletePages bool) error {
	if !ValidWiki(wiki) {
		return fmt.Errorf("invalid wiki name %q", wiki)
	}
	tx, err := a.DB.Begin()
	if err != nil {
		return err
	}
	if _, err := tx.Exec(`update wiki set progress = 0 where wikiname = ?`, wiki); err != nil {
		tx.Rollback()
		return err
	}
	if deletePages {
		if _, err := tx.Exec(`delete from page where wikiname = ?`, wiki); err != nil {
			tx.Rollback()
			return err
		}
	}
	return tx.Commit()
}

func (a *Archive) Pending() (map[string]int, error) {
	rows, err := a.DB.Query(`select wikiname, progress from wiki where progress < 4`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := map[string]int{}
	for rows.Next() {
		var name string
		var stage int
		if err := rows.Scan(&name, &stage); err != nil {
			return nil, err
		}
		result[name] = stage
	}
	return result, rows.Err()
}
