package render

import (
	_ "embed"
	"encoding/json"
	"html"
	"net/url"
	"regexp"
	"strings"
)

type independentWiki struct {
	WikiNames   []string `json:"wikinames"`
	Banner      string   `json:"banner"`
	Group       string   `json:"group"`
	Name        string   `json:"name"`
	Home        string   `json:"home"`
	Logo        *string  `json:"logo"`
	Description string   `json:"description"`
}

type wikiGroup struct {
	links       [][2]string
	description string
}

//go:embed notices.json
var noticeData []byte

var independentWikis = func() map[string]independentWiki {
	var entries []independentWiki
	if err := json.Unmarshal(noticeData, &entries); err != nil {
		panic("invalid embedded independent wiki data: " + err.Error())
	}
	index := make(map[string]independentWiki)
	for _, item := range entries {
		for _, wiki := range item.WikiNames {
			if _, exists := index[wiki]; !exists {
				index[wiki] = item
			}
		}
	}
	return index
}()

var independentWikiGroups = map[string]wikiGroup{
	"NIWA": {
		links:       [][2]string{{"Why did editors leave Fandom?", "https://www.kotaku.com.au/2022/10/massive-zelda-wiki-reclaims-independence-six-months-before-tears-of-the-kingdom/"}},
		description: `<p>Most major Nintendo wikis are part of the <a href="https://www.niwanetwork.org/about/">Nintendo Independent Wiki Alliance</a> and have their own wikis off Fandom.</p>`,
	},
	"SEIWA": {
		links:       [][2]string{{"SEIWA Website", "https://seiwanetwork.org/"}},
		description: `<p>The Square Enix Indpendent Wiki Alliance, or SEIWA, is a network of independent wikis established in 2011 and focused on providing high-quality coverage of Square Enix and its content. We work together, along with our affiliates and others, to co-operate and support one another while providing the best-quality content on the various Square Enix video games and media.</p>`,
	},
	"GWN": {
		links:       [][2]string{{"Gaming Wiki Network", "https://gamingwikinetwork.org/"}},
		description: `<p>This wiki is part of the Gaming Wiki Network, a network of independently-hosted wikis about video game franchises. The GWN was founded on October 21, 2022. It aims to support all gaming communities in building independently-hosted wikis.</p>`,
	},
	"Terraria": {
		links: [][2]string{{"Announcement: New Official Terraria Wiki!", "https://forums.terraria.org/index.php?threads/new-official-terraria-wiki-launches-today.111239/"}, {"In the media", "https://www.pcgamesn.com/terraria/wiki"}},
	},
	"Calamity_Mod": {
		links: [][2]string{{"Announcement: Moving to wiki.gg", "https://www.reddit.com/r/CalamityMod/comments/ts0586/important_calamity_wiki_announcement/"}},
	},
	"ARK": {
		links: [][2]string{{"Announcement: Official Wiki Is Moving!", "https://survivetheark.com/index.php?/forums/topic/657902-official-ark-wiki-feedback/"}, {"Reasons", "https://todo.sr.ht/~cadence/breezewiki-todo/4#event-216613"}, {"Browser Extension", "https://old.reddit.com/r/playark/comments/xe51sy/official_ark_wiki_launched_a_browser_extension_to/"}},
	},
	"Astroneer": {
		links: [][2]string{{"Migration discussion", "https://old.reddit.com/r/Astroneer/comments/z905id/the_official_astroneer_wiki_has_moved_to_wikigg/"}, {"Migration info", "https://astroneer.fandom.com/wiki/Talk:Astroneer_Wiki/Migration_to_Wiki.gg"}},
	},
	"RuneScape": {
		links: [][2]string{{"Leaving Wikia", "https://runescape.wiki/w/Forum:Leaving_Wikia"}, {"In the media", "https://kotaku.com/video-game-wikis-abandon-their-platform-after-year-of-p-1829401866"}, {"Browser Extension", "https://runescape.wiki/w/RuneScape:Finding_the_wikis_with_ease#Extensions"}},
	},
	"Minecraft": {
		links:       [][2]string{{"Post-Move FAQ", "https://minecraft.wiki/w/Minecraft_Wiki:Moving_from_Fandom"}, {"Pre-Move Discussion", "https://minecraft.fandom.com/wiki/Minecraft_Wiki:Moving_from_Fandom"}, {"In the media: PCGamer", "https://www.pcgamer.com/official-minecraft-wiki-editors-so-furious-at-fandoms-degraded-functionality-and-popups-theyre-overwhelmingly-voting-to-leave-the-site/"}, {"In the media: PCGamesN", "https://www.pcgamesn.com/minecraft/wiki-fandom"}},
		description: `<p>The wiki was founded by Citricsquid on July 16th, 2009 as a way to document information from Minecraft. Since November 15th, 2010, it has been hosted by Curse Media. On December 12th, 2018, it moved to Fandom as it purchased Curse Media. Since September 24, 2023, it forked from Fandom and has been hosted by Weird Gloop.</p>`,
	},
	"Tardis": {
		links: [][2]string{{"Forking announcement", "https://tardis.wiki/wiki/Tardis:Forking_announcement"}, {"Discussion on Reddit", "https://old.reddit.com/r/doctorwho/comments/1azxmrl/tardis_wiki_has_regenerated/"}},
	},
	"Rainverse": {
		links: [][2]string{{"Forking announcement", "https://transfem.social/notes/9qsqdkmqi78e01bh"}},
	},
	"Undertale": {
		links:       [][2]string{{"Reasons and community vote", "https://undertale.fandom.com/f/p/4400000000000078624"}, {"Moving announcement", "https://undertale.fandom.com/f/p/4400000000000078715"}, {"About the new wiki", "https://undertale.wiki/w/Undertale_Wiki:About"}},
		description: `<p>Following a community vote on March 26, 2025, most of the wiki's editors have moved to a new host. As a result, this wiki's content may be outdated or incorrect.</p><p>The Undertale &amp; Deltarune Wikis voted to move off their previous host, Fandom. Their reasons included bad user experience, lack of editorial freedom, and disagreement with the sporadic AI Quick Answers that were visible to logged off readers on August 19, 2023. Both wikis moved, and are now independently hosted by the wikis' staff.</p>`,
	},
	"empty": {
		links: [][2]string{{"This wiki doesn't have a description yet. Add one?", "https://docs.breezewiki.com/Reporting_Bugs.html"}},
	},
}

