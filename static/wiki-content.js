function toggleControl(control, expanded, change) {
    if (!control.matches("button, a[href], input")) {
        control.role = "button"
        control.tabIndex = 0
        control.addEventListener("keydown", event => {
            if (event.target === control && ["Enter", " "].includes(event.key)) {
                event.preventDefault()
                control.click()
            }
        })
    }
    control.setAttribute("aria-expanded", expanded)
    control.addEventListener("click", event => {
        if (event.target !== control && event.target.closest("a[href], button, input")) return
        event.preventDefault()
        expanded = !expanded
        control.setAttribute("aria-expanded", expanded)
        change(expanded)
    })
}

function initCollapsibles(root) {
    for (const box of root.querySelectorAll(".mw-collapsible:not([id^='mw-customcollapsible-'])")) {
        if (box.closest(".custom-tt-wrapper")) continue
        const rows = box.matches("table") ? [...box.rows] : []
        const header = rows[0] ?? box
        let control = header.querySelector(".mw-collapsible-toggle")
        if (control?.closest(".mw-collapsible") !== box) control = null
        const children = rows.length ? rows.slice(1) : [...box.children].filter(child => child !== control)
        if (!children.length) continue
        if (!control) {
            control = document.createElement("button")
            control.type = "button"
            control.className = "bw-collapse-button"
            if (rows.length) {
                if (!header.cells.length) continue
                header.cells[0].prepend(control)
            } else {
                box.prepend(control)
            }
        }
        const update = expanded => {
            for (const child of children) {
                if (child.contains(control)) continue
                child.classList.toggle("bw-content-hidden", !expanded)
            }
            if (control.classList.contains("bw-collapse-button")) {
                control.textContent = expanded ? "Collapse" : "Expand"
            }
        }
        const expanded = !box.classList.contains("mw-collapsed")
        toggleControl(control, expanded, update)
        update(expanded)
    }

    const customTargets = new Map()
    for (const target of root.querySelectorAll("[id^='mw-customcollapsible-']")) {
        const name = target.id.slice("mw-customcollapsible-".length)
        if (!customTargets.has(name)) customTargets.set(name, [])
        customTargets.get(name).push(target)
    }
    for (const control of root.querySelectorAll("[class*='mw-customtoggle-']")) {
        const name = [...control.classList].find(name => name.startsWith("mw-customtoggle-"))
        const targets = customTargets.get(name?.slice("mw-customtoggle-".length))
        if (!targets) continue
        const update = expanded => {
            for (const target of targets) target.classList.toggle("bw-content-hidden", !expanded)
        }
        const expanded = targets.every(target => !target.classList.contains("mw-collapsed"))
        toggleControl(control, expanded, update)
        update(expanded)
    }
}

const activeTooltips = new Set()

document.addEventListener("keydown", event => {
    if (event.key !== "Escape" || event.defaultPrevented || !activeTooltips.size) return
    for (const dismiss of activeTooltips) dismiss()
    event.preventDefault()
})

function initTooltips(root) {
    for (const [index, box] of [...root.querySelectorAll(".custom-tt-wrapper")].entries()) {
        const control = box.querySelector(":scope > .mw-collapsible-toggle")
        const body = box.querySelector(":scope > .mw-collapsible-content")
        if (!control || !body) continue
        box.classList.add("bw-tooltip")
        body.style.removeProperty("display")
        if (!body.id) body.id = `bw-tooltip-${index}`
        control.setAttribute("aria-controls", body.id)
        if (control.textContent.trim() === "ⓘ") control.setAttribute("aria-label", "More information")

        let pinned = false
        let hovered = false
        let focused = false
        let dismissed = false
        function update() {
            const open = !dismissed && (pinned || hovered || focused)
            control.setAttribute("aria-expanded", open)
            box.classList.toggle("bw-tooltip-visible", open)
            if (!open) {
                activeTooltips.delete(dismiss)
                return
            }
            activeTooltips.add(dismiss)
            body.style.removeProperty("--bw-tooltip-shift")
            const {left, right} = body.getBoundingClientRect()
            const edge = document.documentElement.clientWidth - 8
            const shift = left < 8 ? 8 - left : Math.min(0, edge - right)
            body.style.setProperty("--bw-tooltip-shift", `${shift}px`)
        }
        function dismiss() {
            if (pinned) control.click()
            if (body.contains(document.activeElement)) control.focus({preventScroll: true})
            dismissed = true
            update()
        }
        toggleControl(control, false, expanded => {
            pinned = expanded
            dismissed = !expanded && (hovered || focused)
            update()
        })
        box.addEventListener("pointerenter", event => {
            if (event.pointerType !== "mouse") return
            hovered = true
            dismissed = false
            update()
        })
        box.addEventListener("pointerleave", () => {
            hovered = false
            update()
        })
        box.addEventListener("focusin", event => {
            focused = true
            if (!box.contains(event.relatedTarget)) dismissed = false
            update()
        })
        box.addEventListener("focusout", event => {
            if (box.contains(event.relatedTarget)) return
            focused = false
            update()
        })
    }
}

