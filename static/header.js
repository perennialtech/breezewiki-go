const menus = [...document.querySelectorAll(".bw-header__menu")]

document.addEventListener("click", event => {
    for (const menu of menus) {
        if (!menu.contains(event.target)) menu.open = false
    }
})

document.addEventListener("focusin", event => {
    for (const menu of menus) {
        if (!menu.contains(event.target)) menu.open = false
    }
})

document.addEventListener("keydown", event => {
    if (event.key !== "Escape" || event.defaultPrevented) return
    const menu = menus.find(menu => menu.open)
    if (menu) {
        menu.open = false
        menu.querySelector("summary").focus()
        event.preventDefault()
    }
})
