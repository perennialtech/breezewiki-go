package server

import (
	"html/template"
	"net/http"
	"net/url"
	"strings"
)

var homeTemplate = template.Must(template.New("home").Parse(`<!DOCTYPE html>
<html lang="en"><head>
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>About | BreezeWiki</title>
<link rel="stylesheet" type="text/css" href="{{.InternalCSS}}">
<link rel="stylesheet" type="text/css" href="{{.MainCSS}}">
<link rel="icon" href="{{.Favicon}}">
</head><body class="skin-fandomdesktop theme-fandomdesktop-light internal">
<div class="main-container"><div class="fandom-community-header__background tileBoth header"></div><div class="page">
<main class="page__main"><div class="custom-top"><h1 class="page-title">About BreezeWiki</h1></div>
<div id="content"><div id="mw-content-text">
<h2>BreezeWiki makes wiki pages on Fandom readable</h2>
<p>It removes ads, videos, and suggested content, leaving you with a clean page that doesn't slow down your device or use up your data.</p>
<p>To use BreezeWiki, just replace "fandom.com" with "{{.Host}}", and you'll instantly be teleported to a better world.</p>
<p>If you'd like to be automatically sent to BreezeWiki every time in the future,
{{if .PromoteExtension}}<a href="https://getindie.wiki">get our affiliated browser extension (NEW!)</a> or {{end}}
<a href="https://docs.breezewiki.com/Automatic_Redirection.html">check out the tutorial in the manual.</a></p>
<p>BreezeWiki is available on several different websites called <a href="https://en.wikipedia.org/wiki/Mirror_site">mirrors</a>. Each is independently run. If one mirror is offline, the others still work. <a href="https://docs.breezewiki.com/Links.html#%28part._.Mirrors%29">See the list.</a></p>
<h2>Find a page</h2>
<form action="/search">
<label class="paired__label">Wiki name<input name="wikiname" class="paired__input" type="text" placeholder="pokemon" required></label>
<label class="paired__label">Search query<input name="q" class="paired__input" type="text" placeholder="Eevee"></label>
<button>Search</button>
</form>
<h2>Example pages</h2>
<ul><li><a href="/minecraft/wiki/Bricks">minecraft: Bricks</a></li>
<li><a href="/crosscode/wiki/CrossCode_Wiki">crosscode: CrossCode Wiki</a></li>
<li><a href="/undertale/wiki/Hot_Dog...%3F">undertale: Hot Dog...?</a></li>
<li><a href="/tardis/wiki/Eleanor_Blake">tardis: Eleanor Blake</a></li>
<li><a href="/zelda/wiki/Boomerang">zelda: Boomerang</a></li></ul>
<h2>Testimonials</h2>
<p class="testimonial">&gt;so glad someone introduced me to a F*ndom alternative (BreezeWiki) because that x-factorized spillway of an ad-infested radioactive dumpsite can go die in a fire —RB</p>
<p class="testimonial">&gt;apparently there are thousands of people essentially running our company <em>for free</em> right now, creating tons of content, and we just put ads on top of it and they're not even employees. thousands of people we can't lay off. thousands! —<a href="https://hard-drive.net/fandom-ceo-frustrated-its-impossible-to-lay-off-unpaid-users-who-update-wikias-for-fun/?utm_source=breezewiki" target="_blank">Perkins Miller, Fandom CEO</a></p>
<p class="testimonial">&gt;attempting to go to a wiki's forum page with breezewiki doesn't work, which is based honestly —Tom Skeleton</p>
<p class="testimonial">&gt;Fandom pages crashing and closing, taking forever to load and locking up as they load the ads on the site... they are causing the site to crash because they are trying to load video ads both at the top and bottom of the site as well as two or three banner ads, then a massive top of site ad and eventually my anti-virus shuts the whole site down because it's literally pulling more resources than WoW in ultra settings... —Anonymous</p>
<p class="testimonial">&gt;reblogs EXTREMELY appreciated I want that twink* (*fandom wiki) obliterated —footlong</p>
<h2>What BreezeWiki isn't</h2>
<p>BreezeWiki isn't an "alternative" to Fandom, and it doesn't let you edit or write new pages.</p>
<p>If you want to create your own wiki, try Miraheze!</p>
</div></div>
<footer class="custom-footer"><div class="internal-footer"><div>
<p><img class="my-logo" src="{{.Logo}}" alt="BreezeWiki"></p>
<p><a href="https://gitdab.com/cadence/breezewiki">{{.ApplicationName}} source code</a></p>
<p><a href="https://docs.breezewiki.com">Documentation and more information</a></p>
<p><a href="https://lists.sr.ht/~cadence/breezewiki-discuss">Chat / Bug reports / Feature requests</a></p>
{{if .PromoteFooter}}<p><a href="https://getindie.wiki/">Get Indie Wiki Buddy browser extension - be redirected to BreezeWiki every time!</a></p>{{end}}
{{if .Official}}<p>This instance is run by the {{.ApplicationName}} developer, <a href="https://cadence.moe/contact">Cadence</a>.</p>
{{else}}<p>This unofficial instance is based off the {{.ApplicationName}} source code, but is not controlled by the code developer.</p>{{end}}
</div><div><p>Text content on wikis run by Fandom is available under the Creative Commons Attribution-Share Alike License 3.0 (Unported), <a href="https://www.fandom.com/licensing" rel="nofollow">see license info.</a> Media files and official Fandom documents have different copying restrictions.</p>
<p>Fandom is a trademark of Fandom, Inc. {{.ApplicationName}} is not affiliated with Fandom.</p></div></div></footer>
</main></div></div></body></html>`))

func (s *Server) home(w http.ResponseWriter) {
	host := "breezewiki.com"
	if raw := s.Config.Get("canonical_origin"); raw != "" {
		if u, err := url.Parse(raw); err == nil && u.Host != "" {
			host = u.Host
		}
	}
	var buf strings.Builder
	err := homeTemplate.Execute(&buf, map[string]any{
		"InternalCSS":      s.staticURL("internal.css"),
		"MainCSS":          s.staticURL("main.css"),
		"Favicon":          s.staticURL("breezewiki-favicon.svg"),
		"Logo":             s.staticURL("breezewiki.svg"),
		"Host":             host,
		"ApplicationName":  s.Config.Get("application_name"),
		"PromoteExtension": s.Config.Member("promotions::indie_wiki_buddy", "home"),
		"PromoteFooter":    s.Config.Member("promotions::indie_wiki_buddy", "footer"),
		"Official":         s.Config.True("instance_is_official"),
	})
	if err != nil {
		s.internalError(w, err)
		return
	}
	s.html(w, 200, buf.String())
}
