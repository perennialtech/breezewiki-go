function initComments() {
    const section = document.getElementById("comments")
    if (!section || section.dataset.initialized) return
    section.dataset.initialized = "true"
    const results = section.querySelector(".bw-comments__results")
    async function show(page) {
        const url = new URL(section.dataset.commentsUrl, location.href)
        url.searchParams.set("page", page)
        results.textContent = "Loading comments…"
        try {
            const response = await fetch(url)
            if (!response.ok) throw new Error(`HTTP ${response.status}`)
            results.innerHTML = await response.text()
        } catch {
            results.textContent = "Comments are unavailable right now."
        }
    }
    section.addEventListener("click", event => {
        const button = event.target.closest("button[data-comments-page]")
        if (!button || !section.contains(button)) return
        show(button.dataset.commentsPage)
    })
    show(new URLSearchParams(location.search).get("comments_page") || "1")
}

initComments()
document.addEventListener("breezewiki:content-loaded", initComments)