func externalSearchPage(home, title string) string {
	query := "search=" + percentEncodeQuery(title) + "&go=Go"
	base := strings.TrimRight(home, "/")
	if strings.HasSuffix(home, "/") {
		return base + "/Special:Search?" + query
	}
	if idx := strings.Index(base, "/wiki/"); idx >= 0 {
		return base[:idx] + "/wiki/Special:Search?" + query
	}
	if idx := strings.Index(base, "/w/"); idx >= 0 {
		return base[:idx] + "/w/Special:Search?" + query
	}
	u, err := url.Parse(base)
	if err != nil {
		return base + "/Special:Search?" + query
	}
	return u.Scheme + "://" + u.Host + "/Special:Search?" + query
}

func noticeLink(b *strings.Builder, href, label, class string) {
	b.WriteString(`<a href="`)
	b.WriteString(html.EscapeString(href))
	b.WriteByte('"')
	if class != "" {
		b.WriteString(` class="`)
		b.WriteString(class)
		b.WriteByte('"')
	}
	b.WriteByte('>')
	b.WriteString(html.EscapeString(label))
	b.WriteString("</a>")
}

func groupFeedback(b *strings.Builder, group wikiGroup) {
	for i, link := range group.links {
		if i > 0 {
			b.WriteString(" / ")
		}
		noticeLink(b, link[1], link[0], "")
	}
	if len(group.links) != 0 {
		b.WriteString(" / ")
	}
	b.WriteString("This notice is from BreezeWiki")
	b.WriteString(" / ")
	noticeLink(b, "https://docs.breezewiki.com/Reporting_Bugs.html", "Feedback?", "")
}

var noticeTokenRE = regexp.MustCompile(`^[A-Za-z0-9-]{1,50}$`)

func validNotices(tokens []string) []string {
	seen := map[string]bool{}
	var valid []string
	for _, token := range tokens {
		if noticeTokenRE.MatchString(token) && !seen[token] {
			valid = append(valid, token)
			seen[token] = true
		}
	}
	return valid
}

