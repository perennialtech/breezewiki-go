package render

import "strings"

const (
	homeIcon    = `<path d="m3 10 9-7 9 7v10a1 1 0 0 1-1 1h-5v-7H9v7H4a1 1 0 0 1-1-1z"/>`
	browseIcon  = `<rect x="3.5" y="4" width="5" height="5" rx="1"/><rect x="3.5" y="15" width="5" height="5" rx="1"/><path d="M13 6.5h7M13 17.5h7"/>`
	searchIcon  = `<circle cx="10.5" cy="10.5" r="6"/><path d="m15 15 5 5"/>`
	historyIcon = `<path d="M3 11a9 9 0 1 1 1.5 6M3 5v6h6m3-4v5l3 2"/>`
)

func headerIcon(path string) string {
	return `<svg class="bw-header__icon" aria-hidden="true" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.75" stroke-linecap="round" stroke-linejoin="round">` + path + `</svg>`
}

func writeHeader(b *strings.Builder, opts Options, theme string) {
	prefix := "/" + opts.Wiki
	home := prefix + "/"
	if opts.Lang != "" {
		prefix += "/" + opts.Lang
		home = prefix + "/wiki"
	}
	b.WriteString(`<header class="bw-header"><div class="bw-header__inner">
	<a class="bw-header__control" aria-label="Wiki home" title="Wiki home" href="` + escaped(home) + `">`)
	b.WriteString(headerIcon(homeIcon) + `<span class="bw-header__label">Wiki home</span></a>
	<details class="bw-header__menu bw-header__browse" name="bw-header-menu">
	<summary class="bw-header__control" aria-label="Browse" title="Browse">` + headerIcon(browseIcon) + `<span class="bw-header__label">Browse</span></summary>
	<nav class="bw-header__panel" aria-label="Wiki navigation">
	<a class="bw-header__panel-link" href="` + escaped(prefix) + `/wiki/Special:AllPages">All pages</a>
	<div id="bw-wiki-navigation">` + opts.WikiNavigationHTML + `</div></nav></details>`)

	b.WriteString(`<form action="` + escaped(prefix) + `/search" role="search" class="bw-search-form" id="bw-pr-search-form">
	<label class="bw-visually-hidden" for="bw-search-input">Search this wiki</label>
	<button class="bw-search-form__submit" type="submit" aria-label="Submit search">` + headerIcon(searchIcon) + `</button>
	<div id="bw-pr-search-input"><input type="text" name="q" id="bw-search-input" placeholder="Search" autocomplete="off"></div>
	<div class="bw-ss__container" id="bw-pr-search-suggestions"></div></form>`)

	if href := historyURL(opts); href != "" && !strings.Contains(opts.Path, "/wiki/Special:") && opts.SourceURL != "" {
		b.WriteString(`<a class="bw-header__control" aria-label="History" title="History" rel="nofollow" href="` + escaped(href) + `">`)
		b.WriteString(headerIcon(historyIcon) + `<span class="bw-header__label">History</span></a>`)
	}

	b.WriteString(`<details class="bw-header__menu bw-header__theme" name="bw-header-menu">
	<summary class="bw-header__control" aria-label="Theme" title="Theme"><span class="bw-theme__icon" aria-hidden="true">`)
	b.WriteString(themeIcons[theme])
	b.WriteString(`</span><span class="bw-header__label">Theme</span></summary>
	<div class="bw-header__panel">`)
	for _, option := range []struct{ value, label string }{{"default", "Wiki default"}, {"light", "Light"}, {"dark", "Dark"}} {
		b.WriteString(`<a rel="nofollow" href="` + escaped(themeLink(opts, option.value)) + `"`)
		if option.value == theme {
			b.WriteString(` aria-current="true"`)
		}
		b.WriteString(` class="bw-header__panel-link bw-theme__item"><span class="bw-theme__icon" aria-hidden="true">`)
		b.WriteString(themeIcons[option.value] + "</span>" + option.label + "</a>")
	}
	b.WriteString("</div></details></div></header>")
}