function filterQuery(scope, selector) {
    if (typeof selector !== "string" || !selector) return []
    // The wiki's filter definitions use jQuery's :contains inside :has.
    const textMatch = selector.match(/^([\w.-]+):has\((.+):contains\(([^()]*)\)\)$/)
    try {
        if (!textMatch) return [...scope.querySelectorAll(selector)]
        const [, target, child, text] = textMatch
        return [...scope.querySelectorAll(target)].filter(element =>
            [...element.querySelectorAll(child)].some(match => match.textContent.includes(text)))
    } catch {
        return []
    }
}

function initFilters(root) {
    for (const settings of root.querySelectorAll(".fl-wrapper > .fl-sett")) {
        let groups
        try {
            groups = JSON.parse(settings.textContent)
        } catch {
            continue
        }
        if (!Array.isArray(groups)) continue
        const wrapper = settings.parentElement
        const filters = document.createElement("div")
        filters.className = "bw-filters"
        const rows = new Set()
        const predicates = []
        for (const group of groups) {
            if (typeof group?.label !== "string") continue
            if (group.search) {
                const {query, source, attr, placeholder} = group.search
                const label = document.createElement("label")
                const input = document.createElement("input")
                input.type = "search"
                input.placeholder = typeof placeholder === "string" ? placeholder : group.label
                const names = new Map()
                for (const row of filterQuery(wrapper, query)) {
                    const field = filterQuery(row, source)[0]
                    if (!field) continue
                    names.set(row, (typeof attr === "string" ? (field.getAttribute(attr) ?? field.textContent) : field.textContent).trim().toLowerCase())
                    rows.add(row)
                }
                if (!names.size) continue
                predicates.push(row => !names.has(row) || names.get(row).includes(input.value.trim().toLowerCase()))
                label.append(group.label, input)
                filters.append(label)
            }
            if (!Array.isArray(group.toggles)) continue
            const fieldset = document.createElement("fieldset")
            const legend = document.createElement("legend")
            legend.textContent = group.label
            fieldset.append(legend)
            const options = []
            for (const option of group.toggles) {
                const matches = new Set(filterQuery(wrapper, option?.query))
                const alternate = typeof option?.alt === "string" ? option.alt.replace(/^Toggle /, "").replace(/ characters$/, "") : ""
                const text = option?.label ?? alternate
                if (typeof text !== "string" || !text.trim()) continue
                const label = document.createElement("label")
                const input = document.createElement("input")
                input.type = "checkbox"
                label.append(input, text.trim())
                fieldset.append(label)
                options.push({input, matches})
                for (const row of matches) rows.add(row)
            }
            if (!options.length) continue
            predicates.push(row => {
                const selected = options.filter(option => option.input.checked)
                return !selected.length || selected.some(option => option.matches.has(row))
            })
            filters.append(fieldset)
        }
        if (!predicates.length) continue
        const update = () => {
            for (const row of rows) {
                row.classList.toggle("bw-filter-hidden", !predicates.every(match => match(row)))
            }
        }
        filters.addEventListener("input", update)
        settings.after(filters)
    }
}