func noticeDismissURL(opts Options) string {
	notices := append([]string{opts.Wiki}, opts.Notices...)
	next := opts.CurrentURL
	if !strings.HasPrefix(next, "/") || strings.HasPrefix(next, "//") {
		next = "/" + opts.Wiki + "/wiki/" + pathForPage(opts)
	}
	theme := opts.Theme
	if theme != "light" && theme != "dark" {
		theme = "default"
	}
	values := url.Values{}
	values.Set("next_location", next)
	values.Set("new_settings", "#s(user-cookies^ "+theme+" ("+strings.Join(validNotices(notices), " ")+"))")
	return "/set-user-settings?" + values.Encode()
}

func noticeSeen(opts Options) bool {
	for _, token := range opts.Notices {
		if token == opts.Wiki {
			return true
		}
	}
	return false
}

func externalWikiNotice(opts Options) string {
	wiki, title := opts.Wiki, opts.Title
	info, ok := independentWikis[wiki]
	if !ok {
		return ""
	}
	group := independentWikiGroups[info.Group]
	goURL := externalSearchPage(info.Home, title)
	var b strings.Builder
	switch info.Banner {
	case "parallel":
		b.WriteString(`<aside class="niwa__parallel"><div class="niwa__summary"><strong class="niwa__header-mini">See also `)
		noticeLink(&b, goURL, info.Name, "")
		b.WriteString(`</strong></div><details class="niwa__details"><summary>More information</summary><p>This topic has multiple communities of editors, some active on the Fandom wiki, others active on `)
		b.WriteString(html.EscapeString(info.Name))
		b.WriteString(".</p><p>For thorough research, be sure to check both communities since they may have different information!</p>")
		b.WriteString(`<p class="niwa__feedback">`)
		groupFeedback(&b, group)
		b.WriteString("</p></details></aside>")
	case "empty":
		b.WriteString(`<aside class="niwa__notice niwa__notice--alt"><h1 class="niwa__header">You will be redirected to `)
		b.WriteString(html.EscapeString(info.Name))
		b.WriteString(`.</h1>`)
		noticeLink(&b, goURL, "Take me there! →", "niwa__go")
		b.WriteString(`<details class="niwa__details"><summary>More information</summary><p>This independent wiki community has its own site separate from Fandom.</p><p class="niwa__feedback" style="text-align: left">`)
		groupFeedback(&b, group)
		b.WriteString("</p></details></aside>")
	default:
		if noticeSeen(opts) {
			b.WriteString(`<aside class="niwa__notice niwa--seen">`)
		} else {
			b.WriteString(`<aside class="niwa__notice">`)
		}
		b.WriteString(`<div class="niwa__summary"><strong class="niwa__header">`)
		b.WriteString(html.EscapeString(info.Name))
		b.WriteString(" has its own website separate from Fandom.</strong>")
		noticeLink(&b, goURL, "Read "+title+" on "+info.Name+" →", "niwa__go")
		b.WriteString(`<a rel="nofollow" class="niwa__got-it" href="`)
		b.WriteString(html.EscapeString(noticeDismissURL(opts)))
		b.WriteString(`">OK, got it</a></div><details class="niwa__details"><summary>More information</summary><div class="niwa__cols"><div class="niwa__left">`)
		b.WriteString(info.Description)
		b.WriteString(group.description)
		b.WriteString("<p>This wiki's core community has largely migrated away from Fandom. You should ")
		noticeLink(&b, goURL, "go to "+info.Name+" now!", "")
		b.WriteString(`</p><p class="niwa__feedback">`)
		for i, link := range group.links {
			if i > 0 {
				b.WriteString(" / ")
			}
			noticeLink(&b, link[1], link[0], "")
		}
		if len(group.links) > 0 {
			b.WriteString(" / ")
		}
		b.WriteString("This notice is from BreezeWiki")
		b.WriteString("</p></div>")
		if info.Logo != nil && *info.Logo != "" {
			b.WriteString(`<div class="niwa__right"><img class="niwa__logo" src="`)
			b.WriteString(html.EscapeString(*info.Logo))
			b.WriteString(`"></div>`)
		}
		b.WriteString("</div></details></aside>")
	}
	return b.String()
}
