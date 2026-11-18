function setUpTabber(tabber) {
    const tabs = [...tabber.querySelectorAll(":scope > .wds-tabs__wrapper .wds-tabs__tab")]
    const panels = [...tabber.querySelectorAll(":scope > .wds-tab__content")]
    if (!tabs.length || tabs.length !== panels.length) return

    const active = tabs.findIndex(tab => tab.classList.contains("wds-is-current"))
    function select(index, updateHash = false) {
        tabs.forEach((tab, i) => {
            const selected = i === index
            tab.classList.toggle("wds-is-current", selected)
            tab.setAttribute("aria-selected", selected)
            tab.tabIndex = selected ? 0 : -1
            panels[i].classList.toggle("wds-is-current", selected)
            panels[i].hidden = !selected
        })
        if (updateHash && tabs[index].dataset.hash) {
            const url = new URL(location.href)
            url.hash = tabs[index].dataset.hash
            history.replaceState(null, "", url)
        }
    }

    tabs.forEach((tab, i) => {
        tab.setAttribute("role", "tab")
        tab.addEventListener("click", event => {
            event.preventDefault()
            select(i, true)
        })
        tab.addEventListener("keydown", event => {
            let target
            switch (event.key) {
                case "ArrowLeft": target = (i + tabs.length - 1) % tabs.length; break
                case "ArrowRight": target = (i + 1) % tabs.length; break
                case "Home": target = 0; break
                case "End": target = tabs.length - 1; break
                case "Enter":
                case " ": target = i; break
                default: return
            }
            event.preventDefault()
            select(target, true)
            tabs[target].focus()
        })
    })
    select(Math.max(0, active))
    return {tabs, panels, select}
}

let tabbers = []

function showTabFromHash() {
    if (!location.hash) return
    let hash
    try {
        hash = decodeURIComponent(location.hash.slice(1))
    } catch {
        return
    }

    let anchor = document.getElementById(hash)
    for (const {tabs, panels, select} of tabbers) {
        const index = tabs.findIndex(tab => tab.dataset.hash === hash)
        if (index >= 0) {
            select(index)
            anchor = tabs[index]
            break
        }
    }
    if (!anchor) return
    let changed = false
    for (const {panels, select} of tabbers) {
        const panel = panels.findIndex(panel => panel.contains(anchor))
        if (panel >= 0 && panels[panel].hidden) {
            select(panel)
            changed = true
        }
    }
    if (changed || anchor.matches(".wds-tabs__tab")) anchor.scrollIntoView()
}

function initTabs() {
    const content = document.getElementById("mw-content-text")
    if (!content || content.dataset.bwTabsReady) return
    content.dataset.bwTabsReady = "1"
    tabbers = [...content.querySelectorAll(".wds-tabber")].map(setUpTabber).filter(Boolean)
    document.body.classList.remove("bw-tabs-nojs")
    showTabFromHash()
}

initTabs()
document.addEventListener("breezewiki:content-loaded", initTabs)
window.addEventListener("hashchange", showTabFromHash)
