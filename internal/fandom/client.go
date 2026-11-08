// Package fandom implements BreezeWiki's MediaWiki and static-resource requests.
package fandom

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/perennialtech/breezewiki-go/internal/config"
)

var wikiPattern = regexp.MustCompile(`^[a-zA-Z0-9-]{1,50}$`)
var fandomURL = regexp.MustCompile(`^https://(?:static\.wikia\.nocookie\.net/|[a-zA-Z0-9-]{1,50}\.fandom\.com/)`)

// AllowedProxyURL accepts only the upstream's two approved HTTPS hosts.
func AllowedProxyURL(raw string) bool { return fandomURL.MatchString(raw) }
func ValidWiki(wiki string) bool      { return wikiPattern.MatchString(wiki) }

type Param struct{ Key, Value string }
type Client struct {
	HTTP *http.Client
	// Origin replaces https://<wiki>.fandom.com, primarily for mock upstreams.
	Origin func(wiki string) string
	Config *config.Config

	mu           sync.Mutex
	deniedUntil  time.Time
	deniedStatus int
	deniedBody   []byte
	info         map[string]map[string]any
}

func New(cfg *config.Config) *Client {
	return &Client{HTTP: &http.Client{Timeout: 5 * time.Second}, Config: cfg,
		Origin: func(wiki string) string { return "https://" + wiki + ".fandom.com" },
		info:   make(map[string]map[string]any)}
}

// Response includes both the HTTP status and the body; MediaWiki 200 responses
// may contain an API-level error, including missingtitle.
type Response struct {
	Status int
	Body   []byte
	Header http.Header
}

func (r Response) JSON() (map[string]any, error) {
	var data map[string]any
	if err := json.Unmarshal(r.Body, &data); err != nil {
		return nil, err
	}
	return data, nil
}
func (c *Client) Get(ctx context.Context, wiki, path, theme string) (Response, error) {
	if !ValidWiki(wiki) {
		return Response{}, fmt.Errorf("invalid wiki %q", wiki)
	}
	c.mu.Lock()
	if time.Now().Before(c.deniedUntil) {
		r := Response{Status: c.deniedStatus, Body: append([]byte(nil), c.deniedBody...)}
		c.mu.Unlock()
		return r, nil
	}
	c.mu.Unlock()
	origin := c.Origin(wiki)
	u, err := url.Parse(origin + path)
	if err != nil {
		return Response{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return Response{}, err
	}
	req.Header.Set("User-Agent", "BreezeWiki/1.0 ("+canonical(c.Config)+")")
	if theme != "" {
		req.Header.Set("Cookie", "theme="+theme)
	}
	if c.Config != nil && c.Config.True("log_outgoing") {
		log.Printf("out: %s %s", wiki, path)
	}
	client := c.HTTP
	if client == nil {
		client = http.DefaultClient
	}
	res, err := client.Do(req)
	if err != nil {
		return Response{}, err
	}
	defer res.Body.Close()
	body, err := io.ReadAll(res.Body)
	if err != nil {
		return Response{}, err
	}
	r := Response{Status: res.StatusCode, Body: body, Header: res.Header.Clone()}
	if r.Status == 403 || r.Status == 406 {
		c.mu.Lock()
		c.deniedUntil = time.Now().Add(30 * time.Second)
		c.deniedStatus = r.Status
		c.deniedBody = append([]byte(nil), r.Body...)
		c.mu.Unlock()
	}
	return r, nil
}
func canonical(cfg *config.Config) string {
	if cfg == nil || !cfg.True("canonical_origin") {
		return "local"
	}
	return cfg.Get("canonical_origin")
}
func (c *Client) API(ctx context.Context, wiki string, params []Param, theme string) (Response, error) {
	q := url.Values{}
	for _, p := range params {
		q.Add(p.Key, p.Value)
	}
	return c.Get(ctx, wiki, "/api.php?"+q.Encode(), theme)
}

var SiteInfoParams = []Param{
	{"action", "query"}, {"meta", "siteinfo"}, {"siprop", "general|rightsinfo"},
	{"format", "json"}, {"formatversion", "2"},
}
var DefaultSiteInfo = map[string]any{"query": map[string]any{
	"general":    map[string]any{"sitename": "Unknown Wiki", "base": "/wiki/Main_Page"},
	"rightsinfo": map[string]any{"text": "CC-BY-SA", "url": "https://www.fandom.com/licensing"},
}}

// SiteInfo mirrors upstream's memoized siteinfo-fetch: unsuccessful requests
// produce defaults; successful results are cached for the process lifetime.
func (c *Client) SiteInfo(ctx context.Context, wiki string) map[string]any {
	c.mu.Lock()
	if c.info == nil {
		c.info = make(map[string]map[string]any)
	}
	if data, ok := c.info[wiki]; ok {
		c.mu.Unlock()
		return data
	}
	c.mu.Unlock()
	data := DefaultSiteInfo
	res, err := c.API(ctx, wiki, SiteInfoParams, "")
	if err == nil && res.Status == 200 {
		if parsed, err := res.JSON(); err == nil {
			if _, ok := parsed["query"].(map[string]any); ok {
				data = parsed
			}
		}
	}
	c.mu.Lock()
	c.info[wiki] = data
	c.mu.Unlock()
	return data
}
func (c *Client) SiteInfoFromPost(v any) map[string]any {
	if m, ok := v.(map[string]any); ok {
		return m
	}
	return DefaultSiteInfo
}
func General(data map[string]any) (name, base string) {
	name = "Unknown Wiki"
	base = "Main_Page"
	q, _ := data["query"].(map[string]any)
	g, _ := q["general"].(map[string]any)
	if s, ok := g["sitename"].(string); ok && s != "" {
		name = s
	}
	if s, ok := g["base"].(string); ok {
		if i := strings.Index(s, "/wiki/"); i >= 0 {
			base = s[i+6:]
		}
	}
	return
}
func Value(data any, keys ...string) any {
	var v any = data
	for _, k := range keys {
		m, ok := v.(map[string]any)
		if !ok {
			return nil
		}
		v = m[k]
	}
	return v
}
func String(data any, keys ...string) string {
	if s, ok := Value(data, keys...).(string); ok {
		return s
	}
	return ""
}
func ErrorCode(data map[string]any) string { return String(data, "error", "code") }
func ParseText(data map[string]any) string {
	parse, _ := data["parse"].(map[string]any)
	switch v := parse["text"].(type) {
	case string:
		return v
	case map[string]any:
		if s, ok := v["*"].(string); ok {
			return s
		}
	}
	return ""
}
