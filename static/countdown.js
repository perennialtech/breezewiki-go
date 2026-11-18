const units = [
    ["w", 7 * 86400],
    ["d", 86400],
    ["h", 3600],
    ["m", 60],
    ["s", 1],
]

function displayTime(seconds) {
    const parts = []
    for (const [label, size] of units) {
        const count = Math.floor(seconds / size)
        seconds %= size
        if (count || parts.length || size === 1) parts.push(count + label)
    }
    return parts.join(" ")
}

let interval

function initCountdowns() {
    clearInterval(interval)
    const timers = []
    for (const element of document.querySelectorAll("#mw-content-text .countdown")) {
        const date = element.querySelector(".countdowndate")
        const end = Date.parse(date?.textContent)
        if (!Number.isFinite(end)) continue
        const next = element.nextElementSibling
        const fallback = next?.matches(".nocountdown") ? next : next?.nextElementSibling?.matches(".nocountdown") ? next.nextElementSibling : null
        const replacement = element.dataset.toggle === "next" && next?.querySelector(".countdown") ? next : null
        element.classList.remove("hidden")
        element.style.removeProperty("display")
        if (fallback) fallback.hidden = true
        timers.push({element, date, end, replacement, fallback})
    }
    if (!timers.length) return

    const toggle = document.createElement("button")
    toggle.type = "button"
    toggle.className = "bw-countdown-toggle"
    toggle.textContent = "Pause countdowns"
    toggle.setAttribute("aria-pressed", "false")
    const first = timers[0].element
    const anchor = first.closest('[class^="gi-countdown-"], [class*=" gi-countdown-"]') ?? first
    anchor.before(toggle)

    function update() {
        const now = Date.now()
        let active = false
        for (const timer of timers) {
            const {element, date, end, replacement, fallback} = timer
            const seconds = Math.ceil((end - now) / 1000)
            if (seconds > 0) {
                date.textContent = displayTime(seconds)
                active = true
                continue
            }
            if (element.dataset.end === "remove" || replacement) {
                element.hidden = true
                if (replacement) {
                    replacement.classList.remove("hidden")
                    replacement.style.removeProperty("display")
                }
            } else {
                date.textContent = fallback ? fallback.textContent.trim() : new Date(end).toLocaleString()
            }
        }
        if (!active) clearInterval(interval)
    }

    toggle.addEventListener("click", () => {
        const paused = toggle.getAttribute("aria-pressed") === "true"
        toggle.setAttribute("aria-pressed", !paused)
        if (paused) {
            interval = setInterval(update, 1000)
            update()
        } else {
            clearInterval(interval)
        }
    })

    interval = setInterval(update, 1000)
    update()
}

initCountdowns()
document.addEventListener("breezewiki:content-loaded", initCountdowns)
