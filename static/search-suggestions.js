import {h, htm, render, signal, effect, useLayoutEffect, batch} from "preact"
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
let pendingEnter = false
const selectedIndex = signal(-1)
let focusAfterUpdate = null

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
    return html`<li class="bw-ss__item"><a class=${classNames(["bw-ss__button", selectedIndex.value === index && "bw-ss__button--selected"])} href=${suggestionURL(hit)} onFocus=${() => selectedIndex.value = index}>${hit.title}</a></li>`
}

function DefaultSearch({q}) {
    useLayoutEffect(() => {
        if (focusAfterUpdate === null) return
        const index = focusAfterUpdate
        focusAfterUpdate = null
        eSuggestions.querySelectorAll(".bw-ss__button")[index]?.focus()
    })
    if (!q) return ""
	return html`<li class="bw-ss__item"><button type="submit" onFocus=${() => selectedIndex.value = suggestions.value.length} class=${classNames(["bw-ss__button", selectedIndex.value === suggestions.value.length && "bw-ss__button--selected"])}>Results for "${q}"</button></li>`
}

function SuggestionList() {
	return html`
<ul class=${classNames(["bw-ss__list", focus.value && "bw-ss__list--focus", `bw-ss__list--${st.value}`])}>
	${suggestions.value.map((hit, index) => html`<${Suggestion} key=${hit.url || hit.title} ...${hit} index=${index} />`)}
	<${DefaultSearch} key="default-search" q=${query.value} />
</ul>`
}

render(html`<${SuggestionList} />`, eSuggestions)

// input view

effect(() => {
    const requested = query.value
    if (st.peek() === "accepted") return
    st.value = "loading"
    fetchSuggestions(requested).then(res => {
        if (requested !== query.value) return
        const focused = eSuggestions.querySelector(".bw-ss__button:focus")
        if (focused) {
            const href = focused.getAttribute("href")
            let index = href === null ? res.length : res.findIndex(hit => suggestionURL(hit) === href)
            if (index < 0) index = Math.min(Math.max(selectedIndex.peek(), 0), res.length)
            focusAfterUpdate = index
            batch(() => {
                suggestions.value = res
                selectedIndex.value = index
            })
        } else {
            suggestions.value = res
        }
        st.value = "ready"
        if (pendingEnter) {
            pendingEnter = false
            if (res.length) acceptSuggestion(res[0])
            else eForm.requestSubmit()
        }
    })
})

window.addEventListener("pageshow", event => {
    if (!event.persisted) return
    pendingEnter = false
    st.value = "ready"
})

function onSearchKeyDown(event) {
    if (event.key === "ArrowDown" || event.key === "ArrowUp") {
        pendingEnter = false
        if (st.value !== "ready" || !query.value) return
        event.preventDefault()
        const options = suggestions.value.length + 1
        selectedIndex.value = event.key === "ArrowDown"
            ? (selectedIndex.value + 1) % (options + 1)
            : (selectedIndex.value === -1 ? options - 1 : selectedIndex.value - 1)
        if (selectedIndex.value === options) selectedIndex.value = -1
        if (event.target !== eInput.querySelector("input")) {
            const entries = eSuggestions.querySelectorAll(".bw-ss__button")
            const next = selectedIndex.value === -1 ? eInput.querySelector("input") : entries[selectedIndex.value]
            next?.focus()
        }
        return
    }
    if (event.key !== "Enter" || event.target.closest("a")) return
    if (st.value === "accepted") {
        event.preventDefault()
    } else if (st.value === "loading") {
        event.preventDefault()
        pendingEnter = true
    } else if (selectedIndex.value >= 0) {
        if (selectedIndex.value < suggestions.value.length) {
            event.preventDefault()
            acceptSuggestion(suggestions.value[selectedIndex.value])
        }
    } else if (suggestions.value.length) {
        event.preventDefault()
        acceptSuggestion(suggestions.value[0])
    }
}

function SuggestionInput() {
	return html`
<input type="text" name="q" id="bw-search-input" autocomplete="off" onInput=${e => { pendingEnter = false; selectedIndex.value = -1; query.value = e.target.value }} onKeyDown=${onSearchKeyDown} value=${query.value} class=${classNames(["bw-ss__input", `bw-ss__input--${st.value}`])} />`
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
