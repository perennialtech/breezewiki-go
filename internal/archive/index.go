package archive

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"golang.org/x/net/html"
)

// Document has the Solr fields used by the bundled schema and search provider.
type Document struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	Body  string `json:"body"`
	Table string `json:"table"`
	Len   int    `json:"len"`
}

var whitespaceRuns = regexp.MustCompile(`(?m)([ \t]*\r?\n+)+`)
var rejectedClassSubstrings = [...]string{"collapsed", "selflink", "label", "toc", "editsection", "reviews"}

func hiddenByClass(node *html.Node) bool {
	if node.Type != html.ElementNode {
		return false
	}
	for _, attribute := range node.Attr {
		if attribute.Key != "class" {
			continue
		}
		for _, fragment := range rejectedClassSubstrings {
			if strings.Contains(attribute.Val, fragment) {
				return true
			}
		}
	}
	return false
}

func shouldSkipElement(node *html.Node, tables bool) bool {
	if node.Type != html.ElementNode {
		return false
	}
	if tables {
		switch node.Data {
		case "h1", "h2", "h3", "p", "blockquote", "q":
			return true
		}
	} else {
		switch node.Data {
		case "ul", "ol", "dl", "table":
			return true
		}
	}
	return false
}

func extractText(node *html.Node, tables bool) string {
	var out strings.Builder
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.CommentNode {
			return
		}
		if hiddenByClass(n) {
			// Racket replaces each rejected node with an empty div.
			out.WriteByte('\n')
			return
		}
		if shouldSkipElement(n, tables) {
			return
		}
		if n.Type == html.TextNode {
			out.WriteString(n.Data)
			return
		}
		if n.Type == html.ElementNode {
			switch n.Data {
			case "div", "p", "li", "td", "dd", "dt", "br":
				out.WriteByte('\n')
			}
		}
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(node)
	return whitespaceRuns.ReplaceAllString(out.String(), "\n")
}

func ExtractDocument(raw []byte) (Document, error) {
	var response struct {
		Parse struct {
			Title  string      `json:"title"`
			PageID json.Number `json:"pageid"`
			Text   string      `json:"text"`
		} `json:"parse"`
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(&response); err != nil {
		return Document{}, err
	}
	if response.Parse.PageID == "" {
		return Document{}, fmt.Errorf("page has no parse/pageid")
	}
	if response.Parse.Title == "" {
		return Document{}, fmt.Errorf("page has no parse/title")
	}
	root, err := html.Parse(strings.NewReader(response.Parse.Text))
	if err != nil {
		return Document{}, err
	}
	body := extractText(root, false)
	return Document{ID: response.Parse.PageID.String(), Title: response.Parse.Title, Body: body, Table: extractText(root, true), Len: utf8.RuneCountInString(body)}, nil
}

func readPageFile(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var reader io.Reader = f
	if strings.HasSuffix(path, ".json.gz") {
		z, err := gzip.NewReader(f)
		if err != nil {
			return nil, err
		}
		defer z.Close()
		reader = z
	}
	return io.ReadAll(reader)
}

// BuildDocuments reads the original .json and .json.gz archive formats.
// A compressed/uncompressed copy of the same page is indexed only once.
func (a *Archive) BuildDocuments(ctx context.Context, wiki string, callback Progress) ([]Document, error) {
	if !ValidWiki(wiki) {
		return nil, fmt.Errorf("invalid wiki name %q", wiki)
	}
	root := filepath.Join(a.Storage, "archive", wiki)
	files, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}
	selected := map[string]string{}
	for _, file := range files {
		if file.IsDir() {
			continue
		}
		name := file.Name()
		suffix := ""
		switch {
		case strings.HasSuffix(name, ".json.gz"):
			suffix = ".json.gz"
		case strings.HasSuffix(name, ".json"):
			suffix = ".json"
		default:
			continue
		}
		key := strings.TrimSuffix(name, suffix)
		if _, ok := selected[key]; !ok || suffix == ".json" {
			selected[key] = filepath.Join(root, name)
		}
	}
	paths := make([]string, 0, len(selected))
	for _, file := range selected {
		paths = append(paths, file)
	}
	sort.Strings(paths)
	result := make([]Document, 0, len(paths))
	for i, path := range paths {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		body, err := readPageFile(path)
		if err == nil {
			var doc Document
			doc, err = ExtractDocument(body)
			if err == nil {
				result = append(result, doc)
			}
			if err == nil && callback != nil {
				callback(i+1, len(paths), doc.Title)
			}
		}
		if err != nil {
			// The Racket indexer logs bad pages and continues with the rest.
			fmt.Fprintf(os.Stderr, "extract: %s: %v\n", path, err)
			if callback != nil {
				callback(i+1, len(paths), filepath.Base(path))
			}
		}
	}

	return result, nil
}

// IndexOptions controls the Racket indexer's cache and local Solr endpoint.
type IndexOptions struct {
	ReadFromCache bool
	CachePath     string
	SolrURL       string
}

