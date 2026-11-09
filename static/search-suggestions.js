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
const selectedIndex = signal(-1)

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

function suggestionURL(hit) {
	const url = new URL(hit.url)
	return `/${BWData.wikiname}${url.pathname}${url.search}${url.hash}`
}

function acceptSuggestion(hit) {
	st.value = "accepted"
	query.value = hit.title
	location = suggestionURL(hit)
}

// suggestion list view

function Suggestion({index, ...hit}) {
	useSignalEffect(() => {
		if (index === 0 && selectedIndex.value === -1 && enterWasLastKey.value && st.value === "ready") {
			enterWasLastKey.value.preventDefault()
			acceptSuggestion(hit)
		}
	})
	return html`<li class="bw-ss__item"><a class=${classNames(["bw-ss__button", selectedIndex.value === index && "bw-ss__button--selected"])} href=${suggestionURL(hit)} onFocus=${() => selectedIndex.value = index}>${hit.title}</a></li>`
}

function DefaultSearch({q}) {
	if (!q) return ""
	return html`<li class="bw-ss__item"><button type="submit" onFocus=${() => selectedIndex.value = suggestions.value.length} class=${classNames(["bw-ss__button", selectedIndex.value === suggestions.value.length && "bw-ss__button--selected"])}>Results for "${q}"</button></li>`
}

function SuggestionList() {
	return html`
<ul class=${classNames(["bw-ss__list", focus.value && "bw-ss__list--focus", `bw-ss__list--${st.value}`])}>
	${suggestions.value.map((hit, index) => html`<${Suggestion} ...${hit} index=${index} />`)}
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
        const resultsFocused = eSuggestions.querySelector("button:focus") !== null
		suggestions.value = res
        if (resultsFocused) selectedIndex.value = res.length
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

function onSearchKeyDown(event) {
	if (event.key === "ArrowDown" || event.key === "ArrowUp") {
		if (st.value !== "ready" || !query.value) return
		event.preventDefault()
		const options = suggestions.value.length + 1 // includes "Results for ..."
		selectedIndex.value = event.key === "ArrowDown"
			? (selectedIndex.value + 1) % (options + 1)
			: (selectedIndex.value === -1 ? options - 1 : selectedIndex.value - 1)
        if (selectedIndex.value === options) selectedIndex.value = -1
        if (event.target !== eInput.querySelector("input")) {
            const options = eSuggestions.querySelectorAll(".bw-ss__button")
            const next = selectedIndex.value === -1 ? eInput.querySelector("input") : options[selectedIndex.value]
            next?.focus()
        }
		return
	}
	if (event.key === "Enter" && selectedIndex.value >= 0) {
		enterWasLastKey.value = false
		if (selectedIndex.value < suggestions.value.length) {
			event.preventDefault()
			acceptSuggestion(suggestions.value[selectedIndex.value])
		}
		return // allow the "Results for ..." entry to submit the form
	}
	enterWasLastKey.value = event.key === "Enter" && event
}

function SuggestionInput() {
	return html`
<input type="text" name="q" id="bw-search-input" autocomplete="off" onInput=${e => { selectedIndex.value = -1; query.value = e.target.value }} onKeyDown=${onSearchKeyDown} value=${query.value} class=${classNames(["bw-ss__input", `bw-ss__input--${st.value}`])} />`
}

render(html`<${SuggestionInput} />`, eInput)
eSuggestions.addEventListener("keydown", onSearchKeyDown)

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