function initMedia(root) {
    for (const wrapper of root.querySelectorAll(".audio-button")) {
        const link = wrapper.querySelector("a.internal[href]")
        if (!link) continue
        const src = trustedMedia(link.href)
        if (!src) continue
        const audio = document.createElement("audio")
        audio.controls = true
        audio.preload = "none"
        audio.src = src
        audio.setAttribute("aria-label", link.title || "Play audio")
        wrapper.classList.remove("hidden")
        wrapper.style.removeProperty("display")
        wrapper.replaceChildren(audio)
    }
    for (const link of root.querySelectorAll("a.embedvideo-evl[data-video-json]")) {
        let video
        try {
            video = JSON.parse(link.dataset.videoJson)
        } catch {
            continue
        }
        if (video?.service !== "youtube" || !/^[\w-]{11}$/.test(video.id)) continue
        const watch = new URL("https://www.youtube.com/watch")
        watch.searchParams.set("v", video.id)
        if (/^\d+$/.test(video.start)) watch.searchParams.set("t", video.start)
        link.href = watch.href
    }
}

function trustedMedia(href) {
    const url = new URL(href)
    if (url.origin === "https://static.wikia.nocookie.net") return url.href
    if (url.origin !== location.origin || url.pathname !== "/proxy") return ""
    return url.searchParams.get("dest")?.startsWith("https://static.wikia.nocookie.net/") ? url.href : ""
}

function initImageViewer(root) {
    const selector = "a.image[href]:has(img[data-image-key]), a.lightbox[href]:has(img[data-image-key])"
    root.addEventListener("click", event => {
        if (event.defaultPrevented || event.button !== 0 || event.ctrlKey || event.metaKey || event.shiftKey || event.altKey) return
        const link = event.target.closest(selector)
        if (!link || link.classList.contains("video-thumbnail") || !trustedMedia(link.href)) return
        const gallery = link.closest(".wikia-gallery, .gallery")
        const images = gallery ? [...gallery.querySelectorAll(selector)].filter(item =>
            !item.classList.contains("video-thumbnail") && trustedMedia(item.href)) : [link]
        let index = images.indexOf(link)
        if (index < 0) return
        event.preventDefault()

        const dialog = document.createElement("dialog")
        dialog.className = "bw-image-viewer"
        dialog.setAttribute("aria-label", "Image viewer")
        const figure = document.createElement("figure")
        const image = document.createElement("img")
        const caption = document.createElement("figcaption")
        const status = document.createElement("div")
        status.className = "bw-visually-hidden"
        status.setAttribute("role", "status")
        const controls = document.createElement("div")
        const original = document.createElement("a")
        original.textContent = "Open original"
        const close = document.createElement("button")
        close.textContent = "Close"
        close.autofocus = true
        close.addEventListener("click", () => dialog.close())
        const previous = document.createElement("button")
        previous.textContent = "Previous"
        const next = document.createElement("button")
        next.textContent = "Next"
        function show(change = 0) {
            index = (index + change + images.length) % images.length
            const item = images[index]
            const preview = item.querySelector("img")
            image.src = item.href
            image.alt = preview.alt
            caption.textContent = item.closest(".wikia-gallery-item, .gallerybox, figure")?.querySelector(
                ".lightbox-caption, .gallerytext, figcaption")?.textContent.trim() || preview.alt
            original.href = item.href
            if (change) status.textContent = `Image ${index + 1} of ${images.length}. ${caption.textContent}`
        }
        if (images.length > 1) {
            previous.addEventListener("click", () => show(-1))
            next.addEventListener("click", () => show(1))
            dialog.addEventListener("keydown", key => {
                if (key.key === "ArrowLeft") show(-1)
                if (key.key === "ArrowRight") show(1)
            })
            controls.append(previous, next)
        }
        controls.append(original, close)
        figure.append(image, caption)
        dialog.append(controls, figure, status)
        dialog.addEventListener("close", () => {
            dialog.remove()
            link.focus({preventScroll: true})
        }, {once: true})
        root.append(dialog)
        show()
        dialog.showModal()
    })
}

function initContent() {
    const root = document.getElementById("mw-content-text")
    if (!root || root.dataset.bwInteractive) return
    root.dataset.bwInteractive = "1"
    initCollapsibles(root)
    initTooltips(root)
    initFilters(root)
    initMedia(root)
    initImageViewer(root)
}

initContent()
document.addEventListener("breezewiki:content-loaded", initContent)
