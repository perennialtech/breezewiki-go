package server

import (
	"encoding/json"
	"fmt"
	"html"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/perennialtech/breezewiki-go/internal/fandom"
)

type commentPost struct {
	JSONModel string `json:"jsonModel"`
	CreatedBy struct {
		Name string `json:"name"`
	} `json:"createdBy"`
	CreationDate struct {
		EpochSecond int64 `json:"epochSecond"`
	} `json:"creationDate"`
	Attachments struct {
		ContentImages []struct {
			Position int    `json:"position"`
			URL      string `json:"url"`
		} `json:"contentImages"`
	} `json:"attachments"`
}

type commentPageData struct {
	Threads []struct {
		FirstPost commentPost   `json:"firstPost"`
		Posts     []commentPost `json:"posts"`
	} `json:"threads"`
	TotalCount int             `json:"totalCount"`
	Links      json.RawMessage `json:"links"`
}

type commentNode struct {
	Type    string        `json:"type"`
	Text    string        `json:"text"`
	Content []commentNode `json:"content"`
	Attrs   struct {
		ID int `json:"id"`
	} `json:"attrs"`
	Marks []struct {
		Type  string `json:"type"`
		Attrs struct {
			Href string `json:"href"`
		} `json:"attrs"`
	} `json:"marks"`
}

func commentContent(model string, post commentPost) string {
	var doc commentNode
	if err := json.Unmarshal([]byte(model), &doc); err != nil {
		return ""
	}
	var b strings.Builder
	var node func(commentNode)
	node = func(n commentNode) {
		tag := ""
		switch n.Type {
		case "paragraph":
			tag = "p"
		case "heading":
			tag = "strong"
		case "blockquote":
			tag = "blockquote"
		case "bulletList":
			tag = "ul"
		case "orderedList":
			tag = "ol"
		case "listItem":
			tag = "li"
		case "codeBlock":
			tag = "pre"
		case "hardBreak":
			b.WriteString("<br>")
			return
		case "image":
			for _, image := range post.Attachments.ContentImages {
				if image.Position == n.Attrs.ID && fandom.AllowedProxyURL(image.URL) {
					fmt.Fprintf(&b, `<img loading="lazy" src="%s" alt="Comment image">`, html.EscapeString(image.URL))
					break
				}
			}
			return
		case "text":
			value := html.EscapeString(n.Text)
			for _, mark := range n.Marks {
				switch mark.Type {
				case "strong":
					value = "<strong>" + value + "</strong>"
				case "em":
					value = "<em>" + value + "</em>"
				case "strike":
					value = "<s>" + value + "</s>"
				case "code":
					value = "<code>" + value + "</code>"
				case "link":
					if u, err := url.Parse(mark.Attrs.Href); err == nil && (u.Scheme == "https" || u.Scheme == "http") && u.Host != "" {
						value = `<a rel="nofollow noopener noreferrer" href="` + html.EscapeString(u.String()) + `">` + value + `</a>`
					}
				}
			}
			b.WriteString(value)
			return
		}
		if tag != "" {
			b.WriteString("<" + tag + ">")
		}
		for _, child := range n.Content {
			node(child)
		}
		if tag != "" {
			b.WriteString("</" + tag + ">")
		}
	}
	node(doc)
	return b.String()
}

func writeComment(b *strings.Builder, post commentPost) {
	b.WriteString(`<div class="bw-comments__post"><div class="bw-comments__meta"><strong>`)
	author := post.CreatedBy.Name
	if author == "" {
		author = "Anonymous"
	}
	b.WriteString(html.EscapeString(author))
	b.WriteString(`</strong>`)
	if post.CreationDate.EpochSecond > 0 {
		at := time.Unix(post.CreationDate.EpochSecond, 0).UTC()
		fmt.Fprintf(b, ` <time datetime="%s">%s</time>`, at.Format(time.RFC3339), at.Format("2006-01-02"))
	}
	b.WriteString(`</div><div class="bw-comments__body">`)
	b.WriteString(commentContent(post.JSONModel, post))
	b.WriteString(`</div></div>`)
}

func commentsPlaceholder(wiki, title string) string {
	target := "/" + wiki + "/comments?title=" + url.QueryEscape(title)
	return `<section id="comments" class="bw-comments" data-comments-url="` + html.EscapeString(target) + `"><h2>Comments</h2><div class="bw-comments__results">Loading comments…</div></section>`
}

func (s *Server) comments(w http.ResponseWriter, r *http.Request, wiki string) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	title := r.URL.Query().Get("title")
	if title == "" {
		http.Error(w, "Article title required", http.StatusBadRequest)
		return
	}
	page := 1
	if raw := r.URL.Query().Get("page"); raw != "" {
		var err error
		page, err = strconv.Atoi(raw)
		if err != nil || page < 1 {
			http.Error(w, "Invalid comments page", http.StatusBadRequest)
			return
		}
	}
	q := url.Values{}
	q.Set("controller", "ArticleCommentsController")
	q.Set("method", "getComments")
	q.Set("title", title)
	q.Set("namespace", "0")
	q.Set("page", strconv.Itoa(page))
	result, err := s.Fandom.GetWithoutDenyCache(r.Context(), wiki, "/wikia.php?"+q.Encode(), "")
	if err != nil || result.Status != http.StatusOK {
		http.Error(w, "Comments are unavailable", http.StatusBadGateway)
		return
	}
	var data commentPageData
	if err := json.Unmarshal(result.Body, &data); err != nil {
		http.Error(w, "Invalid comments response", http.StatusBadGateway)
		return
	}
	var b strings.Builder
	fmt.Fprintf(&b, `<p class="bw-comments__count">%d comments</p>`, data.TotalCount)
	if len(data.Threads) == 0 {
		b.WriteString("<p>No comments on this page.</p>")
	}
	if len(data.Threads) > 0 {
		b.WriteString(`<ol class="bw-comments__threads">`)
	}
	for _, thread := range data.Threads {
		b.WriteString("<li>")
		writeComment(&b, thread.FirstPost)
		if len(thread.Posts) > 0 {
			b.WriteString(`<ol class="bw-comments__replies">`)
			for _, post := range thread.Posts {
				b.WriteString("<li>")
				writeComment(&b, post)
				b.WriteString("</li>")
			}
			b.WriteString("</ol>")
		}
		b.WriteString("</li>")
	}
	if len(data.Threads) > 0 {
		b.WriteString("</ol>")
	}
	var links map[string]string
	_ = json.Unmarshal(data.Links, &links)
	if page > 1 || links["next"] != "" {
		b.WriteString(`<nav class="bw-comments__pages" aria-label="Comment pages">`)
		if page > 1 {
			fmt.Fprintf(&b, `<button type="button" data-comments-page="%d">Newer comments</button>`, page-1)
		}
		if links["next"] != "" {
			fmt.Fprintf(&b, `<button type="button" data-comments-page="%d">Older comments</button>`, page+1)
		}
		b.WriteString("</nav>")
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(b.String()))
}