func (a *Archive) Index(ctx context.Context, wiki string, options IndexOptions, callback Progress) error {
	if !ValidWiki(wiki) {
		return fmt.Errorf("invalid wiki name %q", wiki)
	}
	cache := options.CachePath
	if cache == "" {
		cache = "cache.rkt"
	}
	var docs []Document
	if options.ReadFromCache {
		var err error
		docs, err = readIndexCache(cache)
		if os.IsNotExist(err) {
			docs, err = a.BuildDocuments(ctx, wiki, callback)
			if err == nil {
				err = writeIndexCache(cache, docs)
			}
		}
		if err != nil {
			return fmt.Errorf("read index cache: %w", err)
		}
		if callback != nil {
			callback(len(docs), len(docs), "Reading cache...")
		}
	} else {
		var err error
		docs, err = a.BuildDocuments(ctx, wiki, callback)
		if err != nil {
			return err
		}
		if err := writeIndexCache(cache, docs); err != nil {
			return err
		}
	}

	endpoint := options.SolrURL
	if endpoint == "" {
		endpoint = "http://localhost:8983/solr"
	}
	endpoint = strings.TrimRight(endpoint, "/") + "/" + url.PathEscape(wiki) + "/update?commit=true"
	client := a.Client
	if client == nil {
		client = &http.Client{Timeout: 300 * time.Second}
	}
	const size = 30000
	for start := 0; start < len(docs); start += size {
		end := start + size
		if end > len(docs) {
			end = len(docs)
		}
		body, err := json.Marshal(docs[start:end])
		if err != nil {
			return err
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", "application/json")
		res, err := client.Do(req)
		if err != nil {
			return err
		}
		responseBody, err := io.ReadAll(res.Body)
		res.Body.Close()
		if err != nil {
			return err
		}
		if res.StatusCode < 200 || res.StatusCode >= 300 {
			return fmt.Errorf("Solr update: %s: %s", res.Status, responseBody)
		}
		if callback != nil {
			callback(end, len(docs), fmt.Sprintf("Posted %d/%d", (start/size)+1, (len(docs)+size-1)/size))
		}
	}
	return nil
}

// SearchSolr reproduces the query, ranking, and highlighting options of the
// offline Solr search provider. Highlight fragments are HTML (escaped by Solr).
type SearchResult struct {
	ID               string
	Title            string
	Length           int
	HighlightedTitle []string
	HighlightedBody  []string
}

type SearchResponse struct {
	Found       int
	QueryTimeMS int
	Results     []SearchResult
}

func (a *Archive) SearchSolr(ctx context.Context, solrURL, wiki, query, op, sortBy string) (SearchResponse, error) {
	if !ValidWiki(wiki) {
		return SearchResponse{}, fmt.Errorf("invalid wiki name %q", wiki)
	}
	if solrURL == "" {
		solrURL = "http://localhost:8983/solr"
	}
	operator := "AND"
	if op == "or" {
		operator = "OR"
	}
	sort := "score desc"
	if sortBy == "len" {
		sort = "len desc"
	}
	q := url.Values{"defType": {"edismax"}, "q": {query}, "q.op": {operator}, "qf": {"title_prefix title^2.0 body table^0.3"}, "hl": {"true"}, "hl.method": {"unified"}, "hl.defaultSummary": {"true"}, "hl.fl": {"title,body"}, "fl": {"id,len,title"}, "hl.encoder": {"html"}, "hl.tag.pre": {"<mark>"}, "hl.tag.post": {"</mark>"}, "sort": {sort}}
	body, _, err := a.get(ctx, strings.TrimRight(solrURL, "/")+"/"+url.PathEscape(wiki)+"/select?"+q.Encode())
	if err != nil {
		return SearchResponse{}, err
	}
	var parsed struct {
		ResponseHeader struct {
			QTime int `json:"QTime"`
		} `json:"responseHeader"`
		Response struct {
			NumFound int `json:"numFound"`
			Docs     []struct {
				ID    string          `json:"id"`
				Title json.RawMessage `json:"title"`
				Len   int             `json:"len"`
			} `json:"docs"`
		} `json:"response"`
		Highlighting map[string]struct {
			Title []string `json:"title"`
			Body  []string `json:"body"`
		} `json:"highlighting"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return SearchResponse{}, err
	}
	out := SearchResponse{Found: parsed.Response.NumFound, QueryTimeMS: parsed.ResponseHeader.QTime}
	for _, doc := range parsed.Response.Docs {
		var title string
		if err := json.Unmarshal(doc.Title, &title); err != nil {
			var titles []string
			if err := json.Unmarshal(doc.Title, &titles); err != nil {
				return SearchResponse{}, err
			}
			if len(titles) > 0 {
				title = titles[0]
			}
		}
		hl := parsed.Highlighting[doc.ID]
		out.Results = append(out.Results, SearchResult{ID: doc.ID, Title: title, Length: doc.Len, HighlightedTitle: hl.Title, HighlightedBody: hl.Body})
	}
	return out, nil
}
