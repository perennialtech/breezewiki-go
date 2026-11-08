import {effect} from "preact"
import {loaded} from "jsonp"

const tabFromHash = location.hash.length > 1 ? location.hash.substring(1) : null

function setUpAllTabs() {
	for (const tabber of document.body.querySelectorAll(".wds-tabber")) {
		for (const [tab, content] of getTabberTabs(tabber)) {
			// set up click listener on every tab
			tab.addEventListener("click", e => {
				setCurrentTab(tabber, tab, content)
				e.preventDefault()
			})

			// re-open a specific tab on page load based on the URL hash
			if (tab.dataset.hash === tabFromHash) {
				setCurrentTab(tabber, tab, content)
				tab.scrollIntoView()
			}
		}
	}
}

function getTabberTabs(tabber) {
	// need to scope the selector to handle nested tabs. see /unturned/wiki/Crate for an example
	const tabs = [...tabber.querySelectorAll(":scope > .wds-tabs__wrapper .wds-tabs__tab")]
	const contents = [...tabber.querySelectorAll(":scope > .wds-tab__content")]
	return tabs.map((_, index) => [tabs[index], contents[index]]) // transpose arrays into [[tab, content], ...]
}

function setCurrentTab(tabber, tab, content) {
	// clear currently selected tab
	getTabberTabs(tabber).flat().forEach(e => e.classList.remove("wds-is-current"))

	// select new tab
	tab.classList.add("wds-is-current")
	content.classList.add("wds-is-current")
	if (tab.dataset.hash) {
		history.replaceState(null, "", `#${tab.dataset.hash}`)
	}
}

if (!BWData.jsonp) {
	setUpAllTabs()
} else if (loaded.value) {
	setUpAllTabs()
} else {
	effect(() => {
		if (loaded.value) {
			setUpAllTabs()
		}
	})
}

document.body.classList.remove("bw-tabs-nojs")
