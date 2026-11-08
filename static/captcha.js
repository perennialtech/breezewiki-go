const u = new URL(location)
const from = u.searchParams.get("from") || location.href
let answered = false

const area = document.getElementById("captcha-area")
const areaBox = area.getBoundingClientRect()
const width = Math.floor(areaBox.width)
const height = Math.floor(window.innerHeight - areaBox.bottom - areaBox.left)
const img = document.createElement("img")
img.src = `/captcha/img/${width}/${height}`
img.addEventListener("click", event => {
	if (answered) return
	answered = true
	location = `/captcha/verify/${width}/${height}/${event.offsetX}/${event.offsetY}?` + new URLSearchParams({from})
})
area.appendChild(img)

document.addEventListener("keydown", event => {
	if (event.repeat) {
		if (answered) return
		answered = true
		location = `/captcha/verify/0/0/${event.key}/0?` + new URLSearchParams({from})
	}
})
