import {h, htm, render, signal, computed, effect, useSignalEffect} from "preact"
const html = htm.bind(h)
const classNames = classArr => classArr.filter(el => el).join(" ")

const eForm = document.getElementById("bw-pr-search-form")
const eInput = document.getElementById("bw-pr-search-input")
const eSuggestions = document.getElementById("bw-pr-search-suggestions")

const hitsPromise = new Map()
const hitsDone = new Set()

const query = signal("")
const focus = signal(false)
const st = signal("ready")
const suggestions = signal([])
const enterWasLastKey = signal(false)

// processing functions

function fetchSuggestions(query, setSuggestions) {
	if (query === "") query = "\0"
	if (hitsPromise.has(query)) return hitsPromise.get(query)
	const url = new URL(`https://${BWData.wikiname}.fandom.com/api.php`)
	url.searchParams.set("action", "opensearch")
	url.searchParams.set("format", "json")
	url.searchParams.set("namespace", "0") // wiki namespace, 0 is default
	url.searchParams.set("origin", "*") // mediawiki api cors
	url.searchParams.set("search", query)
	const sendUrl = BWData.strict_proxy
			? "/proxy?" + new URLSearchParams({dest: url})
			: url
	const promise = fetch(sendUrl).then(res => res.json()).then(root => {
		hitsDone.add(query)
		return Array(root[1].length).fill().map((_, i) => ({
			title: root[1][i],
			url: root[3][i]
		}))
	})
	hitsPromise.set(query, promise)
	return promise
}

function acceptSuggestion(hit) {
	st.value = "accepted"
	query.value = hit.title
	const dest = new URL(hit.url).pathname.match("/wiki/.*")
	location = `/${BWData.wikiname}${dest}`
}

// suggestion list view

function Suggestion(hit) {
	useSignalEffect(() => {
		if (enterWasLastKey.value && st.value === "ready") {
			enterWasLastKey.value.preventDefault()
			acceptSuggestion(hit)
		}
	})
	return html`<li class="bw-ss__item"><button type="button" class="bw-ss__button" onClick=${() => acceptSuggestion(hit)}>${hit.title}</button></li>`
}

function DefaultSearch({q}) {
	if (!q) return ""
	return html`<li class="bw-ss__item"><button type="submit" class="bw-ss__button">Results for "${q}"</button></li>`
}

function SuggestionList() {
	return html`
<ul class=${classNames(["bw-ss__list", focus.value && "bw-ss__list--focus", `bw-ss__list--${st.value}`])}>
	${suggestions.value.map(hit => html`<${Suggestion} ...${hit} />`)}
	<${DefaultSearch} q=${query.value} />
</ul>`
}

render(html`<${SuggestionList} />`, eSuggestions)

// input view

effect(() => {
	query.value // dependency should always be tracked
	if (st.peek() === "accepted") return // lock results from changing during navigation
	st.value = "loading"
	fetchSuggestions(query.value).then(res => {
		suggestions.value = res
		if (hitsDone.size === hitsPromise.size) {
			st.value = "ready"
		}
	})
})

window.addEventListener("pageshow", () => {
	st.value = "ready" // unlock results from changing after returning to page
})

effect(() => {
	if (enterWasLastKey.value && (st.value === "loading" || st.value === "accepted")) {
		enterWasLastKey.value.preventDefault() // wait for results before going
	}
})

function SuggestionInput() {
	return html`
<input type="text" name="q" id="bw-search-input" autocomplete="off" onInput=${e => query.value = e.target.value} onKeyDown=${e => enterWasLastKey.value = e.key === "Enter" && e} value=${query.value} class=${classNames(["bw-ss__input", `bw-ss__input--${st.value}`])} />`
}

render(html`<${SuggestionInput} />`, eInput)

// form focus

eForm.addEventListener("focusin", () => focus.value = true)
eForm.addEventListener("focusout", event => {
	if (eForm.contains(event.relatedTarget)) {
		// event fired when changing from one form element to the other
		focus.value = true
	} else {
		// event fired when moving out of the form element
		focus.value = false
	}
})
