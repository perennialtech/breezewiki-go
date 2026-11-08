// countdown timer for gacha enthusiasts
// sample: bandori/wiki/BanG_Dream!_Wikia
// sample: ensemble-stars/wiki/The_English_Ensemble_Stars_Wiki

import {h, htm, render, signal, computed, effect} from "preact"
const html = htm.bind(h)

const now = signal(Date.now())
setInterval(() => now.value = Date.now(), 1000)

const units = [
	["w", 7*24*60*60*1000],
	["d", 24*60*60*1000],
	["h", 60*60*1000],
	["m", 60*1000],
	["s", 1000]
]

function getDisplayTime(datetime, now, or) {
	let difference = datetime - now
	let foundSignificantField = false
	if (difference > 0) {
		return units.map(([letter, duration], index) => {
			const multiplier = Math.floor(difference / duration)
			difference -= multiplier * duration
			if (multiplier > 0 || foundSignificantField) {
				foundSignificantField = true
				return multiplier + letter
			}
		}).filter(s => s).join(" ")
	} else if (or) {
		return or
	} else {
		return `[timer ended on ${new Date(datetime).toLocaleString()}]`
	}
}

function Countdown(props) {
	return html`<span>${props.display}</span>`
}

document.querySelectorAll(".countdown").forEach(eCountdown => {
	// grab information and make variables
	const eDate = eCountdown.querySelector(".countdowndate")
	const eOr = eCountdown.nextElementSibling
	const or = eOr?.textContent
	const datetime = new Date(eDate.textContent).getTime()
	// the mapped signal
	const display = computed(() => getDisplayTime(datetime, now.value, or))
	// clear content and render
	while (eDate.childNodes[0] !== undefined) eDate.childNodes[0].remove()
	render(html`<${Countdown} display=${display} />`, eDate);
})
